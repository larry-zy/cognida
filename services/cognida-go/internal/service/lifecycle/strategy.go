package lifecycle

import (
	d "cognida/internal/model/lifecycle"
	policy "cognida/internal/model/strategy"
	strategies "cognida/internal/service/strategy"
	"context"
	"encoding/json"
	"errors"
)

const strategyOption = "_strategy_snapshot"

func (s *Service) WithStrategies(v *strategies.Service) *Service { s.strategies = v; return s }

// Baseline preserves the actual options of each existing caller. Registering a
// strategy must not silently rewrite a user-selected cleaning plan.
func DefaultCleaningStrategy(context.Context, d.Principal) (json.RawMessage, error) {
	return json.RawMessage(`{"inherit_defaults":true}`), nil
}
func (s *Service) strategyOptions(ctx context.Context, p d.Principal, agent, key string, options map[string]any) (map[string]any, error) {
	if s.strategies == nil || agent != "data-cleaning" {
		return options, nil
	}
	effective, e := effectiveCleaningOptions(options)
	if e != nil {
		return nil, e
	}
	raw, _ := json.Marshal(effective)
	snap, e := s.strategies.Pin(ctx, p, "cleaning.default", policy.Digest([]string{"lifecycle", key}), raw)
	if errors.Is(e, policy.ErrNotFound) {
		return options, nil
	}
	if e != nil {
		return nil, e
	}
	out, e := mergeCleaningStrategy(options, snap.EffectiveBody)
	if e != nil {
		return nil, e
	}
	out[strategyOption] = snap
	return out, nil
}
func mergeCleaningStrategy(options map[string]any, raw json.RawMessage) (map[string]any, error) {
	var body map[string]any
	if json.Unmarshal(raw, &body) != nil {
		return nil, d.ErrInvalid
	}
	if inherited, _ := body["inherit_defaults"].(bool); inherited {
		body = nil
	}
	out := map[string]any{}
	for k, v := range options {
		out[k] = v
	}
	for k, v := range body {
		out[k] = v
	}
	return out, nil
}
func (s *Service) checkStrategy(ctx context.Context, r *d.StageRequest) error {
	raw, ok := r.Stage.Options[strategyOption]
	if !ok {
		opts, e := s.strategyOptions(ctx, r.Principal, r.Stage.AgentID, r.Stage.ID, r.Stage.Options)
		if e != nil {
			return e
		}
		r.Stage.Options = opts
		raw, ok = opts[strategyOption]
		if !ok {
			return nil
		}
	}
	if s.strategies == nil {
		return d.ErrForbidden
	}
	encoded, e := json.Marshal(raw)
	if e != nil {
		return e
	}
	var snap policy.Snapshot
	if json.Unmarshal(encoded, &snap) != nil || snap.Kind != "cleaning" {
		return d.ErrInvalid
	}
	return s.strategies.ValidateSnapshot(ctx, r.Principal, &snap)
}

func (s *Service) previewStrategyOptions(ctx context.Context, p d.Principal, options map[string]any) (map[string]any, *policy.Version, error) {
	if s.strategies == nil {
		return options, nil, nil
	}
	v, e := s.strategies.Active(ctx, p, "cleaning.default")
	if errors.Is(e, policy.ErrNotFound) {
		effective, e := effectiveCleaningOptions(options)
		return effective, nil, e
	}
	if e != nil {
		return nil, nil, e
	}
	effective, e := effectiveCleaningOptions(options)
	if e != nil {
		return nil, nil, e
	}
	opts, e := mergeCleaningStrategy(effective, v.Body)
	return opts, v, e
}

func effectiveCleaningOptions(options map[string]any) (map[string]any, error) {
	opts, e := cleanOptions(options)
	if e != nil {
		return nil, e
	}
	for _, key := range []string{"min_score", "dimensions", "reference_text"} {
		if value, ok := options[key]; ok {
			opts[key] = value
		}
	}
	if _, ok := opts["min_score"]; !ok {
		opts["min_score"] = 70
	}
	// Confirmation is request metadata, never part of the resolved strategy itself.
	return opts, nil
}
