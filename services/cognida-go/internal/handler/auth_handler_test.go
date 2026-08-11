package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	appAccount "cognida/internal/service/account"
)

// respondAuthError 按账号域 sentinel 精准映射 HTTP 语义，且对外不泄漏内部细节〔R2-1〕。
func TestRespondAuthError_Mapping(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantMsg    string // 期望对外文案（空串表示不校验具体文案）
	}{
		{"email-exists", appAccount.ErrEmailExists, http.StatusConflict, "邮箱已被注册"},
		{"username-exists", appAccount.ErrUsernameExists, http.StatusConflict, "用户名已被使用"},
		{"invalid-credential", appAccount.ErrInvalidCredential, http.StatusUnauthorized, "邮箱或密码错误"},
		{"invalid-token", appAccount.ErrInvalidToken, http.StatusUnauthorized, "无效的令牌"},
		{"account-disabled", appAccount.ErrAccountDisabled, http.StatusForbidden, "账号已被禁用"},
		{"user-not-found", appAccount.ErrUserNotFound, http.StatusNotFound, "用户不存在"},
		{"old-password", appAccount.ErrOldPasswordIncorrect, http.StatusBadRequest, "旧密码错误"},
		// service 侧对 sentinel 做 %w 包裹（如 ValidateToken 的「缺少 user_id」）仍应经
		// errors.Is 归 401，且对外只暴露 sentinel 文案、不泄漏包裹链细节。
		{"wrapped-invalid-token", fmt.Errorf("%w: 缺少 user_id", appAccount.ErrInvalidToken), http.StatusUnauthorized, "无效的令牌"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, w := newCtx("rid-auth")
			respondAuthError(c, tc.err)
			if w.Code != tc.wantStatus {
				t.Fatalf("状态码应为 %d，得 %d", tc.wantStatus, w.Code)
			}
			var resp Response
			_ = json.Unmarshal(w.Body.Bytes(), &resp)
			if resp.RequestID != "rid-auth" {
				t.Fatalf("错误响应也应回带 request_id，得 %q", resp.RequestID)
			}
			if tc.wantMsg != "" && resp.Message != tc.wantMsg {
				t.Fatalf("对外文案应为 %q，得 %q", tc.wantMsg, resp.Message)
			}
		})
	}
}

// 未识别错误：委托 RespondError → 500 且回通用文案，不回显内部细节（防泄漏回归）。
func TestRespondAuthError_UnknownFallsBackToGeneric(t *testing.T) {
	c, w := newCtx("rid-auth")
	respondAuthError(c, fmt.Errorf("内部数据库连接字符串 root:secret@tcp(...)"))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("未识别错误应归 500，得 %d", w.Code)
	}
	var resp Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Message != "服务内部错误" {
		t.Fatalf("未识别错误应回通用文案，得 %q（疑似泄漏内部细节）", resp.Message)
	}
}
