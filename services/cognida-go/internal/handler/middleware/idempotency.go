package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// ========================================
// Idempotency 幂等中间件〔M6〕
// ========================================
//
// 背景：项目约定「写操作使用 idempotency_key」，但 HTTP 写接口层此前既不解析
// Idempotency-Key 头也不做请求级去重——网络重传/前端重复点击可致重复下单式副作用。
//
// 设计（Stripe 风格，opt-in、非破坏）：
//   - 仅对写方法（POST/PUT/PATCH/DELETE）且携带 Idempotency-Key 头的请求生效；
//     安全方法与未带键的请求零开销直接放行，不改变既有行为。
//   - 去重维度：tenant_id + user_id + method + path + key，避免跨租户/跨用户串号。
//   - 首个请求：SETNX 占位（processing），执行业务并捕获响应（状态码+body），
//     完成后以 done 记录覆盖（含响应快照，TTL 内可重放）。
//   - 重放请求：命中 done 记录 → 直接回放缓存的状态码+body，不再执行业务；
//     命中 processing（并发在途）→ 返回 409 让客户端稍后重试。
//   - 5xx 不缓存：删除占位放行后续重试（服务端错误应可重试成功）。
//   - 首选 Redis（多实例共享），未配置时降级为进程内 TTL 表（单实例仍有防护）；
//     Redis 运行期报错 fail-open（放行 + 记日志），中间件自身故障不升级为全站不可用。
//   - 跳过 /stream 路径：SSE 长流不宜整体缓冲进内存。

const (
	idemHeader   = "Idempotency-Key"
	idemStateNew = "processing"
	idemStateDon = "done"
)

// idemRecord 幂等记录：占位期仅 State，完成后含响应快照。
type idemRecord struct {
	State       string `json:"state"`
	Status      int    `json:"status,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Body        []byte `json:"body,omitempty"` // json 将 []byte 编为 base64
}

// Idempotency 幂等中间件：持有 Redis 客户端与进程内降级表。
type Idempotency struct {
	redis   *redis.Client
	local   *localIdemStore
	lockTTL time.Duration // 占位（在途）TTL，应大于写请求最大耗时
	doneTTL time.Duration // 完成记录保留窗口，重放有效期
}

// NewIdempotency 创建幂等中间件。client 为 nil 时全程走进程内 TTL 表。
func NewIdempotency(client *redis.Client) *Idempotency {
	lockTTL := time.Duration(envInt("IDEMPOTENCY_LOCK_TTL_SEC", 60)) * time.Second
	doneTTL := time.Duration(envInt("IDEMPOTENCY_DONE_TTL_SEC", 24*3600)) * time.Second
	return &Idempotency{
		redis:   client,
		local:   newLocalIdemStore(lockTTL),
		lockTTL: lockTTL,
		doneTTL: doneTTL,
	}
}

// Apply 返回幂等中间件。仅写方法 + 携带 Idempotency-Key 头才介入。
func (m *Idempotency) Apply() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isWriteMethod(c.Request.Method) {
			c.Next()
			return
		}
		key := c.GetHeader(idemHeader)
		if key == "" {
			c.Next()
			return
		}
		// SSE/流式响应不做 body 缓冲。
		if p := c.Request.URL.Path; containsSub(p, "/stream") {
			c.Next()
			return
		}

		full := m.compositeKey(c, key)
		first, existing, err := m.begin(c.Request.Context(), full)
		if err != nil {
			// 中间件自身故障不阻断业务：fail-open + 记日志。
			log.Printf("[Idempotency] 后端故障，本次放行(fail-open): key=%s err=%v", full, err)
			c.Next()
			return
		}
		if !first {
			m.serveExisting(c, existing)
			return
		}

		// 首个请求：捕获响应后落库。
		bcw := &bodyCaptureWriter{ResponseWriter: c.Writer, buf: &bytes.Buffer{}}
		c.Writer = bcw
		c.Next()

		status := c.Writer.Status()
		if status >= 500 {
			// 服务端错误不缓存，删占位以放行后续重试。
			m.release(context.Background(), full)
			return
		}
		rec := idemRecord{
			State:       idemStateDon,
			Status:      status,
			ContentType: c.Writer.Header().Get("Content-Type"),
			Body:        bcw.buf.Bytes(),
		}
		m.complete(context.Background(), full, rec)
	}
}

// serveExisting 处理命中既有记录：done 回放、processing 返回 409。
func (m *Idempotency) serveExisting(c *gin.Context, rec *idemRecord) {
	if rec == nil || rec.State == idemStateNew {
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{
			"code":       http.StatusConflict,
			"message":    "相同 Idempotency-Key 的请求正在处理中，请稍后重试",
			"request_id": c.GetString("request_id"),
		})
		return
	}
	// 回放缓存响应快照。
	ct := rec.ContentType
	if ct == "" {
		ct = "application/json; charset=utf-8"
	}
	c.Header("Idempotency-Replayed", "true")
	c.Data(rec.Status, ct, rec.Body)
	c.Abort()
}

// compositeKey 组装去重键：租户 + 用户 + 方法 + 路径 + 客户端键。
func (m *Idempotency) compositeKey(c *gin.Context, key string) string {
	tenant := c.GetInt64("tenant_id")
	user := c.GetInt64("user_id")
	return "idem:" +
		int64Str(tenant) + ":" +
		int64Str(user) + ":" +
		c.Request.Method + ":" +
		c.FullPath() + ":" +
		key
}

// begin 尝试占位。first=true 表示本请求为首个（需执行业务）；否则 existing 为既有记录。
func (m *Idempotency) begin(ctx context.Context, key string) (first bool, existing *idemRecord, err error) {
	if m.redis == nil {
		f, ex := m.local.begin(key)
		return f, ex, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	placeholder, _ := json.Marshal(idemRecord{State: idemStateNew})
	// SET key placeholder NX EX：占位成功返回 OK，键已存在则返回 redis.Nil（非错误）。
	// 用 SetArgs 替代已弃用的 SetNX（SA1019）。
	err = m.redis.SetArgs(ctx, key, placeholder, redis.SetArgs{Mode: "NX", TTL: m.lockTTL}).Err()
	if err == nil {
		return true, nil, nil
	}
	if err != redis.Nil {
		return false, nil, err
	}
	// 已存在：读取既有记录。
	val, err := m.redis.Get(ctx, key).Bytes()
	if err == redis.Nil {
		// 占位刚被删（如首个请求 5xx 释放）：视为可执行，fail-open 放行。
		return true, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	var rec idemRecord
	if e := json.Unmarshal(val, &rec); e != nil {
		return false, nil, e
	}
	return false, &rec, nil
}

// complete 覆盖为完成记录（含响应快照）。
func (m *Idempotency) complete(ctx context.Context, key string, rec idemRecord) {
	if m.redis == nil {
		m.local.complete(key, rec, m.doneTTL)
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	b, _ := json.Marshal(rec)
	if err := m.redis.Set(ctx, key, b, m.doneTTL).Err(); err != nil {
		log.Printf("[Idempotency] 写完成记录失败: key=%s err=%v", key, err)
	}
}

// release 删除占位（5xx 时放行后续重试）。
func (m *Idempotency) release(ctx context.Context, key string) {
	if m.redis == nil {
		m.local.release(key)
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	m.redis.Del(ctx, key)
}

// ========================================
// bodyCaptureWriter 响应体旁路捕获
// ========================================

type bodyCaptureWriter struct {
	gin.ResponseWriter
	buf *bytes.Buffer
}

func (w *bodyCaptureWriter) Write(b []byte) (int, error) {
	w.buf.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *bodyCaptureWriter) WriteString(s string) (int, error) {
	w.buf.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

// ========================================
// 进程内 TTL 降级实现
// ========================================

type localIdemEntry struct {
	rec idemRecord
	exp time.Time
}

type localIdemStore struct {
	mu      sync.Mutex
	m       map[string]*localIdemEntry
	lockTTL time.Duration
}

func newLocalIdemStore(lockTTL time.Duration) *localIdemStore {
	s := &localIdemStore{m: make(map[string]*localIdemEntry), lockTTL: lockTTL}
	go s.janitor()
	return s
}

// begin 进程内占位：first=true 表示新占位成功。
func (s *localIdemStore) begin(key string) (bool, *idemRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.m[key]; e != nil && time.Now().Before(e.exp) {
		r := e.rec
		return false, &r
	}
	s.m[key] = &localIdemEntry{
		rec: idemRecord{State: idemStateNew},
		exp: time.Now().Add(s.lockTTL),
	}
	return true, nil
}

func (s *localIdemStore) complete(key string, rec idemRecord, ttl time.Duration) {
	s.mu.Lock()
	s.m[key] = &localIdemEntry{rec: rec, exp: time.Now().Add(ttl)}
	s.mu.Unlock()
}

func (s *localIdemStore) release(key string) {
	s.mu.Lock()
	delete(s.m, key)
	s.mu.Unlock()
}

func (s *localIdemStore) janitor() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		s.mu.Lock()
		for k, e := range s.m {
			if now.After(e.exp) {
				delete(s.m, k)
			}
		}
		s.mu.Unlock()
	}
}

// ========================================
// 小工具
// ========================================

func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func containsSub(s, sub string) bool {
	return strings.Contains(s, sub)
}

func int64Str(v int64) string {
	return strconv.FormatInt(v, 10)
}
