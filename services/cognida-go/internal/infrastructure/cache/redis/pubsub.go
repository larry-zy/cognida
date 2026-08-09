// Package redis 提供 Redis 发布订阅操作实现
package redis

import (
	"context"

	"github.com/redis/go-redis/v9"

	"cognida/internal/model/cache"
)

// ========================================
// PubSub Implementation
// ========================================

// pubSub 实现 PubSub 接口
type pubSub struct {
	client *redis.Client
}

// NewPubSub 创建发布订阅实例
func NewPubSub(client *redis.Client) cache.PubSub {
	return &pubSub{client: client}
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

	// 创建消息通道
	msgChan := make(chan *cache.Message, 100)

	// 启动 goroutine 转发消息
	go func() {
		defer close(msgChan)
		for {
			select {
			case <-ctx.Done():
				sub.Close()
				return
			case msg, ok := <-sub.Channel():
				if !ok {
					return
				}
				msgChan <- &cache.Message{
					Channel: msg.Channel,
					Payload: msg.Payload,
				}
			}
		}
	}()

	return msgChan, nil
}

// Unsubscribe 取消订阅频道
func (p *pubSub) Unsubscribe(ctx context.Context, channels ...string) error {
	if len(channels) == 0 {
		return nil
	}

	sub := p.client.Subscribe(ctx, channels...)
	defer sub.Close()

	// Unsubscribe 不返回错误
	sub.Unsubscribe(ctx, channels...)
	return nil
}

// PSubscribe 模式订阅
func (p *pubSub) PSubscribe(ctx context.Context, patterns ...string) (<-chan *cache.Message, error) {
	if len(patterns) == 0 {
		return nil, cache.ErrChannelEmpty
	}

	sub := p.client.PSubscribe(ctx, patterns...)

	msgChan := make(chan *cache.Message, 100)

	go func() {
		defer close(msgChan)
		for {
			select {
			case <-ctx.Done():
				sub.Close()
				return
			case msg, ok := <-sub.Channel():
				if !ok {
					return
				}
				msgChan <- &cache.Message{
					Channel: msg.Channel,
					Payload: msg.Payload,
				}
			}
		}
	}()

	return msgChan, nil
}

// PUnsubscribe 取消模式订阅
func (p *pubSub) PUnsubscribe(ctx context.Context, patterns ...string) error {
	if len(patterns) == 0 {
		return nil
	}

	sub := p.client.PSubscribe(ctx, patterns...)
	defer sub.Close()

	// PUnsubscribe 不返回错误
	sub.PUnsubscribe(ctx, patterns...)
	return nil
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
