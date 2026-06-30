# Chat Service Migration Guide

This guide helps you migrate from the old chat implementation to the new unified ChatService.

## Overview of Changes

### Architecture Changes

**Before:**
```
ChatService
  ├─ Mode=Normal → Direct LLM
  ├─ Mode=RAG → RAG retrieval + LLM
  └─ Mode=Agent → Eino Agent
```

**After:**
```
ChatService (unified entry point)
    │
    ├─ agent_id="default" → Default Agent (Normal mode)
    ├─ agent_id="agent-rag-001" → RAG Agent
    └─ agent_id="custom" → Custom Agent
```

### Key Changes

1. **Request Format**: New `messages` array format instead of `content`
2. **Agent Selection**: Use `agent_id` instead of `mode`
3. **RAG Configuration**: Moved to Agent definition, not request parameter
4. **Response Format**: Unified response structure

## Step-by-Step Migration

### 1. Update Request Format

**Old Code:**
```go
// Old request
req := chatuc.ChatRequest{
    Content:   "Hello",
    Mode:      "rag",
    KBID:      123,
    SessionID: "sess-001",
}
```

**New Code:**
```go
// New request
req := ChatRequest{
    AgentID:   "agent-rag-001",
    Messages: []*llm.Message{
        {Role: "user", Content: "Hello"},
    },
    SessionID: "sess-001",
}
```

### 2. Update Response Handling

**Old Code:**
```go
resp, err := chatService.Chat(ctx, req)
fmt.Println(resp.Answer)
```

**New Code:**
```go
resp, err := chatService.Chat(ctx, req)
fmt.Println(resp.Content)
```

### 3. Update Streaming

**Old Code:**
```go
stream, err := chatService.ChatStream(ctx, req)
for chunk := range stream {
    fmt.Println(chunk.Content)
}
```

**New Code:**
```go
stream, err := chatService.ChatStream(ctx, req)
for chunk := range stream {
    switch chunk.Event {
    case "content":
        fmt.Println(chunk.Data["content"])
    case "tool":
        fmt.Println("Tool called:", chunk.Data["name"])
    }
}
```

### 4. Configure RAG Agent

**Old:** RAG config passed in request

**New:** RAG config in Agent definition

1. Create RAG Agent configuration in Agent Registry
2. Use `agent_id` to reference the Agent
3. No need to pass `kb_id` or `rag_config` in request

### 5. Update Handler Dependencies

**Old:**
```go
chatHandler := handler.NewChatHandler(
    ragService,
    llmService,
)
```

**New:**
```go
chatHandler := handler.NewChatHandler(
    chatService,  // Unified ChatService
)
```

## Code Examples

### Example 1: Simple Chat

```go
// Request
req := &ChatRequest{
    Messages: []*llm.Message{
        {Role: "user", Content: "What is the weather?"},
    },
}

// Uses default agent automatically
resp, err := chatService.Chat(ctx, req)
```

### Example 2: RAG Chat

```go
// Request
req := &ChatRequest{
    AgentID: "agent-rag-001",
    Messages: []*llm.Message{
        {Role: "user", Content: "Find documents about..."},
    },
}

// Agent handles RAG retrieval internally
resp, err := chatService.Chat(ctx, req)
```

### Example 3: Streaming

```go
req := &ChatRequest{
    AgentID: "default",
    Messages: []*llm.Message{
        {Role: "user", Content: "Tell me a story"},
    },
    Stream: true,
}

stream, err := chatService.ChatStream(ctx, req)
for chunk := range stream {
    // Handle SSE events
}
```

## Breaking Changes

### Removed Fields

| Old Field | Replacement |
|-----------|-------------|
| `mode` | Use `agent_id` instead |
| `kb_id` | Configure in Agent definition |
| `rag_config` | Configure in Agent definition |
| `content` | Use `messages` array |

### New Required Fields

| Field | Description |
|-------|-------------|
| `messages` | Array of messages (required) |

## Testing Your Migration

1. **Unit Tests**: Update test fixtures to use new request format
2. **Integration Tests**: Verify Agent routing works correctly
3. **API Tests**: Test with and without `agent_id`
4. **Streaming Tests**: Verify SSE event format

## Rollback Plan

If issues arise, you can temporarily use the deprecated endpoints:

- `application/usecases/chat/chat_usecase.go` (marked @Deprecated)
- `application/usecases/rag/chat.go` (marked @Deprecated)

These will be removed in a future version.

## Getting Help

- API Documentation: See `unified-chat-api.md`
- Agent Configuration: See Agent examples below
- Issues: Report to the development team

## Agent Configuration Examples

### Normal Agent (Default)

```yaml
id: "default"
name: "默认助手"
type: "react"
config:
  name: "默认助手"
  prompt: "你是一个有帮助的助手。"
  tools: []
  max_iterations: 1
```

### RAG Agent

```yaml
id: "rag-agent"
name: "知识库助手"
type: "react"
config:
  name: "知识库助手"
  prompt: "基于知识库回答问题"
  tools: ["rag_retrieve", "rag_search"]
  rag:
    enabled: true
    kb_id: "kb-001"
    retrieval_modes: ["vector", "bm25"]
    vector_top_k: 10
    similarity_threshold: 0.7
```

### Agentic Agent

```yaml
id: "data-agent"
name: "数据分析助手"
type: "react"
config:
  name: "数据分析助手"
  prompt: "执行数据查询和分析"
  tools: ["sql_query", "calculator", "chart_generate"]
  max_iterations: 10
```
