package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	"cognida/internal/service/agent/resultstore"
	ragtool "cognida/internal/service/agent/tools"
	"cognida/internal/service/agent/uibinding"
)

// setupUISurfaceTest 构造携内存 Result Store + 内存绑定存储的工具注册表（经 SetToolGateway
// 注入 handler 网关）。绑定存储经 ToolDeps.UIBinding 显式注入（去 uibinding 包级单例，
// 架构加固 Phase 6），handler 回调路由从注册表网关 UIBinding()/SessionState() 读取。
func setupUISurfaceTest(t *testing.T) (*uibinding.MemoryStore, resultstore.Store, *AgentHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, _, err := sqlmock.New()
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

	bs := uibinding.NewMemoryStore()
	rs := resultstore.NewMemoryStore()
	reg, err := ragtool.NewToolRegistry(ragtool.ToolDeps{SQLDB: gdb, ResultStore: rs, UIBinding: bs})
	if err != nil {
		t.Fatalf("构造工具注册表: %v", err)
	}
	h := &AgentHandler{}
	h.SetToolGateway(reg)
	return bs, rs, h
}

// callUISurfacePage 以 gin 测试上下文调用 GetUISurfacePage。
func callUISurfacePage(t *testing.T, h *AgentHandler, surface, rawQuery string) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "surface", Value: surface}}
	c.Request = httptest.NewRequest(http.MethodGet, "/agent/ui/surfaces/"+surface+"/page?"+rawQuery, nil)

	h.GetUISurfacePage(c)

	var body struct {
		Data map[string]interface{} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w, body.Data
}

// TestGetUISurfacePage_SessionExpired 绑定缺失/超会话 TTL → session_expired 占位（任务 4.8）。
func TestGetUISurfacePage_SessionExpired(t *testing.T) {
	_, _, h := setupUISurfaceTest(t)

	w, data := callUISurfacePage(t, h, "sfc_gone", "token=tk_x")
	if w.Code != http.StatusOK {
		t.Fatalf("过期应走占位而非错误路由, code=%d", w.Code)
	}
	if data["status"] != "session_expired" {
		t.Errorf("期望 session_expired, got %v", data["status"])
	}
	if data["message"] != "会话已过期" {
		t.Errorf("占位文案不符: %v", data["message"])
	}
}

// TestGetUISurfacePage_TokenMismatch 伪造 token → 403。
func TestGetUISurfacePage_TokenMismatch(t *testing.T) {
	bs, _, h := setupUISurfaceTest(t)
	_ = bs.Put(context.Background(), &uibinding.Binding{
		Surface: "sfc_a", TenantID: 1, SessionID: "s1", ResultID: "res_1", Token: "tk_real",
	}, time.Hour)

	w, _ := callUISurfacePage(t, h, "sfc_a", "token=tk_fake")
	if w.Code != http.StatusForbidden {
		t.Errorf("token 不匹配期望 403, got %d", w.Code)
	}
}

// TestGetUISurfacePage_DataExpired result_id 已过期 → data_expired 占位（任务 4.7 大数据降级）。
func TestGetUISurfacePage_DataExpired(t *testing.T) {
	bs, _, h := setupUISurfaceTest(t)
	_ = bs.Put(context.Background(), &uibinding.Binding{
		Surface: "sfc_b", TenantID: 1, SessionID: "s1", ResultID: "res_已回收", Token: "tk_b",
	}, time.Hour)

	w, data := callUISurfacePage(t, h, "sfc_b", "token=tk_b")
	if w.Code != http.StatusOK {
		t.Fatalf("数据过期应走占位而非错误路由, code=%d", w.Code)
	}
	if data["status"] != "data_expired" {
		t.Errorf("期望 data_expired, got %v", data["status"])
	}
	if data["message"] != "数据已过期，可重跑" {
		t.Errorf("占位文案不符: %v", data["message"])
	}
}

// TestGetUISurfacePage_OKPaged 正常路径按 cursor 分页返回，不重跑查询。
func TestGetUISurfacePage_OKPaged(t *testing.T) {
	bs, rs, h := setupUISurfaceTest(t)
	ctx := context.Background()

	rows := make([]map[string]interface{}, 0, 5)
	for i := 0; i < 5; i++ {
		rows = append(rows, map[string]interface{}{"n": float64(i)})
	}
	resultID, err := rs.Put(ctx, &resultstore.Result{
		Owner:   resultstore.OwnerKey(1, "s1"),
		Columns: []string{"n"},
		Rows:    rows,
	}, resultstore.DefaultTTL)
	if err != nil {
		t.Fatalf("预置结果集失败: %v", err)
	}
	_ = bs.Put(ctx, &uibinding.Binding{
		Surface: "sfc_ok", TenantID: 1, SessionID: "s1", ResultID: resultID, Token: "tk_ok",
	}, time.Hour)

	// 第一页：cursor=0, page_size=2 → 2 行，next_cursor=2
	w, data := callUISurfacePage(t, h, "sfc_ok", "token=tk_ok&cursor=0&page_size=2")
	if w.Code != http.StatusOK || data["status"] != "ok" {
		t.Fatalf("正常路径失败: code=%d status=%v", w.Code, data["status"])
	}
	if got := data["rows"].([]interface{}); len(got) != 2 {
		t.Errorf("第一页应 2 行, got %d", len(got))
	}
	if data["next_cursor"] != "2" {
		t.Errorf("next_cursor 期望 2, got %v", data["next_cursor"])
	}
	if data["row_count"].(float64) != 5 {
		t.Errorf("row_count 期望 5, got %v", data["row_count"])
	}

	// 尾页：cursor=4 → 1 行，无 next_cursor
	_, last := callUISurfacePage(t, h, "sfc_ok", "token=tk_ok&cursor=4&page_size=2")
	if got := last["rows"].([]interface{}); len(got) != 1 {
		t.Errorf("尾页应 1 行, got %d", len(got))
	}
	if last["next_cursor"] != "" {
		t.Errorf("尾页 next_cursor 应为空, got %v", last["next_cursor"])
	}
}

// TestGetUISurfacePage_ParamValidation surface/token 缺失 → 400。
func TestGetUISurfacePage_ParamValidation(t *testing.T) {
	_, _, h := setupUISurfaceTest(t)

	w, _ := callUISurfacePage(t, h, "sfc_x", "")
	if w.Code != http.StatusBadRequest {
		t.Errorf("缺 token 期望 400, got %d", w.Code)
	}
}
