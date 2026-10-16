package governance

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type featureEmbeddingProvider struct {
	ModelID, Revision string
	Embed             func(context.Context, []string) ([][]float64, error)
}

// SetFeatureEmbedding is called once at startup. Identity/revision are trusted
// platform configuration; request data cannot select an arbitrary provider.
func (s *Service) SetFeatureEmbedding(modelID, revision string, embed func(context.Context, []string) ([][]float64, error)) {
	if strings.TrimSpace(modelID) == "" || strings.TrimSpace(revision) == "" || embed == nil {
		s.featureEmbedding = nil
		return
	}
	s.featureEmbedding = &featureEmbeddingProvider{modelID, revision, embed}
}
func (s *Service) FeatureEmbeddingModel() map[string]any {
	p := s.featureEmbedding
	if p == nil {
		return map[string]any{"available": false, "reason": "缺少真实 embedding 模型或可信修订号"}
	}
	return map[string]any{"available": true, "model_id": p.ModelID, "revision": p.Revision, "max_batch": 32, "max_text_characters": 4096, "cost_limit_enforced": false}
}
func (s *Service) checkFeatureEmbedding(def d.FeatureDefinition) error {
	p := s.featureEmbedding
	if p == nil {
		return featureErr("embedding_unavailable", "a real embedding model with a trusted revision is not configured")
	}
	if def.Embedding == nil || p.ModelID != def.Embedding.ModelID || p.Revision != def.Embedding.Revision {
		return featureErr("embedding_identity_mismatch", "requested embedding identity/revision is not the configured immutable model")
	}
	return nil
}
func (s *Service) computeFeature(ctx context.Context, f d.Feature, ref l.DataAssetRef, input l.Input) ([]d.FeatureValue, []string, error) {
	def, e := parseFeatureDefinition(f.Definition)
	if e != nil {
		return nil, nil, e
	}
	if def.Kind != "embedding" {
		return computeFeatureValues(ctx, f, ref, input)
	}
	if e = s.checkFeatureEmbedding(def); e != nil {
		return nil, nil, e
	}
	if f.Type != "vector" || input.Kind != "table" || input.Validate() != nil || len(input.Rows) > 1000 {
		return nil, nil, featureErr("invalid_embedding_input", "embedding requires a bounded table of at most 1000 rows")
	}
	indexes := map[string]int{}
	for i, c := range input.Columns {
		if _, exists := indexes[c]; exists {
			return nil, nil, featureErr("duplicate_column", "source columns must be unique")
		}
		indexes[c] = i
	}
	for _, field := range []string{def.Field, def.EntityField, def.EventTimeField} {
		if field != "" {
			if _, ok := indexes[field]; !ok {
				return nil, nil, featureErr("missing_field", "embedding source column is absent")
			}
		}
	}
	texts := make([]string, len(input.Rows))
	out := make([]d.FeatureValue, len(input.Rows))
	seen := map[string]bool{}
	// Validate the entire batch before the first paid model request.
	for i, row := range input.Rows {
		text, ok := row[indexes[def.Field]].(string)
		if !ok || strings.TrimSpace(text) == "" || utf8.RuneCountInString(text) > 4096 {
			return nil, nil, rowErr(featureErr("invalid_embedding_text", "embedding field must contain nonempty text up to 4096 characters"), i)
		}
		texts[i] = text
		entity := ref.AssetID + ":" + strconv.Itoa(i)
		if def.EntityField != "" {
			entity, e = featureEntity(row[indexes[def.EntityField]])
			if e != nil {
				return nil, nil, rowErr(e, i)
			}
		}
		at := ref.CreatedAt.UTC().Truncate(time.Microsecond)
		if def.EventTimeField != "" {
			at, e = featureEvent(row[indexes[def.EventTimeField]])
			if e != nil {
				return nil, nil, rowErr(e, i)
			}
		}
		key := entity + "\x00" + at.Format(time.RFC3339Nano)
		if seen[key] {
			return nil, nil, featureErr("duplicate_entity_time", "source contains repeated entity/event keys")
		}
		seen[key] = true
		out[i] = d.FeatureValue{FeatureID: f.ID, Version: f.Version, EntityID: entity, Timestamp: at, SourceRefID: f.SourceRefID}
	}
	for start := 0; start < len(texts); start += 32 {
		end := start + 32
		if end > len(texts) {
			end = len(texts)
		}
		call, cancel := context.WithTimeout(ctx, 45*time.Second)
		vectors, err := s.featureEmbedding.Embed(call, texts[start:end])
		cancel()
		if err != nil {
			return nil, nil, featureErr("embedding_failed", "configured embedding model request failed; no fallback vector was created")
		}
		if len(vectors) != end-start {
			return nil, nil, featureErr("embedding_response", "embedding count does not match input")
		}
		for j, v := range vectors {
			if len(v) != def.Embedding.Dimension {
				return nil, nil, featureErr("embedding_dimension", "embedding response dimension does not match the pinned definition")
			}
			nonzero := false
			for _, x := range v {
				if math.IsNaN(x) || math.IsInf(x, 0) {
					return nil, nil, featureErr("embedding_response", "embedding contains non-finite coordinates")
				}
				nonzero = nonzero || x != 0
			}
			if !nonzero {
				return nil, nil, featureErr("embedding_response", "embedding response contains a zero vector")
			}
			raw, _ := json.Marshal(v)
			out[start+j].Value = raw
			out[start+j].ValueExact = string(raw)
		}
	}
	return out, []string{"真实模型调用已绑定模型身份与修订；不提供货币费用上限保证。"}, nil
}
