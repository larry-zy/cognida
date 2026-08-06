package evaluation

import (
	"strings"
	"testing"
)

// TestValidateReadOnlySQL 覆盖只读校验：允许 SELECT/WITH，拒绝写/DDL/注释/多语句。
func TestValidateReadOnlySQL(t *testing.T) {
	cases := []struct {
		sql     string
		wantErr bool
	}{
		{"SELECT a FROM t", false},
		{"  select a from t  ", false},
		{"WITH x AS (SELECT 1) SELECT * FROM x", false},
		{"SELECT a FROM t;", false}, // 尾分号允许
		{"UPDATE t SET a=1", true},
		{"DELETE FROM t", true},
		{"DROP TABLE t", true},
		{"SELECT a FROM t; DROP TABLE t", true}, // 多语句
		{"SELECT a FROM t -- comment", true},    // 行注释
		{"SELECT a /* x */ FROM t", true},       // 块注释
		{"SHOW TABLES", true},
		{"EXPLAIN SELECT 1", true},
		{"", true},
	}
	for _, c := range cases {
		err := validateReadOnlySQL(c.sql)
		if (err != nil) != c.wantErr {
			t.Errorf("validateReadOnlySQL(%q) err=%v, wantErr=%v", c.sql, err, c.wantErr)
		}
	}
}

// TestEnsureReadOnlyLimit 覆盖 LIMIT 补全/收敛逻辑。
func TestEnsureReadOnlyLimit(t *testing.T) {
	// 无 LIMIT → 补上
	if got := ensureReadOnlyLimit("SELECT a FROM t", 1000); !strings.HasSuffix(got, "LIMIT 1000") {
		t.Errorf("ensureReadOnlyLimit 未补 LIMIT: %q", got)
	}
	// 尾分号 → 去掉再补
	if got := ensureReadOnlyLimit("SELECT a FROM t;", 1000); strings.Contains(got, ";") {
		t.Errorf("ensureReadOnlyLimit 未去尾分号: %q", got)
	}
	// 超上限 → 收敛到 maxRows
	if got := ensureReadOnlyLimit("SELECT a FROM t LIMIT 99999", 1000); !strings.Contains(got, "LIMIT 1000") {
		t.Errorf("ensureReadOnlyLimit 未收敛超限 LIMIT: %q", got)
	}
	// 未超上限 → 保留原值
	if got := ensureReadOnlyLimit("SELECT a FROM t LIMIT 10", 1000); !strings.Contains(got, "LIMIT 10") {
		t.Errorf("ensureReadOnlyLimit 误改合规 LIMIT: %q", got)
	}
}
