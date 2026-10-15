//go:build integration

package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	d "cognida/internal/model/lifecycle"
	"github.com/stretchr/testify/require"
)

func TestReviewAnalysisReplayMustReuseDurableResult(t *testing.T) {
	s, _, _, p, _ := integrationService(t)
	ctx := context.Background()
	input := d.Input{Kind: "text", Text: "evidence"}
	raw, err := s.Import(ctx, p, input, "source")
	require.NoError(t, err)
	body, err := json.Marshal(input)
	require.NoError(t, err)
	policy := raw.Policy
	policy.Approved = true
	published, err := s.ImportDerived(ctx, p, d.PublishedAsset, "review-publish", []d.RefID{raw.RefID}, body, policy)
	require.NoError(t, err)
	calls := 0
	s.config.Compute = goalCompute(func(context.Context, string, d.Input, map[string]any, string) (ComputeResult, error) {
		calls++
		return ComputeResult{Method: "goal-directed-llm-v1", Result: map[string]any{"narrative": fmt.Sprintf("interpretation %d", calls), "verification_status": "citations_checked_interpretation_unverified"}}, nil
	})
	r := d.StageRequest{Principal: p, Stage: d.StageRun{ID: "review-analysis", InputRefs: []d.RefID{published.RefID}, Options: map[string]any{"prompt": "summarize evidence"}}, Inputs: []d.DataAssetRef{published}, Payloads: []json.RawMessage{body}}
	first, err := s.analyze(ctx, r)
	require.NoError(t, err)
	second, err := s.analyze(ctx, r)
	require.NoError(t, err, "retry must reuse durable result after stage-completion failure")
	require.Equal(t, first.Refs[0].RefID, second.Refs[0].RefID)
	require.Equal(t, 1, calls)
}
