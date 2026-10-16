package agent

import (
	"context"
	"errors"
	"fmt"
	"math"
)

var (
	ErrConfigInvalid     = errors.New("invalid agent configuration")
	ErrConfigConflict    = errors.New("agent configuration version or idempotency conflict")
	ErrConfigUnavailable = errors.New("agent configuration storage unavailable")
)

// Preferences are private defaults, never tool grants or a global runtime configuration.
// Nil fields preserve the existing runtime defaults; explicit null resets an override.
type RAGPreferences struct {
	MaxIterations *int     `json:"max_iterations"`
	DefaultTopK   *int     `json:"default_top_k"`
	MinScore      *float64 `json:"min_score"`
	RerankEnabled *bool    `json:"rerank_enabled"`
	Alpha         *float64 `json:"alpha"`
}
type ResearchPreferences struct {
	MaxSubQuestions *int `json:"max_sub_questions"`
	MaxConcurrency  *int `json:"max_concurrency"`
	MaxSources      *int `json:"max_sources"`
	TimeoutSeconds  *int `json:"timeout_seconds"`
}
type Preferences struct {
	Version      int64               `json:"version"`
	AgenticRAG   RAGPreferences      `json:"agentic_rag"`
	DeepResearch ResearchPreferences `json:"deep_research"`
}
type ConfigScope struct{ TenantID, UserID int64 }

func (p ConfigScope) Validate() error {
	if p.TenantID <= 0 || p.UserID <= 0 {
		return ErrConfigInvalid
	}
	return nil
}
func (p Preferences) Validate() error {
	for _, bound := range []struct {
		name   string
		value  *int
		lo, hi int
	}{
		{"max_iterations", p.AgenticRAG.MaxIterations, 1, 20}, {"default_top_k", p.AgenticRAG.DefaultTopK, 1, 100},
		{"max_sub_questions", p.DeepResearch.MaxSubQuestions, 1, 5}, {"max_concurrency", p.DeepResearch.MaxConcurrency, 1, 3},
		{"max_sources", p.DeepResearch.MaxSources, 1, 30}, {"timeout_seconds", p.DeepResearch.TimeoutSeconds, 30, 300},
	} {
		if bound.value != nil && (*bound.value < bound.lo || *bound.value > bound.hi) {
			return fmt.Errorf("%w: %s must be %d..%d", ErrConfigInvalid, bound.name, bound.lo, bound.hi)
		}
	}
	for _, value := range []*float64{p.AgenticRAG.MinScore, p.AgenticRAG.Alpha} {
		if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 || *value > 1) {
			return fmt.Errorf("%w: scores and alpha must be 0..1", ErrConfigInvalid)
		}
	}
	// The existing fusion contract uses zero as "runtime default". Reject zero
	// here rather than persist an override that downstream would silently replace.
	if p.AgenticRAG.Alpha != nil && *p.AgenticRAG.Alpha == 0 {
		return fmt.Errorf("%w: alpha must be > 0 and <= 1; use null to reset", ErrConfigInvalid)
	}
	return nil
}

type PreferencesRepository interface {
	Read(context.Context, ConfigScope) (Preferences, error)
	// Update must first check the durable key/hash replay, then atomically CAS the version.
	Update(context.Context, ConfigScope, int64, string, string, Preferences) (Preferences, error)
}

type iterationLimitKey struct{}

// WithIterationLimit only supplies a per-run cap; the runtime retains its stricter bound.
func WithIterationLimit(ctx context.Context, n int) context.Context {
	return context.WithValue(ctx, iterationLimitKey{}, n)
}
func EffectiveIterationLimit(ctx context.Context, configured int) int {
	n, _ := ctx.Value(iterationLimitKey{}).(int)
	if n > 0 && n < configured {
		return n
	}
	return configured
}
