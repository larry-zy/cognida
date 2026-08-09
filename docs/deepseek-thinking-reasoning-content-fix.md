# DeepSeek Thinking Mode `reasoning_content` 回传缺陷分析与修复方案

> 状态：✅ 已实现并通过 E2E 回归（2026-07-05）
> 影响：Agent 多轮工具调用 100% 失败 —— 系统两个 Agent 均无法完成完整任务
> 关联代码：`cognida-go/internal/infrastructure/llm/chat/openai.go`、`cognida-go/internal/service/agent/framework/eino_agent.go`

---

## 1. 现象（Symptom）

Agent 在一次会话中：

1. `kb_list` ✅
2. `rag_query` ✅（可多次）
3. 准备基于工具结果生成回复 → **HTTP 400，任务中断**

日志中的原始错误：

```
生成失败: api error (status 400): {"error":{"message":"The `reasoning_content` in the thinking mode must be passed back to the API.","type":"invalid_request_error","param":null,"code":"invalid_request_error"}}
```

特征：**第一轮总是成功；只要进入"带工具调用后的下一轮生成"就必然失败**。纯聊天（无工具）不受影响。系统里两个 Agent 共用同一套 `chat` 客户端，因此表现为"两个 Agent 都完不成任务"——这是底层 LLM 客户端的结构性缺陷，不是某个 Agent 的业务 bug。

---

## 2. 根因（Root Cause）

用的是 **DeepSeek V4 系列**（`deepseek-v4-pro` / `deepseek-v4-flash`），thinking 模式**默认开启**。V4 的协议约定：

> 两条 user 消息之间，如果模型**执行过工具调用**，那么中间 assistant 消息的 `reasoning_content` **必须**参与上下文拼接，并在后续所有请求中原样回传；否则返回 400。

而本项目的 `chat` 客户端在四个层面都把 `reasoning_content` 丢掉了：

### 2.1 响应解析丢弃 —— `openai.go`

`Generate`（非流式，`openai.go:95-99`）与 `Stream`（流式，`openai.go:154-158 / 240-244`）返回的 `schema.Message` 只带 `Role/Content/ToolCalls`，从不读取响应里的 `reasoning_content`：

```go
// openai.go:95-99  Generate
return &schema.Message{
    Role:      schema.RoleType(choice.Message.Role),
    Content:   choice.Message.Content,
    ToolCalls: toolCalls,        // ← reasoning_content 未提取
}, nil
```

### 2.2 消息类型无字段 —— `openai.go:389-394`

```go
type openaiMessage struct {
    Role       string           `json:"role"`
    Content    string           `json:"content"`
    ToolCalls  []openaiToolCall `json:"tool_calls,omitempty"`
    ToolCallID string           `json:"tool_call_id,omitempty"`
    // ← 缺少 ReasoningContent
}
```

`openaiChoice.Message` / `openaiDelta` 同样没有 `reasoning_content` 字段，反序列化时直接被忽略。

### 2.3 请求构建不回传 —— `openai.go:264-294 buildRequest`

拼装发往 API 的历史消息时，只拷贝 `Role/Content/ToolCalls/ToolCallID`，没有把 assistant 轮的 `reasoning_content` 写回请求体。

### 2.4 Agent 循环重建消息时丢失（且有第二个隐患）—— `eino_agent.go:617-624`

```go
// 处理工具调用
for _, tc := range resp.ToolCalls {
    ...
    // 添加工具响应消息
    messages = append(messages, &schema.Message{
        Role:      schema.Assistant,
        Content:   "",
        ToolCalls: []schema.ToolCall{tc},   // ← 用单个 tc 重建，丢了 resp.ReasoningContent
    })
    messages = append(messages, schema.ToolMessage(compactObservation(toolCall.Output), tc.ID))
}
```

这里有**两个**问题：

- **主因**：丢弃了 `resp.ReasoningContent`（原始 `resp` 是携带 reasoning 的那条消息，却没被复用）。
- **隐患**：把一轮里的多个 `ToolCall` 拆成多条各含单个 tool_call 的 assistant 消息，与后续 tool 消息交错。并行工具调用场景下这是**畸形消息序列**，DeepSeek 校验会更严格地失败。正确做法是 assistant 轮只 append 一次（含全部 tool_calls + reasoning_content），后面紧跟每个 tool_call 对应的 tool 消息。

> ✅ 好消息：eino 的 `schema.Message` 自 v0.7.x 起**已内置 `ReasoningContent string` 字段**（`schema/message.go:647`，本项目用 v0.7.32）。所以有现成载体，无需自建旁路结构；`internal/` 现在对该字段零引用（grep 无命中），补齐即可。

---

## 3. DeepSeek Thinking Mode 官方规则（务必按此实现）

来源：DeepSeek 官方 [Thinking Mode 指南](https://api-docs.deepseek.com/guides/thinking_mode)。

### 3.1 开启方式（OpenAI 兼容端点）

- 开关：`extra_body` 里 `"thinking": {"type": "enabled" | "disabled"}`
- 力度：顶层 `"reasoning_effort": "high" | "max"`
- 默认：thinking = enabled，effort = high（复杂 agent 请求自动升到 max）

### 3.2 响应结构

`reasoning_content` 与 `content` **同级**返回：

```json
{ "choices": [ { "message": {
    "role": "assistant",
    "content": "最终答案",
    "reasoning_content": "思维链……",
    "tool_calls": [ ... ]
} } ] }
```

### 3.3 多轮回传规则（核心）

| 场景 | reasoning_content 是否回传 |
|------|---------------------------|
| 两条 user 之间**无**工具调用 | 不需要回传；即使传了也会被忽略 |
| 两条 user 之间**有**工具调用 | **必须**在后续所有请求中原样回传，否则 400 |

### 3.4 不支持的参数

thinking 模式下 `temperature`、`top_p`、`presence_penalty`、`frequency_penalty` **无效**（不报错但不生效）。当前 `buildRequest` 仍会带 `temperature/top_p`，无害但应知悉。

### 3.5 版本差异（关键陷阱）

- **V4 系列（deepseek-v4-pro/flash）**：有工具调用时**必须回传** reasoning_content（本项目命中此规则）。
- **旧版 `deepseek-chat` / `deepseek-reasoner`**：规则**相反** —— 如果把 reasoning_content 塞进输入消息，反而报 400，必须剔除。

因此回传逻辑**不能无脑全开**，需按模型/provider 区分（见 §4.5）。

---

## 4. 修复方案（Fix Plan）

四层贯通 + Agent 循环修正 + 差异化开关。

### 4.1 响应类型补字段 —— `openai.go`

给 `openaiMessage` 与 `openaiDelta` 增加：

```go
ReasoningContent string `json:"reasoning_content,omitempty"`
```

### 4.2 响应解析写入 `ReasoningContent` —— `openai.go`

- `Generate`：返回的 `schema.Message` 补 `ReasoningContent: choice.Message.ReasoningContent`。
- `Stream`：delta 里累积 `reasoning_content`；流式下 reasoning 与 content 分片到达，需与 content 一样按 delta 拼接，并在最终（含 tool_calls）的 assistant 消息上带出。

### 4.3 请求构建回传 —— `openai.go buildRequest`

assistant 消息序列化时，若 `msg.ReasoningContent != ""` 则写入 `oaiMsg.ReasoningContent`。

### 4.4 Agent 循环正确复用 assistant 消息 —— `eino_agent.go`

把 `chatWithTools`（`:617-624`）与 `streamWithTools`（`:617-624` 附近）、`chatWithMemoryAndTools`（`:353-360`）中"逐 tc 重建 assistant 消息"的写法，改为：

```go
// assistant 轮整体只 append 一次（保留 reasoning + 全部 tool_calls）
messages = append(messages, resp)
// 再为每个 tool_call 追加对应的 tool 结果消息
for _, tc := range resp.ToolCalls {
    ...
    messages = append(messages, schema.ToolMessage(compactObservation(output), tc.ID))
}
```

> `resp` 本身即 `Generate/Stream` 返回、已带 `ReasoningContent` 与全部 `ToolCalls` 的那条消息，直接复用即天然满足回传要求，并顺带修掉并行工具调用的消息畸形问题。

### 4.5 差异化开关（避免打挂旧模型）

在 `ChatConfig` / provider 层增加判定：

- V4 系列：透传 reasoning_content（如上）。
- `deepseek-reasoner` / `deepseek-chat` 旧模型：`buildRequest` 时**剔除** reasoning_content。
- 通过 `thinking` 开关（`ChatOptions.Thinking` 已存在，`types.go:48`）驱动 `extra_body.thinking` 与 `reasoning_effort` 的下发，thinking 关闭时不涉及本问题。

判定建议基于 `ModelName` 前缀（`deepseek-v4*` vs `deepseek-reasoner`/`deepseek-chat`）或显式配置项，集中在一个 helper 里，避免散落。

---

## 5. 边界与坑（Caveats）

1. **不能用空串糊弄**：社区有实现给缺失 reasoning 的 assistant 轮填 `reasoning_content=""`，在长链/子 agent 场景下仍被 400 拒绝。必须回传**真实**的上一轮 reasoning 内容 —— §4.4 复用 `resp` 天然满足，切勿改成填空串。
2. **只在"有工具调用"的中间轮回传**；纯对话轮不必带。
3. **thinking 模式忽略采样参数**：`temperature/top_p` 无效，别指望靠它们调输出。
4. **流式解析**：reasoning_content 在 SSE delta 里独立分片到达，需与 content 分开累积，`[DONE]`/`finish_reason` 时组装进最终 assistant 消息。
5. **旧模型反向规则**：见 §3.5，务必按模型区分，否则会把 `deepseek-reasoner` 打挂。
6. **备选方案**：若 OpenAI 格式处理 reasoning 状态过于繁琐，DeepSeek 亦提供 Anthropic 格式端点（`https://api.deepseek.com/anthropic`），原生支持 thinking，可作为后备。本项目已有 OpenAI 兼容栈，优先按 §4 在现有栈修复。

---

## 6. 验证方案（E2E）

修复后需覆盖：

- **单元**：`buildRequest` 对 assistant(带 reasoning+tool_calls) 消息的序列化；`Generate/Stream` 对 `reasoning_content` 的解析；旧模型剔除逻辑。
- **集成/E2E**：对系统**两个 Agent** 各跑一段**多轮 + 工具调用**对话，断言不再出现 `reasoning_content ... must be passed back` 的 400，且能给出最终答复。E2E 脚本单独提供。

---

## 7. 待办清单（Checklist）

- [x] `openaiMessage` / `openaiDelta` 增加 `ReasoningContent` 字段
- [x] `Generate` 解析 reasoning_content → `schema.Message.ReasoningContent`
- [x] `Stream` 累积并组装 reasoning_content
- [x] `buildRequest` 回传 assistant 轮 reasoning_content（含旧模型剔除分支 `needsReasoningRoundTrip`）
- [x] `eino_agent.go` 三处循环改为复用 `resp` 整条 assistant 消息（`chatWithTools` / `chatWithMemoryAndTools` / `streamWithTools`）
- [x] `tool_model.go` `WithTools` 透传 `roundTripReasoning`（绑定工具后的实例保留开关）
- [ ] thinking 开关驱动 `extra_body.thinking` + `reasoning_effort` 下发（V4 默认开启，暂未显式下发，后续增强）
- [x] 单元测试：`openai_reasoning_test.go`（解析 / V4 回传 / 旧版剔除 / 判定表，4 用例全绿）
- [x] 两个 Agent 的多轮 E2E：`cognida-go/scripts/test-agents-e2e.sh`（RAG + Data 各 2 轮，含并行/多轮工具调用，无 400）
- [ ] 顺带排查 RAG 检索返回 0（144 chunks 却 count=0，独立问题——E2E 中 RAG 首次检索仍偶发空结果，见验证记录）

### 验证记录（E2E, 2026-07-05, deepseek-v4-flash）

| Agent | 端点 | 轮次 | 工具调用 | reasoning_content 400 | 结论 |
|-------|------|------|----------|----------------------|------|
| RAG   | `/api/v1/agent/knowledge/stream` | 2 | 5 次（含单轮 4 次 ReAct 迭代） | 无 | ✅ |
| Data  | `/api/v1/agent/text2sql/stream`  | 2 | 1 次 | 无 | ✅ |

关键点：修复前"带工具调用后的下一轮生成"必 400；修复后 RAG 单轮内连续 4 次工具迭代的
reasoning_content 均正确回传，多轮对话贯通。

---

## 参考

- DeepSeek 官方：[Thinking Mode 指南](https://api-docs.deepseek.com/guides/thinking_mode)
- DeepSeek 官方：[Reasoning Model (deepseek-reasoner)](https://api-docs.deepseek.com/guides/reasoning_model)
- 同类问题（多框架复现同一根因）：OpenCode [#24190](https://github.com/anomalyco/opencode/issues/24190)、KiloCode [#9471](https://github.com/Kilo-Org/kilocode/issues/9471)
