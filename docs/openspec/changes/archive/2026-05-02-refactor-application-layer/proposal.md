# Application & Domain Layer Refactoring

## Why

`internal/application` 和 `internal/domain` 层都存在架构问题：

**Application 层**：
- 包含基础设施实现细节（chunker）
- 混合业务逻辑与用例编排（graph、evaluation）
- 包职责不清晰

**Domain 层**：
- 领域划分不合理：`llm/`, `rag/`, `agent/` 是技术组件，不是业务领域
- `services/` 和 `types/interfaces/` 职责混乱，包含应用层接口
- `graph/` 与 `kb/` 领域重叠，应合并
- 包命名使用技术术语而非业务语言

这导致代码难以维护、测试和扩展，需要重构以建立清晰的层次边界和合理的领域模型。

## What Changes

### Domain 层重构

- **重命名** `domain/chat/` → `domain/conversation/` - 更准确的业务命名
- **合并** `domain/kb/` + `domain/graph/` → `domain/knowledge/` - 知识管理统一领域
- **移除** `domain/llm/` - 移至 `infrastructure/llm/`，非业务领域
- **移除** `domain/rag/` - 移至 `infrastructure/rag/`，是应用能力非领域
- **移除** `domain/agent/` - 移至 `application/usecases/assistant/`，是编排模式非领域
- **移除** `domain/evaluation/` - 移至 `application/usecases/evaluation/`，是用例非领域
- **清理** `domain/services/` - 按领域分配或删除
- **清理** `domain/types/interfaces/` - 分离 Repository（保留）和 Service（移至 application）

### Application 层重构

- **移除** `application/chunker/` - 移至 `infrastructure/document/`
- **移除** `application/repository/` - 空目录，Repository 接口属于 domain 层
- **移除** `application/llm/service.go` - 拆分为独立的 usecase
- **移除** `application/graph/graph.go` - 业务逻辑移至 domain，基础设施调用移至 infrastructure
- **移除** `application/evaluation/evaluation.go` - 业务逻辑移至 domain
- **重构** `application/usecases/chat/chat_usecase.go` - 移除 infrastructure 依赖，重命名为 conversation
- **移除** `application/usecases/kb/service.go` - 删除适配器
- **移除** `application/usecases/rag/service.go` - 删除适配器

### 新增/重组结构

```
domain/
├── user/                    # 用户领域
├── tenant/                  # 租户领域
├── conversation/            # 对话领域（原 chat）
├── knowledge/               # 知识管理领域（原 kb + graph）
│   ├── entity.go
│   ├── graph.go            # 图谱相关
│   ├── vector.go           # 向量相关
│   └── repository.go
└── shared/                  # 共享类型（原 types）

infrastructure/
├── llm/                     # LLM 实现（原 domain/llm）
├── rag/                     # RAG 实现（原 domain/rag）
├── document/                # 文档处理（原 application/chunker）
└── graph/                   # 图谱基础设施

application/usecases/
├── conversation/            # 对话用例（原 chat）
├── knowledge/               # 知识库用例（原 kb）
├── assistant/               # 助手用例（原 agent）
├── evaluation/              # 测评用例
└── shared/                  # 共享接口
```

## Capabilities

### New Capabilities

- `domain-restructuring`: Domain 层领域重构，建立合理的业务领域边界
- `llm-infrastructure`: LLM 从 domain 移至 infrastructure 作为技术组件
- `rag-infrastructure`: RAG 从 domain 移至 infrastructure 作为应用能力
- `conversation-domain`: 对话领域（原 chat），更准确的业务命名
- `knowledge-domain`: 知识管理统一领域（合并 kb + graph）
- `llm-chat-usecase`: LLM 聊天用例，封装聊天模型的调用
- `llm-embedding-usecase`: LLM 嵌入向量用例，封装向量生成
- `llm-model-usecase`: LLM 模型管理用例，处理模型配置
- `evaluation-usecase`: 测评用例，编排检索、重排、生成流程
- `clean-conversation-usecase`: 重构后的对话用例，移除 infrastructure 依赖

### Modified Capabilities

- `kb-usecase`: 知识库用例重构为 knowledge-usecase，移除适配器
- `chat-usecase`: 重构为 conversation-usecase，移除适配器

### Removed Capabilities

- `rag-usecase`: RAG 用例作为基础设施能力，不再作为应用层用例
- `agent-usecase`: Agent 用例重构为 assistant-usecase

## Impact

### Domain 层受影响的代码

- `domain/chat/*` → `domain/conversation/*` - 重命名
- `domain/kb/*` + `domain/graph/*` → `domain/knowledge/*` - 合并
- `domain/llm/*` → `infrastructure/llm/*` - 移出 domain
- `domain/rag/*` → `infrastructure/rag/*` - 移出 domain
- `domain/agent/*` → `application/usecases/assistant/*` - 移出 domain
- `domain/evaluation/*` → `application/usecases/evaluation/*` - 移出 domain
- `domain/services/*` - 删除或按领域分配
- `domain/types/interfaces/*` - 清理，Service 接口移至 application

### Application 层受影响的代码

- `application/chunker/*` → `infrastructure/document/*`
- `application/llm/service.go` - 拆分重构
- `application/graph/graph.go` - 拆分至 domain/infrastructure
- `application/evaluation/evaluation.go` - 业务逻辑下移至 domain
- `application/usecases/chat/*` → `application/usecases/conversation/*`
- `application/usecases/kb/*` → `application/usecases/knowledge/*`
- `application/usecases/kb/service.go` - 删除
- `application/usecases/rag/service.go` - 删除

### API 变更

- **BREAKING**: `domain/chat` → `domain/conversation`
- **BREAKING**: `domain/kb` + `domain/graph` → `domain/knowledge`
- **BREAKING**: `domain/llm` → `infrastructure/llm`
- **BREAKING**: `domain/rag` → `infrastructure/rag`
- **BREAKING**: `domain/agent` → `application/usecases/assistant`
- **BREAKING**: `application/chat` → `application/conversation`
- **BREAKING**: `application/kb` → `application/knowledge`

### 依赖变更

- Domain 层减少技术组件，更聚焦业务
- Application 层减少对 infrastructure 的直接依赖
- Wire 依赖注入配置需要全面更新
- 所有引用旧包路径的代码需要更新
