package tools

import (
	"context"
	"testing"
	"time"

	"link/internal/model/dataprofile"
	model_datasource "link/internal/model/datasource"

	"gorm.io/gorm"
)

// staleProfilesFor 造一批「已过期」的列画像（触发后台刷新用）。
func staleProfilesFor(cols ...string) []*dataprofile.ColumnProfile {
	old := time.Now().Add(-profileTTL - time.Hour)
	out := make([]*dataprofile.ColumnProfile, 0, len(cols))
	for _, c := range cols {
		out = append(out, &dataprofile.ColumnProfile{
			ColumnName: c, ProfiledAt: old, RowCount: 100, Distinct: 3,
			TopValues: []dataprofile.ValueFrequency{{Value: "completed", Count: 55}},
		})
	}
	return out
}

func tablesWithStatus(names ...string) []TableSchema {
	out := make([]TableSchema, 0, len(names))
	for _, n := range names {
		out = append(out, TableSchema{
			TableName: n,
			Columns:   []ColumnSchema{{Name: "id"}, {Name: "status"}},
		})
	}
	return out
}

// withStubbedRefresh 临时替换后台刷新触发器，返回计数指针与还原函数。
func withStubbedRefresh(t *testing.T) (*int, func()) {
	t.Helper()
	prev := triggerProfileRefresh
	count := 0
	triggerProfileRefresh = func(_ dataprofile.Store, _ *gorm.DB, _ model_datasource.ConnectionProvider, _ int64, _, _ string, _ TableSchema) {
		count++
	}
	return &count, func() { triggerProfileRefresh = prev }
}

// TestBatch_AttachesCachedFactsToAllCandidates 关键词候选表全部挂上缓存数据事实——
// 这是本修复的核心：候选表此前只回结构无事实，Agent 据此写 SQL 会猜枚举值。
func TestBatch_AttachesCachedFactsToAllCandidates(t *testing.T) {
	_, restore := withStubbedRefresh(t) // 缓存新鲜，不应触发；桩住以防真起 goroutine
	defer restore()

	fresh := []*dataprofile.ColumnProfile{
		{ColumnName: "status", ProfiledAt: time.Now(), RowCount: 100, Distinct: 3,
			TopValues: []dataprofile.ValueFrequency{{Value: "completed", Count: 55}}},
	}
	store := &fakeProfileStore{profiles: fresh}
	tables := tablesWithStatus("orders", "customers", "items")

	attachAndRefreshProfilesBatch(ctxWithIdentity(7, "ds-1"), store, nil, nil,
		&queryTarget{dbName: "shop"}, "", tables)

	for _, tb := range tables {
		var statusCol *ColumnSchema
		for i := range tb.Columns {
			if tb.Columns[i].Name == "status" {
				statusCol = &tb.Columns[i]
			}
		}
		if statusCol == nil || statusCol.Facts == nil {
			t.Fatalf("候选表 %s 的 status 列未挂上数据事实", tb.TableName)
		}
		if statusCol.Facts.Distinct != 3 || len(statusCol.Facts.TopValues) != 1 {
			t.Fatalf("候选表 %s 事实内容异常: %+v", tb.TableName, statusCol.Facts)
		}
	}
}

// TestBatch_CapsBackgroundRefresh 全部候选表过期时，后台刷新触发被限流到 maxProfileRefreshPerCall，
// 避免冷库首次选表对十余张表同时全表扫描（扫描风暴）；但缓存事实仍挂给全部候选表。
func TestBatch_CapsBackgroundRefresh(t *testing.T) {
	count, restore := withStubbedRefresh(t)
	defer restore()

	store := &fakeProfileStore{profiles: staleProfilesFor("status")}

	// 造 maxProfileRefreshPerCall+3 张全过期候选表。
	names := make([]string, 0, maxProfileRefreshPerCall+3)
	for i := 0; i < maxProfileRefreshPerCall+3; i++ {
		names = append(names, "t"+string(rune('a'+i)))
	}
	tables := tablesWithStatus(names...)

	attachAndRefreshProfilesBatch(ctxWithIdentity(7, "ds-1"), store, nil, nil,
		&queryTarget{dbName: "shop"}, "", tables)

	if *count != maxProfileRefreshPerCall {
		t.Fatalf("后台刷新应限流到 %d, got %d", maxProfileRefreshPerCall, *count)
	}
	// 限流只约束刷新触发，缓存事实（含过期快照）仍挂给全部候选表。
	for _, tb := range tables {
		if tb.Columns[1].Facts == nil || !tb.Columns[1].Facts.Stale {
			t.Fatalf("候选表 %s 应挂上过期(stale)快照, got %+v", tb.TableName, tb.Columns[1].Facts)
		}
	}
}

// TestBatch_FreshCacheNoRefresh 缓存新鲜时不触发任何后台刷新。
func TestBatch_FreshCacheNoRefresh(t *testing.T) {
	count, restore := withStubbedRefresh(t)
	defer restore()

	store := &fakeProfileStore{profiles: []*dataprofile.ColumnProfile{
		{ColumnName: "status", ProfiledAt: time.Now(), Distinct: 3},
	}}
	attachAndRefreshProfilesBatch(ctxWithIdentity(7, "ds-1"), store, nil, nil,
		&queryTarget{dbName: "shop"}, "", tablesWithStatus("orders", "customers"))

	if *count != 0 {
		t.Fatalf("缓存新鲜不应触发后台刷新, got %d", *count)
	}
}

// TestBatch_NilStoreNoop store 未接线时静默跳过——零回归。
func TestBatch_NilStoreNoop(t *testing.T) {
	count, restore := withStubbedRefresh(t)
	defer restore()

	tables := tablesWithStatus("orders")
	attachAndRefreshProfilesBatch(context.Background(), nil, nil, nil,
		&queryTarget{dbName: "shop"}, "", tables)

	if *count != 0 {
		t.Fatalf("store 为 nil 不应触发刷新, got %d", *count)
	}
	if tables[0].Columns[1].Facts != nil {
		t.Fatal("store 为 nil 不应挂载 Facts")
	}
}
