## ADDED Requirements

### Requirement: 类型化 LLM 错误分级

所有 LLM provider 客户端（chat / embedding / rerank）在执行失败时 SHALL 返回一个类型化的 `APIError`，携带 `Provider`、`Model`、`StatusCode`（传输层错误为 0）与结构化 `error_class`，MUST NOT 仅返回裸底层错误字符串。`error_class` 枚举 SHALL 至少覆盖：`rate_limited`、`transient`、`terminal`、`canceled`。分级 SHALL 由统一的分类器裁决，各客户端 MUST NOT 各自实现不一致的分级逻辑。`APIError` SHALL 可经 `errors.Unwrap` 回溯到底层错误。

#### Scenario: 限流错误被归类为 rate_limited

- **WHEN** provider 返回 HTTP 429
- **THEN** 错误 SHALL 为 `APIError` 且 `error_class = "rate_limited"`
- **AND** 若响应含 `Retry-After` 头，SHALL 解析为 `RetryAfter` 值（封顶后）

#### Scenario: 服务端错误被归类为 transient

- **WHEN** provider 返回 HTTP 500/502/503/504，或发生连接重置/拒绝/EOF/可重试超时
- **THEN** 错误 SHALL 为 `APIError` 且 `error_class = "transient"`

#### Scenario: 鉴权/参数错误被归类为 terminal

- **WHEN** provider 返回 HTTP 400/401/403/404/422
- **THEN** 错误 SHALL 为 `APIError` 且 `error_class = "terminal"`

#### Scenario: 调用方取消被归类为 canceled

- **WHEN** 调用方传入的 ctx 被取消或其 deadline 到期
- **THEN** 错误 SHALL 为 `error_class = "canceled"`
- **AND** MUST NOT 被误判为 transient

#### Scenario: 未识别错误保守归入 terminal

- **WHEN** 底层错误无法匹配任何已知分类
- **THEN** `error_class` SHALL 归为 `terminal`（不确定不重试）
- **AND** SHALL 保留一段脱敏后的原始错误摘要，MUST NOT 泄露 api_key/host

### Requirement: 请求级退避重试

弹性客户端 SHALL 对 `transient` 与 `rate_limited` 错误在**同一目标**上以有限次数（`maxAttempts`）做指数退避 + 抖动重试；对 `terminal` 与 `canceled` 错误 SHALL 立即失败，MUST NOT 重试。重试 SHALL 全程尊重调用方 ctx 的 deadline 与 cancel。`rate_limited` 且存在 `Retry-After` 时 SHALL 优先采用其等待时长，并 SHALL 对该时长封顶（`retryAfterCap`）以防超长/恶意等待。

#### Scenario: 瞬时错重试后成功

- **WHEN** 首次调用因 `transient` 失败、退避后重试成功
- **THEN** 弹性客户端 SHALL 返回成功响应
- **AND** 调用方 SHALL NOT 观察到中间失败

#### Scenario: 终态错不重试

- **WHEN** 调用因 `terminal`（如 401 鉴权失败）失败
- **THEN** 弹性客户端 SHALL 立即返回该错误
- **AND** MUST NOT 发起任何重试

#### Scenario: 遵循 Retry-After

- **WHEN** provider 返回 429 且 `Retry-After: 2`
- **THEN** 下次重试前的等待 SHALL 不小于 2 秒（除非超过 `retryAfterCap` 被封顶）

#### Scenario: 重试尊重调用方取消

- **WHEN** 调用方 ctx 在重试等待期间被取消
- **THEN** 弹性客户端 SHALL 立即返回 `canceled`
- **AND** MUST NOT 继续后续重试或降级

#### Scenario: 重试次数有上限

- **WHEN** 某目标持续返回 `transient`
- **THEN** 重试次数 SHALL 不超过 `maxAttempts`
- **AND** 耗尽后 SHALL 进入降级流程（若有 fallback）或返回终态错

### Requirement: 模型降级与故障转移链

弹性客户端 SHALL 支持为一次逻辑调用配置**有序的 fallback 链**（主目标 + 若干备用目标）。当主目标重试耗尽、或其熔断器处于 `open` 时，客户端 SHALL 自动前进到链中下一个目标并重置该目标的重试预算。仅当整条链全部耗尽时，客户端才 SHALL 返回终态错，且该错误 SHALL 聚合各目标的最后失败摘要（脱敏）。链的顺序 SHALL 稳定：先穷尽单目标的重试，再切换目标。

#### Scenario: 主目标失败后降级到备用

- **WHEN** 主目标在重试耗尽后仍 `transient` 失败，且链中存在备用目标
- **THEN** 弹性客户端 SHALL 用下一个目标重新尝试
- **AND** 若备用目标成功，SHALL 返回其成功响应

#### Scenario: 全链耗尽返回聚合错误

- **WHEN** 链中所有目标均失败
- **THEN** 弹性客户端 SHALL 返回一个终态 `APIError`
- **AND** 其 `Detail` SHALL 聚合各目标的最后失败原因
- **AND** MUST NOT 泄露 api_key/host

#### Scenario: 无备用目标时退化为单目标弹性

- **WHEN** fallback 链只含一个目标
- **THEN** 客户端 SHALL 只在该目标上做重试与熔断
- **AND** 行为 SHALL 与"仅重试+熔断"等价（无降级跳转）

#### Scenario: 熔断打开时直接降级

- **WHEN** 主目标熔断器处于 `open`
- **THEN** 客户端 SHALL 跳过对主目标的网络请求，直接尝试下一个目标
- **AND** MUST NOT 对 `open` 的目标发起调用

### Requirement: Per-target 熔断器

弹性客户端 SHALL 为每个 `provider+model` 目标维护一个熔断器，具备 `closed`/`open`/`half-open` 三态。连续失败达到 `failThreshold` 时 SHALL 转为 `open` 并在 `cooldown` 窗口内对该目标快速失败（不发请求）；`cooldown` 后 SHALL 转 `half-open` 并只放行有限个探测请求；探测成功 SHALL 转回 `closed`、失败 SHALL 转回 `open`。`terminal` 类错误 MUST NOT 计入熔断失败计数（配置类错误不代表目标不健康）。熔断器状态 SHALL 在同一 `provider+model` 目标的所有客户端实例间进程内共享。

#### Scenario: 连续失败触发熔断打开

- **WHEN** 某目标连续失败次数达到 `failThreshold`
- **THEN** 其熔断器 SHALL 转为 `open`
- **AND** `cooldown` 窗口内对该目标的调用 SHALL 快速失败且不发起网络请求

#### Scenario: 冷却后半开探测

- **WHEN** 熔断器 `open` 且 `cooldown` 已过
- **THEN** 熔断器 SHALL 转为 `half-open`
- **AND** SHALL 只放行有限个探测请求，其余调用直接降级

#### Scenario: 探测成功恢复闭合

- **WHEN** `half-open` 探测请求成功
- **THEN** 熔断器 SHALL 转回 `closed`
- **AND** 失败计数 SHALL 归零

#### Scenario: 终态错不触发熔断

- **WHEN** 某目标反复返回 `terminal`（如 401）
- **THEN** 该目标的熔断器 MUST NOT 因此转为 `open`

#### Scenario: 熔断状态跨实例共享

- **WHEN** 同一 `provider+model` 目标存在多个客户端实例
- **THEN** 它们 SHALL 共享同一熔断器状态

### Requirement: 流式失败转移边界

对 `ChatStream` 流式调用，弹性客户端的重试与降级 SHALL 仅在**首个 chunk 到达调用方之前**生效。一旦已向调用方转发首个 chunk，之后发生的底层错误 SHALL 作为流的终止错误透传给调用方，MUST NOT 静默切换目标重放已消费的流。首个 chunk 之后的失败 MUST NOT 计入熔断器失败计数。

#### Scenario: 首 chunk 前失败可转移

- **WHEN** 底层 `ChatStream` 在产出任何 chunk 之前失败
- **THEN** 弹性客户端 SHALL 按重试+降级逻辑处理
- **AND** 调用方 SHALL NOT 观察到这些前置失败

#### Scenario: 首 chunk 后失败透传不重放

- **WHEN** 已向调用方转发至少一个 chunk 后底层出错
- **THEN** 弹性客户端 SHALL 把错误作为流终止透传给调用方
- **AND** MUST NOT 切换目标重放该流
- **AND** 该失败 MUST NOT 计入熔断计数

### Requirement: 装饰透明性与可关闭

弹性能力 SHALL 以装饰器形式实现，返回类型仍为既有领域接口（`LLMClient`/`EmbeddingRepository`/`RerankRepository`），对所有上层调用方透明，MUST NOT 要求调用方改动。成功路径下 SHALL 原样透传底层响应，信封与未装饰时一致。弹性能力 SHALL 可整体关闭（opt-out），关闭时退化为直连底层客户端且无额外行为。

#### Scenario: 成功路径信封不变

- **WHEN** 底层调用一次成功
- **THEN** 弹性客户端 SHALL 原样返回底层响应
- **AND** 响应信封 SHALL 与未装饰时逐字段一致

#### Scenario: 可整体关闭

- **WHEN** 弹性配置 `Enabled=false`
- **THEN** 客户端 SHALL 直连底层，不做重试/降级/熔断
- **AND** 行为 SHALL 与调用底层客户端等价

### Requirement: 弹性调用可观测性

弹性客户端 SHALL 就重试、降级、熔断跃迁发射可观测信号：SHALL 计量尝试次数、重试次数、fallback 激活次数、熔断状态跃迁、各目标调用时延与结果；SHALL 在结构化日志中透传 `request_id` 与目标 `provider+model`。所有可观测输出 MUST NOT 包含 api_key、host 等敏感信息。

#### Scenario: 降级作为可观测事件记录

- **WHEN** 一次调用从主目标降级到备用目标
- **THEN** SHALL 发射一条 fallback 激活信号，标注 from/to 目标
- **AND** 携带 `request_id`

#### Scenario: 熔断跃迁被记录

- **WHEN** 某目标熔断器发生状态跃迁（closed↔open↔half-open）
- **THEN** SHALL 发射对应的状态跃迁信号

#### Scenario: 可观测输出脱敏

- **WHEN** 发射任何指标或日志
- **THEN** 输出 MUST NOT 包含 api_key 或 host
