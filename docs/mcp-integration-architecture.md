# Link MCP 集成架构文档

## 文档说明

本文档描述 Link 系统中 Go Agent 通过 MCP（Model Context Protocol）调用 Python 服务的架构设计。

**架构模式**: MCP + gRPC 混合模式

**版本**: v1.0
**更新时间**: 2026-05-05

---

## 目录

- [一、架构概述](#一架构概述)
- [二、混合模式设计](#二混合模式设计)
- [三、协议选择策略](#三协议选择策略)
- [四、Python 端实现](#四python-端实现)
- [五、Go 端实现](#五go-端实现)
- [六、调用流程](#六调用流程)
- [七、实现路径](#七实现路径)

---

## 一、架构概述

### 1.1 设计目标

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              设计目标                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   1. 灵活性: 根据场景选择最优通信协议                                        │
│   2. 性能: 核心链路保持高性能                                                │
│   3. 标准: MCP 提供与 LLM 原生对齐的工具接口                                 │
│   4. 兼容: 不破坏现有 gRPC 架构                                             │
│   5. 扩展: 支持动态工具发现和外部集成                                        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 1.2 整体架构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Go Agent Service                                  │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                        LLM Harness                                  │   │
│  └─────────────────────────────────────┬───────────────────────────────┘   │
│                                        │                                   │
│  ┌─────────────────────────────────────▼───────────────────────────────┐   │
│  │                        ReAct Executor                                │   │
│  └─────────────────────────────────────┬───────────────────────────────┘   │
│                                        │                                   │
│  ┌─────────────────────────────────────▼───────────────────────────────┐   │
│  │                        Tool Registry                                 │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐ │   │
│  │  │  Internal   │  │    gRPC     │  │     MCP     │  │  External   │ │   │
│  │  │   Tools     │  │   Tools     │  │   Tools     │  │     MCP     │ │   │
│  │  │             │  │             │  │             │  │   Servers   │ │   │
│  │  │ - kb_search │  │ - doc_read  │  │ - data_ana  │  │ - 第三方    │ │   │
│  │  │ - rag_query │  │ - vector    │  │ - llm_judge │  │   工具      │ │   │
│  │  └─────────────┘  └──────┬──────┘  └──────┬──────┘  └─────────────┘ │   │
│  └────────────────────────────────┼───────────────┼─────────────────────┘   │
│                                   │               │                         │
└───────────────────────────────────┼───────────────┼─────────────────────────┘
                                    │               │
                          ┌─────────▼───────────────▼─────────────┐
                          │         协议选择策略                    │
                          │  • 高频/大数据 → gRPC                  │
                          │  • AI工具调用 → MCP                    │
                          │  • 外部集成 → MCP                      │
                          └─────────────────────────────────────────┘
                                    │               │
                    ┌───────────────▼───────┐  ┌──▼──────────────┐
                    │   Python gRPC Server  │  │ Python MCP      │
                    │   (核心/高频能力)      │  │ Server          │
                    │                        │  │ (扩展/实验能力)  │
                    │  • Document Reader    │  │                  │
                    │  • Vector Search      │  │  • Data Analysis│
                    │  • ML Inference       │  │  • Data Cleaning│
                    │                        │  │  • ETL Pipeline │
                    └────────────────────────┘  └─────────────────┘
```

### 1.3 MCP vs gRPC 对比

| 维度 | gRPC | MCP |
|-----|------|-----|
| **设计目标** | 通用微服务通信 | LLM 工具调用协议 |
| **协议层** | HTTP/2 + Protobuf | stdio/HTTP/SSE + JSON-RPC |
| **性能** | 高（二进制） | 中（JSON 文本） |
| **类型系统** | 强类型（编译时） | 弱类型（运行时 Schema） |
| **LLM 集成** | 需要适配层 | 原生对齐 |
| **工具发现** | 需要手动定义 | 自动发现 |
| **流式支持** | 双向流 | 单向 SSE |
| **生态成熟度** | 成熟 | 新兴 |

---

## 二、混合模式设计

### 2.1 混合模式定义

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           混合模式 = gRPC + MCP                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   gRPC 路径 (性能优先)                                                      │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │  特点: 二进制协议、低延迟、双向流、强类型                            │   │
│   │  用途: 核心链路、大数据传输、高频调用、流式处理                      │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│   MCP 路径 (灵活性优先)                                                     │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │  特点: JSON 协议、工具发现、LLM 原生、动态扩展                       │   │
│   │  用途: AI 工具调用、实验功能、外部集成、低频操作                      │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 Tool 分类

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Tool 分类与协议映射                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   gRPC Tools (80%)                       MCP Tools (20%)                   │
│   ┌─────────────────────────────────┐    ┌─────────────────────────────┐  │
│   │ • DocumentReader (已实现)        │    │ • data_analyze              │  │
│   │ • VectorSearch                   │    │ • data_clean                │  │
│   │ • MLInference                    │    │ • llm_judge                 │  │
│   │ • BatchProcessing                │    │ • semantic_similarity       │  │
│   │ • StreamingExport                │    │ • etl_run                   │  │
│   │                                  │    │ • 第三方集成工具             │  │
│   │ 特点:                            │    │                             │  │
│   │ - 高频调用                       │    │ 特点:                       │  │
│   │ - 大数据量                       │    │ - 动态发现                  │  │
│   │ - 低延迟要求                     │    │ - 快速迭代                  │  │
│   │ - 已有实现                       │    │ - 实验性功能                │  │
│   └─────────────────────────────────┘    └─────────────────────────────┘  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.3 调用路径对比

```
【gRPC 路径】
Go Agent ──▶ Tool Registry (gRPC 类型) ──▶ gRPC Client ──▶ Python gRPC Server
    │                                                            │
    └─ ReAct: Tool Call ───────────────────────────────────────▶ Business Logic

【MCP 路径】
Go Agent ──▶ Tool Registry (MCP 类型) ──▶ MCP Client ──▶ Python MCP Server
    │                                                          │
    └─ ReAct: Tool Call ─────────────────────────────────────▶ Business Logic
```

---

## 三、协议选择策略

### 3.1 决策树

```
需要暴露 Python 能力？
        │
        ▼
  ┌─────────────────┐
  │ 是否已存在 gRPC？│
  └─────────────────┘
    │           │
   是           否
    │           │
    ▼           ▼
┌─────────┐  ┌─────────────────┐
│保持 gRPC│  │ 是否 AI 工具？  │
│+ 可选MCP│  └─────────────────┘
└─────────┘    │           │
              是           否
               │           │
               ▼           ▼
           ┌─────────┐  ┌───────────────┐
           │  MCP    │  │数据量>100MB?  │
           │         │  └───────────────┘
           └─────────┘    │           │
                         是          否
                          │           │
                          ▼           ▼
                       ┌───────┐  ┌─────────┐
                       │ gRPC  │  │  MCP   │
                       └───────┘  └─────────┘
```

### 3.2 选择指南

| 场景 | 推荐协议 | 理由 |
|-----|---------|------|
| 已有 gRPC 实现 | gRPC | 复用现有代码 |
| AI Agent 工具调用 | MCP | LLM 原生，自动发现 |
| 数据量 > 100MB | gRPC | 二进制高效 |
| 调用频率 > 100 QPS | gRPC | 性能优势 |
| 需要双向流 | gRPC | 协议支持 |
| 实验性功能 | MCP | 快速迭代 |
| 第三方集成 | MCP | 标准协议 |
| 低频操作 (< 1 QPS) | MCP | 开发效率 |

---

## 四、Python 端实现

### 4.1 目录结构

```
link-python/
├── mcp/                              # MCP 协议层
│   ├── __init__.py
│   ├── server.py                     # MCP Server 入口
│   ├── transport/                    # 传输层实现
│   │   ├── __init__.py
│   │   ├── stdio.py                  # stdio 传输
│   │   └── http.py                   # HTTP 传输
│   └── tools/                        # MCP Tool 定义
│       ├── __init__.py
│       ├── base.py                   # Tool 基类
│       ├── registry.py               # Tool 注册表
│       ├── data/                     # 数据分析工具
│       │   ├── __init__.py
│       │   ├── analysis.py           # data_analyze
│       │   ├── cleaning.py           # data_clean
│       │   └── etl.py                # etl_run
│       ├── evaluation/               # 评测工具
│       │   ├── __init__.py
│       │   ├── llm_judge.py
│       │   └── similarity.py
│       └── knowledge/                # 知识工具
│           ├── __init__.py
│           └── dataset_ops.py
│
├── grpc/                             # gRPC 协议层 (保持现有)
│   ├── server.py
│   └── servicer/
│       ├── doc_reader.py             # 已有
│       └── ...
│
├── services/                         # 业务逻辑层 (被 gRPC 和 MCP 共用)
│   ├── data/
│   │   ├── analyzer.py
│   │   ├── cleaner.py
│   │   └── etl.py
│   ├── evaluation/
│   │   └── judge.py
│   └── knowledge/
│       └── dataset.py
│
├── config/
│   ├── __init__.py
│   ├── settings.py                   # 配置管理
│   └── mcp_config.py                 # MCP 专用配置
│
└── scripts/
    └── generate_grpc.py              # gRPC 代码生成 (已有)
```

### 4.2 MCP Server 实现

```python
# link-python/mcp_service/server.py
from mcp.server import Server
from mcp.transport.stdio import stdio_server
from link_python.mcp.tools.registry import ToolRegistry

class MCPServer:
    """MCP Server 主入口"""

    def __init__(self):
        self.app = Server("link-python")
        self.tool_registry = ToolRegistry()
        self._setup_handlers()

    def _setup_handlers(self):
        # 注册 MCP 标准处理器
        @self.app.list_tools()
        async def list_tools() -> list[dict]:
            return self.tool_registry.list_tools()

        @self.app.call_tool()
        async def call_tool(name: str, arguments: dict) -> list[dict]:
            return await self.tool_registry.call_tool(name, arguments)

async def main():
    server = MCPServer()
    async with stdio_server() as streams:
        await server.app.run(streams[0], streams[1])

if __name__ == "__main__":
    import asyncio
    asyncio.run(main())
```

### 4.3 MCP Tool 定义示例

```python
# link-python/mcp_service/tools/data/analysis.py
from mcp.server.models import Tool
from link_python.services.data.analyzer import analyze_dataframe

class DataAnalysisTool:
    """数据分析 MCP Tool"""

    @staticmethod
    def tool_definition() -> Tool:
        return Tool(
            name="data_analyze",
            description="分析数据表，生成统计报告",
            inputSchema={
                "type": "object",
                "properties": {
                    "table_name": {
                        "type": "string",
                        "description": "要分析的数据表名"
                    },
                    "operations": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "分析操作: describe, correlation, distribution"
                    },
                    "output_table": {
                        "type": "string",
                        "description": "结果保存表名（可选）"
                    }
                },
                "required": ["table_name", "operations"]
            }
        )

    @staticmethod
    async def execute(table_name: str, operations: list, output_table: str = None) -> dict:
        """执行数据分析"""
        result = await analyze_dataframe(table_name, operations, output_table)
        return {
            "content": [
                {
                    "type": "text",
                    "text": f"分析完成: {result}"
                }
            ]
        }
```

### 4.4 复用业务逻辑

```python
# link-python/services/data/analyzer.py
# 这是被 gRPC 和 MCP 共用的业务逻辑

import pandas as pd
from typing import List

async def analyze_dataframe(
    table_name: str,
    operations: List[str],
    output_table: str = None
) -> dict:
    """分析数据表

    被 gRPC servicer 和 MCP tool 共用
    """
    # 1. 加载数据
    df = await load_table(table_name)

    # 2. 执行分析
    results = {}
    for op in operations:
        if op == "describe":
            results["describe"] = df.describe().to_dict()
        elif op == "correlation":
            results["correlation"] = df.corr().to_dict()

    # 3. 保存结果（如果指定）
    if output_table:
        await save_table(output_table, results)

    return results
```

### 4.5 配置管理

```python
# link-python/config/mcp_config.py
from pydantic_settings import BaseSettings

class MCPSettings(BaseSettings):
    """MCP 配置"""

    # 服务模式: stdio, http, sse
    mode: str = "stdio"

    # HTTP 端口（mode=http 时使用）
    port: int = 3000

    # 启用的 tool 类别
    enabled_tool_categories: list[str] = [
        "data",
        "evaluation",
        "knowledge"
    ]

    # 是否暴露 gRPC 工具（通过 adapter）
    expose_grpc_via_mcp: bool = False

    class Config:
        env_prefix = "MCP_"
```

---

## 五、Go 端实现

### 5.1 目录结构

```
link-go/internal/
├── infrastructure/
│   ├── mcp/                            # MCP 客户端基础设施
│   │   ├── client.go                  # MCP Client 封装
│   │   ├── transport.go               # 传输层 (stdio/HTTP)
│   │   └── codec.go                   # JSON-RPC 编解码
│   │
│   └── grpc/                           # gRPC 客户端 (已有)
│       └── python/
│           ├── doc_reader.go
│           └── ...
│
└── application/
    └── agent/
        ├── mcp/                        # MCP 相关
        │   ├── registry.go             # MCP Server 注册
        │   ├── tool_adapter.go         # MCP Tool → LLM Tool 转换
        │   └── dispatcher.go           # MCP 调用分发器
        │
        └── tools/                      # 统一工具层
            ├── registry.go             # 统一工具注册表
            ├── tool.go                 # Tool 定义
            ├── internal/               # 内部工具
            ├── grpc/                   # gRPC 工具
            └── mcp/                    # MCP 工具
```

### 5.2 统一 Tool 定义

```go
// link-go/internal/application/agent/tools/tool.go
package tools

// ToolType 定义工具类型
type ToolType int

const (
    ToolTypeInternal ToolType = iota
    ToolTypeGRPC
    ToolTypeMCP
)

// Tool 统一工具定义
type Tool struct {
    Name        string                 // 工具名称
    Type        ToolType               // 工具类型
    Description string                 // 工具描述
    Parameters  map[string]Parameter   // 参数定义
    Handler     ToolHandler            // 执行函数
}

// Parameter 参数定义
type Parameter struct {
    Type        string   // 类型: string, number, array, object
    Description string   // 描述
    Required    bool     // 是否必需
    Enum        []string // 枚举值（可选）
}

// ToolHandler 工具执行函数
type ToolHandler func(ctx context.Context, args map[string]interface{}) (interface{}, error)

// LLMFunction LLM Function Calling 格式
type LLMFunction struct {
    Name        string                 `json:"name"`
    Description string                 `json:"description"`
    Parameters  map[string]interface{} `json:"parameters"`
}
```

### 5.3 统一 Tool Registry

```go
// link-go/internal/application/agent/tools/registry.go
package tools

import (
    "context"
    "sync"
)

type Registry struct {
    mu         sync.RWMutex
    internal   map[string]*Tool              // 内部工具
    grpcTools  map[string]*Tool              // gRPC 工具
    mcpClients map[string]*MCPClientWrapper  // MCP 客户端
    mcpTools   map[string]*Tool              // MCP 工具
}

func NewRegistry() *Registry {
    return &Registry{
        internal:   make(map[string]*Tool),
        grpcTools:  make(map[string]*Tool),
        mcpClients: make(map[string]*MCPClientWrapper),
        mcpTools:   make(map[string]*Tool),
    }
}

// RegisterInternal 注册内部工具
func (r *Registry) RegisterInternal(tool *Tool) {
    r.mu.Lock()
    defer r.mu.Unlock()
    tool.Type = ToolTypeInternal
    r.internal[tool.Name] = tool
}

// RegisterGRPC 注册 gRPC 工具
func (r *Registry) RegisterGRPC(tool *Tool) {
    r.mu.Lock()
    defer r.mu.Unlock()
    tool.Type = ToolTypeGRPC
    r.grpcTools[tool.Name] = tool
}

// RegisterMCPClient 注册 MCP 客户端
func (r *Registry) RegisterMCPClient(name string, client *MCPClientWrapper) error {
    r.mu.Lock()
    defer r.mu.Unlock()

    r.mcpClients[name] = client

    // 自动同步 MCP server 的 tools
    tools, err := client.ListTools(context.Background())
    if err != nil {
        return err
    }

    for _, t := range tools {
        tool := &Tool{
            Name:        t.Name,
            Type:        ToolTypeMCP,
            Description: t.Description,
            Parameters:  convertParameters(t.InputSchema),
            Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
                return client.CallTool(ctx, t.Name, args)
            },
        }
        r.mcpTools[t.Name] = tool
    }

    return nil
}

// ListTools 列出所有工具
func (r *Registry) ListTools() []*Tool {
    r.mu.RLock()
    defer r.mu.RUnlock()

    var tools []*Tool
    for _, t := range r.internal {
        tools = append(tools, t)
    }
    for _, t := range r.grpcTools {
        tools = append(tools, t)
    }
    for _, t := range r.mcpTools {
        tools = append(tools, t)
    }
    return tools
}

// GetTool 获取工具
func (r *Registry) GetTool(name string) (*Tool, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    if t, ok := r.internal[name]; ok {
        return t, true
    }
    if t, ok := r.grpcTools[name]; ok {
        return t, true
    }
    if t, ok := r.mcpTools[name]; ok {
        return t, true
    }
    return nil, false
}

// CallTool 调用工具
func (r *Registry) CallTool(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
    tool, ok := r.GetTool(name)
    if !ok {
        return nil, fmt.Errorf("tool not found: %s", name)
    }
    return tool.Handler(ctx, args)
}

// ToLLMFunctions 转换为 LLM Function Calling 格式
func (r *Registry) ToLLMFunctions() []LLMFunction {
    tools := r.ListTools()
    functions := make([]LLMFunction, 0, len(tools))

    for _, t := range tools {
        functions = append(functions, LLMFunction{
            Name:        t.Name,
            Description: t.Description,
            Parameters:  convertToLLMParameters(t.Parameters),
        })
    }

    return functions
}
```

### 5.4 MCP Client 封装

```go
// link-go/internal/infrastructure/mcp/client.go
package mcp

import (
    "context"
    "encoding/json"
)

type MCPClient struct {
    transport Transport
    serverInfo ServerInfo
}

type MCPClientWrapper struct {
    client *MCPClient
    name   string
}

func NewMCPClient(transport Transport) *MCPClient {
    return &MCPClient{transport: transport}
}

func (c *MCPClient) Initialize(ctx context.Context) error {
    // 发送 initialize 请求
    return c.transport.Send(ctx, Request{
        JSONRPC: "2.0",
        ID:      1,
        Method:  "initialize",
        Params: map[string]interface{}{
            "protocolVersion": "2024-11-05",
            "capabilities": map[string]interface{}{
                "tools": map[string]bool{},
            },
        },
    })
}

func (c *MCPClient) ListTools(ctx context.Context) ([]MCPTool, error) {
    resp, err := c.transport.Call(ctx, "tools/list", nil)
    if err != nil {
        return nil, err
    }

    var result struct {
        Tools []MCPTool `json:"tools"`
    }
    if err := json.Unmarshal(resp, &result); err != nil {
        return nil, err
    }

    return result.Tools, nil
}

func (c *MCPClient) CallTool(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
    params := map[string]interface{}{
        "name":      name,
        "arguments": args,
    }

    resp, err := c.transport.Call(ctx, "tools/call", params)
    if err != nil {
        return nil, err
    }

    return resp, nil
}
```

### 5.5 Tool Dispatcher

```go
// link-go/internal/application/agent/mcp/dispatcher.go
package mcp

import (
    "context"
    "fmt"
)

type Dispatcher struct {
    registry *tools.Registry
}

func NewDispatcher(registry *tools.Registry) *Dispatcher {
    return &Dispatcher{registry: registry}
}

func (d *Dispatcher) Dispatch(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
    // 调用工具
    result, err := d.registry.CallTool(ctx, toolName, args)
    if err != nil {
        return "", fmt.Errorf("tool call failed: %w", err)
    }

    // 格式化返回结果
    return formatResult(result), nil
}

func formatResult(result interface{}) string {
    // 将结果格式化为字符串供 LLM 使用
    // ...
}
```

---

## 六、调用流程

### 6.1 ReAct 循环集成

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           ReAct 循环流程                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   1. Thought: LLM 分析当前状态                                              │
│                                                                             │
│   2. Action: LLM 选择工具                                                   │
│      ┌─────────────────────────────────────────────────────────────────┐   │
│      │ Tool Registry.ToLLMFunctions() → 提供给 LLM                     │   │
│      │                                                                  │   │
│      │ [                                                              │   │
│      │   {name: "kb_search", type: "internal"},                       │   │
│      │   {name: "doc_read", type: "grpc"},                            │   │
│      │   {name: "data_analyze", type: "mcp"}                          │   │
│      │ ]                                                              │   │
│      └─────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│   3. Observation: 执行工具调用                                               │
│      ┌─────────────────────────────────────────────────────────────────┐   │
│      │ switch tool.Type {                                             │   │
│      │   case Internal: → 直接调用                                    │   │
│      │   case GRPC:    → gRPC Client → Python                        │   │
│      │   case MCP:     → MCP Client → Python                         │   │
│      │ }                                                              │   │
│      └─────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│   4. 重复直到完成                                                           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 6.2 完整调用示例

```
【场景: Agent 需要分析数据】

1. LLM 输入:
   "分析 sales 表的统计信息和相关性"

2. ReAct Thought:
   "需要使用数据分析工具来分析 sales 表"

3. 可用工具 (Tool Registry):
   - data_analyze (MCP): 分析数据表，生成统计报告

4. LLM Action:
   {
     "tool": "data_analyze",
     "arguments": {
       "table_name": "sales",
       "operations": ["describe", "correlation"]
     }
   }

5. Go Agent Dispatcher:
   → 查询 Tool Registry: data_analyze 类型为 MCP
   → 调用 MCP Client
   → 发送 tools/call 请求到 Python MCP Server

6. Python MCP Server:
   → 接收请求
   → 路由到 DataAnalysisTool.execute()
   → 调用 analyze_dataframe() (共用业务逻辑)
   → 返回 JSON 结果

7. Go Agent:
   → 接收结果
   → 格式化为 Observation
   → 返回给 LLM

8. LLM Observation:
   "分析完成: sales 表有 1000 行，describe 显示..."
```

---

## 七、实现路径

### 7.1 阶段规划

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              实现阶段规划                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   阶段 1: Python MCP Server (Week 1)                                        │
│   ├── MCP 基础设施                                                         │
│   │   ├── mcp/server.py                                                    │
│   │   ├── mcp/transport/                                                   │
│   │   └── mcp/tools/registry.py                                            │
│   ├── 数据分析 Tools                                                       │
│   │   ├── data_analyze                                                    │
│   │   ├── data_clean                                                      │
│   │   └── etl_run                                                         │
│   └── 测试与验证                                                           │
│       └── tools/list, tools/call 手动测试                                  │
│                                                                             │
│   阶段 2: Go MCP Client (Week 2)                                           │
│   ├── MCP 客户端基础设施                                                    │
│   │   ├── infrastructure/mcp/client.go                                     │
│   │   ├── infrastructure/mcp/transport.go                                  │
│   │   └── infrastructure/mcp/codec.go                                      │
│   ├── Tool Registry 集成                                                   │
│   │   ├── application/agent/tools/registry.go                              │
│   │   └── application/agent/mcp/tool_adapter.go                            │
│   └── 测试与验证                                                           │
│       └── 端到端调用测试                                                   │
│                                                                             │
│   阶段 3: Agent 集成 (Week 3)                                              │
│   ├── ReAct 循环集成                                                       │
│   │   ├── application/agent/mcp/dispatcher.go                              │
│   │   └── Tool → LLM Function 转换                                         │
│   ├── 配置管理                                                             │
│   │   └── MCP 连接配置                                                     │
│   └── 测试与验证                                                           │
│       └── 完整流程测试                                                     │
│                                                                             │
│   阶段 4: 扩展与优化 (Week 4)                                             │
│   ├── 更多 MCP Tools                                                       │
│   ├── 性能优化                                                             │
│   ├── 监控与可观测性                                                       │
│   └── 文档完善                                                             │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 7.2 依赖项

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                                依赖项                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   Python 端:                                                                │
│   ├── mcp-python: MCP SDK                                                  │
│   ├── pydantic: 数据验证                                                   │
│   ├── pandas, numpy: 数据处理                                              │
│   └── (现有) grpcio: gRPC 服务                                             │
│                                                                             │
│   Go 端:                                                                    │
│   ├── (待选) go-mcp-sdk 或自建 MCP Client                                  │
│   └── (现有) grpc-go: gRPC 客户端                                          │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 7.3 配置示例

```yaml
# config/agent.yaml
agent:
  mcp_servers:
    - name: python-data
      transport: stdio
      command: python -m link_python.mcp.server
      # 或 HTTP 模式:
      # transport: http
      # url: http://python-service:3000/mcp

  grpc_clients:
    python:
      address: python-service:50051
```

---

## 附录

### A. MCP 协议参考

| 方法 | 描述 | 参数 | 返回值 |
|-----|------|------|--------|
| `initialize` | 初始化连接 | 协议版本、能力 | Server 信息 |
| `tools/list` | 列出可用工具 | - | `{"tools": [...]}` |
| `tools/call` | 调用工具 | name, arguments | `{"content": [...]}` |
| `resources/list` | 列出资源 | - | `{"resources": [...]}` |
| `resources/read` | 读取资源 | uri | `{"contents": [...]}` |
| `prompts/list` | 列出提示词模板 | - | `{"prompts": [...]}` |

### B. 传输层选择

| 传输层 | 适用场景 | 配置 |
|-------|---------|------|
| **stdio** | 本地开发、单机部署 | `python -m link_python.mcp.server` |
| **HTTP** | 容器/跨机器 | `http://python-service:3000/mcp` |
| **SSE** | 需要服务端推送 | `http://python-service:3000/mcp?stream` |

### C. 版本历史

| 版本 | 日期 | 变更 |
|-----|------|------|
| v1.0 | 2026-05-05 | 初版，混合模式设计 |

---

**文档版本**: v1.0
**更新时间**: 2026-05-05
