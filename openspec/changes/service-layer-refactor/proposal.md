## Why

当前 `internal/service` 层存在架构不一致和命名问题：
- `account` 包使用 UseCase 命名（与 3-Layer 架构规范冲突）
- `account` 包在 service 层定义接口（应在 model 层）
- `agent` 包过度拆分（core, framework, orchestration, tools 等 11 个子包）
- 接口定义位置违反依赖倒置原则

## What Changes

### 统一命名规范
- UseCase → Service（与 3-Layer 架构一致）
- 移除 service 层的接口定义，迁移到 model 层

### 优化包结构
- agent 子包重组：合并 core/framework 为单一包
- 移除 test 子包（测试应使用 *_test.go 文件）

## Capabilities

### New Capabilities
- `service-cleanup`: 统一 service 层架构和命名规范

### Modified Capabilities
- 无 spec 级别行为变更，仅内部重构

## Impact

**影响范围**：`internal/service/account`、`internal/service/agent`

**保留不变**：`chat`、`evaluation`、`knowledge` 服务正常使用

**潜在风险**：
- import 路径变更影响 handler 和 repository 层
- 需要更新 wire 依赖注入配置
