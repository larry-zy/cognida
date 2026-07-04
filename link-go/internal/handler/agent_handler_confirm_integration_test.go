//go:build integration

// confirm-resume 端点集成测试（真实 MySQL）：
// 确认后事务真实提交落库 + 审计留痕；token 不匹配拒绝且立即失效、库不被改。
//
// 通过 MYSQL_DSN 提供 DSN，例如：
//
//	MYSQL_DSN='root:password@tcp(localhost:3306)/link?charset=utf8mb4&parseTime=True&loc=Local' \
//	go test -tags=integration ./internal/handler/ -run TestConfirmIntegration -v
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"link/internal/model/agent/operations"
	mysqlrepo "link/internal/repository/mysql"
	"link/internal/service/agent/pendingaction"
	ragtool "link/internal/service/agent/tools"
)

// setupConfirmIntegration 连接真实 DB，预置源表与操作工具配置。
func setupConfirmIntegration(t *testing.T) (*gorm.DB, *pendingaction.MemoryStore) {
	t.Helper()
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Skip("MYSQL_DSN not set; skipping confirm endpoint integration test")
	}
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := db.AutoMigrate(&mysqlrepo.OperationAuditModel{}); err != nil {
		t.Fatalf("automigrate audit table: %v", err)
	}

	source := "it_confirm_src"
	db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS `%s`", source))
	if err := db.Exec(fmt.Sprintf(
		"CREATE TABLE `%s` (id INT PRIMARY KEY, gmv DOUBLE)", source)).Error; err != nil {
		t.Fatalf("create source table: %v", err)
	}
	if err := db.Exec(fmt.Sprintf(
		"INSERT INTO `%s` (id, gmv) VALUES (1,100),(2,200),(3,300)", source)).Error; err != nil {
		t.Fatalf("seed source table: %v", err)
	}
	t.Cleanup(func() { db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS `%s`", source)) })

	pending := pendingaction.NewMemoryStore()
	ragtool.InitOperationTools(ragtool.OperationConfig{
		DB:      db,
		Audit:   mysqlrepo.NewOperationAuditRepository(db),
		Pending: pending,
	})
	return db, pending
}

// callConfirmIT 以 gin 测试上下文调用 ConfirmOperation（tenant_id 固定 991）。
func callConfirmIT(t *testing.T, payload map[string]interface{}) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	body, _ := json.Marshal(payload)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("tenant_id", int64(991))
	c.Request = httptest.NewRequest(http.MethodPost, "/agent/operations/confirm", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	(&AgentHandler{}).ConfirmOperation(c)

	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp.Data
}

// putConfirmITAction 预置归属 991:sess-cf-it 的待确认写操作。
func putConfirmITAction(t *testing.T, pending *pendingaction.MemoryStore, key string) *pendingaction.PendingAction {
	t.Helper()
	action := &pendingaction.PendingAction{
		Owner: "991:sess-cf-it",
		Kind:  operations.OpMutate,
		SQL:   "UPDATE it_confirm_src SET gmv = gmv * 2",
		Params: map[string]interface{}{
			"idempotency_key": key,
			"target":          "it_confirm_src",
			"rows_affected":   int64(3),
		},
	}
	if _, err := pending.Put(context.Background(), action, pendingaction.DefaultTTL); err != nil {
		t.Fatalf("put pending: %v", err)
	}
	return action
}

// gmvOf 读取 id=1 行的 gmv。
func gmvOf(t *testing.T, db *gorm.DB) float64 {
	t.Helper()
	var gmv float64
	if err := db.Raw("SELECT gmv FROM it_confirm_src WHERE id = 1").Scan(&gmv).Error; err != nil {
		t.Fatalf("read gmv: %v", err)
	}
	return gmv
}

// TestConfirmIntegration_CommitsAndAudits 确认后真实落库 + 审计 success。
func TestConfirmIntegration_CommitsAndAudits(t *testing.T) {
	db, pending := setupConfirmIntegration(t)
	action := putConfirmITAction(t, pending, "cf-it-commit-1")

	w, data := callConfirmIT(t, map[string]interface{}{
		"pending_action_id": action.ID,
		"token":             action.Token,
		"session_id":        "sess-cf-it",
	})
	if w.Code != http.StatusOK || data["status"] != operations.StatusSuccess {
		t.Fatalf("确认应成功, code=%d body=%s", w.Code, w.Body.String())
	}
	if gmv := gmvOf(t, db); gmv != 200 {
		t.Fatalf("确认后应真实落库, gmv=%v", gmv)
	}

	var audit mysqlrepo.OperationAuditModel
	if err := db.Where("tenant_id = ? AND idempotency_key = ?", 991, "cf-it-commit-1").
		Order("id DESC").First(&audit).Error; err != nil {
		t.Fatalf("load audit: %v", err)
	}
	if audit.Status != operations.StatusSuccess {
		t.Errorf("审计应为 success, got %s", audit.Status)
	}
}

// TestConfirmIntegration_TokenMismatchNoWrite token 不匹配 → 403、立即失效、库不被改。
func TestConfirmIntegration_TokenMismatchNoWrite(t *testing.T) {
	db, pending := setupConfirmIntegration(t)
	action := putConfirmITAction(t, pending, "cf-it-mismatch-1")

	w, _ := callConfirmIT(t, map[string]interface{}{
		"pending_action_id": action.ID,
		"token":             "forged-token",
		"session_id":        "sess-cf-it",
	})
	if w.Code != http.StatusForbidden {
		t.Fatalf("token 不匹配期望 403, got %d", w.Code)
	}
	if gmv := gmvOf(t, db); gmv != 100 {
		t.Fatalf("拒绝后库不应被改, gmv=%v", gmv)
	}

	// 失效后携正确 token 也无法再确认，库依旧原样
	w2, data2 := callConfirmIT(t, map[string]interface{}{
		"pending_action_id": action.ID,
		"token":             action.Token,
		"session_id":        "sess-cf-it",
	})
	if w2.Code != http.StatusOK || data2["status"] != "expired" {
		t.Errorf("失效后应返回 expired, code=%d status=%v", w2.Code, data2["status"])
	}
	if gmv := gmvOf(t, db); gmv != 100 {
		t.Errorf("失效后库不应被改, gmv=%v", gmv)
	}
}
