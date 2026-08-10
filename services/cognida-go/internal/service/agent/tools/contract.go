package tools

// ========================================
// Go↔Python 跨服务契约单一真源〔M13 P1+P2〕
// ========================================
//
// 本文件把原先散落在 data_analysis.go 里的两类字面量收敛为「单一真源 + 类型化」：
//   - P1：Go→Python 经 MCP 调用的 analytics 工具名（7 个），Python 侧对照真源见
//     services/cognida-python/tools/analytics/contract.py 的 ToolName(StrEnum)。
//   - P2：data_analysis 工具入参 analysis_type 枚举，原先在 map key / schema Enum /
//     白名单校验三处重复，现统一由 analysisTypeMappings 派生。
//
// wire 值（字符串字面量）一律保持不变，仅做类型化与去重；跨语言锁定测试见
// contract_test.go（Go）与 tests/tools/test_tool_name_contract.py（Python）。

// MCPToolName 是 Python analytics 引擎在 MCP 侧暴露的工具名（wire 值）。
type MCPToolName string

// 7 个 analytics MCP 工具名常量。wire 值必须与 Python 侧
// tools/analytics/*_tool.py 的 name 属性逐字一致（由两侧锁定测试守护）。
const (
	MCPToolDataDescribe    MCPToolName = "data_describe"
	MCPToolDataTrend       MCPToolName = "data_trend"
	MCPToolDataAnomaly     MCPToolName = "data_anomaly"
	MCPToolDataCorrelation MCPToolName = "data_correlation"
	MCPToolDataInsight     MCPToolName = "data_insight"
	MCPToolDataComparison  MCPToolName = "data_comparison"
	MCPToolDataAttribution MCPToolName = "data_attribution"
)

// AnalysisType 是 data_analysis 工具入参 analysis_type 的取值（wire 值）。
type AnalysisType string

const (
	AnalysisTrend       AnalysisType = "trend"
	AnalysisComparison  AnalysisType = "comparison"
	AnalysisAttribution AnalysisType = "attribution"
	AnalysisReport      AnalysisType = "report"
	AnalysisDescribe    AnalysisType = "describe"
	AnalysisAnomaly     AnalysisType = "anomaly"
	AnalysisCorrelation AnalysisType = "correlation"
	AnalysisInsight     AnalysisType = "insight"
)

// analysisTypeMappings 是 analysis_type → MCP 工具名的**唯一有序真源**。
// 顺序即 schema Enum 的展示顺序；map 白名单与 Enum 数组均由此派生，杜绝三处漂移。
//
// Phase 3.5（openspec: data-agent-evolution D9）把 data_analysis 分化为命名能力：
// 趋势（trend）、对比（comparison）、归因/根因（attribution）、报告解读（report，
// 复用综合洞察引擎 data_insight）；describe/anomaly/correlation/insight 为底层原子分析保留。
var analysisTypeMappings = []struct {
	Type AnalysisType
	Tool MCPToolName
}{
	{AnalysisTrend, MCPToolDataTrend},
	{AnalysisComparison, MCPToolDataComparison},
	{AnalysisAttribution, MCPToolDataAttribution},
	{AnalysisReport, MCPToolDataInsight},
	{AnalysisDescribe, MCPToolDataDescribe},
	{AnalysisAnomaly, MCPToolDataAnomaly},
	{AnalysisCorrelation, MCPToolDataCorrelation},
	{AnalysisInsight, MCPToolDataInsight},
}

// analysisTypeToMCPTool 由单一真源派生的 analysis_type→MCP 工具名映射，
// 同时充当白名单（键集即合法 analysis_type 全集）。
var analysisTypeToMCPTool = func() map[string]MCPToolName {
	m := make(map[string]MCPToolName, len(analysisTypeMappings))
	for _, e := range analysisTypeMappings {
		m[string(e.Type)] = e.Tool
	}
	return m
}()

// analysisTypeEnum 由单一真源按序派生的 schema Enum 数组（供 Info 的 ParameterInfo.Enum）。
var analysisTypeEnum = func() []string {
	out := make([]string, len(analysisTypeMappings))
	for i, e := range analysisTypeMappings {
		out[i] = string(e.Type)
	}
	return out
}()
