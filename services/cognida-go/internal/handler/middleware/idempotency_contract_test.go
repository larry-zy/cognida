package middleware

import (
	"bytes"
	"context"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
	"time"
)

func testBoundIdempotency(t *testing.T, m *Idempotency) {
	t.Helper()
	calls := 0
	keyValue := "review-" + uuid.NewString()
	if m.redis != nil {
		for _, path := range []string{"/resource/a", "/resource/b"} {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("PUT", path, nil)
			c.Set("tenant_id", int64(7))
			c.Set("user_id", int64(42))
			key := m.compositeKey(c, keyValue)
			t.Cleanup(func() { require.NoError(t, m.redis.Del(context.Background(), key).Err()) })
		}
	}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("tenant_id", int64(7)); c.Set("user_id", int64(42)) })
	r.Use(m.Apply())
	r.PUT("/resource/:id", func(c *gin.Context) {
		calls++
		var body map[string]any
		require.NoError(t, c.ShouldBindJSON(&body))
		c.JSON(200, gin.H{"id": c.Param("id"), "body": body})
	})
	request := func(path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		q := httptest.NewRequest("PUT", path, bytes.NewBufferString(body))
		q.Header.Set("Content-Type", "application/json")
		q.Header.Set(idemHeader, keyValue)
		r.ServeHTTP(w, q)
		return w
	}
	a := request("/resource/a", `{"value":1}`)
	require.Equal(t, 200, a.Code)
	b := request("/resource/b", `{"value":1}`)
	require.Equal(t, 200, b.Code)
	require.Contains(t, b.Body.String(), `"id":"b"`)
	replay := request("/resource/a", `{"value":1}`)
	require.Equal(t, "true", replay.Header().Get("Idempotency-Replayed"))
	require.Equal(t, a.Body.String(), replay.Body.String())
	require.Equal(t, 409, request("/resource/a", `{"value":2}`).Code)
	require.Equal(t, 409, request("/resource/a?force=true", `{"value":1}`).Code)
	require.Equal(t, 2, calls)
}
func TestIdempotencyConcreteResourceAndPayload(t *testing.T) {
	testBoundIdempotency(t, NewIdempotency(nil))
}
func TestIdempotencyStaleClaimCannotReplaceNewPayload(t *testing.T) {
	m := NewIdempotency(nil)
	ctx := context.Background()
	old := idemRecord{State: idemStateNew, Owner: "old", Digest: "old-body"}
	next := idemRecord{State: idemStateNew, Owner: "new", Digest: "new-body"}
	first, _, err := m.begin(ctx, "k", old)
	require.NoError(t, err)
	require.True(t, first)
	m.local.m["k"].exp = time.Now().Add(-time.Second)
	first, _, err = m.begin(ctx, "k", next)
	require.NoError(t, err)
	require.True(t, first)
	old.State = idemStateDon
	m.complete(ctx, "k", old)
	m.release(ctx, "k", old.Owner)
	first, got, err := m.begin(ctx, "k", old)
	require.NoError(t, err)
	require.False(t, first)
	require.Equal(t, next, *got)
}
func TestIdempotencyRechecksMessageAndDatasourceAuthorization(t *testing.T) {
	for _, path := range []string{"/api/v1/messages/:id", "/api/v1/datasources/:id"} {
		calls := 0
		r := gin.New()
		r.Use(NewIdempotency(nil).Apply())
		r.PUT(path, func(c *gin.Context) {
			calls++
			if calls == 1 {
				c.JSON(200, gin.H{"ok": true})
			} else {
				c.AbortWithStatus(403)
			}
		})
		for _, want := range []int{200, 403} {
			w := httptest.NewRecorder()
			q := httptest.NewRequest("PUT", bytes.NewBufferString(path).String(), nil)
			q.Header.Set(idemHeader, "same")
			r.ServeHTTP(w, q)
			require.Equal(t, want, w.Code)
		}
		require.Equal(t, 2, calls)
	}
}

func TestIdempotencyRedisFailureDoesNotExecuteWrite(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	require.NoError(t, client.Close()) // deterministic failure; no connection attempt
	calls := 0
	r := gin.New()
	r.Use(NewIdempotency(client).Apply())
	r.POST("/orders", func(c *gin.Context) { calls++; c.Status(200) })
	q := httptest.NewRequest("POST", "/orders", nil)
	q.Header.Set(idemHeader, "secret-client-key")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, q)
	require.Equal(t, 503, w.Code)
	require.Zero(t, calls)
	require.NotContains(t, w.Body.String(), "secret-client-key")
}
