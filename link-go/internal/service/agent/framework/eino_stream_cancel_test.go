// SSE 感知取消测试：ctx 取消后不再向 channel 发送、上游停止（3.6.2）。
package framework

import (
	"context"
	"testing"
	"time"
)

// TestStreamInternal_CancelStopsSending 消费者读取两个 chunk 后取消 ctx 且不再读取。
// 旧实现的裸 `ch<-` 会让生产者永久阻塞（goroutine 泄漏、上游空跑）；
// 新实现所有发送带 <-ctx.Done() 分支，生产者应及时退出并 close(ch)。
func TestStreamInternal_CancelStopsSending(t *testing.T) {
	// 1000 个 chunk 确保取消发生在生产中途
	chunks := make([]string, 1000)
	for i := range chunks {
		chunks[i] = "x"
	}
	a := &agentImpl{
		name:  "t",
		model: &errStreamModel{chunks: chunks},
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan *Chunk) // 无缓冲：生产者必然阻塞在发送点

	go a.runStream(ctx, runRequest{message: "hi", rawMessage: "hi"}, ch)

	// 消费 start 事件 + 一个内容 chunk 后取消
	for i := 0; i < 2; i++ {
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatal("未收到初始 chunk")
		}
	}
	cancel()

	// 生产者应经 ctx.Done 分支退出并 close(ch)：带超时排空直到关闭
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // channel 已关闭 → runStream 已返回，上游停止 ✓
			}
		case <-deadline:
			t.Fatal("取消后 2s 内 channel 未关闭（生产者阻塞在 ch<-）")
		}
	}
}

// TestStreamWithTools_CancelStopsSending 工具调用路径同样感知取消。
func TestStreamWithTools_CancelStopsSending(t *testing.T) {
	chunks := make([]string, 1000)
	for i := range chunks {
		chunks[i] = "y"
	}
	a := &agentImpl{
		name:      "t",
		toolModel: &errStreamModel{chunks: chunks},
		maxIter:   3,
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan *Chunk)

	// runStream 自身 defer close(ch)，不再手工 close。
	go a.runStream(ctx, runRequest{message: "hi", rawMessage: "hi"}, ch)

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("未收到首个 chunk")
	}
	cancel()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("取消后 2s 内 runStream（工具路径）未返回")
		}
	}
}
