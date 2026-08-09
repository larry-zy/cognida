## Why

Cognida-Go 当前采用 4 层 Clean Architecture（Interface/Application/Domain/Infrastructure），导致代码冗余严重：层间转换代码占比 35%，Repository 接口重复定义 48 个，新功能开发时间平均 60 分钟。简化为 3 层实用架构可减少 30% 代码量，提升开发效率 58%。

## What Changes

- **目录结构重构**：`internal/application/usecases/*` + `internal/application/services/*` → `internal/service/*`
- **层合并**：Interface → Handler，Application → Service，Infrastructure/Persistence → Repository，Domain → Model
- **Agent 服务重构**：`service/agent/` 采用分层结构
  - `core/` 存放通用编排引擎（ReAct、工具调用、记忆管理）
  - `builtin/` 存放业务 Agent（text2sql、data_analysis、code_review 等）
  - `custom/` 支持自定义 Agent
- **删除适配器**：移除 `AgentExecutableAdapter` 等适配器模式，直接使用正确接口
- **简化 DTO**：Handler 直接使用 Service 类型，Service 直接使用 Model 实体，减少 50% 类型转换
- **接口统一**：Repository 接口只在 Model 层定义，从 48 个减少到约 15 个
- **依赖修复**：消除 Application → Infrastructure 违规依赖

**BREAKING**: 内部包路径全面变更（`link/internal/application/*` → `link/internal/service/*`，`link/internal/domain/*` → `link/internal/model/*`），外部 API 接口保持不变。

## Capabilities

### New Capabilities

无新增业务能力，本变更为架构重构，不改变外部行为。

### Modified Capabilities

无规格级别行为变更。重构仅影响内部实现，外部 API 接口、请求/响应格式、业务逻辑均保持一致。

## Impact

- **代码移动**：约 30% 文件需要移动或重命名
- **导入路径**：所有 Go 文件需要更新 import 路径
- **测试更新**：单元测试、集成测试需要更新导入路径和 mock 对象
- **外部系统**：无影响，外部 HTTP/gRPC API 接口保持不变
- **依赖关系**：无新增或删除外部依赖
- **数据库**：无 Schema 变更
- **部署**：无变化，编译产物功能等价
