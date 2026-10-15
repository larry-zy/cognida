//go:build integration

package governance_test

import (
	"context"
	"errors"
	"testing"

	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	lifecycle "cognida/internal/service/lifecycle"
	"github.com/stretchr/testify/require"
)

type notificationFailureOnce struct {
	d.Repository
	failed bool
}

func (r *notificationFailureOnce) Create(ctx context.Context, record *d.Record) (*d.Record, error) {
	if record.Kind == "notification" && !r.failed {
		r.failed = true
		return nil, errors.New("review: transient notification failure")
	}
	return r.Repository.Create(ctx, record)
}

func TestReviewReportNotificationRetryReusesDurableInterpretation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	compute := &reportGoalCompute{t: t, goal: "解释销售差异"}
	f.s.Compute = compute
	repo := &notificationFailureOnce{Repository: f.s.Repo}
	f.s.Repo = repo
	ref := f.publish(t, l.Input{Kind: "table", Columns: []string{"sales"}, Rows: [][]any{{10}, {20}}}, "sales", "public")
	sub, err := f.s.SaveSubscription(ctx, f.p, "", "review-goal-report", d.Subscription{Name: "goal", Kind: "report", Channel: "in_app", Schedule: d.Schedule{Cron: "0 9 * * *", Timezone: "UTC"}, Input: d.AnalysisInput{AssetRefID: ref.AssetRefID, Prompt: compute.goal}}, 0)
	require.NoError(t, err)
	run, err := f.s.RunSubscription(ctx, f.p, sub.ID, "review-goal-run")
	require.NoError(t, err)
	delivery, err := f.q.TryClaim(ctx)
	require.NoError(t, err)
	require.NotNil(t, delivery)
	err = f.s.Handle(ctx, *delivery)
	require.ErrorContains(t, err, "transient notification failure")
	require.Equal(t, 1, compute.calls)
	resultID := lifecycle.DerivedRefID(f.p.TenantID, l.Insight, run.ID, []l.RefID{ref.RefID})
	_, before, err := f.assets.Resolve(ctx, f.p.TenantID, f.p.UserID, resultID)
	require.NoError(t, err)
	require.NoError(t, f.s.Handle(ctx, *delivery))
	require.Equal(t, 1, compute.calls, "a notification retry must not call the model again")
	_, after, err := f.assets.Resolve(ctx, f.p.TenantID, f.p.UserID, resultID)
	require.NoError(t, err)
	require.Equal(t, before, after)
	completed, err := f.s.Get(ctx, f.p, "subscription_run", run.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", completed.Status)
	notifications, err := f.s.List(ctx, f.p, d.Filter{Kind: "notification"})
	require.NoError(t, err)
	require.Len(t, notifications.Items, 1)
	require.NoError(t, f.s.Handle(ctx, *delivery))
	require.Equal(t, 1, compute.calls)
	require.NoError(t, f.q.Ack(ctx, *delivery))
}
