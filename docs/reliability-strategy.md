# 跨通路可靠性策略（重试 / 超时 / 熔断）〔X-4 / X-5〕

统一后各出站通路的重试、超时、熔断策略由 `internal/infrastructure/reliability` 单一实现驱动，
不再各写一套或缺失。核心是与传输无关的三态 per-target 熔断器 + gRPC 透明重试服务配置。

## 统一实现

`internal/infrastructure/reliability`：

- `Config` —— 统一配置（`MaxAttempts` / `BaseBackoff` / `MaxBackoff` / `FailThreshold` / `Cooldown` /
  `HalfOpenProbes`）。`DefaultConfig()` 默认开启：3 次尝试、200ms→5s 退避、连续 5 次可计失败熔断、
  30s 冷却、1 个半开探测。
- `Breaker` / `Registry` —— 三态熔断（closed / open / half-open），按目标地址隔离，并发安全。
  仅「目标不健康」类失败计入（Unavailable / ResourceExhausted / DeadlineExceeded；HTTP 网络错误/5xx），
  调用方取消、4xx/契约错误不计入。
- `ServiceConfigJSON` —— gRPC 透明重试服务配置；`UnaryClientInterceptor` / `StreamClientInterceptor` ——
  按连接 target 熔断的拦截器。

透明重试位于连接层、熔断拦截器之下：**短暂抖动由重试吸收，持续故障由熔断在多次失败后打开、快速失败**，二者互补。

## 各通路接入

| 通路 | 位置 | 重试 | 熔断 | 说明 |
|------|------|------|------|------|
| gRPC（docreader / quality） | `infrastructure/grpc/client.go` | 服务配置 retryPolicy（UNAVAILABLE / RESOURCE_EXHAUSTED） | per-target 拦截器 | 基础客户端默认开启，所有走它的 gRPC 通路自动获得，无需各自实现 |
| HTTP 评测 | `service/evaluation/python_client.go` | 既有 5xx/网络重试 | 复用 `reliability.Breaker`，仅护重的 `compute-metrics`（健康检查/目录查询不熔断） | compute 逐条 8s 预算可堆到 20min，熔断避免持续不可用时空等 |
| MCP | `infrastructure/mcp/client.go` | 自带 `reconnectConfig` 重连+退避 | 实验性通路，沿用自带重连，未接统一熔断 | |

> 关闭方式：给 gRPC `Config.Reliability = &reliability.Config{Enabled:false}`。

## 服务地址集中〔X-5〕

Python gRPC 目标统一由 `config.LoadPythonGrpcConfig()`（`PYTHON_GRPC_TARGET`，默认 `localhost:50051`）提供。
`ProvideDocReaderClient` 不再硬编码 `localhost:50051`，与 quality 网关复用同一目标——地址单一事实源。

## 与 LLM 弹性模块的关系

`internal/infrastructure/llm/resilience`（含降级链）按「模型目标」粒度、本包按「服务目标」粒度，二者策略同构、
各司其职，互不替代。
