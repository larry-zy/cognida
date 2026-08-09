## 1. Python：注册引导（修复空注册缺口）

- [x] 1.1 新增 `cognida-python/tools/bootstrap.py`，实现幂等的 `register_default_tools(registry)`（延迟 import analytics 工具）
- [x] 1.2 在 `cognida-python/mcp/server.py` 初始化拿到 `self._registry` 后调用 `register_default_tools`
- [x] 1.3 单测：注册后 registry 含全部 analytics 工具，重复调用不报错且无重复

## 2. Python：Analytics MCP 工具

- [x] 2.1 新增 `cognida-python/tools/analytics/` 包与公共数据转换（JSON records ↔ DataFrame，复用 `validation.py` 的 `sanitize_dataframe`）
- [x] 2.2 `data_describe` 工具，包装 `DescriptiveStats`/`DistributionAnalysis`，定义 `input_schema`
- [x] 2.3 `data_trend` 工具，包装 `LinearTrendAnalyzer`+forecast/`GrowthRate`，要求 `time_col`/`value_col`
- [x] 2.4 `data_anomaly` 工具，包装 `AnomalyInsightFinder`（IQR/zscore），要求 `value_col`
- [x] 2.5 `data_correlation` 工具，包装 `CorrelationAnalysis`，要求 ≥2 数值列
- [x] 2.6 `data_insight` 工具，包装 `InsightGenerator`，返回 insights + recommendations
- [x] 2.7 空/非法数据返回结构化错误而非异常；结果保证 JSON-serializable
- [x] 2.8 单测：每个工具的正常/边界/错误路径

## 3. Python：HTTP MCP 暴露与验证

- [x] 3.1 确认 MCP server 以 http 模式常驻并读取端口配置
- [x] 3.2 集成测试：`tools/list` 返回五个工具（含 inputSchema）
- [x] 3.3 集成测试：对每个工具发 `tools/call`（http JSON-RPC）并校验结果结构

## 4. Go：data_analysis Agent 工具

- [x] 4.1 新增 `cognida-go/internal/service/agent/tools/data_analysis.go`，实现 eino `tool.InvokableTool`（参考 `sql_execute.go`）
- [x] 4.2 `Info()` 定义参数：`analysis_type`(describe/trend/anomaly/correlation/insight)、`data`、`options`
- [x] 4.3 `InvokableRun()` 按 `analysis_type` 路由到 MCP 工具名，复用 `infrastructure/mcp.MCPClient.Invoke`（经 `MCPInvoker` 接口注入，避免 service→infrastructure 直接依赖）
- [x] 4.4 MCP endpoint 在组合根从 `infrastructure/config` 读取（与 skill 同源）后注入；MCP 失败返回非致命错误结果
- [x] 4.5 在 `cognida-go/internal/service/agent/tools/init.go` 注册 `data_analysis`
- [x] 4.6 单测：mock MCPInvoker 验证 Info/路由/错误处理

## 5. Go：结论闭环接线

- [x] 5.1 `framework/hooks/conclusion.go`：将 `data_analysis` 加入 `ConclusionGenerator.dataTools`（默认种子）
- [x] 5.2 把 `data_analysis` 分析输出作为 `ToolCallInfo.Output` 喂入结论生成（经通用 buildDataSummary 路径）
- [x] 5.3 单测：检测到 `data_analysis` 触发结论生成，结论基于分析输出

## 6. 链路接入与编排

- [x] 6.1 Text2SQL/数据分析 Agent 工具集与系统提示词加入 `data_analysis`，引导「取数→分析→结论」
- [ ] 6.2（可选）用 `orchestration/sequential.go` 固化 `get_schema→sql_execute→data_analysis→conclusion` 兜底（暂跳过：LLM 已由提示词引导按需调用，固化编排留待后续）
- [x] 6.3 约定并实现行集最大行数上限（截断 + 告警）：Python `_common.truncate`(MAX_ROWS=DEFAULT_MAX_ROWS)，结果带 `truncated` 标志告警

## 7. 测试与验收

- [x] 7.1 Python：analytics + MCP 集成测试通过（`pytest tests/tools/ -q` → 35 passed；其余 quality/evaluation 模块为既有 `tools.evaluation` 缺失，与本变更无关）
- [x] 7.2 Go：相关包 `go test` 全绿（tools/hooks/text2sql/mcp）；新增跨栈 `-tags=integration` e2e 通过；仅 `infrastructure/grpc/docreader` 因需外部 gRPC(:50051) 既有失败，与本变更无关
- [x] 7.3 E2E：取数行集 → data_analysis → 真实 MCPClient → 真实 Python MCP server → 五类分析全通过且 insight 产出 recommendations（NL→SQL 前端与 LLM 叙述需 live LLM/DB 凭证，本环境未跑）
- [x] 7.4 代码审查：项目未安装 code-review skill，改为人工审查；发现并修复 gofmt 问题，`go vet` 通过，无命名冲突，符合 service↛infrastructure 依赖方向
- [x] 7.5 收尾：终止本次启动的所有服务进程（MUST）：已 pkill Python MCP server，端口 8899 释放
