// streamAgentChunks 取消感知测试：客户端断开（request ctx 取消）后停止消费并返回（3.6.2）。
package handler

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	agentuc "cognida/internal/service/agent"
)

// TestStreamAgentChunks_CancelStopsConsuming chunkChan 永不关闭的场景下取消
// request ctx，streamAgentChunks 必须及时返回而非永久阻塞在接收上。
func TestStreamAgentChunks_CancelStopsConsuming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	ctx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest("POST", "/", nil).WithContext(ctx)

	chunkChan := make(chan *agentuc.ChatChunkDTO) // 模拟上游：永不关闭
	h := &AgentHandler{}

	done := make(chan agentStreamResult, 1)
	go func() {
		done <- h.streamAgentChunks(c, chunkChan, genUIOption{})
	}()

	// 先正常消费一个 chunk，确认循环已启动
	select {
	case chunkChan <- &agentuc.ChatChunkDTO{Content: "部分回答"}:
	case <-time.After(2 * time.Second):
		t.Fatal("streamAgentChunks 未开始消费")
	}

	cancel()

	select {
	case res := <-done:
		// 已消费的内容应保留在结果中（用于持久化部分回答）
		if res.Content != "部分回答" {
			t.Errorf("Content = %q, 期望 %q", res.Content, "部分回答")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("取消后 2s 内 streamAgentChunks 未返回")
	}

	// 取消后不再消费：向 channel 发送应一直阻塞
	select {
	case chunkChan <- &agentuc.ChatChunkDTO{Content: "不应被消费"}:
		t.Fatal("取消后仍在消费 chunkChan")
	case <-time.After(100 * time.Millisecond):
		// ✓ 无人接收
	}
}
