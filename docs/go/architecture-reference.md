# Cognida-Go 架构参考设计

> 基于 Eino (字节跳动) 和 Grafana 的架构模式，结合项目实际情况设计

## 目录结构

```
internal/
├── pkg/                    # 公共库
│   ├── errors/            # 错误定义
│   ├── types/             # 通用类型
│   └── utils/             # 工具函数
│
├── components/             # 核心组件接口 (Domain 层接口定义)
│   ├── llm/               # LLM 组件接口
│   │   ├── interface.go   # ChatModel, EmbeddingModel
│   │   └── types.go       # Message, ToolCall 等
│   ├── retriever/         # 检索器接口
│   │   ├── interface.go   # Retriever 接口
│   │   └── types.go       # Query, Result 等
│   ├── agent/             # Agent 组件接口
│   │   ├── interface.go   # Agent 接口
│   │   └── types.go       # AgentConfig, Response 等
│   └── memory/            # Memory 组件接口
│       ├── interface.go   # Memory 接口
│       └── types.go       # Message, Summary 等
│
├── service/                # 业务逻辑实现
│   ├── agent/             # Agent 服务实现
│   │   ├── base.go        # 基础 Agent
│   │   ├── react.go       # ReAct 模式
│   │   ├── rag.go         # RAG Agent
│   │   ├── research.go    # 深度研究 Agent
│   │   └── builder.go     # Agent 构建器
│   ├── rag/               # RAG 服务
│   │   ├── retriever.go   # 检索服务
│   │   ├── hybrid.go      # 混合检索
│   │   └── rerank.go      # 重排服务
│   ├── kb/                # 知识库服务
│   │   ├── service.go     # 知识库管理
│   │   └── document.go    # 文档处理
│   ├── chat/              # 聊天服务
│   │   ├── service.go     # 聊天编排
│   │   └── session.go     # 会话管理
│   └── llm/               # LLM 服务
│       ├── chat.go        # 聊天模型
│       └── embedding.go   # 向量化
│
├── store/                  # 数据访问层
│   ├── mysql/             # MySQL 实现
│   │   ├── agent_repo.go
│   │   ├── kb_repo.go
│   │   └── session_repo.go
│   ├── milvus/            # Milvus 实现
│   │   └── vector_repo.go
│   ├── redis/             # Redis 实现
│   │   ├── cache.go
│   │   └── lock.go
│   └── interface.go       # 存储接口定义
│
└── handler/                # HTTP 处理层
    ├── agent/             # Agent handlers
    │   └── agent_handler.go
    ├── chat/              # Chat handlers
    │   └── chat_handler.go
    ├── kb/                # KB handlers
    │   └── kb_handler.go
    └── middleware/        # 中间件
        ├── auth.go
        └── logging.go
```

## 层次职责

### 1. pkg/ - 公共库

**职责**：跨层使用的公共代码

```go
// pkg/errors/errors.go
var (
    ErrAgentNotFound    = errors.New("agent not found")
    ErrInvalidRequest   = errors.New("invalid request")
    ErrRetrievalFailed  = errors.New("retrieval failed")
)

// pkg/types/message.go
type Message struct {
    Role    string
    Content string
}
```

### 2. components/ - 组件接口

**职责**：定义核心能力接口，不包含实现

```go
// components/llm/interface.go
package llm

type ChatModel interface {
    Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
    Stream(ctx context.Context, req *ChatRequest) (<-chan *Chunk, error)
}

type ChatRequest struct {
    Messages []*Message
    Tools    []*Tool
}
```

### 3. service/ - 业务逻辑

**职责**：实现 components 定义的业务接口

```go
// service/agent/react.go
package agent

import "link/internal/components/llm"

type ReActAgent struct {
    model   llm.ChatModel
    tools   []Tool
    maxIter int
}

func (a *ReActAgent) Chat(ctx context.Context, msg string) (*Response, error) {
    // ReAct 循环逻辑
}
```

### 4. store/ - 数据访问

**职责**：封装所有数据存储操作

```go
// store/mysql/agent_repo.go
package mysql

type AgentRepository struct {
    db *gorm.DB
}

func (r *AgentRepository) Save(ctx context.Context, agent *Agent) error {
    // 持久化逻辑
}
```

### 5. handler/ - HTTP 层

**职责**：处理 HTTP 请求，调用 service

```go
// handler/agent/agent_handler.go
package handler

type AgentHandler struct {
    agentService *agent.Service
}

func (h *AgentHandler) Chat(c *gin.Context) {
    var req ChatRequest
    c.ShouldBindJSON(&req)

    resp, err := h.agentService.Chat(ctx, &req)
    // 返回响应
}
```

## 依赖规则

```
handler → service → components
              ↓
            store
              ↑
              └── pkg (所有层都可使用)
```

**关键规则**：
- handler 依赖 service
- service 依赖 components 接口
- store 实现 components/ service 定义的存储接口
- pkg 可被所有层使用

## 迁移路径

### 阶段 1：建立新结构（不影响现有代码）

```
internal/
├── pkg/              # 新建
├── components/       # 新建（从 domain 提取接口）
├── service_v2/       # 新建
└── ...
```

### 阶段 2：逐步迁移

1. 迁移一个模块（如 evaluation）验证结构
2. 逐步迁移其他模块
3. 删除旧结构

### 阶段 3：清理

删除旧的 domain/application/infrastructure 结构

## 参考项目

| 项目 | 链接 | 参考点 |
|------|------|--------|
| Eino | github.com/cloudwego/eino | components/ 架构、接口设计 |
| Eino Examples | github.com/cloudwego/eino-examples | 实际用法 |
| Grafana | github.com/grafana/grafana | service/ store/ 分层 |
| LangChainGo | github.com/tmc/langchaingo | 平铺模块结构 |
