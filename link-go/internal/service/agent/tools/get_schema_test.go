// Package tools 测试 Schema 获取工具
package tools

import (
	"context"
	"regexp"
	"testing"

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

// TestGetSchema 测试 Schema 获取
func TestGetSchema(t *testing.T) {
	gormDB, mock := setupMockSchemaDB(t)
	InitGetSchemaTool(gormDB)

	ctx := context.Background()

	t.Run("get all tables", func(t *testing.T) {
		// Mock information_schema.tables 查询
		tableRows := sqlmock.NewRows([]string{"table_name"}).
			AddRow("users").
			AddRow("orders").
			AddRow("products")

		mock.ExpectQuery(regexp.QuoteMeta(
			"SELECT table_name FROM information_schema.tables WHERE table_schema = ? AND table_type = 'BASE TABLE'",
		)).WithArgs("main").WillReturnRows(tableRows)

		// Mock information_schema.columns 查询 (为每个表调用)
		usersColumns := sqlmock.NewRows([]string{"column_name", "data_type", "is_nullable", "column_comment"}).
			AddRow("id", "int", "NO", "").
			AddRow("name", "varchar", "NO", "").
			AddRow("email", "varchar", "YES", "").
			AddRow("created_at", "datetime", "YES", "")

		mock.ExpectQuery(regexp.QuoteMeta(
			"SELECT column_name, data_type, is_nullable, column_comment FROM information_schema.columns WHERE table_schema = ? AND table_name = ? ORDER BY ordinal_position",
		)).WithArgs("main", "users").WillReturnRows(usersColumns)

		// Mock primary key 查询
		pkRows := sqlmock.NewRows([]string{"column_name"}).AddRow("id")
		mock.ExpectQuery(regexp.QuoteMeta(
			"SELECT column_name FROM information_schema.key_column_usage WHERE table_schema = ? AND table_name = ? AND constraint_name = 'PRIMARY'",
		)).WithArgs("main", "users").WillReturnRows(pkRows)

		// Orders 表
		ordersColumns := sqlmock.NewRows([]string{"column_name", "data_type", "is_nullable", "column_comment"}).
			AddRow("id", "int", "NO", "").
			AddRow("user_id", "int", "YES", "").
			AddRow("total", "decimal", "YES", "").
			AddRow("status", "varchar", "YES", "")

		mock.ExpectQuery(regexp.QuoteMeta(
			"SELECT column_name, data_type, is_nullable, column_comment FROM information_schema.columns WHERE table_schema = ? AND table_name = ? ORDER BY ordinal_position",
		)).WithArgs("main", "orders").WillReturnRows(ordersColumns)

		mock.ExpectQuery(regexp.QuoteMeta(
			"SELECT column_name FROM information_schema.key_column_usage WHERE table_schema = ? AND table_name = ? AND constraint_name = 'PRIMARY'",
		)).WithArgs("main", "orders").WillReturnRows(sqlmock.NewRows([]string{"column_name"}).AddRow("id"))

		// Products 表
		productsColumns := sqlmock.NewRows([]string{"column_name", "data_type", "is_nullable", "column_comment"}).
			AddRow("id", "int", "NO", "").
			AddRow("name", "varchar", "NO", "").
			AddRow("price", "decimal", "YES", "")

		mock.ExpectQuery(regexp.QuoteMeta(
			"SELECT column_name, data_type, is_nullable, column_comment FROM information_schema.columns WHERE table_schema = ? AND table_name = ? ORDER BY ordinal_position",
		)).WithArgs("main", "products").WillReturnRows(productsColumns)

		mock.ExpectQuery(regexp.QuoteMeta(
			"SELECT column_name FROM information_schema.key_column_usage WHERE table_schema = ? AND table_name = ? AND constraint_name = 'PRIMARY'",
		)).WithArgs("main", "products").WillReturnRows(sqlmock.NewRows([]string{"column_name"}).AddRow("id"))

		req := &GetSchemaRequest{
			DatabaseID: "main",
			TableName:  "",
		}

		result, err := getSchema(ctx, req)
		if err != nil {
			t.Fatalf("getSchema() error = %v", err)
		}

		if len(result.Tables) != 3 {
			t.Errorf("expected 3 tables, got %d", len(result.Tables))
		}

		if result.Database != "main" {
			t.Errorf("expected database 'main', got '%s'", result.Database)
		}
	})

	t.Run("get specific table", func(t *testing.T) {
		// Mock table 查询
		tableRows := sqlmock.NewRows([]string{"table_name"}).AddRow("users")
		mock.ExpectQuery(regexp.QuoteMeta(
			"SELECT table_name FROM information_schema.tables WHERE table_schema = ? AND table_type = 'BASE TABLE' AND table_name = ?",
		)).WithArgs("main", "users").WillReturnRows(tableRows)

		// Mock columns 查询
		columnRows := sqlmock.NewRows([]string{"column_name", "data_type", "is_nullable", "column_comment"}).
			AddRow("id", "int", "NO", "Primary key").
			AddRow("name", "varchar", "NO", "User name").
			AddRow("email", "varchar", "YES", "Email address").
			AddRow("created_at", "datetime", "YES", "Creation time")

		mock.ExpectQuery(regexp.QuoteMeta(
			"SELECT column_name, data_type, is_nullable, column_comment FROM information_schema.columns WHERE table_schema = ? AND table_name = ? ORDER BY ordinal_position",
		)).WithArgs("main", "users").WillReturnRows(columnRows)

		// Mock primary key 查询
		pkRows := sqlmock.NewRows([]string{"column_name"}).AddRow("id")
		mock.ExpectQuery(regexp.QuoteMeta(
			"SELECT column_name FROM information_schema.key_column_usage WHERE table_schema = ? AND table_name = ? AND constraint_name = 'PRIMARY'",
		)).WithArgs("main", "users").WillReturnRows(pkRows)

		req := &GetSchemaRequest{
			DatabaseID: "main",
			TableName:  "users",
		}

		result, err := getSchema(ctx, req)
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

		// 检查列
		expectedColumns := []string{"id", "name", "email", "created_at"}
		if len(table.Columns) != len(expectedColumns) {
			t.Errorf("expected %d columns, got %d", len(expectedColumns), len(table.Columns))
		}

		for i, col := range table.Columns {
			if col.Name != expectedColumns[i] {
				t.Errorf("expected column '%s', got '%s'", expectedColumns[i], col.Name)
			}
		}

		// 检查主键
		if table.PrimaryKey != "id" {
			t.Errorf("expected primary key 'id', got '%s'", table.PrimaryKey)
		}
	})

	t.Run("non-existent table", func(t *testing.T) {
		// Mock table 查询返回空
		tableRows := sqlmock.NewRows([]string{"table_name"})
		mock.ExpectQuery(regexp.QuoteMeta(
			"SELECT table_name FROM information_schema.tables WHERE table_schema = ? AND table_type = 'BASE TABLE' AND table_name = ?",
		)).WithArgs("main", "nonexistent").WillReturnRows(tableRows)

		req := &GetSchemaRequest{
			DatabaseID: "main",
			TableName:  "nonexistent",
		}

		result, err := getSchema(ctx, req)
		if err != nil {
			t.Fatalf("getSchema() error = %v", err)
		}

		if len(result.Tables) != 0 {
			t.Errorf("expected 0 tables for non-existent table, got %d", len(result.Tables))
		}
	})

	t.Run("default database", func(t *testing.T) {
		// 需要模拟 CurrentDatabase() 调用
		// 由于 GORM 的 CurrentDatabase() 实现较复杂，这里简化测试
		req := &GetSchemaRequest{
			DatabaseID: "main", // 显式指定而不是使用默认
			TableName:  "",
		}

		tableRows := sqlmock.NewRows([]string{"table_name"}).AddRow("users")
		mock.ExpectQuery(regexp.QuoteMeta(
			"SELECT table_name FROM information_schema.tables WHERE table_schema = ? AND table_type = 'BASE TABLE'",
		)).WithArgs("main").WillReturnRows(tableRows)

		columnRows := sqlmock.NewRows([]string{"column_name", "data_type", "is_nullable", "column_comment"}).
			AddRow("id", "int", "NO", "")
		mock.ExpectQuery(regexp.QuoteMeta(
			"SELECT column_name, data_type, is_nullable, column_comment FROM information_schema.columns WHERE table_schema = ? AND table_name = ? ORDER BY ordinal_position",
		)).WithArgs("main", "users").WillReturnRows(columnRows)

		pkRows := sqlmock.NewRows([]string{"column_name"}).AddRow("id")
		mock.ExpectQuery(regexp.QuoteMeta(
			"SELECT column_name FROM information_schema.key_column_usage WHERE table_schema = ? AND table_name = ? AND constraint_name = 'PRIMARY'",
		)).WithArgs("main", "users").WillReturnRows(pkRows)

		result, err := getSchema(ctx, req)
		if err != nil {
			t.Fatalf("getSchema() error = %v", err)
		}

		if result.Database != "main" {
			t.Errorf("expected database 'main', got '%s'", result.Database)
		}
	})
}

// TestGetSchemaWithoutInit 测试未初始化的情况
func TestGetSchemaWithoutInit(t *testing.T) {
	// 保存原有的 DB
	oldDB := getSchemaDB
	defer func() {
		getSchemaDB = oldDB
	}()

	// 设置为 nil
	getSchemaDB = nil

	ctx := context.Background()
	req := &GetSchemaRequest{
		DatabaseID: "main",
	}

	_, err := getSchema(ctx, req)
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
	InitGetSchemaTool(gormDB)

	ctx := context.Background()

	// Mock table 查询
	tableRows := sqlmock.NewRows([]string{"table_name"}).AddRow("users")
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT table_name FROM information_schema.tables WHERE table_schema = ? AND table_type = 'BASE TABLE' AND table_name = ?",
	)).WithArgs("main", "users").WillReturnRows(tableRows)

	// Mock columns 查询 - name 是 NOT NULL, email 是 nullable
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

	req := &GetSchemaRequest{
		DatabaseID: "main",
		TableName:  "users",
	}

	result, err := getSchema(ctx, req)
	if err != nil {
		t.Fatalf("getSchema() error = %v", err)
	}

	table := result.Tables[0]

	// 检查 name 列的 NOT NULL 约束
	nameCol := findColumn(table.Columns, "name")
	if nameCol == nil {
		t.Fatal("name column not found")
	}

	if nameCol.Nullable {
		t.Error("expected name column to be NOT NULL")
	}

	// 检查 email 列可为空
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
	InitGetSchemaTool(gormDB)

	ctx := context.Background()

	tableRows := sqlmock.NewRows([]string{"table_name"}).AddRow("users")
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT table_name FROM information_schema.tables WHERE table_schema = ? AND table_type = 'BASE TABLE' AND table_name = ?",
	)).WithArgs("main", "users").WillReturnRows(tableRows)

	// Mock columns with comments
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

	req := &GetSchemaRequest{
		DatabaseID: "main",
		TableName:  "users",
	}

	result, err := getSchema(ctx, req)
	if err != nil {
		t.Fatalf("getSchema() error = %v", err)
	}

	table := result.Tables[0]

	// 检查列注释
	idCol := findColumn(table.Columns, "id")
	if idCol.Description != "用户ID" {
		t.Errorf("expected id comment '用户ID', got '%s'", idCol.Description)
	}

	nameCol := findColumn(table.Columns, "name")
	if nameCol.Description != "用户名" {
		t.Errorf("expected name comment '用户名', got '%s'", nameCol.Description)
	}
}
