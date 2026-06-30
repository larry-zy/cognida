# API Documentation - Unified Chat Service

## Overview

The Unified Chat Service provides a single entry point for all chat interactions through Agent-driven architecture. All conversations are handled by Agents, with Normal/RAG modes determined by Agent configuration.

## Chat Endpoint

### POST /api/v1/chat

Unified chat endpoint that routes requests to the appropriate Agent based on `agent_id`.

#### Request Format

```json
{
  "agent_id": "default",  // Optional: defaults to "default"
  "messages": [
    {
      "role": "user",
      "content": "Hello, how are you?"
    }
  ],
  "session_id": "sess-abc123",  // Optional: creates new session if omitted
  "stream": false,  // Optional: defaults to false
  "metadata": {}    // Optional: additional metadata
}
```

#### Response Format (Non-Streaming)

```json
{
  "success": true,
  "data": {
    "content": "Hello! I'm doing well, thank you for asking.",
    "tool_calls": [],
    "usage": {
      "prompt_tokens": 10,
      "completion_tokens": 15,
      "total_tokens": 25
    },
    "request_id": "req-xyz789",
    "session_id": "sess-abc123",
    "agent_id": "default"
  }
}
```

#### Response Format (Streaming)

When `stream: true`, the response uses Server-Sent Events (SSE):

```
event: start
data: {"session_id": "sess-abc123"}

event: content
data: {"content": "Hello!", "done": false}

event: content
data: {"content": " I'm doing well.", "done": false}

event: end
data: {"done": true}
```

### Agent Chat Endpoint

### POST /api/v1/agents/{agent_id}/chat

Chat with a specific Agent by ID.

#### Request Format

Same as `/api/v1/chat`, but `agent_id` is taken from the URL path.

```json
{
  "messages": [
    {
      "role": "user",
      "content": "Search for information about..."
    }
  ],
  "session_id": "sess-xyz789",  // Optional
  "stream": false  // Optional
}
```

## Built-in Agents

### default

**Description**: Default conversational agent without tools.

**Use Case**: General Q&A, casual conversation

**Configuration**:
- No tools
- Single-turn response
- Helpful and friendly prompt

### agent-rag-001

**Description**: RAG-enabled agent with knowledge base retrieval.

**Use Case**: Questions requiring domain knowledge from documents

**Configuration**:
- Tools: `rag_query`
- Max iterations: 5
- Knowledge base integration

### agent-chat-001

**Description**: Simple conversational agent.

**Use Case**: Basic chat interactions

**Configuration**:
- No tools
- Friendly prompt

## Message Types

### Role Values

- `system`: System message for setting agent behavior
- `user`: User message
- `assistant`: Agent response
- `tool`: Tool call result message

### Tool Calls

When an Agent uses tools, the response includes `tool_calls`:

```json
{
  "tool_calls": [
    {
      "id": "call_123",
      "type": "function",
      "function": {
        "name": "rag_query",
        "arguments": "{\"query\":\"example\"}"
      }
    }
  ]
}
```

## SSE Event Types

| Event | Description |
|-------|-------------|
| `start` | Stream started |
| `content` | Content fragment |
| `tool` | Tool invocation |
| `tool_start` | Tool call started |
| `tool_result` | Tool result received |
| `end` | Stream completed |
| `error` | Error occurred |

## Error Handling

### 400 Bad Request

Invalid request format or missing required fields.

### 404 Not Found

Agent or Session not found.

### 500 Internal Server Error

Server error during processing.

## Migration from Old API

### Old Format (Deprecated)

```json
{
  "mode": "rag",  // REMOVED
  "kb_id": 123,   // REMOVED
  "content": "Hello",
  "rag_config": {...}  // REMOVED
}
```

### New Format

```json
{
  "agent_id": "agent-rag-001",  // Use agent_id instead
  "messages": [{"role": "user", "content": "Hello"}]
  // RAG config is in Agent definition, not request
}
```
