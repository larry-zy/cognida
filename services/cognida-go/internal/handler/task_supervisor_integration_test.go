//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cognida/internal/handler/middleware"
	llminfra "cognida/internal/infrastructure/llm"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/model/llm"
	d "cognida/internal/model/taskcontrol"
	repository "cognida/internal/repository/mysql"
	svc "cognida/internal/service/taskcontrol"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const supervisorBase = "/api/v1/governed-tasks"

type supervisorPlannerFunc func(context.Context, svc.PlanningInput) (svc.PlannerDecision, error)

func (f supervisorPlannerFunc) Propose(ctx context.Context, in svc.PlanningInput) (svc.PlannerDecision, error) {
	return f(ctx, in)
}

// The HTTP tests exercise real MySQL with a fixed authorization resolver and an
// injected planner. They are not evidence of live model or domain execution.
type supervisorHTTPFixture struct {
	t       *testing.T
	db      *gorm.DB
	repo    *repository.TaskControlRepository
	router  *gin.Engine
	owner   d.Scope
	ref     l.RefID
	extra   l.RefID
	budget  svc.Budget
	plan    svc.PlanSpec
	revoked atomic.Bool
	calls   atomic.Int64
	planner supervisorPlannerFunc
	service *svc.Service
	handler *TaskControlHandler
	kinds   map[l.RefID]l.Kind
}

func newSupervisorHTTPFixture(t *testing.T) *supervisorHTTPFixture {
	t.Helper()
	dsn := os.Getenv("COGNIDA_TASKCONTROL_TEST_DSN")
	require.NotEmpty(t, dsn, "set COGNIDA_TASKCONTROL_TEST_DSN to an isolated, migrated review database")
	cfg, err := driver.ParseDSN(dsn)
	if err != nil {
		t.Fatal("invalid supervisor test database configuration")
	}
	require.True(t, strings.HasPrefix(cfg.DBName, "cognida_review_"), "isolated review database required")
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("cannot connect to supervisor review database (connection details suppressed)")
	}
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	var version int64
	var dirty bool
	require.NoError(t, sqlDB.QueryRow("SELECT version, dirty FROM schema_migrations LIMIT 1").Scan(&version, &dirty))
	require.GreaterOrEqual(t, version, int64(13))
	require.False(t, dirty)
	f := &supervisorHTTPFixture{t: t, db: db, repo: repository.NewTaskControlRepository(db), owner: d.Scope{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 941}, ref: l.RefID{AssetID: uuid.NewString(), Version: 1}, extra: l.RefID{AssetID: uuid.NewString(), Version: 1}, budget: svc.Budget{MaxNodes: 2, MaxConcurrency: 1, MaxAttempts: 2, MaxCalls: 4, MaxCostMicros: 100, TimeoutSeconds: 60}}
	t.Cleanup(func() {
		for _, table := range []string{"task_control_leases", "task_control_outbox", "task_control_children", "task_control_plans", "task_control_tasks"} {
			require.NoError(t, db.Table(table).Where("tenant_id = ? AND user_id = ?", f.owner.TenantID, f.owner.UserID).Delete(map[string]any{}).Error)
		}
	})
	f.plan = svc.PlanSpec{SchemaVersion: 1, Budget: f.budget, Nodes: []svc.Node{{ID: "clean", Capability: "lifecycle.clean", CapabilityVersion: 1, Inputs: []svc.InputBinding{{Asset: &f.ref, Kind: l.RawAsset}}, OutputKinds: []l.Kind{l.CleanedAsset}, Tools: []string{"clean"}, Permissions: []string{"read"}, MaxAttempts: 1, MaxCallsPerAttempt: 1, MaxCostMicrosPerAttempt: 10, TimeoutSeconds: 10}}}
	validator, err := svc.NewValidator([]svc.Capability{{ID: "lifecycle.clean", Version: 1, Domain: "lifecycle", InputKinds: []l.Kind{l.RawAsset}, OutputKinds: []l.Kind{l.CleanedAsset}, Tools: []string{"clean"}, Permissions: []string{"read"}}}, func(_ context.Context, scope d.Scope, permission string) error {
		if scope.Validate() != nil || permission != "read" {
			return d.ErrForbidden
		}
		return nil
	}, func(_ context.Context, scope d.Scope, ref l.RefID) (l.Kind, error) {
		if f.revoked.Load() || scope != f.owner || (ref != f.ref && ref != f.extra) {
			return "", d.ErrForbidden
		}
		if kind, ok := f.kinds[ref]; ok {
			return kind, nil
		}
		return l.RawAsset, nil
	})
	require.NoError(t, err)
	service, err := svc.NewService(f.repo, validator)
	require.NoError(t, err)
	f.service = service
	f.planner = func(context.Context, svc.PlanningInput) (svc.PlannerDecision, error) {
		plan := f.clonePlan()
		return svc.PlannerDecision{Kind: "proposal", Explanation: "为合成测试资产生成一次清洗候选。", Plan: &plan}, nil
	}
	supervisor, err := svc.NewSupervisor(service, supervisorPlannerFunc(func(ctx context.Context, in svc.PlanningInput) (svc.PlannerDecision, error) {
		f.calls.Add(1)
		return f.planner(ctx, in)
	}))
	require.NoError(t, err)
	f.router = gin.New()
	f.router.Use(func(c *gin.Context) {
		identity := c.GetHeader("X-Test-Identity")
		if identity == "missing" {
			return
		}
		scope := f.owner
		if identity == "tenant" {
			scope.TenantID++
		}
		if identity == "user" {
			scope.UserID++
		}
		c.Set("tenant_id", scope.TenantID)
		c.Set("user_id", scope.UserID)
		c.Set("request_id", uuid.NewString())
	})
	f.router.Use(middleware.NewIdempotency(nil).Apply())
	handler := NewTaskControlHandler(service)
	f.handler = handler
	handler.SetSupervisor(supervisor)
	handler.Register(f.router.Group("/api/v1"))
	return f
}

func (f *supervisorHTTPFixture) clonePlan() svc.PlanSpec {
	f.t.Helper()
	raw, err := json.Marshal(f.plan)
	require.NoError(f.t, err)
	var plan svc.PlanSpec
	require.NoError(f.t, json.Unmarshal(raw, &plan))
	return plan
}

func (f *supervisorHTTPFixture) request(method, path string, body any, identity string) *httptest.ResponseRecorder {
	f.t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		require.NoError(f.t, err)
	}
	req := httptest.NewRequest(method, supervisorBase+path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "supervisor-test-repeated-key")
	req.Header.Set("X-Test-Identity", identity)
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	require.Equal(f.t, "no-store", w.Header().Get("Cache-Control"))
	require.Empty(f.t, w.Header().Get("Idempotency-Replayed"))
	return w
}

func supervisorDecode[T any](t *testing.T, w *httptest.ResponseRecorder, status int) T {
	t.Helper()
	require.Equal(t, status, w.Code, w.Body.String())
	var response struct {
		Data T `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	return response.Data
}

func (f *supervisorHTTPFixture) draft() d.Task {
	f.t.Helper()
	return supervisorDecode[d.Task](f.t, f.request(http.MethodPost, "", svc.DraftInput{Goal: "仅为合成原始资产做一次清洗，产出 cleaned_asset，不执行任务。", InputRefs: []l.RefID{f.ref}, Budget: f.budget}, ""), 201)
}

func (f *supervisorHTTPFixture) counts() map[string]int64 {
	f.t.Helper()
	out := map[string]int64{}
	for _, table := range []string{"task_control_tasks", "task_control_plans", "task_control_children", "task_control_outbox", "lifecycle_runs"} {
		var count int64
		require.NoError(f.t, f.db.Table(table).Where("tenant_id = ?", f.owner.TenantID).Count(&count).Error)
		out[table] = count
	}
	return out
}

func TestTaskSupervisorHTTPIntegrationClarificationPreviewAndCAS(t *testing.T) {
	f := newSupervisorHTTPFixture(t)
	before := f.counts()
	assessment := supervisorDecode[svc.Assessment](t, f.request(http.MethodPost, "/supervisor/assess", svc.AssessmentInput{}, ""), 200)
	require.False(t, assessment.Ready)
	require.False(t, assessment.ExecutionEnabled)
	require.Len(t, assessment.Questions, 3)
	require.Zero(t, f.calls.Load())
	require.Equal(t, before, f.counts())
	assessment = supervisorDecode[svc.Assessment](t, f.request(http.MethodPost, "/supervisor/assess", svc.AssessmentInput{Goal: "清洗", InputRefs: []l.RefID{f.ref}, Budget: &f.budget}, ""), 200)
	require.True(t, assessment.Ready)
	require.Empty(t, assessment.Questions)
	task := f.draft()
	path := "/" + task.ID
	before = f.counts()
	for _, kind := range []string{"clarify", "unsupported", "proposal"} {
		f.planner = func(ctx context.Context, in svc.PlanningInput) (svc.PlannerDecision, error) {
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			require.Greater(t, time.Until(deadline), 45*time.Second, "HTTP must leave room for the 45-second model request")
			require.LessOrEqual(t, time.Until(deadline), 60*time.Second)
			require.Equal(t, task.Goal, in.Goal)
			require.Equal(t, []svc.SupervisorSource{{Ref: f.ref, Kind: l.RawAsset}}, in.Sources)
			require.Equal(t, f.budget, in.Budget)
			require.Len(t, in.Capabilities, 1)
			require.Equal(t, []svc.SupervisorAnswer{{QuestionID: "format", Question: "需要哪种清洗方式？", Answer: "保留原始内容"}}, in.Answers)
			decision := svc.PlannerDecision{Kind: kind, Explanation: "合成数据的受限规划结果。"}
			if kind == "clarify" {
				decision.Questions = []svc.SupervisorQuestion{{ID: "missing_values", Question: "缺失值应如何处理？", Options: []string{"保留并标记", "删除空行"}}}
			}
			if kind == "proposal" {
				plan := f.clonePlan()
				decision.Plan = &plan
			}
			return decision, nil
		}
		proposal := supervisorDecode[svc.SupervisorProposal](t, f.request(http.MethodPost, path+"/supervisor/propose", svc.ProposeInput{ExpectedVersion: task.Version, Answers: []svc.SupervisorAnswer{{QuestionID: "format", Question: "需要哪种清洗方式？", Answer: "保留原始内容"}}}, ""), 200)
		require.Equal(t, kind, proposal.Decision.Kind)
		require.Equal(t, task.Version, proposal.TaskVersion)
		require.Equal(t, 1, proposal.PlanningCalls)
		require.False(t, proposal.Saved)
		require.False(t, proposal.ExecutionEnabled)
		if kind == "proposal" {
			require.True(t, proposal.Validation.Valid)
		}
		require.Equal(t, before, f.counts())
		stored, err := f.repo.GetTask(context.Background(), f.owner, task.ID)
		require.NoError(t, err)
		wantJSON, err := json.Marshal(task)
		require.NoError(t, err)
		gotJSON, err := json.Marshal(stored)
		require.NoError(t, err)
		require.JSONEq(t, string(wantJSON), string(gotJSON))
	}
	require.Equal(t, int64(3), f.calls.Load())
	appendInput := svc.AppendPlanInput{ExpectedVersion: task.Version, Plan: f.plan}
	saved := supervisorDecode[svc.PlanResult](t, f.request(http.MethodPost, path+"/plans", appendInput, ""), 201)
	require.Equal(t, int64(1), saved.Plan.Version)
	require.Equal(t, 409, f.request(http.MethodPost, path+"/plans", appendInput, "").Code)
	require.Equal(t, 409, f.request(http.MethodPost, path+"/supervisor/propose", svc.ProposeInput{ExpectedVersion: task.Version}, "").Code)
	require.Equal(t, int64(3), f.calls.Load(), "stale requests must not spend a model call")
	summary := supervisorDecode[svc.SupervisorSummary](t, f.request(http.MethodGet, path+"/supervisor/summary", nil, ""), 200)
	require.Equal(t, d.StatusDraft, summary.Status)
	require.Equal(t, int64(2), summary.TaskVersion)
	require.Equal(t, 1, summary.PlannedNodes)
	require.Empty(t, summary.Children)
	require.Empty(t, summary.ResultRefs)
	require.False(t, summary.ExecutionEnabled)
	counts := f.counts()
	require.Equal(t, int64(1), counts["task_control_plans"])
	for _, table := range []string{"task_control_children", "task_control_outbox", "lifecycle_runs"} {
		require.Zero(t, counts[table], table)
	}
}

func TestTaskSupervisorHTTPIntegrationRejectsEscalationAndRevocation(t *testing.T) {
	f := newSupervisorHTTPFixture(t)
	task := f.draft()
	path := "/" + task.ID
	before := f.counts()
	for _, identity := range []string{"missing", "tenant", "user"} {
		status := 404
		if identity == "missing" {
			status = 401
		}
		require.Equal(t, status, f.request(http.MethodPost, path+"/supervisor/propose", svc.ProposeInput{ExpectedVersion: task.Version}, identity).Code)
		require.Equal(t, status, f.request(http.MethodGet, path+"/supervisor/summary", nil, identity).Code)
	}
	require.Equal(t, 401, f.request(http.MethodPost, "/supervisor/assess", svc.AssessmentInput{}, "missing").Code)
	require.Zero(t, f.calls.Load())
	for _, tc := range []struct {
		name   string
		mutate func(*svc.PlanSpec)
		status int
		issue  string
	}{
		{"expanded budget", func(p *svc.PlanSpec) { p.Budget.MaxCalls++ }, 400, "task_budget_exceeded"},
		{"accessible source outside task", func(p *svc.PlanSpec) { p.Nodes[0].Inputs[0].Asset = &f.extra }, 400, "task_input_scope"},
		{"unregistered capability", func(p *svc.PlanSpec) { p.Nodes[0].Capability = "host.shell" }, 403, ""},
		{"unregistered tool", func(p *svc.PlanSpec) { p.Nodes[0].Tools = []string{"host.shell"} }, 400, ""},
		{"extra permission", func(p *svc.PlanSpec) { p.Nodes[0].Permissions = []string{"read", "write"} }, 400, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f.planner = func(context.Context, svc.PlanningInput) (svc.PlannerDecision, error) {
				plan := f.clonePlan()
				tc.mutate(&plan)
				return svc.PlannerDecision{Kind: "proposal", Explanation: "越界的合成测试候选。", Plan: &plan}, nil
			}
			w := f.request(http.MethodPost, path+"/supervisor/propose", svc.ProposeInput{ExpectedVersion: task.Version}, "")
			require.Equal(t, tc.status, w.Code, w.Body.String())
			if tc.issue != "" {
				require.Contains(t, w.Body.String(), tc.issue)
			}
			require.Equal(t, before, f.counts())
		})
	}
	f.planner = func(context.Context, svc.PlanningInput) (svc.PlannerDecision, error) {
		return svc.PlannerDecision{Kind: "clarify", Explanation: "需要确认。", Questions: []svc.SupervisorQuestion{{ID: "format", Question: "采用何种格式？"}}}, nil
	}
	require.Equal(t, 200, f.request(http.MethodPost, path+"/supervisor/propose", svc.ProposeInput{ExpectedVersion: task.Version}, "").Code)
	called := f.calls.Load()
	f.revoked.Store(true)
	require.Equal(t, 403, f.request(http.MethodPost, path+"/supervisor/propose", svc.ProposeInput{ExpectedVersion: task.Version}, "").Code)
	require.Equal(t, called, f.calls.Load(), "same idempotency key cannot bypass current input authorization")
	require.Equal(t, 403, f.request(http.MethodPost, "/supervisor/assess", svc.AssessmentInput{Goal: "清洗", InputRefs: []l.RefID{f.ref}, Budget: &f.budget}, "").Code)
	f.revoked.Store(false)
	f.planner = func(context.Context, svc.PlanningInput) (svc.PlannerDecision, error) {
		f.revoked.Store(true)
		return svc.PlannerDecision{Kind: "unsupported", Explanation: "当前不支持。"}, nil
	}
	require.Equal(t, 403, f.request(http.MethodPost, path+"/supervisor/propose", svc.ProposeInput{ExpectedVersion: task.Version}, "").Code)
	require.Equal(t, before, f.counts())
}

func TestTaskSupervisorHTTPIntegrationFailureAndConcurrentChange(t *testing.T) {
	f := newSupervisorHTTPFixture(t)
	task := f.draft()
	path := "/" + task.ID
	before := f.counts()
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{"unavailable", svc.ErrPlannerUnavailable, 503},
		{"malformed model output", svc.ErrPlannerOutput, 502},
		{"cancelled", context.Canceled, 504},
		{"timeout", context.DeadlineExceeded, 504},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f.planner = func(context.Context, svc.PlanningInput) (svc.PlannerDecision, error) {
				return svc.PlannerDecision{}, tc.err
			}
			require.Equal(t, tc.status, f.request(http.MethodPost, path+"/supervisor/propose", svc.ProposeInput{ExpectedVersion: task.Version}, "").Code)
			require.Equal(t, before, f.counts())
		})
	}
	f.planner = func(ctx context.Context, _ svc.PlanningInput) (svc.PlannerDecision, error) {
		_, err := f.repo.RequestCancel(ctx, f.owner, task.ID, task.Version)
		require.NoError(t, err)
		return svc.PlannerDecision{Kind: "unsupported", Explanation: "模型调用期间任务已取消。"}, nil
	}
	require.Equal(t, 409, f.request(http.MethodPost, path+"/supervisor/propose", svc.ProposeInput{ExpectedVersion: task.Version}, "").Code)
	stored, err := f.repo.GetTask(context.Background(), f.owner, task.ID)
	require.NoError(t, err)
	require.Equal(t, d.StatusCancelled, stored.Status)
	require.Equal(t, before, f.counts(), "only the explicit cancellation may alter task state; no proposal is saved")
}

func TestTaskSupervisorHTTPIntegrationSummaryPreservesUnknownAndCancelledReceipts(t *testing.T) {
	f := newSupervisorHTTPFixture(t)
	task := f.draft()
	path := "/" + task.ID
	supervisorDecode[svc.PlanResult](t, f.request(http.MethodPost, path+"/plans", svc.AppendPlanInput{ExpectedVersion: task.Version, Plan: f.plan}, ""), 201)
	ctx := context.Background()
	current, err := f.repo.GetTask(ctx, f.owner, task.ID)
	require.NoError(t, err)
	current, err = f.repo.UpdateTaskStatus(ctx, f.owner, task.ID, current.Version, d.TaskUpdate{Status: d.StatusReady})
	require.NoError(t, err)
	// Fixture-only insertion through the repository establishes real persisted
	// submission facts; it does not invoke a domain worker or claim domain success.
	child, err := f.repo.EnqueueChild(ctx, f.owner, task.ID, current.Version, d.DispatchInput{PlanVersion: 1, NodeID: "clean", Attempt: 1, Domain: "lifecycle", Payload: json.RawMessage(`{"synthetic":true}`)})
	require.NoError(t, err)
	page, err := f.repo.ListOutbox(ctx, f.owner, d.OutboxFilter{})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	outbox, err := f.repo.ClaimOutbox(ctx, f.owner, page.Items[0].ID, page.Items[0].Version)
	require.NoError(t, err)
	_, err = f.repo.MarkDispatchUnknown(ctx, f.owner, outbox.ID, outbox.Version, "synthetic submit result is unknown")
	require.NoError(t, err)
	summary := supervisorDecode[svc.SupervisorSummary](t, f.request(http.MethodGet, path+"/supervisor/summary", nil, ""), 200)
	require.Equal(t, d.StatusReconcileRequired, summary.Status)
	require.Len(t, summary.Children, 1)
	require.Equal(t, d.ChildReconcileRequired, summary.Children[0].DispatchStatus)
	require.Equal(t, "unknown", summary.Children[0].DomainStatus)
	require.False(t, summary.Children[0].ReceiptPresent)
	current, err = f.repo.GetTask(ctx, f.owner, task.ID)
	require.NoError(t, err)
	_, err = f.repo.RequestCancel(ctx, f.owner, task.ID, current.Version)
	require.NoError(t, err)
	child, err = f.repo.GetChild(ctx, f.owner, child.ID)
	require.NoError(t, err)
	_, err = f.repo.BindChild(ctx, f.owner, child.ID, child.Version, child.Attempt, d.Binding{DomainID: "synthetic-domain-run", Receipt: json.RawMessage(`{"submitted":true}`)})
	require.NoError(t, err)
	summary = supervisorDecode[svc.SupervisorSummary](t, f.request(http.MethodGet, path+"/supervisor/summary", nil, ""), 200)
	require.Equal(t, d.StatusCancelRequested, summary.Status)
	require.Equal(t, d.ChildBound, summary.Children[0].DispatchStatus)
	require.Equal(t, "synthetic-domain-run", summary.Children[0].DomainID)
	require.Equal(t, "unknown", summary.Children[0].DomainStatus)
	require.True(t, summary.Children[0].ReceiptPresent)
	require.Empty(t, summary.ResultRefs)
	require.False(t, summary.ExecutionEnabled)
	require.Zero(t, f.calls.Load(), "summary must never ask a model to invent execution facts")
	require.Zero(t, f.counts()["lifecycle_runs"])
}

// Explicit opt-in: this sends only generated reference IDs, a synthetic goal and
// the test capability contract to the configured provider. No source contents,
// credentials or model responses are printed. It still does not execute a plan.
func TestTaskSupervisorRealLLMHTTPIntegrationSyntheticProposal(t *testing.T) {
	if os.Getenv("SUPERVISOR_REAL_LLM") != "1" {
		t.Skip("set SUPERVISOR_REAL_LLM=1 and CHAT_* to exercise the configured real provider")
	}
	for _, key := range []string{"CHAT_PROVIDER", "CHAT_BASE_URL", "CHAT_MODEL_NAME", "CHAT_API_KEY"} {
		if os.Getenv(key) == "" {
			t.Fatalf("required live-model configuration %s is missing", key)
		}
	}
	client, err := llminfra.NewChatRepository(&llm.ModelConfig{Provider: llm.Provider(os.Getenv("CHAT_PROVIDER")), BaseURL: os.Getenv("CHAT_BASE_URL"), ModelName: os.Getenv("CHAT_MODEL_NAME"), APIKey: os.Getenv("CHAT_API_KEY")})
	if err != nil {
		t.Fatal("real model construction failed (configuration details suppressed)")
	}
	f := newSupervisorHTTPFixture(t)
	planner := svc.NewLLMPlanner(client)
	f.planner = planner.Propose
	task := f.draft()
	before := f.counts()
	proposal := supervisorDecode[svc.SupervisorProposal](t, f.request(http.MethodPost, "/"+task.ID+"/supervisor/propose", svc.ProposeInput{ExpectedVersion: task.Version}, ""), 200)
	require.Equal(t, "proposal", proposal.Decision.Kind, "unambiguous synthetic one-step goal must yield a validated candidate")
	require.NotNil(t, proposal.Decision.Plan)
	require.NotNil(t, proposal.Validation)
	require.True(t, proposal.Validation.Valid)
	require.Equal(t, 1, proposal.PlanningCalls)
	require.Equal(t, int64(1), f.calls.Load())
	require.False(t, proposal.Saved)
	require.False(t, proposal.ExecutionEnabled)
	require.Equal(t, before, f.counts())
	stored, err := f.repo.GetTask(context.Background(), f.owner, task.ID)
	require.NoError(t, err)
	wantJSON, err := json.Marshal(task)
	require.NoError(t, err)
	gotJSON, err := json.Marshal(stored)
	require.NoError(t, err)
	require.JSONEq(t, string(wantJSON), string(gotJSON))
	t.Log("real provider + HTTP + MySQL: one authorized, validated proposal; no execution or persistence side effects")
}
