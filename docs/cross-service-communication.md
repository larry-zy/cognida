# 跨服务通信规则（Go ↔ Python）

> 本文规定 cognida-go 与 cognida-python 之间的通信选型与职责边界，是「Python 只做计算、Go 承主后端」
> 这一物理边界的落地约束。新增跨服务调用前，先对照本文选择通道。

## 一、职责边界（编排 vs 计算）

| 维度 | Go（cognida-go） | Python（cognida-python） |
|------|---------------|------------------------|
| 角色 | 主后端 · **权威编排** | **无状态计算/工具** 增强 |
| 持有 | 流程、状态、进度、事务、落库 | 无状态：入参进、结果出，不留状态 |
| 评测 | 编排评测流程（样本集、顺序、检索、生成、聚合、进度、落库） | 仅算分（给 reference/hypothesis 出指标） |
| 数据分析 | 触发与结果消费 | 统计/趋势/洞察计算 |
| 文档 | 触发与结果消费 | 解析/OCR/分块/URL 抓取 |

**铁律**：Python 侧不得再出现「第二套编排引擎」（状态机、跨样本流程、进度推送、落库）。
凡涉及流程与状态的编排，归 Go worker 唯一权威。Python 端一律收敛为 `compute_*` 形态的纯函数式计算。

## 二、通道选型

| 通道 | 端口 | 用途 | 语义 |
|------|------|------|------|
| **gRPC**（主通道） | :50051 文档 / :50053 分析 | 高性能、大数据、二进制 Protobuf 的计算调用 | 无状态请求-响应 |
| **HTTP/JSON** | :18888 | 评测指标计算、健康检查 | 无状态请求-响应 |
| **MCP** | — | AI 工具调用、实验功能 | 工具语义 |

选型原则：

1. **默认走 gRPC**：高性能、大数据量、需强类型契约的计算（文档解析、OCR、数据分析）。
2. **HTTP :18888 仅用于评测的无状态 compute 与健康检查**——它是 Go worker 编排评测流程时的算分后端，
   不承载任何编排/进度/流式状态。契约见 `cognida-python/services/evaluation/fastapi_app.py`。
3. **不使用流式 RPC 承载编排进度**。进度是编排产物，属于 Go；Python 不推进度。
   （历史上的 `EvaluationService.ExecuteEvaluation` 流式编排已废弃删除。）

## 三、评测调用形态（示例）

Go worker 编排评测，逐批把样本发给 Python 算分，取回指标后自行聚合、记进度、落库：

```
Go worker (编排)                         Python (:18888 无状态算分)
  ├─ 拉数据集 / 检索 / 生成
  ├─ POST /api/v1/evaluation/compute-metrics ──▶  算 ROUGE/BLEU/NDCG/语义/LLM-Judge
  │      { eval_type, items:[{reference,hypothesis,question,context}] }
  │   ◀── { 各族指标分数 }
  ├─ 聚合 / 记进度 / 更新状态
  └─ 落库
```

- Go 端客户端：`cognida-go/internal/service/evaluation/python_client.go`（HTTP-only）。
- Python 端：`cognida-python/services/evaluation/fastapi_app.py`（无状态 compute 薄壳）。

## 四、链路追踪

跨进程调用必须透传 `request_id`：

- gRPC：`RequestIDServerInterceptor` 从 metadata 提取绑定日志上下文。
- HTTP :18888：`RequestIDMiddleware` 读取 `X-Request-ID`；Go 侧 `python_client.go` 的
  `setTraceHeaders` 从 `agentctx.GetRequestID(ctx)` 写入该头。

同一 `request_id` 在 Go/Python 两侧日志与 MySQL 审计中可关联，实现全链路追踪。
