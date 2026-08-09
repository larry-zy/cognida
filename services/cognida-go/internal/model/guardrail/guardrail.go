// Package guardrail 提供安全护栏领域接口定义
package guardrail

import (
	"context"
)

// ========================================
// Input Filter 输入过滤器接口
// ========================================

// InputFilter 输入过滤器接口
type InputFilter interface {
	// Validate 验证输入是否安全
	Validate(ctx context.Context, input string, opts *FilterOptions) (*FilterResult, error)

	// Sanitize 清理输入内容
	Sanitize(ctx context.Context, input string, opts *FilterOptions) (string, error)

	// CheckPatterns 检查输入中的特定模式
	CheckPatterns(ctx context.Context, input string, patterns []string) ([]*PatternMatch, error)
}

// FilterOptions 过滤器选项
type FilterOptions struct {
	// EnablePII 是否启用 PII 检测
	EnablePII bool

	// EnableProfanity 是否启用脏话检测
	EnableProfanity bool

	// EnableJailbreak 是否启用越狱检测
	EnableJailbreak bool

	// EnableSQLInjection 是否启用 SQL 注入检测
	EnableSQLInjection bool

	// EnableXSS 是否启用 XSS 检测
	EnableXSS bool

	// CustomPatterns 自定义检测模式
	CustomPatterns []*PatternRule

	// MaskPII 是否遮蔽 PII 信息
	MaskPII bool

	// AllowedLanguages 允许的语言列表
	AllowedLanguages []string

	// MaxLength 最大输入长度
	MaxLength int
}

// DefaultFilterOptions 默认过滤器选项
func DefaultFilterOptions() *FilterOptions {
	return &FilterOptions{
		EnablePII:          true,
		EnableProfanity:    true,
		EnableJailbreak:    true,
		EnableSQLInjection: true,
		EnableXSS:          true,
		MaskPII:            true,
		MaxLength:          10000,
	}
}

// FilterResult 过滤结果
type FilterResult struct {
	// IsSafe 输入是否安全
	IsSafe bool

	// SanitizedInput 清理后的输入
	SanitizedInput string

	// Violations 违规项列表
	Violations []*Violation

	// PIIEntities 检测到的 PII 实体
	PIIEntities []*PIIEntity

	// RiskScore 风险评分 (0-1)
	RiskScore float64

	// Metadata 元数据
	Metadata map[string]interface{}
}

// Violation 违规项
type Violation struct {
	// Type 违规类型
	Type ViolationType

	// Category 违规类别
	Category string

	// Description 描述
	Description string

	// Position 位置
	Position *TextPosition

	// Severity 严重程度 (1-10)
	Severity int

	// Suggestion 建议
	Suggestion string
}

// ViolationType 违规类型
type ViolationType string

const (
	ViolationTypePII           ViolationType = "pii"
	ViolationTypeProfanity     ViolationType = "profanity"
	ViolationTypeJailbreak     ViolationType = "jailbreak"
	ViolationTypeSQLInjection  ViolationType = "sql_injection"
	ViolationTypeXSS           ViolationType = "xss"
	ViolationTypeCustomPattern ViolationType = "custom_pattern"
	ViolationTypeLengthExceeded ViolationType = "length_exceeded"
)

// TextPosition 文本位置
type TextPosition struct {
	Start int
	End   int
	Line  int
}

// PIIEntity PII 实体
type PIIEntity struct {
	// Type PII 类型
	Type PIIType

	// Value 原始值
	Value string

	// MaskedValue 遮蔽后的值
	MaskedValue string

	// Confidence 置信度 (0-1)
	Confidence float64

	// Position 位置
	Position *TextPosition
}

// PIIType PII 类型
type PIIType string

const (
	PIITypeEmail       PIIType = "email"
	PIITypePhone       PIIType = "phone"
	PIITypeIDCard      PIIType = "id_card"
	PIITypeCreditCard  PIIType = "credit_card"
	PIITypeSSN         PIIType = "ssn"
	PIITypeIPAddress   PIIType = "ip_address"
	PIITypeName        PIIType = "name"
	PIITypeAddress     PIIType = "address"
	PIITypePassport    PIIType = "passport"
	PIITypeBankAccount PIIType = "bank_account"
)

// PatternRule 模式规则
type PatternRule struct {
	// Name 规则名称
	Name string

	// Pattern 正则表达式模式
	Pattern string

	// Description 描述
	Description string

	// Severity 严重程度
	Severity int

	// Category 类别
	Category string
}

// PatternMatch 模式匹配结果
type PatternMatch struct {
	// RuleName 规则名称
	RuleName string

	// Match 匹配的内容
	Match string

	// Position 位置
	Position *TextPosition

	// Rule 规则
	Rule *PatternRule
}

// ========================================
// Output Filter 输出过滤器接口
// ========================================

// OutputFilter 输出过滤器接口
type OutputFilter interface {
	// Validate 验证输出是否安全
	Validate(ctx context.Context, output string, input string, opts *OutputFilterOptions) (*OutputFilterResult, error)

	// Sanitize 清理输出内容
	Sanitize(ctx context.Context, output string, input string, opts *OutputFilterOptions) (string, error)

	// CheckHallucination 检查幻觉内容
	CheckHallucination(ctx context.Context, output string, sources []string) (*HallucinationResult, error)
}

// OutputFilterOptions 输出过滤器选项
type OutputFilterOptions struct {
	// EnablePII 是否启用 PII 检测
	EnablePII bool

	// EnableProfanity 是否启用脏话检测
	EnableProfanity bool

	// EnableHateSpeech 是否启用仇恨言论检测
	EnableHateSpeech bool

	// EnableViolence 是否启用暴力内容检测
	EnableViolence bool

	// EnableSelfHarm 是否启用自残内容检测
	EnableSelfHarm bool

	// EnableSexual 是否启用色情内容检测
	EnableSexual bool

	// EnableFairness 是否启用公平性检测
	EnableFairness bool

	// EnableHallucination 是否启用幻觉检测
	EnableHallucination bool

	// MaskPII 是否遮蔽 PII 信息
	MaskPII bool

	// MaxLength 最大输出长度
	MaxLength int
}

// DefaultOutputFilterOptions 默认输出过滤器选项
func DefaultOutputFilterOptions() *OutputFilterOptions {
	return &OutputFilterOptions{
		EnablePII:          true,
		EnableProfanity:    true,
		EnableHateSpeech:   true,
		EnableViolence:     true,
		EnableSelfHarm:     true,
		EnableSexual:       true,
		EnableFairness:     false,
		EnableHallucination: false,
		MaskPII:            true,
		MaxLength:          50000,
	}
}

// OutputFilterResult 输出过滤结果
type OutputFilterResult struct {
	// IsSafe 输出是否安全
	IsSafe bool

	// SanitizedOutput 清理后的输出
	SanitizedOutput string

	// Violations 违规项列表
	Violations []*OutputViolation

	// PIIEntities 检测到的 PII 实体
	PIIEntities []*PIIEntity

	// RiskScore 风险评分 (0-1)
	RiskScore float64

	// Metadata 元数据
	Metadata map[string]interface{}
}

// OutputViolation 输出违规项
type OutputViolation struct {
	// Type 违规类型
	Type OutputViolationType

	// Category 违规类别
	Category string

	// Description 描述
	Description string

	// Position 位置
	Position *TextPosition

	// Severity 严重程度 (1-10)
	Severity int

	// Suggestion 建议
	Suggestion string
}

// OutputViolationType 输出违规类型
type OutputViolationType string

const (
	OutputViolationTypePII         OutputViolationType = "pii"
	OutputViolationTypeProfanity   OutputViolationType = "profanity"
	OutputViolationTypeHateSpeech  OutputViolationType = "hate_speech"
	OutputViolationTypeViolence    OutputViolationType = "violence"
	OutputViolationTypeSelfHarm    OutputViolationType = "self_harm"
	OutputViolationTypeSexual      OutputViolationType = "sexual"
	OutputViolationTypeFairness    OutputViolationType = "fairness"
	OutputViolationTypeHallucination OutputViolationType = "hallucination"
)

// HallucinationResult 幻觉检测结果
type HallucinationResult struct {
	// HasHallucination 是否存在幻觉
	HasHallucination bool

	// HallucinatedParts 幻觉部分
	HallucinatedParts []*HallucinatedPart

	// FaithfulnessScore 忠实度评分 (0-1)
	FaithfulnessScore float64

	// SourceCoverage 源文档覆盖率
	SourceCoverage float64
}

// HallucinatedPart 幻觉部分
type HallucinatedPart struct {
	// Content 幻觉内容
	Content string

	// Position 位置
	Position *TextPosition

	// Confidence 置信度
	Confidence float64

	// Suggestion 建议
	Suggestion string
}

// ========================================
// Jailbreak Detector 越狱检测器接口
// ========================================

// JailbreakDetector 越狱检测器接口
type JailbreakDetector interface {
	// Detect 检测越狱攻击
	Detect(ctx context.Context, input string, opts *JailbreakOptions) (*JailbreakResult, error)

	// AnalyzePattern 分析攻击模式
	AnalyzePattern(ctx context.Context, input string) (*AttackPattern, error)

	// AddKnownPattern 添加已知攻击模式
	AddKnownPattern(ctx context.Context, pattern *AttackPattern) error
}

// JailbreakOptions 越狱检测选项
type JailbreakOptions struct {
	// EnablePatternMatching 是否启用模式匹配
	EnablePatternMatching bool

	// EnableLLMCheck 是否启用 LLM 检测
	EnableLLMCheck bool

	// Strictness 严格程度 (1-10)
	Strictness int

	// CustomPatterns 自定义攻击模式
	CustomPatterns []*AttackPattern
}

// DefaultJailbreakOptions 默认越狱检测选项
func DefaultJailbreakOptions() *JailbreakOptions {
	return &JailbreakOptions{
		EnablePatternMatching: true,
		EnableLLMCheck:        false,
		Strictness:            7,
	}
}

// JailbreakResult 越狱检测结果
type JailbreakResult struct {
	// IsJailbreak 是否是越狱攻击
	IsJailbreak bool

	// Confidence 置信度 (0-1)
	Confidence float64

	// DetectedPatterns 检测到的攻击模式
	DetectedPatterns []*AttackPattern

	// AttackType 攻击类型
	AttackType string

	// Severity 严重程度 (1-10)
	Severity int

	// SuggestedAction 建议的操作
	SuggestedAction string
}

// AttackPattern 攻击模式
type AttackPattern struct {
	// Name 模式名称
	Name string

	// Category 类别
	Category string

	// Pattern 正则表达式模式
	Pattern string

	// Description 描述
	Description string

	// Examples 示例
	Examples []string

	// Severity 严重程度
	Severity int

	// Countermeasure 对策
	Countermeasure string
}

// ========================================
// Guardrail Service 护栏服务接口
// ========================================

// GuardrailService 安全护栏服务接口
type GuardrailService interface {
	// CheckInput 检查输入
	CheckInput(ctx context.Context, input string, opts *FilterOptions) (*FilterResult, error)

	// CheckOutput 检查输出
	CheckOutput(ctx context.Context, output string, input string, outputOpts *OutputFilterOptions) (*OutputFilterResult, error)

	// CheckJailbreak 检查越狱攻击
	CheckJailbreak(ctx context.Context, input string, opts *JailbreakOptions) (*JailbreakResult, error)

	// SanitizeInput 清理输入
	SanitizeInput(ctx context.Context, input string, opts *FilterOptions) (string, error)

	// SanitizeOutput 清理输出
	SanitizeOutput(ctx context.Context, output string, input string, outputOpts *OutputFilterOptions) (string, error)
}
