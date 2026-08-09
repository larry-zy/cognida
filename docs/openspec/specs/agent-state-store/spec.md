# agent-state-store Specification

## Purpose
TBD - created by archiving change architecture-hardening. Update Purpose after archive.
## Requirements
### Requirement: 统一 AgentState 会话态门面

系统 SHALL 提供 `AgentState` 领域门面作为会话态的统一访问入口，收敛 `memory`、`framework/memory_registry`、`convcontext`、`context/window`、`resultstore`、`pendingaction`、`uibinding`、`semanticcache` 各处会话态。门面 SHALL 提供明确的生命周期入口，MUST NOT 让各子域各自暴露不受约束的独立读写入口。

#### Scenario: 会话态经门面访问

- **WHEN** agent 执行需要读写会话态（记忆/结果集/待确认动作/UI 绑定等）
- **THEN** 访问 SHALL 经 `AgentState` 门面入口进行
- **AND** 门面 SHALL 路由到对应具名子域，MUST NOT 要求调用方直接跨越 6+ 个独立包各自装配

### Requirement: AgentState 明确生命周期语义

`AgentState` SHALL 具备明确的生命周期语义：创建/加载（`New`/`Load`）、变更（mutate）、持久化/过期（`Persist`/`Expire`）。会话态的持久化与过期 SHALL 在门面生命周期内可推理，MUST NOT 散落到各子域各自为政的隐式时机。

#### Scenario: 生命周期入口完整

- **WHEN** 检查 `AgentState` 门面
- **THEN** 门面 SHALL 提供加载态、变更态、持久化态的显式入口
- **AND** 会话态的过期/失效 SHALL 有明确的生命周期归属

#### Scenario: 持久化经门面收口

- **WHEN** 一次 agent 请求结束需要落库会话态
- **THEN** 持久化 SHALL 经门面的 `Persist` 语义触发
- **AND** MUST NOT 依赖调用方逐个记住向每个子域单独落库

### Requirement: 界定 chat.session 写路径与 conversation.memory 读路径边界

系统 SHALL 明确区分 `chat.session`（UI 会话 write-path，用户可见会话/消息/UI 绑定）与 `conversation.memory`（跨轮记忆 read-path，供 agent 执行读取的只读投影）两条路径。跨轮记忆的读路径 MUST NOT 被 UI 会话写路径直接污染。

#### Scenario: UI 会话走写路径

- **WHEN** 处理用户可见的会话记录/消息/UI 绑定
- **THEN** 数据 SHALL 经 `chat.session` 写路径写入
- **AND** 该写入 MUST NOT 直接改写 `conversation.memory` 的只读投影

#### Scenario: 跨轮记忆走读路径

- **WHEN** agent 执行时构建跨轮上下文
- **THEN** 上下文 SHALL 从 `conversation.memory` 读路径读取只读投影
- **AND** 读路径 SHALL 与 UI 会话写路径边界清晰、职责不混

