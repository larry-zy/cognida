package lifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	d "cognida/internal/model/lifecycle"
)

func validatePipelineOptions(def d.PipelineDefinition, options map[string]any) error {
	for key := range options {
		if key != "stage_options" {
			return fmt.Errorf("%w: pipeline options must use stage_options", d.ErrInvalid)
		}
	}
	raw, exists := options["stage_options"]
	if !exists {
		return nil
	}
	stages, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: stage_options must be an object", d.ErrInvalid)
	}
	known := map[string]bool{}
	for _, stage := range def.Stages {
		known[stage.ID] = true
	}
	for key, value := range stages {
		if !known[key] {
			return fmt.Errorf("%w: unknown stage %s", d.ErrInvalid, key)
		}
		options, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: stage options must be objects", d.ErrInvalid)
		}
		if _, exists := options[strategyOption]; exists {
			return d.ErrInvalid
		}
		if err := validateOptions(options); err != nil {
			return err
		}
	}
	return nil
}

func validateExpertOptions(agent string, options map[string]any) error {
	if err := validateOptions(options); err != nil {
		return err
	}
	if agent == "data-preparation" {
		_, err := preparationOptions(options)
		return err
	}
	if value, exists := options["prompt"]; exists {
		text, ok := value.(string)
		if !ok || utf8.RuneCountInString(text) > 4000 {
			return fmt.Errorf("%w: prompt must be text of at most 4000 characters", d.ErrInvalid)
		}
	}
	if agent == "data-cleaning" {
		if _, err := cleanOptions(options); err != nil {
			return err
		}
	}
	if agent == "data-quality" || agent == "data-cleaning" {
		if _, err := threshold(options); err != nil {
			return err
		}
	}
	if agent == "data-enrichment" {
		if method, exists := options["method"]; exists && method != "rules" && method != "llm" {
			return fmt.Errorf("%w: enrichment method must be rules or llm", d.ErrInvalid)
		}
	}
	return nil
}

func (s *Service) preflight(ctx context.Context, p d.Principal, agent string, refs []d.RefID, options map[string]any) error {
	if err := validateExpertOptions(agent, options); err != nil {
		return err
	}
	if agent == "data-cleaning" {
		if expected, ok := options["preview_digest"]; ok {
			found := false
			for _, id := range refs {
				ref, _, err := s.Resolve(ctx, p.TenantID, p.UserID, id)
				if err != nil {
					return err
				}
				if ref.Kind == d.QualityReport {
					continue
				}
				if expected != cleaningDigest(ref, options) {
					return d.ErrConflict
				}
				found = true
				break
			}
			if !found {
				return d.ErrInvalid
			}
		}
	}
	if agent != "data-asset" || s.config.ValidatePublication == nil {
		return nil
	}
	for _, id := range refs {
		ref, raw, err := s.Resolve(ctx, p.TenantID, p.UserID, id)
		if err != nil {
			return err
		}
		if ref.Kind == d.QualityReport || ref.Kind == d.Insight || ref.Kind == d.Evaluation {
			continue
		}
		var input d.Input
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err = decoder.Decode(&input); err != nil {
			return fmt.Errorf("%w: invalid publication input", d.ErrInvalid)
		}
		if err = input.Validate(); err != nil {
			return err
		}
		return s.config.ValidatePublication(ctx, p, input, options)
	}
	return fmt.Errorf("%w: publication requires text or table input", d.ErrInvalid)
}
