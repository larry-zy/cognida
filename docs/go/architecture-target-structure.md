# Cognida-Go 迁移后目录结构

> Component-based Architecture 完整目录树

## 完整目录结构

```
cognida-go/
├── api/                           # API 定义
│   ├── http/                      # HTTP API
│   │   └── v1/                    # v1 版本
│   │       ├── agent.go           # Agent API 定义
│   │       ├── rag.go             # RAG API 定义
│   │       ├── kb.go              # 知识库 API 定义
│   │       ├── chat.go            # 聊天 API 定义
│   │       └── evaluation.go      # 评测 API 定义
│   └── proto/                     # gRPC Proto 文件
│       ├── docreader.proto        # 文档读取服务
│       ├── evaluation.proto       # 评测服务
│       └── ml.proto               # ML 服务
│
├── cmd/                           # 应用入口
│   └── api/                       # API 服务
│       └── main.go                # 主入口
│
├── configs/                       # 配置文件
│   ├── config.yaml                # 主配置
│   ├── config.dev.yaml            # 开发环境
│   └── config.prod.yaml           # 生产环境
│
├── internal/                      # 内部代码
│   │
│   ├── pkg/                       # ═════ 公共库 ═════
│   │   ├── errors/                # 错误定义
│   │   │   ├── errors.go          # 包级错误变量
│   │   │   ├── validation.go      # 验证错误
│   │   │   └── errors_test.go
│   │   │
│   │   ├── types/                 # 通用类型
│   │   │   ├── message.go         # 消息类型
│   │   │   ├── request.go         # 请求类型
│   │   │   ├── response.go        # 响应类型
│   │   │   ├── page.go            # 分页类型
│   │   │   └── types_test.go
│   │   │
│   │   ├── utils/                 # 工具函数
│   │   │   ├── string.go          # 字符串工具
│   │   │   ├── time.go            # 时间工具
│   │   │   ├── json.go            # JSON 工具
│   │   │   ├── crypto.go          # 加密工具
│   │   │   └── utils_test.go
│   │   │
│   │   └── logger/                # 日志工具
│       ├── logger.go              # 日志初始化
│       ├── middleware.go          # 日志中间件
│       └── logger_test.go
│
│   ├── components/                # ═════ 组件接口层 ═════
│   │   │
│   │   ├── llm/                   # LLM 组件接口
│   │   │   ├── interface.go       # ChatModel, EmbeddingModel 接口
│   │   │   ├── types.go           # Message, ToolCall, Chunk 等
│   │   │   ├── options.go         # 配置选项
│   │   │   └── llm_test.go        # 接口测试（使用 mock）
│   │   │
│   │   ├── retriever/             # 检索器组件接口
│   │   │   ├── interface.go       # Retriever 接口
│   │   │   ├── types.go           # Query, Result, Document
│   │   │   ├── options.go
│   │   │   └── retriever_test.go
│   │   │
│   │   ├── agent/                 # Agent 组件接口
│   │   │   ├── interface.go       # Agent 接口
│   │   │   ├── types.go           # AgentConfig, Response, Tool
│   │   │   ├── memory.go          # Memory 接口
│   │   │   └── agent_test.go
│   │   │
│   │   ├── memory/                # Memory 组件接口
│   │   │   ├── interface.go       # Memory 接口
│   │   │   ├── types.go           # Message, Summary
│   │   │   └── memory_test.go
│   │   │
│   │   └── kb/                    # 知识库组件接口
│       ├── interface.go           # KnowledgeBase 接口
│       ├── types.go               # Document, Chunk, Metadata
│       └── kb_test.go
│
│   ├── service/                   # ═════ 业务逻辑层 ═════
│   │   │
│   │   ├── agent/                 # Agent 服务
│   │   │   ├── base.go            # 基础 Agent 实现
│   │   │   ├── react.go           # ReAct 模式
│   │   │   ├── rag.go             # RAG Agent
│   │   │   ├── research.go        # 深度研究 Agent
│   │   │   ├── builder.go         # Agent 构建器
│   │   │   ├── tools/             # Agent 工具集
│   │   │   │   ├── search.go      # 搜索工具
│   │   │   │   ├── sql.go         # SQL 查询工具
│   │   │   │   └── calculator.go  # 计算器工具
│   │   │   └── agent_test.go
│   │   │
│   │   ├── rag/                   # RAG 服务
│   │   │   ├── retriever.go       # 检索服务
│   │   │   ├── hybrid.go          # 混合检索
│   │   │   ├── rerank.go          # 重排服务
│   │   │   ├── pipeline.go        # RAG Pipeline
│   │   │   └── rag_test.go
│   │   │
│   │   ├── kb/                    # 知识库服务
│   │   │   ├── service.go         # 知识库管理
│   │   │   ├── document.go        # 文档处理
│   │   │   ├── chunker.go         # 分块服务
│   │   │   └── kb_test.go
│   │   │
│   │   ├── chat/                  # 聊天服务
│   │   │   ├── service.go         # 聊天编排
│   │   │   ├── session.go         # 会话管理
│   │   │   ├── stream.go          # 流式处理
│   │   │   └── chat_test.go
│   │   │
│   │   ├── llm/                   # LLM 服务
│   │   │   ├── chat.go            # 聊天模型
│   │   │   ├── embedding.go       # 向量化
│   │   │   ├── factory.go         # 模型工厂
│   │   │   └── llm_test.go
│   │   │
│   │   ├── evaluation/            # 评测服务
│   │   │   ├── service.go         # 评测服务
│   │   │   ├── task.go            # 任务管理
│   │   │   ├── executor.go        # 执行器
│   │   │   ├── grader/            # 评分器
│   │   │   │   ├── rouge.go       # ROUGE 评分
│   │   │   │   ├── bleu.go        # BLEU 评分
│   │   │   │   └── semantic.go    # 语义相似度
│   │   │   └── evaluation_test.go
│   │   │
│   │   └── graph/                 # 图谱服务（可选）
│       ├── builder.go             # 图谱构建
│       ├── query.go               # 图谱查询
│       └── graph_test.go
│
│   ├── store/                     # ═════ 数据访问层 ═════
│   │   │
│   │   ├── interface.go           # 存储接口定义
│   │   │
│   │   ├── mysql/                 # MySQL 实现
│   │   │   ├── client.go          # 客户端封装
│   │   │   ├── agent_repo.go      # Agent 存储
│   │   │   ├── kb_repo.go         # 知识库存储
│   │   │   ├── session_repo.go    # 会话存储
│   │   │   ├── eval_repo.go       # 评测存储
│   │   │   ├── user_repo.go       # 用户存储
│   │   │   └── mysql_test.go
│   │   │
│   │   ├── milvus/                # Milvus 实现
│   │   │   ├── client.go          # 客户端封装
│   │   │   ├── vector_repo.go     # 向量存储
│   │   │   ├── collection.go      # 集合管理
│   │   │   └── milvus_test.go
│   │   │
│   │   ├── redis/                 # Redis 实现
│   │   │   ├── client.go          # 客户端封装
│   │   │   ├── cache.go           # 缓存
│   │   │   ├── lock.go            # 分布式锁
│   │   │   ├── queue.go           # 队列
│   │   │   └── redis_test.go
│   │   │
│   │   ├── neo4j/                 # Neo4j 实现
│   │   │   ├── client.go          # 客户端封装
│   │   │   ├── graph_repo.go      # 图谱存储
│   │   │   └── neo4j_test.go
│   │   │
│   │   └── migrate/               # 数据迁移工具
│       ├── mysql/
│       │   └── migrations/        # 迁移脚本
│       └── migrate.go             # 迁移命令
│
│   ├── handler/                   # ═════ HTTP 处理层 ═════
│   │   │
│   │   ├── agent/                 # Agent Handlers
│   │   │   └── agent_handler.go   # Agent 处理器
│   │   │       ├── CreateAgent
│   │   │       ├── ExecuteAgent
│   │   │       └── StreamAgent
│   │   │
│   │   ├── chat/                  # Chat Handlers
│   │   │   └── chat_handler.go    # 聊天处理器
│   │   │       ├── Chat
│   │   │       └── StreamChat
│   │   │
│   │   ├── kb/                    # KB Handlers
│   │   │   └── kb_handler.go      # 知识库处理器
│   │   │       ├── CreateKB
│   │   │       ├── UploadDocument
│   │   │       └── Search
│   │   │
│   │   ├── evaluation/            # Evaluation Handlers
│   │   │   └── eval_handler.go    # 评测处理器
│   │   │       ├── CreateTask
│   │   │       ├── GetResult
│   │   │       └── ListTasks
│   │   │
│   │   ├── middleware/            # 中间件
│   │   │   ├── auth.go            # 认证中间件
│   │   │   ├── logging.go         # 日志中间件
│   │   │   ├── recovery.go        # 恢复中间件
│   │   │   ├── cors.go            # CORS 中间件
│   │   │   └── tenant.go          # 多租户中间件
│   │   │
│   │   └── response/              # 响应封装
│       ├── response.go            # 统一响应格式
│       ├── error.go               # 错误响应
│       └── page.go                # 分页响应
│
│   └── initializer/               # ═════ 初始化 ═════
│       ├── wire.go                # Wire 依赖注入
│       ├── store.go               # Store 初始化
│       ├── service.go             # Service 初始化
│       └── handler.go             # Handler 初始化
│
├── pkg/                           # 公共库（可外部使用）
│   ├── config/                    # 配置管理
│   │   ├── config.go
│   │   └── config_test.go
│   │
│   └── telemetry/                 # 可观测性
│       ├── metrics/               # 指标
│       ├── tracing/               # 链路追踪
│       └── logging/               # 日志
│
├── scripts/                       # 脚本
│   ├── generate_grpc.py           # gRPC 代码生成
│   ├── migrate.sh                 # 迁移脚本
│   └── test.sh                    # 测试脚本
│
├── test/                          # 测试
│   ├── integration/               # 集成测试
│   │   ├── agent_test.go
│   │   ├── rag_test.go
│   │   └── kb_test.go
│   └── e2e/                       # 端到端测试
│       └── api_test.go
│
├── docs/                          # 文档
│   ├── architecture-migration-guide.md    # 迁移指南
│   ├── architecture-reference.md          # 架构参考
│   ├── architecture-target-structure.md   # 本文档
│   ├── unified-chat-api.md                # 统一聊天 API
│   └── feature-roadmap.md                 # 功能规划
│
├── go.mod
├── go.sum
├── Makefile
├── Dockerfile
└── README.md
```

---

## 目录职责说明

### 1. pkg/ - 公共库

**职责**：跨层使用的公共代码，所有层都可以使用

| 子目录 | 职责 |
|--------|------|
| `errors/` | 包级错误定义，如 `ErrAgentNotFound` |
| `types/` | 通用类型，如 `Message`、`Request`、`Response` |
| `utils/` | 工具函数，如字符串、时间、加密等 |
| `logger/` | 日志工具和中间件 |

### 2. components/ - 组件接口层

**职责**：定义核心能力接口，不包含实现

| 组件 | 接口 | 说明 |
|------|------|------|
| `llm/` | `ChatModel`, `EmbeddingModel` | LLM 调用接口 |
| `retriever/` | `Retriever` | 检索接口 |
| `agent/` | `Agent`, `Tool`, `Memory` | Agent 组件接口 |
| `memory/` | `Memory` | 记忆接口 |
| `kb/` | `KnowledgeBase` | 知识库接口 |

### 3. service/ - 业务逻辑层

**职责**：实现 components 定义的业务接口，包含具体业务逻辑

| 服务 | 职责 |
|------|------|
| `agent/` | Agent 实现（ReAct、RAG、Research） |
| `rag/` | RAG 流程、检索、重排 |
| `kb/` | 知识库管理、文档处理 |
| `chat/` | 聊天编排、会话管理 |
| `llm/` | LLM 客户端封装 |
| `evaluation/` | 评测任务和评分 |

### 4. store/ - 数据访问层

**职责**：封装所有数据存储操作

| 存储 | 实现 |
|------|------|
| `mysql/` | 元数据、用户、会话 |
| `milvus/` | 向量存储 |
| `redis/` | 缓存、锁、队列 |
| `neo4j/` | 知识图谱 |

### 5. handler/ - HTTP 处理层

**职责**：处理 HTTP 请求，调用 service

| 处理器 | 路由 |
|--------|------|
| `agent/` | `/api/v1/agents/*` |
| `chat/` | `/api/v1/chat` |
| `kb/` | `/api/v1/kb/*` |
| `evaluation/` | `/api/v1/evaluations/*` |

---

## 依赖关系图

```
┌─────────────────────────────────────────────────────────────┐
│                        handler/                              │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐        │
│  │  agent  │  │  chat   │  │    kb   │  │   eval  │        │
│  └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘        │
└───────┼────────────┼────────────┼────────────┼──────────────┘
        │            │            │            │
        ▼            ▼            ▼            ▼
┌─────────────────────────────────────────────────────────────┐
│                        service/                              │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐        │
│  │  agent  │  │   rag   │  │    kb   │  │  eval   │        │
│  └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘        │
└───────┼────────────┼────────────┼────────────┼──────────────┘
        │            │            │            │
        ▼            ▼            ▼            ▼
┌─────────────────────────────────────────────────────────────┐
│                      components/                             │
│  ┌──────┐  ┌──────────┐  ┌──────┐  ┌─────┐  ┌────────┐    │
│  │ llm  │  │retriever │  │agent │  │mem  │  │   kb   │    │
│  └──┬───┘  └────┬─────┘  └──┬───┘  └──┬──┘  └────┬───┘    │
└─────┼──────────┼──────────┼──────────┼──────────┼─────────┘
      │          │          │          │          │
      ▼          ▼          ▼          ▼          ▼
┌─────────────────────────────────────────────────────────────┐
│                        store/                                │
│  ┌──────┐  ┌──────┐  ┌──────┐  ┌──────┐                    │
│  │ mysql│  │milvus │  │ redis │  │ neo4j │                   │
│  └──────┘  └──────┘  └──────┘  └──────┘                    │
└─────────────────────────────────────────────────────────────┘

      ┌─────────────────────────────────────────┐
      │                  pkg/                    │
      │  (所有层都可以使用)                      │
      └─────────────────────────────────────────┘
```

---

## 迁移对照表

### 旧结构 → 新结构

| 旧路径 | 新路径 | 变化说明 |
|--------|--------|----------|
| `domain/errors/` | `pkg/errors/` | 移到公共库 |
| `domain/types/*.go` | `pkg/types/` | 移到公共库 |
| `domain/llm/` | `components/llm/` | 接口层 |
| `domain/agent/` | `components/agent/` + `service/agent/` | 接口+实现分离 |
| `domain/rag/` | `components/retriever/` + `service/rag/` | 拆分检索器和 RAG |
| `domain/knowledge/` | `components/kb/` + `service/kb/` | 接口+实现 |
| `domain/memory/` | `components/memory/` + `service/memory/` | 接口+实现 |
| `domain/evaluation/` | `service/evaluation/` | 纯服务层 |
| `application/usecases/` | `service/` | 合并到服务层 |
| `application/services/` | `service/` | 合并到服务层 |
| `infrastructure/persistence/` | `store/` | 统一存储层 |
| `infrastructure/llm/` | `service/llm/` | 服务实现 |
| `infrastructure/agent/` | `service/agent/` | 服务实现 |
| `interface/http/handler/` | `handler/` | 简化处理层 |
| `interface/http/middleware/` | `handler/middleware/` | 移到处理层 |

---

## 文件命名规范

### 服务层

```
service/<module>/
├── service.go           # 主服务实现
├── pipeline.go          # 流程编排
├── executor.go          # 执行器
├── builder.go           # 构建器
├── factory.go           # 工厂
└── <module>_test.go     # 测试
```

### 存储层

```
store/<engine>/
├── client.go            # 客户端封装
├── <entity>_repo.go     # 实体存储
└── <engine>_test.go     # 测试
```

### 处理层

```
handler/<module>/
└── <module>_handler.go  # 处理器
```

---

## 导入路径示例

### 服务层导入

```go
package agent

import (
    "link/internal/components/llm"      // 组件接口
    "link/internal/components/agent"    // 组件接口
    "link/internal/store"               // 存储接口
    "link/internal/pkg/errors"          // 公共错误
)
```

### 存储层导入

```go
package mysql

import (
    "link/internal/components/agent"    // 组件接口（如需要）
    "link/internal/pkg/errors"          // 公共错误
)
```

### 处理层导入

```go
package handler

import (
    "link/internal/service/agent"       // 服务
    "link/internal/pkg/types"           # 公共类型
)
```
