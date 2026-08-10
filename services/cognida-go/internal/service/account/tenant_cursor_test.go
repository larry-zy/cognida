package account

import (
	"context"
	"strconv"
	"testing"

	"cognida/internal/model/tenant"
	"cognida/internal/pkg/pagination"
)

// cursorTenantRepo 内存版租户仓储，按 rows 顺序做索引式 keyset 翻页，仅用于验证游标分页〔M5〕。
// 其余接口方法经内嵌继承（未实现，调用即 panic）。
type cursorTenantRepo struct {
	tenant.TenantRepository
	rows []*tenant.Tenant
}

func (r *cursorTenantRepo) FindAllByCursor(_ context.Context, cursor string, limit int) ([]*tenant.Tenant, string, error) {
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

// TestListTenantsByCursor 验证 tenantService 游标分页：首页返回 next_cursor/has_more，续页取回剩余并到末页。
func TestListTenantsByCursor(t *testing.T) {
	repo := &cursorTenantRepo{rows: []*tenant.Tenant{
		{ID: 3, Name: "t3"},
		{ID: 2, Name: "t2"},
		{ID: 1, Name: "t1"},
	}}
	svc := NewTenantService(repo)

	page1, next1, err := svc.ListTenantsByCursor(context.Background(), "", 2)
	if err != nil {
		t.Fatalf("ListTenantsByCursor(page1) error = %v", err)
	}
	if len(page1) != 2 || next1 == "" {
		t.Fatalf("page1 = len %d cursor %q, want 2/non-empty", len(page1), next1)
	}

	page2, next2, err := svc.ListTenantsByCursor(context.Background(), next1, 2)
	if err != nil {
		t.Fatalf("ListTenantsByCursor(page2) error = %v", err)
	}
	if len(page2) != 1 || next2 != "" {
		t.Fatalf("page2 = len %d cursor %q, want 1/empty", len(page2), next2)
	}
	if page2[0].Name != "t1" {
		t.Errorf("page2 first = %q, want t1", page2[0].Name)
	}
}
