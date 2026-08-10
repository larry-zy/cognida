package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"cognida/internal/pkg/pagination"
)

func init() { gin.SetMode(gin.TestMode) }

// newCtx 构造带 request_id 的测试 gin 上下文（模拟 Trace 中间件已注入）。
func newCtx(requestID string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	if requestID != "" {
		c.Set("request_id", requestID)
	}
	return c, w
}

// 所有走统一封套的响应体都应回带 request_id（闭合链路断点，〔M7〕）。
func TestResponseEnvelope_CarriesRequestID(t *testing.T) {
	cases := []struct {
		name       string
		call       func(c *gin.Context)
		wantStatus int
		wantCode   int
	}{
		{"OK", func(c *gin.Context) { OK(c, gin.H{"x": 1}) }, http.StatusOK, 0},
		{"Created", func(c *gin.Context) { Created(c, nil) }, http.StatusCreated, 0},
		{"BadRequest", func(c *gin.Context) { BadRequest(c, "bad") }, http.StatusBadRequest, http.StatusBadRequest},
		{"NotFound", func(c *gin.Context) { NotFound(c, "nope") }, http.StatusNotFound, http.StatusNotFound},
		{"InternalError", func(c *gin.Context) { InternalError(c, "boom") }, http.StatusInternalServerError, http.StatusInternalServerError},
		{"PageSuccessJSON", func(c *gin.Context) { PageSuccessJSON(c, 3, []int{1, 2, 3}, 1, 20) }, http.StatusOK, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, w := newCtx("rid-123")
			tc.call(c)
			if w.Code != tc.wantStatus {
				t.Fatalf("状态码应为 %d，得 %d", tc.wantStatus, w.Code)
			}
			var resp Response
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("响应体应为合法封套 JSON: %v\n%s", err, w.Body.String())
			}
			if resp.RequestID != "rid-123" {
				t.Fatalf("封套应回带 request_id=rid-123，得 %q", resp.RequestID)
			}
			if resp.Code != tc.wantCode {
				t.Fatalf("code 应为 %d，得 %d", tc.wantCode, resp.Code)
			}
		})
	}
}

// 无 request_id 时 omitempty 生效，字段不出现（不误报空串）。
func TestResponseEnvelope_OmitsEmptyRequestID(t *testing.T) {
	c, w := newCtx("")
	OK(c, nil)
	if body := w.Body.String(); jsonHasKey(t, body, "request_id") {
		t.Fatalf("无 request_id 时不应出现该键：%s", body)
	}
}

// RespondError 按错误语义映射状态码，且未识别错误不回显内部细节。
func TestRespondError_Mapping(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"canceled", context.Canceled, 499},
		{"timeout", context.DeadlineExceeded, http.StatusGatewayTimeout},
		{"notfound", gorm.ErrRecordNotFound, http.StatusNotFound},
		{"wrapped-notfound", errors.New("wrap: " + gorm.ErrRecordNotFound.Error()), http.StatusInternalServerError}, // 非 errors.Is 包裹 → 归 500
		{"invalid-cursor", pagination.ErrInvalidCursor, http.StatusBadRequest},
		// 仓储层的实际包裹形态（%w 挂链）也应被 errors.Is 识别为 400，而非 500。
		{"wrapped-invalid-cursor", fmt.Errorf("解析游标失败: %w", pagination.ErrInvalidCursor), http.StatusBadRequest},
		{"unknown", errors.New("内部数据库连接字符串 root:secret@..."), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, w := newCtx("rid-err")
			RespondError(c, tc.err)
			if w.Code != tc.wantStatus {
				t.Fatalf("状态码应为 %d，得 %d", tc.wantStatus, w.Code)
			}
			var resp Response
			_ = json.Unmarshal(w.Body.Bytes(), &resp)
			if resp.RequestID != "rid-err" {
				t.Fatalf("错误响应也应回带 request_id，得 %q", resp.RequestID)
			}
			// 未识别错误：对外文案不得泄漏内部细节（如 DSN 密码）。
			if tc.name == "unknown" && resp.Message != "服务内部错误" {
				t.Fatalf("未识别错误应回通用文案，得 %q（疑似泄漏内部细节）", resp.Message)
			}
		})
	}
}

func jsonHasKey(t *testing.T, body, key string) bool {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("响应体应为 JSON 对象: %v", err)
	}
	_, ok := m[key]
	return ok
}
