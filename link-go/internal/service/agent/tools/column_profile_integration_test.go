// Package tools 集成测试 - 列画像采集（连接本地 MySQL）
//go:build integration
// +build integration

package tools

import (
	"context"
	"testing"
)

// TestComputeColumnProfilesIntegration 建临时表、灌入已知分布的数据，验证画像采集：
// 行数、空值率、基数、以及低基数枚举列的 Top-N 实际取值分布。
func TestComputeColumnProfilesIntegration(t *testing.T) {
	gormDB := getTestDB(t)
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("取底层 *sql.DB: %v", err)
	}
	ctx := context.Background()

	schema := gormDB.Migrator().CurrentDatabase()
	if schema == "" {
		t.Fatal("无法解析当前库名")
	}
	const table = "itest_profile_orders"

	exec := func(q string) {
		if _, err := sqlDB.ExecContext(ctx, q); err != nil {
			t.Fatalf("建表/灌数失败: %v\nSQL: %s", err, q)
		}
	}
	exec("DROP TABLE IF EXISTS " + table)
	exec(`CREATE TABLE ` + table + ` (
		id BIGINT PRIMARY KEY,
		status VARCHAR(32) NOT NULL,
		note VARCHAR(255) NULL,
		amount DECIMAL(10,2) NULL
	)`)
	defer sqlDB.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+table)

	// 10 行：status 分布 completed×6 / shipped×3 / paid×1（枚举，基数 3）；
	// note 前 4 行为 NULL（空值率 0.4）；id 唯一（基数 10，非枚举）。
	exec(`INSERT INTO ` + table + ` (id, status, note, amount) VALUES
		(1,'completed',NULL,10.00),
		(2,'completed',NULL,20.00),
		(3,'completed',NULL,30.00),
		(4,'completed',NULL,40.00),
		(5,'completed','a',50.00),
		(6,'completed','b',60.00),
		(7,'shipped','c',70.00),
		(8,'shipped','d',80.00),
		(9,'shipped','e',90.00),
		(10,'paid','f',100.00)`)

	tableSchema := TableSchema{
		TableName: table,
		Columns: []ColumnSchema{
			{Name: "id", Type: "bigint"},
			{Name: "status", Type: "varchar(32)"},
			{Name: "note", Type: "varchar(255)"},
			{Name: "amount", Type: "decimal(10,2)"},
		},
	}

	profiles, err := computeColumnProfiles(ctx, sqlDB, 1, "", schema, tableSchema)
	if err != nil {
		t.Fatalf("computeColumnProfiles: %v", err)
	}
	if len(profiles) != 4 {
		t.Fatalf("期望 4 列画像，实得 %d", len(profiles))
	}
	byCol := map[string]int{}
	for i, p := range profiles {
		byCol[p.ColumnName] = i
		if p.RowCount != 10 {
			t.Errorf("%s 行数应为 10，实得 %d", p.ColumnName, p.RowCount)
		}
	}

	// id：基数 10，唯一→非枚举候选，无 top_values。
	id := profiles[byCol["id"]]
	if id.Distinct != 10 || len(id.TopValues) != 0 {
		t.Errorf("id 画像不符: distinct=%d top=%v", id.Distinct, id.TopValues)
	}

	// status：基数 3，枚举→有 top_values，按频次降序 completed(6) > shipped(3) > paid(1)。
	st := profiles[byCol["status"]]
	if st.Distinct != 3 {
		t.Errorf("status 基数应为 3，实得 %d", st.Distinct)
	}
	if st.NullRate != 0 {
		t.Errorf("status 空值率应为 0，实得 %v", st.NullRate)
	}
	if len(st.TopValues) != 3 {
		t.Fatalf("status 应有 3 个枚举取值，实得 %d: %+v", len(st.TopValues), st.TopValues)
	}
	if st.TopValues[0].Value != "completed" || st.TopValues[0].Count != 6 {
		t.Errorf("status 首个枚举应为 completed×6，实得 %+v", st.TopValues[0])
	}
	if st.TopValues[2].Value != "paid" || st.TopValues[2].Count != 1 {
		t.Errorf("status 末个枚举应为 paid×1，实得 %+v", st.TopValues[2])
	}

	// note：4/10 为 NULL，空值率 0.4；基数 6（a~f）→枚举。
	note := profiles[byCol["note"]]
	if note.NullCount != 4 || note.NullRate != 0.4 {
		t.Errorf("note 空值统计不符: nullCount=%d nullRate=%v", note.NullCount, note.NullRate)
	}
	if note.Distinct != 6 {
		t.Errorf("note 基数应为 6，实得 %d", note.Distinct)
	}

	// amount：decimal 连续型→非枚举候选，无 top_values（即便基数低）。
	amount := profiles[byCol["amount"]]
	if len(amount.TopValues) != 0 {
		t.Errorf("decimal 连续列不应采集枚举分布: %+v", amount.TopValues)
	}
}

// TestComputeColumnProfilesGeometryIntegration 回归：含几何列的表不能因
// COUNT(DISTINCT geom) 报错而拖垮整表画像——几何列跳过基数，其余列照常采集。
func TestComputeColumnProfilesGeometryIntegration(t *testing.T) {
	gormDB := getTestDB(t)
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("取底层 *sql.DB: %v", err)
	}
	ctx := context.Background()
	schema := gormDB.Migrator().CurrentDatabase()
	const table = "itest_profile_geo"

	exec := func(q string) {
		if _, err := sqlDB.ExecContext(ctx, q); err != nil {
			t.Fatalf("建表/灌数失败: %v\nSQL: %s", err, q)
		}
	}
	exec("DROP TABLE IF EXISTS " + table)
	exec(`CREATE TABLE ` + table + ` (
		id BIGINT PRIMARY KEY,
		status VARCHAR(32) NOT NULL,
		loc POINT NOT NULL
	)`)
	defer sqlDB.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+table)
	exec(`INSERT INTO ` + table + ` (id, status, loc) VALUES
		(1,'a',ST_GeomFromText('POINT(1 1)')),
		(2,'a',ST_GeomFromText('POINT(2 2)')),
		(3,'b',ST_GeomFromText('POINT(3 3)'))`)

	tableSchema := TableSchema{
		TableName: table,
		Columns: []ColumnSchema{
			{Name: "id", Type: "bigint"},
			{Name: "status", Type: "varchar(32)"},
			{Name: "loc", Type: "point"},
		},
	}

	profiles, err := computeColumnProfiles(ctx, sqlDB, 1, "", schema, tableSchema)
	if err != nil {
		t.Fatalf("含几何列的聚合画像不应报错: %v", err)
	}
	if len(profiles) != 3 {
		t.Fatalf("期望 3 列画像，实得 %d", len(profiles))
	}
	byCol := map[string]int{}
	for i, p := range profiles {
		byCol[p.ColumnName] = i
	}
	// status 列照常采集到基数与枚举分布。
	st := profiles[byCol["status"]]
	if st.Distinct != 2 || len(st.TopValues) != 2 {
		t.Errorf("几何列旁的 status 列应正常画像: distinct=%d top=%+v", st.Distinct, st.TopValues)
	}
	// 几何列跳过基数（占位 0），无枚举分布。
	loc := profiles[byCol["loc"]]
	if loc.Distinct != 0 || len(loc.TopValues) != 0 {
		t.Errorf("几何列应跳过基数/枚举: distinct=%d top=%+v", loc.Distinct, loc.TopValues)
	}
	if loc.RowCount != 3 {
		t.Errorf("几何列行数仍应为 3，实得 %d", loc.RowCount)
	}
}
