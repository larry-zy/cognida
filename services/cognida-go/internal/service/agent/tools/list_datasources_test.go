// Package tools 测试 list_datasources 选库工具。
package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	agentctx "cognida/internal/model/agent"
	model_datasource "cognida/internal/model/datasource"
)

// fakeDatasourceLister 测试用数据源仓储：只实现 List，其余方法非本测试关注，返回零值。
type fakeDatasourceLister struct {
	items []*model_datasource.DataSource
	total int64
	err   error
	// gotFilter 记录最近一次 List 的过滤条件，供断言租户/分页透传。
	gotFilter model_datasource.ListFilter
}

func (f *fakeDatasourceLister) List(_ context.Context, filter model_datasource.ListFilter) ([]*model_datasource.DataSource, int64, error) {
	f.gotFilter = filter
	if f.err != nil {
		return nil, 0, f.err
	}
	return f.items, f.total, nil
}

func (f *fakeDatasourceLister) Create(context.Context, *model_datasource.DataSource) error { return nil }
func (f *fakeDatasourceLister) Get(context.Context, string, int64) (*model_datasource.DataSource, error) {
	return nil, nil
}
func (f *fakeDatasourceLister) GetByName(context.Context, string, int64) (*model_datasource.DataSource, error) {
	return nil, nil
}
func (f *fakeDatasourceLister) ListAll(context.Context) ([]*model_datasource.DataSource, error) {
	return nil, nil
}
func (f *fakeDatasourceLister) Update(context.Context, *model_datasource.DataSource) error { return nil }
func (f *fakeDatasourceLister) UpdateStatus(context.Context, string, int64, string, time.Time) error {
	return nil
}
func (f *fakeDatasourceLister) Delete(context.Context, string, int64) error { return nil }

// TestListDatasourcesNilLister lister 为 nil → 返回未启用提示且空清单，不报错。
func TestListDatasourcesNilLister(t *testing.T) {
	res, err := listDatasources(context.Background(), nil)
	if err != nil {
		t.Fatalf("nil lister 不应报错: %v", err)
	}
	if len(res.Datasources) != 0 {
		t.Fatalf("nil lister 应返回空清单, got %d", len(res.Datasources))
	}
	if !strings.Contains(res.Note, "未启用") {
		t.Fatalf("nil lister 应提示未启用, got %q", res.Note)
	}
}

// TestListDatasourcesReturnsCardsWithDescription 正常列出：回显描述、透传租户、无锁定时给选库指引。
func TestListDatasourcesReturnsCardsWithDescription(t *testing.T) {
	lister := &fakeDatasourceLister{
		items: []*model_datasource.DataSource{
			{ID: "ds-ecom", Name: "电商库", Description: "电商订单、商品、用户", Type: model_datasource.TypeMySQL, DatabaseName: "ecommerce", Status: model_datasource.StatusActive},
			{ID: "ds-fin", Name: "财务库", Description: "财务凭证与账目", Type: model_datasource.TypeMySQL, DatabaseName: "finance", Status: model_datasource.StatusActive},
		},
		total: 2,
	}
	ctx := agentctx.WithTenantID(context.Background(), 42)
	res, err := listDatasources(ctx, lister)
	if err != nil {
		t.Fatalf("列出数据源失败: %v", err)
	}
	if lister.gotFilter.TenantID != 42 {
		t.Fatalf("应按会话租户过滤, got tenant %d", lister.gotFilter.TenantID)
	}
	if len(res.Datasources) != 2 {
		t.Fatalf("应返回 2 个数据源, got %d", len(res.Datasources))
	}
	if res.Datasources[0].Description != "电商订单、商品、用户" {
		t.Fatalf("应回显业务描述, got %q", res.Datasources[0].Description)
	}
	for _, c := range res.Datasources {
		if c.Selected {
			t.Fatalf("无会话锁定时不应有 selected 项, got %+v", c)
		}
	}
	if !strings.Contains(res.Note, "description") {
		t.Fatalf("无锁定时应给出据描述选库的指引, got %q", res.Note)
	}
}

// TestListDatasourcesMarksSessionLocked 会话已手动选库 → 对应项 selected=true，且指引提示直接查询。
func TestListDatasourcesMarksSessionLocked(t *testing.T) {
	lister := &fakeDatasourceLister{
		items: []*model_datasource.DataSource{
			{ID: "ds-ecom", Name: "电商库", Type: model_datasource.TypeMySQL, DatabaseName: "ecommerce", Status: model_datasource.StatusActive},
			{ID: "ds-fin", Name: "财务库", Type: model_datasource.TypeMySQL, DatabaseName: "finance", Status: model_datasource.StatusActive},
		},
		total: 2,
	}
	ctx := agentctx.WithDatasourceID(agentctx.WithTenantID(context.Background(), 42), "ds-fin")
	res, err := listDatasources(ctx, lister)
	if err != nil {
		t.Fatalf("列出数据源失败: %v", err)
	}
	var lockedSelected, otherSelected bool
	for _, c := range res.Datasources {
		if c.ID == "ds-fin" {
			lockedSelected = c.Selected
		} else if c.Selected {
			otherSelected = true
		}
	}
	if !lockedSelected {
		t.Fatal("会话锁定的 ds-fin 应标记 selected=true")
	}
	if otherSelected {
		t.Fatal("非锁定库不应标记 selected")
	}
	// 指引应点名锁定库（财务库）并提示直接查询、勿传 database_id 覆盖。
	if !strings.Contains(res.Note, "手动锁定") || !strings.Contains(res.Note, "财务库") {
		t.Fatalf("锁定时指引应点名锁定库并提示直接查询, got %q", res.Note)
	}
}
