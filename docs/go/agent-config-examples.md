# Agent Configuration Examples

This document provides examples of different Agent configurations for various use cases.

## Normal Agent (Default)

**Description**: A simple conversational agent without tools. Suitable for general Q&A and casual conversation.

```yaml
id: "default"
name: "默认助手"
description: "普通对话 Agent，无工具调用，适用于日常问答"
type: "react"
status: "idle"
config: |
  {
    "name": "默认助手",
    "prompt": "你是一个有帮助的 AI 助手。\n\n你的任务：\n- 回答用户的各种问题\n- 提供帮助和建议\n- 保持对话友好和连贯",
    "tools": [],
    "max_iterations": 1
  }
metadata:
  builtin: "true"
  version: "1.0.0"
```

**Behavior**: Direct LLM calls, no tool invocation, single-turn response.

## RAG Agent

**Description**: An agent with knowledge base retrieval capabilities. Suitable for domain-specific questions requiring document context.

```yaml
id: "rag-agent"
name: "知识库助手"
description: "RAG 检索助手，可以查询知识库并回答问题"
type: "react"
status: "idle"
config: |
  {
    "name": "知识库助手",
    "prompt": "你是一个智能助手，具有知识库检索能力。\n\n当用户提问时：\n1. 使用 rag_retrieve 工具从知识库中检索相关信息\n2. 基于检索结果给出准确答案\n3. 如果知识库没有相关信息，诚实告知用户\n\n使用 rag_retrieve 的时机：\n- 用户询问文档内容或专业知识时\n- 需要提供准确来源的信息时",
    "tools": ["rag_retrieve", "rag_search"],
    "rag": {
      "enabled": true,
      "kb_id": "kb-001",
      "retrieval_modes": ["vector", "bm25"],
      "vector_top_k": 10,
      "similarity_threshold": 0.7
    },
    "max_iterations": 5
  }
metadata:
  version: "1.0.0"
  tools: "rag_retrieve,rag_search"
```

**Behavior**:
1. Retrieves relevant documents from knowledge base
2. Injects retrieved context into LLM prompt
3. Generates response based on retrieved information

## Agentic Agent (Data Analysis)

**Description**: An agent capable of executing data queries and analysis. Suitable for business intelligence tasks.

```yaml
id: "data-agent"
name: "数据分析助手"
description: "执行数据查询和分析的 Agent"
type: "react"
status: "idle"
config: |
  {
    "name": "数据分析助手",
    "prompt": "你是一个数据分析助手。\n\n你的能力：\n- 执行 SQL 查询获取数据\n- 进行数据计算和统计\n- 生成图表可视化\n\n使用工具的时机：\n- 用户询问具体数据时 → 使用 sql_query\n- 需要计算时 → 使用 calculator\n- 需要可视化时 → 使用 chart_generate",
    "tools": ["sql_query", "calculator", "chart_generate"],
    "max_iterations": 10
  }
metadata:
  version: "1.0.0"
  tools: "sql_query,calculator,chart_generate"
```

**Behavior**:
1. Decomposes complex queries into tool calls
2. Executes SQL queries against database
3. Performs calculations and generates visualizations
4. Can iterate multiple times to complete multi-step tasks

## Web Search Agent

**Description**: An agent with web search capabilities. Suitable for questions requiring current information.

```yaml
id: "web-agent"
name: "网络搜索助手"
description: "可以搜索网络信息的 Agent"
type: "react"
status: "idle"
config: |
  {
    "name": "网络搜索助手",
    "prompt": "你是一个具有网络搜索能力的助手。\n\n当用户询问时事新闻、最新信息或你不确定的内容时：\n1. 使用 web_search 工具搜索相关信息\n2. 综合搜索结果给出准确回答\n3. 注明信息来源",
    "tools": ["web_search"],
    "max_iterations": 3
  }
metadata:
  version: "1.0.0"
  tools: "web_search"
```

## Graph Query Agent

**Description**: An agent for knowledge graph queries. Suitable for relationship-based questions.

```yaml
id: "graph-agent"
name: "图谱查询助手"
description: "可以查询知识图谱的 Agent"
type: "react"
status: "idle"
config: |
  {
    "name": "图谱查询助手",
    "prompt": "你是一个知识图谱查询助手。\n\n你的能力：\n- 查询实体之间的关系\n- 发现隐藏的关联\n- 进行路径分析\n\n使用 graph_query 工具时，请提供：\n- 起始实体\n- 目标实体（可选）\n- 关系类型（可选）",
    "tools": ["graph_query"],
    "max_iterations": 5
  }
metadata:
  version: "1.0.0"
  tools: "graph_query"
```

## Configuration Field Reference

### Core Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | Yes | Unique agent identifier |
| `name` | string | Yes | Display name |
| `description` | string | No | Agent description |
| `type` | string | Yes | Agent type (currently "react") |
| `status` | string | Yes | Agent status ("idle", "busy") |
| `config` | string | Yes | JSON-encoded configuration |
| `metadata` | map | No | Additional metadata |

### Config Fields

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Agent display name |
| `prompt` | string | System prompt for the agent |
| `tools` | []string | List of tool names |
| `rag` | object | RAG configuration (optional) |
| `max_iterations` | int | Maximum tool call iterations |

### RAG Config Fields

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | boolean | Enable RAG |
| `kb_id` | string | Knowledge base ID |
| `retrieval_modes` | []string | Retrieval modes (vector, bm25, graph) |
| `vector_top_k` | int | Vector retrieval count |
| `similarity_threshold` | float | Similarity threshold (0-1) |

## Tool Reference

### Available Tools

| Tool Name | Description |
|-----------|-------------|
| `rag_retrieve` | Retrieve documents from knowledge base |
| `rag_search` | Advanced search in knowledge base |
| `sql_query` | Execute SQL queries |
| `calculator` | Perform calculations |
| `chart_generate` | Generate chart visualizations |
| `web_search` | Search the web |
| `graph_query` | Query knowledge graph |

### Creating Custom Tools

To add a custom tool:

1. Implement the tool interface in `application/usecases/agent/tools/`
2. Register the tool in the Tool Registry
3. Reference the tool name in Agent config

Example:
```go
// application/usecases/agent/tools/my_tool.go
package tools

type MyTool struct {
    name        string
    description string
}

func (t *MyTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
    return &schema.ToolInfo{
        Name:        "my_tool",
        Description: t.description,
        Params:      map[string]any{...},
    }, nil
}

func (t *MyTool) InvokableRun(ctx context.Context, args string) (string, error) {
    // Tool implementation
}
```

## Agent Lifecycle

### Registration

Agents are registered at application startup:

```go
// application/agent/init.go
func (init *Initializer) Initialize(ctx context.Context) error {
    // Register built-in agents
    init.registerDefaultAgent(ctx)
    init.registerRAGAgent(ctx, toolModel)
    init.registerChatAgent(ctx)

    // Load custom agents from database
    customAgents, _ := agentRepo.LoadAll(ctx)
    for _, agent := range customAgents {
        registry.Register(ctx, agent)
    }
}
```

### Storage

- **Built-in Agents**: Hardcoded in `application/agent/init.go`, stored in memory (Redis)
- **Custom Agents**: Stored in database, loaded into memory at startup

### Runtime Behavior

1. Client sends chat request with `agent_id`
2. ChatService gets Agent from registry
3. AgentFactory creates Agent instance
4. Agent executes chat with configured tools/prompt
5. Response returned to client

## Best Practices

### 1. Prompt Design

- Clearly define the agent's role and capabilities
- Specify when to use each available tool
- Include examples of desired behavior
- Set appropriate iteration limits

### 2. Tool Selection

- Only include tools the agent actually needs
- Too many tools can confuse the agent
- Use tool descriptions to guide selection

### 3. Iteration Limits

- Simple tasks: 1-3 iterations
- Complex tasks: 5-10 iterations
- Multi-step reasoning: 10+ iterations
- Balance between capability and latency

### 4. RAG Configuration

- Start with high similarity threshold (0.7-0.8)
- Adjust `vector_top_k` based on document length
- Use hybrid retrieval (vector + bm25) for better results
