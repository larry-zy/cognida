//go:build integration

package lifecycle

import (
	"cognida/internal/infrastructure/job"
	store "cognida/internal/infrastructure/objectstore"
	obs "cognida/internal/infrastructure/observability"
	agent "cognida/internal/model/agent"
	d "cognida/internal/model/lifecycle"
	repo "cognida/internal/repository/mysql"
	"cognida/internal/service/agent/framework"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"testing"
	"time"
)

type computeFixture struct {
	score     float64
	failClean bool
}

func (c *computeFixture) Call(_ context.Context, op string, input d.Input, _ map[string]any, _ string) (ComputeResult, error) {
	r := ComputeResult{SchemaVersion: "1", Method: "integration_rules", Result: map[string]any{}}
	switch op {
	case "profile":
		r.Result["overall_score"] = c.score
	case "clean":
		if c.failClean {
			return r, errors.New("temporary compute failure")
		}
		r.Result["data"] = input
	case "enrich":
		r.Result["tags"] = map[string]any{"value": []string{"verified"}, "method": "rules"}
	}
	return r, nil
}
func integrationService(t *testing.T) (*Service, *gorm.DB, *job.RedisQueue, d.Principal, *computeFixture) {
	t.Helper()
	dsn := os.Getenv("MYSQL_DSN")
	require.NotEmpty(t, dsn, "explicit isolated MYSQL_DSN required")
	db, e := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	sql, e := db.DB()
	require.NoError(t, e)
	var count int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='lifecycle_runs'").Scan(&count).Error)
	require.EqualValues(t, 1, count)
	p := d.Principal{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 991}
	t.Cleanup(func() {
		for _, table := range []string{"lifecycle_events", "lifecycle_approvals", "lifecycle_stages", "lifecycle_runs", "lifecycle_assets"} {
			require.NoError(t, db.Table(table).Where("tenant_id = ?", p.TenantID).Delete(map[string]any{}).Error)
		}
		require.NoError(t, sql.Close())
	})
	redisAddress := os.Getenv("REDIS_ADDR")
	if redisAddress == "" {
		redisAddress = "127.0.0.1:6379"
	}
	client := redis.NewClient(&redis.Options{Addr: redisAddress, Password: os.Getenv("REDIS_PASSWORD")})
	require.NoError(t, client.Ping(context.Background()).Err())
	prefix := "cognida:lifecycle-test:" + uuid.NewString()
	t.Cleanup(func() {
		var cursor uint64
		for {
			keys, next, e := client.Scan(context.Background(), cursor, prefix+":*", 100).Result()
			require.NoError(t, e)
			if len(keys) > 0 {
				require.NoError(t, client.Del(context.Background(), keys...).Err())
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
		require.NoError(t, client.Close())
	})
	q := job.NewRedisQueueWithPrefix(client, prefix)
	objects, e := store.NewLocal(t.TempDir())
	require.NoError(t, e)
	compute := &computeFixture{score: 95}
	s, e := NewService(repo.NewLifecycleRepository(db), objects, q, Config{Compute: compute, Authorize: func(_ context.Context, p d.Principal, scope string) error {
		if scope != "read" && p.UserID != 991 {
			return d.ErrForbidden
		}
		return p.Validate()
	}, Publish: func(_ context.Context, _ d.Principal, ref d.DataAssetRef, _ json.RawMessage, _ map[string]any) (map[string]any, error) {
		return map[string]any{"knowledge_id": ref.AssetID, "publication_status": "published"}, nil
	}})
	require.NoError(t, e)
	return s, db, q, p, compute
}
func drain(t *testing.T, s *Service, q *job.RedisQueue) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < 30; i++ {
		j, e := q.TryClaim(ctx)
		require.NoError(t, e)
		if j == nil {
			return
		}
		require.NoError(t, s.Handle(ctx, *j))
		require.NoError(t, q.Ack(ctx, *j))
	}
	t.Fatal("unbounded pipeline")
}
func TestLifecycleIntegrationEndToEnd(t *testing.T) {
	s, db, q, p, _ := integrationService(t)
	ctx := context.Background()
	ref, e := s.Import(ctx, p, d.Input{Kind: "table", Columns: []string{"revenue"}, Rows: [][]any{{100}, {200}}}, "test")
	require.NoError(t, e)
	run, e := s.StartRun(ctx, p, "web-knowledge-to-insight", []d.RefID{ref.RefID}, nil, "run-1", "trace-1")
	require.NoError(t, e)
	replay, e := s.StartRun(ctx, p, "web-knowledge-to-insight", []d.RefID{ref.RefID}, nil, "run-1", "trace-1")
	require.NoError(t, e)
	require.Equal(t, run.ID, replay.ID)
	drain(t, s, q)
	got, e := s.repo.FindRun(ctx, p.TenantID, run.ID, false)
	require.NoError(t, e)
	require.Equal(t, d.StatusApproval, got.Status)
	approvals, e := s.repo.ListApprovals(ctx, p.TenantID, run.ID)
	require.NoError(t, e)
	require.Len(t, approvals, 1)
	require.ErrorIs(t, s.ResolveApproval(ctx, d.Principal{TenantID: p.TenantID, UserID: 123}, approvals[0].ID, "approved", ""), d.ErrForbidden)
	require.NoError(t, s.ResolveApproval(ctx, p, approvals[0].ID, "approved", "reviewed"))
	drain(t, s, q)
	got, e = s.repo.FindRun(ctx, p.TenantID, run.ID, false)
	require.NoError(t, e)
	require.Equal(t, d.StatusSucceeded, got.Status)
	stages, e := s.repo.ListStages(ctx, p.TenantID, run.ID)
	require.NoError(t, e)
	require.Len(t, stages, 7)
	for _, st := range stages {
		require.Equal(t, d.StatusSucceeded, st.Status)
		require.NotEmpty(t, st.Output.Refs)
		for _, ref := range st.Output.Refs {
			_, body, e := s.Resolve(ctx, p.TenantID, p.UserID, ref.RefID)
			require.NoError(t, e)
			require.True(t, json.Valid(body))
		}
	}
	_, _, e = s.Resolve(ctx, p.TenantID+1, p.UserID, ref.RefID)
	require.ErrorIs(t, e, d.ErrNotFound)
	var bodies []string
	require.NoError(t, db.Table("lifecycle_stages").Where("tenant_id = ?", p.TenantID).Pluck("body", &bodies).Error)
	for _, body := range bodies {
		require.NotContains(t, body, `"rows"`)
	}
	events, e := s.repo.Events(ctx, p.TenantID, run.ID, 0)
	require.NoError(t, e)
	require.NotEmpty(t, events)
	for _, event := range events {
		require.Equal(t, "trace-1", event.RequestID)
	}
	run2, e := s.StartRun(ctx, p, "web-knowledge-to-insight", []d.RefID{ref.RefID}, nil, "recovery", "trace-2")
	require.NoError(t, e)
	j, e := q.TryClaim(ctx)
	require.NoError(t, e)
	require.NotNil(t, j)
	var msg delivery
	require.NoError(t, json.Unmarshal(j.Payload, &msg))
	st, e := s.repo.FindStage(ctx, p.TenantID, msg.StageID, false)
	require.NoError(t, e)
	st.Status = d.StatusSucceeded
	out := passedOutput("recovered", ref)
	st.Output = &out
	require.NoError(t, s.repo.UpdateStage(ctx, st))
	require.NoError(t, q.Ack(ctx, *j))
	jobs, e := s.PendingJobs(ctx, 100)
	require.NoError(t, e)
	require.NotEmpty(t, jobs)
	for _, j := range jobs {
		require.NoError(t, q.Enqueue(ctx, j))
	}
	drain(t, s, q)
	restored, e := s.repo.FindRun(ctx, p.TenantID, run2.ID, false)
	require.NoError(t, e)
	require.Equal(t, d.StatusApproval, restored.Status)
}
func TestLifecycleIntegrationGatesCancelExpiry(t *testing.T) {
	s, db, q, p, compute := integrationService(t)
	ctx := context.Background()
	ref, e := s.Import(ctx, p, d.Input{Kind: "text", Text: "valid text"}, "")
	require.NoError(t, e)
	_, e = s.Invoke(ctx, p, "data-analysis", []d.RefID{ref.RefID}, nil, "bad", "trace")
	require.ErrorIs(t, e, d.ErrInvalid)
	compute.score = 10
	st, e := s.Invoke(ctx, p, "data-cleaning", []d.RefID{ref.RefID}, nil, "clean", "trace")
	require.NoError(t, e)
	drain(t, s, q)
	got, e := s.repo.FindStage(ctx, p.TenantID, st.ID, false)
	require.NoError(t, e)
	require.Equal(t, d.StatusQuarantined, got.Status)
	require.Empty(t, got.RunID)
	st, e = s.Invoke(ctx, p, "data-access", []d.RefID{ref.RefID}, nil, "cancel", "trace")
	require.NoError(t, e)
	require.NoError(t, s.CancelStage(ctx, p, st.ID))
	drain(t, s, q)
	got, e = s.repo.FindStage(ctx, p.TenantID, st.ID, false)
	require.NoError(t, e)
	require.Equal(t, d.StatusCancelled, got.Status)
	st, e = s.Invoke(ctx, p, "data-asset", []d.RefID{ref.RefID}, nil, "publish", "trace")
	require.NoError(t, e)
	drain(t, s, q)
	approvals, e := s.repo.ListApprovals(ctx, p.TenantID, "")
	require.NoError(t, e)
	require.Len(t, approvals, 1)
	a := approvals[0]
	a.ExpiresAt = time.Now().Add(-time.Minute)
	require.NoError(t, s.repo.UpdateApproval(ctx, &a))
	require.NoError(t, db.Table("lifecycle_approvals").Where("tenant_id=? AND id=?", p.TenantID, a.ID).Update("expires_at", a.ExpiresAt).Error)
	_, e = s.PendingJobs(ctx, 100)
	require.NoError(t, e)
	got, e = s.repo.FindStage(ctx, p.TenantID, st.ID, false)
	require.NoError(t, e)
	require.Equal(t, d.StatusQuarantined, got.Status)
}
func TestLifecycleIntegrationRetryAndStableArtifacts(t *testing.T) {
	s, _, q, p, compute := integrationService(t)
	ctx := context.Background()
	ref, e := s.Import(ctx, p, d.Input{Kind: "text", Text: "hello"}, "")
	require.NoError(t, e)
	compute.failClean = true
	st, e := s.Invoke(ctx, p, "data-cleaning", []d.RefID{ref.RefID}, nil, "retry", "trace")
	require.NoError(t, e)
	j, e := q.TryClaim(ctx)
	require.NoError(t, e)
	require.NotNil(t, j)
	require.Error(t, s.Handle(ctx, *j))
	require.NoError(t, q.Retry(ctx, *j, 0))
	_, e = q.PromoteDue(ctx, 10)
	require.NoError(t, e)
	compute.failClean = false
	drain(t, s, q)
	got, e := s.repo.FindStage(ctx, p.TenantID, st.ID, false)
	require.NoError(t, e)
	require.Equal(t, d.StatusSucceeded, got.Status)
	require.Equal(t, 2, got.Attempt)
	body := json.RawMessage(`{"kind":"text","text":"same"}`)
	one, e := s.ImportDerived(ctx, p, d.CleanedAsset, "stable", []d.RefID{ref.RefID}, body, ref.Policy)
	require.NoError(t, e)
	two, e := s.ImportDerived(ctx, p, d.CleanedAsset, "stable", []d.RefID{ref.RefID}, body, ref.Policy)
	require.NoError(t, e)
	require.Equal(t, one.RefID, two.RefID)
	require.Equal(t, 2, one.Version)
	_, e = s.Invoke(ctx, p, "data-cleaning", []d.RefID{ref.RefID}, map[string]any{"keep": "last"}, "retry", "trace")
	require.ErrorIs(t, e, d.ErrConflict)
}

func TestLifecycleIntegrationFanInAndRunRetry(t *testing.T) {
	s, db, q, p, _ := integrationService(t)
	ctx := context.Background()
	ref, e := s.Import(ctx, p, d.Input{Kind: "text", Text: "fan in"}, "")
	require.NoError(t, e)
	definition := d.PipelineDefinition{ID: "test-" + uuid.NewString(), Version: 1, Name: "fan-in", MaxStages: 3, Enabled: true, Stages: []d.StageDefinition{{ID: "left", AgentID: "data-access", MaxAttempts: 1}, {ID: "right", AgentID: "data-access", MaxAttempts: 1}, {ID: "join", AgentID: "data-quality", DependsOn: []string{"left", "right"}, AllowedGates: []string{d.GatePassed}, MaxAttempts: 1}}}
	require.NoError(t, definition.Validate(s.contracts))
	s.templates[definition.ID] = definition
	t.Cleanup(func() {
		require.NoError(t, db.Table("lifecycle_definitions").Where("id = ?", definition.ID).Delete(map[string]any{}).Error)
	})
	fail := true
	s.adapters["data-quality"] = adapterFunc(func(_ context.Context, r d.StageRequest) (d.Output, error) {
		if fail {
			return d.Output{}, errors.New("temporary join failure")
		}
		require.Len(t, r.Inputs, 2)
		return passedOutput("joined", r.Inputs...), nil
	})
	run, e := s.StartRun(ctx, p, definition.ID, []d.RefID{ref.RefID}, nil, "fan", "trace")
	require.NoError(t, e)
	drain(t, s, q)
	got, e := s.repo.FindRun(ctx, p.TenantID, run.ID, false)
	require.NoError(t, e)
	require.Equal(t, d.StatusFailed, got.Status)
	fail = false
	require.NoError(t, s.RetryRun(ctx, p, run.ID))
	drain(t, s, q)
	got, e = s.repo.FindRun(ctx, p.TenantID, run.ID, false)
	require.NoError(t, e)
	require.Equal(t, d.StatusSucceeded, got.Status)
}
func TestLifecycleIntegrationCancellationFencesRunningResult(t *testing.T) {
	s, _, q, p, _ := integrationService(t)
	ctx := context.Background()
	ref, e := s.Import(ctx, p, d.Input{Kind: "text", Text: "cancel running"}, "")
	require.NoError(t, e)
	started := make(chan struct{})
	release := make(chan struct{})
	s.adapters["data-access"] = adapterFunc(func(ctx context.Context, r d.StageRequest) (d.Output, error) {
		close(started)
		<-release
		return passedOutput("stale result", r.Inputs...), nil
	})
	st, e := s.Invoke(ctx, p, "data-access", []d.RefID{ref.RefID}, nil, "running", "trace")
	require.NoError(t, e)
	j, e := q.TryClaim(ctx)
	require.NoError(t, e)
	require.NotNil(t, j)
	done := make(chan error, 1)
	go func() { done <- s.Handle(ctx, *j) }()
	<-started
	require.NoError(t, s.CancelStage(ctx, p, st.ID))
	close(release)
	require.NoError(t, <-done)
	require.NoError(t, q.Ack(ctx, *j))
	got, e := s.repo.FindStage(ctx, p.TenantID, st.ID, false)
	require.NoError(t, e)
	require.Equal(t, d.StatusCancelled, got.Status)
	require.Nil(t, got.Output)
}

func TestLifecycleIntegrationRealPython(t *testing.T) {
	base := os.Getenv("LIFECYCLE_TEST_PYTHON_URL")
	if base == "" {
		t.Skip("set LIFECYCLE_TEST_PYTHON_URL to run the live internal compute contract")
	}
	s, _, q, p, _ := integrationService(t)
	s.config.Compute = NewHTTPCompute(base, os.Getenv("LIFECYCLE_TEST_TOKEN"))
	ctx := context.Background()
	ref, e := s.Import(ctx, p, d.Input{Kind: "table", Columns: []string{"region", "revenue"}, Rows: [][]any{{"east", 100}, {"west", 200}, {"north", 300}}}, "live-python")
	require.NoError(t, e)
	opts := map[string]any{"stage_options": map[string]any{"quality": map[string]any{"min_score": float64(0)}, "cleaning": map[string]any{"min_score": float64(0)}}}
	run, e := s.StartRun(ctx, p, "web-knowledge-to-insight", []d.RefID{ref.RefID}, opts, "live-python", "trace-live")
	require.NoError(t, e)
	drain(t, s, q)
	approvals, e := s.repo.ListApprovals(ctx, p.TenantID, run.ID)
	require.NoError(t, e)
	require.Len(t, approvals, 1)
	require.NoError(t, s.ResolveApproval(ctx, p, approvals[0].ID, "approved", "integration"))
	drain(t, s, q)
	final, e := s.repo.FindRun(ctx, p.TenantID, run.ID, false)
	require.NoError(t, e)
	require.Equal(t, d.StatusSucceeded, final.Status)
	stages, e := s.repo.ListStages(ctx, p.TenantID, run.ID)
	require.NoError(t, e)
	for _, st := range stages {
		if st.AgentID == "data-quality" {
			report := st.Output.Refs[1]
			_, payload, e := s.Resolve(ctx, p.TenantID, p.UserID, report.RefID)
			require.NoError(t, e)
			require.Contains(t, string(payload), "quality-engine-v1")
		}
		if st.AgentID == "data-enrichment" {
			require.NotEmpty(t, st.Output.Metadata["recommendations"])
		}
		if st.AgentID == "data-analysis" {
			require.Equal(t, "verified_computation", st.Output.Decision["verification_status"])
		}
	}
}

func TestLifecycleIntegrationSpecialistAndOrchestratorIsolation(t *testing.T) {
	s, _, q, p, _ := integrationService(t)
	ctx := context.Background()
	ref, e := s.Import(ctx, p, d.Input{Kind: "text", Text: "registry fixture"}, "")
	require.NoError(t, e)
	registry := framework.NewSpecRegistry()
	collab := framework.NewCollaborationRegistry()
	require.NoError(t, s.RegisterAgents(ctx, registry, collab))
	analysis, ok := registry.GetInstance("lifecycle-data-analysis")
	require.True(t, ok)
	require.Equal(t, "data-analysis", analysis.Name())
	gov := collab.GetGovernance("data-analysis")
	require.NotNil(t, gov.Contract)
	require.NotContains(t, gov.Tools, "sql_mutate")
	require.Equal(t, agent.ContextModeIsolated, collab.GetContextMode("data-analysis"))
	ctx = agent.WithTenantID(ctx, p.TenantID)
	ctx = agent.WithUserID(ctx, p.UserID)
	ctx = agent.WithRequestID(ctx, "typed-trace")
	orchestrator, ok := registry.GetInstance("pipeline-orchestrator")
	require.True(t, ok)
	_, e = orchestrator.Chat(ctx, `{"action":"sql_mutate","run_id":"anything"}`)
	require.ErrorIs(t, e, d.ErrInvalid)
	_, e = orchestrator.Chat(ctx, `{"action":"templates","tenant_id":1}`)
	require.ErrorIs(t, e, d.ErrInvalid)
	access, ok := registry.GetInstance("lifecycle-data-access")
	require.True(t, ok)
	command, _ := json.Marshal(map[string]any{"input_refs": []d.RefID{ref.RefID}, "idempotency_key": "typed"})
	response, e := access.Chat(ctx, string(command))
	require.NoError(t, e)
	require.NotContains(t, response.Content, "registry fixture")
	var invocation d.StageRun
	require.NoError(t, json.Unmarshal([]byte(response.Content), &invocation))
	require.Empty(t, invocation.RunID)
	drain(t, s, q)
	st, e := s.repo.FindStage(ctx, p.TenantID, invocation.ID, false)
	require.NoError(t, e)
	require.Equal(t, d.StatusSucceeded, st.Status)
}

type failTerminalRepository struct {
	d.Repository
	fail *bool
}

func (r failTerminalRepository) Transaction(ctx context.Context, fn func(d.Repository) error) error {
	return r.Repository.Transaction(ctx, func(tx d.Repository) error { return fn(failTerminalRepository{Repository: tx, fail: r.fail}) })
}
func (r failTerminalRepository) UpdateStage(ctx context.Context, st *d.StageRun) error {
	if *r.fail && st.Status == d.StatusSucceeded {
		*r.fail = false
		return errors.New("injected terminal commit failure")
	}
	return r.Repository.UpdateStage(ctx, st)
}
func TestLifecycleIntegrationComputeCheckpoint(t *testing.T) {
	s, _, q, p, compute := integrationService(t)
	ctx := context.Background()
	ref, e := s.Import(ctx, p, d.Input{Kind: "text", Text: "checkpoint"}, "")
	require.NoError(t, e)
	stage, e := s.Invoke(ctx, p, "data-quality", []d.RefID{ref.RefID}, nil, "checkpoint", "trace")
	require.NoError(t, e)
	fail := true
	s.repo = failTerminalRepository{Repository: s.repo, fail: &fail}
	j, e := q.TryClaim(ctx)
	require.NoError(t, e)
	require.NotNil(t, j)
	require.ErrorContains(t, s.Handle(ctx, *j), "injected terminal commit failure")
	require.NoError(t, q.Retry(ctx, *j, 0))
	_, e = q.PromoteDue(ctx, 10)
	require.NoError(t, e)
	compute.score = 1
	drain(t, s, q)
	got, e := s.repo.FindStage(ctx, p.TenantID, stage.ID, false)
	require.NoError(t, e)
	require.Equal(t, d.StatusSucceeded, got.Status)
	require.Equal(t, float64(95), *got.Output.Gate.Score, "restore prior compute checkpoint rather than recompute changed score")
}
func TestLifecycleIntegrationDeclaredReadApproval(t *testing.T) {
	s, db, q, p, _ := integrationService(t)
	ctx := context.Background()
	ref, e := s.Import(ctx, p, d.Input{Kind: "text", Text: "approval"}, "")
	require.NoError(t, e)
	definition := d.PipelineDefinition{ID: "test-" + uuid.NewString(), Version: 1, Name: "read approval", MaxStages: 1, Enabled: true, Stages: []d.StageDefinition{{ID: "access", AgentID: "data-access", Approval: true, MaxAttempts: 2}}}
	require.NoError(t, definition.Validate(s.contracts))
	s.templates[definition.ID] = definition
	t.Cleanup(func() {
		require.NoError(t, db.Table("lifecycle_definitions").Where("id=?", definition.ID).Delete(map[string]any{}).Error)
	})
	run, e := s.StartRun(ctx, p, definition.ID, []d.RefID{ref.RefID}, nil, "read-approval", "trace")
	require.NoError(t, e)
	drain(t, s, q)
	got, e := s.repo.FindRun(ctx, p.TenantID, run.ID, false)
	require.NoError(t, e)
	require.Equal(t, d.StatusApproval, got.Status)
	approvals, e := s.repo.ListApprovals(ctx, p.TenantID, run.ID)
	require.NoError(t, e)
	require.Len(t, approvals, 1)
}

func TestLifecycleIntegrationAssetVersionCursor(t *testing.T) {
	s, _, _, p, _ := integrationService(t)
	ctx := context.Background()
	first, e := s.Import(ctx, p, d.Input{Kind: "text", Text: "versioned payload"}, "version cursor")
	require.NoError(t, e)
	second := first
	second.Version = 2
	second.AssetRefID = second.RefID.String()
	require.NoError(t, s.repo.PutAsset(ctx, second))
	page, e := s.repo.ListAssets(ctx, p.TenantID, d.RawAsset, "", 1)
	require.NoError(t, e)
	require.Len(t, page, 1)
	require.Equal(t, first.RefID, page[0].RefID)
	page, e = s.repo.ListAssets(ctx, p.TenantID, d.RawAsset, page[0].RefID.String(), 1)
	require.NoError(t, e)
	require.Len(t, page, 1)
	require.Equal(t, second.RefID, page[0].RefID)
	page, e = s.repo.ListAssets(ctx, p.TenantID, d.RawAsset, page[0].RefID.String(), 1)
	require.NoError(t, e)
	require.Empty(t, page)
}

func TestLifecycleIntegrationAuditAndTrace(t *testing.T) {
	s, db, q, p, _ := integrationService(t)
	ctx := context.Background()
	previous := otel.GetTracerProvider()
	shutdown, e := obs.InitTracing(repo.NewTraceRepository(db))
	require.NoError(t, e)
	t.Cleanup(func() {
		require.NoError(t, shutdown(context.Background()))
		otel.SetTracerProvider(previous)
		require.NoError(t, db.Table("trace_spans").Where("tenant_id=?", p.TenantID).Delete(map[string]any{}).Error)
	})
	records := []map[string]any{}
	s.config.Audit = func(_ context.Context, principal d.Principal, action, resource string, details map[string]any) {
		require.Equal(t, p.TenantID, principal.TenantID)
		details["action"] = action
		details["resource"] = resource
		records = append(records, details)
	}
	ref, e := s.Import(ctx, p, d.Input{Kind: "text", Text: "private body must never be in audit"}, "test")
	require.NoError(t, e)
	stage, e := s.Invoke(ctx, p, "data-access", []d.RefID{ref.RefID}, nil, "audit-trace", "audit-request")
	require.NoError(t, e)
	drain(t, s, q)
	require.NoError(t, shutdown(context.Background()))
	var rows []repo.TraceSpanModel
	require.NoError(t, db.Where("tenant_id=? AND request_id=?", p.TenantID, "audit-request").Find(&rows).Error)
	require.NotEmpty(t, rows)
	found := false
	for _, r := range rows {
		if r.Name == "agent.orchestration" {
			require.Contains(t, r.Attributes, stage.ID)
			require.Contains(t, r.Attributes, "lifecycle:"+stage.ID)
			found = true
		}
	}
	require.True(t, found)
	raw, e := json.Marshal(records)
	require.NoError(t, e)
	require.Contains(t, string(raw), "lifecycle.stage.execute")
	require.Contains(t, string(raw), "audit-request")
	require.Contains(t, string(raw), "succeeded")
	require.NotContains(t, string(raw), "private body")
}
