// Package critic provides Critic implementations for reflection.
package critic

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"cognida/internal/model/agent/reflection"
)

// ========================================
// LLMCritic 基于 LLM 的评估器
// ========================================

// LLMCritic 使用 LLM 进行输出质量评估
type LLMCritic struct {
	llm            model.BaseChatModel
	dimensions     []reflection.DimensionConfig
	promptTemplate string
}

// NewLLMCritic 创建 LLM Critic
func NewLLMCritic(llm model.BaseChatModel, dimensions []reflection.DimensionConfig) *LLMCritic {
	return &LLMCritic{
		llm:        llm,
		dimensions: dimensions,
		promptTemplate: defaultPromptTemplate(),
	}
}

// Evaluate 评估输出质量
func (c *LLMCritic) Evaluate(ctx context.Context, task, output string) (*reflection.CritiqueResult, error) {
	prompt := c.buildPrompt(task, output)

	messages := []*schema.Message{
		schema.UserMessage(prompt),
	}

	resp, err := c.llm.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", reflection.ErrCriticEvaluationFailed, err)
	}

	result := &reflection.CritiqueResult{}
	if err := json.Unmarshal([]byte(resp.Content), result); err != nil {
		// 解析失败时返回默认结果
		return c.fallbackResult(output), nil
	}

	return result, nil
}

// ShouldRefine 判断是否需要改进
func (c *LLMCritic) ShouldRefine(result *reflection.CritiqueResult) bool {
	// 总分低于阈值
	if result.OverallScore < 0.7 {
		return true
	}

	// 任一维度低于及格线
	for _, dim := range c.dimensions {
		if score, ok := result.Dimensions[dim.Name]; ok {
			if score.Score < dim.PassThreshold {
				return true
			}
		}
	}

	// 有明确问题
	if len(result.Issues) > 0 {
		return true
	}

	return false
}

// buildPrompt 构建评估提示词
func (c *LLMCritic) buildPrompt(task, output string) string {
	dimDesc := c.formatDimensions()

	return fmt.Sprintf(c.promptTemplate, task, output, dimDesc)
}

// formatDimensions 格式化维度描述
func (c *LLMCritic) formatDimensions() string {
	var desc string
	for _, dim := range c.dimensions {
		desc += fmt.Sprintf("- %s: 及格线 %.2f\n", dim.Name, dim.PassThreshold)
	}
	return desc
}

// fallbackResult 当 LLM 解析失败时返回的默认结果
func (c *LLMCritic) fallbackResult(output string) *reflection.CritiqueResult {
	return &reflection.CritiqueResult{
		OverallScore: 0.5,
		Dimensions:   make(map[string]reflection.DimensionScore),
		Issues:      []string{"无法解析评估结果"},
		ShouldRefine: false,
	}
}

// defaultPromptTemplate 返回默认提示词模板
func defaultPromptTemplate() string {
	return `请评估以下回答的质量：

任务：%s

回答：
%s

请按照以下维度评估（每项 0-1 分）：
%s

请以 JSON 格式返回评估结果：
{
    "overall_score": 总体评分 (0-1),
    "dimensions": {
        "factual": {"score": 分数, "feedback": "反馈"},
        "logic": {"score": 分数, "feedback": "反馈"},
        "completeness": {"score": 分数, "feedback": "反馈"},
        "clarity": {"score": 分数, "feedback": "反馈"}
    },
    "issues": ["问题1", "问题2"],
    "suggestions": ["建议1", "建议2"],
    "should_refine": true/false
}`
}
