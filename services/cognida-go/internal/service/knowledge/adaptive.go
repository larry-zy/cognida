package knowledge

import (
	"context"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type RetrievalAttempt struct {
	Mode      string `json:"mode"`
	Chunks    int    `json:"chunks"`
	LatencyMS int64  `json:"latency_ms"`
	Failed    bool   `json:"failed"`
}

var identifierQuery = regexp.MustCompile("(?:[A-Za-z]+[-_][A-Za-z0-9_-]+|[A-Za-z]+[0-9]{2,}|`[^`]+`|\"[^\"]+\")")

// Auto mode chooses from existing governed strategies, then makes at most one
// fallback attempt if no eligible evidence survives. It never compares scores
// across vector and keyword scales or changes a tenant's saved configuration.
func (c *RetrievalCapability) retrieveAdaptive(ctx context.Context, tenant int64, kbs []string, q GovernedQuery) (*GovernedResult, error) {
	mode := "hybrid"
	if identifierQuery.MatchString(q.Query) {
		mode = "bm25"
	}
	q.Mode = mode
	if q.TopK <= 0 {
		q.TopK = 8
		if utf8.RuneCountInString(q.Query) > 80 {
			q.TopK = 16
		}
	}
	started := time.Now()
	result, err := c.Retrieve(ctx, tenant, kbs, q)
	attempts := []RetrievalAttempt{{Mode: mode, LatencyMS: time.Since(started).Milliseconds(), Failed: err != nil}}
	if result != nil {
		attempts[0].Chunks = len(result.Chunks)
	}
	if err == nil && result != nil && result.HasAnswer {
		result.Attempts = attempts
		return result, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	fallback := "vector"
	if mode == "bm25" {
		fallback = "hybrid"
	}
	q.Mode = fallback
	started = time.Now()
	second, secondErr := c.Retrieve(ctx, tenant, kbs, q)
	attempt := RetrievalAttempt{Mode: fallback, LatencyMS: time.Since(started).Milliseconds(), Failed: secondErr != nil}
	if second != nil {
		attempt.Chunks = len(second.Chunks)
	}
	attempts = append(attempts, attempt)
	if secondErr != nil {
		return nil, secondErr
	}
	second.Attempts = attempts
	return second, nil
}

// Adaptive chunking uses only document structure and records its chosen
// strategy in the gRPC request. Existing explicitly chosen strategies win.
func adaptiveChunkPlan(text string) (string, int, int) {
	length := utf8.RuneCountInString(text)
	switch {
	case strings.Count(text, "\n\n") >= 2:
		return "paragraph", 1000, 100
	case strings.Contains(text, "\n#") || strings.Count(text, "\n") >= 10:
		return "recursive", 1000, 100
	case length > 2000:
		return "sentence", 800, 80
	default:
		return "paragraph", 1000, 100
	}
}
