// Package tools 集成测试 - 连接本地 MySQL
//go:build integration
// +build integration

package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/joho/godotenv"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// getTestDB 从环境变量获取测试数据库连接
func getTestDB(t *testing.T) *gorm.DB {
	// 加载 .env 文件
	if err := loadEnv(); err != nil {
		t.Logf("Warning: failed to load .env file: %v", err)
	}

	// 从环境变量读取配置
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "3306")
	user := getEnv("DB_USER", "root")
	password := getEnv("DB_PASSWORD", "")
	dbname := getEnv("DB_NAME", "link")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		user, password, host, port, dbname)

	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to mysql: %v", err)
	}

	// 测试连接
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql.DB: %v", err)
	}

	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("failed to ping mysql: %v", err)
	}

	return db
}

// getEnv 获取环境变量，带默认值
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// loadEnv 加载 .env 文件
func loadEnv() error {
	// 尝试多个可能的 .env 位置
	paths := []string{
		".env",
		"../../.env",
		"../../../.env",
		"../../../../.env",
		"../../../../../.env",
	}

	for _, path := range paths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		if err := godotenv.Load(absPath); err == nil {
			return nil
		}
	}
	return godotenv.Load() // 尝试默认位置
}

// setupIntegrationTestData 创建集成测试数据
func setupIntegrationTestData(t *testing.T, db *gorm.DB) {
	// 创建测试表（如果不存在）
	db.Exec(`CREATE TABLE IF NOT EXISTS integration_test_users (
		id INT PRIMARY KEY AUTO_INCREMENT,
		name VARCHAR(100) NOT NULL COMMENT '用户名',
		email VARCHAR(200) COMMENT '邮箱',
		age INT COMMENT '年龄',
		status TINYINT DEFAULT 1 COMMENT '状态',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	) COMMENT='集成测试用户表'`)

	db.Exec(`CREATE TABLE IF NOT EXISTS integration_test_orders (
		id INT PRIMARY KEY AUTO_INCREMENT,
		user_id INT NOT NULL,
		order_no VARCHAR(50) NOT NULL,
		total_amount DECIMAL(10,2) NOT NULL,
		status VARCHAR(20) DEFAULT 'pending',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		KEY idx_user_id (user_id)
	) COMMENT='集成测试订单表'`)

	// 清空测试数据
	db.Exec(`DELETE FROM integration_test_orders WHERE user_id IN (SELECT id FROM integration_test_users WHERE name LIKE 'integration_%')`)
	db.Exec(`DELETE FROM integration_test_users WHERE name LIKE 'integration_%'`)

	// 插入测试数据
	db.Exec(`INSERT INTO integration_test_users (name, email, age, status) VALUES
		('integration_Alice', 'alice@test.com', 25, 1),
		('integration_Bob', 'bob@test.com', 30, 1),
		('integration_Charlie', 'charlie@test.com', 28, 0)`)

	db.Exec(`INSERT INTO integration_test_orders (user_id, order_no, total_amount, status) VALUES
		((SELECT id FROM integration_test_users WHERE name='integration_Alice' LIMIT 1), 'INT001', 99.99, 'completed'),
		((SELECT id FROM integration_test_users WHERE name='integration_Alice' LIMIT 1), 'INT002', 149.50, 'pending'),
		((SELECT id FROM integration_test_users WHERE name='integration_Bob' LIMIT 1), 'INT003', 299.00, 'completed')`)
}

// ========================================
// SQL 执行工具集成测试
// ========================================

func TestSQLExecuteIntegration(t *testing.T) {
	db := getTestDB(t)
	setupIntegrationTestData(t, db)

	ctx := context.Background()

	t.Run("简单 SELECT 查询", func(t *testing.T) {
		req := &SQLExecuteRequest{
			SQL:     "SELECT * FROM integration_test_users",
			MaxRows: 10,
		}

		result, err := sqlExecute(ctx, req, db, nil, nil)
		if err != nil {
			t.Fatalf("sqlExecute() error = %v", err)
		}

		if result.RowCount < 3 {
			t.Errorf("expected at least 3 rows, got %d", result.RowCount)
		}

		// 检查列名
		expectedCols := []string{"id", "name", "email", "age", "status", "created_at"}
		for _, col := range expectedCols {
			found := false
			for _, c := range result.Columns {
				if c == col {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected column '%s' not found", col)
			}
		}
	})

	t.Run("带 WHERE 条件查询", func(t *testing.T) {
		req := &SQLExecuteRequest{
			SQL:     "SELECT * FROM integration_test_users WHERE status = 1",
			MaxRows: 10,
		}

		result, err := sqlExecute(ctx, req, db, nil, nil)
		if err != nil {
			t.Fatalf("sqlExecute() error = %v", err)
		}

		if result.RowCount < 2 {
			t.Errorf("expected at least 2 active users, got %d", result.RowCount)
		}
	})

	t.Run("JOIN 查询", func(t *testing.T) {
		req := &SQLExecuteRequest{
			SQL:     "SELECT u.name, o.order_no, o.total_amount FROM integration_test_users u JOIN integration_test_orders o ON u.id = o.user_id",
			MaxRows: 10,
		}

		result, err := sqlExecute(ctx, req, db, nil, nil)
		if err != nil {
			t.Fatalf("sqlExecute() error = %v", err)
		}

		if result.RowCount < 3 {
			t.Errorf("expected at least 3 order records, got %d", result.RowCount)
		}

		// 检查列名
		expectedCols := []string{"name", "order_no", "total_amount"}
		for _, col := range expectedCols {
			found := false
			for _, c := range result.Columns {
				if c == col {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected column '%s' not found", col)
			}
		}
	})

	t.Run("聚合查询", func(t *testing.T) {
		req := &SQLExecuteRequest{
			SQL:     "SELECT status, COUNT(*) as count FROM integration_test_users GROUP BY status",
			MaxRows: 10,
		}

		result, err := sqlExecute(ctx, req, db, nil, nil)
		if err != nil {
			t.Fatalf("sqlExecute() error = %v", err)
		}

		if result.RowCount == 0 {
			t.Error("expected at least 1 status group")
		}
	})

	t.Run("ORDER BY 查询", func(t *testing.T) {
		req := &SQLExecuteRequest{
			SQL:     "SELECT * FROM integration_test_users ORDER BY age DESC",
			MaxRows: 10,
		}

		result, err := sqlExecute(ctx, req, db, nil, nil)
		if err != nil {
			t.Fatalf("sqlExecute() error = %v", err)
		}

		if result.RowCount < 2 {
			t.Errorf("expected at least 2 rows, got %d", result.RowCount)
		}
	})

	t.Run("危险 SQL 被阻止", func(t *testing.T) {
		req := &SQLExecuteRequest{
			SQL:     "DELETE FROM integration_test_users",
			MaxRows: 10,
		}

		_, err := sqlExecute(ctx, req, db, nil, nil)
		if err == nil {
			t.Error("expected error for DELETE SQL, got nil")
		}
	})
}

// ========================================
// Schema 获取工具集成测试
// ========================================

func TestGetSchemaIntegration(t *testing.T) {
	db := getTestDB(t)
	setupIntegrationTestData(t, db)

	ctx := context.Background()

	t.Run("获取集成测试表结构", func(t *testing.T) {
		// database_id 留空走业务库路径（db 直连的即为目标库）。
		req := &GetSchemaRequest{
			TableName: "integration_test_users",
		}

		result, err := getSchema(ctx, req, db, nil, nil)
		if err != nil {
			t.Fatalf("getSchema() error = %v", err)
		}

		if len(result.Tables) != 1 {
			t.Fatalf("expected 1 table, got %d", len(result.Tables))
		}

		table := result.Tables[0]

		// 检查表名
		if table.TableName != "integration_test_users" {
			t.Errorf("expected table name 'integration_test_users', got '%s'", table.TableName)
		}

		// 检查主键
		if table.PrimaryKey != "id" {
			t.Errorf("expected primary key 'id', got '%s'", table.PrimaryKey)
		}

		// 检查列
		expectedColumns := []string{"id", "name", "email", "age", "status", "created_at"}
		if len(table.Columns) != len(expectedColumns) {
			t.Logf("expected %d columns, got %d", len(expectedColumns), len(table.Columns))
		}

		// 检查列名存在
		colMap := make(map[string]ColumnSchema)
		for _, col := range table.Columns {
			colMap[col.Name] = col
		}

		// 检查 name 列 NOT NULL
		nameCol := colMap["name"]
		if nameCol.Name == "" {
			t.Error("name column not found")
		} else {
			if nameCol.Nullable {
				t.Error("expected name column to be NOT NULL")
			}
		}

		// 检查注释
		if nameCol.Description != "用户名" {
			t.Logf("expected name column comment '用户名', got '%s'", nameCol.Description)
		}
	})

	t.Run("获取所有表（包含集成测试表）", func(t *testing.T) {
		req := &GetSchemaRequest{
			TableName: "",
		}

		result, err := getSchema(ctx, req, db, nil, nil)
		if err != nil {
			t.Fatalf("getSchema() error = %v", err)
		}

		// 应该至少有我们的集成测试表
		tableNames := make(map[string]bool)
		for _, table := range result.Tables {
			tableNames[table.TableName] = true
		}

		if !tableNames["integration_test_users"] {
			t.Error("expected table 'integration_test_users' not found")
		}
		if !tableNames["integration_test_orders"] {
			t.Error("expected table 'integration_test_orders' not found")
		}
	})
}

// ========================================
// 端到端场景测试
// ========================================

func TestEndToEndScenarios(t *testing.T) {
	db := getTestDB(t)
	setupIntegrationTestData(t, db)

	ctx := context.Background()

	t.Run("场景1：先获取Schema再执行查询", func(t *testing.T) {
		// 1. 获取表结构
		schemaReq := &GetSchemaRequest{
			TableName: "integration_test_users",
		}

		schema, err := getSchema(ctx, schemaReq, db, nil, nil)
		if err != nil {
			t.Fatalf("getSchema() error = %v", err)
		}

		if len(schema.Tables) == 0 {
			t.Fatal("expected at least 1 table")
		}

		// 2. 根据Schema执行查询
		table := schema.Tables[0]
		sqlQuery := fmt.Sprintf("SELECT %s FROM %s LIMIT 3", table.Columns[0].Name, table.TableName)

		queryReq := &SQLExecuteRequest{
			SQL:     sqlQuery,
			MaxRows: 10,
		}

		result, err := sqlExecute(ctx, queryReq, db, nil, nil)
		if err != nil {
			t.Fatalf("sqlExecute() error = %v", err)
		}

		if result.RowCount > 3 {
			t.Errorf("expected max 3 rows, got %d", result.RowCount)
		}
	})

	t.Run("场景2：复杂JOIN查询", func(t *testing.T) {
		sqlQuery := `
			SELECT
				u.name as user_name,
				COUNT(o.id) as order_count,
				COALESCE(SUM(o.total_amount), 0) as total_spent
			FROM integration_test_users u
			LEFT JOIN integration_test_orders o ON u.id = o.user_id
			WHERE u.name LIKE 'integration_%'
			GROUP BY u.id, u.name
			ORDER BY total_spent DESC
		`

		req := &SQLExecuteRequest{
			SQL:     sqlQuery,
			MaxRows: 10,
		}

		result, err := sqlExecute(ctx, req, db, nil, nil)
		if err != nil {
			t.Fatalf("sqlExecute() error = %v", err)
		}

		// 应该返回 3 个集成测试用户
		if result.RowCount < 3 {
			t.Errorf("expected at least 3 users, got %d", result.RowCount)
		}
	})
}
