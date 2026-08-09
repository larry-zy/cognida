package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/go-sql-driver/mysql"

	"cognida/internal/service/agent/framework"
)

// TestClassifySQLError_ByCodeAndText 校验按 MySQL 错误码/文本的分级与肇事标识符抽取。
func TestClassifySQLError_ByCodeAndText(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantKind   sqlErrorKind
		wantIdent  string
	}{
		{"未知列", &mysql.MySQLError{Number: 1054, Message: "Unknown column 'user_name' in 'field list'"}, sqlErrUnknownColumn, "user_name"},
		{"未知表", &mysql.MySQLError{Number: 1146, Message: "Table 'link.ordrs' doesn't exist"}, sqlErrUnknownTable, "ordrs"},
		{"语法错误", &mysql.MySQLError{Number: 1064, Message: "You have an error in your SQL syntax"}, sqlErrSyntax, ""},
		{"权限不足_1044", &mysql.MySQLError{Number: 1044, Message: "Access denied"}, sqlErrPermission, ""},
		{"权限不足_1142", &mysql.MySQLError{Number: 1142, Message: "SELECT command denied"}, sqlErrPermission, ""},
		{"死锁", &mysql.MySQLError{Number: 1213, Message: "Deadlock found"}, sqlErrTransient, ""},
		{"锁等待", &mysql.MySQLError{Number: 1205, Message: "Lock wait timeout exceeded"}, sqlErrTransient, ""},
		{"上下文超时", context.DeadlineExceeded, sqlErrTimeout, ""},
		{"连接重置", errors.New("read: connection reset by peer"), sqlErrTransient, ""},
		{"其它", errors.New("something odd"), sqlErrOther, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, ident := classifySQLError(tc.err)
			if kind != tc.wantKind {
				t.Fatalf("kind=%q, want %q", kind, tc.wantKind)
			}
			if ident != tc.wantIdent {
				t.Fatalf("identifier=%q, want %q", ident, tc.wantIdent)
			}
		})
	}
}

// TestRetriableByKind 校验 retriable 标记：可定向改写者为 true，权限/其它为 false。
func TestRetriableByKind(t *testing.T) {
	retriable := []sqlErrorKind{sqlErrUnknownColumn, sqlErrUnknownTable, sqlErrSyntax, sqlErrTimeout, sqlErrTransient}
	for _, k := range retriable {
		if !k.retriable() {
			t.Fatalf("%q 应可重试", k)
		}
	}
	for _, k := range []sqlErrorKind{sqlErrPermission, sqlErrOther} {
		if k.retriable() {
			t.Fatalf("%q 不应可重试", k)
		}
	}
}

// TestNewRepairableSQLError_ObservationShape 校验可修复观察 JSON 结构与 RepairableToolError 载体。
func TestNewRepairableSQLError_ObservationShape(t *testing.T) {
	// target=nil → 走通用提示分支，不检索 schema。
	re := newRepairableSQLError(context.Background(), nil, "SELECT bad FROM orders",
		&mysql.MySQLError{Number: 1054, Message: "Unknown column 'bad' in 'field list'"}, nil, "")

	if re.ErrorKind != string(sqlErrUnknownColumn) {
		t.Fatalf("ErrorKind=%q, want unknown_column", re.ErrorKind)
	}
	// Error() 即观察本体，可被框架识别为可修复。
	if _, ok := framework.AsRepairable(re); !ok {
		t.Fatal("应可被 framework.AsRepairable 识别")
	}

	var obs struct {
		ErrorKind string          `json:"error_kind"`
		Retriable bool            `json:"retriable"`
		Hint      json.RawMessage `json:"hint"`
		Detail    string          `json:"detail"`
	}
	if err := json.Unmarshal([]byte(re.Observation), &obs); err != nil {
		t.Fatalf("观察非合法 JSON: %v (%s)", err, re.Observation)
	}
	if obs.ErrorKind != "unknown_column" || !obs.Retriable {
		t.Fatalf("观察字段异常: %+v", obs)
	}
	if obs.Detail == "" {
		t.Fatal("内部库应带脱敏 detail")
	}
}

// TestNewRepairableSQLError_ExternalRedaction 校验外部数据源失败仅给通用摘要，不透传底层报文。
func TestNewRepairableSQLError_ExternalRedaction(t *testing.T) {
	target := &queryTarget{external: true, dbName: "remote"}
	raw := "Access denied for user 'admin'@'10.0.0.5' (using password: YES)"
	re := newRepairableSQLError(context.Background(), target, "SELECT * FROM t",
		&mysql.MySQLError{Number: 1044, Message: raw}, nil, "")

	if re.ErrorKind != string(sqlErrPermission) {
		t.Fatalf("ErrorKind=%q, want permission", re.ErrorKind)
	}
	var obs struct {
		Retriable bool   `json:"retriable"`
		Detail    string `json:"detail"`
	}
	if err := json.Unmarshal([]byte(re.Observation), &obs); err != nil {
		t.Fatalf("观察非合法 JSON: %v", err)
	}
	if obs.Retriable {
		t.Fatal("权限错误不应标记可重试")
	}
	if obs.Detail != "外部数据源拒绝该操作（权限不足）" {
		t.Fatalf("detail 未脱敏: %q", obs.Detail)
	}
	// 底层账号/主机绝不出现在整段观察里。
	for _, leak := range []string{"admin", "10.0.0.5", "password"} {
		if contains(re.Observation, leak) {
			t.Fatalf("观察泄露底层信息 %q: %s", leak, re.Observation)
		}
	}
}

// TestBuildSchemaHint_NilDBDegrades 校验 schema 检索失败（此处 db 为 nil 触发 panic）时降级为通用提示，绝不 panic 外溢。
func TestBuildSchemaHint_NilDBDegrades(t *testing.T) {
	target := &queryTarget{external: false, dbName: "link"} // db 为 nil
	// 不应 panic；应退化为 tip-only。
	hint := buildSchemaHint(context.Background(), target, sqlErrUnknownTable, "ordrs", "SELECT 1 FROM ordrs", nil, "")
	m, ok := hint.(map[string]interface{})
	if !ok {
		t.Fatalf("hint 类型异常: %T", hint)
	}
	if _, hasTip := m["tip"]; !hasTip {
		t.Fatalf("降级 hint 应含 tip: %+v", m)
	}
}

// TestReferencedTables 校验从 SQL 粗取 FROM/JOIN 表名（去重保序）。
func TestReferencedTables(t *testing.T) {
	got := referencedTables("SELECT a.* FROM orders a JOIN `order_items` i ON a.id=i.oid JOIN orders b")
	want := []string{"orders", "order_items"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

// TestRankNamesByProximity 校验候选表名按近似度排序（相等>包含>共享前缀）。
func TestRankNamesByProximity(t *testing.T) {
	names := []string{"users", "user_profile", "orders", "user"}
	got := rankNamesByProximity(names, "user", 3)
	if len(got) != 3 {
		t.Fatalf("应截断到 3，got %v", got)
	}
	if got[0] != "user" {
		t.Fatalf("精确匹配应排首位，got %v", got)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
