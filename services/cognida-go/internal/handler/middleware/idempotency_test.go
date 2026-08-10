package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

// 构造挂了幂等中间件（进程内降级，redis=nil）的测试引擎，业务 handler 每次执行 +1 计数。
func newIdemEngine(calls *int32) *gin.Engine {
	idem := NewIdempotency(nil)
	r := gin.New()
	r.Use(func(c *gin.Context) { // 模拟 tenant/user 上下文
		c.Set("tenant_id", int64(7))
		c.Set("user_id", int64(42))
		c.Next()
	})
	r.Use(idem.Apply())
	r.POST("/orders", func(c *gin.Context) {
		atomic.AddInt32(calls, 1)
		c.JSON(http.StatusCreated, gin.H{"id": "order-1"})
	})
	return r
}

func doPost(r *gin.Engine, key string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/orders", nil)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	r.ServeHTTP(w, req)
	return w
}

// 携带相同 Idempotency-Key 的重复写请求应只执行一次业务，重放返回缓存响应。
func TestIdempotency_ReplaysCachedResponse(t *testing.T) {
	var calls int32
	r := newIdemEngine(&calls)

	w1 := doPost(r, "abc-123")
	if w1.Code != http.StatusCreated {
		t.Fatalf("首个请求应 201，得 %d", w1.Code)
	}

	w2 := doPost(r, "abc-123")
	if w2.Code != http.StatusCreated {
		t.Fatalf("重放应回放缓存的 201，得 %d", w2.Code)
	}
	if w2.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatalf("重放响应应带 Idempotency-Replayed=true 头")
	}
	if w1.Body.String() != w2.Body.String() {
		t.Fatalf("重放 body 应与首个一致：%q vs %q", w1.Body.String(), w2.Body.String())
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("业务应只执行一次，实际执行 %d 次", got)
	}
}

// 不带 Idempotency-Key 的请求不介入：每次都执行业务（非破坏、opt-in）。
func TestIdempotency_NoKeyPassesThrough(t *testing.T) {
	var calls int32
	r := newIdemEngine(&calls)
	doPost(r, "")
	doPost(r, "")
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("无幂等键应每次执行，期望 2 次，得 %d", got)
	}
}

// 不同键互不影响：各自执行一次。
func TestIdempotency_DistinctKeysIndependent(t *testing.T) {
	var calls int32
	r := newIdemEngine(&calls)
	doPost(r, "k1")
	doPost(r, "k2")
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("不同键应各执行一次，期望 2 次，得 %d", got)
	}
}

// GET（安全方法）即便带键也不介入。
func TestIdempotency_SkipsSafeMethods(t *testing.T) {
	var calls int32
	idem := NewIdempotency(nil)
	r := gin.New()
	r.Use(idem.Apply())
	r.GET("/orders", func(c *gin.Context) {
		atomic.AddInt32(&calls, 1)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/orders", nil)
		req.Header.Set("Idempotency-Key", "same")
		r.ServeHTTP(w, req)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("GET 不应被幂等拦截，期望 2 次，得 %d", got)
	}
}

// 在途（processing）语义：同键第二次 begin 不再成为首个，且返回 processing 记录，
// 中间件据此回 409（serveExisting）。直接驱动进程内 store 断言占位语义。
func TestIdempotency_InFlightBeginSemantics(t *testing.T) {
	idem := NewIdempotency(nil)
	const key = "idem:7:42:POST:/orders:live"

	if first, _ := idem.local.begin(key); !first {
		t.Fatalf("首次 begin 应成为首个（first=true）")
	}
	first, ex := idem.local.begin(key)
	if first {
		t.Fatalf("同键第二次 begin 不应再成为首个")
	}
	if ex == nil || ex.State != idemStateNew {
		t.Fatalf("在途记录状态应为 processing，得 %+v", ex)
	}
}
