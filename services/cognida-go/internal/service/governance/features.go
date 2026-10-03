package governance

import (
	"bytes"
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"math"
	"strconv"
	"time"
)

type featureDefinition struct {
	Kind        string `json:"kind"`
	Field       string `json:"field"`
	EntityField string `json:"entity_field"`
}

func (s *Service) validateFeature(ctx context.Context, p l.Principal, in d.Feature) error {
	if in.Name == "" || len(in.Name) > 128 || len(in.Description) > 4000 {
		return d.ErrInvalid
	}
	switch in.Type {
	case "numerical", "categorical", "vector":
	default:
		return d.ErrInvalid
	}
	var def featureDefinition
	if json.Unmarshal(in.Definition, &def) != nil || def.Kind != "field" || def.Field == "" {
		return fmt.Errorf("%w: field definition required", d.ErrInvalid)
	}
	_, input, err := s.input(ctx, p, in.SourceRefID, false)
	if err != nil {
		return err
	}
	if input.Kind != "table" {
		return d.ErrInvalid
	}
	found := false
	entityFound := def.EntityField == ""
	for _, col := range input.Columns {
		if col == def.Field {
			found = true
		}
		if col == def.EntityField {
			entityFound = true
		}
	}
	if !found || !entityFound {
		return fmt.Errorf("%w: feature field absent in source", d.ErrInvalid)
	}
	return nil
}
func (s *Service) CreateFeature(ctx context.Context, p l.Principal, in d.Feature, key string) (*d.Record, error) {
	if err := s.validateFeature(ctx, p, in); err != nil {
		return nil, err
	}
	in.Meta = d.Meta{}
	var record *d.Record
	err := s.Repo.Transaction(ctx, func(repo d.Repository) error {
		tx := *s
		tx.Repo = repo
		var err error
		record, err = tx.create(ctx, p, "feature", "", "active", key, in, nil)
		if err != nil {
			return err
		}
		var original d.Feature
		if err = json.Unmarshal(record.Payload, &original); err != nil {
			return err
		}
		return tx.storeFeatureVersion(ctx, record, original, record.Version)
	})
	return record, err
}
func versionID(id string, version int) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("feature:"+id+":"+strconv.Itoa(version))).String()
}
func (s *Service) storeFeatureVersion(ctx context.Context, head *d.Record, in d.Feature, version int) error {
	in.Meta = head.Meta
	in.Version = version
	payload, err := json.Marshal(in)
	if err != nil {
		return err
	}
	record, err := s.Repo.Create(ctx, &d.Record{Meta: d.Meta{ID: versionID(head.ID, version), TenantID: head.TenantID, UserID: head.UserID}, Kind: "feature_version", ParentID: head.ID, Status: "active", IdempotencyKey: head.ID + ":" + strconv.Itoa(version), Payload: payload})
	if err != nil {
		return err
	}
	var old d.Feature
	if err = json.Unmarshal(record.Payload, &old); err != nil {
		return err
	}
	if old.Name != in.Name || old.Type != in.Type || old.SourceRefID != in.SourceRefID || !d.JSONEqual(old.Definition, in.Definition) || old.Description != in.Description {
		return d.ErrConflict
	}
	return nil
}
func (s *Service) NewFeatureVersion(ctx context.Context, p l.Principal, id string, in d.Feature, expected int) (*d.Record, error) {
	if err := s.validateFeature(ctx, p, in); err != nil {
		return nil, err
	}
	head, err := s.Get(ctx, p, "feature", id)
	if err != nil {
		return nil, err
	}
	if head.Version != expected {
		return nil, d.ErrConflict
	}
	in.Meta = d.Meta{}
	err = s.Repo.Transaction(ctx, func(repo d.Repository) error {
		tx := *s
		tx.Repo = repo
		if err := tx.update(ctx, head, in, "active", expected); err != nil {
			return err
		}
		return tx.storeFeatureVersion(ctx, head, in, expected+1)
	})
	if err != nil {
		return nil, err
	}
	return head, nil
}
func (s *Service) featureVersion(ctx context.Context, p l.Principal, id string, version int) (d.Feature, error) {
	head, err := s.Get(ctx, p, "feature", id)
	if err != nil {
		return d.Feature{}, err
	}
	if version <= 0 || version > head.Version {
		return d.Feature{}, d.ErrNotFound
	}
	record, err := s.Get(ctx, p, "feature_version", versionID(id, version))
	if err != nil {
		return d.Feature{}, err
	}
	var f d.Feature
	err = json.Unmarshal(record.Payload, &f)
	return f, err
}
func validateFeatureValue(kind string, raw json.RawMessage) error {
	if len(raw) == 0 || len(raw) > 65536 {
		return d.ErrInvalid
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return d.ErrInvalid
	}
	switch kind {
	case "numerical":
		n, ok := value.(float64)
		if !ok || math.IsInf(n, 0) || math.IsNaN(n) {
			return d.ErrInvalid
		}
	case "categorical":
		s, ok := value.(string)
		if !ok || len(s) > 4096 {
			return d.ErrInvalid
		}
	case "vector":
		v, ok := value.([]any)
		if !ok || len(v) == 0 || len(v) > 4096 {
			return d.ErrInvalid
		}
		for _, x := range v {
			n, ok := x.(float64)
			if !ok || math.IsInf(n, 0) || math.IsNaN(n) {
				return d.ErrInvalid
			}
		}
	default:
		return d.ErrInvalid
	}
	return nil
}
func (s *Service) WriteFeatures(ctx context.Context, p l.Principal, values []d.FeatureValue) error {
	if len(values) == 0 || len(values) > 1000 {
		return d.ErrInvalid
	}
	// Definitions are immutable. Resolve each requested version once per call,
	// including its current source policy, rather than reading the source object
	// again for every entity in a batch.
	definitions := map[string]d.Feature{}
	for i := range values {
		v := &values[i]
		key := versionID(v.FeatureID, v.Version)
		f, ok := definitions[key]
		if !ok {
			var err error
			f, err = s.featureVersion(ctx, p, v.FeatureID, v.Version)
			if err != nil {
				return err
			}
			definitions[key] = f
		}
		if v.EntityID == "" || len(v.EntityID) > 128 || v.Timestamp.IsZero() || v.Timestamp.After(time.Now().Add(time.Minute)) {
			return d.ErrInvalid
		}
		if err := validateFeatureValue(f.Type, v.Value); err != nil {
			return err
		}
		v.SourceRefID = f.SourceRefID
	}
	return s.Repo.FeatureWrite(ctx, p.TenantID, values)
}
func (s *Service) QueryFeatures(ctx context.Context, p l.Principal, entities []string, features []d.FeatureValue, at time.Time) ([]d.FeatureValue, error) {
	if len(entities) == 0 || len(features) == 0 || len(entities)*len(features) > 1000 {
		return nil, d.ErrInvalid
	}
	for _, f := range features {
		if _, err := s.featureVersion(ctx, p, f.FeatureID, f.Version); err != nil {
			return nil, err
		}
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return s.Repo.FeatureQuery(ctx, p.TenantID, entities, features, at)
}
func (s *Service) ComputeFeatures(ctx context.Context, p l.Principal, id string, version int) (int, error) {
	f, err := s.featureVersion(ctx, p, id, version)
	if err != nil {
		return 0, err
	}
	var def featureDefinition
	if err = json.Unmarshal(f.Definition, &def); err != nil {
		return 0, err
	}
	ref, input, err := s.input(ctx, p, f.SourceRefID, false)
	if err != nil {
		return 0, err
	}
	if input.Kind != "table" || len(input.Rows) > 1000 {
		return 0, d.ErrInvalid
	}
	field, entity := -1, -1
	for i, col := range input.Columns {
		if col == def.Field {
			field = i
		}
		if col == def.EntityField {
			entity = i
		}
	}
	if field < 0 || (def.EntityField != "" && entity < 0) {
		return 0, d.ErrInvalid
	}
	values := []d.FeatureValue{}
	for i, row := range input.Rows {
		if row[field] == nil {
			continue
		}
		v := row[field]
		if f.Type == "numerical" {
			// Validation may use float64 for finiteness, but never replace the
			// original decimal spelling with a rounded floating-point value.
			number := json.Number(fmt.Sprint(v))
			parsed, e := number.Float64()
			if e != nil || math.IsInf(parsed, 0) || math.IsNaN(parsed) || !json.Valid([]byte(number)) {
				return 0, d.ErrInvalid
			}
			v = number
		}
		if f.Type == "vector" {
			if raw, ok := v.(string); ok {
				decoder := json.NewDecoder(bytes.NewReader([]byte(raw)))
				decoder.UseNumber()
				if !json.Valid([]byte(raw)) || decoder.Decode(&v) != nil {
					return 0, d.ErrInvalid
				}
			}
		}
		raw, e := json.Marshal(v)
		if e != nil {
			return 0, e
		}
		entityID := fmt.Sprintf("%s:%d", ref.AssetID, i)
		if entity >= 0 {
			if row[entity] == nil {
				return 0, d.ErrInvalid
			}
			entityID = fmt.Sprint(row[entity])
		}
		values = append(values, d.FeatureValue{FeatureID: id, Version: version, EntityID: entityID, Value: raw, Timestamp: ref.CreatedAt})
	}
	if len(values) == 0 {
		return 0, nil
	}
	return len(values), s.WriteFeatures(ctx, p, values)
}
