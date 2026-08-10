// Package experience 实现会话经验自动沉淀：把一段已结束会话蒸馏为结构化经验，
// 写入知识图谱。本文件是「蒸馏」环节：单次独立 LLM 调用。
package experience

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	domain_experience "cognida/internal/model/agent/experience"
	domain_conversation "cognida/internal/model/conversation"
	prompts "cognida/internal/prompt"
)

// LLM 是蒸馏所需的最小生成接口；eino 的 model.BaseChatModel / ToolCallingChatModel 均满足，
// 单测可用轻量 fake 实现，无需 Stream。
type LLM interface {
	Generate(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.Message, error)
}

// summarizeSystemPrompt 正文集中于 internal/prompt/templates/experience.yaml。
var summarizeSystemPrompt = prompts.MustGet("experience", "summarize_system")

// Summarizer 负责调用 LLM 把会话蒸馏为结构化经验。
type Summarizer struct {
	llm LLM
	// maxMessages 拼装进提示的最多消息条数（防止超长会话撑爆上下文）。
	maxMessages int
	// maxContentChars 单条消息正文截断上限。
	maxContentChars int
}

// NewSummarizer 创建蒸馏器。
func NewSummarizer(llm LLM) *Summarizer {
	return &Summarizer{llm: llm, maxMessages: 40, maxContentChars: 2000}
}

// Summarize 把会话消息蒸馏为一条经验的 LLM 产出字段（不含落库元数据）。
// 返回的 Experience 只填充 Title/Problem/Solution/Tools/Tags。
func (s *Summarizer) Summarize(ctx context.Context, messages []*domain_conversation.Message) (*domain_experience.Experience, error) {
	if s.llm == nil {
		return nil, fmt.Errorf("summarizer: llm 未注入")
	}
	transcript := s.buildTranscript(messages)
	if strings.TrimSpace(transcript) == "" {
		return nil, fmt.Errorf("summarizer: 会话无有效内容")
	}

	resp, err := s.llm.Generate(ctx, []*schema.Message{
		schema.SystemMessage(summarizeSystemPrompt),
		schema.UserMessage("以下是完整对话：\n\n" + transcript),
	})
	if err != nil {
		return nil, fmt.Errorf("summarizer: llm 生成失败: %w", err)
	}

	var parsed struct {
		Title             string   `json:"title"`
		Problem           string   `json:"problem"`
		Solution          string   `json:"solution"`
		Tools             []string `json:"tools"`
		Tags              []string `json:"tags"`
		Success           *bool    `json:"success"`            // 指针：区分「模型未给」与「显式 false」
		Confidence        int      `json:"confidence"`         // 0~100
		SkillWorthy       bool     `json:"skill_worthy"`       // 是否值得抽象为可复用技能
		SkillInstructions string   `json:"skill_instructions"` // skill_worthy 时的操作指引正文
	}
	if err := json.Unmarshal([]byte(extractJSON(resp.Content)), &parsed); err != nil {
		return nil, fmt.Errorf("summarizer: llm 输出非合法 JSON: %w", err)
	}

	exp := &domain_experience.Experience{
		Title:             strings.TrimSpace(parsed.Title),
		Problem:           strings.TrimSpace(parsed.Problem),
		Solution:          strings.TrimSpace(parsed.Solution),
		Tools:             dedupNonEmpty(parsed.Tools),
		Tags:              dedupNonEmpty(parsed.Tags),
		Confidence:        clampConfidence(parsed.Confidence),
		SkillWorthy:       parsed.SkillWorthy,
		SkillInstructions: strings.TrimSpace(parsed.SkillInstructions),
	}
	// skill_worthy 但没给出指引正文 → 无正文不成技能，降级为普通经验，避免落出空壳 SKILL.md。
	if exp.SkillWorthy && exp.SkillInstructions == "" {
		exp.SkillWorthy = false
	}
	// success 显式为 false（会话未真正解决）→ 视为无可沉淀经验：折叠为空、置信 0、撤销技能沉淀，
	// 交由 worker 的空值 skipped 路径统一处理，避免失败经验带着高分蒙混过关。
	if parsed.Success != nil && !*parsed.Success {
		exp.Title, exp.Problem, exp.Solution = "", "", ""
		exp.Confidence = 0
		exp.SkillWorthy, exp.SkillInstructions = false, ""
	}
	return exp, nil
}

// clampConfidence 把置信度夹到 [0,100]，容忍模型给出的越界值。
func clampConfidence(c int) int {
	if c < 0 {
		return 0
	}
	if c > 100 {
		return 100
	}
	return c
}

// buildTranscript 把消息拼装为「角色：正文」的可读对话文本，超长时保留最近若干条。
func (s *Summarizer) buildTranscript(messages []*domain_conversation.Message) string {
	// 只取最近 maxMessages 条（messages 已按时间正序）。
	start := 0
	if len(messages) > s.maxMessages {
		start = len(messages) - s.maxMessages
	}
	var b strings.Builder
	for _, m := range messages[start:] {
		if m == nil {
			continue
		}
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		if len([]rune(content)) > s.maxContentChars {
			content = truncate(content, s.maxContentChars) + "…(截断)"
		}
		role := roleLabel(m.Role)
		fmt.Fprintf(&b, "【%s】%s\n", role, content)
	}
	return b.String()
}

// roleLabel 把内部角色名映射为中文可读标签。
func roleLabel(role string) string {
	switch role {
	case domain_conversation.RoleUser:
		return "用户"
	case domain_conversation.RoleAssistant:
		return "助手"
	case domain_conversation.RoleSystem:
		return "系统"
	default:
		return role
	}
}

// dedupNonEmpty 去空白去重，保持首次出现顺序。
func dedupNonEmpty(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	var out []string
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it == "" {
			continue
		}
		if _, ok := seen[it]; ok {
			continue
		}
		seen[it] = struct{}{}
		out = append(out, it)
	}
	return out
}

// extractJSON 从可能带围栏/前后缀的文本中截出首个平衡的 JSON 对象。
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	start := strings.Index(s, "{")
	if start < 0 {
		return s
	}
	depth, inStr, esc := 0, false, false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return s[start:]
}
