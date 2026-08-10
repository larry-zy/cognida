package chat

import (
	"context"
	"strconv"
	"testing"

	"cognida/internal/model/llm"
	"cognida/internal/pkg/pagination"
)

// cursorModelRepo 内存版模型仓储，按 rows 顺序做索引式 keyset 翻页，仅用于验证游标分页〔M5〕。
type cursorModelRepo struct {
	llm.ModelRepository
	rows []*llm.ModelConfig
}

func (r *cursorModelRepo) ListByCursor(_ context.Context, _ int64, _ string, _ *bool, cursor string, limit int) ([]*llm.ModelConfig, string, error) {
	if limit <= 0 {
		limit = 20
	}
	start := 0
	if cur, err := pagination.Decode(cursor); err == nil && !cur.IsZero() {
		start, _ = strconv.Atoi(cur.Sort)
	}
	if start > len(r.rows) {
		start = len(r.rows)
	}
	end := start + limit
	next := ""
	if end < len(r.rows) {
		next = pagination.Cursor{Sort: strconv.Itoa(end), ID: r.rows[end-1].ID}.Encode()
	} else {
		end = len(r.rows)
	}
	return r.rows[start:end], next, nil
}

// TestListModelsByCursor 验证 ModelService 游标分页：首页返回 next_cursor/has_more，续页取回剩余并到末页。
func TestListModelsByCursor(t *testing.T) {
	repo := &cursorModelRepo{rows: []*llm.ModelConfig{
		{ID: 3, TenantID: 1, Name: "m3"},
		{ID: 2, TenantID: 1, Name: "m2"},
		{ID: 1, TenantID: 1, Name: "m1"},
	}}
	svc := NewModelService(repo, nil)

	page1, err := svc.ListModelsByCursor(context.Background(), 1, &ListModelsRequestDTO{PageSize: 2}, "")
	if err != nil {
		t.Fatalf("ListModelsByCursor(page1) error = %v", err)
	}
	if len(page1.Models) != 2 || !page1.HasMore || page1.NextCursor == "" {
		t.Fatalf("page1 = len %d has_more %v cursor %q, want 2/true/non-empty", len(page1.Models), page1.HasMore, page1.NextCursor)
	}

	page2, err := svc.ListModelsByCursor(context.Background(), 1, &ListModelsRequestDTO{PageSize: 2}, page1.NextCursor)
	if err != nil {
		t.Fatalf("ListModelsByCursor(page2) error = %v", err)
	}
	if len(page2.Models) != 1 || page2.HasMore || page2.NextCursor != "" {
		t.Fatalf("page2 = len %d has_more %v cursor %q, want 1/false/empty", len(page2.Models), page2.HasMore, page2.NextCursor)
	}
	if page2.Models[0].Name != "m1" {
		t.Errorf("page2 first = %q, want m1", page2.Models[0].Name)
	}
}
