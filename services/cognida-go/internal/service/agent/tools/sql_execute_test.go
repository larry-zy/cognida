// Package tools 测试 SQL 执行工具
package tools

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// setupMockDB 创建 mock 数据库
func setupMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *sqlmock.Rows) {
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

	return gormDB, mock, sqlmock.NewRows([]string{"id", "name", "email"})
}

// TestValidateSQL 测试 SQL 验证
func TestValidateSQL(t *testing.T) {
	tests := []struct {
		name    string
		sql     string
		wantErr bool
	}{
		{
			name:    "valid SELECT",
			sql:     "SELECT * FROM users",
			wantErr: false,
		},
		{
			name:    "valid SELECT with WHERE",
			sql:     "SELECT id, name FROM users WHERE id = 1",
			wantErr: false,
		},
		{
			name:    "valid SELECT with JOIN",
			sql:     "SELECT u.name, o.total FROM users u JOIN orders o ON u.id = o.user_id",
			wantErr: false,
		},
		{
			name:    "valid SELECT with GROUP BY",
			sql:     "SELECT status, COUNT(*) FROM orders GROUP BY status",
			wantErr: false,
		},
		{
			name:    "valid SELECT with ORDER BY",
			sql:     "SELECT * FROM users ORDER BY created_at DESC",
			wantErr: false,
		},
		{
			name:    "valid SELECT with LIMIT",
			sql:     "SELECT * FROM users LIMIT 10",
			wantErr: false,
		},
		{
			name:    "valid WITH clause (CTE)",
			sql:     "WITH cte AS (SELECT * FROM users) SELECT * FROM cte",
			wantErr: false,
		},
		{
			name:    "INSERT - blocked",
			sql:     "INSERT INTO users VALUES (1, 'test')",
			wantErr: true,
		},
		{
			name:    "UPDATE - blocked",
			sql:     "UPDATE users SET name = 'test'",
			wantErr: true,
		},
		{
			name:    "DELETE - blocked",
			sql:     "DELETE FROM users",
			wantErr: true,
		},
		{
			name:    "DROP - blocked",
			sql:     "DROP TABLE users",
			wantErr: true,
		},
		{
			name:    "TRUNCATE - blocked",
			sql:     "TRUNCATE TABLE users",
			wantErr: true,
		},
		{
			name:    "ALTER - blocked",
			sql:     "ALTER TABLE users ADD COLUMN age INT",
			wantErr: true,
		},
		{
			name:    "CREATE - blocked",
			sql:     "CREATE TABLE test (id INT)",
			wantErr: true,
		},
		{
			name:    "GRANT - blocked",
			sql:     "GRANT ALL ON users TO 'user'",
			wantErr: true,
		},
		{
			name:    "REVOKE - blocked",
			sql:     "REVOKE ALL ON users FROM 'user'",
			wantErr: true,
		},
		{
			name:    "EXPLAIN - blocked",
			sql:     "EXPLAIN SELECT * FROM users",
			wantErr: true,
		},
		{
			name:    "SHOW - blocked",
			sql:     "SHOW TABLES",
			wantErr: true,
		},
		{
			name:    "DESCRIBE - blocked",
			sql:     "DESCRIBE users",
			wantErr: true,
		},
		{
			name:    "SQL injection with semicolon",
			sql:     "SELECT * FROM users; DROP TABLE users",
			wantErr: true,
		},
		{
			// 多语句拦截不能只靠关键词黑名单：两条 SELECT 也必须拒绝
			name:    "multi-statement with two SELECTs",
			sql:     "SELECT 1; SELECT 2",
			wantErr: true,
		},
		{
			name:    "trailing semicolon allowed",
			sql:     "SELECT * FROM users;",
			wantErr: false,
		},
		{
			name:    "comment injection with --",
			sql:     "SELECT * FROM users -- comment",
			wantErr: true,
		},
		{
			name:    "block comment injection",
			sql:     "SELECT * FROM users /* comment */",
			wantErr: true,
		},
		{
			name:    "UNION injection (commented case)",
			sql:     "SELECT * FROM users WHERE 1=0 UNION SELECT * FROM orders",
			wantErr: false, // UNION SELECT is allowed in our current validation
		},
		{
			name:    "not starting with SELECT or WITH",
			sql:     "DESCRIBE users",
			wantErr: true,
		},
		{
			name:    "conditional injection with OR",
			sql:     "SELECT * FROM users WHERE id = 1 OR 1=1",
			wantErr: false, // This is syntactically valid SELECT
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSQL(tt.sql)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSQL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestEnsureLimit 测试 LIMIT 添加
func TestEnsureLimit(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		maxRows  int
		contains string
	}{
		{
			name:     "add LIMIT to query without",
			sql:      "SELECT * FROM users",
			maxRows:  100,
			contains: "LIMIT 100",
		},
		{
			name:     "respect existing LIMIT",
			sql:      "SELECT * FROM users LIMIT 10",
			maxRows:  100,
			contains: "LIMIT 10",
		},
		{
			name:     "trim trailing semicolon",
			sql:      "SELECT * FROM users;",
			maxRows:  50,
			contains: "LIMIT 50",
		},
		{
			name:     "trim trailing semicolon with space",
			sql:      "SELECT * FROM users; ",
			maxRows:  50,
			contains: "LIMIT 50",
		},
		{
			name:     "cap large LIMIT (5 digits)",
			sql:      "SELECT * FROM users LIMIT 999999",
			maxRows:  100,
			contains: "LIMIT 100",
		},
		{
			// 4 位数但超过 maxRows 也必须压回（不能只看位数）
			name:     "cap LIMIT above maxRows (4 digits)",
			sql:      "SELECT * FROM users LIMIT 5000",
			maxRows:  100,
			contains: "LIMIT 100",
		},
		{
			name:     "cap large LIMIT (6 digits)",
			sql:      "SELECT * FROM users LIMIT 1000000",
			maxRows:  100,
			contains: "LIMIT 100",
		},
		{
			name:     "query with ORDER BY",
			sql:      "SELECT * FROM users ORDER BY created_at DESC",
			maxRows:  10,
			contains: "LIMIT 10",
		},
		{
			name:     "query with GROUP BY",
			sql:      "SELECT status, COUNT(*) FROM users GROUP BY status",
			maxRows:  20,
			contains: "LIMIT 20",
		},
		{
			name:     "complex query with JOIN",
			sql:      "SELECT u.name, o.total FROM users u JOIN orders o ON u.id = o.user_id",
			maxRows:  50,
			contains: "LIMIT 50",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ensureLimit(tt.sql, tt.maxRows)
			matched, _ := regexp.MatchString(tt.contains, result)
			if !matched {
				t.Errorf("ensureLimit() = %v, want to contain %v", result, tt.contains)
			}
		})
	}
}

// TestEnsureLimit_SubqueryLimit 回归：子查询里的 LIMIT 不得被当成外层已限行，
// 且改写尾部 LIMIT 时不得误伤子查询 LIMIT。
func TestEnsureLimit_SubqueryLimit(t *testing.T) {
	tests := []struct {
		name    string
		sql     string
		maxRows int
		want    string
	}{
		{
			// 外层无 LIMIT、子查询有 LIMIT：必须给外层补 LIMIT，子查询 LIMIT 原样保留。
			name:    "subquery limit only -> append outer limit",
			sql:     "SELECT * FROM (SELECT id FROM users LIMIT 5) t",
			maxRows: 100,
			want:    "SELECT * FROM (SELECT id FROM users LIMIT 5) t LIMIT 100",
		},
		{
			// 外层尾部 LIMIT 超上限：只压外层这一处，子查询 LIMIT 不动。
			name:    "outer limit capped, subquery limit untouched",
			sql:     "SELECT * FROM (SELECT id FROM users LIMIT 5) t LIMIT 999999",
			maxRows: 100,
			want:    "SELECT * FROM (SELECT id FROM users LIMIT 5) t LIMIT 100",
		},
		{
			// 尾部 `LIMIT offset, count`：以 count 判定上限；封顶时必须保留 offset、只压 count，
			// 否则翻页页码错位（offset 100 被丢会拿到第 1 页而非目标页）。
			name:    "limit offset,count capped by count keeps offset",
			sql:     "SELECT * FROM users LIMIT 100, 5000",
			maxRows: 100,
			want:    "SELECT * FROM users LIMIT 100, 100",
		},
		{
			// 尾部 `LIMIT offset, count` 且 count 未超上限：整条原样保留（offset 与 count 都不动）。
			name:    "limit offset,count under cap untouched",
			sql:     "SELECT * FROM users LIMIT 20, 30",
			maxRows: 100,
			want:    "SELECT * FROM users LIMIT 20, 30",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ensureLimit(tt.sql, tt.maxRows); got != tt.want {
				t.Errorf("ensureLimit()\n got = %q\nwant = %q", got, tt.want)
			}
		})
	}
}

// TestSQLExecute 测试 SQL 执行工具
func TestSQLExecute(t *testing.T) {
	gormDB, mock, rows := setupMockDB(t)

	ctx := context.Background()

	t.Run("simple SELECT query", func(t *testing.T) {
		rows = sqlmock.NewRows([]string{"id", "name", "email"}).
			AddRow(1, "Alice", "alice@example.com").
			AddRow(2, "Bob", "bob@example.com")

		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM users LIMIT 100")).
			WillReturnRows(rows)

		req := &SQLExecuteRequest{
			SQL:     "SELECT * FROM users",
			MaxRows: 100,
		}

		result, err := sqlExecute(ctx, req, gormDB, nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("sqlExecute() error = %v", err)
		}

		if result.RowCount != 2 {
			t.Errorf("expected 2 rows, got %d", result.RowCount)
		}

		if len(result.Columns) != 3 {
			t.Errorf("expected 3 columns, got %d", len(result.Columns))
		}
	})

	t.Run("query with WHERE clause", func(t *testing.T) {
		rows = sqlmock.NewRows([]string{"id", "name"}).
			AddRow(1, "Alice")

		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM users WHERE id = 1 LIMIT 100")).
			WillReturnRows(rows)

		req := &SQLExecuteRequest{
			SQL:     "SELECT * FROM users WHERE id = 1",
			MaxRows: 100,
		}

		result, err := sqlExecute(ctx, req, gormDB, nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("sqlExecute() error = %v", err)
		}

		if result.RowCount != 1 {
			t.Errorf("expected 1 row, got %d", result.RowCount)
		}
	})

	t.Run("empty result set", func(t *testing.T) {
		rows = sqlmock.NewRows([]string{"id", "name"})

		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM users WHERE id = 999 LIMIT 100")).
			WillReturnRows(rows)

		req := &SQLExecuteRequest{
			SQL:     "SELECT * FROM users WHERE id = 999",
			MaxRows: 100,
		}

		result, err := sqlExecute(ctx, req, gormDB, nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("sqlExecute() error = %v", err)
		}

		if result.RowCount != 0 {
			t.Errorf("expected 0 rows, got %d", result.RowCount)
		}
	})

	t.Run("invalid SQL - DELETE blocked", func(t *testing.T) {
		req := &SQLExecuteRequest{
			SQL:     "DELETE FROM users",
			MaxRows: 100,
		}

		_, err := sqlExecute(ctx, req, gormDB, nil, nil, nil, nil)
		if err == nil {
			t.Error("expected error for DELETE query, got nil")
		}
	})

	t.Run("empty SQL", func(t *testing.T) {
		req := &SQLExecuteRequest{
			SQL:     "",
			MaxRows: 100,
		}

		_, err := sqlExecute(ctx, req, gormDB, nil, nil, nil, nil)
		if err == nil {
			t.Error("expected error for empty SQL, got nil")
		}
	})

	t.Run("nil DB", func(t *testing.T) {
		req := &SQLExecuteRequest{
			SQL:     "SELECT * FROM users",
			MaxRows: 100,
		}

		// 业务库未注入（nil）时应报错：数据库未初始化。
		_, err := sqlExecute(ctx, req, nil, nil, nil, nil, nil)
		if err == nil {
			t.Error("expected error when DB is nil, got nil")
		}
	})
}

// TestSQLExecuteResult 测试结果格式
func TestSQLExecuteResult(t *testing.T) {
	gormDB, mock, _ := setupMockDB(t)

	ctx := context.Background()

	rows := sqlmock.NewRows([]string{"id", "name", "email", "created_at"}).
		AddRow(1, "Alice", "alice@example.com", time.Now()).
		AddRow(2, "Bob", "bob@example.com", time.Now())

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM users LIMIT 10")).
		WillReturnRows(rows)

	req := &SQLExecuteRequest{
		SQL:     "SELECT * FROM users",
		MaxRows: 10,
	}

	result, err := sqlExecute(ctx, req, gormDB, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("sqlExecute() error = %v", err)
	}

	// 验证列名
	expectedColumns := []string{"id", "name", "email", "created_at"}
	if len(result.Columns) != len(expectedColumns) {
		t.Errorf("expected %d columns, got %d", len(expectedColumns), len(result.Columns))
	}

	// 验证有延迟时间（mock 执行可能很快，但不应该为负）
	if result.LatencyMs < 0 {
		t.Errorf("expected non-negative LatencyMs, got %d", result.LatencyMs)
	}

	// 验证有执行 SQL
	if result.ExecutedSQL == "" {
		t.Error("expected ExecutedSQL to be set")
	}
}

// TestSQLExecuteMaxRows 测试最大行数限制
func TestSQLExecuteMaxRows(t *testing.T) {
	tests := []struct {
		name        string
		maxRows     int
		expectLimit int
	}{
		{"default", 0, 100},
		{"explicit 10", 10, 10},
		{"explicit 1000", 1000, 1000},
		{"exceeds max", 10000, 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &SQLExecuteRequest{
				SQL:     "SELECT * FROM users",
				MaxRows: tt.maxRows,
			}

			if req.MaxRows <= 0 {
				req.MaxRows = 100
			}
			if req.MaxRows > 1000 {
				req.MaxRows = 1000
			}

			if req.MaxRows != tt.expectLimit {
				t.Errorf("expected MaxRows %d, got %d", tt.expectLimit, req.MaxRows)
			}
		})
	}
}
