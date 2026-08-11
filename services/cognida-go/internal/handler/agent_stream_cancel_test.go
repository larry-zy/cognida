// streamAgentChunks 取消感知测试：客户端断开（request ctx 取消）后停止消费并返回（3.6.2）。
package handler

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cognida/internal/handler/sse"
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
		done <- h.streamAgentChunks(c, sse.NewWriter(w), chunkChan, genUIOption{})
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

// TestStreamAgentChunks_ChannelClosedWithoutDoneStillEmitsDone 通道未发 Done 标记
// 即关闭（模拟 producer 发 Done 前异常退出/panic 被兜住）时，streamAgentChunks 必须
// 仍补发一条终局 done——保证「每次问答必有结束标识」，前端不至只落到占位语。〔终局标识兜底〕
func TestStreamAgentChunks_ChannelClosedWithoutDoneStillEmitsDone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/", nil) // ctx 未取消：客户端仍在

	chunkChan := make(chan *agentuc.ChatChunkDTO, 2)
	chunkChan <- &agentuc.ChatChunkDTO{Content: "已生成的部分内容"}
	close(chunkChan) // 关键：只发内容、不发 Done 就关闭

	h := &AgentHandler{}
	res := h.streamAgentChunks(c, sse.NewWriter(w), chunkChan, genUIOption{sessionID: "sess-x"})

	if res.Content != "已生成的部分内容" {
		t.Errorf("Content = %q, 期望 %q", res.Content, "已生成的部分内容")
	}
	body := w.Body.String()
	if !strings.Contains(body, "\"event\":\"done\"") && !strings.Contains(body, "event: done") {
		t.Errorf("通道无 Done 关闭后应补发终局 done，SSE 输出未见 done 事件：\n%s", body)
	}
	if !strings.Contains(body, "已生成的部分内容") {
		t.Errorf("终局 done 的 answer 应含已累积内容，实际：\n%s", body)
	}
	if !strings.Contains(body, "sess-x") {
		t.Errorf("终局 done 应回传 session_id，实际：\n%s", body)
	}
}
