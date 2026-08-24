package framework

// 端到端回归（issue #4 / #8）：不依赖任何外部服务，全进程内自洽。
//
//   - #8 走真实 wire 链路：httptest 起 OpenAI 兼容 mock 服务 → 生产 chat 客户端
//     （NewToolCallingChatModel，含 DSML 归一化装饰）→ 生产 Builder/Agent → 生产
//     RegistryAgentOrchestrator（Chat 与 SSE Stream 双路径）。mock 持续返回参数为非法
//     JSON 的工具调用，验证自我修复护栏在 wire 级真实生效（4 次即提前收尾，而非空转到 maxIter）。
//   - #4 走 orchestrator 级：模型桩在工具轮后返回 (nil, nil)（OpenAI/Ollama 客户端会把
//     异常转为 error，该分支防御的是其它/未来 provider），验证经生产编排器后用户仍拿到
//     非空 wind-down 结论而非空答。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/eino/schema"

	"cognida/internal/infrastructure/llm/chat"
)

// ========================================
// mock OpenAI 兼容服务
// ========================================

// wireMessage/wireRequest 是 OpenAI wire 协议请求侧的最小解码集（仅测试断言所需字段）。
type wireMessage struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id"`
	ToolCalls  []struct {
		ID       string `json:"id"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls"`
}

type wireRequest struct {
	Model    string        `json:"model"`
	Messages []wireMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// mockOpenAI 是可脚本的 OpenAI 兼容服务：非 wind-down 请求一律返回「参数为非法 JSON 的
// 工具调用」；末条消息为 wind-down 指令（System）时返回正常收尾内容。记录全部请求供断言。
type mockOpenAI struct {
	mu       sync.Mutex
	requests []wireRequest
	t        *testing.T
}

const (
	malformedTool     = "query_data"
	malformedArgs     = `{"sql": "SELECT`
	windDownContent   = "已基于现有观察给出部分结论。"
	streamFinalText   = "流式收尾结论"
	windDownMarker    = "请勿再调用任何工具"
	nonStreamFinalJSD = `{"id":"m","object":"chat.completion","created":1,"model":"mock",` +
		`"choices":[{"index":0,"message":{"role":"assistant","content":"` + windDownContent + `"},` +
		`"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120}}`
)

func toolCallRespJSON(callID int) string {
	return fmt.Sprintf(`{"id":"m","object":"chat.completion","created":1,"model":"mock",`+
		`"choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[`+
		`{"id":"call_%d","type":"function","function":{"name":"%s","arguments":%s}}]},`+
		`"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":50,"completion_tokens":5,"total_tokens":55}}`,
		callID, malformedTool, mustJSONString(malformedArgs))
}

// mustJSONString 把任意字符串编码为 JSON 字符串字面量（保留非法 JSON 原样转义）。
func mustJSONString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (m *mockOpenAI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req wireRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	m.mu.Lock()
	m.requests = append(m.requests, req)
	n := len(m.requests)
	m.mu.Unlock()

	isWindDown := false
	if len(req.Messages) > 0 {
		last := req.Messages[len(req.Messages)-1]
		isWindDown = last.Role == "system" && strings.Contains(last.Content, windDownMarker)
	}

	if req.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		var chunks []string
		if isWindDown {
			chunks = []string{
				sseChunk(`{"role":"assistant","content":`+mustJSONString(streamFinalText)+`}`, ""),
				sseChunk(`{}`, "stop"),
			}
		} else {
			chunks = []string{
				sseChunk(`{"role":"assistant","tool_calls":[{"index":0,"id":"call_s`+fmt.Sprint(n)+
					`","type":"function","function":{"name":"`+malformedTool+`","arguments":`+mustJSONString(malformedArgs)+`}}]}`, ""),
				sseChunk(`{}`, "tool_calls"),
			}
		}
		for _, c := range chunks {
			fmt.Fprint(w, "data: "+c+"\n\n")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if isWindDown {
		fmt.Fprint(w, nonStreamFinalJSD)
		return
	}
	fmt.Fprint(w, toolCallRespJSON(n))
}

func sseChunk(delta, finishReason string) string {
	fr := "null"
	if finishReason != "" {
		fr = mustJSONString(finishReason)
	}
	return `{"id":"m","object":"chat.completion.chunk","created":1,"model":"mock",` +
		`"choices":[{"index":0,"delta":` + delta + `,"finish_reason":` + fr + `}]}`
}

func (m *mockOpenAI) requestCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.requests)
}

func (m *mockOpenAI) requestAt(i int) wireRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.requests[i]
}

// newWireTestAgent 构造「生产 chat 客户端 + 生产 Builder」的真实 Agent。
func newWireTestAgent(t *testing.T, baseURL string, toolInvocations *[]string) Agent {
	t.Helper()
	tm, err := chat.NewToolCallingChatModel(context.Background(), &chat.ChatConfig{
		Source:    "remote",
		APIKey:    "test-key",
		BaseURL:   baseURL,
		ModelName: "mock-model",
		Provider:  "openai",
	})
	if err != nil {
		t.Fatalf("创建生产 chat 客户端失败: %v", err)
	}
	qtool := &recordingTool{name: malformedTool, calls: toolInvocations}
	agent, err := New(tm).
		WithToolModel(tm).
		Name("e2e-wire").
		Prompt("你是测试助手").
		Tools(qtool).
		WithMaxIterations(20).
		Build(context.Background())
	if err != nil {
		t.Fatalf("构建 Agent 失败: %v", err)
	}
	return agent
}

// ========================================
// #8：wire 级端到端（Chat 路径）
// ========================================

// TestE2E_MalformedArgs_WireLevelChat 验证：真实 wire 链路（HTTP→生产 chat 客户端→DSML 归一化
// →Builder/Agent→execLoop）下，模型持续返回参数为非法 JSON 的工具调用时——
//   - 工具本体从不被执行（畸形参数不触达工具）；
//   - 自我修复护栏 4 次失败即提前收尾：恰好 windDownThreshold+1 次 HTTP 请求（修复前会空转到 maxIter）；
//   - wind-down 请求的历史里 assistant.tool_calls 与 tool 消息严格 1:1（4:4），观察均为解析失败合成错误，
//     且已注入带签名的再规划提示；
//   - 最终响应 terminated_by=repair_exhausted、partial=true、内容非空。
func TestE2E_MalformedArgs_WireLevelChat(t *testing.T) {
	mock := &mockOpenAI{t: t}
	server := httptest.NewServer(mock)
	defer server.Close()

	var invoked []string
	agent := newWireTestAgent(t, server.URL, &invoked)

	resp, err := agent.Chat(context.Background(), "统计用户数")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	if len(invoked) != 0 {
		t.Errorf("畸形参数绝不应触达工具执行, got %v", invoked)
	}
	if resp.Metadata["terminated_by"] != TerminatedByRepairExhausted {
		t.Errorf("expected terminated_by=repair_exhausted, got %v", resp.Metadata["terminated_by"])
	}
	if resp.Metadata["partial"] != true {
		t.Error("护栏提前收尾应标记 partial")
	}
	if !strings.Contains(resp.Content, windDownContent[:6]) {
		t.Errorf("应交付 wind-down 结论, got %q", resp.Content)
	}

	// 请求次数 = 4 轮畸形工具调用 + 1 次 wind-down；修复前护栏失明，会一路空转到 maxIter=20。
	if got := mock.requestCount(); got != windDownThreshold+1 {
		t.Fatalf("HTTP 请求数应为 %d（护栏 %d 次即收尾+wind-down）, got %d", windDownThreshold+1, windDownThreshold, got)
	}

	// 末次（wind-down）请求：1:1 配对 + 再规划提示注入 + 观察为合成解析错误。
	last := mock.requestAt(mock.requestCount() - 1)
	assistantWithCalls, toolObs, replanNote := 0, 0, false
	for _, msg := range last.Messages {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			assistantWithCalls++
		}
		if msg.Role == "tool" {
			toolObs++
			if !strings.Contains(msg.Content, "参数解析失败") {
				t.Errorf("tool 观察应为合成解析失败错误, got %q", msg.Content)
			}
		}
		if msg.Role == "system" && strings.Contains(msg.Content, "重复失败："+malformedTool) {
			replanNote = true
		}
	}
	if assistantWithCalls != windDownThreshold || toolObs != windDownThreshold {
		t.Errorf("wire 历史 tool_call/tool 消息应 %d:%d, got %d:%d",
			windDownThreshold, windDownThreshold, assistantWithCalls, toolObs)
	}
	if !replanNote {
		t.Error("wire 历史应含带签名的再规划提示（畸形参数计入护栏的旁证）")
	}
	// 每次记录的 ToolCall 均为解析失败。
	for _, tc := range resp.ToolCalls {
		if tc.Error == nil || !strings.Contains(tc.Error.Error(), "invalid arguments") {
			t.Errorf("每次工具调用应记录解析失败: %+v", tc)
		}
	}
}

// ========================================
// #8：wire 级端到端（SSE Stream + 生产编排器）
// ========================================

// TestE2E_MalformedArgs_WireLevelStream 验证流式链路（streamSink→RegistryAgentOrchestrator）：
// 畸形参数同样计入护栏（4 次即收尾、恰 5 次 HTTP 请求），最终 content 事件为 wind-down 结论。
func TestE2E_MalformedArgs_WireLevelStream(t *testing.T) {
	mock := &mockOpenAI{t: t}
	server := httptest.NewServer(mock)
	defer server.Close()

	var invoked []string
	agent := newWireTestAgent(t, server.URL, &invoked)

	orch := NewRegistryAgentOrchestrator(func(id string) (Agent, bool) { return agent, id == "e2e-wire" })

	events, err := orch.ExecuteStream(context.Background(), "e2e-wire", "统计用户数")
	if err != nil {
		t.Fatalf("ExecuteStream: %v", err)
	}

	var content strings.Builder
	toolCallEvents := 0
	for ev := range events {
		switch ev.Type {
		case "tool_call":
			toolCallEvents++
		case "content":
			content.WriteString(ev.Content)
		case "error":
			t.Fatalf("不应发生 error 事件: %s", ev.Error)
		}
	}

	if len(invoked) != 0 {
		t.Errorf("畸形参数绝不应触达工具执行, got %v", invoked)
	}
	// 注：parseErr 分支只发 tool_call 事件（不发 tool_result），且编排器把状态硬编码为
	// calling——属既有流式事件语义缺口（与 #3/#7 同族），不在此断言、留待对应 issue 收敛。
	if toolCallEvents != windDownThreshold {
		t.Errorf("流式 tool_call 事件应恰 %d（护栏收尾的旁证）, got %d", windDownThreshold, toolCallEvents)
	}
	if !strings.Contains(content.String(), streamFinalText) {
		t.Errorf("流式最终内容应含 wind-down 结论, got %q", content.String())
	}
	if got := mock.requestCount(); got != windDownThreshold+1 {
		t.Fatalf("HTTP 请求数应为 %d, got %d", windDownThreshold+1, got)
	}
}

// ========================================
// #4：orchestrator 级端到端
// ========================================

// TestE2E_NilResponseAfterTools_Orchestrator 验证：经生产编排器（RegistryAgentOrchestrator.Execute）
// 执行时，模型桩在工具轮后返回 (nil, nil)（异常空响应）——用户仍拿到非空 wind-down 结论，
// 而非修复前的空字符串。
func TestE2E_NilResponseAfterTools_Orchestrator(t *testing.T) {
	var invoked []string
	qtool := &recordingTool{name: "query", calls: &invoked}

	tm := &scriptedToolModel{script: []*schema.Message{
		toolCallMsg("1", "query"),
		nil, // 工具轮后异常空响应
	}}
	agent, err := New(tm).
		WithToolModel(tm).
		Name("e2e-nil").
		Prompt("你是测试助手").
		Tools(qtool).
		WithMaxIterations(10).
		Build(context.Background())
	if err != nil {
		t.Fatalf("构建 Agent 失败: %v", err)
	}

	orch := NewRegistryAgentOrchestrator(func(id string) (Agent, bool) { return agent, id == "e2e-nil" })

	result, err := orch.Execute(context.Background(), "e2e-nil", "查一下")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "final") {
		t.Fatalf("nil 响应（已跑过工具轮）经编排器应交付 wind-down 结论, got %q", result)
	}
	if len(invoked) != 1 {
		t.Errorf("query 工具应恰执行一次, got %v", invoked)
	}
}
