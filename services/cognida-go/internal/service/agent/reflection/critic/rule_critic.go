// Package critic provides Critic implementations for reflection.
package critic

import (
	"context"
	"strings"

	"cognida/internal/model/agent/reflection"
)

// ========================================
// RuleCritic 基于规则的评估器
// ========================================

// RuleCritic 使用预定义规则进行输出质量评估，零 LLM 成本
type RuleCritic struct {
	rules []Rule
}

// Rule 评估规则
type Rule interface {
	// Evaluate 评估输出，返回 (分数, 反馈)
	Evaluate(task, output string) (float64, string)
	// Name 返回规则名称
	Name() string
}

// NewRuleCritic 创建 Rule Critic
func NewRuleCritic(rules []Rule) *RuleCritic {
	if len(rules) == 0 {
		rules = DefaultRules()
	}
	return &RuleCritic{rules: rules}
}

// Evaluate 评估输出质量
func (c *RuleCritic) Evaluate(ctx context.Context, task, output string) (*reflection.CritiqueResult, error) {
	result := &reflection.CritiqueResult{
		Dimensions: make(map[string]reflection.DimensionScore),
		Issues:     []string{},
	}

	totalScore := 0.0
	totalWeight := 0.0

	for _, rule := range c.rules {
		score, feedback := rule.Evaluate(task, output)

		if score < 0.7 {
			result.Issues = append(result.Issues, feedback)
		}

		result.Dimensions[rule.Name()] = reflection.DimensionScore{
			Score:    score,
			Feedback: feedback,
		}

		// 简化：假设每个规则权重相同
		totalScore += score
		totalWeight += 1.0
	}

	if totalWeight > 0 {
		result.OverallScore = totalScore / totalWeight
	}

	result.ShouldRefine = len(result.Issues) > 0

	return result, nil
}

// ShouldRefine 判断是否需要改进
func (c *RuleCritic) ShouldRefine(result *reflection.CritiqueResult) bool {
	return result.ShouldRefine
}

// ========================================
// 默认规则实现
// ========================================

// DefaultRules 返回默认规则集
func DefaultRules() []Rule {
	return []Rule{
		&LengthRule{},
		&CompletenessRule{},
		&FormatRule{},
	}
}

// LengthRule 长度规则
type LengthRule struct{}

func (r *LengthRule) Name() string { return "length" }

func (r *LengthRule) Evaluate(task, output string) (float64, string) {
	length := len([]rune(output))
	switch {
	case length < 20:
		return 0.3, "回答过短"
	case length < 50:
		return 0.6, "回答较短"
	case length > 2000:
		return 0.7, "回答过长"
	default:
		return 1.0, "长度适当"
	}
}

// CompletenessRule 完整性规则
type CompletenessRule struct{}

func (r *CompletenessRule) Name() string { return "completeness" }

func (r *CompletenessRule) Evaluate(task, output string) (float64, string) {
	// 检查是否为空
	if strings.TrimSpace(output) == "" {
		return 0.0, "回答为空"
	}

	// 检查是否只是"不知道"等敷衍回答
	responses := []string{"不知道", "不清楚", "无法回答", "sorry"}
	for _, resp := range responses {
		if strings.Contains(strings.ToLower(output), resp) && len([]rune(output)) < 50 {
			return 0.4, "回答不完整"
		}
	}

	return 1.0, "回答完整"
}

// FormatRule 格式规则
type FormatRule struct{}

func (r *FormatRule) Name() string { return "format" }

func (r *FormatRule) Evaluate(task, output string) (float64, string) {
	// 检查是否有明显的格式问题
	if strings.HasPrefix(output, "```") && !strings.HasSuffix(output, "```") {
		return 0.6, "代码块格式不完整"
	}

	// 检查是否有过多重复字符（同一字符连续出现 6 次及以上）
	if hasExcessiveRepeat(output, 6) {
		return 0.5, "存在重复字符"
	}

	return 1.0, "格式正常"
}

// hasExcessiveRepeat 判断字符串中是否存在同一字符连续出现 minRun 次及以上。
// Go 的 RE2 正则不支持反向引用（\1），故用一次线性扫描替代原正则实现。
func hasExcessiveRepeat(s string, minRun int) bool {
	if minRun <= 1 {
		return s != ""
	}
	run := 0
	var prev rune
	for i, r := range s {
		if i > 0 && r == prev {
			run++
			if run >= minRun {
				return true
			}
		} else {
			run = 1
			prev = r
		}
	}
	return false
}
