// Package redis 提供 Redis 发布订阅单元测试
package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cognida/internal/model/cache"
)

func setupPubSub(t *testing.T) (*miniredis.Miniredis, cache.PubSub) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	return mr, NewPubSub(client)
}

func TestPubSub_Publish(t *testing.T) {
	mr, ps := setupPubSub(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("发布消息到频道", func(t *testing.T) {
		err := ps.Publish(ctx, "channel1", "hello world")
		assert.NoError(t, err)
	})

	t.Run("发布不同类型的消息", func(t *testing.T) {
		err := ps.Publish(ctx, "channel1", "string message")
		assert.NoError(t, err)

		err = ps.Publish(ctx, "channel2", 12345)
		assert.NoError(t, err)

		err = ps.Publish(ctx, "channel3", true)
		assert.NoError(t, err)
	})

	t.Run("空频道名", func(t *testing.T) {
		err := ps.Publish(ctx, "", "message")
		assert.ErrorIs(t, err, cache.ErrChannelEmpty)
	})
}

func TestPubSub_Subscribe(t *testing.T) {
	mr, ps := setupPubSub(t)
	defer mr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	t.Run("订阅频道并接收消息", func(t *testing.T) {
		// 订阅频道
		msgChan, err := ps.Subscribe(ctx, "channel1")
		require.NoError(t, err)
		defer cancel()

		// 发布消息
		err = ps.Publish(ctx, "channel1", "test message")
		require.NoError(t, err)

		// 接收消息
		select {
		case msg := <-msgChan:
			assert.Equal(t, "channel1", msg.Channel)
			assert.Equal(t, "test message", msg.Payload)
		case <-time.After(time.Second * 2):
			t.Fatal("timeout waiting for message")
		}
	})

	t.Run("空频道列表", func(t *testing.T) {
		_, err := ps.Subscribe(ctx)
		assert.ErrorIs(t, err, cache.ErrChannelEmpty)
	})
}

func TestPubSub_Unsubscribe(t *testing.T) {
	mr, ps := setupPubSub(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("取消订阅", func(t *testing.T) {
		// 订阅频道
		_, err := ps.Subscribe(ctx, "channel1")
		require.NoError(t, err)

		// 取消订阅 - 不应该返回错误
		err = ps.Unsubscribe(ctx, "channel1")
		assert.NoError(t, err)
	})

	t.Run("取消订阅不存在的频道", func(t *testing.T) {
		err := ps.Unsubscribe(ctx, "nonexistent")
		assert.NoError(t, err) // 不应该返回错误
	})

	t.Run("取消订阅空频道列表", func(t *testing.T) {
		err := ps.Unsubscribe(ctx)
		assert.NoError(t, err)
	})
}

func TestPubSub_Unsubscribe_ClosesLiveSubscription(t *testing.T) {
	// H10 回归：Unsubscribe 必须作用于 Subscribe 建立的真实连接。
	// 旧实现另开一条无关连接 unsubscribe，对活跃订阅零影响（空操作）。
	// 用 background ctx 订阅（不靠 ctx 取消），断言 Unsubscribe 后转发 goroutine
	// 退出、msgChan 关闭。
	mr, ps := setupPubSub(t)
	defer mr.Close()

	ctx := context.Background()

	msgChan, err := ps.Subscribe(ctx, "channel1")
	require.NoError(t, err)

	err = ps.Unsubscribe(ctx, "channel1")
	require.NoError(t, err)

	select {
	case _, ok := <-msgChan:
		assert.False(t, ok, "Unsubscribe 后消息通道应关闭")
	case <-time.After(time.Second * 2):
		t.Fatal("Unsubscribe 未关闭订阅：msgChan 仍未关闭（转发 goroutine 泄漏）")
	}
}

func TestPubSub_Unsubscribe_KeepsRemainingChannels(t *testing.T) {
	// 多频道订阅只退订其一时，连接不应被关闭，msgChan 保持可用。
	mr, ps := setupPubSub(t)
	defer mr.Close()

	ctx := context.Background()

	msgChan, err := ps.Subscribe(ctx, "channel1", "channel2")
	require.NoError(t, err)

	err = ps.Unsubscribe(ctx, "channel1")
	require.NoError(t, err)

	// channel2 仍在订阅：通道不应关闭
	select {
	case _, ok := <-msgChan:
		if !ok {
			t.Fatal("仍有 channel2 订阅时不应关闭 msgChan")
		}
	case <-time.After(time.Millisecond * 100):
		// 无消息、通道未关闭，符合预期
	}
}

func TestPubSub_PSubscribe(t *testing.T) {
	mr, ps := setupPubSub(t)
	defer mr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	t.Run("空模式列表", func(t *testing.T) {
		_, err := ps.PSubscribe(ctx)
		assert.ErrorIs(t, err, cache.ErrChannelEmpty)
	})
}

func TestPubSub_PUnsubscribe(t *testing.T) {
	mr, ps := setupPubSub(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("取消订阅不存在的模式", func(t *testing.T) {
		err := ps.PUnsubscribe(ctx, "nonexistent:*")
		assert.NoError(t, err)
	})

	t.Run("空模式列表", func(t *testing.T) {
		err := ps.PUnsubscribe(ctx)
		assert.NoError(t, err)
	})
}

func TestPubSub_PubSubChannels(t *testing.T) {
	mr, ps := setupPubSub(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("获取活跃频道", func(t *testing.T) {
		// 订阅一些频道
		subCtx, subCancel := context.WithCancel(context.Background())
		defer subCancel()

		_, err := ps.Subscribe(subCtx, "channel1", "channel2", "channel3")
		require.NoError(t, err)

		// 给一些时间让订阅生效
		time.Sleep(time.Millisecond * 50)

		// 获取活跃频道
		channels, err := ps.PubSubChannels(ctx, "")
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(channels), 3)
		assert.Contains(t, channels, "channel1")
		assert.Contains(t, channels, "channel2")
		assert.Contains(t, channels, "channel3")
	})
}

func TestPubSub_PubSubNumSub(t *testing.T) {
	mr, ps := setupPubSub(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("获取频道订阅者数量", func(t *testing.T) {
		// 创建多个订阅
		subCtx1, cancel1 := context.WithCancel(context.Background())
		defer cancel1()
		_, err := ps.Subscribe(subCtx1, "channel1")
		require.NoError(t, err)

		subCtx2, cancel2 := context.WithCancel(context.Background())
		defer cancel2()
		_, err = ps.Subscribe(subCtx2, "channel1", "channel2")
		require.NoError(t, err)

		time.Sleep(time.Millisecond * 50)

		// 获取订阅者数量
		nums, err := ps.PubSubNumSub(ctx, "channel1", "channel2", "channel3")
		assert.NoError(t, err)

		// channel1 有2个订阅者
		assert.GreaterOrEqual(t, nums["channel1"], int64(2))
		// channel2 有1个订阅者
		assert.GreaterOrEqual(t, nums["channel2"], int64(1))
		// channel3 没有订阅者
		assert.Equal(t, int64(0), nums["channel3"])
	})
}

func TestPubSub_PubSubNumPat(t *testing.T) {
	mr, ps := setupPubSub(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("获取模式订阅数量", func(t *testing.T) {
		subCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// 创建几个模式订阅
		_, err := ps.PSubscribe(subCtx, "user:*", "admin:*", "session:*")
		require.NoError(t, err)

		time.Sleep(time.Millisecond * 50)

		// 获取模式订阅数量
		num, err := ps.PubSubNumPat(ctx)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, num, int64(3))
	})
}

func TestPubSub_ContextCancellation(t *testing.T) {
	mr, ps := setupPubSub(t)
	defer mr.Close()

	t.Run("上下文取消关闭订阅", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		// 订阅频道
		msgChan, err := ps.Subscribe(ctx, "channel1")
		require.NoError(t, err)

		// 取消上下文
		cancel()

		// 消息通道应该关闭
		select {
		case msg, ok := <-msgChan:
			if !ok {
				// 通道已关闭，正确
				return
			}
			t.Errorf("unexpected message: %v", msg)
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for channel close")
		}
	})
}
