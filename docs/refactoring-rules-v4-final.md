# Link 项目重构规则 V4：务实分层架构

> 解决横切关注点（RAG、Memory）的归属问题

## 一、核心问题：横切关注点如何组织？

### 1.1 问题分析

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           什么是横切关注点？                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  【核心业务模块】（垂直领域，对外实体）                                      │
│  - Agent: AI 助手                                                           │
│  - KB: 知识库                                                               │
│  - Chat: 对话                                                               │
│  - Evaluation: 评测                                                         │
│                                                                             │
│  【能力模块】（水平能力，被多处复用）⭐ 重点                                  │
│  - RAG: 检索增强生成                                                        │
│  - Memory: 对话记忆                                                         │
│  - LLM: 模型调用                                                            │
│  - Embedding: 向量化                                                         │
│                                                                             │
│  【基础设施】（底层支持）                                                    │
│  - Store: 数据访问                                                          │
│  - Pkg: 公共库                                                              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 1.2 为什么 RAG 和 Memory 难归类？

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           RAG 的使用场景                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   Agent 使用 RAG ─────────────────────────────────────────────┐           │
│   │   Agent 执行时需要检索知识                                 │           │
│   │                                                            │           │
│   │                                                            ▼           │
│   │         ┌──────────────────────────────────────┐            │           │
│   │         │  RAG Pipeline                        │            │           │
│   │         │  ┌─────────┐  ┌─────────┐  ┌────────┐ │            │           │
│   │         │  │ Query   │  │Retrieve │  │Rerank │ │            │           │
│   │         │  │ Rewrite │  │  Docs   │  │ Docs  │ │            │           │
│   │         │  └─────────┘  └─────────┘  └────────┘ │            │           │
│   │         └──────────────────────────────────────┘            │           │
│   │                                                            │           │
│   KB 使用 RAG ──────────────────────────────────────────────┘           │
│   │   知识库需要提供检索能力                                              │
│   │                                                                        │
│   Chat 使用 RAG ────────────────────────────────────────                   │
│       对话时可能需要检索上下文                                              │
│                                                                             │
│   问题: RAG 应该放在哪里？                                                 │
│   - 放在 agent/?  不对，KB 也用                                            │
│   - 放在 kb/?     不对，Chat 也用                                          │
│   - 放在 rag/?     作为一个独立的能力模块 ✅                                │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 二、务实分层架构：三层分类

### 2.1 架构分层

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          务实分层架构                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   ┌───────────────────────────────────────────────────────────────────┐   │
│   │  Layer 1: 业务层 (Business)                                        │   │
│   │  ─────────────────────────────────────────────────────────────────  │   │
│   │  职责: 对外的业务实体、API 入口                                       │   │
│   │  模块: agent, kb, chat, evaluation                                   │   │
│   │  特点: 垂直领域，独立可测试                                           │   │
│   └───────────────────────────────────────────────────────────────────┘   │
│                                    │                                         │
│   ┌───────────────────────────────────────────────────────────────────┐   │
│   │  Layer 2: 能力层 (Capability) ⭐ 新增                              │   │
│   │  ─────────────────────────────────────────────────────────────────  │   │
│   │  职责: 可复用的技术能力，被业务层调用                                │   │
│   │  模块: rag, memory, llm, embedding, retrieval                       │   │
│   │  特点: 横切关注点，无状态，可组合                                     │   │
│   └───────────────────────────────────────────────────────────────────┘   │
│                                    │                                         │
│   ┌───────────────────────────────────────────────────────────────────┐   │
│   │  Layer 3: 基础层 (Infrastructure)                                 │   │
│   │  ─────────────────────────────────────────────────────────────────  │   │
│   │  职责: 数据访问、外部依赖                                            │   │
│   │  模块: store(mysql/milvus/redis/neo4j), pkg/errors/types           │   │
│   │  特点: 技术实现，可替换                                               │   │
│   └───────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│   ┌───────────────────────────────────────────────────────────────────┐   │
│   │  Handler 层 (HTTP)                                                 │   │
│   │  职责: 协议处理、请求验证                                            │   │
│   └───────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 依赖关系

```
                  ┌─────────────┐
                  │   Handler   │
                  └──────┬──────┘
                         │
            ┌────────────┴────────────┐
            │                         │
      ┌─────▼─────┐            ┌─────▼─────┐
      │ Business  │            │ Business  │
      │  Layer    │            │  Layer    │
      │ (agent/kb │            │ (chat/eval │
      └─────┬─────┘            └─────┬─────┘
            │                         │
            └────────┬────────────────┘
                     │
            ┌────────▼────────┐
            │  Capability    │
            │  (rag/memory   │
            │   /llm/emb)    │
            └────────┬────────┘
                     │
            ┌────────▼────────┐
            │ Infrastructure │
            │ (store/pkg)     │
            └─────────────────┘

规则:
✅ Handler → Business: 业务模块
✅ Business → Capability: 调用能力
✅ Capability → Infrastructure: 访问数据
❌ 反向依赖禁止
```

### 2.3 模块分类表

| 分类 | 模块 | 类型 | 职责 | 被谁调用 |
|------|------|------|------|---------|
| **Business** | agent | 业务 | AI 助手管理 | Handler |
| **Business** | kb | 业务 | 知识库管理 | Handler |
| **Business** | chat | 业务 | 对话管理 | Handler |
| **Business** | evaluation | 业务 | 评测任务 | Handler |
| **Capability** | rag | 能力 | 检索增强 | agent, kb, chat |
| **Capability** | memory | 能力 | 对话记忆 | agent, chat |
| **Capability** | llm | 能力 | 模型调用 | agent, rag, chat |
| **Capability** | retrieval | 能力 | 向量检索 | rag, kb |
| **Capability** | embedding | 能力 | 文本向量化 | kb, rag |
| **Infrastructure** | store | 基础 | 数据访问 | 所有 |
| **Infrastructure** | pkg | 基础 | 公共库 | 所有 |

---

## 三、目录结构：清晰分层

### 3.1 完整目录结构

```
link-go/internal/
├── handler/                          # HTTP 处理层
│   ├── agent.go
│   ├── chat.go
│   ├── kb.go
│   └── middleware/
│
├── business/                         # 业务层（对外实体）
│   ├── agent/                        # Agent 业务模块
│   │   ├── agent.go                  # Agent 业务逻辑
│   │   ├── types.go                  # Agent 类型
│   │   ├── repository.go             # Agent 存储接口
│   │   └── service.go                # Agent 对外服务
│   │
│   ├── kb/                           # 知识库业务模块
│   │   ├── kb.go                     # KB 业务逻辑
│   │   ├── types.go
│   │   ├── repository.go
│   │   └── document.go                # 文档管理
│   │
│   ├── chat/                         # 对话业务模块
│   │   ├── chat.go                   # 对话编排
│   │   ├── session.go                # 会话管理
│   │   └── types.go
│   │
│   └── evaluation/                   # 评测业务模块
│       ├── service.go
│       └── types.go
│
├── capability/                       # 能力层（可复用能力）⭐
│   ├── rag/                          # RAG 能力模块
│   │   ├── pipeline.go               # RAG 流程编排
│   │   ├── retriever.go              # 检索器接口
│   │   ├── rerank.go                 # 重排序
│   │   ├── query_rewrite.go          # 查询重写
│   │   └── types.go
│   │
│   ├── memory/                       # Memory 能力模块
│   │   ├── memory.go                 # 记忆接口
│   │   ├── buffer.go                 # 短期记忆（缓冲）
│   │   ├── persistent.go             # 长期记忆（持久化）
│   │   ├── summary.go                 # 记忆摘要
│   │   └── types.go
│   │
│   ├── llm/                          # LLM 能力模块
│   │   ├── chat.go                   # 聊天接口
│   │   ├── stream.go                 # 流式处理
│   │   ├── harness.go                # LLM Harness
│   │   └── types.go
│   │
│   ├── retrieval/                    # 检索能力模块
│   │   ├── vector.go                 # 向量检索
│   │   ├── hybrid.go                 # 混合检索
│   │   ├── graph.go                  # 图谱检索
│   │   └── types.go
│   │
│   └── embedding/                    # Embedding 能力模块
│       ├── embedding.go              # 向量化接口
│       ├── cache.go                  # 向量缓存
│       └── types.go
│
├── store/                            # 基础层（数据访问）
│   ├── mysql/
│   │   ├── agent_repo.go             # 实现 business.agent.Repository
│   │   ├── kb_repo.go                # 实现 business.kb.Repository
│   │   └── session_repo.go           # 实现 business.chat.Repository
│   │
│   ├── milvus/
│   │   ├── vector_repo.go            # 实现 capability.retrieval.VectorStore
│   │   └── chunk_repo.go             # 实现 capability.rag.ChunkStore
│   │
│   ├── redis/
│   │   ├── cache.go                  # 缓存
│   │   └── lock.go                   # 分布式锁
│   │
│   ├── neo4j/
│   │   └── graph_repo.go             # 图谱访问
│   │
│   └── grpc/                         # gRPC 客户端
│       └── doc_reader.go
│
└── pkg/                              # 公共库
    ├── errors/
    ├── types/
    └── utils/
```

### 3.2 层次关系图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           模块调用关系                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   Handler                                                                   │
│     │                                                                       │
│     ├──▶ business.agent.Service                                            │
│     │        │                                                             │
│     │        ├──▶ capability.llm.Chat                                      │
│     │        │     └──▶ store.grpc.LLMClient                              │
│     │        │                                                             │
│     │        ├──▶ capability.rag.Pipeline                                 │
│     │        │     ├──▶ capability.retrieval.Vector                        │
│     │        │     │     └──▶ store.milvus.VectorRepo                      │
│     │        │     │                                                       │
│     │        │     ├──▶ capability.llm.Chat                                │
│     │        │     │                                                       │
│     │        │     └──▶ capability.rerank.Reranker                        │
│     │        │                                                           │
│     │        └──▶ capability.memory.Buffer                                │
│     │              └──▶ store.redis.Cache                                  │
│     │                                                                      │
│     ├──▶ business.kb.Service                                               │
│     │      │                                                              │
│     │      └──▶ capability.retrieval.Vector                               │
│     │                                                                      │
│     └──▶ business.chat.Service                                            │
│            │                                                             │
│            ├──▶ capability.memory.Persistent                              │
│            └──▶ business.agent.Service                                     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 四、关键设计决策

### 4.1 RAG 的组织

```go
// ========== capability/rag/pipeline.go ==========

package rag

import (
    "link/internal/capability/retrieval"
    "link/internal/capability/llm"
    "link/internal/capability/rerank"
)

// Pipeline RAG 流程编排
type Pipeline struct {
    retriever retrieval.Retriever    // 向量检索能力
    llm       llm.ChatModel         // LLM 能力
    reranker  rerank.Reranker       // 重排能力
}

// Retrieve 增强检索
func (p *Pipeline) Retrieve(ctx context.Context, query string, topK int) (*Result, error) {
    // 1. 查询重写（可选）
    optimizedQuery := query

    // 2. 检索
    docs, err := p.retriever.Search(ctx, optimizedQuery, topK*2)
    if err != nil {
        return nil, err
    }

    // 3. 重排
    if p.reranker != nil {
        docs, err = p.reranker.Rerank(ctx, query, docs, topK)
        if err != nil {
            return nil, err
        }
    }

    return &Result{Documents: docs}, nil
}

// Generate RAG 生成
func (p *Pipeline) Generate(ctx context.Context, query string) (string, error) {
    // 1. 检索
    result, err := p.Retrieve(ctx, query, 5)
    if err != nil {
        return "", err
    }

    // 2. 构建提示
    prompt := p.buildPrompt(query, result.Documents)

    // 3. 生成
    response, err := p.llm.Chat(ctx, &llm.ChatRequest{
        Messages: []*llm.Message{{Role: "user", Content: prompt}},
    })
    if err != nil {
        return "", err
    }

    return response.Content, nil
}
```

### 4.2 Memory 的组织

```go
// ========== capability/memory/memory.go ==========

package memory

import (
    "link/internal/store/redis"
)

// Memory 记忆接口
type Memory interface {
    Add(ctx context.Context, sessionID string, messages []*Message) error
    Get(ctx context.Context, sessionID string, limit int) ([]*Message, error)
    Clear(ctx context.Context, sessionID string) error
}

// Buffer 短期记忆（基于缓冲区）
type Buffer struct {
    buffers map[string]*MessageRing
    mu      sync.RWMutex
}

func (b *Buffer) Add(ctx context.Context, sessionID string, messages []*Message) error {
    b.mu.Lock()
    defer b.mu.Unlock()

    if b.buffers[sessionID] == nil {
        b.buffers[sessionID] = NewMessageRing(20) // 保留最近 20 条
    }

    for _, msg := range messages {
        b.buffers[sessionID].Add(msg)
    }

    return nil
}

// Persistent 长期记忆（基于持久化）
type Persistent struct {
    cache    *redis.CacheStore
    summary  *Summary
}

func (p *Persistent) Add(ctx context.Context, sessionID string, messages []*Message) error {
    // 1. 存储原始消息
    for _, msg := range messages {
        key := fmt.Sprintf("session:%s:msg:%d", sessionID, msg.ID)
        if err := p.cache.Set(ctx, key, msg, 24*time.Hour); err != nil {
            return err
        }
    }

    // 2. 生成摘要（可选）
    if len(messages) > 10 {
        summary, err := p.summary.Generate(ctx, messages)
        if err == nil {
            key := fmt.Sprintf("session:%s:summary", sessionID)
            p.cache.Set(ctx, key, summary, 7*24*time.Hour)
        }
    }

    return nil
}
```

### 4.3 Agent 如何使用能力

```go
// ========== business/agent/agent.go ==========

package agent

import (
    "link/internal/capability/llm"
    "link/internal/capability/rag"
    "link/internal/capability/memory"
)

type Service struct {
    repo      Repository
    llm       llm.ChatModel
    rag       *rag.Pipeline      // 使用 RAG 能力
    memory    memory.Memory      // 使用 Memory 能力
}

// Chat 执行对话
func (s *Service) Chat(ctx context.Context, agentID string, message string) (*Response, error) {
    // 1. 获取 Agent 配置
    agent, err := s.repo.FindByID(ctx, agentID)
    if err != nil {
        return nil, err
    }

    // 2. 加载对话记忆
    history, _ := s.memory.Get(ctx, agent.SessionID, 10)

    // 3. 如果是 RAG Agent，检索知识
    var context string
    if agent.Type == AgentTypeRAG {
        result, _ := s.rag.Retrieve(ctx, message, 5)
        context = formatContext(result.Documents)
    }

    // 4. 构建 LLM 请求
    messages := append(history, &llm.Message{
        Role:    "user",
        Content: formatPrompt(message, context),
    })

    // 5. 调用 LLM
    response, err := s.llm.Chat(ctx, &llm.ChatRequest{
        Messages: messages,
    })
    if err != nil {
        return nil, err
    }

    // 6. 保存对话
    s.memory.Add(ctx, agent.SessionID, []*llm.Message{
        {Role: "user", Content: message},
        {Role: "assistant", Content: response.Content},
    })

    return &Response{Content: response.Content}, nil
}
```

---

## 五、重构映射规则

### 5.1 代码迁移映射

```
当前 Clean Architecture              务实分层架构
─────────────────────────────        ───────────────────────────
internal/                            internal/
├── interface/http/handler/     →   ├── handler/
│                                    ├── agent.go
│                                    └── chat.go
│
├── application/usecases/       →   ├── business/              # 业务层
│   ├── agent/                     ├── agent/
│   │   ├── execute.go              ├── agent.go
│   │   └── research.go             ├── types.go
│   ├── kb/                     →   ├── kb/
│   │   └── kb_usecase.go           ├── kb.go
│   └── chat/                    →   └── chat/
│       └── chat_usecase.go            └── chat.go
│
├── application/usecases/       →   ├── capability/            # 能力层 ⭐
│   ├── rag/           ───────▶   ├── rag/
│   │   └── retrieve.go              ├── pipeline.go
│   ├── llm/           ───────▶   ├── llm/
│   │   └── chat.go                  ├── chat.go
│   └── cache/          ───────▶   ├── memory/
│       └── semantic_cache.go        ├── memory.go
│                                    └── retrieval/
│                                        ├── vector.go
│                                        └── hybrid.go
│
├── domain/                    →   (合并到各模块)
│   ├── agent/         ───────▶   business/agent/types.go
│   ├── rag/           ───────▶   capability/rag/types.go
│   └── types/          ───────▶   pkg/types/
│
└── infrastructure/            →   ├── store/
    ├── persistence/              ├── mysql/
    │   ├── mysql/      ───────▶     ├── agent_repo.go
    │   └── milvus/     ───────▶   ├── milvus/
    ├── llm/           ───────▶   │   └── vector_repo.go
    └── agent/         ───────▶   └── redis/
```

### 5.2 类型定义归属

```
【类型定义归属规则】

1. 业务实体类型 → business/{module}/types.go
   - Agent, KnowledgeBase, ChatSession, EvaluationTask

2. 能力接口类型 → capability/{module}/types.go
   - RAGQuery, Retriever, Memory, LLMMessage

3. 存储接口 → business/{module}/repository.go
   - AgentRepository, KBRepository

4. 公共类型 → pkg/types/
   - ErrorResponse, PageRequest
```

---

## 六、解决你的疑问

### 6.1 RAG 和 Memory 放哪里？

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           RAG 和 Memory 的归属                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  分类: 能力层 (Capability)                                                  │
│  理由:                                                                      │
│    1. 被多个业务模块使用                                                    │
│       - agent 使用 RAG 检索知识                                              │
│       - kb 使用 RAG 提供检索能力                                            │
│       - chat 使用 RAG 增强对话                                              │
│                                                                             │
│    2. 是技术能力，不是业务实体                                               │
│       - RAG 是一种技术方案                                                  │
│       - 用户不直接操作 RAG，而是通过 Agent/KB/Chat                           │
│                                                                             │
│    3. 需要独立演进                                                          │
│       - RAG 算法可以独立升级                                                │
│       - 不影响业务模块                                                       │
│                                                                             │
│  位置: capability/rag/, capability/memory/                                   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 6.2 跨模块调用示例

```
【场景：Agent 对话时使用 RAG + Memory】

Handler
  │
  └─▶ business.agent.Service.Chat()
       │
       ├─▶ capability.rag.Pipeline.Retrieve()    检索知识
       │    └─▶ capability.retrieval.Vector       向量检索
       │         └─▶ store.milvus.VectorRepo       数据访问
       │
       ├─▶ capability.llm.ChatModel.Chat()         LLM 调用
       │    └─▶ store.grpc.LLMClient
       │
       └─▶ capability.memory.Persistent.Add()     保存对话
            └─▶ store.redis.Cache

路径清晰:
业务层 → 能力层 → 基础层
```

### 6.3 简单功能是否还需要跨很多地方？

```
【获取 Agent 列表】

Handler.agent.ListAgents()
  │
  └─▶ business.agent.Service.List()
       │
       └─▶ store.mysql.AgentRepository.FindAll()

只需要 3 步，逻辑清晰。

【复杂功能：RAG 对话】

Handler.chat.RAGChat()
  │
  └─▶ business.chat.Service.RAGChat()
       │
       ├─▶ capability.rag.Pipeline.Retrieve()    能力调用
       ├─▶ capability.llm.Chat()                能力调用
       └─▶ capability.memory.Persistent.Add()   能力调用

虽然调用多个能力，但:
1. 每个调用职责清晰
2. 能力可以独立测试
3. 业务逻辑集中在 chat.Service
```

---

## 七、实施建议

### 7.1 渐进式迁移

```
阶段 1: 创建分层结构（不迁移代码）
├── business/
├── capability/
└── handler/

阶段 2: 迁移业务模块
├── business/agent/    (从 domain/agent + application/usecases/agent)
├── business/kb/       (从 domain/knowledge + application/usecases/knowledge)
└── business/chat/     (从 application/usecases/chat)

阶段 3: 迁移能力模块
├── capability/rag/    (从 application/usecases/rag)
├── capability/llm/    (从 application/usecases/llm)
└── capability/memory/ (从 application/usecases/cache)

阶段 4: 重命名基础设施
infrastructure/persistence/ → store/

阶段 5: 清理旧代码
```

### 7.2 接口定义原则

```go
// ✅ 业务接口定义在业务层
// business/agent/repository.go
package agent

type Repository interface {
    Save(ctx context.Context, agent *Agent) error
    FindByID(ctx context.Context, id string) (*Agent, error)
}

// ✅ 能力接口定义在能力层
// capability/retrieval/retriever.go
package retrieval

type Retriever interface {
    Search(ctx context.Context, query string, topK int) ([]*Document, error)
}

// ✅ 基础层实现接口
// store/mysql/agent_repo.go
package mysql

type AgentRepository struct {
    db *gorm.DB
}

// 实现 business.agent.Repository
func (r *AgentRepository) Save(ctx context.Context, agent *agent.Agent) error {
    // ...
}
```

---

## 八、总结

### 8.1 核心改进

| 问题 | V3 方案 | V4 方案（本版） |
|------|---------|----------------|
| RAG 归属 | 不明确 | capability/rag/（能力层）|
| Memory 归属 | 不明确 | capability/memory/（能力层）|
| 组织方式 | 按模块 | **按层次 + 按模块** |
| 跨模块调用 | 混乱 | **清晰的调用链** |

### 8.2 三层分类

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              三层分类                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Business (业务层): 对外实体                                                 │
│  - agent, kb, chat, evaluation                                              │
│                                                                             │
│  Capability (能力层): 可复用能力 ⭐                                          │
│  - rag, memory, llm, retrieval, embedding                                   │
│                                                                             │
│  Infrastructure (基础层): 数据访问                                          │
│  - store, pkg                                                              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 8.3 关键原则

1. **对外实体归业务层**：Agent、KB、Chat 是用户理解的业务概念
2. **技术能力归能力层**：RAG、Memory 是技术实现，用户不直接感知
3. **数据访问归基础层**：Store 只负责访问，不包含业务逻辑
4. **清晰依赖链**：Handler → Business → Capability → Infrastructure

---

**文档版本**: v4.0
**更新时间**: 2026-05-30
**核心改进**: 引入能力层（Capability），解决横切关注点归属问题
