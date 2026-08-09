# 架构加固：去全局化 / 契约单一源 / 编排收口 - Tasks

> **目标**：消灭 agent 边界全局态、收口跨服务契约与评测编排、拆解 eino_agent 上帝对象、统一会话态门面。
> 阶段间以 `go build ./...` + 相关 `go test` 绿灯为闸门，逐阶段可回滚。

---

## 0. 准备工作

- [x] 0.1 统计受影响文件：`grep -rl "GlobalRegistry\|SetRAGService\|SetGraphService\|opConfig\|GetAgentByID" cognida-go/internal`
- [x] 0.2 记录 eino_agent 变体基线：原 6 变体 `chatWithMemoryAndTools`/`chatWithMemoryOnly`/`chatWithTools`/`chatWithoutTools`/`streamWithTools`/`streamWithoutTools` + `chatWithMemory`/`streamInternal` 编排器（Phase 3 已全部收敛为单一 `run`）
- [x] 0.3 确认 `ExecuteEvaluation` 无 Go 侧调用：`grep -rn ExecuteEvaluation cognida-go`（非生成代码零引用，Go 全走 HTTP :18888；据此判定为死契约，支撑 Phase 5 删活编排 + 5.1 推迟到 Phase 4）
- [x] 0.4 备份 `cognida-go/api/proto` 与 `cognida-python/proto`，为阶段 4 打 tag 做准备（两处生成物+手抄 `.proto` 在 HEAD 均 clean，`git tag -a pre-proto-single-source`(96e08e0) 即完整恢复点，覆盖回生成与 4.7 删除前全部状态）

---

## Phase 1: Agent/Tools 去全局化

- [x] 1.1 定义 `ToolDeps` 结构，聚合 RAG/Graph/SQL/审计等工具依赖
- [x] 1.2 实现 `NewToolRegistry(deps ToolDeps) (*ToolRegistry, error)`，构造期校验必需依赖
- [x] 1.3 把 `init.go` 的 `registerRAGTools/registerSQLTools/.../registerSkillTools` 改为注册表实例方法，依赖从 deps 取
- [x] 1.4 删除 `service/agent/tools/init.go` 的 `func init()` 副作用注册
- [x] 1.5 删除 `service/agent/tools/registry.go` 的 `var GlobalRegistry` 单例与其包级转发函数
- [x] 1.6 删除 `rag_query.go` 的 `var ragService` + `SetRAGService`，改为字段注入
- [x] 1.7 删除 `graph_query.go` 的 `SetGraphService`，改为字段注入
- [x] 1.8 删除 `etl_run.go`/operation 工具的包级 `opConfig`，改为构造传入
- [x] 1.9 排查并删除其余 `SetXxx`/`var xxxService`/`dsProvider` 等 agent 边界全局
- [x] 1.10 改造工具测试：`graph_query_gating_test.go`、`operation_tools_integration_test.go` 等去 `Set/Reset` 脚手架，改 `NewToolRegistry(mockDeps)` 表驱动
- [x] 1.11 `go build ./...` + `go test ./internal/service/agent/tools/...`

## Phase 2: Agent 注册表驱动

- [x] 2.1 定义 `AgentSpec{ID, Name, ToolNames []string, Prompt, Capabilities}` 声明式描述
- [x] 2.2 实现 `AgentRegistry`（`Register(spec)` / `Get(id) (Agent, bool)` / `List()`），按 `ToolNames` 用 `ToolRegistry` 装配 agent
- [x] 2.3 实现 `NewAgentRegistry(deps)`，删除 `var *AgentInstance` 全局单例
- [x] 2.4 各 preset（`presets/*`）改为提供 `AgentSpec` 声明式注册，去命令式装配
- [x] 2.5 `initializer/init.go` 的 `GetAgentByID` 改为向 `AgentRegistry` 查询
- [x] 2.6 校验 `GetAgentByID` 内无硬编码 switch 分支
- [x] 2.7 wire：AgentRegistry provider + orchestrator 已切 `NewRegistryAgentOrchestrator(registry.Get)`；`ToolRegistry` 按 Decision A 不入 wire 图——由组合根（cmd/server）构造后经 `App.AgentHandler.SetToolGateway` + `NewInitializer(registry, reg, ...)` 显式注入（default.go 桥接已删除）
- [x] 2.8 wire 同步：`App` 增 `AgentHandler` 字段 + `ProvideApp` 形参，wire.go/wire_gen.go 手工同步并对齐（wire CLI 因 x/tools 版本 < go1.25 暂无法重生成，一致性由 `go build`/`go vet` 编译校验保证）
- [x] 2.9 `go build ./...` 通过 + `go vet -tags=integration` 通过 + `go test ./internal/service/agent/... ./internal/handler/...` 全绿；Gate 7.4 全局单例扫描为空

## Phase 3: 拆解 eino_agent 上帝对象

- [x] 3.1 先补组合矩阵回归测试：memory×tool×streaming 八组合经公开 Chat/Stream 执行，锚定「愈合后」一致行为（`eino_matrix_test.go`）
- [x] 3.2 抽 `MemoryStrategy`：`buildInitialMessages`（读入口，统一走 collab-aware 构建）+ `persistResult`（写出口，落库+摘要）
- [x] 3.3 抽 `ToolLoop`：`execLoop`（ReAct，token 预算前/后置检查 + wind-down）+ `handleToolCall`（参数解析愈合，tool_call↔tool 1:1）
- [x] 3.4 抽 `StreamSink`：`execSink` 接口 + `bufferedSink`（累积 *Response，收尾跑 middleware.After/afterHooks）/ `streamSink`（即时下发 *Chunk）
- [x] 3.5 收敛为单一执行主干 `run(ctx, req, sink)`，组合三组件；`runStream` 包一层 defer close
- [x] 3.6 删除 6 变体 + `chatWithMemory`/`streamInternal` 编排器；旧测试改接（react→`Chat`；eof→`streamSink`+`run`；cancel→`runStream`）
- [x] 3.7 组合矩阵逐组合比对：8 组合 + react/eof/cancel/memory/middleware 回归全绿
- [x] 3.8 `go build ./...` + `go test ./internal/service/agent/framework/...` 通过（含 `go vet -tags=integration`）

## Phase 4: proto 单一源 + 代码生成

- [x] 4.1 建顶层 `proto/`，迁入 `analytics/docreader/evaluation/judge/quality` 的 `.proto`（6 个 flat `.proto`：5 服务 + 新增 `common.proto`；`analytics` 的 `go_package` 由分号形式 `link/api/proto;analytics` 归一为斜杠形式 `link/api/proto/analytics`，因其 Go 侧零消费故改路径安全）
- [x] 4.2 在 proto 契约中定义跨服务 `enum ErrorCode`（`proto/common.proto`，14 值：通用 0–9 段 + 领域 100+ 段；契约先行——存量 string code 迁移作后续增量，不在本阶段一次性重写以控回归面）
- [x] 4.3 编写 `buf.yaml` + `buf.gen.yaml`（Go 与 Python 两组输出 + lint/breaking 规则）：**关键决策**——`buf.yaml` module root 设在**仓库根**(`path: .`)而非 `proto/`，使 `.proto` 被寻址为 `proto/<svc>.proto`，Python stub 内部 import 保持 `from proto import <svc>_pb2`、序列化描述符文件名保持 `proto/<svc>.proto`（消费方零改动）；`excludes: [cognida-go, cognida-python, cognida-web]` 排除待删手抄双份避免同包冲突；lint 用 MINIMAL 并 except `PACKAGE_DIRECTORY_MATCH`+`DIRECTORY_SAME_PACKAGE`（扁平单一源刻意多包共处一目录）；breaking 用 FILE。`buf lint` clean、`buf build` OK
- [x] 4.4 新增 `make proto` 目标封装 `buf generate`（`cognida-go/Makefile`：新增 `proto`(lint+generate)、`proto-check`(一致性门) 两 target；旧 `grpc-gen` 委托给 `proto`，原 protoc 直连手抄源已废弃）
- [x] 4.5 `buf generate` 生成 Go/Python stub，确认新生成物可编译（四插件 pin 到现有产物同版本：Go `protoc-gen-go v1.36.11`+`grpc/go v1.6.1`、Python `protocolbuffers/python v27.2`(gencode 5.27.2，已知兼容运行时 protobuf 6.33.6)+`grpc/python v1.66.1`≤运行时 grpcio 1.81.1；避免 latest 漂移致 gencode 高于运行时而 import 失败）
- [x] 4.6 切换 cognida-go 与 cognida-python 的 import 到新生成物路径 —— **偏差（Strategy A，最小改动就地回生成）**：buf 按 `module=link`(Go 用 `go_package` 去前缀)与仓库根 module root(Python 保 `proto/` 寻址)将 stub 回落到 `cognida-go/api/proto/<svc>/` 与 `cognida-python/proto/` **既有路径**，故消费方 import **零改动**（验证：Go docreader(1)+quality(4) 消费点、Python `from proto import` 全部未变，`git diff` 消费方无 proto 相关改动）；相比原设想的「搬到新路径再改 import」，就地回生成回归面更小
- [x] 4.7 删除 `cognida-go/api/proto` 与 `cognida-python/proto` 的手抄 `.proto` 及旧生成物（删除前打 tag）—— 打 tag `pre-proto-single-source` 后 `git rm` 两处 10 个手抄 `.proto` 源；**注**：生成物(`.pb.go`/`_pb2.py`)是就地回生成的**唯一权威输出**，保留不删（原任务假设生成物搬走，Strategy A 下不适用），仅删多余 `.proto` 源。删后 `make proto` 端到端仍成功，单一源仅剩 `proto/` 6 文件
- [x] 4.8 CI 加一致性门：`buf generate` 后 `git diff --exit-code`（`make proto-check` target；另证幂等：快照→二次 generate→checksum 逐字节一致 `d4467733...`）
- [x] 4.9 `go build ./...`（rc=0）；Python `import proto.<svc>_pb2/_pb2_grpc` 全 12/12 OK；消费方单测 docreader `ok`、quality 编译通过

## Phase 5: Python 评测编排收口 + 通信收口

- [x] 5.1 从 proto 移除 `EvaluationService.ExecuteEvaluation` RPC 与流式响应消息 —— 已随 Phase 4 单一源完成：`proto/evaluation.proto` 删 `ExecuteEvaluation` RPC + 12 个独占消息树（EvaluationRequest/Config/GraderConfig/Response/Progress/Result/RetrievalMetrics/GenerationMetrics/SemanticMetrics/LLMJudgeMetrics/QAResult/Error），仅保留 3 个无状态查询 RPC（ListGraders/ListDatasets/GetDatasetInfo）；`buf generate` 重生成 Go/Python stub，`grep evaluationpb./evaluation_pb2.` 确认零活消费；Python import 校验 `ExecuteEvaluation gone: True`（原推迟说明见 git 历史）
- [x] 5.2 删除 `cognida-python/services/evaluation/runner.py`（`ProgressStage` + `EvaluationRunner.run`）
- [x] 5.3 删除 `grpc_service` 中 `ExecuteEvaluation` servicer 实现（删 `service.py` + servicer.py 移除 import/`get_evaluation_servicer`/注册块）
- [x] 5.4 Python 评测收敛为无状态 `compute_*` + FastAPI 薄壳（:18888）——`fastapi_app.py` 已满足（无编排/状态/进度），`__init__.py` 去 runner/service 导出
- [x] 5.5 确认 Go worker 为唯一权威编排，Go 调 Python 仅取指标结果（`python_client.go` 为 HTTP-only `ComputeMetrics`，非生成代码零 `ExecuteEvaluation` 引用）
- [x] 5.6 更新 `cognida-python/README.md`：移除 `ExecuteEvaluation` 文档，明确无状态 compute（同步更新 `docs/evaluation.md`）
- [x] 5.7 补跨服务通信规则文档：gRPC 为主通道，HTTP:18888 仅用于无状态 compute/健康检查（新增 `docs/cross-service-communication.md`）
- [x] 5.8 `cd cognida-python && pytest tests/ -v`（评测子集 34 passed 全绿；全库 385 passed，仅 3 个 analytics gRPC 集成测试因未起 :50054 活服务而 Connection refused，与本次改动无关）

## Phase 6: 统一 AgentState 门面

- [x] 6.1 定义 `AgentState` 门面（`agentstate/state.go`，**薄门面**决策 C）：生命周期入口 `New`/`Load` → 经具名子域 mutate → `Persist`/`Expire`；`Owner{tenant,session}` 统一归属键（复用 `resultstore.OwnerKey`），`Stores` 聚合子域后端由组合根一次装配注入，不吞子域实现（各子域保留自己的后端与 TTL）
- [x] 6.2 收敛 per-session 会话态到门面读入口 —— **偏离原文并修订（用户 2026-07-05 决策）**：原列的 `framework/memory_registry`（agent 定义元信息注册中心，与会话态正交）与 `context/{window,layers,budget}`（无状态纯函数计算，无生命周期可收敛）**均非 per-session 会话态，明确排除**；门面只纳入真正的会话态——`memory`（读写编排，经 `SessionMemory` 最小接口结构化满足）+ `convcontext`（跨轮记忆只读投影，读路径）+ 6.3 的 4 个 store。排除理由已写入 `agentstate` 包 doc
- [x] 6.3 收敛 `resultstore`/`pendingaction`/`uibinding`/`semanticcache` 到门面读写入口（保留各子域内聚实现与各自 TTL）：`AgentState.Results/Pending/UI/Cache` 具名访问器；`ToolRegistry.SessionState(tenant,session)` 工厂装配门面；**顺带消灭 `uibinding` 包级单例** `SetStore`/`GetStore`/`defaultStore`——改经 `ToolDeps.UIBinding` 显式注入，`render_ui` 与 handler 回调路由从注入实例读取（对齐 Phase 1 去全局化 + gate 7.4）
- [x] 6.4 界定 `chat.session`（UI 写路径：`service/chat` + `model/conversation` 落 sessions/messages）与 `conversation.memory`（跨轮读投影：`convcontext.Build` 回放 messages）边界：以 messages 表为单一数据源、以 convcontext 只读契约为分界，写路径落库、读路径回放，读投影不反向改写会话写入状态。边界规则写入 `agentstate` 包 doc + `convcontext`/`chat` 两侧包 doc 互指
- [x] 6.5 会话态持久化经门面 `Persist` 收口（薄门面下各子域 Put/写穿透即落库，`Persist` 为语义锚点，标记「写路径在此收口」）、过期语义归门面 `Expire`（TTL 型子域自然回收，跨轮记忆若接线则经 `Memory.ClearSession` 显式清理防无界增长）
- [x] 6.6 `go build ./...`（OK）+ `go test ./internal/service/agent/... ./internal/handler/... ./internal/service/chat/...`（全绿；`go vet` 亦通过）

---

## 7. 验证与验收

- [x] 7.1 `cd cognida-go && go build ./...`（BUILD OK）
- [x] 7.2 `cd cognida-go && go test ./...`（全绿，仅 go-m1cpu cgo 折叠警告，无测试失败）
- [x] 7.3 `cd cognida-go && go vet ./...`（clean）
- [x] 7.4 校验无 agent 边界全局：`grep -rn "GlobalRegistry\|func init()\|SetRAGService\|SetGraphService\|var ragService\|var opConfig" cognida-go/internal/service/agent` 应为空（EMPTY OK；uibinding 包级单例已于 Phase 6 一并清零）
- [x] 7.5 校验无 eino_agent 变体：`grep -n "chatWithMemoryAndTools\|chatWithoutTools\|streamWithoutTools" cognida-go/internal/service/agent/framework/eino_agent.go` 应为空（无变体函数定义；仅第 183–184 行注释描述"收敛旧 6 变体"，非实现）
- [x] 7.6 校验 proto 单一源：`cognida-go/api/proto` 与 `cognida-python/proto` 无手抄 `.proto`（已 `git rm`，`find *.proto` 仅剩 `proto/` 6 文件）；`buf generate` 幂等——快照→二次 generate→checksum 逐字节一致（`d4467733...`），`make proto-check` 一致性门就位（buf 1.71.0）
- [x] 7.7 校验编排收口：`grep -rn ExecuteEvaluation cognida-go cognida-python`（除 archive/文档外）应为空；无 `runner.py`（go+python 两侧均 EMPTY OK；`runner.py` 已删。注：proto 内 `ExecuteEvaluation` RPC 声明为死契约，5.1 已推迟到 Phase 4 从单一源删除）
- [x] 7.8 `cd cognida-python && pytest tests/ -v`（385 passed；3 个 analytics gRPC 集成测试因未起 :50054 活服务而 `UNAVAILABLE: Connection refused`，属预存环境依赖，与本次改动无关）
- [x] 7.9 触发 code-review skill，修复问题
- [x] 7.10 `openspec validate architecture-hardening` 通过（Change 'architecture-hardening' is valid）
