package tools

import (
	"reflect"
	"sort"
	"testing"
)

// 跨语言对照锚点〔M13 P1〕：7 个 analytics MCP 工具名的字面量期望值。
// 此集合必须与 Python 侧锁定测试
// services/cognida-python/tests/tools/test_tool_name_contract.py 的 EXPECTED_TOOL_NAMES
// 逐字一致——两侧各自写死同一份字面量，任一侧改 wire 值都会被本测试或对侧测试打红。
var expectedMCPToolNames = map[string]struct{}{
	"data_describe":    {},
	"data_trend":       {},
	"data_anomaly":     {},
	"data_correlation": {},
	"data_insight":     {},
	"data_comparison":  {},
	"data_attribution": {},
}

// TestMCPToolNameConstantsMatchExpected 锁定 7 个 MCP 工具名常量集合 == 期望字面量集合。
func TestMCPToolNameConstantsMatchExpected(t *testing.T) {
	got := map[string]struct{}{
		string(MCPToolDataDescribe):    {},
		string(MCPToolDataTrend):       {},
		string(MCPToolDataAnomaly):     {},
		string(MCPToolDataCorrelation): {},
		string(MCPToolDataInsight):     {},
		string(MCPToolDataComparison):  {},
		string(MCPToolDataAttribution): {},
	}
	if !reflect.DeepEqual(got, expectedMCPToolNames) {
		t.Fatalf("MCP 工具名常量集合漂移\n got=%v\nwant=%v", keys(got), keys(expectedMCPToolNames))
	}
}

// 跨语言对照锚点〔M13 P2〕：data_analysis 入参 analysis_type 的 8 个合法取值。
var expectedAnalysisTypes = map[string]struct{}{
	"trend":       {},
	"comparison":  {},
	"attribution": {},
	"report":      {},
	"describe":    {},
	"anomaly":     {},
	"correlation": {},
	"insight":     {},
}

// TestAnalysisTypeSingleSourceConsistent 锁定 P2 三处（map key / schema Enum / 白名单）
// 均派生自同一真源 analysisTypeMappings，集合一致且等于期望字面量。
func TestAnalysisTypeSingleSourceConsistent(t *testing.T) {
	// map key（白名单）集合
	mapKeys := make(map[string]struct{}, len(analysisTypeToMCPTool))
	for k := range analysisTypeToMCPTool {
		mapKeys[k] = struct{}{}
	}
	// schema Enum 集合
	enumSet := make(map[string]struct{}, len(analysisTypeEnum))
	for _, e := range analysisTypeEnum {
		enumSet[e] = struct{}{}
	}
	// 单一真源集合
	srcSet := make(map[string]struct{}, len(analysisTypeMappings))
	for _, m := range analysisTypeMappings {
		srcSet[string(m.Type)] = struct{}{}
	}

	if !reflect.DeepEqual(mapKeys, expectedAnalysisTypes) {
		t.Errorf("map 白名单键集合漂移: got=%v want=%v", keys(mapKeys), keys(expectedAnalysisTypes))
	}
	if !reflect.DeepEqual(enumSet, expectedAnalysisTypes) {
		t.Errorf("schema Enum 集合漂移: got=%v want=%v", keys(enumSet), keys(expectedAnalysisTypes))
	}
	if !reflect.DeepEqual(srcSet, expectedAnalysisTypes) {
		t.Errorf("单一真源集合漂移: got=%v want=%v", keys(srcSet), keys(expectedAnalysisTypes))
	}

	// Enum 无重复且长度与真源一致（防重复项混入）
	if len(analysisTypeEnum) != len(analysisTypeMappings) {
		t.Errorf("Enum 长度(%d) != 真源长度(%d)，疑有重复", len(analysisTypeEnum), len(analysisTypeMappings))
	}
}

// TestAnalysisTypeMappingsTargetsAreKnownTools 锁定映射目标（MCP 工具名）落在 7 个已知工具内，
// 且 report 与 insight 复用同一综合洞察引擎（data_insight）。
func TestAnalysisTypeMappingsTargetsAreKnownTools(t *testing.T) {
	for _, m := range analysisTypeMappings {
		if _, ok := expectedMCPToolNames[string(m.Tool)]; !ok {
			t.Errorf("analysis_type %q 映射到未知 MCP 工具 %q", m.Type, m.Tool)
		}
	}
	if analysisTypeToMCPTool[string(AnalysisReport)] != analysisTypeToMCPTool[string(AnalysisInsight)] {
		t.Errorf("report 应复用 insight 引擎(data_insight)，实得 report=%q insight=%q",
			analysisTypeToMCPTool[string(AnalysisReport)], analysisTypeToMCPTool[string(AnalysisInsight)])
	}
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
