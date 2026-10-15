//go:build integration

package governance_test

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	lifecycle "cognida/internal/service/lifecycle"
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

type reportGoalCompute struct {
	t     *testing.T
	calls int
	goal  string
}

func (c *reportGoalCompute) Call(_ context.Context, op string, input l.Input, opts map[string]any, requestID string) (lifecycle.ComputeResult, error) {
	c.calls++
	require.Equal(c.t, "analyze", op)
	require.Equal(c.t, c.goal, opts["prompt"])
	require.NotEmpty(c.t, opts["source_ref"])
	require.Contains(c.t, requestID, "subscription:")
	return lifecycle.ComputeResult{SchemaVersion: "1", Method: "goal-directed-llm-v1", Result: map[string]any{"narrative": "围绕目标的销售分析", "findings": []map[string]any{{"text": "销售均值15", "evidence_ids": []string{"stat:column:0"}}}, "evidence": []map[string]any{{"id": "stat:column:0", "content": "mean:15"}}, "verification_status": "citations_checked_interpretation_unverified"}}, nil
}
func TestGovernanceReportGoalSnapshotAndResultAPI(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	goal := &reportGoalCompute{t: t, goal: "解释销售差异"}
	f.s.Compute = goal
	ref := f.publish(t, l.Input{Kind: "table", Columns: []string{"sales"}, Rows: [][]any{{10}, {20}}}, "sales", "public")
	sub, err := f.s.SaveSubscription(ctx, f.p, "", "goal-report", d.Subscription{Name: "goal", Kind: "report", Channel: "in_app", Schedule: d.Schedule{Cron: "0 9 * * *", Timezone: "UTC"}, Input: d.AnalysisInput{AssetRefID: ref.AssetRefID, Prompt: goal.goal}}, 0)
	require.NoError(t, err)
	run, err := f.s.RunSubscription(ctx, f.p, sub.ID, "goal-run")
	require.NoError(t, err)
	delivery, err := f.q.TryClaim(ctx)
	require.NoError(t, err)
	require.NotNil(t, delivery)
	require.NoError(t, f.s.Handle(ctx, *delivery))
	require.NoError(t, f.q.Ack(ctx, *delivery))
	require.Equal(t, 1, goal.calls)
	code, out := f.api(t, f.p.UserID, f.p.TenantID, "GET", "/subscription-runs/"+run.ID, nil)
	require.Equal(t, 200, code, out)
	resultID := out["data"].(map[string]any)["result_ref_id"].(string)
	id, err := l.ParseRefID(resultID)
	require.NoError(t, err)
	_, raw, err := f.assets.Resolve(ctx, f.p.TenantID, f.p.UserID, id)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal(raw, &result))
	require.Equal(t, goal.goal, result["prompt"])
	require.Equal(t, "围绕目标的销售分析", result["narrative"])
	require.Equal(t, "citations_checked_interpretation_unverified", result["decision"].(map[string]any)["verification_status"])
	require.Equal(t, 15.0, result["columns"].([]any)[0].(map[string]any)["mean"])
	code, _ = f.api(t, f.p.UserID, f.p.TenantID+1, "GET", "/subscription-runs/"+run.ID, nil)
	require.Equal(t, 404, code)
}
