//go:build integration
// +build integration

// Package mysql: 列画像仓储（column_profiles）真实 DB 集成测试。
//
//	MYSQL_DSN='root:password@tcp(localhost:3306)/cognida?charset=utf8mb4&parseTime=True&loc=Local' \
//	  go test -tags=integration ./internal/repository/mysql/ -run TestColumnProfileRepo -v
//
// 覆盖：Upsert 首写、覆盖式 upsert（非累积、同坐标更新快照）、ListByTable 读回与
// top_values JSON 往返、物理坐标隔离（租户/数据源/库/表互不覆盖）。
package mysql

import (
	"context"
	"testing"
	"time"

	"cognida/internal/model/dataprofile"
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

// TestColumnProfileRepoListByDatasourceTable 验证跨 schema 的「按数据源+表」检索：
// 语义层不知具体库名，故同一数据源下同名表的多库画像应一并返回；数据源隔离仍生效。
func TestColumnProfileRepoListByDatasourceTable(t *testing.T) {
	db := newIntegrationDB(t)
	if err := db.AutoMigrate(&ColumnProfileModel{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	repo := NewColumnProfileRepository(db)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	const tenant = int64(9902)
	mk := func(ds, schema, col string) *dataprofile.ColumnProfile {
		return &dataprofile.ColumnProfile{TenantID: tenant, DatasourceID: ds, SchemaName: schema,
			TableName: "dst_orders", ColumnName: col, RowCount: 1, Distinct: 1, ProfiledAt: now}
	}
	db.Where("tenant_id = ? AND table_name = ?", tenant, "dst_orders").Delete(&ColumnProfileModel{})

	// 当前业务库(ds="")下：db_a 有 status/amount 两列，db_b 也有同名表 status 一列（跨库）；
	// 另有外部数据源 ds=ext 的同名表——应被数据源隔离排除。
	seed := []*dataprofile.ColumnProfile{
		mk("", "db_a", "status"), mk("", "db_a", "amount"),
		mk("", "db_b", "status"),
		mk("ext", "db_a", "status"),
	}
	if err := repo.Upsert(ctx, seed); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// 业务库(ds="")：跨 db_a/db_b 共 3 行；外部 ds=ext 的行不应混入。
	got, err := repo.ListByDatasourceTable(ctx, tenant, "", "dst_orders")
	if err != nil {
		t.Fatalf("ListByDatasourceTable: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("业务库跨 schema 应返回 3 行，实得 %d", len(got))
	}
	schemas := map[string]struct{}{}
	for _, p := range got {
		schemas[p.SchemaName] = struct{}{}
		if p.DatasourceID != "" {
			t.Errorf("混入了外部数据源画像: %+v", p)
		}
	}
	if _, ok := schemas["db_a"]; !ok {
		t.Error("缺 db_a 的画像")
	}
	if _, ok := schemas["db_b"]; !ok {
		t.Error("缺 db_b 的画像（跨 schema 未返回）")
	}

	// 外部数据源隔离：ds=ext 只应看到自己那 1 行。
	gotExt, _ := repo.ListByDatasourceTable(ctx, tenant, "ext", "dst_orders")
	if len(gotExt) != 1 || gotExt[0].SchemaName != "db_a" {
		t.Errorf("外部数据源隔离失败: %+v", gotExt)
	}
}
