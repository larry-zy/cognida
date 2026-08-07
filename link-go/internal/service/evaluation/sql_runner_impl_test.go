package evaluation

import (
	"strings"
	"testing"
)

// TestValidateReadOnlySQL 覆盖只读校验：白名单首关键字放行只读语句（SELECT/WITH/SHOW/DESC/
// DESCRIBE/EXPLAIN），黑名单拦真实写/DDL；字面量内的关键字/分号/注释不再误杀合法金标准 SQL。
func TestValidateReadOnlySQL(t *testing.T) {
	cases := []struct {
		name    string
		sql     string
		wantErr bool
	}{
		// —— 合法只读语句放行 ——
		{"select", "SELECT a FROM t", false},
		{"select lowercase padded", "  select a from t  ", false},
		{"with cte", "WITH x AS (SELECT 1) SELECT * FROM x", false},
		{"trailing semicolon", "SELECT a FROM t;", false}, // 尾分号允许
		{"show tables", "SHOW TABLES", false},             // SHOW 只读，白名单放行
		{"explain select", "EXPLAIN SELECT 1", false},     // EXPLAIN 只读，白名单放行
		{"describe", "DESCRIBE t", false},
		{"desc", "DESC t", false},
		{"select paren first", "(SELECT a FROM t)", false}, // 括号包裹的子查询也放行

		// —— B5 误杀修复：字面量里的关键字/分号/注释不应误判 ——
		{"literal keyword delete", "SELECT id FROM orders WHERE status='delete'", false},
		{"literal semicolon", "SELECT id FROM t WHERE remark='a;b'", false},
		{"literal comment dashes", "SELECT id FROM t WHERE note='x--y'", false},
		{"literal block comment", "SELECT id FROM t WHERE note='a/*b*/c'", false},
		{"literal keyword update dquote", `SELECT id FROM t WHERE tag="update now"`, false},
		{"literal drop word", "SELECT id FROM t WHERE action='please drop it'", false},
		{"escaped quote in literal", "SELECT id FROM t WHERE name='O''Brien;drop'", false},

		// —— 真实写/DDL 仍被拦 ——
		{"update", "UPDATE t SET a=1", true},
		{"delete", "DELETE FROM t", true},
		{"drop", "DROP TABLE t", true},
		{"insert", "INSERT INTO t VALUES (1)", true},
		{"truncate", "TRUNCATE TABLE t", true},
		{"alter", "ALTER TABLE t ADD c INT", true},
		{"create", "CREATE TABLE t (a INT)", true},
		{"call proc", "CALL do_something()", true},
		{"grant", "GRANT SELECT ON t TO u", true},
		// 白名单开头但内含真实写操作也拦（黑名单兜底）
		{"explain delete", "EXPLAIN DELETE FROM t", true},
		{"select then drop", "SELECT a FROM t; DROP TABLE t", true}, // 多语句 + 写
		{"multi statement select", "SELECT a FROM t; SELECT b FROM t2", true},
		{"line comment", "SELECT a FROM t -- comment", true},
		{"block comment", "SELECT a /* x */ FROM t", true},
		{"empty", "", true},
		{"whitespace only", "   ", true},
		// 非白名单开头（即使无黑名单词）也拒绝
		{"leading garbage", "FOO SELECT a FROM t", true},
		// SELECT ... INTO OUTFILE/DUMPFILE 以 SELECT 开头骗过白名单，却是写磁盘的写操作，必须拦
		{"select into outfile", "SELECT * FROM users INTO OUTFILE '/tmp/pwn.csv'", true},
		{"select into dumpfile", "SELECT a FROM t INTO DUMPFILE '/tmp/x'", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateReadOnlySQL(c.sql)
			if (err != nil) != c.wantErr {
				t.Errorf("validateReadOnlySQL(%q) err=%v, wantErr=%v", c.sql, err, c.wantErr)
			}
		})
	}
}

// TestStripSQLStringLiterals 验证字面量剥离：引号内内容整体丢弃，含 '' / "" 与反斜杠转义。
func TestStripSQLStringLiterals(t *testing.T) {
	cases := []struct {
		in       string
		contains string // 剥离后不应再包含的子串
	}{
		{"WHERE status='delete'", "delete"},
		{"WHERE remark='a;b'", ";"},
		{"WHERE note='x--y'", "--"},
		{`WHERE tag="update"`, "update"},
		{"WHERE name='O''Brien;'", ";"},
	}
	for _, c := range cases {
		got := stripSQLStringLiterals(c.in)
		if strings.Contains(strings.ToLower(got), strings.ToLower(c.contains)) {
			t.Errorf("stripSQLStringLiterals(%q)=%q 仍含字面量内容 %q", c.in, got, c.contains)
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
	// WITH 行式查询 → 补 LIMIT
	if got := ensureReadOnlyLimit("WITH x AS (SELECT 1) SELECT * FROM x", 1000); !strings.HasSuffix(got, "LIMIT 1000") {
		t.Errorf("ensureReadOnlyLimit 未给 WITH 补 LIMIT: %q", got)
	}
	// SHOW/DESCRIBE/EXPLAIN 语法不接 LIMIT → 原样返回，避免拼出非法 SQL
	for _, s := range []string{"SHOW TABLES", "DESCRIBE orders", "DESC orders", "EXPLAIN SELECT 1"} {
		if got := ensureReadOnlyLimit(s, 1000); got != s {
			t.Errorf("ensureReadOnlyLimit(%q) 误加 LIMIT: %q", s, got)
		}
	}
}
