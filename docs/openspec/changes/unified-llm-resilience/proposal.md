## Why

项目当前**没有任何统一的 LLM 调用容错能力**。所有聊天/嵌入/重排流量最终都经 `domainllm.LLMClient` / `EmbeddingRepository` / `RerankRepository`（由 `ModelFactory` 构造）直接打到单一 provider，而这条链路上：

- **无请求级重试**：`infrastructure/llm/httpx/transport.go` 只调了连接池与握手超时；chat provider（`chat/openai.go`/`ollama.go`/`remote.go`）对 429 限流、5xx、网络抖动、可重试超时**一次都不重试**，一抖就整轮失败。（唯一的 `retry:` 出现在 `chat/sse.go` 里解析 SSE 字段的注释，且未落实。）
- **无故障转移/降级**：主模型 provider 宕机或额度耗尽时，不会切换到备用模型；`GetDefaultModel` 只是"选默认"，不是运行时切换。
- **无熔断**：某个 provider 持续超时/5xx 时，后续请求仍会一次次打过去等超时，既放大延迟又拖垮上游。
- **错误裸奔**：各 client 返回 `fmt.Errorf("api error (status %d)...")` 之类的字符串错误，调用方无法区分"可重试瞬时错"与"终态错"（鉴权失败/参数非法），无法据此决策。

代码里几处叫 "fallback" 的东西都不是这回事：`fallbackOrchestrator` 是 agent 初始化失败后的"简单回声"降级，`fallbackConclusion` 是结构化输出解析失败的兜底，均与 LLM provider/请求级容错无关。已有的 `data-agent-self-repair` 提案里的"瞬时错工具内重试"也**只作用于 SQL 工具**，不覆盖 LLM 调用本身。

结果：任何一次上游 provider 抖动（限流、5xx、连接重置）都会直接冒泡成用户可见的失败，可用性完全取决于单一 provider 的稳定性。

## What Changes

在 `domainllm.LLMClient` / `EmbeddingRepository` / `RerankRepository` 这一**领域接口 seam** 上引入一层**透明的弹性装饰器**，把重试、降级、熔断统一收口，对所有上层调用方（chat-service、rag-service、agent framework）零改动生效。

- **统一错误分级**：引入带 `StatusCode`/`Provider`/`Class` 的类型化 `APIError`；各 provider client 在失败路径返回它而非裸字符串。分级枚举 `error_class`：`rate_limited`（429）、`transient`（5xx / 网络抖动 / 可重试超时 / 连接重置）、`terminal`（4xx 鉴权/参数/未找到）、`canceled`（调用方 ctx 取消，透传不重试）。
- **请求级退避重试**：对 `transient`/`rate_limited` 在**同一目标**上做有限次指数退避 + 抖动重试；`rate_limited` 优先遵循 `Retry-After` 响应头；`terminal`/`canceled` 立即失败、绝不重试；全程尊重调用方 ctx deadline/cancel。
- **模型降级/故障转移链**：为每种模型类型配置**有序 fallback 链**（主模型 → 备用模型…）。主目标重试耗尽或其熔断器打开时，自动切到下一个目标；全链耗尽才返回携带各目标失败摘要的终态错。
- **熔断器（per-target）**：按 `provider+model` 维度统计连续失败，达阈值 `open` 冷却窗口内直接快速失败并跳到 fallback；冷却后 `half-open` 探测，成功则 `closed`，失败则重新 `open`。避免对已知故障目标反复空等。
- **流式失败转移边界**：`ChatStream` 的重试/降级**只在首个 chunk 到达前**生效；一旦已向调用方吐出 token，中途错误 SHALL 作为流错误上抛，MUST NOT 静默切换目标重放（防止重复/错乱输出）。
- **可观测性与配置**：统一发射指标（尝试次数、重试数、fallback 激活、熔断状态跃迁、各目标成功/失败/时延），全链路透传 `request_id`；弹性策略可经配置调参，并可整体关闭（opt-out）退化为直连——**无 fallback 链、无策略配置时行为与现状等价（向后兼容）**。

## Capabilities

### New Capabilities
- `llm-resilience`: LLM 调用的统一弹性契约——类型化错误分级、请求级退避重试、有序模型降级/故障转移链、per-target 熔断器、流式失败转移边界、可观测性与安全默认（可 opt-out）。

### Modified Capabilities
- `chat-service`: "Model Instance Creation" 产出的 chat/embedding/rerank 实例 SHALL 经弹性装饰器包装后返回；装饰对上层透明，不改变既有创建契约与 DTO。

## Impact

- **cognida-go**：
  - `internal/model/llm/`：新增 `error.go`（类型化 `APIError` + `error_class` 枚举 + `Classify(err)` 契约）、`resilience.go`（`ResilienceConfig`/`FallbackChain` 领域配置结构）。
  - `internal/infrastructure/llm/resilience/`（新增包）：`retry.go`（退避+抖动+Retry-After）、`circuit_breaker.go`（三态机）、`resilient_client.go`（`LLMClient` 装饰器）、`resilient_embedding.go`/`resilient_rerank.go`（同构装饰器）、`classifier.go`（HTTP status + 网络错误 → `error_class`）。
  - `internal/infrastructure/llm/chat/`（`openai.go`/`ollama.go`/`remote.go`）与 `embedding_repo.go`/`rerank_repo.go`：失败路径返回类型化 `APIError`（含 `StatusCode`/`Provider`）。
  - `internal/infrastructure/llm/model_factory.go`：`CreateChatModel`/`CreateEmbeddingModel`/`CreateRerankModel` 在返回前套装饰器（可经 factory option 关闭）。
  - `internal/service/chat/model_service.go`：组合根按模型类型装配 fallback 链（主 = `IsDefault`，其余同类模型按序追加），无备用时退化为单目标弹性（仅重试+熔断）。
- **契约**：新增类型化 `APIError`（成功路径不变）；`ModelFactory` 可选 `WithResilience(cfg)`；弹性策略默认开启但可整体 opt-out，无 fallback 链时等价现状。
- **测试**：分级/重试/退避/Retry-After 单测、熔断三态跃迁单测、fallback 链耗尽与中途成功单测、流式首 chunk 前后失败边界单测、装饰透明性回归（成功路径信封不变）、ctx 取消透传不重试。
- **依赖**：无新增外部依赖；纯 Go 标准库实现退避与熔断（不引入第三方 resilience 库）。
- **无 DB 变更**：不新增表/列；fallback 链由组合根从既有 `model_configs` 同类模型推导，v1 不落库额外优先级字段。
- **与既有工作的边界**：本提案覆盖 **LLM 调用层**（provider/请求级）容错；`data-agent-self-repair` 覆盖 **工具/ReAct 层**（SQL 错误修复、循环护栏），两者互补、无重叠——ReAct 的 SQL 瞬时重试与本层的 LLM 请求重试各司其职。
