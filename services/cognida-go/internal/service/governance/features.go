package governance

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"math"
	"strconv"
	"time"
)

func (s *Service) validateFeature(ctx context.Context, p l.Principal, in d.Feature) error {
	if s.Assets == nil {
		return d.ErrForbidden
	}
	if err := s.authorize(ctx, p, "read"); err != nil {
		return err
	}
	if in.Name == "" || len(in.Name) > 128 || len(in.Description) > 4000 {
		return d.ErrInvalid
	}
	switch in.Type {
	case "numerical", "categorical", "vector":
	default:
		return d.ErrInvalid
	}
	def, err := parseFeatureDefinition(in.Definition)
	if err != nil {
		return err
	}
	if def.Kind == "embedding" {
		if in.Type != "vector" {
			return featureErr("invalid_type", "embedding produces vector features")
		}
		if err = s.checkFeatureEmbedding(def); err != nil {
			return err
		}
	}
	_, input, err := s.input(ctx, p, in.SourceRefID, false)
	if err != nil {
		return err
	}
	if input.Kind != "table" || len(input.Rows) > 10000 {
		return featureErr("input_limit", "feature definition input must be a table of 1..10000 rows; direct previews are limited to 1000")
	}
	fields := map[string]bool{}
	for _, c := range input.Columns {
		if fields[c] {
			return featureErr("duplicate_column", "source columns must be unique")
		}
		fields[c] = true
	}
	check := func(field string) error {
		if field != "" && !fields[field] {
			return &d.FeatureError{Code: "missing_field", Field: field, Message: "column is absent in source"}
		}
		return nil
	}
	for _, field := range []string{def.Field, def.EntityField, def.EventTimeField} {
		if err = check(field); err != nil {
			return err
		}
	}
	var visit func(d.FeatureExpression) error
	visit = func(e d.FeatureExpression) error {
		if e.Op == "column" {
			if err := check(e.Field); err != nil {
				return err
			}
		}
		for _, a := range e.Args {
			if err := visit(a); err != nil {
				return err
			}
		}
		return nil
	}
	if def.Expression != nil {
		return visit(*def.Expression)
	}
	if def.Kind == "window_aggregate" && in.Type != "numerical" {
		return featureErr("invalid_type", "window aggregates produce numerical values")
	}
	return nil
}
func (s *Service) CreateFeature(ctx context.Context, p l.Principal, in d.Feature, key string) (*d.Record, error) {
	if err := s.validateFeature(ctx, p, in); err != nil {
		return nil, err
	}
	in.Meta = d.Meta{}
	in.DefinitionExact = string(in.Definition)
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
		if err = decodeStoredFeature(record.Payload, &original); err != nil {
			return err
		}
		if original.Name != in.Name || original.Type != in.Type || original.Description != in.Description || original.SourceRefID != in.SourceRefID || !d.JSONEqual(original.Definition, in.Definition) {
			return d.ErrConflict
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
	if err = decodeStoredFeature(record.Payload, &old); err != nil {
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
	in.DefinitionExact = string(in.Definition)
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
	err = decodeStoredFeature(record.Payload, &f)
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
		if v.ValueExact != "" {
			exact := json.RawMessage(v.ValueExact)
			if len(v.Value) > 0 && string(v.Value) != "null" && !d.JSONEqual(v.Value, exact) {
				return featureErr("conflicting_value", "value and value_exact must agree when both are provided")
			}
			if err := validateFeatureValue(f.Type, exact); err != nil {
				return err
			}
			v.Value = exact
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
	var values []d.FeatureValue
	var err error
	if jobs, ok := s.Repo.(d.FeatureJobRepository); ok {
		values, err = jobs.FeatureQueryOwned(ctx, p.TenantID, p.UserID, entities, features, at)
	} else {
		values, err = s.Repo.FeatureQuery(ctx, p.TenantID, entities, features, at)
	}
	if err != nil {
		return nil, err
	}
	checked := map[string]bool{}
	for _, value := range values {
		if !checked[value.SourceRefID] {
			if _, _, err = s.input(ctx, p, value.SourceRefID, false); err != nil {
				return nil, err
			}
			checked[value.SourceRefID] = true
		}
	}
	return values, nil
}

// ComputeFeatures retains the legacy endpoint while using the same atomic,
// versioned materialization ledger as the new explicit API.
func (s *Service) ComputeFeatures(ctx context.Context, p l.Principal, id string, version int) (int, error) {
	r, err := s.MaterializeFeature(ctx, p, id, version, "legacy:"+id+":"+strconv.Itoa(version))
	if err != nil {
		return 0, err
	}
	return r.Count, nil
}
func (s *Service) GetFeatureVersion(ctx context.Context, p l.Principal, id string, version int) (d.Feature, error) {
	return s.featureVersion(ctx, p, id, version)
}
func (s *Service) ValidateFeatureVersion(ctx context.Context, p l.Principal, id string, version int, sourceRef string) error {
	f, err := s.featureVersion(ctx, p, id, version)
	if err != nil {
		return err
	}
	if f.SourceRefID != sourceRef {
		return d.ErrConflict
	}
	return s.validateFeature(ctx, p, f)
}

func decodeStoredFeature(raw json.RawMessage, f *d.Feature) error {
	if err := json.Unmarshal(raw, f); err != nil {
		return err
	}
	if f.DefinitionExact != "" {
		if !json.Valid([]byte(f.DefinitionExact)) {
			return d.ErrInvalid
		}
		f.Definition = []byte(f.DefinitionExact)
	}
	return nil
}
