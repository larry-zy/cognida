package agent

import (
	"bytes"
	d "cognida/internal/model/agent"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

type ConfigService struct{ repo d.PreferencesRepository }

func NewConfigService(repos ...d.PreferencesRepository) *ConfigService {
	s := &ConfigService{}
	if len(repos) > 0 {
		s.repo = repos[0]
	}
	return s
}

type ConfigUpdate struct {
	ExpectedVersion *int64                     `json:"expected_version"`
	AgenticRAG      map[string]json.RawMessage `json:"agentic_rag"`
	DeepResearch    map[string]json.RawMessage `json:"deep_research"`
}

func (s *ConfigService) Read(ctx context.Context, p d.ConfigScope) (d.Preferences, error) {
	if err := p.Validate(); err != nil {
		return d.Preferences{}, err
	}
	if s == nil || s.repo == nil {
		return d.Preferences{}, d.ErrConfigUnavailable
	}
	return s.repo.Read(ctx, p)
}
func mergePreferences(target any, patch map[string]json.RawMessage) error {
	b, err := json.Marshal(target)
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(b, &fields); err != nil {
		return err
	}
	for key, value := range patch {
		if _, ok := fields[key]; !ok {
			return fmt.Errorf("%w: unsupported field %s", d.ErrConfigInvalid, key)
		}
		fields[key] = value
	}
	b, err = json.Marshal(fields)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: field type mismatch", d.ErrConfigInvalid)
	}
	return nil
}
func (s *ConfigService) Update(ctx context.Context, p d.ConfigScope, req ConfigUpdate, key string) (d.Preferences, error) {
	if req.ExpectedVersion == nil || *req.ExpectedVersion < 0 || len(key) < 1 || len(key) > 200 || strings.TrimSpace(key) != key || len(req.AgenticRAG)+len(req.DeepResearch) == 0 {
		return d.Preferences{}, d.ErrConfigInvalid
	}
	next, err := s.Read(ctx, p)
	if err != nil {
		return next, err
	}
	if err = mergePreferences(&next.AgenticRAG, req.AgenticRAG); err != nil {
		return d.Preferences{}, err
	}
	if err = mergePreferences(&next.DeepResearch, req.DeepResearch); err != nil {
		return d.Preferences{}, err
	}
	if err = next.Validate(); err != nil {
		return d.Preferences{}, err
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return d.Preferences{}, d.ErrConfigInvalid
	}
	hash := sha256.Sum256(payload)
	keyHash := sha256.Sum256([]byte(key))
	return s.repo.Update(ctx, p, *req.ExpectedVersion, hex.EncodeToString(keyHash[:]), hex.EncodeToString(hash[:]), next)
}
func configScope(ctx context.Context) (d.ConfigScope, error) {
	tenant, _ := d.GetTenantID(ctx)
	user, _ := d.GetUserID(ctx)
	p := d.ConfigScope{TenantID: tenant, UserID: user}
	return p, p.Validate()
}

// ApplyRAG snapshots preferences once before an interactive run. Explicit session
// retrieval values take precedence, and the framework clamps the iteration cap.
func (s *ConfigService) ApplyRAG(ctx context.Context) (context.Context, error) {
	p, err := configScope(ctx)
	if err != nil {
		return nil, err
	}
	prefs, err := s.Read(ctx, p)
	if err != nil {
		return nil, err
	}
	v := prefs.AgenticRAG
	if v.MaxIterations != nil {
		ctx = d.WithIterationLimit(ctx, *v.MaxIterations)
	}
	retrieval := d.RetrievalParams{TopK: v.DefaultTopK, MinScore: v.MinScore, RerankEnabled: v.RerankEnabled, Alpha: v.Alpha}
	if explicit, ok := d.GetRetrievalParams(ctx); ok && explicit != nil {
		if explicit.TopK != nil {
			retrieval.TopK = explicit.TopK
		}
		if explicit.MinScore != nil {
			retrieval.MinScore = explicit.MinScore
		}
		if explicit.RerankEnabled != nil {
			retrieval.RerankEnabled = explicit.RerankEnabled
		}
		if explicit.Alpha != nil {
			retrieval.Alpha = explicit.Alpha
		}
	}
	if retrieval.TopK != nil || retrieval.MinScore != nil || retrieval.RerankEnabled != nil || retrieval.Alpha != nil {
		ctx = d.WithRetrievalParams(ctx, &retrieval)
	}
	return ctx, nil
}
func (s *ConfigService) Research(ctx context.Context) (d.ResearchPreferences, error) {
	p, err := configScope(ctx)
	if err != nil {
		return d.ResearchPreferences{}, err
	}
	prefs, err := s.Read(ctx, p)
	return prefs.DeepResearch, err
}
