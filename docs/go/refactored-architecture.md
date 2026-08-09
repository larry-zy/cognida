# Cognida-Go 重构后架构详细说明

## 1. 整体架构图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           External Clients                                  │
│                        (HTTP / gRPC / WebSocket)                             │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                          API Gateway Layer                                   │
│                         (Gin Router / gRPC Server)                          │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                    ┌─────────────────┴─────────────────┐
                    ▼                                       ▼
┌───────────────────────────────┐     ┌───────────────────────────────────┐
│        Handler 层              │     │        Middleware                │
│    (HTTP/gRPC 协议处理)         │     │    (认证 / 日志 / 限流)           │
│                               │     │                                   │
│  ┌─────────────────────────┐  │     │  ┌─────────────────────────┐    │
│  │  handler/agent.go       │  │     │  │  middleware/auth.go     │    │
│  │  handler/chat.go        │  │     │  │  middleware/logging.go  │    │
│  │  handler/knowledge.go          │  │     │  │  middleware/rate_limit.go│    │
│  │  handler/rag.go         │  │     │  │  middleware/cors.go     │    │
│  │  handler/evaluation.go  │  │     │  └─────────────────────────┘    │
│  └─────────────────────────┘  │     └───────────────────────────────────┘
└───────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Service 层                                      │
│                         (业务逻辑编排 / 核心实现)                            │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
        ┌───────────────┬───────────────┼───────────────┬───────────────┐
        ▼               ▼               ▼               ▼               ▼
┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌──────────────┐
│ service/     │ │ service/     │ │ service/     │ │ service/     │ │ service/     │
│ agent/      │ │ rag/        │ │ llm/        │ │ knowledge/  │ │ chat/       │
│             │ │             │ │             │ │             │ │             │
│ • agent.go  │ │ • retriever │ │ • chat.go   │ │ • knowledge.go     │ │ • service.go│
│ • react.go  │ │ • pipeline  │ │ • embedding│ │ • document  │ │ • session  │
│ • tools.go  │ │ • rerank    │ │ • stream.go │ │ • chunk     │ │ • history  │
│             │ │ • optimizer │ │             │ │ • vector    │ │             │
└──────────────┘ └──────────────┘ └──────────────┘ └──────────────┘ └──────────────┘
        │               │               │               │               │
        └───────────────┴───────────────┼───────────────┴───────────────┘
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                            Repository 层                                     │
│                          (数据访问 / 外部服务调用)                             │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
        ┌──────────────────┼──────────────────┬──────────────────┐
        ▼                  ▼                  ▼                  ▼
┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐
│ repository/  │  │ repository/  │  │ repository/  │  │ repository/  │
│ mysql/       │  │ milvus/      │  │ redis/       │  │ neo4j/       │
│              │  │              │  │              │  │              │
│ • agent_repo │  │ • vector_    │  │ • cache.go   │  │ • graph_    │
│ • kb_repo    │  │   repo       │  │ • lock.go    │  │   repo       │
│ • session_   │  │ • chunk_     │  │ • queue.go   │  │ • entity_    │
│   repo       │  │   repo       │  │ • pubsub.go  │  │   repo       │
│ • chat_      │  │              │  │              │  │              │
│   repo       │  │              │  │              │  │              │
└──────────────┘  └──────────────┘  └──────────────┘  └──────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Model 层                                       │
│                    (数据模型定义 / 接口契约 / 通用类型)                       │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
        ┌──────────────────┼──────────────────┬──────────────────┐
        ▼                  ▼                  ▼                  ▼
┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐
│ model/       │  │ model/       │  │ model/       │  │ model/       │
│ agent/       │  │ rag/         │  │ knowledge/   │  │ chat/        │
│              │  │              │  │              │  │              │
│ • entity.go  │  │ • entity.go  │  │ • entity.go  │  │ • entity.go  │
│ • types.go   │  │ • types.go   │  │ • types.go   │  │ • types.go   │
│ • interface  │  │ • interface  │  │ • interface  │  │ • interface  │
│   .go        │  │   .go        │  │   .go        │  │   .go        │
└──────────────┘  └──────────────┘  └──────────────┘  └──────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                            External Storage                                  │
│                        MySQL / Milvus / Redis / Neo4j                          │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. 目录结构详解

```
cognida-go/internal/
├── handler/                      # HTTP/gRPC 处理层
│   ├── agent.go                  # Agent 相关 HTTP 处理
│   ├── chat.go                   # Chat 相关 HTTP 处理
│   ├── knowledge.go                     # Knowledge 相关 HTTP 处理
│   ├── rag.go                    # RAG 相关 HTTP 处理
│   ├── evaluation.go             # 评测相关 HTTP 处理
│   └── middleware/               # 中间件
│       ├── auth.go               # 认证中间件
│       ├── logging.go            # 日志中间件
│       ├── rate_limit.go         # 限流中间件
│       ├── cors.go               # 跨域中间件
│       └── recovery.go           # 异常恢复中间件
│
├── service/                      # 业务逻辑层（核心实现）
│   ├── agent/                    # Agent 服务
│   │   ├── agent.go              # Agent 核心服务
│   │   ├── react.go              # ReAct 编排逻辑
│   │   ├── tools.go              # 工具调用管理
│   │   └── types.go              # Service 类型定义（可嵌入 Model 类型）
│   │
│   ├── rag/                       # RAG 服务
│   │   ├── retriever.go          # 检索服务
│   │   ├── pipeline.go           # RAG 端到端流程
│   │   ├── rerank.go             # 重排服务
│   │   ├── optimizer.go          # 查询优化
│   │   └── types.go              # Service 类型定义
│   │
│   ├── llm/                       # LLM 服务
│   │   ├── chat.go               # 聊天服务
│   │   ├── embedding.go          # 向量化服务
│   │   ├── stream.go             # 流式输出
│   │   ├── client.go             # LLM 客户端封装
│   │   └── types.go              # Service 类型定义
│   │
│   ├── knowledge/                        # Knowledge 服务
│   │   ├── knowledge.go                 # Knowledge 核心服务
│   │   ├── document.go           # 文档管理
│   │   ├── chunk.go              # 文档分块
│   │   ├── vector.go             # 向量管理
│   │   └── types.go              # Service 类型定义
│   │
│   ├── chat/                      # 聊天服务
│   │   ├── service.go            # 聊天编排服务
│   │   ├── session.go            # 会话管理
│   │   ├── history.go            # 历史记录
│   │   └── types.go              # Service 类型定义
│   │
│   └── evaluation/                # 评测服务
│       ├── service.go            # 评测核心服务
│       ├── metrics.go            # 指标计算
│       ├── dataset.go            # 数据集管理
│       └── types.go              # Service 类型定义
│
├── repository/                   # 数据访问层
│   ├── mysql/                     # MySQL 数据访问
│   │   ├── agent_repo.go         # Agent 数据访问
│   │   ├── knowledge_repo.go            # Knowledge 数据访问
│   │   ├── session_repo.go       # 会话数据访问
│   │   ├── chat_repo.go          # 聊天数据访问
│   │   └── client.go             # MySQL 客户端
│   │
│   ├── milvus/                    # Milvus 向量数据库访问
│   │   ├── vector_repo.go        # 向量数据访问
│   │   ├── chunk_repo.go         # 文档块数据访问
│   │   └── client.go             # Milvus 客户端
│   │
│   ├── redis/                     # Redis 缓存/消息队列
│   │   ├── cache.go              # 缓存操作
│   │   ├── lock.go               # 分布式锁
│   │   ├── queue.go              # 队列操作
│   │   ├── pubsub.go             # 发布订阅
│   │   └── client.go             # Redis 客户端
│   │
│   ├── neo4j/                     # Neo4j 图数据库访问
│   │   ├── graph_repo.go         # 图数据访问
│   │   ├── entity_repo.go        # 实体数据访问
│   │   └── client.go             # Neo4j 客户端
│   │
│   └── llm/                       # LLM 外部服务访问
│       ├── openai_client.go      # OpenAI 客户端
│       ├── anthropic_client.go   # Anthropic 客户端
│       └── local_client.go       # 本地模型客户端
│
└── model/                        # 数据模型定义层
    ├── agent/                     # Agent 数据模型
    │   ├── entity.go             # Agent 实体定义
    │   ├── types.go              # Agent 相关类型
    │   └── repository.go         # AgentRepository 接口定义
    │
    ├── rag/                       # RAG 数据模型
    │   ├── entity.go             # RAG 实体定义
    │   ├── types.go              # RAG 相关类型
    │   └── repository.go         # RAGRepository 接口定义
    │
    ├── knowledge/                        # Knowledge 数据模型
    │   ├── entity.go             # Knowledge 实体定义
    │   ├── types.go              # Knowledge 相关类型
    │   └── repository.go         # KnowledgeRepository 接口定义
    │
    ├── chat/                      # 聊天数据模型
    │   ├── entity.go             # Chat 实体定义
    │   ├── types.go              # Chat 相关类型
    │   └── repository.go         # ChatRepository 接口定义
    │
    ├── llm/                       # LLM 数据模型
    │   ├── types.go              # LLM 相关类型
    │   └── interface.go          # LLM 接口定义
    │
    └── types/                     # 通用类型
        ├── common.go             # 通用类型定义
        ├── errors.go             # 错误类型定义
        ├── request.go            # 通用请求类型
        └── response.go           # 通用响应类型
```

---

## 3. 各层职责详解

### 3.1 Handler 层

**职责**：
- HTTP/gRPC 协议处理
- 请求参数解析和验证
- 响应封装和错误码映射
- 调用 Service 层处理业务

**特点**：
- 很薄（~10-30 行代码）
- 不包含业务逻辑
- 直接使用 Service 层定义的类型

**示例**：
```go
// handler/agent.go
func (h *Handler) ExecuteAgent(c *gin.Context) {
    var req agent.ExecuteRequest  // Service 类型
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": "invalid request"})
        return
    }

    resp, err := h.agentService.Execute(c.Request.Context(), &req)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, resp)
}
```

**不应该**：
- ❌ 包含业务逻辑
- ❌ 直接访问数据库
- ❌ 调用多个 Service（协调逻辑应在 Service 层）

---

### 3.2 Service 层

**职责**：
- 核心业务逻辑实现
- 流程控制和编排
- 规则校验
- 协调多个 Repository
- 协调外部服务调用

**特点**：
- 最厚的一层（~50-200 行代码）
- 所有业务逻辑集中在这里
- 可以相互调用（如 RAG Service 调用 LLM Service）

**示例**：
```go
// service/agent/agent.go
func (s *Service) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error) {
    // 1. 参数校验
    if err := s.validateRequest(req); err != nil {
        return nil, err
    }

    // 2. 加载 Agent 配置
    agent, err := s.repo.GetAgent(ctx, req.AgentID)
    if err != nil {
        return nil, err
    }

    // 3. 构建 Prompt
    prompt := s.buildPrompt(agent, req.Input)

    // 4. 调用 LLM（可能调用其他 Service）
    llmResp, err := s.llmService.Chat(ctx, &llm.ChatRequest{
        Model: agent.Model,
        Messages: []llm.Message{{Role: "user", Content: prompt}},
    })
    if err != nil {
        return nil, err
    }

    // 5. 处理工具调用
    if len(llmResp.ToolCalls) > 0 {
        result, err := s.executeTools(ctx, llmResp.ToolCalls)
        if err != nil {
            return nil, err
        }
        return &ExecuteResponse{Result: result}, nil
    }

    return &ExecuteResponse{Content: llmResp.Content}, nil
}
```

**类型定义策略**：
```go
// service/agent/types.go
package agent

import "link/internal/model/agent"

// ExecuteRequest 可直接嵌入 Model 类型
type ExecuteRequest struct {
    agent.ExecuteRequest     // 嵌入 Model 类型
    Stream bool `json:"stream"`  // 扩展字段
}

// ExecuteResponse 可直接嵌入 Model 类型
type ExecuteResponse struct {
    agent.ExecuteResponse    // 嵌入 Model 类型
    RequestID string `json:"request_id"`  // 扩展字段
}
```

---

### 3.3 Repository 层

**职责**：
- 数据库 CRUD 操作
- 外部服务 API 调用
- 缓存操作
- 事务管理

**特点**：
- 很薄（~5-30 行代码）
- 只做数据访问，不包含业务逻辑
- 实现 Model 层定义的接口

**示例**：
```go
// repository/mysql/agent_repo.go
package mysql

import (
    "context"
    "link/internal/model/agent"
)

type AgentRepository struct {
    db *gorm.DB
}

// 实现 model/agent/repository.go 中定义的接口
func (r *AgentRepository) GetAgent(ctx context.Context, id string) (*agent.Agent, error) {
    var a agent.Agent
    err := r.db.WithContext(ctx).Where("id = ?", id).First(&a).Error
    if err != nil {
        return nil, err
    }
    return &a, nil
}

func (r *AgentRepository) SaveAgent(ctx context.Context, a *agent.Agent) error {
    return r.db.WithContext(ctx).Save(a).Error
}
```

---

### 3.4 Model 层

**职责**：
- 定义数据实体（Entity）
- 定义值对象（Value Object）
- 定义 Repository 接口（契约）
- 定义通用类型和常量

**特点**：
- 只包含数据结构定义
- 不包含业务逻辑
- 接口定义集中在这里

**示例**：
```go
// model/agent/entity.go
package agent

// Agent 实体定义
type Agent struct {
    ID          string    `json:"id" gorm:"primaryKey"`
    Name        string    `json:"name"`
    Description string    `json:"description"`
    Model       string    `json:"model"`
    Prompt      string    `json:"prompt"`
    Tools       []Tool    `json:"tools" gorm:"foreignKey:AgentID"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

// Tool 工具定义
type Tool struct {
    ID     string `json:"id" gorm:"primaryKey"`
    AgentID string `json:"agent_id"`
    Name   string `json:"name"`
    Config string `json:"config"`
}
```

```go
// model/agent/repository.go
package agent

import "context"

// AgentRepository 接口定义（Repository 层实现此接口）
type AgentRepository interface {
    GetAgent(ctx context.Context, id string) (*Agent, error)
    SaveAgent(ctx context.Context, agent *Agent) error
    ListAgents(ctx context.Context, opts ListOptions) ([]*Agent, error)
    DeleteAgent(ctx context.Context, id string) error
}
```

---

## 4. 典型请求流程

### 4.1 Agent 执行请求流程

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           HTTP POST /api/v1/agent/execute                    │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  handler/agent.go: Handler.ExecuteAgent()                                    │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │ 1. 解析 JSON 请求 → agent.ExecuteRequest                                │ │
│  │ 2. 参数校验（gin binding）                                               │ │
│  │ 3. 调用 agentService.Execute(ctx, req)                                  │ │
│  │ 4. 封装响应 → JSON                                                       │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  service/agent/agent.go: Service.Execute()                                  │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │ 1. 参数校验（业务规则）                                                  │ │
│  │ 2. agentRepo.GetAgent(ctx, req.AgentID) → model.Agent                    │ │
│  │ 3. 构建对话 Prompt                                                        │ │
│  │ 4. llmService.Chat(ctx, llmReq) → LLM 响应                              │ │
│  │ 5. 解析工具调用                                                          │ │
│  │ 6. 如果有工具调用：toolsService.Execute()                                │ │
│  │ 7. 返回 ExecuteResponse                                                  │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                    ┌─────────────────┴─────────────────┐
                    ▼                                   ▼
┌───────────────────────────────┐     ┌───────────────────────────────────┐
│ repository/mysql/agent_repo.go│     │ repository/llm/openai_client.go    │
│                               │     │                                   │
│ SELECT * FROM agents          │     │ POST https://api.openai.com/...   │
│ WHERE id = ?                  │     │                                   │
└───────────────────────────────┘     └───────────────────────────────────┘
                    │                                   │
                    ▼                                   ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                            External Data Stores                              │
│                        MySQL Database / OpenAI API                           │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.2 RAG 检索请求流程

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          HTTP POST /api/v1/rag/retrieve                      │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  handler/rag.go: Handler.Retrieve()                                         │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │ 1. 解析请求 → rag.RetrieveRequest                                        │ │
│  │ 2. 调用 ragService.Retrieve(ctx, req)                                   │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  service/rag/retriever.go: Retriever.Retrieve()                            │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │ 1. 查询优化（可选）                                                       │ │
│  │ 2. embeddingService.Embed(ctx, query) → 向量                             │ │
│  │ 3. vectorRepo.Search(ctx, vector, topK) → 文档列表                       │ │
│  │ 4. 如果启用重排：rerankerService.Rerank(ctx, docs)                        │ │
│  │ 5. 返回 RetrieveResponse                                                  │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                    ┌─────────────────┴─────────────────┐
                    ▼                                   ▼
┌───────────────────────────────┐     ┌───────────────────────────────────┐
│ repository/milvus/vector_    │     │ repository/milvus/chunk_          │
│   repo.go                     │     │   repo.go                         │
│                               │     │                                   │
│ 向量相似度搜索                │     │ 根据文档 ID 获取完整内容           │
│                               │     │                                   │
└───────────────────────────────┘     └───────────────────────────────────┘
                    │                                   │
                    └─────────────────┬─────────────────┘
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Milvus Vector Database                          │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 5. 层间依赖关系

```
                    ┌─────────────────────────────────┐
                    │         Handler 层               │
                    │    (只依赖 Service 层)            │
                    └──────────────┬───────────────────┘
                                   │
                                   ▼
                    ┌─────────────────────────────────┐
                    │         Service 层                │
                    │  (依赖 Model 层 + Repository 层) │
                    │  (可调用其他 Service)              │
                    └──────────────┬───────────────────┘
                                   │
                    ┌──────────────┴──────────────┐
                    ▼                             ▼
        ┌───────────────────────┐   ┌───────────────────────┐
        │     Repository 层      │   │      Model 层          │
        │ (只依赖 Model 层)      │   │   (无依赖)            │
        │ 实现 Model 接口         │   │   定义接口契约         │
        └───────────┬───────────┘   └───────────────────────┘
                    │
                    ▼
        ┌───────────────────────────────────┐
        │      External Storage/API          │
        │  (MySQL / Milvus / Redis / Neo4j)  │
        └───────────────────────────────────┘
```

**依赖规则**：

| 层 | 可依赖 | 不可依赖 |
|----|--------|----------|
| Handler | Service | ❌ Repository / Model / 外部服务 |
| Service | Model / Repository / 其他 Service | ❌ Handler |
| Repository | Model | ❌ Service / Handler |
| Model | 无 | ❌ Service / Repository / Handler |

---

## 6. 关键设计原则

### 6.1 业务逻辑集中在 Service

**正确** ✅
```go
// service/agent/agent.go - 所有 Agent 逻辑集中在这里
func (s *Service) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error) {
    // 完整的 Agent 执行流程：加载、构建 Prompt、调用 LLM、处理工具
}
```

**错误** ❌
```go
// handler/agent.go - Handler 不应该包含业务逻辑
func (h *Handler) ExecuteAgent(c *gin.Context) {
    // ... 加载 Agent、构建 Prompt、调用 LLM（这些应该在 Service 层）
}
```

### 6.2 类型定义避免重复

**正确** ✅
```go
// service/agent/types.go - 通过嵌入复用 Model 类型
type ExecuteRequest struct {
    model.ExecuteRequest  // 嵌入
    Stream bool `json:"stream"`  // 只添加扩展字段
}
```

**错误** ❌
```go
// service/agent/types.go - 重复定义所有字段
type ExecuteRequest struct {
    AgentID string `json:"agent_id"`  // 重复
    Input   string `json:"input"`     // 重复
    Stream  bool   `json:"stream"`    // 扩展
}
```

### 6.3 Repository 接口只在 Model 层定义

**正确** ✅
```go
// model/agent/repository.go - 接口定义
type AgentRepository interface {
    GetAgent(ctx context.Context, id string) (*Agent, error)
}

// repository/mysql/agent_repo.go - 实现
type AgentRepository struct { ... }
func (r *AgentRepository) GetAgent(ctx context.Context, id string) (*Agent, error) {
    // 实现
}
```

**错误** ❌
```go
// service/agent/repository.go - Service 层不应该定义接口
type AgentRepository interface {
    GetAgent(ctx context.Context, id string) (*Agent, error)
}
```

### 6.4 Service 可以相互调用

**正确** ✅
```go
// service/rag/retriever.go - RAG 调用 LLM Service
type Retriever struct {
    llmService *llm.Service  // 可以依赖其他 Service
}

func (r *Retriever) retrieveWithRerank(ctx context.Context, query string) ([]*Document, error) {
    // 调用 LLM Service 进行查询优化
    optimizedQuery, err := r.llmService.OptimizeQuery(ctx, query)
    // ...
}
```

### 6.5 Handler 保持简单

**正确** ✅
```go
// handler/agent.go - 简洁的 Handler
func (h *Handler) ExecuteAgent(c *gin.Context) {
    var req agent.ExecuteRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    resp, err := h.agentService.Execute(c.Request.Context(), &req)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, resp)
}
```

**错误** ❌
```go
// handler/agent.go - 包含过多逻辑
func (h *Handler) ExecuteAgent(c *gin.Context) {
    var req agent.ExecuteRequest
    // ... 大量业务逻辑判断
    if req.AgentID == "special" {
        // ... 特殊处理
    }
    // ... 调用多个 Service
    // ... 手动组装结果
    // ... 这些都应该在 Service 层完成
}
```

---

## 7. 与外部系统交互

### 7.1 与 Python 服务通信

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Go Service                                         │
│                          (service/document)                                  │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼ gRPC
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Python gRPC Server (localhost:50051)                     │
│                    (docreader.proto: ParseDocument, OCR, Split)              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 7.2 与数据库交互

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        Go Repository Layer                                   │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐    │
│  │ MySQL    │  │ Milvus   │  │ Redis    │  │ Neo4j    │  │ External │    │
│  │ Repo     │  │ Repo     │  │ Repo     │  │ Repo     │  │ API      │    │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘    │
└───────┼─────────────┼─────────────┼─────────────┼─────────────┼──────────┘
        │             │             │             │             │
        ▼             ▼             ▼             ▼             ▼
┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐
│    MySQL    │ │   Milvus    │ │   Redis     │ │   Neo4j     │ │ OpenAI API  │
│  (元数据)   │ │  (向量数据) │ │  (缓存/队列)│ │  (图谱)    │ │ (LLM 服务)  │
└─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘
```

---

## 8. 重构前后对比

| 维度 | 重构前（4 层 Clean Architecture） | 重构后（3 层实用架构） |
|------|-----------------------------------|------------------------|
| 目录结构 | `interface/application/domain/infrastructure` | `handler/service/repository/model` |
| 业务逻辑位置 | Application 层（分散在 usecases 和 services） | Service 层（集中） |
| 接口定义 | Domain 层 + Application 层（重复） | Model 层（统一） |
| 类型转换 | Handler → UseCase DTO → Domain → Infrastructure（3 次转换） | Handler → Service（嵌入 Model）→ Model（最少转换） |
| 适配器 | 大量适配器修正架构违规 | 无适配器，依赖关系清晰 |
| Repository 数量 | 48 个接口（大量重复） | ~15 个接口（无重复） |
| 新功能开发 | 平均 60 分钟 | 预计 25 分钟 |
| 代码行数 | ~10 万行 | ~7 万行（减少 30%） |

---

## 9. 迁移路径

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           重构前                                             │
│  internal/interface/http/handler/*        →  internal/handler/*             │
│  internal/application/usecases/*          →  internal/service/*             │
│  internal/application/services/*          →  internal/service/* (合并)       │
│  internal/infrastructure/persistence/*     →  internal/repository/*           │
│  internal/domain/*                        →  internal/model/*                │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼ 迁移
┌─────────────────────────────────────────────────────────────────────────────┐
│                           重构后                                             │
│  internal/handler/*                                                         │
│  internal/service/*                                                         │
│  internal/repository/*                                                      │
│  internal/model/*                                                           │
└─────────────────────────────────────────────────────────────────────────────┘
```
