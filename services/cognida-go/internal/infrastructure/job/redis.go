package job

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var _ Queue = (*RedisQueue)(nil)

const jobLeaseDuration = time.Minute

type RedisQueue struct {
	client *redis.Client
	prefix string
}

func NewRedisQueue(client *redis.Client) *RedisQueue { return NewRedisQueueWithPrefix(client, "jobs") }
func NewRedisQueueWithPrefix(client *redis.Client, prefix string) *RedisQueue {
	return &RedisQueue{client: client, prefix: prefix}
}
func (q *RedisQueue) ready() string           { return q.prefix + ":ready" }
func (q *RedisQueue) processing() string      { return q.prefix + ":processing" }
func (q *RedisQueue) delayed() string         { return q.prefix + ":delayed" }
func (q *RedisQueue) marker(id string) string { return q.prefix + ":id:" + id }
func (q *RedisQueue) leases() string          { return q.prefix + ":leases" }
func (q *RedisQueue) owners() string          { return q.prefix + ":owners" }

// The marker and delivery are committed atomically. A process crash can never
// leave a permanent idempotency marker without the corresponding job.
const enqueueScript = `if redis.call('EXISTS',KEYS[1])==1 then return 0 end
if ARGV[2]=='ready' then redis.call('LPUSH',KEYS[2],ARGV[1]) else redis.call('ZADD',KEYS[2],ARGV[2],ARGV[1]) end
redis.call('SET',KEYS[1],'1');return 1`

func (q *RedisQueue) Enqueue(ctx context.Context, j Job) error {
	return q.enqueue(ctx, j, q.ready(), "ready")
}
func (q *RedisQueue) Schedule(ctx context.Context, j Job, delay time.Duration) error {
	if delay < 0 {
		delay = 0
	}
	return q.enqueue(ctx, j, q.delayed(), fmt.Sprint(time.Now().Add(delay).UnixMilli()))
}
func (q *RedisQueue) enqueue(ctx context.Context, j Job, destination, score string) error {
	if err := valid(j); err != nil {
		return err
	}
	body, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return q.client.Eval(ctx, enqueueScript, []string{q.marker(j.ID), destination}, string(body), score).Err()
}

// A unique delivery receipt fences stale workers after lease expiry. Claim,
// processing-list insertion and lease creation are one atomic operation.
const claimScript = `local body=redis.call('RPOP',KEYS[1]);if not body then return nil end
redis.call('LPUSH',KEYS[2],body);redis.call('HSET',KEYS[3],body,ARGV[1]);redis.call('ZADD',KEYS[4],ARGV[2],body);return body`

func (q *RedisQueue) TryClaim(ctx context.Context) (*Job, error) {
	receipt := uuid.NewString()
	raw, err := q.client.Eval(ctx, claimScript, []string{q.ready(), q.processing(), q.owners(), q.leases()}, receipt, time.Now().Add(jobLeaseDuration).UnixMilli()).Text()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim job: %w", err)
	}
	var j Job
	if err = json.Unmarshal([]byte(raw), &j); err == nil {
		err = valid(j)
	}
	j.Delivery = &deliveryReceipt{receipt: receipt, raw: raw}
	if err != nil {
		_ = q.Ack(ctx, j)
		return nil, fmt.Errorf("decode job: %w", err)
	}
	return &j, nil
}
func (q *RedisQueue) Claim(ctx context.Context, wait time.Duration) (*Job, error) {
	if wait <= 0 {
		wait = 5 * time.Second
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		j, err := q.TryClaim(ctx)
		if err != nil || j != nil {
			return j, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			return nil, nil
		case <-ticker.C:
		}
	}
}
func (q *RedisQueue) ReadySize(ctx context.Context) (int64, error) {
	return q.client.LLen(ctx, q.ready()).Result()
}

const purgeScript = `local all=redis.call('LRANGE',KEYS[1],0,-1);local processing=redis.call('LRANGE',KEYS[2],0,-1);local delayed=redis.call('ZRANGE',KEYS[3],0,-1)
for _,list in ipairs({all,processing,delayed}) do for _,body in ipairs(list) do local ok,j=pcall(cjson.decode,body);if ok and j.id then redis.call('DEL',ARGV[1]..j.id) end end end
return redis.call('DEL',unpack(KEYS))`

func (q *RedisQueue) Purge(ctx context.Context) error {
	return q.client.Eval(ctx, purgeScript, []string{q.ready(), q.processing(), q.delayed(), q.owners(), q.leases()}, q.marker("")).Err()
}

const ackScript = `if redis.call('HGET',KEYS[2],ARGV[1])~=ARGV[2] then return 0 end
redis.call('LREM',KEYS[1],1,ARGV[1]);redis.call('HDEL',KEYS[2],ARGV[1]);redis.call('ZREM',KEYS[3],ARGV[1]);redis.call('DEL',KEYS[4]);return 1`

func (q *RedisQueue) Ack(ctx context.Context, j Job) error {
	delivery, ok := j.Delivery.(*deliveryReceipt)
	if !ok || delivery == nil {
		return fmt.Errorf("job delivery lease is missing: %s", j.ID)
	}
	n, err := q.client.Eval(ctx, ackScript, []string{q.processing(), q.owners(), q.leases(), q.marker(j.ID)}, delivery.raw, delivery.receipt).Int()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("job delivery lease lost: %s", j.ID)
	}
	return nil
}

const retryScript = `if redis.call('HGET',KEYS[2],ARGV[1])~=ARGV[2] then return 0 end
redis.call('ZADD',KEYS[4],ARGV[4],ARGV[3]);redis.call('LREM',KEYS[1],1,ARGV[1]);redis.call('HDEL',KEYS[2],ARGV[1]);redis.call('ZREM',KEYS[3],ARGV[1]);return 1`

func (q *RedisQueue) Retry(ctx context.Context, j Job, delay time.Duration) error {
	delivery, ok := j.Delivery.(*deliveryReceipt)
	if !ok || delivery == nil {
		return fmt.Errorf("job delivery lease is missing: %s", j.ID)
	}
	if delay < 0 {
		delay = 0
	}
	j.Attempt++
	body, err := json.Marshal(j)
	if err != nil {
		return err
	}
	n, err := q.client.Eval(ctx, retryScript, []string{q.processing(), q.owners(), q.leases(), q.delayed()}, delivery.raw, delivery.receipt, string(body), time.Now().Add(delay).UnixMilli()).Int()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("job delivery lease lost: %s", j.ID)
	}
	return nil
}

const heartbeatScript = `if redis.call('HGET',KEYS[1],ARGV[1])~=ARGV[2] then return 0 end;redis.call('ZADD',KEYS[2],ARGV[3],ARGV[1]);return 1`

func (q *RedisQueue) Heartbeat(ctx context.Context, j Job) error {
	delivery, ok := j.Delivery.(*deliveryReceipt)
	if !ok || delivery == nil {
		return fmt.Errorf("job delivery lease is missing: %s", j.ID)
	}
	n, err := q.client.Eval(ctx, heartbeatScript, []string{q.owners(), q.leases()}, delivery.raw, delivery.receipt, time.Now().Add(jobLeaseDuration).UnixMilli()).Int()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("job delivery lease lost: %s", j.ID)
	}
	return nil
}

const promoteScript = `local d=redis.call('ZRANGEBYSCORE',KEYS[1],'-inf',ARGV[1],'LIMIT',0,ARGV[2]);for i=1,#d do if redis.call('ZREM',KEYS[1],d[i])==1 then redis.call('LPUSH',KEYS[2],d[i]) end end;return #d`

func (q *RedisQueue) PromoteDue(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	return q.client.Eval(ctx, promoteScript, []string{q.delayed(), q.ready()}, time.Now().UnixMilli(), limit).Int()
}

// Recover only expired (or pre-lease legacy) deliveries, never another live
// worker's jobs. Redis executes the scan and moves atomically.
const recoverScript = `local all=redis.call('LRANGE',KEYS[1],0,-1);local n=0
for _,body in ipairs(all) do local deadline=redis.call('ZSCORE',KEYS[4],body);if not deadline or tonumber(deadline)<=tonumber(ARGV[1]) then
if redis.call('LREM',KEYS[1],1,body)>0 then redis.call('LPUSH',KEYS[2],body);redis.call('HDEL',KEYS[3],body);redis.call('ZREM',KEYS[4],body);n=n+1 end end end;return n`

func (q *RedisQueue) Recover(ctx context.Context) (int, error) {
	return q.client.Eval(ctx, recoverScript, []string{q.processing(), q.ready(), q.owners(), q.leases()}, time.Now().UnixMilli()).Int()
}
func valid(j Job) error {
	if j.ID == "" || j.Type == "" || j.Attempt < 0 {
		return fmt.Errorf("invalid job")
	}
	return nil
}

// deliveryReceipt cannot be constructed by domain services.
type deliveryReceipt struct{ receipt, raw string }
