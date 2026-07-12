// Package handler 评测处理器测试 - 可用指标目录（注册表驱动）
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"link/internal/service/evaluation"
)

// fakeGraderCatalog 记录收到的 eval_type 并返回预置目录，验证 Go 侧归一化与代理。
type fakeGraderCatalog struct {
	gotEvalType string
	resp        *evaluation.GraderCatalog
	err         error
}

func (f *fakeGraderCatalog) ListGraders(_ context.Context, evalType string) (*evaluation.GraderCatalog, error) {
	f.gotEvalType = evalType
	return f.resp, f.err
}

func newGradersRouter(catalog evaluation.GraderCatalogClient) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := evaluation.NewService(nil, nil, nil, nil, nil, catalog)
	h := NewEvaluationHandler(svc, nil, nil, nil)
	r := gin.New()
	r.GET("/api/v1/evaluation/graders", h.ListGraders)
	return r
}

// TestListGraders_FilterByType 按 eval_type 过滤返回目录，契约字段齐全。
func TestListGraders_FilterByType(t *testing.T) {
	fake := &fakeGraderCatalog{
		resp: &evaluation.GraderCatalog{
			EvalType: "rag",
			Graders: []*evaluation.GraderMeta{
				{
					Name:             "faithfulness",
					Label:            "忠实度",
					Group:            "rag",
					EvalTypes:        []string{"rag"},
					RequiresContexts: true,
				},
			},
		},
	}
	r := newGradersRouter(fake)

	req := httptest.NewRequest("GET", "/api/v1/evaluation/graders?eval_type=rag", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "rag", fake.gotEvalType)

	var resp struct {
		Code int                        `json:"code"`
		Data *evaluation.GraderCatalog `json:"data"`
	}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.Code)
	assert.Equal(t, "rag", resp.Data.EvalType)
	assert.Len(t, resp.Data.Graders, 1)
	g := resp.Data.Graders[0]
	assert.Equal(t, "faithfulness", g.Name)
	assert.Equal(t, "忠实度", g.Label)
	assert.Equal(t, "rag", g.Group)
	assert.Equal(t, []string{"rag"}, g.EvalTypes)
	assert.True(t, g.RequiresContexts)
}

// TestListGraders_QAAliasNormalized qa 在 Go 侧归一化为 llm 再代理给注册表。
func TestListGraders_QAAliasNormalized(t *testing.T) {
	fake := &fakeGraderCatalog{resp: &evaluation.GraderCatalog{EvalType: "llm", Graders: nil}}
	r := newGradersRouter(fake)

	req := httptest.NewRequest("GET", "/api/v1/evaluation/graders?eval_type=qa", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "llm", fake.gotEvalType, "qa 应归一化为 llm 再代理")
}

// TestListGraders_MissingType 缺 eval_type 返回 400，不代理。
func TestListGraders_MissingType(t *testing.T) {
	fake := &fakeGraderCatalog{}
	r := newGradersRouter(fake)

	req := httptest.NewRequest("GET", "/api/v1/evaluation/graders", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Empty(t, fake.gotEvalType, "缺类型不应代理到注册表")
}

// TestListGraders_UnknownType 未知 eval_type 返回 400，不做静默全量回退。
func TestListGraders_UnknownType(t *testing.T) {
	fake := &fakeGraderCatalog{}
	r := newGradersRouter(fake)

	req := httptest.NewRequest("GET", "/api/v1/evaluation/graders?eval_type=nope", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Empty(t, fake.gotEvalType, "未知类型应在 Go 侧被拒，不代理")
}
