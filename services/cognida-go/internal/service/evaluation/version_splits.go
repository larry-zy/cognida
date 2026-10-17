package evaluation

import (
	d "cognida/internal/model/evaluation"
	"crypto/sha256"
	"encoding/binary"
	"strings"
	"unicode"
)

func assignDatasetSplits(v *d.DatasetVersion) error {
	s := &v.Split
	if s.Algorithm == "" {
		s.Algorithm = "none"
	}
	if s.NearDuplicateThreshold == 0 {
		s.NearDuplicateThreshold = .85
	}
	if s.NearDuplicateThreshold < .5 || s.NearDuplicateThreshold > 1 {
		return versionError("near duplicate threshold must be 0.5..1")
	}
	switch s.Algorithm {
	case "none":
		if s.TrainPercent != 0 || s.ValidationPercent != 0 || s.TrainBefore != nil || s.ValidationBefore != nil {
			return versionError("none split cannot declare training boundaries")
		}
	case "source_hash_v1":
		if s.Seed == "" || len(s.Seed) > 128 || s.TrainPercent < 1 || s.ValidationPercent < 1 || s.TrainPercent+s.ValidationPercent >= 100 {
			return versionError("invalid deterministic split")
		}
	case "source_time_v1":
		if s.TrainBefore == nil || s.ValidationBefore == nil || !s.TrainBefore.Before(*s.ValidationBefore) {
			return versionError("invalid time boundaries")
		}
	default:
		return versionError("unknown split algorithm")
	}
	for i := range v.Samples {
		sample := &v.Samples[i]
		sample.Split = "evaluation"
		switch s.Algorithm {
		case "source_hash_v1":
			h := sha256.Sum256([]byte(s.Seed + "\x00" + sample.Group))
			bucket := int(binary.BigEndian.Uint64(h[:8]) % 100)
			sample.Split = "test"
			if bucket < s.TrainPercent {
				sample.Split = "train"
			} else if bucket < s.TrainPercent+s.ValidationPercent {
				sample.Split = "validation"
			}
		case "source_time_v1":
			if sample.SourceTime.IsZero() {
				return versionError("missing source time")
			}
			sample.Split = "test"
			if sample.SourceTime.Before(*s.TrainBefore) {
				sample.Split = "train"
			} else if sample.SourceTime.Before(*s.ValidationBefore) {
				sample.Split = "validation"
			}
		}
	}
	grams := make([]map[string]bool, len(v.Samples))
	normalized := make([]string, len(v.Samples))
	// QA-derived profiles consume the final reviewed Question, not the immutable
	// source text. Answers/labels are targets and must not distinguish identical
	// evaluation inputs (even when reviewers provide different target answers).
	inputs := make([]string, len(v.Samples))
	inputGrams := make([]map[string]bool, len(v.Samples))
	qaProfile := v.Profile == "qa" || v.Profile == "rag" || v.Profile == "agent" || v.Profile == "sql"
	if qaProfile && len(v.QAPairs) != len(v.Samples) {
		return versionError("reviewed evaluation inputs do not match samples")
	}
	for i, sample := range v.Samples {
		normalized[i] = normalizeLeakText(sample.Content)
		grams[i] = leakGrams(normalized[i])
		inputs[i] = normalized[i]
		if qaProfile {
			inputs[i] = normalizeLeakText(v.QAPairs[i].Question)
			if inputs[i] == "" {
				return versionError("empty reviewed evaluation input")
			}
		}
		inputGrams[i] = leakGrams(inputs[i])
	}
	for i := range v.Samples {
		for j := 0; j < i; j++ {
			if v.Samples[i].Split == v.Samples[j].Split {
				continue
			}
			if normalized[i] == normalized[j] || gramSimilarity(grams[i], grams[j]) >= s.NearDuplicateThreshold {
				return versionError("exact or near duplicate samples cross dataset splits")
			}
			if inputs[i] == inputs[j] || gramSimilarity(inputGrams[i], inputGrams[j]) >= s.NearDuplicateThreshold {
				return versionError("exact or near duplicate reviewed inputs cross dataset splits")
			}
		}
	}
	return nil
}
func normalizeLeakText(s string) string {
	return strings.ToLower(strings.Join(strings.FieldsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsPunct(r) }), ""))
}
func leakGrams(s string) map[string]bool {
	r := []rune(s)
	if len(r) > 2048 {
		r = r[:2048]
	}
	out := map[string]bool{}
	if len(r) < 3 {
		out[string(r)] = true
		return out
	}
	for i := 0; i+3 <= len(r); i++ {
		out[string(r[i:i+3])] = true
	}
	return out
}
func gramSimilarity(a, b map[string]bool) float64 {
	same := 0
	for k := range a {
		if b[k] {
			same++
		}
	}
	total := len(a) + len(b) - same
	if total == 0 {
		return 1
	}
	return float64(same) / float64(total)
}
