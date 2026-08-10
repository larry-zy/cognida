// Package redis 提供 Redis 发布订阅操作实现
package redis

import (
	"context"
	"sync"

	"github.com/redis/go-redis/v9"

	"cognida/internal/model/cache"
	"cognida/internal/pkg/safego"
)

// ========================================
// PubSub Implementation
// ========================================

// liveSub 记录一次 Subscribe/PSubscribe 建立的活跃订阅，供 Unsubscribe 反查。
// names 为该订阅当前仍生效的频道/模式集合，仅在 pubSub.mu 下读写。
type liveSub struct {
	sub     *redis.PubSub
	names   map[string]struct{}
	pattern bool
}

// pubSub 实现 PubSub 接口。
// 维护一份活跃订阅注册表：Subscribe 建立的每个底层连接登记在册，
// Unsubscribe 据此定位真正的连接下发 UNSUBSCRIBE，而非另开一条无关连接（旧实现的空操作 bug）。
type pubSub struct {
	client *redis.Client
	mu     sync.Mutex
	subs   map[*liveSub]struct{}
}

// NewPubSub 创建发布订阅实例
func NewPubSub(client *redis.Client) cache.PubSub {
	return &pubSub{
		client: client,
		subs:   make(map[*liveSub]struct{}),
	}
}

// Publish 发布消息到频道
func (p *pubSub) Publish(ctx context.Context, channel string, message interface{}) error {
	if channel == "" {
		return cache.ErrChannelEmpty
	}

	return p.client.Publish(ctx, channel, message).Err()
}

// Subscribe 订阅频道
func (p *pubSub) Subscribe(ctx context.Context, channels ...string) (<-chan *cache.Message, error) {
	if len(channels) == 0 {
		return nil, cache.ErrChannelEmpty
	}

	sub := p.client.Subscribe(ctx, channels...)
	return p.register(ctx, sub, channels, false), nil
}

// PSubscribe 模式订阅
func (p *pubSub) PSubscribe(ctx context.Context, patterns ...string) (<-chan *cache.Message, error) {
	if len(patterns) == 0 {
		return nil, cache.ErrChannelEmpty
	}

	sub := p.client.PSubscribe(ctx, patterns...)
	return p.register(ctx, sub, patterns, true), nil
}

// register 登记订阅并启动转发 goroutine，返回消息通道。
// goroutine 在以下任一情况退出：ctx 取消、底层通道关闭（含被 Unsubscribe 关闭）。
// 退出时从注册表自摘除并关闭 msgChan，杜绝泄漏。
func (p *pubSub) register(ctx context.Context, sub *redis.PubSub, names []string, pattern bool) <-chan *cache.Message {
	ls := &liveSub{sub: sub, names: make(map[string]struct{}, len(names)), pattern: pattern}
	for _, n := range names {
		ls.names[n] = struct{}{}
	}

	p.mu.Lock()
	p.subs[ls] = struct{}{}
	p.mu.Unlock()

	msgChan := make(chan *cache.Message, 100)

	go func() {
		defer safego.Recover("pubsub-receive")
		defer close(msgChan)
		defer p.remove(ls)
		defer sub.Close()

		ch := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				// 送出也受 ctx 约束：消费方停读时不再永久阻塞在此（旧实现的 goroutine 泄漏根因）
				select {
				case msgChan <- &cache.Message{Channel: msg.Channel, Payload: msg.Payload}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return msgChan
}

// remove 从注册表摘除订阅（幂等）。
func (p *pubSub) remove(ls *liveSub) {
	p.mu.Lock()
	delete(p.subs, ls)
	p.mu.Unlock()
}

// Unsubscribe 取消订阅频道
func (p *pubSub) Unsubscribe(ctx context.Context, channels ...string) error {
	if len(channels) == 0 {
		return nil
	}
	return p.unsubscribe(ctx, channels, false)
}

// PUnsubscribe 取消模式订阅
func (p *pubSub) PUnsubscribe(ctx context.Context, patterns ...string) error {
	if len(patterns) == 0 {
		return nil
	}
	return p.unsubscribe(ctx, patterns, true)
}

// unsubscribe 在活跃订阅中定位命中的频道/模式，对真正的连接下发 UNSUBSCRIBE；
// 某订阅的频道被清空后关闭其连接，转发 goroutine 随即退出并自摘除。
func (p *pubSub) unsubscribe(ctx context.Context, names []string, pattern bool) error {
	type hit struct {
		ls      *liveSub
		matched []string
		drained bool // 该订阅已无剩余频道，需关闭连接
	}

	// 先在锁内定位命中并更新 names，Redis 调用移出锁外，避免持锁做网络 IO
	p.mu.Lock()
	var hits []hit
	for ls := range p.subs {
		if ls.pattern != pattern {
			continue
		}
		var matched []string
		for _, n := range names {
			if _, ok := ls.names[n]; ok {
				matched = append(matched, n)
				delete(ls.names, n)
			}
		}
		if len(matched) > 0 {
			hits = append(hits, hit{ls: ls, matched: matched, drained: len(ls.names) == 0})
		}
	}
	p.mu.Unlock()

	var firstErr error
	for _, h := range hits {
		var err error
		if pattern {
			err = h.ls.sub.PUnsubscribe(ctx, h.matched...)
		} else {
			err = h.ls.sub.Unsubscribe(ctx, h.matched...)
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
		if h.drained {
			// 关闭连接触发 goroutine 退出（其 defer 会摘除注册表并关闭 msgChan）
			_ = h.ls.sub.Close()
		}
	}
	return firstErr
}

// PubSubChannels 获取活跃频道列表
func (p *pubSub) PubSubChannels(ctx context.Context, pattern string) ([]string, error) {
	return p.client.PubSubChannels(ctx, pattern).Result()
}

// PubSubNumSub 获取频道订阅者数量
func (p *pubSub) PubSubNumSub(ctx context.Context, channels ...string) (map[string]int64, error) {
	return p.client.PubSubNumSub(ctx, channels...).Result()
}

// PubSubNumPat 获取模式订阅数量
func (p *pubSub) PubSubNumPat(ctx context.Context) (int64, error) {
	return p.client.PubSubNumPat(ctx).Result()
}
