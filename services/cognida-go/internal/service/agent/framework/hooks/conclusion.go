package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Conclusion 数据结论结构
type Conclusion struct {
	Summary         string   `json:"summary"`         // 数据摘要（1-2句话）
	KeyFindings     []string `json:"key_findings"`    // 关键发现列表（3-5条）
	Insights        []string `json:"insights"`        // 深度洞察列表
	Recommendations []string `json:"recommendations"` // 行动建议列表
	Confidence      float64  `json:"confidence"`      // 置信度（0-1）
	DataSummary     string   `json:"data_summary"`    // 数据概览
}

// ToolCallInfo 表示工具调用信息
type ToolCallInfo struct {
	Name    string                 `json:"name"`
	Input   map[string]interface{} `json:"input"`
	Output  string                 `json:"output"`
	Success bool                   `json:"success"`
}

// ConclusionGenerator 数据结论生成器
type ConclusionGenerator struct {
	*BaseHook
	dataTools map[string]bool // 数据工具名称集合
}

// NewConclusionGenerator 创建 ConclusionGenerator 实例
//
// 默认即把 data_analysis 视为数据工具：其量化分析输出（描述统计/趋势/异常/
// 相关性/洞察）会作为 ToolCallInfo.Output 喂入结论生成，确保结论有计算依据。
// 其余数据工具仍可经 AddDataTools 追加。
func NewConclusionGenerator(llm LLMClient) *ConclusionGenerator {
	return &ConclusionGenerator{
		BaseHook: NewBaseHook("conclusion_generator", llm),
		dataTools: map[string]bool{
			"data_analysis": true,
		},
	}
}

// hasDataToolCall 检测响应中是否调用了数据工具
func (g *ConclusionGenerator) hasDataToolCall(resp interface{}) bool {
	// 尝试从响应中提取 ToolCalls
	var toolCalls []any

	switch r := resp.(type) {
	case map[string]any:
		if tc, ok := r["tool_calls"].([]any); ok {
			toolCalls = tc
		} else if tc, ok := r["ToolCalls"].([]any); ok {
			toolCalls = tc
		}
	default:
		// Unknown type, no tool calls
		return false
	}

	if len(toolCalls) == 0 || len(g.dataTools) == 0 {
		return false
	}

	for _, tc := range toolCalls {
		if tcMap, ok := tc.(map[string]any); ok {
			if name, ok := tcMap["name"].(string); ok {
				if g.dataTools[name] {
					return true
				}
			}
		}
	}
	return false
}

// extractToolCalls 提取工具调用信息
func (g *ConclusionGenerator) extractToolCalls(resp interface{}) []*ToolCallInfo {
	var toolCalls []*ToolCallInfo

	var rawToolCalls []any
	switch r := resp.(type) {
	case map[string]any:
		if tc, ok := r["tool_calls"].([]any); ok {
			rawToolCalls = tc
		} else if tc, ok := r["ToolCalls"].([]any); ok {
			rawToolCalls = tc
		}
	default:
		// Unknown type, no tool calls
		return nil
	}

	for _, tc := range rawToolCalls {
		tcMap, ok := tc.(map[string]any)
		if !ok {
			continue
		}

		info := &ToolCallInfo{}
		if name, ok := tcMap["name"].(string); ok {
			info.Name = name
		}
		if input, ok := tcMap["input"].(map[string]any); ok {
			// Convert map[string]any to map[string]interface{}
			info.Input = make(map[string]interface{})
			for k, v := range input {
				info.Input[k] = v
			}
		}
		if output, ok := tcMap["output"].(string); ok {
			info.Output = output
		}
		if _, ok := tcMap["error"]; ok {
			info.Success = false
		} else {
			info.Success = true
		}

		toolCalls = append(toolCalls, info)
	}

	return toolCalls
}

// buildDataSummary 构建数据摘要
func (g *ConclusionGenerator) buildDataSummary(toolCalls []*ToolCallInfo) string {
	var summary strings.Builder

	summary.WriteString("## 数据工具调用结果\n\n")

	for _, tc := range toolCalls {
		if !g.dataTools[tc.Name] {
			continue
		}

		summary.WriteString(fmt.Sprintf("### 工具: %s\n", tc.Name))

		if tc.Success {
			summary.WriteString("**状态**: 成功\n")
			// 截取输出，避免过长
			output := tc.Output
			if len(output) > 2000 {
				output = output[:2000] + "\n...(已截断)"
			}
			summary.WriteString(fmt.Sprintf("**输出**:\n```\n%s\n```\n", output))
		} else {
			summary.WriteString("**状态**: 失败\n")
			summary.WriteString(fmt.Sprintf("**输出**: %s\n", tc.Output))
		}

		summary.WriteString("\n")
	}

	return summary.String()
}

// getSystemPrompt 获取系统提示
func (g *ConclusionGenerator) getSystemPrompt() string {
	return `你是一个专业的数据分析师，负责分析数据查询结果并生成结构化结论。

请分析提供的数据查询结果，返回 JSON 格式的分析结论，包含以下字段：

1. summary: 数据摘要（1-2句话概括数据整体情况）
2. key_findings: 关键发现列表（3-5条具体的发现）
3. insights: 深度洞察列表（数据背后的趋势、异常、关联等）
4. recommendations: 行动建议列表（基于分析结果的具体建议）
5. confidence: 置信度（0-1的浮点数，表示分析的可靠性）
6. data_summary: 数据概览（数据范围、规模等基本信息）

**注意**：
- 保持分析客观，基于数据事实
- 如果数据查询失败，基于可用信息进行分析
- 置信度应反映数据完整性和分析确定性
- 使用简洁、专业的语言`
}

// buildConclusionPrompt 构建分析提示
func (g *ConclusionGenerator) buildConclusionPrompt(dataSummary string) string {
	return fmt.Sprintf(`请分析以下数据查询结果：

%s

请以 JSON 格式返回分析结论。`, dataSummary)
}

// generateConclusion 生成结论
func (g *ConclusionGenerator) generateConclusion(ctx context.Context, dataSummary string) (*Conclusion, error) {
	prompt := g.buildConclusionPrompt(dataSummary)

	messages := []Message{
		{Role: "system", Content: g.getSystemPrompt()},
		{Role: "user", Content: prompt},
	}

	response, err := g.llm.Chat(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// 尝试解析 JSON
	var conclusion Conclusion
	if err := json.Unmarshal([]byte(response), &conclusion); err != nil {
		// JSON 解析失败，降级处理
		return g.fallbackConclusion(response), nil
	}

	return &conclusion, nil
}

// fallbackConclusion 降级处理：生成简单结论
func (g *ConclusionGenerator) fallbackConclusion(llmOutput string) *Conclusion {
	return &Conclusion{
		Summary:     "数据已查询完成",
		KeyFindings: []string{fmt.Sprintf("分析结果: %s", truncateString(llmOutput, 200))},
		Insights:    []string{"详细分析建议人工审核"},
		Confidence:  0.5,
		DataSummary: llmOutput,
	}
}

// formatConclusion 格式化结论文本
func (g *ConclusionGenerator) formatConclusion(conclusion *Conclusion) string {
	var builder strings.Builder

	// 摘要
	if conclusion.Summary != "" {
		builder.WriteString(fmt.Sprintf("**摘要**: %s\n\n", conclusion.Summary))
	}

	// 关键发现
	if len(conclusion.KeyFindings) > 0 {
		builder.WriteString("**关键发现**:\n")
		for _, finding := range conclusion.KeyFindings {
			builder.WriteString(fmt.Sprintf("- %s\n", finding))
		}
		builder.WriteString("\n")
	}

	// 深度洞察
	if len(conclusion.Insights) > 0 {
		builder.WriteString("**深度洞察**:\n")
		for _, insight := range conclusion.Insights {
			builder.WriteString(fmt.Sprintf("- %s\n", insight))
		}
		builder.WriteString("\n")
	}

	// 行动建议
	if len(conclusion.Recommendations) > 0 {
		builder.WriteString("**行动建议**:\n")
		for _, rec := range conclusion.Recommendations {
			builder.WriteString(fmt.Sprintf("- %s\n", rec))
		}
	}

	return builder.String()
}

// process 处理响应并附加结论
func (g *ConclusionGenerator) process(ctx context.Context, resp interface{}) error {
	// 检测数据工具调用
	if !g.hasDataToolCall(resp) {
		return nil
	}

	// 提取工具调用
	toolCalls := g.extractToolCalls(resp)

	// 构建数据摘要
	dataSummary := g.buildDataSummary(toolCalls)

	// 生成结论
	conclusion, err := g.generateConclusion(ctx, dataSummary)
	if err != nil {
		// 生成失败不影响主流程
		return fmt.Errorf("failed to generate conclusion: %w", err)
	}

	// 附加结论到响应
	g.attachConclusion(resp, conclusion)

	return nil
}

// attachConclusion 附加结论到响应
func (g *ConclusionGenerator) attachConclusion(resp interface{}, conclusion *Conclusion) {
	// 根据响应类型附加结论
	switch r := resp.(type) {
	case map[string]any:
		// 附加到 Metadata
		if r["metadata"] == nil {
			r["metadata"] = make(map[string]any)
		}
		if metadata, ok := r["metadata"].(map[string]any); ok {
			metadata["conclusion"] = conclusion
		}

		// 附加到 Content
		if content, ok := r["content"].(string); ok {
			conclusionText := fmt.Sprintf("\n\n---\n\n## 💡 数据分析结论\n\n%s", g.formatConclusion(conclusion))
			r["content"] = content + conclusionText
		}
	default:
		// Unknown type, nothing to attach
		return
	}
}

// Hook 返回 AfterHook 函数（兼容通用接口）
func (g *ConclusionGenerator) Hook() func(ctx context.Context, resp interface{}) error {
	return func(ctx context.Context, resp interface{}) error {
		return g.SafeExecute(ctx, func() error {
			return g.process(ctx, resp)
		})
	}
}

// AddDataTools 添加自定义数据工具
func (g *ConclusionGenerator) AddDataTools(tools ...string) *ConclusionGenerator {
	for _, tool := range tools {
		g.dataTools[tool] = true
	}
	return g
}

// truncateString 截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
