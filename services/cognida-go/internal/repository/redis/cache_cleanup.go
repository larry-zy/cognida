package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"cognida/internal/model/cache"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func cleanupFenceKey(v cache.CacheVectorIdentity) string {
	return fmt.Sprintf("semantic_cache:cleanup:%d:%s", v.TenantID, v.CacheID)
}

func (r *CacheContentRepository) ListCleanupFences(ctx context.Context, tenant int64, maximum, batch int) ([]cache.CacheCleanupFence, error) {
	if tenant < 1 || maximum < 1 || maximum > 100000 || batch < 1 || batch > 1000 {
		return nil, cache.ErrCleanupInvalid
	}
	var cursor uint64
	seen := make(map[string]bool)
	var out []cache.CacheCleanupFence
	for pages := 0; pages < 10000; pages++ {
		keys, next, err := r.client.Scan(ctx, cursor, fmt.Sprintf("semantic_cache:cleanup:%d:*", tenant), int64(batch)).Result()
		if err != nil {
			return nil, fmt.Errorf("enumerate cache cleanup fences: %w", err)
		}
		for _, key := range keys {
			if seen[key] {
				continue
			}
			seen[key] = true
			if len(seen) > maximum {
				return nil, cache.ErrCleanupIncomplete
			}
			raw, err := r.client.Get(ctx, key).Bytes()
			if err == redis.Nil {
				continue
			} // another acknowledged cleanup finished
			if err != nil {
				return nil, fmt.Errorf("read cache cleanup fence: %w", err)
			}
			var f cache.CacheCleanupFence
			if json.Unmarshal(raw, &f) != nil || f.Identity.Validate() != nil || f.Identity.TenantID != tenant || cleanupFenceKey(f.Identity) != key || f.Token == "" || (f.State != "active" && f.State != "failed" && f.State != "unknown") {
				return nil, cache.ErrCacheOwnership
			}
			out = append(out, f)
		}
		if next == 0 {
			return out, nil
		}
		cursor = next
	}
	return nil, cache.ErrCleanupIncomplete
}

func (r *CacheContentRepository) CleanupContentPresent(ctx context.Context, v cache.CacheVectorIdentity) (bool, error) {
	if err := v.Validate(); err != nil {
		return false, err
	}
	data, err := r.client.HGetAll(ctx, r.buildCacheKey(v.CacheID)).Result()
	if err != nil {
		return false, fmt.Errorf("read cache primary hash: %w", err)
	}
	if len(data) == 0 {
		return false, nil
	}
	tenant, e := strconv.ParseInt(data["tenant_id"], 10, 64)
	created, e2 := strconv.ParseInt(data["created_at"], 10, 64)
	if e != nil || e2 != nil || created < 1 || tenant != v.TenantID || data["cache_id"] != v.CacheID || data["agent_type"] != v.AgentType {
		return false, cache.ErrCacheOwnership
	}
	return true, nil
}

var claimCleanup = redis.NewScript(`
if redis.call('EXISTS',KEYS[1]) == 1 then
  local d=redis.call('HMGET',KEYS[1],'cache_id','tenant_id','agent_type','created_at')
  if d[1]~=ARGV[1] or d[2]~=ARGV[2] or d[3]~=ARGV[3] or not tonumber(d[4]) or tonumber(d[4])<1 then return {5,''} end
  return {0,''}
end
local old=redis.call('GET',KEYS[2])
if old then
  local ok,f=pcall(cjson.decode,old)
  if not ok or type(f.identity)~='table' or f.identity.cache_id~=ARGV[1] or tostring(f.identity.tenant_id)~=ARGV[2] or f.identity.agent_type~=ARGV[3] or tostring(f.identity.created_at)~=ARGV[4] then return {5,''} end
  if f.state=='unknown' then return {3,''} end
  if f.state~='failed' then return {4,''} end
end
redis.call('SET',KEYS[2],ARGV[5])
return {1,ARGV[5]}
`)

func (r *CacheContentRepository) ClaimCleanup(ctx context.Context, v cache.CacheVectorIdentity) (*cache.CacheCleanupFence, error) {
	if err := v.Validate(); err != nil {
		return nil, err
	}
	fence := &cache.CacheCleanupFence{Identity: v, Token: uuid.NewString(), State: "active"}
	raw, err := json.Marshal(fence)
	if err != nil {
		return nil, err
	}
	values, err := claimCleanup.Run(ctx, r.client, []string{r.buildCacheKey(v.CacheID), cleanupFenceKey(v)}, v.CacheID, strconv.FormatInt(v.TenantID, 10), v.AgentType, strconv.FormatInt(v.CreatedAt, 10), string(raw)).Slice()
	if err != nil {
		return nil, fmt.Errorf("claim cache cleanup fence: %w", err)
	}
	if len(values) != 2 {
		return nil, cache.ErrCleanupReconcile
	}
	code, ok := values[0].(int64)
	if !ok {
		return nil, cache.ErrCleanupReconcile
	}
	switch code {
	case 0:
		return nil, nil
	case 1:
		return fence, nil
	case 3:
		return nil, cache.ErrCleanupReconcile
	case 4:
		return nil, cache.ErrCleanupBusy
	default:
		return nil, cache.ErrCacheOwnership
	}
}

var finishCleanup = redis.NewScript(`
local raw=redis.call('GET',KEYS[1])
if not raw then return 0 end
local ok,old=pcall(cjson.decode,raw)
local f=cjson.decode(ARGV[1])
if not ok or old.token~=f.token or old.state~='active' or type(old.identity)~='table' or old.identity.cache_id~=f.identity.cache_id or old.identity.tenant_id~=f.identity.tenant_id or old.identity.agent_type~=f.identity.agent_type or old.identity.created_at~=f.identity.created_at then return 0 end
if ARGV[2]=='deleted' then redis.call('DEL',KEYS[1]) else f.state=ARGV[2];redis.call('SET',KEYS[1],cjson.encode(f)) end
return 1
`)

func (r *CacheContentRepository) FinishCleanup(ctx context.Context, f cache.CacheCleanupFence, state string) error {
	if f.Identity.Validate() != nil || f.Token == "" || (state != "deleted" && state != "failed" && state != "unknown") {
		return cache.ErrCleanupInvalid
	}
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	n, err := finishCleanup.Run(ctx, r.client, []string{cleanupFenceKey(f.Identity)}, string(raw), state).Int()
	if err != nil {
		return fmt.Errorf("persist cache cleanup outcome: %w", err)
	}
	if n != 1 {
		return cache.ErrCleanupReconcile
	}
	return nil
}
