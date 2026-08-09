// Package tools provides guardrail service implementation
package tools

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/model"

	domainguardrail "cognida/internal/model/guardrail"
	"cognida/internal/service/agent/tools/filter"
)

// ========================================
// Guardrail Service 护栏服务
// ========================================

// GuardrailServiceImpl 安全护栏服务实现
type GuardrailServiceImpl struct {
	inputFilter  domainguardrail.InputFilter
	outputFilter domainguardrail.OutputFilter
	jailbreakDetector domainguardrail.JailbreakDetector
}

// NewGuardrailService 创建安全护栏服务
func NewGuardrailService(llm model.ChatModel) domainguardrail.GuardrailService {
	return &GuardrailServiceImpl{
		inputFilter:  filter.NewInputFilter(llm),
		outputFilter: filter.NewOutputFilter(llm),
		jailbreakDetector: filter.NewJailbreakDetector(llm),
	}
}

// CheckInput 检查输入
func (s *GuardrailServiceImpl) CheckInput(ctx context.Context, input string, opts *domainguardrail.FilterOptions) (*domainguardrail.FilterResult, error) {
	result, err := s.inputFilter.Validate(ctx, input, opts)
	if err != nil {
		return nil, fmt.Errorf("input validation failed: %w", err)
	}

	// 添加详细的状态信息
	if !result.IsSafe {
		result.Metadata["status"] = "rejected"
		result.Metadata["reason"] = "unsafe_input_detected"
	} else {
		result.Metadata["status"] = "approved"
	}

	return result, nil
}

// CheckOutput 检查输出
func (s *GuardrailServiceImpl) CheckOutput(ctx context.Context, output string, input string, outputOpts *domainguardrail.OutputFilterOptions) (*domainguardrail.OutputFilterResult, error) {
	result, err := s.outputFilter.Validate(ctx, output, input, outputOpts)
	if err != nil {
		return nil, fmt.Errorf("output validation failed: %w", err)
	}

	if !result.IsSafe {
		result.Metadata["status"] = "rejected"
		result.Metadata["reason"] = "unsafe_output_detected"
	} else {
		result.Metadata["status"] = "approved"
	}

	return result, nil
}

// CheckJailbreak 检查越狱攻击
func (s *GuardrailServiceImpl) CheckJailbreak(ctx context.Context, input string, opts *domainguardrail.JailbreakOptions) (*domainguardrail.JailbreakResult, error) {
	result, err := s.jailbreakDetector.Detect(ctx, input, opts)
	if err != nil {
		return nil, fmt.Errorf("jailbreak detection failed: %w", err)
	}

	if result.IsJailbreak {
		result.SuggestedAction = "拒绝执行该请求，该输入疑似为越狱攻击"
	}

	return result, nil
}

// SanitizeInput 清理输入
func (s *GuardrailServiceImpl) SanitizeInput(ctx context.Context, input string, opts *domainguardrail.FilterOptions) (string, error) {
	sanitized, err := s.inputFilter.Sanitize(ctx, input, opts)
	if err != nil {
		return "", fmt.Errorf("input sanitization failed: %w", err)
	}

	return sanitized, nil
}

// SanitizeOutput 清理输出
func (s *GuardrailServiceImpl) SanitizeOutput(ctx context.Context, output string, input string, outputOpts *domainguardrail.OutputFilterOptions) (string, error) {
	sanitized, err := s.outputFilter.Sanitize(ctx, output, input, outputOpts)
	if err != nil {
		return "", fmt.Errorf("output sanitization failed: %w", err)
	}

	return sanitized, nil
}

// ========================================
// 组合检查方法
// ========================================

// CheckInputAndOutput 检查输入和输出
func (s *GuardrailServiceImpl) CheckInputAndOutput(ctx context.Context, input string, output string, inputOpts *domainguardrail.FilterOptions, outputOpts *domainguardrail.OutputFilterOptions) (*GuardrailCheckResult, error) {
	result := &GuardrailCheckResult{
		IsSafe: true,
		Metadata: make(map[string]interface{}),
	}

	// 检查输入
	inputResult, err := s.CheckInput(ctx, input, inputOpts)
	if err != nil {
		return nil, err
	}

	result.InputResult = inputResult
	if !inputResult.IsSafe {
		result.IsSafe = false
		result.RejectionReason = "unsafe_input"
	}

	// 检查输出
	outputResult, err := s.CheckOutput(ctx, output, input, outputOpts)
	if err != nil {
		return nil, err
	}

	result.OutputResult = outputResult
	if !outputResult.IsSafe {
		result.IsSafe = false
		if result.RejectionReason == "" {
			result.RejectionReason = "unsafe_output"
		}
	}

	result.OverallRiskScore = (inputResult.RiskScore + outputResult.RiskScore) / 2

	return result, nil
}

// FullInputCheck 完整的输入检查（包含越狱检测）
func (s *GuardrailServiceImpl) FullInputCheck(ctx context.Context, input string, opts *FullCheckOptions) (*FullCheckResult, error) {
	if opts == nil {
		opts = DefaultFullCheckOptions()
	}

	result := &FullCheckResult{
		IsSafe:   true,
		Metadata: make(map[string]interface{}),
		Checks:   make(map[string]CheckStatus),
	}

	// 1. 越狱检测
	if opts.EnableJailbreakCheck {
		jailbreakResult, err := s.CheckJailbreak(ctx, input, opts.JailbreakOptions)
		if err != nil {
			result.Checks["jailbreak"] = CheckStatus{Passed: false, Error: err.Error()}
		} else {
			result.Checks["jailbreak"] = CheckStatus{Passed: !jailbreakResult.IsJailbreak}
			result.JailbreakResult = jailbreakResult
			if jailbreakResult.IsJailbreak {
				result.IsSafe = false
				result.RejectionReason = "jailbreak_detected"
			}
		}
	}

	// 2. 通用输入检查
	if opts.EnableInputCheck {
		inputResult, err := s.CheckInput(ctx, input, opts.InputOptions)
		if err != nil {
			result.Checks["input"] = CheckStatus{Passed: false, Error: err.Error()}
		} else {
			result.Checks["input"] = CheckStatus{Passed: inputResult.IsSafe}
			result.InputResult = inputResult
			if !inputResult.IsSafe {
				result.IsSafe = false
				if result.RejectionReason == "" {
					result.RejectionReason = "unsafe_input"
				}
			}
		}
	}

	// 3. 模式检查
	if opts.EnablePatternCheck && len(opts.CustomPatterns) > 0 {
		patternMatches, err := s.inputFilter.(*filter.InputFilterImpl).CheckPatterns(ctx, input, opts.CustomPatterns)
		if err != nil {
			result.Checks["patterns"] = CheckStatus{Passed: false, Error: err.Error()}
		} else {
			passed := len(patternMatches) == 0
			result.Checks["patterns"] = CheckStatus{Passed: passed}
			result.PatternMatches = patternMatches
			if !passed {
				result.IsSafe = false
				result.RejectionReason = "pattern_matched"
			}
		}
	}

	// 计算整体风险评分
	result.OverallRiskScore = s.calculateOverallRisk(result)

	return result, nil
}

// ========================================
// 辅助类型和方法
// ========================================

// GuardrailCheckResult 护栏检查结果
type GuardrailCheckResult struct {
	IsSafe          bool
	RejectionReason string
	InputResult     *domainguardrail.FilterResult
	OutputResult    *domainguardrail.OutputFilterResult
	OverallRiskScore float64
	Metadata        map[string]interface{}
}

// FullCheckResult 完整检查结果
type FullCheckResult struct {
	IsSafe          bool
	RejectionReason string
	InputResult     *domainguardrail.FilterResult
	JailbreakResult *domainguardrail.JailbreakResult
	PatternMatches  []*domainguardrail.PatternMatch
	Checks          map[string]CheckStatus
	OverallRiskScore float64
	Metadata        map[string]interface{}
}

// CheckStatus 检查状态
type CheckStatus struct {
	Passed bool
	Error  string
}

// FullCheckOptions 完整检查选项
type FullCheckOptions struct {
	EnableJailbreakCheck bool
	EnableInputCheck     bool
	EnablePatternCheck   bool
	JailbreakOptions     *domainguardrail.JailbreakOptions
	InputOptions         *domainguardrail.FilterOptions
	CustomPatterns       []string
}

// DefaultFullCheckOptions 默认完整检查选项
func DefaultFullCheckOptions() *FullCheckOptions {
	return &FullCheckOptions{
		EnableJailbreakCheck: true,
		EnableInputCheck:     true,
		EnablePatternCheck:   false,
		JailbreakOptions:     domainguardrail.DefaultJailbreakOptions(),
		InputOptions:         domainguardrail.DefaultFilterOptions(),
		CustomPatterns:       nil,
	}
}

// calculateOverallRisk 计算整体风险
func (s *GuardrailServiceImpl) calculateOverallRisk(result *FullCheckResult) float64 {
	maxRisk := 0.0

	if result.InputResult != nil && result.InputResult.RiskScore > maxRisk {
		maxRisk = result.InputResult.RiskScore
	}

	if result.JailbreakResult != nil && result.JailbreakResult.Confidence > maxRisk {
		maxRisk = result.JailbreakResult.Confidence
	}

	return maxRisk
}

// GetRecommendation 获取处理建议
func (s *GuardrailServiceImpl) GetRecommendation(result *FullCheckResult) string {
	if result.IsSafe {
		return "可以正常处理该请求"
	}

	switch result.RejectionReason {
	case "jailbreak_detected":
		return "检测到越狱攻击，拒绝执行"
	case "unsafe_input":
		return "输入内容不安全，建议清理后重试"
	case "pattern_matched":
		return "输入包含禁止模式，拒绝执行"
	default:
		return "请求不安全，拒绝执行"
	}
}

// ========================================
// 快捷方法
// ========================================

// QuickSanitize 快速清理输入（使用默认配置）
func (s *GuardrailServiceImpl) QuickSanitize(ctx context.Context, input string) (string, error) {
	return s.SanitizeInput(ctx, input, domainguardrail.DefaultFilterOptions())
}

// QuickCheck 快速检查输入（使用默认配置）
func (s *GuardrailServiceImpl) QuickCheck(ctx context.Context, input string) (bool, error) {
	result, err := s.FullInputCheck(ctx, input, DefaultFullCheckOptions())
	if err != nil {
		return false, err
	}
	return result.IsSafe, nil
}

// IsJailbreak 快速检查是否是越狱攻击
func (s *GuardrailServiceImpl) IsJailbreak(ctx context.Context, input string) (bool, *domainguardrail.JailbreakResult, error) {
	result, err := s.CheckJailbreak(ctx, input, domainguardrail.DefaultJailbreakOptions())
	if err != nil {
		return false, nil, err
	}
	return result.IsJailbreak, result, nil
}
