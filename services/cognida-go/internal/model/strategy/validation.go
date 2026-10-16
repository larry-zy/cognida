package strategy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"regexp"
	"strings"
)

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,99}$`)

func ValidSlot(s string) bool { return namePattern.MatchString(s) }
func ValidKey(s string) bool {
	if len(s) < 1 || len(s) > 100 {
		return false
	}
	for _, c := range s {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}
func Digest(v any) string {
	raw, _ := json.Marshal(v)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
func StrictJSON(raw json.RawMessage, out any) error {
	// Reject duplicate keys and case-folded aliases before decoding structs.
	scan := json.NewDecoder(bytes.NewReader(raw))
	scan.UseNumber()
	var value func() error
	value = func() error {
		token, e := scan.Token()
		if e != nil {
			return ErrInvalid
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for scan.More() {
				key, e := scan.Token()
				if e != nil {
					return ErrInvalid
				}
				name, ok := key.(string)
				if !ok || !policyJSONKey(name) || seen[name] {
					return ErrInvalid
				}
				seen[name] = true
				if e = value(); e != nil {
					return e
				}
			}
			end, e := scan.Token()
			if e != nil || end != json.Delim('}') {
				return ErrInvalid
			}
		case '[':
			for scan.More() {
				if e := value(); e != nil {
					return e
				}
			}
			end, e := scan.Token()
			if e != nil || end != json.Delim(']') {
				return ErrInvalid
			}
		default:
			return ErrInvalid
		}
		return nil
	}
	if value() != nil {
		return ErrInvalid
	}
	if _, e := scan.Token(); e != io.EOF {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil || d.Decode(new(any)) != io.EOF {
		return ErrInvalid
	}
	return nil
}
func ValidGates(g Gates) bool {
	return finite(g.MinimumQuality) && g.MinimumQuality >= 0 && g.MinimumQuality <= 1 && finite(g.MaximumQualityDrop) && g.MaximumQualityDrop >= 0 && g.MaximumQualityDrop <= 1 && finite(g.MaximumErrorRate) && g.MaximumErrorRate >= 0 && g.MaximumErrorRate <= 1 && finite(g.MaximumCostRatio) && g.MaximumCostRatio >= 1 && g.MaximumCostRatio <= 10
}
func ValidMetrics(m Metrics) bool {
	return m.Samples > 0 && m.Samples <= 1000 && finite(m.Quality) && m.Quality >= 0 && m.Quality <= 1 && finite(m.ErrorRate) && m.ErrorRate >= 0 && m.ErrorRate <= 1 && finite(m.Cost) && m.Cost >= 0
}
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

type RetrievalPolicy struct {
	InheritDefaults bool    `json:"inherit_defaults,omitempty"`
	TopK            int     `json:"top_k"`
	ScoreThreshold  float64 `json:"score_threshold"`
	Rerank          bool    `json:"rerank"`
}
type CleaningPolicy struct {
	InheritDefaults bool     `json:"inherit_defaults,omitempty"`
	Operations      []string `json:"operations"`
	Keep            string   `json:"keep"`
	MinimumScore    float64  `json:"min_score"`
}
type GuidancePolicy struct {
	Instructions string `json:"instructions"`
}
type OrchestrationPolicy struct {
	MaxParallel int `json:"max_parallel"`
}

// Closed policy schemas deliberately have no authorization, tool, credential,
// approval, network or sandbox fields. Guidance is data, never tool authority.
func ValidateBody(kind string, raw json.RawMessage) error {
	if len(raw) == 0 || len(raw) > 64<<10 {
		return ErrInvalid
	}
	switch kind {
	case "retrieval":
		if isInheritedPolicy(raw) {
			return nil
		}
		var x RetrievalPolicy
		if StrictJSON(raw, &x) != nil || x.InheritDefaults || x.TopK < 1 || x.TopK > 30 || !finite(x.ScoreThreshold) || x.ScoreThreshold < 0 || x.ScoreThreshold > 1 {
			return ErrInvalid
		}
	case "cleaning":
		if isInheritedPolicy(raw) {
			return nil
		}
		var x CleaningPolicy
		if StrictJSON(raw, &x) != nil || x.InheritDefaults || len(x.Operations) < 1 || len(x.Operations) > 6 || (x.Keep != "first" && x.Keep != "last") || !finite(x.MinimumScore) || x.MinimumScore < 0 || x.MinimumScore > 100 {
			return ErrInvalid
		}
		seen := map[string]bool{}
		for _, op := range x.Operations {
			switch op {
			case "trim", "normalize_ws", "drop_empty", "dedup", "text_cleaner", "pii_masker":
			default:
				return ErrInvalid
			}
			if seen[op] {
				return ErrInvalid
			}
			seen[op] = true
		}
	case "orchestration":
		if isInheritedPolicy(raw) {
			return nil
		}
		var x OrchestrationPolicy
		if StrictJSON(raw, &x) != nil || x.MaxParallel < 1 || x.MaxParallel > 8 {
			return ErrInvalid
		}
	case "experience", "skill", "playbook":
		var x GuidancePolicy
		if StrictJSON(raw, &x) != nil || len([]rune(x.Instructions)) > 12000 {
			return ErrInvalid
		}
		for _, forbidden := range []string{"bypass approval", "disable sandbox", "ignore permission", "read credentials", "绕过审批", "关闭沙箱", "读取凭据", "忽略权限", "扩大权限"} {
			if strings.Contains(strings.ToLower(x.Instructions), forbidden) {
				return ErrInvalid
			}
		}
	default:
		return ErrInvalid
	}
	return nil
}
func Compare(g Gates, b, c Metrics) (bool, []string) {
	reasons := []string{}
	if !ValidGates(g) || !ValidMetrics(b) || !ValidMetrics(c) || b.Samples != c.Samples {
		return false, []string{"invalid_or_incomplete_metrics"}
	}
	if c.Quality < g.MinimumQuality {
		reasons = append(reasons, "minimum_quality")
	}
	if b.Quality-c.Quality > g.MaximumQualityDrop {
		reasons = append(reasons, "quality_regression")
	}
	if c.ErrorRate > g.MaximumErrorRate {
		reasons = append(reasons, "error_rate")
	}
	if c.Cost > b.Cost*g.MaximumCostRatio {
		reasons = append(reasons, "cost_regression")
	}
	return len(reasons) == 0, reasons
}

func isInheritedPolicy(raw json.RawMessage) bool {
	var fields map[string]json.RawMessage
	if StrictJSON(raw, &fields) != nil || len(fields) != 1 {
		return false
	}
	var inherit bool
	return json.Unmarshal(fields["inherit_defaults"], &inherit) == nil && inherit
}

func policyJSONKey(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}
