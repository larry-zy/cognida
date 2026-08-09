# 架构加固：去全局化 / 契约单一源 / 编排收口 - Design

## Context

架构评估（整体 B）确认后端分层（handler→service→model←repository）与领域接口化是护城河，本 change 不动这条主干，而是修复两处随规模恶化的结构性债务：

1. **agent 边界处 wire 的依赖注入图"泄"进 package 全局可变状态**。具体证据（当前代码）：
   - `service/agent/tools/init.go` 有 `func init()` 副作用注册，进程加载即把工具塞进 `GlobalRegistry`；
   - `service/agent/tools/registry.go:16` 有 `var GlobalRegistry = NewGlobalRegistry()` 单例；
   - `tools/rag_query.go:15` 有 `var ragService RAGQueryService` + `SetRAGService()` 运行时 setter，`graph_query.go` 有 `SetGraphService()`，`etl_run.go` 依赖包级 `opConfig`；
   - `initializer/init.go:425` 的 `GetAgentByID` 返回全局构造的 agent 实例。
   后果：初始化顺序隐式耦合（谁忘 `Set` 就静默降级为 nil 依赖）、无法在同进程跑两套配置、并发写全局有竞态、测试要靠 `Set/Reset` 脚手架（如 `graph_query_gating_test.go`、`operation_tools_integration_test.go` 里的 `oldCfg := opConfig`）。

2. **跨服务契约与编排缺物理约束**：
   - proto 在 `cognida-go/api/proto`（`analytics/docreader/evaluation/judge/quality`）与 `cognida-python/proto`（手抄 `.proto` + 生成的 `*_pb2.py`）逐字节双份，静默 drift；
   - 评测域两套并行编排：Go worker（活的权威）+ Python `services/evaluation/runner.py` 的 `ProgressStage` 状态机 + `ExecuteEvaluation` 流式 gRPC（几乎死的越界）；
   - `framework/eino_agent.go` 1199 行上帝对象，把 memory/tool/streaming 笛卡尔积展开成 `chatWithMemory` / `chatWithMemoryAndTools` / `chatWithMemoryOnly` / `chatWithTools` / `chatWithoutTools` / `streamWithTools` / `streamWithoutTools` 等 7 个变体；
   - 会话态散在 `memory` / `reflection` / `framework/memory_registry.go` / `convcontext` / `context/window` / `resultstore` / `pendingaction` / `uibinding` / `semanticcache` 各自为政。

现在做，是因为项目正围绕 agent/RAG/评测密集迭代，扩展点未收口导致每次加 agent/tool/指标都要改多处，边际成本持续上升。本 change 是"加固"而非"重构领域边界"。

## Goals / Non-Goals

### Goals
- 消灭 agent 边界所有 package 级可变全局与 `func init()` 副作用注册，改为显式构造注入（`NewToolRegistry(deps)` / `NewAgentRegistry(deps)`），由 wire provider 装配到 agent 边界。
- `GetAgentByID` 从全局单例 + 隐式装配改为注册表查询；preset 声明式注册（工具名列表 + prompt + 能力），"新增 agent = 一条注册，不改代码分支"。
- 把 `eino_agent` 的 7 个 `chatWith*`/`streamWith*` 变体收敛为单一执行主干 + 可插拔 memory/tool-loop/streaming 组件。
- proto 单一源：顶层 `proto/` 为 source-of-truth，`buf generate` 同产 Go 与 Python stub，删手抄双份，CI 校验生成物一致。
- 评测编排收口：删 Python 第二套编排（`runner.py` 状态机 + `ExecuteEvaluation` gRPC），Python 评测收敛为无状态 `compute_*` + FastAPI 薄壳，Go worker 唯一权威。
- 统一 `AgentState` 门面收敛 6+ 处会话态，界定 `chat.session`（UI 写路径）与 `conversation.memory`（跨轮记忆读路径）边界。

### Non-Goals
- 不改 handler/service/model/repository 分层方向与领域接口签名。
- 不改对外 HTTP/gRPC 业务接口语义（唯一例外：移除 `ExecuteEvaluation`）。
- 不重写 agent 业务逻辑本身（tool 执行语义、评测指标算法不变）。
- 不引入新的存储或跨服务传输协议（gRPC/HTTP 既有通道内收敛，不新增第三种）。
- 不做前端改动。

## Decisions

### 1. 去全局化：wire 显式构造注入，替代 init 副作用与 setter

**决策**：删除 `GlobalRegistry` 单例、`var ragService/sqlDB/dsProvider/opConfig`、`func init()` 注册、所有 `SetXxx` setter；改为 `NewToolRegistry(deps ToolDeps) *ToolRegistry`，deps 显式携带 RAG/Graph/SQL/审计等依赖，由 wire provider 构造并经 builder/context 传递给 agent。

**为什么选显式构造注入，不选保留 setter + 加锁**：setter 加锁只解决竞态，解决不了"初始化顺序隐式耦合"和"无法同进程跑两套配置"——这两个才是根因。全局单例 + setter 的本质是把依赖图藏在包加载时序里，而 wire 的价值恰恰是把依赖图显式化；两者冲突。显式构造让"缺依赖"变成编译期/构造期错误而非运行期静默降级。

**备选方案**：
- (A) 全局单例 + `sync.Once` 懒加载：仍无法同进程多配置，且 `Once` 把顺序问题推迟到首次调用，测试仍需 reset。否决。
- (B) DI 容器（如 dig/fx 的 service locator）：引入运行期反射查找，弱化编译期检查，与项目已用 wire（编译期生成）的方向相反。否决。
- (C) 本决策（wire provider + 显式 `NewXxx(deps)`）：编译期可验证、可多实例、测试直接传 mock deps 免脚手架。选定。

### 2. agent 注册表驱动，替代 GetAgentByID 的 switch/隐式装配

**决策**：新增 `AgentRegistry`（`Register(spec AgentSpec)` / `Get(id) (Agent, bool)` / `List()`），`initializer/init.go` 的 `GetAgentByID` 改为向注册表查询。preset 以声明式 `AgentSpec{ID, Name, ToolNames []string, Prompt, Capabilities}` 注册，agent 实例由注册表用 `ToolRegistry` 按 `ToolNames` 惰性/构造装配。`cmd/wire` 用 `NewRegistryAgentOrchestrator(registry.Get)` 替换现在的 `NewRegistryAgentOrchestrator(agentinit.GetAgentByID)`。

**为什么选声明式 spec 注册，不选 factory-func map**：factory-func map（`map[string]func(deps) Agent`）虽也去了 switch，但每个 preset 仍要写一段命令式构造代码，工具装配逻辑分散在 N 个 factory 里；声明式 spec 把"agent = 工具名列表 + prompt + 能力"降为数据，装配逻辑集中在注册表一处，新增 agent 只加一条数据、能力/工具可被外部（UI/配置）读取与校验。

**备选方案**：
- (A) 保留 switch：新增 agent 改多处，本 change 要消灭的正是它。否决。
- (B) factory-func map：见上，装配分散。作为过渡可接受，但非终态。
- (C) 声明式 `AgentSpec` 注册表：数据驱动、可反射能力、可校验工具名存在。选定。

### 3. eino_agent 用策略/组合拆解

**决策**：把 1199 行的 `agentImpl` 拆为单一执行主干 + 三个可插拔组件：
- `MemoryStrategy`（有/无跨轮记忆的上下文构建，吸收 `chatWithMemory*` 的分叉）；
- `ToolLoop`（有/无工具的执行循环，吸收 `*WithTools` / `*WithoutTools` 的分叉）；
- `StreamSink`（流式 vs 非流式输出，吸收 `chatWith*` vs `streamWith*` 的分叉）。
主干只保留一条 `run(ctx, req)`，三个正交维度由组合而非笛卡尔积展开的独立方法承载，消除 7 个变体。

**为什么选正交组合，不选模板方法继承**：Go 无继承，模板方法要靠嵌入 + 覆写，语义隐晦；当前 7 个变体正是三个正交维度（memory×tool×stream）被写成 2³ 近似展开的结果，用策略接口把每个维度独立出来，组合数从"变体数"降为"维度数之和"，加一个维度不再乘。

**备选方案**：
- (A) 保留变体、只抽公共私有函数去重：治标，维度一多仍爆炸。否决。
- (B) 一个巨型 `chat(opts Options)` 带 bool 开关：所有分支挤在一个函数里，圈复杂度不降反升。否决。
- (C) 策略接口 + 组合主干：维度可独立测试、可独立替换。选定。

### 4. proto 单一源 + buf generate

**决策**：建顶层 `proto/`（与 `cognida-go`/`cognida-python` 同级）为唯一 source-of-truth，放 `analytics/docreader/evaluation/judge/quality` 的 `.proto`。引入 `buf.yaml` + `buf.gen.yaml`，`buf generate` 同产 Go stub（输出到 `cognida-go` 生成目录）与 Python stub（输出到 `cognida-python` 生成目录）。删除 `cognida-go/api/proto` 与 `cognida-python/proto` 的手抄 `.proto` 及手工生成物。`make proto` 封装生成，CI 跑 `buf generate` 后 `git diff --exit-code` 校验生成物与提交一致。跨服务统一错误码定义为 proto `enum ErrorCode`，Go/Python 共用。

**为什么选 buf，不选 protoc 脚本或各自维护**：`buf` 提供 lint/breaking-change 检测/远程插件/可复现生成，比手写 protoc 调用链稳定；相比"各自维护 + 约定同步"，物理单一源从根上杜绝 drift。

**备选方案**：
- (A) 保留双份 + CI diff 比对两份 `.proto` 文本：只能事后发现 drift，不能杜绝。否决。
- (B) 裸 protoc + Makefile：可行但缺 lint/breaking 检测，插件版本漂移难复现。否决。
- (C) buf + buf.gen.yaml + CI 一致性门：可复现、能防 breaking。选定。

### 5. Python 评测编排收口

**决策**：删除 `services/evaluation/runner.py`（`ProgressStage` 状态机 + `EvaluationRunner.run`）与 `grpc_service` 中 `ExecuteEvaluation` servicer 实现，移除 proto `EvaluationService.ExecuteEvaluation` RPC。Python 评测收敛为无状态 `compute_*(inputs) -> metrics` 纯函数集 + FastAPI 薄壳（:18888），进度/状态/编排全部由 Go worker 承担；Go 经既有通道（gRPC 计算 RPC 或 HTTP:18888 compute）调 Python 只拿指标，不再拿"进度流"。

**为什么删而非保留双引擎**：两套编排对同一职责各自持有状态，是"改一处炸三处"的典型；Go worker 已是权威（任务状态入 MySQL、有 request_id 链路），Python 侧状态机是历史越界。保留即持续维护两份进度语义。

**备选方案**：
- (A) 保留 `ExecuteEvaluation` 标记 deprecated：死代码继续腐坏，调用方仍可能误用。否决。
- (B) 反向让 Python 成权威：违背"Python 只做计算、Go 承主后端"的既定分工。否决。
- (C) 删 Python 编排、收敛为无状态 compute：单一权威、Python 无状态易测。选定。

### 6. 统一 AgentState 门面收敛 6+ 处会话态

**决策**：引入 `AgentState` 领域门面（明确 `New/Load → mutate → Persist/Expire` 生命周期），把 `memory` / `framework/memory_registry` / `convcontext` / `context/window` / `resultstore` / `pendingaction` / `uibinding` / `semanticcache` 的会话态收敛到其下的具名子域。明确两条路径边界：
- **`chat.session`（UI 会话 write-path）**：用户可见的会话记录、消息、UI 绑定，随请求写；
- **`conversation.memory`（跨轮记忆 read-path）**：跨轮上下文构建的只读投影，供 agent 执行时读取。
`AgentState` 是门面/聚合入口，不吞掉各子域实现，只统一生命周期与访问入口，消除"谁在什么时候读写哪块态"的散乱。

**为什么选门面聚合，不选大一统单体**：把 8 个包合并成一个大结构会制造新的上帝对象；门面只统一生命周期语义和入口，子域各自内聚，既收敛"散在 6 处"的认知负担，又不牺牲模块边界。write-path/read-path 分离是为了防止 UI 会话写入污染跨轮记忆的只读投影。

**备选方案**：
- (A) 现状（各包各自管生命周期）：认知负担高、易漏持久化/过期。否决。
- (B) 合并为单个 `SessionState` 巨结构：新上帝对象。否决。
- (C) `AgentState` 门面 + 具名子域 + read/write 路径分离：收敛入口、保留内聚。选定。

### 7. 跨服务通信选型统一

**决策**：定 gRPC 为"Go 调 Python"主通道；evaluation 的 HTTP:18888 保留但在文档明确"HTTP 仅用于无状态 compute 薄壳/健康检查"，不承载编排。统一错误码走 proto `enum ErrorCode`（决策 4）。

**为什么不强行把 HTTP:18888 并入 gRPC**：评测 compute 薄壳走 FastAPI:18888 已在生产链路（`PYTHON_EVALUATION_ENDPOINT`），强并入 gRPC 是纯搬运、收益低风险高；用文档规则约束其用途即可满足"选型统一"的目标。

## Risks / Trade-offs

- [风险] 去全局化触及 `tools/*`、`initializer`、`presets/*`、`cmd/wire` 大面积改，一次性切换易编译长期红。 → 缓解：按包分阶段（先 `ToolRegistry` 再 `AgentRegistry` 再 wire 收口），每阶段 `go build ./...` 绿灯才推进；wire 生成物每步 `go run ./cmd/wire` 重生成。
- [风险] 删 `SetXxx` setter 破坏大量测试脚手架（`graph_query_gating_test.go`、`operation_tools_integration_test.go` 等）。 → 缓解：改造为构造注入的表驱动测试，测试直接 `NewToolRegistry(mockDeps)`；删除 setter 与对应 `Set/Reset` 同 PR，避免悬空引用。
- [风险] proto 目录搬迁改 import 路径（BREAKING），Go 与 Python 生成物路径同时变。 → 缓解：先落 `proto/` + buf 生成 + 新路径可编译，再一次性切换 import 并删旧目录；CI 一致性门防遗漏。
- [风险] 移除 `ExecuteEvaluation` 是对外 gRPC BREAKING，可能有未知调用方。 → 缓解：REMOVED 需求写明 Reason/Migration（改调 Go worker 触发的评测入口）；先在 README/proto 标注移除，验证无 Go 侧调用（`grep ExecuteEvaluation cognida-go`）后再删。
- [风险] eino_agent 拆解改执行主干，可能引入流式/工具循环行为回归。 → 缓解：拆解前先补主干行为的表驱动测试（memory×tool×stream 组合矩阵）作为回归基线，拆解后逐组合比对。
- [权衡] AgentState 门面增加一层间接。 → 接受：换取会话态生命周期单点可推理，且门面不吞子域实现，间接成本可控。
- [权衡] 引入 buf 工具链增加构建依赖。 → 接受：换取 proto 单一源与可复现生成，长期省下 drift 排查成本。

## Migration Plan

分五阶段落地，阶段间以 `go build ./...` + 相关 `go test` 绿灯为闸门：

- **阶段 1 — Tools 去全局化**：`NewToolRegistry(deps)` 落地 → 删 `GlobalRegistry`/`func init()`/`SetXxx`/`var ragService|opConfig` → wire 装配 → 改测试为构造注入。回滚：本阶段独立分支，未合入前 `git revert` 即可。
- **阶段 2 — Agent 注册表化**：`AgentRegistry` + `AgentSpec` 声明式注册 → `GetAgentByID` 改注册表查询 → `cmd/wire` orchestrator 切到 `registry.Get`。回滚：注册表与旧 `GetAgentByID` 可短暂共存，切换点单一，回退只改 wire 一行。
- **阶段 3 — eino_agent 拆解**：先补组合矩阵回归测试 → 抽 `MemoryStrategy`/`ToolLoop`/`StreamSink` → 收敛主干、删 7 变体。回滚：拆解在 framework 包内，行为由回归测试守门，异常时保留旧变体一版对照。
- **阶段 4 — proto 单一源**：建 `proto/` + buf 配置 → `buf generate` 新生成物可编译 → 切 import、删双份、加 CI 一致性门。回滚：旧 `api/proto` 与 `cognida-python/proto` 删除前打 tag，切换失败可恢复。
- **阶段 5 — 评测编排收口 + AgentState 门面 + 通信文档**：删 `runner.py`/`ExecuteEvaluation`、proto 移除 RPC → Python 收敛无状态 compute → 引入 `AgentState` 门面收敛会话态 → 补跨服务通信规则文档。回滚：`ExecuteEvaluation` 移除前确认 Go 无调用；门面引入不删子域实现，异常可暂缓门面只保留子域。

全局回滚策略：每阶段合入 main 前独立可验证，出问题按阶段 `git revert`；proto 与对外 gRPC 的 BREAKING 变更（阶段 4/5）在移除前打 tag 并 grep 验证无残留调用。

## Open Questions

- proto 生成物应提交入库（vendored）还是构建时生成？倾向提交 + CI diff 校验（离线可编译），待与 CI 约束确认。
- Python compute 是走 gRPC 计算 RPC 还是保留 HTTP:18888 薄壳为主？决策 7 倾向 HTTP compute 保留，但若统一错误码走 proto，是否值得把 compute 也 gRPC 化以复用 enum？待评估。
- `AgentState` 门面的持久化边界：`semanticcache`/`resultstore` 是否纳入统一生命周期，还是仅纳入访问入口而各自管过期？待定各子域 TTL 语义。
- `ErrorCode` proto enum 的取值范围是否需要覆盖 HTTP:18888 通道，还是仅 gRPC 面？待与错误码契约范围确认。
