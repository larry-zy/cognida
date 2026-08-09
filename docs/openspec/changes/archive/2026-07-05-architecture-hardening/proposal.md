# 架构加固：去全局化 / 契约单一源 / 编排收口 - Proposal

## Why

本次架构评估（整体 B）确认后端分层与领域接口化是护城河，但两处结构性债务会随规模从"可维护性隐患"恶化为"改一处炸三处"的系统性摩擦：

1. **wire 的依赖注入图在 agent 边界"泄"进 package 全局可变状态**——`tools/init.go` 的 `func init()` 副作用注册、`SetRAGService`/`SetGraphService` 等运行时 setter 往 `var ragService`/`var sqlDB` 里塞、agent 实例本身也是全局单例。后果：初始化顺序隐式耦合（谁忘 Set 就静默降级）、无法同进程跑两套配置、并发写全局竞态、测试难。
2. **跨服务契约与编排缺乏物理约束**——proto 在 `cognida-go/api/proto` 与 `cognida-python/proto` 逐字节手抄双份（静默 drift 温床）；评测域存在两套并行编排引擎（Go worker 活的权威 + Python `runner.py`/`ExecuteEvaluation` 几乎死的越界）；`eino_agent.go` 1199 行上帝对象把 memory/tool/streaming 笛卡尔积展开成 6 个 `chatWith*`/`streamWith*` 变体；"记忆/会话态"概念散在 6 处各自为政。

现在做，是因为项目正围绕 agent/RAG/评测密集迭代，扩展点未收口导致每次加 agent/tool/指标都要改多处，边际成本持续上升。

## What Changes

- **Agent/Tools 去全局化**：删除 `service/agent/tools`、`service/agent/initializer`、`presets/*` 中的 package 级可变全局（`GlobalRegistry`、`var ragService/sqlDB/dsProvider/opConfig`、`var *AgentInstance`）与 `func init()` 副作用注册；改为 `NewToolRegistry(deps)` / `NewAgentRegistry(deps)` 显式构造，由 wire provider 装配，经 builder/context 传递。**BREAKING**（内部 API：所有 `SetXxx`/`GetXxx`/`InitXxx` 全局函数移除）。
- **Agent 注册表驱动**：`GetAgentByID` 的硬编码 switch 改为注册表查询；preset 用声明式描述（工具名列表 + prompt + 能力）注册，"新增 agent = 一条注册，不改 switch"。
- **拆解 eino_agent 上帝对象**：用策略/组合把 memory / tool-loop / streaming 抽成可插拔组件，消掉 6 个 `chatWith*`/`streamWith*` 重复变体，收敛为单一执行主干 + 可选装配。
- **proto 单一源 + 代码生成**：建顶层 `proto/` 为 single source-of-truth，引入 `buf generate` 同产 Go 与 Python stub，删除 `cognida-go/api/proto` 与 `cognida-python/proto` 手抄双份；CI 校验生成物一致。**BREAKING**（proto import 路径变化）。
- **Python 计算边界收口**：删除/弃用评测第二套编排引擎（`services/evaluation/runner.py` 的 `ProgressStage` 状态机 + `service.py` 的 `ExecuteEvaluation` gRPC 有状态入口），Python evaluation 收敛为无状态 `compute_*` + FastAPI 薄壳，Go worker 成为唯一权威编排。**BREAKING**（移除 gRPC EvaluationService.ExecuteEvaluation）。
- **统一 AgentState 领域门面**：把散在 `agent/memory`、`reflection/memory`、`memory_registry`、`convcontext`、`context/window`、`resultstore`/`pendingaction`/`uibinding`/`semanticcache` 的会话态收敛到一个有明确生命周期语义的 `AgentState` 门面下；明确 `chat.session`（UI 会话 write-path）与 `conversation.memory`（跨轮记忆 read-path）的边界。
- **跨服务通信选型统一**：为"Go 调 Python"定一条主通道（gRPC），evaluation 的 HTTP:18888 调用要么并入 gRPC 要么在文档明确"HTTP 仅用于 X"的规则；定义跨服务统一错误码契约（error code enum 走 proto）。

## Capabilities

### New Capabilities
- `agent-composition-root`: agent/tools/presets 的依赖装配契约——显式构造注入、注册表驱动、无 package 全局与 init 副作用、wire 覆盖到 agent 边界。
- `agent-state-store`: 统一的 AgentState 会话态领域门面——生命周期语义、收敛 memory/resultstore/pendingaction/uibinding/convcontext、界定 chat.session 与 conversation.memory 边界。
- `service-proto-contract`: 跨服务 proto 单一源 + 代码生成 + 统一错误码契约——buf 生成 Go/Python stub，CI 一致性校验。

### Modified Capabilities
- `agent-tools`: 工具注册从全局 `GlobalRegistry` + init 副作用改为实例化 `ToolRegistry` 依赖注入；工具依赖经构造传入而非运行时 setter。
- `agent-core`: `eino_agent` 执行主干去上帝对象化，memory/tool-loop/streaming 组件可插拔，消除 6 个 chatWith*/streamWith* 变体。
- `agent-service`: agent 实例从全局单例改为注册表驱动，`GetAgentByID` 由 switch 改注册表查询，preset 声明式注册。
- `evaluation-grpc`: 移除 `ExecuteEvaluation` 有状态编排 RPC，evaluation gRPC/HTTP 面收敛为无状态指标计算。
- `evaluation-executor`: 明确 Go worker 为唯一权威编排，Python 侧不再承担编排/进度状态机。

## Impact

- **cognida-go**：`internal/service/agent/{tools,initializer,presets,framework,memory,reflection,convcontext,context,resultstore,pendingaction,uibinding,semanticcache}`、`cmd/wire/{wire.go,wire_gen.go}`、`api/proto/`。
- **cognida-python**：`proto/`、`services/evaluation/{runner.py,service.py}`、`grpc_service/servicer.py`（移除 EvaluationServicer 编排）、`services/evaluation/fastapi_app.py`（收敛为薄壳）。
- **构建/CI**：新增 `proto/` 顶层目录与 `buf` 工具链、`make proto` 生成/校验命令。
- **测试**：去全局化后大量 `Set/Reset` 式测试脚手架可删除，改为构造注入的表驱动测试；需保证 `go build ./...` / `go test ./...` 全绿。
- **非破坏面**：HTTP/gRPC 对外业务接口语义不变（除移除 ExecuteEvaluation）；分层方向（handler→service→model←repository）与领域接口不动，本 change 是"加固"而非"重构领域边界"。
