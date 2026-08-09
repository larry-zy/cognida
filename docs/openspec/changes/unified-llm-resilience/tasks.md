## 1. 领域层：类型化错误分级与弹性配置

- [x] 1.1 新增 `model/llm/error.go`：定义 `ErrorClass` 枚举（`rate_limited`/`transient`/`terminal`/`canceled`）与 `APIError`（`Provider`/`Model`/`StatusCode`/`Class`/`RetryAfter`/`Detail`/`Err`），实现 `Error()` 与 `Unwrap()`
- [x] 1.2 定义 `Classify(statusCode int, err error) ErrorClass` 契约：HTTP status 映射（429→rate_limited；5xx→transient；4xx→terminal）+ 传输层映射（`context.Canceled`→canceled、每次尝试子 ctx `DeadlineExceeded`→transient、`net` 连接重置/拒绝/EOF→transient）；未匹配保守归 terminal
- [x] 1.3 新增 `model/llm/resilience.go`：`ResilienceConfig`（maxAttempts/baseBackoff/maxBackoff/retryAfterCap/failThreshold/cooldown/halfOpenProbes/Enabled）与 `FallbackChain`（有序 `[]*ModelConfig`）；提供带安全默认值的构造
- [x] 1.4 单元测试：各 status/网络错的分级归类、ctx 取消与尝试超时的区分、默认配置装配

## 2. 基础设施层：retry + circuit breaker + classifier

- [x] 2.1 新增 `infrastructure/llm/resilience/classifier.go`：实现 `Classify`，解析 `Retry-After`（秒/HTTP-date 两种格式，封顶 `retryAfterCap`）
- [x] 2.2 新增 `resilience/retry.go`：指数退避 + 抖动（full jitter），`rate_limited` 优先用 `RetryAfter`；每次尝试派生带超时子 ctx；父 ctx `Done` 立即返回 `canceled`，MUST NOT 继续
- [x] 2.3 新增 `resilience/circuit_breaker.go`：三态机（closed/open/half-open），`failThreshold` 触发 open、`cooldown` 后 half-open 放行 `halfOpenProbes` 个探测；`terminal` 不计入失败计数；并发安全
- [x] 2.4 新增 `resilience/registry.go`：`breakerRegistry` 按 `provider+model` 托管进程内共享熔断器单例（`map + sync.Mutex`）
- [x] 2.5 单元测试：退避序列与抖动上界、Retry-After 解析与封顶、熔断三态跃迁、terminal 不计入、half-open 单探测放行、注册表同 target 复用

## 3. 基础设施层：弹性装饰器（三类客户端）

- [x] 3.1 新增 `resilience/resilient_client.go`：装饰 `domainllm.LLMClient`，实现两层容错（内层同目标 retry、外层跨目标 fallback 链），全链耗尽返回聚合 `APIError{terminal}`（脱敏汇总各目标失败）
- [x] 3.2 `ChatStream` 失败转移边界：首 chunk 前完整重试+降级；首个 chunk 已转发后错误作为流终止透传，MUST NOT 切目标重放；首 chunk 后失败不计入熔断
- [x] 3.3 新增 `resilient_embedding.go`/`resilient_rerank.go`：同构装饰 `EmbeddingRepository`/`RerankRepository`（无流式，逻辑更简）
- [x] 3.4 `SupportsStreaming()` 等透传方法：以链首目标能力为准
- [x] 3.5 单元测试（fake client 注入故障）：单目标重试后成功、重试耗尽降级到 fb1、全链耗尽聚合错、熔断 open 直接跳目标、成功计数重置、装饰透明性（成功信封逐字节不变）

## 4. provider client 失败路径吐类型化 APIError

- [x] 4.1 `infrastructure/llm/chat/openai.go`/`remote.go`：非 200 与传输错构造 `APIError`（含 `StatusCode`/`Provider`/`Model`/解析 `Retry-After`），经 `Classify` 定级
- [x] 4.2 `chat/ollama.go`：同上（Ollama 无标准 Retry-After，5xx/连接错归 transient）
- [x] 4.3 `embedding_repo.go`/`rerank_repo.go`：`postJSON`/请求失败路径改吐 `APIError`，替换裸 `fmt.Errorf("api error (status %d)...")`
- [x] 4.4 单元测试：各 client 在 429/5xx/4xx/网络错下返回正确 `Class` 与 `StatusCode`；成功路径不变（回归）

## 5. 工厂与组合根装配

- [x] 5.1 `infrastructure/llm/model_factory.go`：新增 `WithResilience(cfg)` option；`CreateChatModel`/`CreateEmbeddingModel`/`CreateRerankModel` 返回前套对应装饰器（默认单目标弹性：仅 retry+熔断）
- [x] 5.2 `service/chat/model_service.go` `CreateChatModel`：查询同租户/同 `model_type`/`Enabled` 的模型，主（`IsDefault`）为首、其余按 `id` 稳定排序构造 `FallbackChain` 传入 factory；无备用则退化单目标
- [x] 5.3 embedding/rerank 创建路径同样装配（多为单目标，链长=1 也走弹性）
- [x] 5.4 `Enabled:false` opt-out 路径：直连底层 client、零装饰开销
- [x] 5.5 集成测试：多模型租户下主目标故障→自动用备用；单模型租户仅重试+熔断；opt-out 直连

## 6. 可观测性与配置

- [x] 6.1 装饰器接线指标：`llm_attempts_total`/`llm_retries_total`/`llm_fallback_activations_total`/`llm_circuit_state`/`llm_call_duration_seconds`（复用 `MetricsCollector` 或轻量 counter）
- [x] 6.2 重试/降级/熔断跃迁打结构化日志，透传 `request_id`，脱敏（无 api_key/host）
- [x] 6.3 `ResilienceConfig` 支持经配置覆盖默认值（env/配置文件），装配处读取
- [x] 6.4 单元测试：指标计数正确、日志脱敏、配置覆盖生效

## 7. 端到端与回归

- [x] 7.1 故障注入集成测试：mock provider 分别注入 429（带 Retry-After）/503/连接重置/超时，断言重试→降级→熔断全链行为
- [x] 7.2 流式端到端：首 chunk 前失败转移成功、首 chunk 后失败正确透传不重放
- [x] 7.3 透明性回归：chat-service / rag / agent 三条既有调用链在装饰后成功路径行为不变
- [x] 7.4 ctx 取消端到端：调用方取消后立即终止，不产生额外重试/降级请求
