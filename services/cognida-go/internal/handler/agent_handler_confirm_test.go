package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	"cognida/internal/model/agent/operations"
	"cognida/internal/service/agent/genui"
	"cognida/internal/service/agent/pendingaction"
	ragtool "cognida/internal/service/agent/tools"
)

// confirmFakeAudit 内存审计仓储：只记录，不做幂等命中。
type confirmFakeAudit struct {
	records []*operations.OperationAudit
}

func (f *confirmFakeAudit) Record(ctx context.Context, a *operations.OperationAudit) error {
	f.records = append(f.records, a)
	return nil
}

func (f *confirmFakeAudit) FindByIdempotencyKey(ctx context.Context, tenantID int64, key string) (*operations.OperationAudit, error) {
	return nil, nil
}

// setupConfirmTest 注入 sqlmock DB + 内存待确认存储 + 内存审计，返回 mock、存储与
// 已注入工具网关的 handler（经 SetToolGateway，替代包级默认槽位）。
func setupConfirmTest(t *testing.T) (sqlmock.Sqlmock, *pendingaction.MemoryStore, *confirmFakeAudit, *AgentHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	gdb, err := gorm.Open(gormmysql.New(gormmysql.Config{
		Conn: db, SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}

	pending := pendingaction.NewMemoryStore()
	audit := &confirmFakeAudit{}
	reg, err := ragtool.NewToolRegistry(ragtool.ToolDeps{
		SQLDB: gdb,
		Operation: ragtool.OperationConfig{
			DB:      gdb,
			Audit:   audit,
			Pending: pending,
		},
	})
	if err != nil {
		t.Fatalf("构造工具注册表: %v", err)
	}
	h := &AgentHandler{}
	h.SetToolGateway(reg)
	return mock, pending, audit, h
}

// callConfirm 以 gin 测试上下文调用 ConfirmOperation（tenant_id 固定 7）。
func callConfirm(t *testing.T, h *AgentHandler, payload map[string]interface{}) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()

	body, _ := json.Marshal(payload)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("tenant_id", int64(7))
	c.Request = httptest.NewRequest(http.MethodPost, "/agent/operations/confirm", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.ConfirmOperation(c)

	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp.Data
}

// putPendingMutation 预置一个归属 7:sess-cf 的待确认写操作。
func putPendingMutation(t *testing.T, pending *pendingaction.MemoryStore) *pendingaction.PendingAction {
	t.Helper()
	action := &pendingaction.PendingAction{
		Owner: "7:sess-cf",
		Kind:  operations.OpMutate,
		SQL:   "UPDATE agent_etl_orders SET amount = amount * 2",
		Params: map[string]interface{}{
			"idempotency_key": "cf-key-1",
			"target":          "agent_etl_orders",
			"rows_affected":   int64(250),
		},
	}
	if _, err := pending.Put(context.Background(), action, pendingaction.DefaultTTL); err != nil {
		t.Fatalf("put pending: %v", err)
	}
	return action
}

// TestConfirmOperation_CommitsOnValidToken token 匹配 → 提交事务并落审计（任务 6.4 确认落库）。
func TestConfirmOperation_CommitsOnValidToken(t *testing.T) {
	mock, pending, audit, h := setupConfirmTest(t)
	action := putPendingMutation(t, pending)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE agent_etl_orders").WillReturnResult(sqlmock.NewResult(0, 250))
	mock.ExpectCommit()

	w, data := callConfirm(t, h, map[string]interface{}{
		"pending_action_id": action.ID,
		"token":             action.Token,
		"session_id":        "sess-cf",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("确认执行应成功, code=%d body=%s", w.Code, w.Body.String())
	}
	if data["status"] != operations.StatusSuccess {
		t.Errorf("期望 success, got %v", data["status"])
	}
	if data["rows_affected"].(float64) != 250 {
		t.Errorf("影响行数期望 250, got %v", data["rows_affected"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("事务未按预期提交: %v", err)
	}
	if len(audit.records) == 0 || audit.records[len(audit.records)-1].Status != operations.StatusSuccess {
		t.Errorf("确认执行必须留痕 success 审计: %+v", audit.records)
	}

	// 已消费：同凭据再次确认 → expired 占位
	w2, data2 := callConfirm(t, h, map[string]interface{}{
		"pending_action_id": action.ID,
		"token":             action.Token,
		"session_id":        "sess-cf",
	})
	if w2.Code != http.StatusOK || data2["status"] != "expired" {
		t.Errorf("已消费的 pending action 应返回 expired, code=%d status=%v", w2.Code, data2["status"])
	}
}

// TestConfirmOperation_TokenMismatchRejectsAndInvalidates token 不匹配 → 403 且立即失效。
func TestConfirmOperation_TokenMismatchRejectsAndInvalidates(t *testing.T) {
	_, pending, _, h := setupConfirmTest(t)
	action := putPendingMutation(t, pending)

	w, _ := callConfirm(t, h, map[string]interface{}{
		"pending_action_id": action.ID,
		"token":             "forged-token",
		"session_id":        "sess-cf",
	})
	if w.Code != http.StatusForbidden {
		t.Fatalf("token 不匹配期望 403, got %d", w.Code)
	}

	// 防暴力尝试：失效后携正确 token 也无法再确认
	w2, data2 := callConfirm(t, h, map[string]interface{}{
		"pending_action_id": action.ID,
		"token":             action.Token,
		"session_id":        "sess-cf",
	})
	if w2.Code != http.StatusOK || data2["status"] != "expired" {
		t.Errorf("token 不匹配后应立即失效, code=%d status=%v", w2.Code, data2["status"])
	}
}

// TestConfirmOperation_ExpiredReturnsPlaceholder 过期/不存在 → expired 占位（非错误路由）。
func TestConfirmOperation_ExpiredReturnsPlaceholder(t *testing.T) {
	_, pending, _, h := setupConfirmTest(t)
	action := putPendingMutation(t, pending)
	// MemoryStore 持有指针：直接回拨过期时间模拟 TTL 到期
	action.ExpiresAt = 1

	w, data := callConfirm(t, h, map[string]interface{}{
		"pending_action_id": action.ID,
		"token":             action.Token,
		"session_id":        "sess-cf",
	})
	if w.Code != http.StatusOK || data["status"] != "expired" {
		t.Errorf("过期应返回 expired 占位, code=%d status=%v", w.Code, data["status"])
	}
}

// TestConfirmOperation_OwnerMismatchNotConsumed 跨会话确认 → expired 占位且不消费。
func TestConfirmOperation_OwnerMismatchNotConsumed(t *testing.T) {
	mock, pending, _, h := setupConfirmTest(t)
	action := putPendingMutation(t, pending)

	// 携他人 session：视同不存在（不泄露存在性），且不消费
	w, data := callConfirm(t, h, map[string]interface{}{
		"pending_action_id": action.ID,
		"token":             action.Token,
		"session_id":        "sess-other",
	})
	if w.Code != http.StatusOK || data["status"] != "expired" {
		t.Fatalf("跨会话确认应返回 expired, code=%d status=%v", w.Code, data["status"])
	}

	// 原归属者仍可确认执行
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE agent_etl_orders").WillReturnResult(sqlmock.NewResult(0, 250))
	mock.ExpectCommit()
	w2, data2 := callConfirm(t, h, map[string]interface{}{
		"pending_action_id": action.ID,
		"token":             action.Token,
		"session_id":        "sess-cf",
	})
	if w2.Code != http.StatusOK || data2["status"] != operations.StatusSuccess {
		t.Errorf("原归属者应仍可确认, code=%d status=%v", w2.Code, data2["status"])
	}
}

// TestExtractPendingConfirmUISpec 危险暂停 → Go 端拼装确认卡片（任务 6.3）。
func TestExtractPendingConfirmUISpec(t *testing.T) {
	out := `{"status":"pending_confirm","target":"agent_etl_orders","rows_affected":250,` +
		`"pending_action_id":"pa_abc","confirm_token":"tk_abc","message":"已暂停"}`

	spec := extractPendingConfirmUISpec(out, "sess-cf")
	if spec == nil {
		t.Fatal("pending_confirm 输出应装配确认卡片")
	}
	if err := genui.Validate(spec); err != nil {
		t.Fatalf("确认卡片规格必须通过校验: %v", err)
	}

	var confirm *genui.Component
	for i := range spec.Components {
		if spec.Components[i].Type == genui.CompConfirm {
			confirm = &spec.Components[i]
		}
	}
	if confirm == nil {
		t.Fatal("规格中缺少 Confirm 组件")
	}
	if confirm.Props["pending_action_id"] != "pa_abc" {
		t.Errorf("pending_action_id 不符: %v", confirm.Props["pending_action_id"])
	}
	action := confirm.Props["action"].(map[string]interface{})
	if action["endpoint"] != genui.ConfirmActionEndpoint {
		t.Errorf("回调端点不符: %v", action["endpoint"])
	}
	params := action["params"].(map[string]interface{})
	if params["token"] != "tk_abc" || params["session_id"] != "sess-cf" || params["pending_action_id"] != "pa_abc" {
		t.Errorf("回调参数不完整: %v", params)
	}

	// 非暂停结果不产卡片
	if s := extractPendingConfirmUISpec(`{"status":"success","rows_affected":3}`, "sess-cf"); s != nil {
		t.Errorf("success 结果不应产确认卡片: %+v", s)
	}
	if s := extractPendingConfirmUISpec("not-json", "sess-cf"); s != nil {
		t.Errorf("非 JSON 输出不应产确认卡片: %+v", s)
	}
}

// TestConfirmOperation_ParamValidation 缺参 → 400。
func TestConfirmOperation_ParamValidation(t *testing.T) {
	_, _, _, h := setupConfirmTest(t)

	w, _ := callConfirm(t, h, map[string]interface{}{"pending_action_id": "pa_x"})
	if w.Code != http.StatusBadRequest {
		t.Errorf("缺 token/session_id 期望 400, got %d", w.Code)
	}
}

// TestConfirmOperation_UnsupportedKind 不支持的操作类型 → 400（凭据已消费）。
func TestConfirmOperation_UnsupportedKind(t *testing.T) {
	_, pending, _, h := setupConfirmTest(t)
	action := &pendingaction.PendingAction{
		Owner: "7:sess-cf",
		Kind:  "unknown_kind",
		SQL:   "SELECT 1",
	}
	if _, err := pending.Put(context.Background(), action, pendingaction.DefaultTTL); err != nil {
		t.Fatalf("put pending: %v", err)
	}

	w, _ := callConfirm(t, h, map[string]interface{}{
		"pending_action_id": action.ID,
		"token":             action.Token,
		"session_id":        "sess-cf",
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("不支持的类型期望 400, got %d", w.Code)
	}
}
