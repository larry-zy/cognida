# Design: 统一 LLM 弹性（重试 / 降级 / 熔断）

## 目标与非目标

**目标**：在不改动任何上层调用方的前提下，为所有 LLM 流量（chat / embedding / rerank）提供统一的：请求级重试、模型降级/故障转移、per-target 熔断、类型化错误分级、可观测性。

**非目标**：
- 不做吞吐型负载均衡（多健康 provider 间轮询分流）——本期只做**故障转移**，均衡留作后续。
- 不做语义缓存（另有 `semantic cache` 相关工作）。
- 不改动工具/ReAct 层的 SQL 错误修复（属 `data-agent-self-repair`）。
- 不做流式**首 chunk 之后**的失败重放（见"流式边界"）。
- 不新增 DB 表/列。

## 关键决策

### 1. 装饰 seam 选在领域接口 `LLMClient`，而非 HTTP transport

候选：
- **(A) HTTP transport 层**（`httpx.SharedTransport` 包 `RoundTripper`）：能拿到 HTTP status 做重试，但**看不到"目标模型"语义**，无法实现"切到另一个 provider/model"的降级，也无法按 provider+model 维度熔断；且 SSE 流式在 transport 层无法安全地做"首 chunk 前才失败转移"。
- **(B) 领域接口 `LLMClient` 装饰器**（选中）：`Chat`/`ChatStream` 语义完整，天然支持"换目标重放"（降级）、按目标熔断、流式边界判定；对上层完全透明（返回类型仍是 `LLMClient`）。

选 **(B)**。retry/circuit/fallback 都在装饰器内实现；分级所需的 HTTP status 由 provider client 通过类型化 `APIError` 上报（见决策 3），装饰器无需感知 HTTP 细节。

```
调用方(chat-service / rag / agent framework)
        │  domainllm.LLMClient (不变)
        ▼
┌─────────────────────────────────────────┐
│ resilientClient (装饰器)                 │
│  ├─ FallbackChain: [primary, fb1, fb2]   │
│  ├─ 每个 target 前置 CircuitBreaker      │
│  └─ 每个 target 内 retry(退避+抖动)       │
└─────────────────────────────────────────┘
        │ 逐目标尝试
        ▼
  底层 LLMClient(openai/ollama/...) → 返回 *APIError
```

### 2. 两层容错：先"同目标重试"，再"跨目标降级"

- **内层（同目标重试）**：对 `transient`/`rate_limited`，在同一 target 上按 `maxAttempts`（默认 3）指数退避 + 抖动重试。`rate_limited` 若响应含 `Retry-After` 则优先取其值（上限封顶，防止恶意/超长等待）。
- **外层（跨目标降级）**：内层在某 target 上重试耗尽、或该 target 熔断器 `open` → 记录该目标失败摘要，前进到 fallback 链的下一个 target，重置内层预算重新尝试。
- **终态**：全链耗尽 → 返回聚合 `APIError{Class: terminal}`，`Detail` 汇总各目标的最后失败原因（脱敏，不含 key/host）。

顺序保证："先穷尽单目标的廉价重试，再付出切目标的代价"——避免因一次瞬时抖动就切走主模型（主模型通常质量/成本更优）。但熔断器 `open` 时跳过重试直接降级（已知故障，重试无意义）。

### 3. 类型化错误分级（`APIError` + `Classify`）

现状各 client 返回裸 `fmt.Errorf`，装饰器无法决策。引入：

```go
// internal/model/llm/error.go
type ErrorClass string
const (
    ClassRateLimited ErrorClass = "rate_limited" // 429
    ClassTransient   ErrorClass = "transient"    // 5xx / 网络抖动 / 可重试超时 / 连接重置
    ClassTerminal    ErrorClass = "terminal"     // 4xx 鉴权/参数/未找到
    ClassCanceled    ErrorClass = "canceled"     // 调用方 ctx 取消/超时
)
type APIError struct {
    Provider   llm.Provider
    Model      string
    StatusCode int           // HTTP 状态码，0 表示传输层错误
    Class      ErrorClass
    RetryAfter time.Duration // 仅 rate_limited，解析自响应头，0 表示无
    Detail     string        // 脱敏摘要
    Err        error         // 包裹的底层错误（errors.Unwrap 可达）
}
```

- **provider client 侧**：失败路径构造 `APIError`，填 `StatusCode`/`Provider`/`Model`/`RetryAfter`；分类由 `classifier.Classify(statusCode, err)` 统一裁决，避免各 client 各写一套。
- **classifier**：`429→rate_limited`；`500/502/503/504→transient`；`400/401/403/404/408?/422→terminal`（`408` 请求超时视 provider 语义，缺省 transient）；传输层：`context.Canceled`→canceled、`context.DeadlineExceeded`（**每次尝试**的子 ctx 超时）→transient、`net` 连接重置/拒绝/EOF→transient；未匹配→terminal（保守：不确定不重试，避免放大副作用——LLM 补全虽幂等，但终态错重试无益且浪费额度）。
- **调用方 ctx 取消判定**：装饰器在每次尝试前后检查**父 ctx**；父 ctx 已 `Done` → 直接返回 `canceled`，MUST NOT 继续重试或降级。

### 4. 熔断器（per `provider+model`，三态）

```
closed ──连续失败≥failThreshold──▶ open
  ▲                                  │ 冷却 cooldown
  │ 探测成功                          ▼
closed ◀──探测成功── half-open ──探测失败──▶ open
```

- 计数只统计 `transient`/`rate_limited`/终态传输错；`terminal`（鉴权/参数错）**不计入**熔断（那是配置问题，不是 provider 健康问题，且会误伤）。
- `open` 期间对该 target 的调用**不发起网络请求**，直接视为失败并让外层降级到下一目标。
- `half-open` 只放行**单个**探测请求，其余并发调用直接降级；探测结果决定回到 `closed` 或 `open`。
- 熔断器状态是 target 级**进程内共享单例**（同一 `provider+model` 的所有 client 实例共享），否则每请求新建 client 会让熔断失效。由 `resilience` 包内 `breakerRegistry`（`map[targetKey]*breaker` + `sync.Mutex`）托管。

### 5. 流式（`ChatStream`）失败转移边界

`ChatStream` 返回 `<-chan *ChatChunk`。**一旦向调用方发出第一个 chunk，就不能再透明失败转移**（部分输出已投递，重放会重复/错乱）。策略：

- 装饰器先调用底层 `ChatStream` 建流；**在读到第一个 chunk 之前**若底层 error（建流失败/首包前断流）→ 走完整重试+降级逻辑（对调用方仍是"还没开始"）。
- **首个 chunk 已转发后**若底层出错 → 关闭并把错误作为**流终止**透传给调用方（`ChatChunk` 的错误通道 / 关闭 chan），MUST NOT 切目标重放。
- 熔断器计数：流在首 chunk 前失败按失败计；首 chunk 后失败**不计入**熔断（已建立连接，非"目标不可用"）。

### 6. fallback 链的装配（组合根，无 DB 变更）

`ModelFactory` 只认识"单个 `ModelConfig`"。fallback 链的**编排放在组合根**（`ModelService` / DI 装配处）：

- 单目标弹性：`factory.CreateChatModel(cfg)` 内部即套 `resilientClient{targets:[cfg]}`（只有重试+熔断，无降级）——这是默认、对所有既有调用点生效。
- 多目标降级：`ModelService.CreateChatModel` 拿到主模型后，查询**同租户、同 `model_type`、`Enabled=true`** 的其余模型，以主（`IsDefault`）为首、其余按 `id` 稳定排序追加，构造 `FallbackChain`，传给 factory 的 `WithResilience(chain)`。
- v1 不加 DB 列：fallback 顺序由"是否默认 + id"隐式决定；显式优先级（`priority` 列）留作后续增强（届时走 `cmd/migrate-db` 同步，见 CLAUDE.md）。

`ResilienceConfig` 缺省值（可被配置覆盖）：`maxAttempts=3`、`baseBackoff=200ms`、`maxBackoff=5s`、`retryAfterCap=30s`、`failThreshold=5`、`cooldown=30s`、`halfOpenProbes=1`。

### 7. 向后兼容与 opt-out

- 无 `WithResilience` / 空 fallback 链时：装饰退化为"单目标 + 重试 + 熔断"，成功路径信封与现状**逐字节一致**（装饰器成功时原样透传底层响应）。
- 提供 `WithResilience(ResilienceConfig{Enabled:false})` 完全关闭 → 直连底层 client，零开销（用于压测/排障对照）。
- 类型化 `APIError` 实现 `error` 且 `Unwrap` 到底层错误，既有 `errors.Is/As` 与日志不受影响。

## 可观测性

- 指标（复用 `domainllm.MetricsCollector` 或新增轻量 counter）：`llm_attempts_total{provider,model,class}`、`llm_retries_total`、`llm_fallback_activations_total{from,to}`、`llm_circuit_state{target}`（gauge：0=closed/1=half/2=open）、`llm_call_duration_seconds{provider,model,outcome}`。
- 日志：每次重试/降级/熔断跃迁打结构化日志，带 `request_id`（全链路透传，见 CLAUDE.md 约定）与目标 `provider+model`，**脱敏**（不含 api_key/host）。
- 审计：降级激活（切换到备用模型）作为可观测事件记录，便于事后定位"为何本次用了备用模型"。

## 风险与缓解

| 风险 | 缓解 |
|------|------|
| 重试放大上游压力（retry storm） | 指数退避 + 抖动 + 熔断快速失败 + `maxAttempts` 封顶；`Retry-After` 封顶但优先遵循 |
| 熔断误伤（把配置错当健康问题） | `terminal`（4xx 鉴权/参数）不计入熔断计数 |
| 流式重复输出 | 首 chunk 后禁止失败转移，硬边界 |
| 备用模型质量/成本漂移 | 降级作为显式可观测事件；主目标优先、仅在必要时降级 |
| 每请求新建 client 使熔断失效 | 熔断器按 `provider+model` 进程内共享单例 |
| ctx 取消被误当瞬时错重试 | classifier 严格区分 `context.Canceled`(canceled) 与每次尝试子 ctx 的 `DeadlineExceeded`(transient)，父 ctx 取消立即终止 |

## 迁移与落地顺序

1. 领域层类型（`APIError`/`ErrorClass`/`ResilienceConfig`/`FallbackChain`）—— 无行为变更。
2. `resilience` 包（classifier / retry / circuit_breaker / 装饰器）+ 单测。
3. provider client 失败路径改吐 `APIError`（成功路径不动）。
4. `model_factory` 套装饰器（默认单目标弹性）——此步起既有调用点即获重试+熔断。
5. 组合根装配 fallback 链（多目标降级）。
6. 指标/日志接线 + 端到端集成测试（含 provider 故障注入）。
