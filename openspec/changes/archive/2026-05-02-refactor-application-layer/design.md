# Application & Domain Layer Refactoring - Design

## Context

### 当前状态

**Application 层问题**：
1. **职责混乱**：包含基础设施实现（chunker）、业务逻辑（graph、evaluation）、用例编排混杂
2. **依赖方向错误**：application 层直接依赖 infrastructure 层（如 `infrastructure/llm/chat`）
3. **包结构不清晰**：llm/service.go 包含 4 个不同服务，缺乏边界
4. **空目录存在**：repository/ 目录为空

**Domain 层问题**：
1. **领域划分不合理**：`llm/`, `rag/`, `agent/` 是技术组件，不是业务领域
2. **`services/` 目录职责混乱**：包含应用层接口，按技术能力组织
3. **`types/interfaces/` 分层不清**：Repository 和 Service 接口混在一起
4. **`graph/` 与 `kb/` 重叠**：知识图谱本质是知识库的一部分
5. **命名使用技术术语**：`rag`, `llm`, `agent` 不是业务语言
6. **`evaluation/` 是用例不是领域**：测评是应用能力，不是核心业务

### 目标架构

```
internal/
├── domain/           # 领域层：业务实体和仓储接口
│   ├── user/        # 用户领域
│   ├── tenant/      # 租户领域
│   ├── conversation/# 对话领域（原 chat）
│   ├── knowledge/   # 知识管理领域（kb + graph 合并）
│   │   ├── entity.go      # KB, Knowledge, Chunk
│   │   ├── graph.go       # GraphNode, GraphRelation
│   │   ├── vector.go      # VectorDocument
│   │   └── repository.go  # 统一仓储接口
│   └── shared/      # 共享类型（原 types）

├── application/      # 应用层：用例编排
│   └── usecases/
│       ├── conversation/  # 对话用例（原 chat）
│       ├── knowledge/     # 知识库用例（原 kb）
│       ├── assistant/     # 助手用例（原 agent）
│       └── evaluation/    # 测评用例

└── infrastructure/   # 基础设施层：技术实现
    ├── llm/         # LLM 实现（原 domain/llm）
    ├── rag/         # RAG 管道实现（原 domain/rag）
    ├── document/    # 文档处理（原 application/chunker）
    ├── vector/      # 向量存储实现
    └── graph/       # 图谱存储实现
```

### 领域划分原则

| 类型 | 定义 | 示例 |
|------|------|------|
| 核心领域 | 核心业务价值所在 | Knowledge（知识管理） |
| 支撑领域 | 支持核心业务 | User（用户）, Tenant（租户）, Conversation（对话） |
| 通用域 | 技术组件，可购买或外包 | LLM, Vector Store, Embedding |
| 应用能力 | 组合多个领域实现的功能 | RAG, Agent, Evaluation |

## Goals / Non-Goals

**Goals:**
1. Domain 层按业务领域组织，使用业务语言命名
2. 技术组件（LLM, RAG, Agent）移至 infrastructure 或 application 层
3. Application 层只包含用例编排，不包含业务逻辑和基础设施实现
4. 清理 services/ 和 types/interfaces/ 的职责混乱
5. 包结构按业务用例组织，职责单一

**Non-Goals:**
- 不修改核心业务实体的字段定义
- 不修改基础设施层的具体实现逻辑
- 不改变 HTTP API 接口
- 不修改数据库表结构

## Decisions

### Domain 层重构决策

#### 1. LLM 移至 Infrastructure

**决策**：将 `domain/llm/` 移至 `infrastructure/llm/`

**理由**：
- LLM 是技术组件，不是业务领域
- 业务语言是"聊天"、"搜索"，不是"调用 LLM"
- 符合 DDD 的通用域（Generic Domain）定义

**迁移步骤**：
1. 创建 `infrastructure/llm/` 目录
2. 移动 `domain/llm/entity.go` → `infrastructure/llm/model.go`
3. 在需要使用的地方定义接口（如 `domain/knowledge/embedder.go`）

#### 2. RAG 移至 Infrastructure

**决策**：将 `domain/rag/` 移至 `infrastructure/rag/`

**理由**：
- RAG 是应用能力组合，不是独立领域
- 包含了 GraphRepository, VectorRepository 等越界定义
- `rag` 是技术术语，不是业务语言

**新结构**：
```
infrastructure/rag/
├── pipeline.go       # RAG 管道编排
├── retriever.go      # 检索实现
└── reranker.go       # 重排实现
```

#### 3. KB + Graph 合并为 Knowledge 领域

**决策**：将 `domain/kb/` 和 `domain/graph/` 合并为 `domain/knowledge/`

**理由**：
- Graph（知识图谱）是知识库的一种增强表示
- 分离导致割裂，管理复杂
- 从业务角度看，都是"知识管理"的一部分

**新结构**：
```
domain/knowledge/
├── entity.go        # KnowledgeBase, Knowledge, Chunk
├── graph.go         # GraphNode, GraphRelation, GraphData
├── vector.go        # VectorDocument, VectorOptions
├── repository.go    # KBRepository, GraphRepository, VectorRepository
├── retriever.go     # Retriever 接口
└── errors.go
```

#### 4. Chat 重命名为 Conversation

**决策**：`domain/chat/` → `domain/conversation/`

**理由**：
- "Chat" 是技术动作，"Conversation" 是业务概念
- 更准确描述会话管理的业务含义
- Session 和 Message 都属于会话上下文

#### 5. Agent 移至 Application 层

**决策**：将 `domain/agent/` 移至 `application/usecases/assistant/`

**理由**：
- Agent 是应用编排模式，不是业务领域
- 业务概念是"智能助手"、"研究工具"
- 作为用例实现更符合其本质

#### 6. Evaluation 移至 Application 层

**决策**：将 `domain/evaluation/` 移至 `application/usecases/evaluation/`

**理由**：
- 测评是应用用例，不是核心业务领域
- 是为了验证系统质量，不是业务目标本身

#### 7. 清理 Services 和 Types/Interfaces

**决策**：
- `domain/services/` 中的接口按领域分配到各自包中
- `domain/types/interfaces/` 中的 Service 接口移至 application 层

**理由**：
- 当前按技术能力组织，违反 DDD
- Service 接口属于应用层，不应在 domain

### Application 层重构决策

#### 8. Chunker 移至 Infrastructure

**决策**：将 `application/chunker/` 完全移至 `infrastructure/document/chunker/`

**理由**：
- Chunker 是对第三方库 `cloudwego/eino` 的封装
- 属于技术实现细节，非业务用例

**迁移步骤**：
1. 创建 `infrastructure/document/chunker/` 目录
2. 移动 `semantic.go` 到新位置
3. 更新导入路径

#### 9. LLM Service 拆分

**决策**：将 `llm/service.go` 拆分为独立的 usecase 文件

**新结构**：
```
application/usecases/llm/
├── chat_usecase.go       # ChatUseCase
├── embedding_usecase.go  # EmbeddingUseCase
├── rerank_usecase.go     # RerankUseCase
└── model_usecase.go      # ModelUseCase
```

**理由**：
- 每个文件对应一个清晰的用例
- 便于独立测试和维护

#### 10. Graph 业务逻辑下移

**决策**：将 `graph/graph.go` 中的业务逻辑移至 domain 层

**职责划分**：

| 层级 | 职责 | 代码 |
|------|------|------|
| Domain | PMI/Weight 计算、图谱合并逻辑 | `domain/knowledge/graph_service.go` |
| Application | 用例编排（提取→合并→存储） | `usecases/knowledge/graph_usecase.go` |
| Infrastructure | LLM 调用、外部服务 | `infrastructure/graph/llm_extractor.go` |

#### 11. Conversation UseCase 依赖倒置

**决策**：`chat_usecase.go` 不再直接依赖 `infrastructure/llm/chat`

**变更前**：
```go
import "link/internal/infrastructure/llm/chat"

func (s *ChatService) createChatInstance() (llmChat.Chat, error) {
    return llmChat.NewChat(config)
}
```

**变更后**：
```go
type ConversationUseCase struct {
    chatService ChatService  // 领域接口
}
```

#### 12. 删除适配器

**决策**：删除 `kb/service.go` 和 `rag/service.go` 适配器

**理由**：
- 纯粹的适配器，没有业务价值
- 增加维护成本
- 外部调用方可以直接使用 usecase

## Risks / Trade-offs

### Risk 1: Wire 配置复杂度增加

**风险**：重构后依赖注入配置变复杂
**缓解**：
- 保持接口稳定性
- 提供清晰的 wire 示例
- 分阶段更新 wire 配置

### Risk 2: 循环依赖

**风险**：domain 层新增接口可能导致循环依赖
**缓解**：
- domain 层只定义接口，不依赖 application
- 使用依赖注入接口，避免具体类型依赖

### Risk 3: 测试覆盖不足

**风险**：重构过程中可能遗漏测试
**缓解**：
- 每个阶段完成后运行测试
- 重构前补充测试用例
- 使用接口 mock 便于单元测试

## Migration Plan

### Domain 层重构阶段

### 阶段 0: 移动 LLM 和 RAG 到 Infrastructure（准备）

1. 创建 `infrastructure/llm/` 目录
2. 移动 `domain/llm/*` 到新位置
3. 创建 `infrastructure/rag/` 目录
4. 移动 `domain/rag/*` 中非领域实体到新位置
5. 更新所有导入路径
6. 运行测试验证

### 阶段 1: 合并 KB 和 Graph 为 Knowledge 领域

1. 创建 `domain/knowledge/` 目录
2. 合并 `domain/kb/*` 和 `domain/graph/*` 内容
3. 重新组织文件结构（entity.go, graph.go, vector.go, repository.go）
4. 更新所有引用
5. 运行测试验证

### 阶段 2: 重命名 Chat 为 Conversation

1. 重命名 `domain/chat/` → `domain/conversation/`
2. 重命名 `application/usecases/chat/` → `application/usecases/conversation/`
3. 更新所有导入和引用
4. 更新 wire 配置

### 阶段 3: 移动 Agent 到 Application 层

1. 创建 `application/usecases/assistant/`
2. 移动 `domain/agent/*` 实体到新位置
3. 保留必要的领域接口在 domain
4. 更新所有引用

### 阶段 4: 清理 Services 和 Types/Interfaces

1. 审查 `domain/services/*` 内容
2. 按领域分配或删除
3. 分离 `domain/types/interfaces/*` 中的 Repository 和 Service 接口
4. Service 接口移至 application 层
5. 删除空目录

### Application 层重构阶段

### 阶段 5: 移动 Chunker（低风险）

1. 创建 `infrastructure/document/chunker/`
2. 移动 `application/chunker/*` 到新位置
3. 更新导入路径
4. 运行测试验证

### 阶段 6: 拆分 LLM Service

1. 创建 `application/usecases/llm/` 目录结构
2. 拆分 `llm/service.go` 为多个 usecase 文件
3. 更新 wire 配置
4. 测试验证

### 阶段 7: 重构 Conversation UseCase

1. 定义领域接口（如果需要）
2. 修改 conversation_usecase.go 依赖接口
3. 移除 infrastructure 直接依赖
4. 更新 wire 配置

### 阶段 8: Graph 业务逻辑下移

1. 在 `domain/knowledge/graph_service.go` 添加业务逻辑
2. 移动 PMI/Weight 计算逻辑
3. 创建 `infrastructure/graph/llm_extractor.go`
4. 重构 application 用例为纯编排

### 阶段 9: Evaluation 用例重构

1. 确认 evaluation 在 application 层
2. 编排检索→生成→评估流程
3. 指标计算使用领域服务

### 阶段 10: 清理适配器

1. 确认无外部调用
2. 删除 kb/service.go
3. 删除 rag/service.go
4. 删除空目录 repository/
5. 清理无用代码

### 回滚策略

每个阶段独立提交，可随时回滚到上一个稳定版本。

## Open Questions

1. **Question**: KB 和 Graph 合并后，repository 接口如何设计？
   **Answer**: 保持独立接口，但放在同一个包中便于管理

2. **Question**: 是否需要保留向后兼容的适配器？
   **Answer**: 取决于是否有外部调用，需要检查

3. **Question**: Graph 缓存应该放在哪一层？
   **Answer**: Infrastructure 层，作为技术实现细节

4. **Question**: Agent 移至 application 层后，实体放在哪里？
   **Answer**: 实体定义可保留在 domain 作为值对象，行为实现移至 application

5. **Question**: LLM 移至 infrastructure 后，如何保证 domain 层不依赖它？
   **Answer**: 通过接口抽象，domain 定义需要的接口（如 Embedder），infrastructure 实现
