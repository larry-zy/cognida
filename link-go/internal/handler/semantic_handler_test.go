// Package handler 语义模型建模处理器 API 测试。
//
// 用真实 *app_semantic.Service（内存 fake 仓储支撑）驱动 HTTP 层，覆盖建模进料线的
// 增改查发布/弃用与覆盖率只读视图的状态码与响应壳约定（code/message/data）。
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	model "link/internal/model/semantic"
	app_semantic "link/internal/service/semantic"
)

// --- 内存测试替身（handler 包内自足，避免跨包引用 service 测试文件） ---------

type memSemanticRepo struct {
	bundles map[string]*model.ModelBundle
}

func newMemSemanticRepo() *memSemanticRepo {
	return &memSemanticRepo{bundles: map[string]*model.ModelBundle{}}
}

func (r *memSemanticRepo) GetActiveModel(_ context.Context, tenantID int64, name string) (*model.ModelBundle, error) {
	for _, b := range r.bundles {
		if b.Model.TenantID == tenantID && b.Model.Name == name && b.Model.Status == model.ModelStatusActive {
			return b, nil
		}
	}
	return nil, model.ErrModelNotFound
}

func (r *memSemanticRepo) ListActiveModels(_ context.Context, tenantID int64) ([]*model.SemanticModel, error) {
	out := make([]*model.SemanticModel, 0)
	for _, b := range r.bundles {
		if b.Model.TenantID == tenantID && b.Model.Status == model.ModelStatusActive {
			out = append(out, b.Model)
		}
	}
	return out, nil
}

func (r *memSemanticRepo) GetModelBundle(_ context.Context, modelID string) (*model.ModelBundle, error) {
	b, ok := r.bundles[modelID]
	if !ok {
		return nil, model.ErrModelNotFound
	}
	return b, nil
}

func (r *memSemanticRepo) UpsertBundle(_ context.Context, bundle *model.ModelBundle) error {
	r.bundles[bundle.Model.ID] = bundle
	return nil
}

func (r *memSemanticRepo) BumpVersion(_ context.Context, modelID string) (int, error) {
	b, ok := r.bundles[modelID]
	if !ok {
		return 0, model.ErrModelNotFound
	}
	b.Model.Version++
	return b.Model.Version, nil
}

type memIDGen struct{ n int }

func (g *memIDGen) Generate() string { g.n++; return "h-" + strconv.Itoa(g.n) }

type memCoverage struct{ stats []model.CoverageModelStat }

func (c *memCoverage) Record(context.Context, model.CoverageEvent) error { return nil }
func (c *memCoverage) Stats(context.Context, int64) ([]model.CoverageModelStat, error) {
	return c.stats, nil
}

// --- 测试装配 --------------------------------------------------------------

// setupSemanticRouter 装配一个语义建模子路由，并把 tenant_id 钉在 int64(1)（模拟鉴权中间件）。
func setupSemanticRouter(t *testing.T, cov model.CoverageReporter) (*gin.Engine, *memSemanticRepo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := newMemSemanticRepo()
	svc := app_semantic.NewService(repo, &memIDGen{}, cov, nil)
	h := NewSemanticHandler(svc)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", int64(1))
		c.Set("user_id", int64(10))
		c.Next()
	})
	sm := r.Group("/api/v1/semantic-models")
	{
		sm.GET("", h.List)
		sm.POST("", h.Create)
		sm.GET("/:id", h.Get)
		sm.PUT("/:id", h.Update)
		sm.POST("/:id/publish", h.Publish)
		sm.POST("/:id/deprecate", h.Deprecate)
	}
	r.GET("/api/v1/semantic-coverage", h.Coverage)
	return r, repo
}

func validBody(name string) []byte {
	body, _ := json.Marshal(map[string]any{
		"name": name,
		"logical_tables": []map[string]any{
			{"id": "lt_order", "name": "orders", "physical_table": "t_order"},
		},
		"dimensions": []map[string]any{
			{"logical_table_id": "lt_order", "name": "区域", "expr": "region"},
		},
		"metrics": []map[string]any{
			{"name": "营收", "expr": "SUM(orders.amount)"},
		},
	})
	return body
}

// envelope 解析响应壳，data 保留为原始 JSON 供逐 case 反序列化。
type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func do(t *testing.T, r *gin.Engine, method, path string, body []byte) (*httptest.ResponseRecorder, envelope) {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var env envelope
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	return w, env
}

// createModel 建一个模型并返回其 ID。
func createModel(t *testing.T, r *gin.Engine, name string) string {
	t.Helper()
	w, env := do(t, r, http.MethodPost, "/api/v1/semantic-models", validBody(name))
	assert.Equal(t, http.StatusOK, w.Code)
	var bundle model.ModelBundle
	if err := json.Unmarshal(env.Data, &bundle); err != nil {
		t.Fatalf("decode created bundle: %v", err)
	}
	assert.NotEmpty(t, bundle.Model.ID)
	return bundle.Model.ID
}

// --- 用例 ------------------------------------------------------------------

func TestAPI_CreateAndGet(t *testing.T) {
	r, _ := setupSemanticRouter(t, nil)
	id := createModel(t, r, "sales")

	w, env := do(t, r, http.MethodGet, "/api/v1/semantic-models/"+id, nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 0, env.Code)
	var got model.ModelBundle
	assert.NoError(t, json.Unmarshal(env.Data, &got))
	assert.Equal(t, "sales", got.Model.Name)
	assert.Equal(t, model.ModelStatusDraft, got.Model.Status)
}

func TestAPI_CreateValidationRejected(t *testing.T) {
	r, _ := setupSemanticRouter(t, nil)
	// 缺 name → 服务层 ErrValidation → 400。
	bad, _ := json.Marshal(map[string]any{
		"logical_tables": []map[string]any{{"id": "lt", "name": "o", "physical_table": "t"}},
	})
	w, _ := do(t, r, http.MethodPost, "/api/v1/semantic-models", bad)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAPI_GetNotFound(t *testing.T) {
	r, _ := setupSemanticRouter(t, nil)
	w, _ := do(t, r, http.MethodGet, "/api/v1/semantic-models/nope", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAPI_PublishThenListThenDeprecate(t *testing.T) {
	r, _ := setupSemanticRouter(t, nil)
	id := createModel(t, r, "sales")

	// 发布 → 进入生效列表。
	w, env := do(t, r, http.MethodPost, "/api/v1/semantic-models/"+id+"/publish", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	var published model.ModelBundle
	assert.NoError(t, json.Unmarshal(env.Data, &published))
	assert.Equal(t, model.ModelStatusActive, published.Model.Status)

	w, env = do(t, r, http.MethodGet, "/api/v1/semantic-models", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	var list struct {
		Items []*model.SemanticModel `json:"items"`
		Total int                    `json:"total"`
	}
	assert.NoError(t, json.Unmarshal(env.Data, &list))
	assert.Equal(t, 1, list.Total)

	// 弃用 → 退出生效列表。
	w, _ = do(t, r, http.MethodPost, "/api/v1/semantic-models/"+id+"/deprecate", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	_, env = do(t, r, http.MethodGet, "/api/v1/semantic-models", nil)
	assert.NoError(t, json.Unmarshal(env.Data, &list))
	assert.Equal(t, 0, list.Total)
}

func TestAPI_UpdateBumpsVersion(t *testing.T) {
	r, _ := setupSemanticRouter(t, nil)
	id := createModel(t, r, "sales")

	w, env := do(t, r, http.MethodPut, "/api/v1/semantic-models/"+id, validBody("sales-v2"))
	assert.Equal(t, http.StatusOK, w.Code)
	var updated model.ModelBundle
	assert.NoError(t, json.Unmarshal(env.Data, &updated))
	assert.Equal(t, "sales-v2", updated.Model.Name)
	assert.Equal(t, 2, updated.Model.Version)
}

func TestAPI_PublishNameConflict(t *testing.T) {
	r, _ := setupSemanticRouter(t, nil)
	// 两个 draft 同名；发布第二个应因命名冲突 → 400。
	first := createModel(t, r, "sales")
	second := createModel(t, r, "sales")
	w, _ := do(t, r, http.MethodPost, "/api/v1/semantic-models/"+first+"/publish", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	w, _ = do(t, r, http.MethodPost, "/api/v1/semantic-models/"+second+"/publish", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAPI_CoverageDisabled(t *testing.T) {
	r, _ := setupSemanticRouter(t, nil) // 未注入 reporter
	w, env := do(t, r, http.MethodGet, "/api/v1/semantic-coverage", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	var payload struct {
		Enabled bool `json:"enabled"`
	}
	assert.NoError(t, json.Unmarshal(env.Data, &payload))
	assert.False(t, payload.Enabled)
}

func TestAPI_CoverageEnabled(t *testing.T) {
	cov := &memCoverage{stats: []model.CoverageModelStat{
		{Model: "sales", Covered: 8, CacheHit: 0, Fallback: 2, Total: 10, HitRatio: 0.8},
	}}
	r, _ := setupSemanticRouter(t, cov)
	w, env := do(t, r, http.MethodGet, "/api/v1/semantic-coverage", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	var payload struct {
		Enabled bool                       `json:"enabled"`
		Total   int                        `json:"total"`
		Items   []model.CoverageModelStat `json:"items"`
	}
	assert.NoError(t, json.Unmarshal(env.Data, &payload))
	assert.True(t, payload.Enabled)
	assert.Equal(t, 1, payload.Total)
	assert.Equal(t, "sales", payload.Items[0].Model)
	assert.InDelta(t, 0.8, payload.Items[0].HitRatio, 0.001)
}
