package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
//     Redis 运行期报错拒绝请求，不能把去重不可用视为允许重复写。
//   - 跳过 /stream 路径：SSE 长流不宜整体缓冲进内存。

const (
	idemHeader   = "Idempotency-Key"
	idemStateNew = "processing"
	idemStateDon = "done"
)

// idemRecord binds the request digest and claim owner in both states.
type idemRecord struct {
	State       string `json:"state"`
	Digest      string `json:"digest"`
	Owner       string `json:"owner"`
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
		// Private memories implement durable, payload-aware idempotency themselves.
		// Never retain or replay their content after an update/deletion.
		path := c.FullPath()
		// Governed task drafts likewise own durable payload-aware idempotency.
		// Preflight and retries must recheck current source permissions, not replay
		// a cached validation or hide a same-key/different-plan conflict.
		if ownsDurableIdempotency(path) {
			c.Next()
			return
		}
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

		digest, err := idempotencyDigest(c.Request)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"code": 413, "message": "无法读取有界幂等请求正文"})
			return
		}
		claim := idemRecord{State: idemStateNew, Digest: digest, Owner: uuid.NewString()}
		full := m.compositeKey(c, key)
		first, existing, err := m.begin(c.Request.Context(), full, claim)
		if err != nil {
			log.Printf("[Idempotency] 后端不可用，请求未执行")
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "幂等状态不可用，请稍后重试"})
			return
		}
		if !first {
			m.serveExisting(c, existing, digest)
			return
		}

		// 首个请求：捕获响应后落库。
		bcw := &bodyCaptureWriter{ResponseWriter: c.Writer, buf: &bytes.Buffer{}}
		c.Writer = bcw
		c.Next()

		status := c.Writer.Status()
		if status >= 500 {
			// 服务端错误不缓存，删占位以放行后续重试。
			m.release(context.Background(), full, claim.Owner)
			return
		}
		rec := idemRecord{
			State:       idemStateDon,
			Digest:      digest,
			Owner:       claim.Owner,
			Status:      status,
			ContentType: c.Writer.Header().Get("Content-Type"),
			Body:        bcw.buf.Bytes(),
		}
		m.complete(context.Background(), full, rec)
	}
}

// These domains own replay semantics or require current resource authorization
// inside their services. HTTP response caching would bypass those checks.
func ownsDurableIdempotency(path string) bool {
	for _, root := range []string{"messages", "datasources", "huggingface", "label-standards", "sft-datasets", "agent/config", "memories", "governed-tasks", "lifecycle", "features", "feature-values", "feature-jobs", "drift", "annotation-tasks", "tagging-tasks", "tagging-results", "research/runs", "evaluation/dataset-versions", "collection-tasks", "sandbox", "warehouse", "api-credentials", "api-sources", "api-ingestion-runs", "strategies", "operations", "query-jobs", "feedback-cases", "feedback-publications", "quality/evaluate/datasource"} {
		prefix := "/api/v1/" + root
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

// serveExisting 处理命中既有记录：done 回放、processing 返回 409。
func (m *Idempotency) serveExisting(c *gin.Context, rec *idemRecord, digest string) {
	if rec != nil && rec.Digest != digest {
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"code": 409, "message": "同一 Idempotency-Key 已绑定不同请求"})
		return
	}
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
	identity, _ := json.Marshal([]string{int64Str(tenant), int64Str(user), c.Request.Method, c.Request.URL.EscapedPath(), key})
	sum := sha256.Sum256(identity)
	return "idem:v2:" + hex.EncodeToString(sum[:])
}

// idempotencyDigest binds raw request bytes plus query and content semantics.
// Body is restored for the handler. Different JSON formatting is intentionally
// a different request; no field can disappear during JSON normalization.
func idempotencyDigest(r *http.Request) (string, error) {
	const maxBody = 128 << 20
	var body []byte
	var err error
	if r.Body != nil {
		body, err = io.ReadAll(io.LimitReader(r.Body, maxBody+1))
		r.Body.Close()
		if err != nil {
			return "", err
		}
		if len(body) > maxBody {
			return "", fmt.Errorf("request body limit")
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	header, _ := json.Marshal([]string{r.URL.RawQuery, r.Header.Get("Content-Type"), r.Header.Get("Content-Encoding")})
	h := sha256.New()
	h.Write(header)
	h.Write([]byte{0})
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil)), nil
}

var idemBeginScript = redis.NewScript(`
local previous = redis.call('GET', KEYS[1])
if previous then return previous end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
return ''
`)
var idemCompleteScript = redis.NewScript(`
local previous = redis.call('GET', KEYS[1])
if not previous then return 0 end
local record = cjson.decode(previous)
if record.owner ~= ARGV[1] or record.state ~= 'processing' then return 0 end
redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
return 1
`)
var idemReleaseScript = redis.NewScript(`
local previous = redis.call('GET', KEYS[1])
if not previous then return 0 end
local record = cjson.decode(previous)
if record.owner ~= ARGV[1] or record.state ~= 'processing' then return 0 end
return redis.call('DEL', KEYS[1])
`)

func (m *Idempotency) begin(ctx context.Context, key string, claim idemRecord) (bool, *idemRecord, error) {
	if m.redis == nil {
		f, ex := m.local.begin(key, claim)
		return f, ex, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	raw, _ := json.Marshal(claim)
	val, err := idemBeginScript.Run(ctx, m.redis, []string{key}, string(raw), m.lockTTL.Milliseconds()).Text()
	if err != nil {
		return false, nil, err
	}
	if val == "" {
		return true, nil, nil
	}
	var rec idemRecord
	if err = json.Unmarshal([]byte(val), &rec); err != nil {
		return false, nil, err
	}
	if rec.Owner == "" || rec.Digest == "" || (rec.State != idemStateNew && rec.State != idemStateDon) {
		return false, nil, fmt.Errorf("invalid idempotency record")
	}
	return false, &rec, nil
}
func (m *Idempotency) complete(ctx context.Context, key string, rec idemRecord) {
	if m.redis == nil {
		m.local.complete(key, rec, m.doneTTL)
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	raw, _ := json.Marshal(rec)
	if err := idemCompleteScript.Run(ctx, m.redis, []string{key}, rec.Owner, string(raw), m.doneTTL.Milliseconds()).Err(); err != nil {
		log.Printf("[Idempotency] 完成回执未确认")
	}
}
func (m *Idempotency) release(ctx context.Context, key, owner string) {
	if m.redis == nil {
		m.local.release(key, owner)
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	if err := idemReleaseScript.Run(ctx, m.redis, []string{key}, owner).Err(); err != nil {
		log.Printf("[Idempotency] 释放占位未确认")
	}
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
	mu        sync.Mutex
	m         map[string]*localIdemEntry
	lockTTL   time.Duration
	nextSweep time.Time
}

func newLocalIdemStore(lockTTL time.Duration) *localIdemStore {
	s := &localIdemStore{m: make(map[string]*localIdemEntry), lockTTL: lockTTL}
	// Expired entries are pruned during requests; no unowned background goroutine.
	return s
}

// begin 进程内占位：first=true 表示新占位成功。
func (s *localIdemStore) begin(key string, claim idemRecord) (bool, *idemRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if !now.Before(s.nextSweep) {
		for k, e := range s.m {
			if !now.Before(e.exp) {
				delete(s.m, k)
			}
		}
		s.nextSweep = now.Add(10 * time.Minute)
	}
	if e := s.m[key]; e != nil && time.Now().Before(e.exp) {
		r := e.rec
		return false, &r
	}
	s.m[key] = &localIdemEntry{
		rec: claim,
		exp: time.Now().Add(s.lockTTL),
	}
	return true, nil
}

func (s *localIdemStore) complete(key string, rec idemRecord, ttl time.Duration) {
	s.mu.Lock()
	if old := s.m[key]; old != nil && time.Now().Before(old.exp) && old.rec.Owner == rec.Owner && old.rec.State == idemStateNew {
		s.m[key] = &localIdemEntry{rec: rec, exp: time.Now().Add(ttl)}
	}
	s.mu.Unlock()
}

func (s *localIdemStore) release(key, owner string) {
	s.mu.Lock()
	if old := s.m[key]; old != nil && old.rec.Owner == owner && old.rec.State == idemStateNew {
		delete(s.m, key)
	}
	s.mu.Unlock()
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
