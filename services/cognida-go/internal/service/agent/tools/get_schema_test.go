// Package tools 测试 Schema 获取工具
package tools

import (
	"context"
	"regexp"
	"testing"

	model_datasource "cognida/internal/model/datasource"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// setupMockSchemaDB 创建 mock 数据库用于 Schema 测试
func setupMockSchemaDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create mock database: %v", err)
	}

	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      db,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open gorm connection: %v", err)
	}

	return gormDB, mock
}

// expectLoadTableCards 为 loadTableCards 的两次 information_schema 查询登记期望。
func expectLoadTableCards(mock sqlmock.Sqlmock, dbName string, tables, columns *sqlmock.Rows) {
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT table_name, table_comment FROM information_schema.tables WHERE table_schema = ? AND table_type = 'BASE TABLE' ORDER BY table_name",
	)).WithArgs(dbName).WillReturnRows(tables)
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT table_name, column_name, column_comment FROM information_schema.columns WHERE table_schema = ? ORDER BY table_name, ordinal_position",
	)).WithArgs(dbName).WillReturnRows(columns)
}

// mainDatasource 将 sqlmock 库封装为外部数据源 "main" 提供者（DatabaseID 非空 → 外部路由）。
func mainDatasource(t *testing.T, gormDB *gorm.DB) model_datasource.ConnectionProvider {
	t.Helper()
	rawDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("failed to get raw db: %v", err)
	}
	return &fakeConnProvider{id: "main", dbName: "main", db: rawDB}
}

// TestGetSchema 测试 Schema 获取（agent 工具路径：有界选表，经外部数据源 "main" 路由）
func TestGetSchema(t *testing.T) {
	gormDB, mock := setupMockSchemaDB(t)
	dsp := mainDatasource(t, gormDB)

	ctx := context.Background()

	t.Run("no table name returns lightweight catalog", func(t *testing.T) {
		// 无表名无关键词：走 loadTableCards → 轻量目录（表名+描述，不含列）。
		tableRows := sqlmock.NewRows([]string{"table_name", "table_comment"}).
			AddRow("orders", "订单表").
			AddRow("products", "商品表").
			AddRow("users", "用户表")
		colRows := sqlmock.NewRows([]string{"table_name", "column_name", "column_comment"}).
			AddRow("orders", "id", "").
			AddRow("products", "id", "").
			AddRow("users", "id", "").
			AddRow("users", "name", "用户名")
		expectLoadTableCards(mock, "main", tableRows, colRows)

		req := &GetSchemaRequest{DatabaseID: "main", TableName: ""}
		result, err := getSchema(ctx, req, gormDB, dsp, nil)
		if err != nil {
			t.Fatalf("getSchema() error = %v", err)
		}

		if len(result.Tables) != 3 {
			t.Errorf("expected 3 tables in catalog, got %d", len(result.Tables))
		}
		if result.Note == "" {
			t.Error("expected a Note explaining the catalog fallback")
		}
		// 目录仅含表名+描述，不含列结构。
		for _, tbl := range result.Tables {
			if len(tbl.Columns) != 0 {
				t.Errorf("catalog table %s should carry no columns, got %d", tbl.TableName, len(tbl.Columns))
			}
		}
		if result.Database != "main" {
			t.Errorf("expected database 'main', got '%s'", result.Database)
		}
	})

	t.Run("keywords select relevant tables", func(t *testing.T) {
		// 有关键词：loadTableCards → 相关度选表 → 对命中表查完整结构。
		tableRows := sqlmock.NewRows([]string{"table_name", "table_comment"}).
			AddRow("orders", "订单表").
			AddRow("products", "商品表").
			AddRow("users", "用户表")
		colRows := sqlmock.NewRows([]string{"table_name", "column_name", "column_comment"}).
			AddRow("orders", "id", "").
			AddRow("orders", "total", "订单金额").
			AddRow("products", "id", "").
			AddRow("users", "id", "")
		expectLoadTableCards(mock, "main", tableRows, colRows)

		// 关键词 "orders" 命中 orders 表 → 查其完整结构。
		ordersColumns := sqlmock.NewRows([]string{"column_name", "data_type", "is_nullable", "column_comment"}).
			AddRow("id", "int", "NO", "").
			AddRow("total", "decimal", "YES", "订单金额")
		mock.ExpectQuery(regexp.QuoteMeta(
			"SELECT column_name, data_type, is_nullable, column_comment FROM information_schema.columns WHERE table_schema = ? AND table_name = ? ORDER BY ordinal_position",
		)).WithArgs("main", "orders").WillReturnRows(ordersColumns)
		mock.ExpectQuery(regexp.QuoteMeta(
			"SELECT column_name FROM information_schema.key_column_usage WHERE table_schema = ? AND table_name = ? AND constraint_name = 'PRIMARY'",
		)).WithArgs("main", "orders").WillReturnRows(sqlmock.NewRows([]string{"column_name"}).AddRow("id"))

		req := &GetSchemaRequest{DatabaseID: "main", TableName: "", Keywords: "orders"}
		result, err := getSchema(ctx, req, gormDB, dsp, nil)
		if err != nil {
			t.Fatalf("getSchema() error = %v", err)
		}

		if len(result.Tables) != 1 {
			t.Fatalf("expected 1 relevant table, got %d", len(result.Tables))
		}
		if result.Tables[0].TableName != "orders" {
			t.Errorf("expected relevant table 'orders', got '%s'", result.Tables[0].TableName)
		}
		if len(result.Tables[0].Columns) != 2 {
			t.Errorf("expected full column detail for selected table, got %d cols", len(result.Tables[0].Columns))
		}
		if result.Note == "" {
			t.Error("expected a Note describing relevance selection")
		}
	})

	t.Run("keywords no hit falls back to catalog", func(t *testing.T) {
		// 关键词无命中：返回受上限约束的轻量目录，禁止无上限全库详细回退。
		tableRows := sqlmock.NewRows([]string{"table_name", "table_comment"}).
			AddRow("orders", "订单表").
			AddRow("users", "用户表")
		colRows := sqlmock.NewRows([]string{"table_name", "column_name", "column_comment"}).
			AddRow("orders", "id", "").
			AddRow("users", "id", "")
		expectLoadTableCards(mock, "main", tableRows, colRows)

		req := &GetSchemaRequest{DatabaseID: "main", TableName: "", Keywords: "不存在的关键词xyz"}
		result, err := getSchema(ctx, req, gormDB, dsp, nil)
		if err != nil {
			t.Fatalf("getSchema() error = %v", err)
		}
		if len(result.Tables) != 2 {
			t.Errorf("expected catalog of 2 tables on no-hit, got %d", len(result.Tables))
		}
		for _, tbl := range result.Tables {
			if len(tbl.Columns) != 0 {
				t.Errorf("no-hit fallback must stay lightweight, table %s carried %d cols", tbl.TableName, len(tbl.Columns))
			}
		}
	})

	t.Run("get specific table", func(t *testing.T) {
		// 指定表名：跳过存在性查询，直接查列 + 主键。
		columnRows := sqlmock.NewRows([]string{"column_name", "data_type", "is_nullable", "column_comment"}).
			AddRow("id", "int", "NO", "Primary key").
			AddRow("name", "varchar", "NO", "User name").
			AddRow("email", "varchar", "YES", "Email address").
			AddRow("created_at", "datetime", "YES", "Creation time")
		mock.ExpectQuery(regexp.QuoteMeta(
			"SELECT column_name, data_type, is_nullable, column_comment FROM information_schema.columns WHERE table_schema = ? AND table_name = ? ORDER BY ordinal_position",
		)).WithArgs("main", "users").WillReturnRows(columnRows)

		pkRows := sqlmock.NewRows([]string{"column_name"}).AddRow("id")
		mock.ExpectQuery(regexp.QuoteMeta(
			"SELECT column_name FROM information_schema.key_column_usage WHERE table_schema = ? AND table_name = ? AND constraint_name = 'PRIMARY'",
		)).WithArgs("main", "users").WillReturnRows(pkRows)

		req := &GetSchemaRequest{DatabaseID: "main", TableName: "users"}
		result, err := getSchema(ctx, req, gormDB, dsp, nil)
		if err != nil {
			t.Fatalf("getSchema() error = %v", err)
		}

		if len(result.Tables) != 1 {
			t.Fatalf("expected 1 table, got %d", len(result.Tables))
		}

		table := result.Tables[0]
		if table.TableName != "users" {
			t.Errorf("expected table name 'users', got '%s'", table.TableName)
		}

		expectedColumns := []string{"id", "name", "email", "created_at"}
		if len(table.Columns) != len(expectedColumns) {
			t.Errorf("expected %d columns, got %d", len(expectedColumns), len(table.Columns))
		}
		for i, col := range table.Columns {
			if col.Name != expectedColumns[i] {
				t.Errorf("expected column '%s', got '%s'", expectedColumns[i], col.Name)
			}
		}
		if table.PrimaryKey != "id" {
			t.Errorf("expected primary key 'id', got '%s'", table.PrimaryKey)
		}
	})

	t.Run("non-existent table", func(t *testing.T) {
		// 指定不存在的表：列查询返回空 → 跳过，不再发主键查询，结果 0 张表。
		mock.ExpectQuery(regexp.QuoteMeta(
			"SELECT column_name, data_type, is_nullable, column_comment FROM information_schema.columns WHERE table_schema = ? AND table_name = ? ORDER BY ordinal_position",
		)).WithArgs("main", "nonexistent").WillReturnRows(
			sqlmock.NewRows([]string{"column_name", "data_type", "is_nullable", "column_comment"}))

		req := &GetSchemaRequest{DatabaseID: "main", TableName: "nonexistent"}
		result, err := getSchema(ctx, req, gormDB, dsp, nil)
		if err != nil {
			t.Fatalf("getSchema() error = %v", err)
		}
		if len(result.Tables) != 0 {
			t.Errorf("expected 0 tables for non-existent table, got %d", len(result.Tables))
		}
	})
}

// TestGetSchemaWithoutInit 测试未初始化的情况
func TestGetSchemaWithoutInit(t *testing.T) {
	// DatabaseID 为空 → 业务库路径；业务库未注入（nil）必须显式报错。
	ctx := context.Background()
	req := &GetSchemaRequest{DatabaseID: ""}

	_, err := getSchema(ctx, req, nil, nil, nil)
	if err == nil {
		t.Error("expected error when DB not initialized")
	}
	if err.Error() != "数据库未初始化" {
		t.Errorf("expected specific error message, got '%v'", err)
	}
}

// TestColumnNullable 测试列的可空属性
func TestColumnNullable(t *testing.T) {
	gormDB, mock := setupMockSchemaDB(t)
	dsp := mainDatasource(t, gormDB)

	ctx := context.Background()

	// name 是 NOT NULL, email 是 nullable
	columnRows := sqlmock.NewRows([]string{"column_name", "data_type", "is_nullable", "column_comment"}).
		AddRow("id", "int", "NO", "").
		AddRow("name", "varchar", "NO", "").  // NOT NULL
		AddRow("email", "varchar", "YES", "") // nullable
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT column_name, data_type, is_nullable, column_comment FROM information_schema.columns WHERE table_schema = ? AND table_name = ? ORDER BY ordinal_position",
	)).WithArgs("main", "users").WillReturnRows(columnRows)

	pkRows := sqlmock.NewRows([]string{"column_name"}).AddRow("id")
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT column_name FROM information_schema.key_column_usage WHERE table_schema = ? AND table_name = ? AND constraint_name = 'PRIMARY'",
	)).WithArgs("main", "users").WillReturnRows(pkRows)

	req := &GetSchemaRequest{DatabaseID: "main", TableName: "users"}
	result, err := getSchema(ctx, req, gormDB, dsp, nil)
	if err != nil {
		t.Fatalf("getSchema() error = %v", err)
	}

	table := result.Tables[0]

	nameCol := findColumn(table.Columns, "name")
	if nameCol == nil {
		t.Fatal("name column not found")
	}
	if nameCol.Nullable {
		t.Error("expected name column to be NOT NULL")
	}

	emailCol := findColumn(table.Columns, "email")
	if emailCol == nil {
		t.Fatal("email column not found")
	}
	if !emailCol.Nullable {
		t.Error("expected email column to be nullable")
	}
}

// findColumn 查找列
func findColumn(columns []ColumnSchema, name string) *ColumnSchema {
	for _, col := range columns {
		if col.Name == name {
			return &col
		}
	}
	return nil
}

// TestGetSchemaWithComments 测试带注释的列
func TestGetSchemaWithComments(t *testing.T) {
	gormDB, mock := setupMockSchemaDB(t)
	dsp := mainDatasource(t, gormDB)

	ctx := context.Background()

	columnRows := sqlmock.NewRows([]string{"column_name", "data_type", "is_nullable", "column_comment"}).
		AddRow("id", "int", "NO", "用户ID").
		AddRow("name", "varchar", "NO", "用户名").
		AddRow("email", "varchar", "YES", "邮箱地址")
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT column_name, data_type, is_nullable, column_comment FROM information_schema.columns WHERE table_schema = ? AND table_name = ? ORDER BY ordinal_position",
	)).WithArgs("main", "users").WillReturnRows(columnRows)

	pkRows := sqlmock.NewRows([]string{"column_name"}).AddRow("id")
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT column_name FROM information_schema.key_column_usage WHERE table_schema = ? AND table_name = ? AND constraint_name = 'PRIMARY'",
	)).WithArgs("main", "users").WillReturnRows(pkRows)

	req := &GetSchemaRequest{DatabaseID: "main", TableName: "users"}
	result, err := getSchema(ctx, req, gormDB, dsp, nil)
	if err != nil {
		t.Fatalf("getSchema() error = %v", err)
	}

	table := result.Tables[0]

	idCol := findColumn(table.Columns, "id")
	if idCol.Description != "用户ID" {
		t.Errorf("expected id comment '用户ID', got '%s'", idCol.Description)
	}

	nameCol := findColumn(table.Columns, "name")
	if nameCol.Description != "用户名" {
		t.Errorf("expected name comment '用户名', got '%s'", nameCol.Description)
	}
}

// --- 纯词法选表函数单测（无需 DB，task 2.5）---

// TestTokenize 校验分词：小写化、按分隔符切分、去重。
func TestTokenize(t *testing.T) {
	got := tokenize("区域, 销售额/Sales_Amount 销售额")
	want := map[string]bool{"区域": true, "销售额": true, "sales": true, "amount": true}
	if len(got) != len(want) {
		t.Fatalf("tokenize len = %d (%v), want %d", len(got), got, len(want))
	}
	for _, tok := range got {
		if !want[tok] {
			t.Errorf("unexpected token %q", tok)
		}
	}
	if len(tokenize("   ")) != 0 {
		t.Error("blank input should yield no tokens")
	}
}

// TestRankTablesByRelevance 校验相关度打分：表名命中权重高、无命中不入选、Top-K 截断、确定性排序。
func TestRankTablesByRelevance(t *testing.T) {
	cards := []tableCard{
		{Name: "sales_order", Comment: "销售订单", Columns: []string{"amount 销售额"}},
		{Name: "region", Comment: "区域维表", Columns: []string{"region name"}},
		{Name: "product", Comment: "商品", Columns: []string{"price"}},
	}

	// "sales" 命中 sales_order 表名(×3) 与 无其它；"区域" 命中 region 注释(×1)。
	got := rankTablesByRelevance(cards, "sales 区域", 15)
	if len(got) != 2 {
		t.Fatalf("expected 2 relevant tables, got %v", got)
	}
	// sales_order 分数(3) > region 分数(1)，排在前。
	if got[0] != "sales_order" {
		t.Errorf("expected sales_order ranked first, got %v", got)
	}

	// 无命中返回空。
	if r := rankTablesByRelevance(cards, "zzz不相关", 15); len(r) != 0 {
		t.Errorf("expected no matches, got %v", r)
	}

	// Top-K 截断。
	if r := rankTablesByRelevance(cards, "sales 区域", 1); len(r) != 1 || r[0] != "sales_order" {
		t.Errorf("expected top-1 = [sales_order], got %v", r)
	}

	// 空关键词返回空。
	if r := rankTablesByRelevance(cards, "   ", 15); r != nil {
		t.Errorf("blank keywords should yield nil, got %v", r)
	}
}

// TestCatalogResult 校验轻量目录：只带表名+描述、无列、受上限约束。
func TestCatalogResult(t *testing.T) {
	cards := []tableCard{
		{Name: "a", Comment: "表A", Columns: []string{"x"}},
		{Name: "b", Comment: "表B"},
	}
	res, err := catalogResult("main", cards, "note")
	if err != nil {
		t.Fatalf("catalogResult error = %v", err)
	}
	if len(res.Tables) != 2 {
		t.Fatalf("expected 2 catalog tables, got %d", len(res.Tables))
	}
	if res.Tables[0].TableName != "a" || res.Tables[0].Description != "表A" {
		t.Errorf("catalog table 0 = %+v, want name a/desc 表A", res.Tables[0])
	}
	if len(res.Tables[0].Columns) != 0 {
		t.Error("catalog must not carry columns")
	}
	if res.Note != "note" {
		t.Errorf("note = %q, want 'note'", res.Note)
	}
}
