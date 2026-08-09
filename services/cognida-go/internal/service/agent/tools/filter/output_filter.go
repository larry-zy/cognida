// Package filter provides output filter implementation
package filter

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/components/model"
	
	domainguardrail "cognida/internal/model/guardrail"
)

// ========================================
// Output Filter 实现
// ========================================

// OutputFilterImpl 输出过滤器实现
type OutputFilterImpl struct {
	llm model.ChatModel
}

// NewOutputFilter 创建输出过滤器
func NewOutputFilter(llm model.ChatModel) domainguardrail.OutputFilter {
	return &OutputFilterImpl{
		llm: llm,
	}
}

// Validate 验证输出是否安全
func (f *OutputFilterImpl) Validate(ctx context.Context, output string, input string, opts *domainguardrail.OutputFilterOptions) (*domainguardrail.OutputFilterResult, error) {
	if opts == nil {
		opts = domainguardrail.DefaultOutputFilterOptions()
	}

	result := &domainguardrail.OutputFilterResult{
		IsSafe:     true,
		Violations: make([]*domainguardrail.OutputViolation, 0),
		PIIEntities: make([]*domainguardrail.PIIEntity, 0),
		Metadata:   make(map[string]interface{}),
	}

	// 检查长度
	if opts.MaxLength > 0 && len(output) > opts.MaxLength {
		result.IsSafe = false
		result.Violations = append(result.Violations, &domainguardrail.OutputViolation{
			Type:     domainguardrail.OutputViolationTypeProfanity,
			Category: "length",
			Description: fmt.Sprintf("输出长度 %d 超过最大限制 %d", len(output), opts.MaxLength),
			Severity: 3,
			Suggestion: "输出内容过长，请缩短",
		})
	}

	// 检查 PII
	if opts.EnablePII {
		entities := f.detectPII(output)
		result.PIIEntities = entities
		if len(entities) > 0 {
			result.Metadata["pii_count"] = len(entities)
		}
	}

	// 检查脏话
	if opts.EnableProfanity {
		violations := f.checkProfanity(output)
		result.Violations = append(result.Violations, violations...)
	}

	// 检查仇恨言论
	if opts.EnableHateSpeech {
		violations := f.checkHateSpeech(output, input)
		result.Violations = append(result.Violations, violations...)
	}

	// 检查暴力内容
	if opts.EnableViolence {
		violations := f.checkViolence(output)
		result.Violations = append(result.Violations, violations...)
	}

	// 检查自残内容
	if opts.EnableSelfHarm {
		violations := f.checkSelfHarm(output)
		result.Violations = append(result.Violations, violations...)
	}

	// 检查色情内容
	if opts.EnableSexual {
		violations := f.checkSexual(output)
		result.Violations = append(result.Violations, violations...)
	}

	// 检查幻觉
	if opts.EnableHallucination {
		// 简化实现：检查输出是否包含"不确定"、"猜测"等词
		uncertainPatterns := []string{
			`(?i)(可能|也许|大概|推测|猜测|不确定)`,
			`(?i)(I think|maybe|possibly|uncertain|guess)`,
		}
		for _, pattern := range uncertainPatterns {
			if matched, _ := regexp.MatchString(pattern, output); matched {
				result.Metadata["uncertain"] = true
				break
			}
		}
	}

	// 计算风险评分
	result.RiskScore = f.calculateRiskScore(result.Violations)
	if result.RiskScore > 0.6 {
		result.IsSafe = false
	}

	return result, nil
}

// Sanitize 清理输出内容
func (f *OutputFilterImpl) Sanitize(ctx context.Context, output string, input string, opts *domainguardrail.OutputFilterOptions) (string, error) {
	if opts == nil {
		opts = domainguardrail.DefaultOutputFilterOptions()
	}

	sanitized := output

	// 清理 PII
	if opts.EnablePII && opts.MaskPII {
		sanitized = f.maskPII(sanitized)
	}

	// 清理脏话
	if opts.EnableProfanity {
		sanitized = f.maskProfanity(sanitized)
	}

	// 截断长度
	if opts.MaxLength > 0 && len(sanitized) > opts.MaxLength {
		sanitized = sanitized[:opts.MaxLength] + "..."
	}

	return sanitized, nil
}

// CheckHallucination 检查幻觉内容
func (f *OutputFilterImpl) CheckHallucination(ctx context.Context, output string, sources []string) (*domainguardrail.HallucinationResult, error) {
	result := &domainguardrail.HallucinationResult{
		HallucinatedParts: make([]*domainguardrail.HallucinatedPart, 0),
	}

	// 简化实现：检查输出中的事实是否在源文档中
	// 实际应用中应该使用更复杂的方法，如：
	// 1. 提取输出中的事实
	// 2. 在源文档中验证这些事实
	// 3. 计算忠实度分数

	if len(sources) == 0 {
		result.HasHallucination = false
		result.FaithfulnessScore = 0.5
		result.SourceCoverage = 0
		return result, nil
	}

	// 检查输出中的关键词是否出现在源文档中
	outputWords := extractKeywords(output)
	sourceWords := make(map[string]bool)
	for _, source := range sources {
		for _, word := range extractKeywords(source) {
			sourceWords[word] = true
		}
	}

	matchedCount := 0
	for _, word := range outputWords {
		if sourceWords[word] {
			matchedCount++
		}
	}

	// 计算覆盖率
	if len(outputWords) > 0 {
		result.SourceCoverage = float64(matchedCount) / float64(len(outputWords))
	}

	// 计算忠实度分数
	result.FaithfulnessScore = result.SourceCoverage

	// 判断是否存在幻觉
	result.HasHallucination = result.FaithfulnessScore < 0.7

	if result.HasHallucination {
		// 标记低覆盖率的段落
		paragraphs := strings.Split(output, "\n")
		for _, para := range paragraphs {
			paraWords := extractKeywords(para)
			paraMatched := 0
			for _, word := range paraWords {
				if sourceWords[word] {
					paraMatched++
				}
			}
			if len(paraWords) > 0 && float64(paraMatched)/float64(len(paraWords)) < 0.5 {
				startIdx := strings.Index(output, para)
				result.HallucinatedParts = append(result.HallucinatedParts, &domainguardrail.HallucinatedPart{
					Content:    para,
					Position:   &domainguardrail.TextPosition{Start: startIdx, End: startIdx + len(para)},
					Confidence: 1 - float64(paraMatched)/float64(len(paraWords)),
					Suggestion: "建议核实此信息的准确性",
				})
			}
		}
	}

	return result, nil
}

// ========================================
// 内容检测方法
// ========================================

// detectPII 检测 PII（复用 input_filter 的逻辑）
func (f *OutputFilterImpl) detectPII(input string) []*domainguardrail.PIIEntity {
	// 使用 input filter 的 PII 检测逻辑
	inputFilter := &InputFilterImpl{}
	return inputFilter.detectPII(input)
}

// maskPII 遮蔽 PII
func (f *OutputFilterImpl) maskPII(input string) string {
	inputFilter := &InputFilterImpl{}
	return inputFilter.maskPII(input)
}

// checkProfanity 检查脏话
func (f *OutputFilterImpl) checkProfanity(output string) []*domainguardrail.OutputViolation {
	violations := make([]*domainguardrail.OutputViolation, 0)
	inputFilter := &InputFilterImpl{}
	inputViolations := inputFilter.checkProfanity(output)

	for _, v := range inputViolations {
		violations = append(violations, &domainguardrail.OutputViolation{
			Type:        domainguardrail.OutputViolationTypeProfanity,
			Category:    v.Category,
			Description: v.Description,
			Severity:    v.Severity,
			Suggestion:  v.Suggestion,
		})
	}

	return violations
}

// maskProfanity 遮蔽脏话
func (f *OutputFilterImpl) maskProfanity(output string) string {
	inputFilter := &InputFilterImpl{}
	return inputFilter.maskProfanity(output)
}

// ========================================
// 仇恨言论检测
// ========================================

// HateSpeechPatterns 仇恨言论模式
var HateSpeechPatterns = []struct {
	pattern    string
	severity   int
	category   string
}{
	{`(?i)(hate|discriminate( against)?|oppress(ion)?)`, 6, "discrimination"},
	{`(?i)(inferior|superior.*race|ethnicity)`, 8, "racism"},
	{`(?i)(should (not )?exist|should die|must be eliminated)`, 9, "extremism"},
	{`(?i)(dirty|filthy|subhuman).*?(race|ethnicity|religion)`, 9, "dehumanization"},
}

// checkHateSpeech 检查仇恨言论
func (f *OutputFilterImpl) checkHateSpeech(output, input string) []*domainguardrail.OutputViolation {
	violations := make([]*domainguardrail.OutputViolation, 0)

	// 首先检查输出本身
	for _, hp := range HateSpeechPatterns {
		re, err := regexp.Compile(hp.pattern)
		if err != nil {
			continue
		}

		if re.MatchString(output) {
			violations = append(violations, &domainguardrail.OutputViolation{
				Type:     domainguardrail.OutputViolationTypeHateSpeech,
				Category: hp.category,
				Description: "检测到可能的仇恨言论内容",
				Severity: hp.severity,
				Suggestion: "请避免使用歧视性或仇恨言论",
			})
			break
		}
	}

	// 如果启用 LLM 检测，使用 LLM 进行更细致的检查
	if f.llm != nil && len(violations) == 0 {
		hateSpeechResult := f.llmHateSpeechCheck(output, input)
		if hateSpeechResult.hasHateSpeech {
			violations = append(violations, &domainguardrail.OutputViolation{
				Type:     domainguardrail.OutputViolationTypeHateSpeech,
				Category: hateSpeechResult.category,
				Description: hateSpeechResult.reason,
				Severity: hateSpeechResult.severity,
				Suggestion: "请重新组织语言，避免歧视性表达",
			})
		}
	}

	return violations
}

// llmHateSpeechCheck 使用 LLM 检查仇恨言论
type hateSpeechCheckResult struct {
	hasHateSpeech bool
	category      string
	reason        string
	severity      int
}

func (f *OutputFilterImpl) llmHateSpeechCheck(output, input string) hateSpeechCheckResult {
	// 简化实现，实际应该调用 LLM
	return hateSpeechCheckResult{
		hasHateSpeech: false,
	}
}

// ========================================
// 暴力内容检测
// ========================================

// ViolencePatterns 暴力内容模式
var ViolencePatterns = []struct {
	pattern  string
	severity int
}{
	{`(?i)(kill|murder|assassinate|slaughter)`, 7},
	{`(?i)(torture|abuse|beat up|assault)`, 6},
	{`(?i)(violent|violence|brutal|gruesome)`, 5},
	{`(?i)(weapon|bomb|explosive|attack)`, 5},
}

// checkViolence 检查暴力内容
func (f *OutputFilterImpl) checkViolence(output string) []*domainguardrail.OutputViolation {
	violations := make([]*domainguardrail.OutputViolation, 0)

	for _, vp := range ViolencePatterns {
		re, err := regexp.Compile(vp.pattern)
		if err != nil {
			continue
		}

		if re.MatchString(output) {
			violations = append(violations, &domainguardrail.OutputViolation{
				Type:     domainguardrail.OutputViolationTypeViolence,
				Category: "violence",
				Description: "检测到可能的暴力内容",
				Severity: vp.severity,
				Suggestion: "请避免描述暴力行为",
			})
			break
		}
	}

	return violations
}

// ========================================
// 自残内容检测
// ========================================

// SelfHarmPatterns 自残内容模式
var SelfHarmPatterns = []struct {
	pattern  string
	severity int
}{
	{`(?i)(kill myself|suicide|end my life)`, 10},
	{`(?i)(self-harm|hurt myself|cut myself)`, 9},
	{`(?i)(want to die|don't want to live)`, 10},
	{`(?i)(depressed|hopeless|worthless)`, 6},
}

// checkSelfHarm 检查自残内容
func (f *OutputFilterImpl) checkSelfHarm(output string) []*domainguardrail.OutputViolation {
	violations := make([]*domainguardrail.OutputViolation, 0)

	for _, shp := range SelfHarmPatterns {
		re, err := regexp.Compile(shp.pattern)
		if err != nil {
			continue
		}

		if re.MatchString(output) {
			violations = append(violations, &domainguardrail.OutputViolation{
				Type:     domainguardrail.OutputViolationTypeSelfHarm,
				Category: "self_harm",
				Description: "检测到可能的自残或自杀相关内容",
				Severity: shp.severity,
				Suggestion: "如果遇到困难，请寻求专业帮助",
			})
		}
	}

	return violations
}

// ========================================
// 色情内容检测
// ========================================

// SexualPatterns 色情内容模式
var SexualPatterns = []struct {
	pattern  string
	severity int
}{
	{`(?i)(porn|xxx|adult content)`, 8},
	{`(?i)(nude|naked|explicit)`, 7},
	{`(?i)(sexual|sexually explicit)`, 6},
}

// checkSexual 检查色情内容
func (f *OutputFilterImpl) checkSexual(output string) []*domainguardrail.OutputViolation {
	violations := make([]*domainguardrail.OutputViolation, 0)

	for _, sp := range SexualPatterns {
		re, err := regexp.Compile(sp.pattern)
		if err != nil {
			continue
		}

		if re.MatchString(output) {
			violations = append(violations, &domainguardrail.OutputViolation{
				Type:     domainguardrail.OutputViolationTypeSexual,
				Category: "sexual",
				Description: "检测到可能的色情内容",
				Severity: sp.severity,
				Suggestion: "请避免色情内容",
			})
		}
	}

	return violations
}

// ========================================
// 辅助函数
// ========================================

// calculateRiskScore 计算风险评分
func (f *OutputFilterImpl) calculateRiskScore(violations []*domainguardrail.OutputViolation) float64 {
	if len(violations) == 0 {
		return 0
	}

	totalScore := 0.0
	for _, v := range violations {
		totalScore += float64(v.Severity) / 10.0
	}

	score := totalScore / float64(len(violations))
	if score > 1 {
		score = 1
	}

	return score
}

// extractKeywords 提取关键词
func extractKeywords(text string) []string {
	// 简化实现：分词并过滤停用词
	words := strings.Fields(text)
	keywords := make([]string, 0)

	stopWords := map[string]bool{
		"的": true, "了": true, "是": true, "在": true,
		"和": true, "与": true, "或": true, "等": true,
		"the": true, "a": true, "an": true, "is": true,
		"and": true, "or": true, "of": true, "to": true,
	}

	for _, word := range words {
		word = strings.ToLower(strings.Trim(word, ".,!?;:\"'"))
		if len(word) > 2 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}

	return keywords
}
