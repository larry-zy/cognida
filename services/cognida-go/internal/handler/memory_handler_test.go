package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMemoryHTTPRejectsMissingIdentityAndOwnerFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, identity := range []struct{ tenant, user int64 }{{0, 0}, {1, 0}, {0, 1}, {-1, 2}} {
		r := gin.New()
		r.Use(func(c *gin.Context) { c.Set("tenant_id", identity.tenant); c.Set("user_id", identity.user) })
		NewMemoryHandler(nil).Register(r.Group("/api"))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/api/memories", nil))
		require.Equal(t, 401, w.Code)
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("tenant_id", int64(1)); c.Set("user_id", int64(2)) })
	NewMemoryHandler(nil).Register(r.Group("/api"))
	for _, body := range []string{`{"content":"x","category":"fact","user_id":3}`, `{"content":"x","category":"fact","tenant_id":3}`, `{} {}`, strings.Repeat("x", 33000)} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/memories", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		require.Equal(t, 400, w.Code, w.Body.String())
	}
}
