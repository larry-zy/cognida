## Why

当前 Go 项目代码组织存在以下问题：

1. **职责分散**：同一概念在 infrastructure 和 service 两层重复（llm, graph, cache）
2. **错位放置**：AI 业务能力（llm, document, search, tool）被放在 infrastructure 层
3. **命名不清**：builtin、reflection、collaboration 等命名不够直观
4. **缺乏公共层**：errors、types 散落在各处，无统一管理

## What Changes

### 1. 新增 common 层

- **新增** `internal/common/` 存放公共代码
- **移动** `model/errors/` → `common/errors/`
- **移动** `model/types/` → `common/types/`

### 2. 整合到 service 层

| 动作 | 原 | 新 |
|------|---|------|
| 整合 | `infrastructure/llm/` + `service/llm/` | `service/llm/` |
| 整合 | `infrastructure/graph/` + `service/graph/` | `service/graph/` |
| 整合 | `infrastructure/cache/` + `service/cache/` | `service/cache/` |
| 移动 | `infrastructure/search/` | `service/retrieval/` |
| 移动 | `infrastructure/document/` | `service/document/` |
| 移动 | `infrastructure/tool/` | `service/tool/` |

### 3. 重命名不合理包

| 原 | 新 |
|---|------|
| `service/agent/builtin` | `service/agent/presets` |
| `service/agent/reflection` | `service/agent/think` |
| `service/agent/collaboration` | 合入 `orchestration` |

### 4. 精简 infrastructure 层

保留纯技术适配：auth, grpc, queue, redis, config, observability, crypto, mcp

## Capabilities

### New Capabilities

- `common-layer`: 公共代码层（errors, types, utils）
- `service-consolidation`: 整合 service 和 infrastructure 中的 AI 能力

### Modified Capabilities

- `agent-service`: 调整内部包名（presets, think, orchestration）

## Impact

- **Breaking**: 所有引用被移动/重命名包的代码需要更新 import
- **无外部影响**: gRPC 接口、API 接口不变
- **依赖不变**: 无新增外部依赖
