package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"cognida/internal/model/trace"
)

type listTraceFakeRepo struct {
	list  []*trace.TraceSummary
	total int64
	err   error
}

func (f *listTraceFakeRepo) SaveBatch(context.Context, []*trace.Span) error { return nil }
func (f *listTraceFakeRepo) ListTraces(context.Context, *trace.Query) ([]*trace.TraceSummary, int64, error) {
	return f.list, f.total, f.err
}
func (f *listTraceFakeRepo) GetSpansByTraceID(context.Context, string) ([]*trace.Span, error) {
	return nil, nil
}
func (f *listTraceFakeRepo) HasRequestIDs(context.Context, int64, []string) (map[string]bool, error) {
	return map[string]bool{}, nil
}

func TestListTraces_RequestIDWithoutSpans_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewTraceHandler(&listTraceFakeRepo{list: nil, total: 0})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/traces?request_id=no-span", nil)

	h.ListTraces(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	var body Response
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Message == "" {
		t.Fatal("404 应带回说明文案")
	}
}

func TestListTraces_NoFilterEmpty_OK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewTraceHandler(&listTraceFakeRepo{list: nil, total: 0})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)

	h.ListTraces(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}
