# Go 层级架构清理与整合 - Design

## 架构设计

### 层级职责定义

#### Handler 层
- **职责**：HTTP 请求处理、参数绑定、响应格式化
- **可依赖**：service 层、model 层
- **禁止**：直接访问 repository、基础设施

#### Service 层
- **职责**：核心业务逻辑编排
- **可依赖**：model 层
- **禁止**：依赖 infrastructure 层（通过接口）、依赖 handler 层

#### Repository 层
- **职责**：数据访问实现
- **可依赖**：model 层
- **禁止**：依赖 service 层

#### Model 层
- **职责**：领域实体、值对象、接口定义
- **可依赖**：无
- **包含**：
  - 领域实体（entity）
  - Repository 接口
  - 领域服务接口
  - 错误定义

#### Infrastructure 层
- **职责**：基础设施实现
- **包含**：
  - 缓存实现（cache）
  - 消息队列（redis）
  - 配置管理（config）
  - 可观测性（observability）

### Service 层模块职责（5 个）

| 模块 | 职责 | 子模块 |
|------|------|--------|
| **agent** | Agent 编排与执行 | core, builtin, framework, orchestration, reflection, tools, memory, collaboration |
| **chat** | LLM 调用与会话管理 | chat, embedding, rerank, model, session, message, converter |
| **knowledge** | 知识库、RAG、图谱 | base, document, retriever, rag_pipeline, optimizer, graph |
| **evaluation** | 评测与任务 | dataset, executor, metrics, task, worker |
| **account** | 账户管理 | tenant, user, auth, profile |

### Model 层模块职责（7 个 + 2 个）

| 模块 | 包含 |
|------|------|
| **agent** | Agent 实体、AgentExecutor 接口、Tool 接口 |
| **chat** | Message、Session、Model、Embedding 实体 |
| **knowledge** | KnowledgeBase、Document、Chunk、Graph 实体 |
| **evaluation** | Dataset、Evaluation、Task 实体 |
| **account** | Tenant、User 实体 |
| **audit** | 审计日志实体、接口（保留） |
| **cache** | 缓存实体、接口（保留） |
| **errors** | 统一错误定义 |
| **common** | Page、IDGenerator、ModelSource 等通用类型 |

## 模块整合策略

### 1. llm → chat（重命名）

**原因**：llm 名称不够准确，核心功能是 Chat

**变更内容**：
- 目录重命名：`service/llm/` → `service/chat/`
- 包名更新：`package llm` → `package chat`
- 导入路径更新：`link/internal/service/llm` → `link/internal/service/chat`

**注意**：与 conversation 合并后处理

### 2. conversation → chat（合并）

**原因**：会话管理是 Chat 功能的核心部分

**迁移内容**：
- `service/conversation/session.go` → `service/chat/session.go`
- `service/conversation/message.go` → `service/chat/message.go`

**注意**：确保与现有 `service/chat/chat_service.go` 无冲突

### 3. memory → agent（合并）

**原因**：记忆是 Agent 的核心能力

**迁移内容**：
- `service/memory/long_term.go` → `service/agent/memory/long_term.go`
- `service/memory/manage.go` → `service/agent/memory/manage.go`

**注意**：与现有 `service/agent/core/memory.go` 整合

### 4. guardrail → agent/tools（合并）

**原因**：防护是 Agent 工具的一种

**迁移内容**：
- `service/guardrail/service.go` → `service/agent/tools/guardrail.go`

**实现方式**：
```go
// GuardrailTool implements AgentTool interface
type GuardrailTool struct {
    // ... fields
}

func (t *GuardrailTool) Execute(ctx context.Context, input string) (string, error) {
    // ... implementation
}
```

### 5. rag → knowledge（合并）

**原因**：RAG 是知识库的检索增强功能

**迁移内容**：
- `service/rag/retriever.go` → `service/knowledge/retriever.go`
- `service/rag/pipeline.go` → `service/knowledge/rag_pipeline.go`
- `service/rag/optimizer.go` → `service/knowledge/optimizer.go`
- `service/rag/graph.go` → `service/knowledge/rag_graph.go`
- `service/rag/types.go` → `service/knowledge/rag_types.go`
- `service/rag/pipeline/` → `service/knowledge/rag_pipeline/`
- `service/rag/service.go` → 合并到 `service/knowledge/knowledge_base_service.go`

**注意**：保留 RAG 术语作为知识库的功能标识

### 6. graph → knowledge（合并）

**原因**：图谱是知识表示的一种形式

**迁移内容**：
- `service/graph/graph.go` → `service/knowledge/graph_service.go`
- `service/graph/dto.go` → `service/knowledge/graph_dto.go`
- `service/graph/graph_test.go` → `service/knowledge/graph_test.go`

**知识服务统一结构**：
```
service/knowledge/
├── knowledge_base_service.go   # 知识库基础服务
├── document_processor_service.go # 文档处理
├── retriever.go                 # 检索器
├── rag_pipeline.go             # RAG 流程
├── optimizer.go                # 优化器
├── graph_service.go            # 图谱服务
├── graph_dto.go                # 图谱 DTO
└── interfaces.go               # 接口定义
```

### 7. task → evaluation（合并）

**原因**：任务是评测的执行方式

**迁移内容**：
- `service/task/service.go` → `service/evaluation/task_service.go`
- `service/task/worker.go` → `service/evaluation/task_worker.go`
- `service/task/dataset_loader.go` → `service/evaluation/task_dataset.go`
- `service/task/types.go` → `service/evaluation/task_types.go`
- `service/task/executor/` → `service/evaluation/task_executor/`

**评测服务统一结构**：
```
service/evaluation/
├── service.go                  # 评测服务
├── dataset_service.go          # 数据集服务
├── task_service.go             # 任务服务（从 task 迁移）
├── task_worker.go              # 任务 Worker（从 task 迁移）
├── task_dataset.go             # 任务数据集（从 task 迁移）
├── task_types.go               # 任务类型（从 task 迁移）
├── task_executor/              # 任务执行器（从 task 迁移）
├── types.go                    # 评测类型
└── worker.go                   # 评测 Worker
```

### 8. tenant + user → account（合并）

**原因**：租户和用户都是账户管理，职责相似

**迁移内容**：
- `service/tenant/*` → `service/account/tenant_*`
- `service/user/*` → `service/account/user_*`
- 创建 `service/account/service.go` 统一入口

**账户服务统一结构**：
```
service/account/
├── service.go                  # 统一入口
├── tenant.go                  # 租户逻辑
├── tenant_service.go          # 租户服务
├── tenant_interfaces.go       # 租户接口
├── tenant_dto.go              # 租户 DTO
├── auth.go                    # 认证逻辑
├── profile.go                 # 用户档案
├── user_service.go            # 用户服务
├── user_interfaces.go         # 用户接口
└── user_dto.go                # 用户 DTO
```

**统一服务接口**：
```go
// AccountService 账户服务统一入口
type AccountService struct {
    tenantService TenantService
    userService   UserService
}

// NewAccountService 创建账户服务
func NewAccountService(/* deps */) *AccountService {
    return &AccountService{
        tenantService: NewTenantService(/* deps */),
        userService:   NewUserService(/* deps */),
    }
}
```

### 9. cache → infrastructure（移除）

**原因**：缓存是基础设施能力，不是业务逻辑

**迁移内容**：
- `service/cache/feature_flag.go` → `infrastructure/cache/feature_flag.go`
- `service/cache/semantic_cache.go` → `infrastructure/cache/semantic_cache.go`
- `service/cache/management.go` → `infrastructure/cache/management.go`
- `service/cache/metrics.go` → `infrastructure/cache/metrics.go`

**依赖方式变更**：
```go
// 之前：直接依赖 service/cache
import "link/internal/service/cache"

// 之后：通过接口依赖
import "link/internal/model/cache"

// Service 层使用接口
type ChatService struct {
    cache cache.SemanticCache  // 接口
}
```

## Model 层重组

### model/types 清理

**移动到 model/common/**：
- `page.go` → `model/common/page.go`（Req, Resp, Info 分页类型）
- `id_generator.go` → `model/common/id_generator.go`（ID 生成器）
- `model.go` → `model/common/model.go`（ModelSource 类型）

**移动到各领域模块**：
- `message.go` → `model/chat/message_entity.go`（MessageEntity, MessageFeedback）
- `session.go` → `model/chat/session_entity.go`（SessionEntity）
- `embedding.go` → `model/chat/embedding_types.go`（SourceType, MatchType, IndexInfo）
- `retriever.go` → `model/knowledge/retriever_types.go`（RetrieverType, RetrieveParams, IndexWithScore）
- `tenant.go` → `model/account/tenant_entity.go`（Tenant, TenantUser）
- `user.go` → `model/account/user_entity.go`（User, UserInfo, RefreshTokenEntity, etc.）

**删除**：
- `types/` 目录（清空后删除）

### model/services 清理

**移动**：
- `similarity.go` → `model/knowledge/similarity.go`

**删除**：
- `services/` 目录（清空后）

### Model 层对应 Service 层变更

| 操作 | Service 层 | Model 层 |
|------|-----------|----------|
| 重命名 | llm → chat | llm → chat |
| 合并 | tenant + user → account | tenant + user → account |
| 合并 | rag + graph → knowledge | rag + graph → knowledge |
| 合并 | task → evaluation | task → evaluation |

## 导入路径更新策略

### 批量更新规则

```bash
# Service 层重命名和合并
link/internal/service/llm → link/internal/service/chat
link/internal/service/conversation → link/internal/service/chat
link/internal/service/memory → link/internal/service/agent/memory
link/internal/service/guardrail → link/internal/service/agent/tools
link/internal/service/rag → link/internal/service/knowledge
link/internal/service/graph → link/internal/service/knowledge
link/internal/service/task → link/internal/service/evaluation
link/internal/service/tenant → link/internal/service/account
link/internal/service/user → link/internal/service/account
link/internal/service/cache → link/internal/infrastructure/cache

# Model 层对应变更
link/internal/model/llm → link/internal/model/chat
link/internal/model/tenant → link/internal/model/account
link/internal/model/user → link/internal/model/account
link/internal/model/rag → link/internal/model/knowledge
link/internal/model/graph → link/internal/model/knowledge
link/internal/model/task → link/internal/model/evaluation
```

### 验证步骤

1. 每次更新后运行 `go build ./...`
2. 运行 `go vet ./...` 检查
3. 更新测试代码
4. 运行 `go test ./...`

## 依赖关系图

```
┌─────────────────────────────────────────────────────────────┐
│                        Handler                              │
│  agent_handler, chat_handler, knowledge_handler, etc.      │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                         Service (5)                           │
│  agent │ chat │ knowledge │ evaluation │ account             │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                          Model (7 + 2)                        │
│  agent │ chat │ knowledge │ evaluation │ account │ audit │ cache  │
│  │ errors │ common                                              │
└─────────────────────────────────────────────────────────────┘
                            ▲
                            │
┌─────────────────────────────────────────────────────────────┐
│                       Repository                             │
│  mysql │ milvus │ neo4j │ redis                              │
└─────────────────────────────────────────────────────────────┘
```

## 测试策略

### 单元测试
- 每个合并操作后更新对应测试
- 确保测试覆盖不降低

### 集成测试
- 验证跨模块调用
- 验证端到端流程

### 架构测试
- 验证依赖方向
- 验证无循环依赖
- 验证无违规导入

## 回滚计划

如需回滚：
1. 恢复备份分支
2. 删除新分支
3. 重新 checkout 原分支

## 文档更新

需要更新的文档：
1. `link-go/CLAUDE.md` - 架构说明
2. `docs/CLEAN_ARCHITECTURE.md` - Clean Architecture 说明
3. `docs/` 下的其他架构文档
