//go:build integration

package governance_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"cognida/internal/infrastructure/job"
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// The production scheduler spans tenants. This adapter must never dispatch
// review records belonging to another developer or running test.
type scopedDueRepo struct {
	d.Repository
	db          *gorm.DB
	tenant      int64
	failAdvance bool
}

func (r scopedDueRepo) Due(ctx context.Context, at time.Time, limit int) ([]d.Record, error) {
	var rows []d.Record
	err := r.db.WithContext(ctx).Table("governance_records").Where("tenant_id=? AND kind='subscription' AND status='enabled' AND due_at<=?", r.tenant, at).Order("due_at,id").Limit(limit).Find(&rows).Error
	return rows, err
}
func (r scopedDueRepo) Transaction(ctx context.Context, fn func(d.Repository) error) error {
	return r.Repository.Transaction(ctx, func(tx d.Repository) error { r.Repository = tx; return fn(r) })
}
func (r scopedDueRepo) Update(ctx context.Context, record *d.Record, version int) error {
	if r.failAdvance && record.Kind == "subscription" {
		return errors.New("injected advancement failure")
	}
	return r.Repository.Update(ctx, record, version)
}

type unavailableQueue struct{ job.Queue }

func (q unavailableQueue) Enqueue(context.Context, job.Job) error {
	return errors.New("injected queue outage")
}

func (f *fixture) dueSubscription(t *testing.T, name string) *d.Record {
	t.Helper()
	ref := f.publish(t, l.Input{Kind: "text", Text: "sales increased"}, "db/schedule", "public")
	sub, e := f.s.SaveSubscription(context.Background(), f.p, "", name, d.Subscription{Name: name, Kind: "report", Enabled: true, Channel: "in_app", Schedule: d.Schedule{Cron: "* * * * *", Timezone: "UTC"}, Input: d.AnalysisInput{AssetRefID: ref.AssetRefID}}, 0)
	require.NoError(t, e)
	at := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	require.NoError(t, f.db.Table("governance_records").Where("tenant_id=? AND id=?", f.p.TenantID, sub.ID).Update("due_at", at).Error)
	return sub
}
func TestGovernanceIntegrationScheduleOutageRecoveryAndAtomicity(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	sub := f.dueSubscription(t, "queue outage")
	base := f.s.Repo
	f.s.Repo = scopedDueRepo{Repository: base, db: f.db, tenant: f.p.TenantID, failAdvance: true}
	require.ErrorContains(t, f.s.Tick(ctx), "advancement failure")
	var n int64
	require.NoError(t, f.db.Table("governance_records").Where("tenant_id=? AND kind='subscription_run'", f.p.TenantID).Count(&n).Error)
	require.Zero(t, n, "failed schedule CAS/write must roll back the run")
	f.s.Repo = scopedDueRepo{Repository: base, db: f.db, tenant: f.p.TenantID}
	f.s.Queue = unavailableQueue{f.q}
	require.ErrorContains(t, f.s.Tick(ctx), "queue outage")
	stored, e := f.s.Get(ctx, f.p, "subscription", sub.ID)
	require.NoError(t, e)
	require.True(t, stored.DueAt.After(time.Now()), "queue failure must not pin the due page")
	runs, e := f.s.List(ctx, f.p, d.Filter{Kind: "subscription_run", Limit: 100})
	require.NoError(t, e)
	require.Len(t, runs.Items, 1)
	require.Equal(t, "queued", runs.Items[0].Status)
	require.NoError(t, f.s.Tick(ctx))
	f.s.Queue = f.q
	dispatcher := job.NewDispatcher(f.q, job.RetryPolicy{MaxAttempts: 3})
	dispatcher.Register("governance", f.s, f.s)
	recovered, e := dispatcher.Recover(ctx, 100)
	require.NoError(t, e)
	require.Equal(t, 1, recovered)
	delivery, e := f.q.TryClaim(ctx)
	require.NoError(t, e)
	require.NotNil(t, delivery)
	require.NoError(t, f.s.Handle(ctx, *delivery))
	require.NoError(t, f.q.Ack(ctx, *delivery))
	code, out := f.api(t, 991, f.p.TenantID, "GET", "/subscription-runs/"+runs.Items[0].ID, nil)
	require.Equal(t, 200, code, out)
	require.Equal(t, "completed", out["data"].(map[string]any)["status"])
	notifications, e := f.s.List(ctx, f.p, d.Filter{Kind: "notification", Limit: 100})
	require.NoError(t, e)
	require.Len(t, notifications.Items, 1)
}
func TestGovernanceIntegrationPoisonSchedulesDoNotStarve(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	good := f.dueSubscription(t, "valid behind poison page")
	past := time.Now().UTC().Add(-2 * time.Hour)
	for i := 0; i < 100; i++ {
		payload, _ := json.Marshal(d.Subscription{Name: "old invalid config", Enabled: true, Schedule: d.Schedule{Cron: "invalid", Timezone: "UTC"}})
		_, e := f.s.Repo.Create(ctx, &d.Record{Meta: d.Meta{TenantID: f.p.TenantID, UserID: f.p.UserID}, Kind: "subscription", Status: "enabled", IdempotencyKey: fmt.Sprintf("poison-%d", i), Payload: payload, DueAt: &past})
		require.NoError(t, e)
	}
	f.s.Repo = scopedDueRepo{Repository: f.s.Repo, db: f.db, tenant: f.p.TenantID}
	require.ErrorContains(t, f.s.Tick(ctx), "invalid_schedule_configuration")
	require.NoError(t, f.s.Tick(ctx))
	stored, e := f.s.Get(ctx, f.p, "subscription", good.ID)
	require.NoError(t, e)
	require.True(t, stored.DueAt.After(time.Now()))
	var n int64
	require.NoError(t, f.db.Table("governance_records").Where("tenant_id=? AND kind='subscription' AND status='disabled' AND due_at IS NULL", f.p.TenantID).Count(&n).Error)
	require.EqualValues(t, 100, n)
	code, out := f.api(t, 991, f.p.TenantID, "GET", "/subscriptions?status=disabled", nil)
	require.Equal(t, 200, code, out)
	items := out["data"].(map[string]any)["items"].([]any)
	require.NotEmpty(t, items)
	require.Equal(t, "invalid_schedule_configuration", items[0].(map[string]any)["schedule_error"])
}
func TestGovernanceIntegrationConcurrentScheduleClaim(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.dueSubscription(t, "concurrent")
	f.s.Repo = scopedDueRepo{Repository: f.s.Repo, db: f.db, tenant: f.p.TenantID}
	start := make(chan struct{})
	failures := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; failures <- f.s.Tick(ctx) }()
	}
	close(start)
	wg.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	runs, e := f.s.List(ctx, f.p, d.Filter{Kind: "subscription_run", Limit: 100})
	require.NoError(t, e)
	require.Len(t, runs.Items, 1)
	first, e := f.q.TryClaim(ctx)
	require.NoError(t, e)
	require.NotNil(t, first)
	duplicate, e := f.q.TryClaim(ctx)
	require.NoError(t, e)
	require.Nil(t, duplicate)
	require.NoError(t, f.s.Handle(ctx, *first))
	require.NoError(t, f.q.Ack(ctx, *first))
}

type countedAssetReads struct {
	assets interface {
		Resolve(context.Context, int64, int64, l.RefID) (l.DataAssetRef, json.RawMessage, error)
		ImportDerived(context.Context, l.Principal, l.Kind, string, []l.RefID, json.RawMessage, l.PolicySnapshot) (l.DataAssetRef, error)
	}
	count int
}

func (c *countedAssetReads) Resolve(ctx context.Context, tenant, user int64, id l.RefID) (l.DataAssetRef, json.RawMessage, error) {
	c.count++
	return c.assets.Resolve(ctx, tenant, user, id)
}
func (c *countedAssetReads) ImportDerived(ctx context.Context, p l.Principal, k l.Kind, producer string, parents []l.RefID, raw json.RawMessage, policy l.PolicySnapshot) (l.DataAssetRef, error) {
	return c.assets.ImportDerived(ctx, p, k, producer, parents, raw, policy)
}
func TestGovernanceIntegrationFeatureProjectionExactNumbersAndBoundedReads(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	rows := [][]any{}
	const start uint64 = 9007199254740992
	for i := uint64(0); i < 100; i++ {
		rows = append(rows, []any{start + i, start + i})
	}
	ref := f.publish(t, l.Input{Kind: "table", Columns: []string{"entity", "value"}, Rows: rows}, "db/exact", "public")
	head, e := f.s.CreateFeature(ctx, f.p, d.Feature{Name: "exact", Type: "numerical", SourceRefID: ref.AssetRefID, Definition: json.RawMessage(`{"kind":"field","field":"value","entity_field":"entity"}`)}, "exact")
	require.NoError(t, e)
	counter := &countedAssetReads{assets: f.s.Assets}
	f.s.Assets = counter
	n, e := f.s.ComputeFeatures(ctx, f.p, head.ID, 1)
	require.NoError(t, e)
	require.Equal(t, 100, n)
	require.LessOrEqual(t, counter.count, 5, "source reads must be bounded per version, independent of entity count")
	values, e := f.s.QueryFeatures(ctx, f.p, []string{"9007199254740992", "9007199254740993"}, []d.FeatureValue{{FeatureID: head.ID, Version: 1}}, time.Now())
	require.NoError(t, e)
	require.Len(t, values, 2)
	require.Equal(t, "9007199254740992", string(values[0].Value))
	require.Equal(t, "9007199254740993", string(values[1].Value))
	n, e = f.s.ComputeFeatures(ctx, f.p, head.ID, 1)
	require.NoError(t, e)
	require.Equal(t, 100, n)
}

func TestGovernanceIntegrationNotificationConcurrentReadAndPreannotationPreservesReview(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	raw, _ := json.Marshal(d.Notification{Title: "ready"})
	notice, e := f.s.Repo.Create(ctx, &d.Record{Meta: d.Meta{TenantID: f.p.TenantID, UserID: f.p.UserID}, Kind: "notification", Status: "unread", Payload: raw})
	require.NoError(t, e)
	failures := make(chan error, 8)
	start := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() { <-start; _, err := f.s.ReadNotification(ctx, f.p, notice.ID); failures <- err }()
	}
	close(start)
	for i := 0; i < 8; i++ {
		require.NoError(t, <-failures)
	}
	code, out := f.api(t, 991, f.p.TenantID, "POST", "/notifications/"+notice.ID+"/read", nil)
	require.Equal(t, 200, code, out)
	require.Equal(t, "read", out["data"].(map[string]any)["status"])
	code, _ = f.api(t, 992, f.p.TenantID, "POST", "/notifications/"+notice.ID+"/read", nil)
	require.Equal(t, 404, code)
	ref := f.publish(t, l.Input{Kind: "text", Text: "sales"}, "db/annotation", "public")
	task, e := f.s.CreateAnnotation(ctx, f.p, d.AnnotationTask{Name: "reset", Type: "classification", SourceRefID: ref.AssetRefID, Labels: []d.Label{{ID: "sales", Name: "sales"}}}, "reset")
	require.NoError(t, e)
	require.NoError(t, f.s.Preannotate(ctx, f.p, task.ID))
	items, e := f.s.List(ctx, f.p, d.Filter{Kind: "annotation_item", ParentID: task.ID, Limit: 10})
	require.NoError(t, e)
	require.Len(t, items.Items, 1)
	rejected, e := f.s.ReviewAnnotation(ctx, f.p, task.ID, items.Items[0].ID, "rejected", "check labels", items.Items[0].Version)
	require.NoError(t, e)
	code, out = f.api(t, 991, f.p.TenantID, "POST", "/annotation-tasks/"+task.ID+"/preannotate", nil)
	require.Equal(t, 200, code, out)
	record, e := f.s.Get(ctx, f.p, "annotation_item", rejected.ID)
	require.NoError(t, e)
	var result d.AnnotationItem
	require.NoError(t, json.Unmarshal(record.Payload, &result))
	require.Equal(t, "rejected", result.ReviewStatus)
	require.Equal(t, f.p.UserID, result.ReviewedBy)
	require.Equal(t, "check labels", result.ReviewComment)
	require.Equal(t, rejected.Version, record.Version, "preannotation does not overwrite or revise a human-reviewed item")
	require.JSONEq(t, string(rejected.Payload), string(record.Payload))
}

func TestGovernanceIntegrationScheduleSuspensionRequiresExplicitReenable(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	sub := f.dueSubscription(t, "revoked owner")
	f.s.Repo = scopedDueRepo{Repository: f.s.Repo, db: f.db, tenant: f.p.TenantID}
	auth := f.s.Authorize
	f.s.Authorize = func(context.Context, l.Principal, string) error { return l.ErrForbidden }
	require.ErrorContains(t, f.s.Tick(ctx), "owner_access_unavailable")
	f.s.Authorize = auth
	code, out := f.api(t, 991, f.p.TenantID, "GET", "/subscriptions/"+sub.ID, nil)
	require.Equal(t, 200, code, out)
	data := out["data"].(map[string]any)
	require.Equal(t, "disabled", data["status"])
	require.Equal(t, "owner_access_unavailable", data["schedule_error"])
	require.NoError(t, f.s.Tick(ctx))
	runs, e := f.s.List(ctx, f.p, d.Filter{Kind: "subscription_run", Limit: 10})
	require.NoError(t, e)
	require.Empty(t, runs.Items)
	data["enabled"] = true
	data["expected_version"] = data["version"]
	code, out = f.api(t, 991, f.p.TenantID, "PUT", "/subscriptions/"+sub.ID, data)
	require.Equal(t, 200, code, out)
	require.Equal(t, "enabled", out["data"].(map[string]any)["status"])
	require.Empty(t, out["data"].(map[string]any)["schedule_error"])
}
