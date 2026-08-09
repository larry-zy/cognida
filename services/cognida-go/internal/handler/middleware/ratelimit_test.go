package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() { gin.SetMode(gin.TestMode) }

// TestRateLimiter_LocalFallback_BlocksBurst 验证 Redis 为 nil 时的进程内令牌桶：
// 容量耗尽后应拒绝。
func TestRateLimiter_LocalFallback_BlocksBurst(t *testing.T) {
	rl := NewRateLimiter(nil)
	ctx := context.Background()
	// 速率极低、容量 2：前两次放行，第三次拒绝。
	assert.True(t, rl.allow(ctx, "k", 0.0001, 2))
	assert.True(t, rl.allow(ctx, "k", 0.0001, 2))
	assert.False(t, rl.allow(ctx, "k", 0.0001, 2))
	// 不同 key 独立计量。
	assert.True(t, rl.allow(ctx, "other", 0.0001, 2))
}

// TestRateLimiter_Redis_TokenBucket 验证 Redis 令牌桶：容量耗尽后拒绝。
func TestRateLimiter_Redis_TokenBucket(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	rl := NewRateLimiter(client)
	ctx := context.Background()
	assert.True(t, rl.allow(ctx, "rk", 0.0001, 2))
	assert.True(t, rl.allow(ctx, "rk", 0.0001, 2))
	assert.False(t, rl.allow(ctx, "rk", 0.0001, 2), "容量耗尽后应拒绝")
}

// TestRateLimiter_LoginMiddleware_Returns429 端到端验证 Login 限流返回 429。
func TestRateLimiter_LoginMiddleware_Returns429(t *testing.T) {
	t.Setenv("RATE_LIMIT_LOGIN_PER_MIN", "60") // 1/s
	t.Setenv("RATE_LIMIT_LOGIN_BURST", "1")

	rl := NewRateLimiter(nil)
	r := gin.New()
	r.POST("/login", rl.Login(), func(c *gin.Context) { c.Status(http.StatusOK) })

	do := func() int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = "203.0.113.7:1234"
		r.ServeHTTP(w, req)
		return w.Code
	}
	assert.Equal(t, http.StatusOK, do(), "首次应放行")
	assert.Equal(t, http.StatusTooManyRequests, do(), "突发耗尽后应 429")
}
