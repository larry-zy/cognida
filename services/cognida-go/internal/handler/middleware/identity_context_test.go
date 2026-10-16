package middleware

import (
	agentctx "cognida/internal/model/agent"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTenantIdentityPropagatesVerifiedScope(t *testing.T) {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", int64(41))
		c.Set("user_id", int64(9))
		ctx := agentctx.WithTenantID(context.Background(), 999)
		c.Request = c.Request.WithContext(agentctx.WithUserID(ctx, 999))
		c.Next()
	})
	r.Use(NewTenantMiddleware().Apply())
	r.GET("/", func(c *gin.Context) {
		tenant, ok := agentctx.GetTenantID(c.Request.Context())
		require.True(t, ok)
		require.Equal(t, int64(41), tenant)
		user, ok := agentctx.GetUserID(c.Request.Context())
		require.True(t, ok)
		require.Equal(t, int64(9), user)
		c.Status(204)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Tenant-ID", "999")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 204, w.Code)
}

func TestTenantIdentityDoesNotInferMissingUser(t *testing.T) {
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("tenant_id", int64(41)); c.Next() })
	r.Use(NewTenantMiddleware().Apply())
	r.GET("/", func(c *gin.Context) { t.Fatal("unauthenticated service context reached") })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, 403, w.Code)
}
