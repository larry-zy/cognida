package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ClarificationQuestion 澄清问题
type ClarificationQuestion struct {
	Question    string   `json:"question"`              // 问题文本
	Options     []string `json:"options,omitempty"`     // 可选答案（如有）
	IsRequired  bool     `json:"is_required"`           // 是否必答
	Context     string   `json:"context,omitempty"`     // 上下文说明
	QuestionID  string   `json:"question_id"`           // 问题ID
}

// ClarificationState 澄清状态
type ClarificationState struct {
	OriginalQuery string                   `json:"original_query"` // 原始查询
	Questions     []*ClarificationQuestion `json:"questions"`      // 澄清问题列表
	Answers       map[string]string        `json:"answers"`        // 答案映射
	Resolved      bool                     `json:"resolved"`       // 是否已解决
	Round         int                      `json:"round"`          // 当前轮次
	BestGuess     string                   `json:"best_guess"`     // 重写后的查询
}

// IntentClarifier 意图澄清器
type IntentClarifier struct {
	*BaseHook
	businessContext string // 业务上下文
	maxRounds       int    // 最大澄清轮次
}

// ClarificationNeededError 澄清需求错误
type ClarificationNeededError struct {
	State *ClarificationState
	Text  string
}

func (e *ClarificationNeededError) Error() string {
	return fmt.Sprintf("clarification needed: %s", e.Text)
}

// NewIntentClarifier 创建 IntentClarifier 实例
func NewIntentClarifier(llm LLMClient) *IntentClarifier {
	return &IntentClarifier{
		BaseHook:        NewBaseHook("intent_clarifier", llm),
		businessContext: "",
		maxRounds:       2,
	}
}

// WithBusinessContext 设置业务上下文
func (c *IntentClarifier) WithBusinessContext(ctx string) *IntentClarifier {
	c.businessContext = ctx
	return c
}

// WithMaxRounds 设置最大澄清轮次
func (c *IntentClarifier) WithMaxRounds(rounds int) *IntentClarifier {
	c.maxRounds = rounds
	return c
}

// getSystemPrompt 获取系统提示
func (c *IntentClarifier) getSystemPrompt() string {
	prompt := `你是一个查询分析专家，负责判断用户查询是否需要澄清。

分析用户查询，判断是否缺少关键信息或存在歧义。如果需要澄清，返回 JSON 格式的澄清问题列表。

返回 JSON 格式：
{
  "needs_clarification": true/false,
  "questions": [
    {
      "question_id": "唯一ID",
      "question": "问题文本",
      "options": ["选项1", "选项2"],  // 可选
      "is_required": true,
      "context": "为何需要此信息的说明"
    }
  ],
  "best_guess": "基于现有信息的最佳猜测查询"
}

**判断标准**：
- 时间范围：缺少具体时间或范围需要澄清
- 数据范围：缺少具体的数据维度或筛选条件需要澄清
- 业务对象：多个可能的业务对象需要澄清
- 分析目标：不明确的分析意图需要澄清`

	if c.businessContext != "" {
		prompt += fmt.Sprintf(`

**业务上下文**: %s

请在分析时考虑此业务上下文。`, c.businessContext)
	}

	return prompt
}

// buildAnalysisPrompt 构建分析提示
func (c *IntentClarifier) buildAnalysisPrompt(query string) string {
	return fmt.Sprintf(`请分析以下用户查询是否需要澄清：

用户查询: %s

返回 JSON 格式的分析结果。`, query)
}

// analyzeQuery 分析查询清晰度
func (c *IntentClarifier) analyzeQuery(ctx context.Context, query string) (*ClarificationResult, error) {
	prompt := c.buildAnalysisPrompt(query)

	messages := []Message{
		{Role: "system", Content: c.getSystemPrompt()},
		{Role: "user", Content: prompt},
	}

	response, err := c.llm.Chat(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// 解析响应
	var result struct {
		NeedsClarification bool                      `json:"needs_clarification"`
		Questions          []*ClarificationQuestion  `json:"questions"`
		BestGuess          string                    `json:"best_guess"`
	}

	if err := json.Unmarshal([]byte(response), &result); err != nil {
		// 解析失败，默认不需要澄清
		return &ClarificationResult{
			NeedsClarification: false,
			rewrittenQuery:    query,
		}, nil
	}

	if !result.NeedsClarification {
		return &ClarificationResult{
			NeedsClarification: false,
			rewrittenQuery:    query,
		}, nil
	}

	// 生成问题ID
	for i, q := range result.Questions {
		if q.QuestionID == "" {
			q.QuestionID = fmt.Sprintf("q_%d", i+1)
		}
	}

	state := &ClarificationState{
		OriginalQuery: query,
		Questions:     result.Questions,
		Answers:       make(map[string]string),
		Resolved:      false,
		Round:         1,
		BestGuess:     result.BestGuess,
	}

	return &ClarificationResult{
		NeedsClarification: true,
		State:             state,
		ClarificationText: c.buildClarificationText(state),
	}, nil
}

// ClarificationResult 澄清结果
type ClarificationResult struct {
	NeedsClarification bool                `json:"needs_clarification"`
	State             *ClarificationState `json:"state"`
	rewrittenQuery    string              `json:"-"`
	ClarificationText string              `json:"clarification_text"`
}

// buildClarificationText 构建澄清文本
func (c *IntentClarifier) buildClarificationText(state *ClarificationState) string {
	var builder strings.Builder

	builder.WriteString("为了更好地帮助您，我需要澄清一些信息：\n\n")

	for _, q := range state.Questions {
		builder.WriteString(fmt.Sprintf("**%s**", q.Question))

		if q.Context != "" {
			builder.WriteString(fmt.Sprintf(" (%s)", q.Context))
		}

		builder.WriteString("\n")

		if len(q.Options) > 0 {
			builder.WriteString("可选答案:\n")
			for _, opt := range q.Options {
				builder.WriteString(fmt.Sprintf("- %s\n", opt))
			}
		}

		builder.WriteString("\n")
	}

	return builder.String()
}

// handleClarificationRound 处理澄清轮次
func (c *IntentClarifier) handleClarificationRound(ctx context.Context, state *ClarificationState, userAnswers map[string]string) (*ClarificationResult, error) {
	// 更新答案
	for key, value := range userAnswers {
		state.Answers[key] = value
	}

	// 检查是否全部必答问题已回答
	allRequiredAnswered := true
	var unansweredQuestions []*ClarificationQuestion

	for _, q := range state.Questions {
		if q.IsRequired && state.Answers[q.QuestionID] == "" {
			allRequiredAnswered = false
			unansweredQuestions = append(unansweredQuestions, q)
		}
	}

	// 检查是否超过最大轮次
	if state.Round >= c.maxRounds {
		// 使用最佳猜测
		rewrittenQuery, err := c.rewriteQuery(ctx, state, true)
		if err != nil {
			return nil, err
		}

		state.Resolved = true
		state.BestGuess = rewrittenQuery
		return &ClarificationResult{
			NeedsClarification: false,
			State:             state,
			rewrittenQuery:    rewrittenQuery,
		}, nil
	}

	if allRequiredAnswered {
		// 全部回答完毕，重写查询
		rewrittenQuery, err := c.rewriteQuery(ctx, state, false)
		if err != nil {
			return nil, err
		}

		state.Resolved = true
		state.BestGuess = rewrittenQuery
		return &ClarificationResult{
			NeedsClarification: false,
			State:             state,
			rewrittenQuery:    rewrittenQuery,
		}, nil
	}

	// 继续澄清未回答的问题
	state.Round++
	state.Questions = unansweredQuestions

	return &ClarificationResult{
		NeedsClarification: true,
		State:             state,
		ClarificationText: c.buildClarificationText(state),
	}, nil
}

// rewriteQuery 基于澄清重写查询
func (c *IntentClarifier) rewriteQuery(ctx context.Context, state *ClarificationState, useBestGuess bool) (string, error) {
	if useBestGuess && state.BestGuess != "" {
		return state.BestGuess, nil
	}

	messages := []Message{
		{
			Role:    "system",
			Content: "你是一个查询重写专家。基于原始查询和用户的澄清回答，重写一个更明确、完整的查询。",
		},
		{
			Role: "user",
			Content: c.buildRewritePrompt(state),
		},
	}

	response, err := c.llm.Chat(ctx, messages)
	if err != nil {
		// 失败时返回原始查询
		return state.OriginalQuery, nil
	}

	return strings.TrimSpace(response), nil
}

// buildRewritePrompt 构建重写提示
func (c *IntentClarifier) buildRewritePrompt(state *ClarificationState) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("原始查询: %s\n\n", state.OriginalQuery))

	builder.WriteString("用户澄清回答:\n")
	for qID, answer := range state.Answers {
		builder.WriteString(fmt.Sprintf("- %s: %s\n", qID, answer))
	}

	if c.businessContext != "" {
		builder.WriteString(fmt.Sprintf("\n业务上下文: %s\n", c.businessContext))
	}

	builder.WriteString("\n请重写查询，使其更加明确和完整。只返回重写后的查询，不要额外说明。")

	return builder.String()
}

// parseAnswer 解析用户回答
func (c *IntentClarifier) parseAnswer(input string) map[string]string {
	answers := make(map[string]string)

	// 简单解析：按行分割，格式为 "问题ID: 答案" 或 "问题ID=答案"
	lines := strings.Split(input, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 尝试解析 "ID: Answer" 或 "ID=Answer" 格式
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				answers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		} else if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				answers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		} else {
			// 简单文本，假设是对第一个问题的回答
			if len(answers) == 0 {
				answers["q_1"] = line
			}
		}
	}

	return answers
}

// getClarificationState 从 Context 获取澄清状态
func (c *IntentClarifier) getClarificationState(ctx context.Context) (*ClarificationState, bool) {
	state, ok := ctx.Value(clarificationStateKey{}).(*ClarificationState)
	return state, ok
}

// setClarificationState 设置澄清状态到 Context
func (c *IntentClarifier) setClarificationState(ctx context.Context, state *ClarificationState) context.Context {
	return context.WithValue(ctx, clarificationStateKey{}, state)
}

// clarificationStateKey Context key 类型
type clarificationStateKey struct{}

// Hook 返回 BeforeHook 函数（兼容通用接口）
func (c *IntentClarifier) Hook() func(ctx context.Context, message string) (context.Context, string, error) {
	return func(ctx context.Context, message string) (context.Context, string, error) {
		if !c.IsEnabled() {
			return ctx, message, nil
		}

		err := c.SafeExecute(ctx, func() error {
			// 检查是否已有澄清状态
			if state, ok := c.getClarificationState(ctx); ok {
				// 处理澄清回答
				answers := c.parseAnswer(message)
				result, err := c.handleClarificationRound(ctx, state, answers)
				if err != nil {
					return err
				}

				if result.NeedsClarification {
					// 继续需要澄清
					return &ClarificationNeededError{
						State: result.State,
						Text:  result.ClarificationText,
					}
				}

				// 澄清完成，使用重写的查询
				ctx = c.setClarificationState(ctx, result.State)
				return nil
			}

			// 首次分析查询
			result, err := c.analyzeQuery(ctx, message)
			if err != nil {
				return err
			}

			if result.NeedsClarification {
				// 需要澄清
				ctx = c.setClarificationState(ctx, result.State)
				return &ClarificationNeededError{
					State: result.State,
					Text:  result.ClarificationText,
				}
			}

			return nil
		})

		// 如果是澄清需求错误，返回它
		if cerr, ok := err.(*ClarificationNeededError); ok {
			return ctx, message, cerr
		}

		// 其他错误也返回
		if err != nil {
			return ctx, message, err
		}

		// 检查是否需要使用重写的查询
		if state, ok := c.getClarificationState(ctx); ok && state.Resolved && state.BestGuess != "" {
			// 使用重写的查询
			return ctx, state.BestGuess, nil
		}

		return ctx, message, nil
	}
}
