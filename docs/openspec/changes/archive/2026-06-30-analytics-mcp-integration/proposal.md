## Why

Text2SQL 链路目前只做到「取数」：自然语言 → SQL → 执行返回行集。取数之后的「结论建议」完全由 LLM 对原始行集做文字归纳（`framework/hooks/conclusion.go`），缺乏统计/趋势/异常等真实计算支撑，无法兑现「输出有依据的行动建议」的产品定位。与此同时，Python 侧 `services/analytics`（描述统计、趋势预测、异常检测、相关性、洞察生成）已较完整实现，但被孤立——既没有暴露为可调用工具，Go Agent 也没有任何客户端去调用它。

本次变更通过**复用现有 MCP 通道**把这套分析引擎接入 Agent，让取数后能跑真实分析并产出有计算依据的洞察与建议，从而打通数据分析全链路。

## What Changes

- **补齐 Python 工具注册缺口**：`ToolRegistry` 启动时未注册任何工具，导致 MCP `tools/list` 为空。新增启动期注册引导（`register_default_tools`），由 MCP server 初始化时调用。
- **新增 Analytics MCP 工具**：将 `services/analytics` 的引擎包装为 MCP 工具（`data_describe`、`data_trend`、`data_anomaly`、`data_correlation`、`data_insight`），通过 `tools/list` / `tools/call` 暴露；输入为行集（JSON records），内部转 `pandas.DataFrame` 计算后回传 JSON。
- **新增 Go `data_analysis` Agent 工具**：实现 eino `tool.InvokableTool`，复用现有 `infrastructure/mcp.MCPClient`（已支持 `tools/call`、重试、缓存）调用上述分析工具，并注册进 Agent 工具表。
- **接入取数链路**：Text2SQL / 数据分析 Agent 的工具集与提示词加入 `data_analysis`，引导「取数 → 分析 → 结论」；`ConclusionGenerator` 将 `data_analysis` 纳入数据工具集合，使结论与建议基于真实分析输出而非纯 LLM 叙述。
- **约定数据契约**：Go 取数行集与 Python 分析输入之间的 JSON records 格式（`columns` + `rows`）及结果结构。

## Capabilities

### New Capabilities
- `analytics-mcp-tools`: Python 侧将 analytics 引擎暴露为 MCP 工具，并补齐工具注册引导，使 `tools/list` / `tools/call` 可发现并调用分析能力。
- `agent-data-analysis-tool`: Go 侧新增 `data_analysis` Agent 工具，经 MCP 调用 Python 分析能力，并接入取数后的分析-结论链路。

### Modified Capabilities
<!-- 不修改既有 spec 的需求：本次为新增能力与接线，analytics-statistics/trend/insight 的既有计算行为不变。 -->

## Impact

- **Python (`cognida-python`)**：新增 `tools/analytics/`（MCP 工具包装）与 `tools/bootstrap.py`（注册引导）；`mcp/server.py` 初始化处调用注册；MCP 以 http 模式常驻以供 Go 调用。
- **Go (`cognida-go`)**：新增 `internal/service/agent/tools/data_analysis.go`；在 `internal/service/agent/tools/init.go` 注册；`framework/hooks/conclusion.go` 将 `data_analysis` 纳入数据工具；复用 `infrastructure/config` 的 MCP endpoint 配置与 `infrastructure/mcp.MCPClient`。
- **链路 / 配置**：Text2SQL/数据分析 Agent 的工具集与提示词更新；需保证 Python MCP（http）与 Go 配置端口一致。
- **测试**：Python 单测 + MCP 集成测试；Go 工具单测（mock HTTP）+ 对接真实 MCP 的集成测试 + Text2SQL→分析→结论 E2E。
- **无破坏性变更**：纯新增能力与接线，既有工具与 analytics 计算行为不变。
