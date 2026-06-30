// Package tools 提供 data_analysis 工具：通过 MCP 调用 Python analytics 引擎
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	modeltools "link/internal/model/agent/tools"
)

// ========================================
// data_analysis 工具 - 量化数据分析
// ========================================

// analysisTypeToMCPTool 将 analysis_type 映射到 Python 侧 MCP 工具名
var analysisTypeToMCPTool = map[string]string{
	"describe":    "data_describe",
	"trend":       "data_trend",
	"anomaly":     "data_anomaly",
	"correlation": "data_correlation",
	"insight":     "data_insight",
}

// MCPInvoker 抽象 MCP 调用能力，便于注入真实客户端或单测 mock。
//
// 该接口仅依赖 model 层类型（SkillInvokeResult），因此 service 工具层
// 无需直接 import infrastructure/mcp，符合 handler → service → model ←
// infrastructure 的依赖方向。真实实现由 infrastructure/mcp.MCPClient 提供，
// 并在组合根（cmd/server）通过 InitDataAnalysisTool 注入。
type MCPInvoker interface {
	Invoke(ctx interface{}, skillName string, params map[string]interface{}) (*modeltools.SkillInvokeResult, error)
}

// dataAnalysisInvoker 注入的 MCP 调用器；未注入时工具返回非致命错误结果。
var dataAnalysisInvoker MCPInvoker

// InitDataAnalysisTool 注入 MCP 调用器（在组合根调用，与 InitSQLExecuteTool 对称）。
func InitDataAnalysisTool(invoker MCPInvoker) {
	dataAnalysisInvoker = invoker
}

// dataAnalysisTool data_analysis 工具实现
//
// 取数（sql_execute）之后，把行集交给 Python analytics 引擎做真正的
// 描述统计 / 趋势 / 异常 / 相关性 / 综合洞察，产出有计算依据的结论。
// 复用既有 MCP 通道（注入的 MCPInvoker → infrastructure/mcp.MCPClient.Invoke）。
type dataAnalysisTool struct{}

// NewDataAnalysisTool 创建 data_analysis 工具
func NewDataAnalysisTool() (tool.InvokableTool, error) {
	return &dataAnalysisTool{}, nil
}

// Info 返回工具信息（含参数 schema）
func (t *dataAnalysisTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "data_analysis",
		Desc: `对取数得到的行集做量化数据分析，产出有计算依据的结论与行动建议。

适用场景：text2sql / sql_execute 取数之后，需要真正的统计计算而非凭空叙述时调用。

analysis_type 取值：
- describe: 描述统计（均值/中位数/分位数/标准差/偏度峰度 + 正态性）
- trend: 时间序列线性趋势、预测与增长率（环比/同比/CAGR），需 options.value_col
- anomaly: 异常点检测（IQR/zscore），需 options.value_col
- correlation: 多数值列相关性矩阵与显著相关对（需 ≥2 数值列）
- insight: 综合洞察，自动汇总趋势/异常/相关性并给出 recommendations

data 为行集，形如 {"columns": [...], "rows": [[...], ...]}（可直接传 sql_execute 的输出）。`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"analysis_type": {
				Type:     schema.String,
				Desc:     "分析类型：describe / trend / anomaly / correlation / insight",
				Required: true,
				Enum:     []string{"describe", "trend", "anomaly", "correlation", "insight"},
			},
			"data": {
				Type:     schema.Object,
				Desc:     "要分析的行集，{columns, rows} 或 records 数组（通常为 sql_execute 的输出）",
				Required: true,
			},
			"options": {
				Type:     schema.Object,
				Desc:     "（可选）分析参数，如 value_col、time_col、columns、method、threshold、forecast_steps 等",
				Required: false,
			},
		}),
	}, nil
}

// InvokableRun 执行数据分析
func (t *dataAnalysisTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	// 解析参数
	var arguments map[string]interface{}
	if err := json.Unmarshal([]byte(argumentsInJSON), &arguments); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	// analysis_type 必填且需在白名单内
	analysisType, _ := arguments["analysis_type"].(string)
	if analysisType == "" {
		return "", fmt.Errorf("missing required parameter: analysis_type")
	}
	mcpTool, ok := analysisTypeToMCPTool[analysisType]
	if !ok {
		// 未知类型属调用方错误，返回非致命错误结果供 Agent 纠正
		return failResult(analysisType, fmt.Sprintf("不支持的 analysis_type: %s", analysisType)), nil
	}

	// data 必填
	data, ok := arguments["data"]
	if !ok || data == nil {
		return "", fmt.Errorf("missing required parameter: data")
	}

	// MCP 调用器需已注入
	if dataAnalysisInvoker == nil {
		return failResult(analysisType, "MCP 调用器未初始化（请在组合根调用 InitDataAnalysisTool）"), nil
	}

	// 组装 MCP 工具参数：data + 展开的 options
	params := map[string]interface{}{"data": data}
	if rawOpts, ok := arguments["options"].(map[string]interface{}); ok {
		for k, v := range rawOpts {
			params[k] = v
		}
	}

	// 通过既有 MCP 通道调用 Python analytics 工具
	result, err := dataAnalysisInvoker.Invoke(ctx, mcpTool, params)
	if err != nil {
		return failResult(analysisType, fmt.Sprintf("分析调用失败: %v", err)), nil
	}

	// 透传结果，附带 analysis_type 便于结论生成定位
	out := map[string]interface{}{
		"analysis_type": analysisType,
		"success":       result.Success,
	}
	if result.Data != nil {
		out["data"] = result.Data
	}
	if result.Error != "" {
		out["error"] = result.Error
	}

	resultBytes, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}
	return string(resultBytes), nil
}

// failResult 构造非致命错误结果（JSON 字符串），交给 Agent 推理
func failResult(analysisType, msg string) string {
	out := map[string]interface{}{
		"analysis_type": analysisType,
		"success":       false,
		"error":         msg,
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	return string(b)
}
