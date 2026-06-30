## Context

当前 Text2SQL 链路止于取数：`get_schema → sql_execute` 返回行集，结论由 `framework/hooks/conclusion.go` 的 `ConclusionGenerator` 用 LLM 对原始行集做文字归纳，无统计计算支撑。

两端 MCP 基础设施已就绪但未接通：
- **Go**：`internal/infrastructure/mcp/client.go` 的 `MCPClient` 已实现真实 JSON-RPC `tools/call` / `tools/list`（HTTP，含指数退避重试与缓存）。Agent 工具为 eino `tool.InvokableTool`（参考 `tools/sql_execute.go`），经 `tools/init.go` 注册。
- **Python**：`mcp/server.py` 支持 stdio / http 模式，`tools/registry.py` 提供全局 `ToolRegistry`，`tools/base.py` 定义 `BaseTool`。`services/analytics`（`statistics.py`/`trend.py`/`insight.py`/`validation.py`）已实现引擎。

关键现状缺口：`ToolRegistry` 启动时**未注册任何工具**（仅测试中注册过），故 `tools/list` 为空。

约束：遵循 CLAUDE.md 的 `handler → service → model ← repository` 依赖方向与开发流程；纯新增，不改既有 analytics 计算行为。

## Goals / Non-Goals

**Goals:**
- 复用现有 MCP 通道，把 `services/analytics` 暴露为可调用工具。
- Go Agent 取数后能调用真实分析（统计/趋势/异常/相关/洞察）。
- 结论与行动建议基于分析输出而非纯 LLM 叙述。
- 约定稳定的行集 JSON 数据契约。

**Non-Goals:**
- 不改 `services/analytics` 既有引擎的计算逻辑。
- 不引入新的 gRPC 服务化（本次走 MCP）。
- 不做前端展示改造（link-web 不在范围内）。
- 不实现新的可视化/图表能力。

## Decisions

### D1：走 MCP 而非新建 gRPC client
复用 Go `MCPClient`（已支持 `tools/call`、重试、缓存）与 Python http MCP server。
- **理由**：通道现成，改动面最小，与已有 skill 集成同源。
- **备选**：新建 Go→Python analytics gRPC client（`analytics_servicer.py` 已存在，端口 50053）。更高性能但需新 client、proto 接线、连接管理，成本高于收益。本次数据量为单次查询行集，HTTP/JSON 足够。

### D2：细分工具而非单一总入口
暴露 `data_describe` / `data_trend` / `data_anomaly` / `data_correlation` / `data_insight` 五个 MCP 工具，Go 侧 `data_analysis` 工具以 `analysis_type` 路由到对应 MCP 工具名。
- **理由**：每个工具有清晰 `input_schema`，便于 ReAct LLM 选择与参数填充；与现有一类工具一职的风格一致。
- **备选**：单一 `data_analysis(analysis_type=...)` MCP 工具，省 token 但 schema 含糊、参数耦合。Go 侧仍保留单一 `data_analysis` Agent 工具做统一入口以简化提示词。

### D3：行集 JSON 数据契约
约定 `{"columns": [...], "rows": [[...], ...]}`（或等价 records 数组）作为分析输入；Python 侧统一转 `DataFrame` 并用 `validation.py` 的 `sanitize_dataframe` 清洗（NaN/类型）。结果为 JSON-serializable，由 MCP handler `json.dumps` 返回。
- **理由**：与 `sql_execute` 输出形态自然对齐，转换集中在 Python 一处。

### D4：注册引导 `register_default_tools`
新增 `tools/bootstrap.py`，在 `mcp/server.py` 初始化拿到 `self._registry` 后调用一次，幂等。
- **理由**：修复空注册缺口，且为后续工具提供统一注册入口。

### D5：结论闭环接线
把 `data_analysis` 加入 `ConclusionGenerator.dataTools`，并将其分析输出作为 `ToolCallInfo.Output` 传入，使 `KeyFindings/Insights/Recommendations` 基于计算结果。
- **理由**：兑现「有依据的行动建议」，改动局部。

### D6：编排策略
以 ReAct 为主（LLM 自行决定取数后是否分析），关键路径可选用 `orchestration/sequential.go` 固化 `get_schema → sql_execute → data_analysis → conclusion` 兜底确定性。

## Risks / Trade-offs

- [HTTP/JSON 传输大行集性能/体积] → 取数侧已 `LIMIT`；必要时在 `data_analysis` 工具约定最大行数并截断+告警。
- [Python MCP 与 Go 配置端口不一致导致调用失败] → 统一从 `infrastructure/config` 读取 MCP endpoint；集成测试覆盖连通性；`MCPClient` 已有健康检查与重试。
- [列名/类型不匹配（如时间列识别）] → 复用 `validation.py` 的 `TimeSeriesDetector`/`DataChecker`；schema 要求显式传 `time_col`/`value_col`，无则返回结构化错误而非异常。
- [LLM 误选 analysis_type 或漏传参数] → `input_schema` 标注必填项；工具对缺参返回可读错误，交给 ReAct 自我修正。
- [注册引导引入循环 import] → `bootstrap.py` 延迟 import analytics 工具，仅在 server 初始化时调用。

## Migration Plan

1. Python：新增 `tools/analytics/` + `tools/bootstrap.py`，`mcp/server.py` 接入注册；以 http 模式起服务。
2. Go：新增 `data_analysis.go`，`tools/init.go` 注册，`conclusion.go` 纳入数据工具。
3. 链路：更新 Text2SQL/数据分析 Agent 工具集与提示词。
4. 测试：Python 单测+MCP 集成；Go 单测（mock HTTP）+ 集成 + E2E。
5. 回滚：纯新增能力，回滚即从 `init.go`/`bootstrap.py` 摘除注册，既有取数链路不受影响。

## Open Questions

- 行集最大行数上限取值？（建议默认上限 + 截断告警）
- `data_insight` 的 recommendations 是否需要 LLM 二次润色，还是直接用 finder 规则输出？
- 是否需要把分析结果持久化（已有 "Analytics Result Storage" 占位）——本次范围外，留待后续。
