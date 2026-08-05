//go:build integration
// +build integration

// Package mysql: 列画像仓储（column_profiles）真实 DB 集成测试。
//
//	MYSQL_DSN='root:password@tcp(localhost:3306)/link?charset=utf8mb4&parseTime=True&loc=Local' \
//	  go test -tags=integration ./internal/repository/mysql/ -run TestColumnProfileRepo -v
//
// 覆盖：Upsert 首写、覆盖式 upsert（非累积、同坐标更新快照）、ListByTable 读回与
// top_values JSON 往返、物理坐标隔离（租户/数据源/库/表互不覆盖）。
package mysql

import (
	"context"
	"testing"
	"time"

	"link/internal/model/dataprofile"
)

func TestColumnProfileRepoUpsertAndList(t *testing.T) {
	db := newIntegrationDB(t)
	if err := db.AutoMigrate(&ColumnProfileModel{}); err != nil {
		t.Fatalf("automigrate column_profiles: %v", err)
	}
	repo := NewColumnProfileRepository(db)
	ctx := context.Background()

	const (
		tenant = int64(9901)
		ds     = ""
		schema = "itest_db"
		table  = "itest_orders"
	)
	// 清理历史残留，保证可重复运行。
	db.Where("tenant_id = ? AND schema_name = ?", tenant, schema).Delete(&ColumnProfileModel{})

	now := time.Now().Truncate(time.Second)
	first := []*dataprofile.ColumnProfile{
		{TenantID: tenant, DatasourceID: ds, SchemaName: schema, TableName: table,
			ColumnName: "status", RowCount: 100, NullCount: 5, NullRate: 0.05, Distinct: 3,
			TopValues: []dataprofile.ValueFrequency{
				{Value: "completed", Count: 55}, {Value: "shipped", Count: 40},
			}, ProfiledAt: now},
		{TenantID: tenant, DatasourceID: ds, SchemaName: schema, TableName: table,
			ColumnName: "id", RowCount: 100, NullCount: 0, NullRate: 0, Distinct: 100, ProfiledAt: now},
	}
	if err := repo.Upsert(ctx, first); err != nil {
		t.Fatalf("首次 Upsert: %v", err)
	}

	got, err := repo.ListByTable(ctx, tenant, ds, schema, table)
	if err != nil {
		t.Fatalf("ListByTable: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("期望 2 列画像，实得 %d", len(got))
	}
	byCol := map[string]*dataprofile.ColumnProfile{}
	for _, p := range got {
		byCol[p.ColumnName] = p
	}
	st := byCol["status"]
	if st == nil || st.Distinct != 3 || st.NullRate != 0.05 || len(st.TopValues) != 2 {
		t.Fatalf("status 画像读回不符: %+v", st)
	}
	if st.TopValues[0].Value != "completed" || st.TopValues[0].Count != 55 {
		t.Errorf("top_values JSON 往返错误: %+v", st.TopValues)
	}

	// 覆盖式 upsert：同坐标再写应更新快照且不新增行（非累积）。
	updated := []*dataprofile.ColumnProfile{
		{TenantID: tenant, DatasourceID: ds, SchemaName: schema, TableName: table,
			ColumnName: "status", RowCount: 200, NullCount: 0, NullRate: 0, Distinct: 4,
			TopValues: []dataprofile.ValueFrequency{{Value: "paid", Count: 80}}, ProfiledAt: now.Add(time.Hour)},
	}
	if err := repo.Upsert(ctx, updated); err != nil {
		t.Fatalf("覆盖 Upsert: %v", err)
	}
	got2, _ := repo.ListByTable(ctx, tenant, ds, schema, table)
	if len(got2) != 2 {
		t.Fatalf("覆盖后仍应为 2 行（非累积），实得 %d", len(got2))
	}
	for _, p := range got2 {
		if p.ColumnName == "status" {
			if p.Distinct != 4 || p.RowCount != 200 || len(p.TopValues) != 1 || p.TopValues[0].Value != "paid" {
				t.Errorf("status 快照未被覆盖更新: %+v", p)
			}
		}
	}
}

func TestColumnProfileRepoCoordinateIsolation(t *testing.T) {
	db := newIntegrationDB(t)
	if err := db.AutoMigrate(&ColumnProfileModel{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	repo := NewColumnProfileRepository(db)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	mk := func(tenant int64, ds, schema, table string) *dataprofile.ColumnProfile {
		return &dataprofile.ColumnProfile{TenantID: tenant, DatasourceID: ds, SchemaName: schema,
			TableName: table, ColumnName: "c", RowCount: 1, Distinct: 1, ProfiledAt: now}
	}
	db.Where("column_name = ? AND schema_name IN ?", "c", []string{"iso_a", "iso_b"}).Delete(&ColumnProfileModel{})

	// 同表名但坐标不同（租户/数据源/库各异），互不覆盖，各成一行。
	all := []*dataprofile.ColumnProfile{
		mk(1, "", "iso_a", "t"),
		mk(2, "", "iso_a", "t"),   // 不同租户
		mk(1, "ext", "iso_a", "t"), // 不同数据源
		mk(1, "", "iso_b", "t"),   // 不同库
	}
	if err := repo.Upsert(ctx, all); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// 只应读回租户1、当前库、iso_a 那一条。
	got, err := repo.ListByTable(ctx, 1, "", "iso_a", "t")
	if err != nil {
		t.Fatalf("ListByTable: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("坐标隔离失败：期望 1 行，实得 %d", len(got))
	}
	if got[0].TenantID != 1 || got[0].DatasourceID != "" || got[0].SchemaName != "iso_a" {
		t.Errorf("读回了错误坐标的画像: %+v", got[0])
	}
}
