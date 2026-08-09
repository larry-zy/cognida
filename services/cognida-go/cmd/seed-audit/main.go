// 工具：给 audit_logs 表灌入一批真实感的审计样本数据（仅开发环境用于演示/验证审计页面）。
// 用法：cd cognida-go && set -a && source .env && set +a && go run ./cmd/seed-audit [数量]
// 默认 180 条，全部归属 tenant_id=1 / user_id=1（dev 用户），时间均匀散布在最近 7 天内。
// 幂等性：每次运行都新增数据（审计日志本就是追加型），需要清空可 TRUNCATE。
package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	mrand "math/rand"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	mysqlrepo "cognida/internal/repository/mysql"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// uuidv4 生成与 TraceMiddleware 同格式的 request_id。
func uuidv4() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func ptr[T any](v T) *T { return &v }

// route 描述一个可被采样的接口：模块 + 方法 + 路由模板 + 资源类型 + 权重。
type route struct {
	module   string
	method   string
	tmpl     string
	resType  string
	weight   int
}

// 覆盖各业务模块的常见接口，权重贴近真实流量（读多写少，agent/知识库偏热）。
var routes = []route{
	{"sessions", "GET", "/api/v1/sessions", "session", 14},
	{"sessions", "POST", "/api/v1/sessions", "session", 6},
	{"sessions", "GET", "/api/v1/sessions/:id/messages", "message", 10},
	{"sessions", "DELETE", "/api/v1/sessions/:id", "session", 3},
	{"agent", "POST", "/api/v1/agent/text2sql/stream", "agent", 12},
	{"agent", "GET", "/api/v1/agent", "agent", 8},
	{"agent", "POST", "/api/v1/agent", "agent", 3},
	{"knowledge-bases", "GET", "/api/v1/knowledge-bases", "knowledge_base", 10},
	{"knowledge-bases", "POST", "/api/v1/knowledge-bases/:id/documents", "document", 6},
	{"knowledge-bases", "GET", "/api/v1/knowledge-bases/:id", "knowledge_base", 7},
	{"knowledge-bases", "POST", "/api/v1/knowledge-bases/:id/retrieve", "chunk", 8},
	{"graphs", "GET", "/api/v1/graphs/:kbId/stats", "graph", 5},
	{"graphs", "GET", "/api/v1/graphs/:kbId/nodes/:nodeId", "graph_node", 6},
	{"evaluation", "GET", "/api/v1/evaluation/tasks", "evaluation_task", 6},
	{"evaluation", "POST", "/api/v1/evaluation/tasks", "evaluation_task", 3},
	{"evaluation", "GET", "/api/v1/evaluation/tasks/:id", "evaluation_task", 5},
	{"datasets", "GET", "/api/v1/datasets", "dataset", 4},
	{"quality", "POST", "/api/v1/quality/checks", "quality_check", 4},
	{"quality", "GET", "/api/v1/quality/records", "quality_check", 3},
	{"datasources", "GET", "/api/v1/datasources", "datasource", 6},
	{"datasources", "POST", "/api/v1/datasources/test", "datasource", 3},
	{"datasources", "GET", "/api/v1/datasources/:id/tables", "datasource", 4},
	{"user", "GET", "/api/v1/user/profile", "user", 4},
	{"audit-logs", "GET", "/api/v1/audit-logs", "audit_log", 5},
	{"audit-logs", "GET", "/api/v1/audit-logs/stats", "audit_log", 3},
}

// 各方法的典型耗时区间（毫秒），流式/写操作更慢。
func sampleDuration(r route) int {
	switch {
	case r.tmpl == "/api/v1/agent/text2sql/stream":
		return 3000 + mrand.Intn(12000)
	case r.method == "POST" && r.resType == "document":
		return 400 + mrand.Intn(3000)
	case r.method == "POST":
		return 30 + mrand.Intn(600)
	default:
		return mrand.Intn(40)
	}
}

var ips = []string{"::1", "127.0.0.1", "192.168.1.24", "192.168.1.108", "10.0.3.17"}

var errMsgs = map[int]string{
	400: "invalid request payload",
	401: "unauthorized",
	404: "resource not found",
	500: "internal server error",
	503: "upstream service unavailable",
}

func main() {
	n := 180
	if len(os.Args) > 1 {
		if v, err := strconv.Atoi(os.Args[1]); err == nil && v > 0 {
			n = v
		}
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		env("DB_USER", "root"), env("DB_PASSWORD", ""),
		env("DB_HOST", "127.0.0.1"), env("DB_PORT", "3306"), env("DB_NAME", "cognida"))

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("connect failed: %v", err)
	}

	// 加权路由展开成采样池。
	var pool []route
	for _, r := range routes {
		for i := 0; i < r.weight; i++ {
			pool = append(pool, r)
		}
	}

	const tenantID, userID = int64(1), int64(1)
	now := time.Now()
	rows := make([]*mysqlrepo.AuditLogModel, 0, n)

	for i := 0; i < n; i++ {
		r := pool[mrand.Intn(len(pool))]

		// 时间：最近 7 天内均匀散布，偏向白天（8:00-22:00）。
		daysAgo := mrand.Intn(7)
		hour := 8 + mrand.Intn(14)
		created := now.AddDate(0, 0, -daysAgo).
			Add(time.Duration(hour-now.Hour()) * time.Hour).
			Add(time.Duration(mrand.Intn(60)) * time.Minute).
			Add(time.Duration(mrand.Intn(60)) * time.Second)

		// 具体路径：把 :id/:kbId/:nodeId 占位替换为拟真 ID（写进 details.path）。
		path := realizePath(r.tmpl)
		durationMs := sampleDuration(r)
		ip := ips[mrand.Intn(len(ips))]
		rid := uuidv4()

		// ~10% 失败，按方法给合理的错误码。
		httpStatus := 200
		status := "success"
		var errMsg *string
		if mrand.Intn(100) < 10 {
			codes := []int{400, 404, 500, 503}
			httpStatus = codes[mrand.Intn(len(codes))]
			status = "error"
			errMsg = ptr(errMsgs[httpStatus])
		}

		details, _ := json.Marshal(map[string]interface{}{
			"http_status": httpStatus,
			"method":      r.method,
			"path":        path,
		})

		rows = append(rows, &mysqlrepo.AuditLogModel{
			TenantID:     ptr(tenantID),
			UserID:       ptr(userID),
			Module:       r.module,
			Action:       r.method + " " + r.tmpl,
			ResourceType: r.resType,
			Details:      string(details),
			Status:       status,
			ErrorMessage: errMsg,
			IPAddress:    ptr(ip),
			RequestID:    ptr(rid),
			DurationMs:   ptr(durationMs),
			CreatedAt:    created,
		})
	}

	// 批量写入（分批避免超长 SQL）。
	if err := db.CreateInBatches(rows, 100).Error; err != nil {
		log.Fatalf("insert failed: %v", err)
	}

	fmt.Printf("OK: seeded %d audit rows (tenant_id=1, spread over last 7 days)\n", len(rows))
}

// realizePath 把路由模板里的占位符替换成拟真 ID。
func realizePath(tmpl string) string {
	replace := func(s, ph string) string {
		id := fmt.Sprintf("%d%x", 20260700+mrand.Intn(99), mustHex(4))
		return replaceFirst(s, ph, id)
	}
	out := tmpl
	for _, ph := range []string{":id", ":kbId", ":nodeId"} {
		for containsPlaceholder(out, ph) {
			out = replace(out, ph)
		}
	}
	return out
}

func mustHex(nbytes int) []byte {
	b := make([]byte, nbytes)
	_, _ = rand.Read(b)
	return b
}

func containsPlaceholder(s, ph string) bool {
	return indexOf(s, ph) >= 0
}

func replaceFirst(s, old, new string) string {
	i := indexOf(s, old)
	if i < 0 {
		return s
	}
	return s[:i] + new + s[i+len(old):]
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
