// Package middleware: 认证与 CORS 中间件单元测试。
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newAuthTestRouter 装配仅含 AuthMiddleware 的测试路由。
// accountService 传 nil：被测路径（无 Authorization → 401）在触达该依赖前即返回。
func newAuthTestRouter() *gin.Engine {
	r := gin.New()
	r.Use(NewAuthMiddleware(nil).Apply())
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

// TestAuth_APIKeyOnly_Returns401 仅带 X-API-Key 的请求必须 401：
// X-API-Key 分支已移除（api_keys 表校验落地前等于无认证）。
func TestAuth_APIKeyOnly_Returns401(t *testing.T) {
	t.Setenv("DEV_MODE", "false")
	r := newAuthTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("X-API-Key", "any-nonempty-key")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("仅带 X-API-Key 的请求应 401, got %d", w.Code)
	}
}

// TestAuth_NoCredentials_Returns401 完全无认证信息的请求必须 401。
func TestAuth_NoCredentials_Returns401(t *testing.T) {
	t.Setenv("DEV_MODE", "false")
	r := newAuthTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("无认证信息的请求应 401, got %d", w.Code)
	}
}

// newCORSTestRouter 装配仅含 CORSMiddleware 的测试路由（白名单来自 env）。
func newCORSTestRouter() *gin.Engine {
	r := gin.New()
	r.Use(NewCORSMiddleware().Apply())
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

// TestCORS_AllowedOrigin_Reflected 白名单内 Origin 被反射且带 Allow-Credentials。
func TestCORS_AllowedOrigin_Reflected(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:3000")
	r := newCORSTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("Allow-Origin = %q, 期望反射白名单内 Origin", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Allow-Credentials = %q, 期望 true", got)
	}
	if got := w.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, 期望 Origin（防缓存投毒）", got)
	}
}

// TestCORS_DisallowedOrigin_NoHeaders 白名单外 Origin 不得回写 Allow-Origin/Allow-Credentials。
func TestCORS_DisallowedOrigin_NoHeaders(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:5173")
	r := newCORSTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("白名单外 Origin 不应回写 Allow-Origin, got %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("白名单外 Origin 不应回写 Allow-Credentials, got %q", got)
	}
}

// TestCORS_EmptyAllowlist_FailClosed 未配置白名单时任何 Origin 都不回写（fail-closed）。
func TestCORS_EmptyAllowlist_FailClosed(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "")
	r := newCORSTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("空白名单不应回写 Allow-Origin, got %q", got)
	}
}

// TestCORS_WildcardInConfig_Ignored 配置中的 "*" 被剔除，不产生反射任意 Origin 的行为。
func TestCORS_WildcardInConfig_Ignored(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "*")
	r := newCORSTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("通配符配置应被剔除, 不应回写 Allow-Origin, got %q", got)
	}
}
