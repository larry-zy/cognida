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
	l "cognida/internal/model/lifecycle"
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

// This is a real MySQL + HTTP contract test with a fixed authorization resolver.
// It is deliberately not an end-to-end source acquisition or execution test.
func TestTaskControlHTTPIntegrationDraftPlansIsolationAndRevocation(t *testing.T) {
	dsn := os.Getenv("COGNIDA_TASKCONTROL_TEST_DSN")
	require.NotEmpty(t, dsn, "COGNIDA_TASKCONTROL_TEST_DSN must name an isolated review database")
	cfg, err := driver.ParseDSN(dsn)
	if err != nil {
		t.Fatal("invalid task-control test DSN")
	}
	require.True(t, strings.HasPrefix(cfg.DBName, "cognida_review_"), "isolated review database required")
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	var version int64
	var dirty bool
	require.NoError(t, sqlDB.QueryRow("SELECT version, dirty FROM schema_migrations LIMIT 1").Scan(&version, &dirty))
	require.GreaterOrEqual(t, version, int64(13))
	require.False(t, dirty)
	owner := d.Scope{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 931}
	t.Cleanup(func() {
		for _, table := range []string{"task_control_outbox", "task_control_children", "task_control_plans", "task_control_tasks"} {
			require.NoError(t, db.Table(table).Where("tenant_id = ?", owner.TenantID).Delete(map[string]any{}).Error)
		}
	})
	ref := l.RefID{AssetID: uuid.NewString(), Version: 1}
	var revoked atomic.Bool
	var resolveCalls atomic.Int64
	capabilities := []svc.Capability{{ID: "lifecycle.clean", Version: 1, Domain: "lifecycle", InputKinds: []l.Kind{l.RawAsset}, OutputKinds: []l.Kind{l.CleanedAsset}, Tools: []string{"clean"}, Permissions: []string{"read"}}}
	validator, err := svc.NewValidator(capabilities, func(_ context.Context, p d.Scope, permission string) error {
		if p.Validate() != nil || permission != "read" {
			return d.ErrForbidden
		}
		return nil
	}, func(_ context.Context, p d.Scope, r l.RefID) (l.Kind, error) {
		resolveCalls.Add(1)
		if revoked.Load() || p != owner || r != ref {
			return "", d.ErrForbidden
		}
		return l.RawAsset, nil
	})
	require.NoError(t, err)
	service, err := svc.NewService(repository.NewTaskControlRepository(db), validator)
	require.NoError(t, err)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-Identity") == "missing" {
			return
		}
		scope := owner
		if c.GetHeader("X-Test-Identity") == "tenant" {
			scope.TenantID++
		}
		if c.GetHeader("X-Test-Identity") == "user" {
			scope.UserID++
		}
		c.Set("tenant_id", scope.TenantID)
		c.Set("user_id", scope.UserID)
		c.Set("request_id", uuid.NewString())
	})
	router.Use(middleware.NewIdempotency(nil).Apply())
	NewTaskControlHandler(service).Register(router.Group("/api/v1"))
	request := func(method, path string, body any, key, identity string) *httptest.ResponseRecorder {
		var data []byte
		if body != nil {
			var e error
			data, e = json.Marshal(body)
			require.NoError(t, e)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		req.Header.Set("X-Test-Identity", identity)
		result := httptest.NewRecorder()
		router.ServeHTTP(result, req)
		return result
	}
	const base = "/api/v1/governed-tasks"
	budget := svc.Budget{MaxNodes: 2, MaxConcurrency: 1, MaxAttempts: 2, MaxCalls: 4, MaxCostMicros: 100, TimeoutSeconds: 60}
	plan := svc.PlanSpec{SchemaVersion: 1, Budget: budget, Nodes: []svc.Node{{ID: "clean", Capability: "lifecycle.clean", CapabilityVersion: 1, Inputs: []svc.InputBinding{{Asset: &ref, Kind: l.RawAsset}}, OutputKinds: []l.Kind{l.CleanedAsset}, Tools: []string{"clean"}, Permissions: []string{"read"}, MaxAttempts: 1, MaxCallsPerAttempt: 1, MaxCostMicrosPerAttempt: 10, TimeoutSeconds: 10}}}
	draft := svc.DraftInput{Goal: "生成可审阅清洗计划", InputRefs: []l.RefID{ref}, Budget: budget}
	w := request(http.MethodGet, base+"/capabilities", nil, "", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"execution_enabled":false`)
	w = request(http.MethodPost, base+"/validate", plan, "preflight-key", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"execution_enabled":false`)
	require.Contains(t, w.Body.String(), `"valid":true`)
	w = request(http.MethodPost, base, draft, "draft-key", "")
	require.Equal(t, 201, w.Code, w.Body.String())
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	var created struct {
		Data d.Task `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.NotEmpty(t, created.Data.ID)
	require.Equal(t, d.StatusDraft, created.Data.Status)
	w = request(http.MethodPost, base, draft, "draft-key", "")
	require.Equal(t, 201, w.Code, w.Body.String())
	var replay struct {
		Data d.Task `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &replay))
	require.Equal(t, created.Data.ID, replay.Data.ID)
	require.Empty(t, w.Header().Get("Idempotency-Replayed"))
	changed := draft
	changed.Goal = "不同目标"
	w = request(http.MethodPost, base, changed, "draft-key", "")
	require.Equal(t, 409, w.Code, w.Body.String())
	path := base + "/" + created.Data.ID
	appendInput := svc.AppendPlanInput{ExpectedVersion: created.Data.Version, Plan: plan}
	w = request(http.MethodPost, path+"/plans", appendInput, "plan-key", "")
	require.Equal(t, 201, w.Code, w.Body.String())
	var firstPlan struct {
		Data svc.PlanResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &firstPlan))
	require.Equal(t, int64(1), firstPlan.Data.Plan.Version)
	w = request(http.MethodPost, path+"/plans", appendInput, "plan-key", "")
	require.Equal(t, 409, w.Code, w.Body.String())
	require.Empty(t, w.Header().Get("Idempotency-Replayed"))
	w = request(http.MethodGet, path, nil, "", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	var current struct {
		Data d.Task `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &current))
	require.Equal(t, int64(2), current.Data.Version)
	require.Equal(t, d.StatusDraft, current.Data.Status)
	appendInput.ExpectedVersion = current.Data.Version
	appendInput.Plan.Nodes = append([]svc.Node(nil), plan.Nodes...)
	appendInput.Plan.Nodes[0].TimeoutSeconds = 11
	w = request(http.MethodPost, path+"/plans", appendInput, "plan-key", "")
	require.Equal(t, 201, w.Code, w.Body.String())
	w = request(http.MethodGet, path+"/plans/1", nil, "", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	var saved struct {
		Data d.Plan `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &saved))
	require.Equal(t, firstPlan.Data.Plan.ContentHash, saved.Data.ContentHash)
	w = request(http.MethodGet, path+"/plans/2", nil, "", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	w = request(http.MethodGet, base+"?page_size=1", nil, "", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), created.Data.ID)
	for _, identity := range []string{"tenant", "user"} {
		for _, route := range []string{path, path + "/plans/1"} {
			w = request(http.MethodGet, route, nil, "", identity)
			require.Equal(t, 404, w.Code, w.Body.String())
		}
		w = request(http.MethodGet, base, nil, "", identity)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.NotContains(t, w.Body.String(), created.Data.ID)
		w = request(http.MethodPost, path+"/plans", appendInput, "foreign-plan-key", identity)
		require.Equal(t, 404, w.Code, w.Body.String())
		w = request(http.MethodPost, base, draft, "foreign-draft-key", identity)
		require.Equal(t, 403, w.Code, w.Body.String())
	}
	w = request(http.MethodGet, base+"/capabilities", nil, "", "missing")
	require.Equal(t, 401, w.Code, w.Body.String())
	w = request(http.MethodPost, base, draft, "missing-identity", "missing")
	require.Equal(t, 401, w.Code, w.Body.String())
	// A previously successful preflight must reauthorize even with the exact same
	// Idempotency-Key and body. The generic response cache must not grant access.
	callsBefore := resolveCalls.Load()
	revoked.Store(true)
	w = request(http.MethodPost, base+"/validate", plan, "preflight-key", "")
	require.Equal(t, 403, w.Code, w.Body.String())
	require.Empty(t, w.Header().Get("Idempotency-Replayed"))
	require.Greater(t, resolveCalls.Load(), callsBefore)
	w = request(http.MethodPost, base, draft, "draft-key", "")
	require.Equal(t, 403, w.Code, w.Body.String())
	for _, table := range []string{"task_control_children", "task_control_outbox", "lifecycle_runs", "lifecycle_stages"} {
		var count int64
		require.NoError(t, db.Table(table).Where("tenant_id = ?", owner.TenantID).Count(&count).Error)
		require.Zero(t, count, "preflight/draft unexpectedly wrote %s", table)
	}
	var count int64
	require.NoError(t, db.Table("task_control_tasks").Where("tenant_id = ?", owner.TenantID).Count(&count).Error)
	require.Equal(t, int64(1), count)
}
