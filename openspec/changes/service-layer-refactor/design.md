## Context

当前 `internal/service` 层包含 5 个子包（account, agent, chat, evaluation, knowledge），约 145 个文件。

**现状问题**：
- `account` 包使用 UseCase 命名（AuthUseCase, ProfileUseCase, TenantUseCase）
- `account` 包在 service 层定义接口（interfaces.go），违反依赖倒置
- `agent` 包结构混乱：
  - `core/` 与 `orchestration/` 功能重复（两者都有 sequential）
  - `core/` 与 `framework/` 职责重叠
  - `test/` 子包应使用标准 *_test.go 文件

**架构约束**：
- 遵循 3-Layer 架构：handler → service → model ← repository
- Service 层只依赖 model 层
- 接口在 model 层定义，实现在 service 层

**不在本次范围**：
- `chat`、`evaluation`、`knowledge` 服务结构正常，暂不改动

## Goals / Non-Goals

**Goals**:
1. 统一命名：UseCase → Service
2. 接口归位：将 service 层接口移至 model 层
3. 简化 agent 包结构：合并碎片化子包
4. 保持功能完整性

**Non-Goals**:
- 不改变 handler 层 API
- 不修改 chat/evaluation/knowledge 服务
- 不影响现有业务功能

## Decisions

### 1. 接口迁移（account）

将 `internal/service/account/interfaces.go` 移至：
- `internal/model/user/` - 用户相关接口
- `internal/model/tenant/` - 租户相关接口

### 2. UseCase 重命名

批量重命名：
- `AuthUseCase` → `AuthService`
- `ProfileUseCase` → `ProfileService`  
- `TenantUseCase` → `TenantService`
- `AccountService` 保持不变（已经是 Service 后缀）

### 3. Agent 包重组

**分析**：
- `core/` 包含简单编排模式（sequential, planner, react）
- `orchestration/` 包含完整编排模式（sequential, parallel, loop, supervisor, conditional）
- `framework/` 包含 Eino 框架核心实现
- `core/` 的功能已被 `orchestration/` 覆盖，与 `framework/` 职责重叠

**合并方案**：
- **删除 `agent/core/`**（功能被 orchestration 覆盖）
- 全局替换 `link/internal/service/agent/core` → `link/internal/service/agent/framework`
- 保留 `agent/orchestration/`、`agent/tools/`、`agent/memory/`、`agent/collaboration/`、`agent/presets/`、`agent/reflection/`、`agent/initializer/`
- **删除 `agent/test/`**，测试文件移至对应包内并重命名为 `*_integration_test.go`

**合并后结构**：
```
agent/
├── framework/      # Eino 框架核心实现
├── orchestration/  # 编排模式（sequential, parallel, loop, supervisor）
├── tools/          # 工具实现
├── memory/         # 记忆服务
├── collaboration/  # 协作功能
├── presets/        # 预设 Agent（text2sql）
├── reflection/     # 反思机制
└── initializer/    # 初始化
```

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| Import 路径变更 | 全局搜索替换 + 编译验证 |
| Wire 配置失效 | 更新 `cmd/wire/wire.go` |
| 接口变更影响测试 | 更新所有 mock 引用 |

## Migration Plan

1. **Phase 1**: 接口迁移（account）
   - 移动接口到 model 层
   - 更新 service 实现引用

2. **Phase 2**: 重命名（account）
   - UseCase → Service
   - 更新所有引用

3. **Phase 3**: Agent 包重组
   - 删除 `agent/core/` 目录
   - 全局替换 import 路径
   - 删除 `agent/test/`，移动测试文件到对应包
   - 编译验证

4. **Phase 4**: Wire 配置更新
   - 更新依赖注入配置
   - 编译验证
