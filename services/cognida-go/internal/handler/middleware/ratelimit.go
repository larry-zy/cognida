package middleware

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

// envFloat 读取浮点型环境变量，缺省/非法时回退默认值。
func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f
		}
	}
	return def
}

// envInt 读取整型环境变量，缺省/非法时回退默认值。
func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// ========================================
// RateLimitMiddleware 限流中间件（Redis 令牌桶，Redis 不可用时降级为进程内令牌桶）
// ========================================
//
// 背景〔INF-2〕：此前 HTTP 层无任何限流——`/auth/login` 可被暴力破解；
// `agent/stream`、`rag`、`evaluation` 等昂贵 LLM 端点可被无限并发打爆（DoS + 成本失控）。
//
// 设计：
//   - 首选 Redis 令牌桶（Lua 原子执行，多实例共享配额）；
//   - Redis 未配置/失联时降级为进程内 golang.org/x/time/rate 令牌桶（单实例仍有防护）；
//   - Redis 运行期报错 fail-open（放行 + 记日志），限流器自身故障不应升级为全站不可用。
//
// 提供三档预设：Global（全局兜底）/ Login（登录暴破收紧）/ Expensive（昂贵端点）。

// tokenBucketScript 是 Redis 令牌桶的原子实现。
// KEYS[1]=桶键；ARGV[1]=速率(tokens/s)；ARGV[2]=容量(burst)；ARGV[3]=本次请求令牌数。
// 用 redis TIME 取服务端时钟，避免各应用实例时钟漂移；返回 1=放行 0=拒绝。
var tokenBucketScript = redis.NewScript(`
local key = KEYS[1]
local rate = tonumber(ARGV[1])
local capacity = tonumber(ARGV[2])
local requested = tonumber(ARGV[3])

local t = redis.call('TIME')
local now_ms = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)

local data = redis.call('HMGET', key, 'tokens', 'ts')
local tokens = tonumber(data[1])
local ts = tonumber(data[2])
if tokens == nil then tokens = capacity end
if ts == nil then ts = now_ms end

local delta = now_ms - ts
if delta < 0 then delta = 0 end
local filled = math.min(capacity, tokens + delta * rate / 1000.0)

local allowed = 0
if filled >= requested then
  allowed = 1
  filled = filled - requested
end

redis.call('HSET', key, 'tokens', filled, 'ts', now_ms)
local ttl = math.ceil(capacity / rate) + 1
redis.call('EXPIRE', key, ttl)
return allowed
`)

// RateLimiter 限流器：持有 Redis 客户端与进程内降级桶。
type RateLimiter struct {
	redis *redis.Client
	local *localLimiterSet
}

// NewRateLimiter 创建限流器。client 为 nil 时全程走进程内令牌桶。
func NewRateLimiter(client *redis.Client) *RateLimiter {
	return &RateLimiter{
		redis: client,
		local: newLocalLimiterSet(),
	}
}

// allow 判定某 key 在给定速率/容量下是否放行。
func (rl *RateLimiter) allow(ctx context.Context, key string, ratePerSec float64, burst int) bool {
	if rl.redis != nil {
		ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()
		res, err := tokenBucketScript.Run(ctx, rl.redis,
			[]string{key},
			ratePerSec, burst, 1,
		).Int()
		if err != nil {
			// 限流器自身故障不阻断业务：fail-open + 记日志。
			log.Printf("[RateLimit] Redis 令牌桶执行失败，本次放行(fail-open): key=%s err=%v", key, err)
			return true
		}
		return res == 1
	}
	// 进程内降级路径。
	return rl.local.allow(key, rate.Limit(ratePerSec), burst)
}

// limit 构造一个限流 gin 中间件。keyFn 生成限流维度键（通常含客户端 IP）。
func (rl *RateLimiter) limit(bucket string, ratePerSec float64, burst int, keyFn func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := "ratelimit:" + bucket + ":" + keyFn(c)
		if !rl.allow(c.Request.Context(), key, ratePerSec, burst) {
			retry := 1
			if ratePerSec > 0 {
				if r := int(1.0 / ratePerSec); r > retry {
					retry = r
				}
			}
			c.Header("Retry-After", strconv.Itoa(retry))
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "请求过于频繁，请稍后重试"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// ipKey 以客户端 IP 作为限流维度。
func ipKey(c *gin.Context) string { return c.ClientIP() }

// Global 全局兜底限流（每 IP）：默认 50 rps、突发 100。
// 跳过 /health 与静态资源，避免健康探针/前端资源被限。
func (rl *RateLimiter) Global() gin.HandlerFunc {
	ratePerSec := envFloat("RATE_LIMIT_GLOBAL_RPS", 50)
	burst := envInt("RATE_LIMIT_GLOBAL_BURST", 100)
	inner := rl.limit("global", ratePerSec, burst, ipKey)
	return func(c *gin.Context) {
		p := c.Request.URL.Path
		if p == "/health" || len(p) >= 7 && p[:7] == "/assets" {
			c.Next()
			return
		}
		inner(c)
	}
}

// Login 登录/注册端点收紧限流（每 IP）：默认 5 次/分钟、突发 5，专防暴力破解。
func (rl *RateLimiter) Login() gin.HandlerFunc {
	perMin := envFloat("RATE_LIMIT_LOGIN_PER_MIN", 5)
	burst := envInt("RATE_LIMIT_LOGIN_BURST", 5)
	return rl.limit("login", perMin/60.0, burst, ipKey)
}

// Expensive 昂贵端点限流（每 IP）：默认 2 rps、突发 5，护住 LLM/检索/评测端点。
func (rl *RateLimiter) Expensive() gin.HandlerFunc {
	ratePerSec := envFloat("RATE_LIMIT_EXPENSIVE_RPS", 2)
	burst := envInt("RATE_LIMIT_EXPENSIVE_BURST", 5)
	return rl.limit("expensive", ratePerSec, burst, ipKey)
}

// ========================================
// 进程内令牌桶降级实现
// ========================================

type localBucket struct {
	lim  *rate.Limiter
	seen time.Time
}

type localLimiterSet struct {
	mu      sync.Mutex
	buckets map[string]*localBucket
}

func newLocalLimiterSet() *localLimiterSet {
	s := &localLimiterSet{buckets: make(map[string]*localBucket)}
	go s.janitor()
	return s
}

func (s *localLimiterSet) allow(key string, r rate.Limit, burst int) bool {
	s.mu.Lock()
	b := s.buckets[key]
	if b == nil {
		b = &localBucket{lim: rate.NewLimiter(r, burst)}
		s.buckets[key] = b
	}
	b.seen = time.Now()
	s.mu.Unlock()
	return b.lim.Allow()
}

// janitor 周期性淘汰久未命中的桶，避免 map 无界增长。
func (s *localLimiterSet) janitor() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-30 * time.Minute)
		s.mu.Lock()
		for k, b := range s.buckets {
			if b.seen.Before(cutoff) {
				delete(s.buckets, k)
			}
		}
		s.mu.Unlock()
	}
}
