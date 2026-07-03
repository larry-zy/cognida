// Package context 实现 Data Agent 的上下文工程：多轮历史的窗口化与摘要，
// 以及分层动态提示词的 token 预算治理。
//
// 两条不变量贯穿本包：
//   - 超窗历史被压缩为紧凑摘要时，仍被引用的 result_id 与关键结论 MUST 保留
//     （data-by-reference 的引用不能因窗口滑出而丢失）。
//   - 分层提示词超预算裁剪时，系统约束与安全策略层（Pinned）MUST 保留。
package context

import (
	"context"
	"regexp"
	"sort"
	"strings"
)

// Turn 表示一轮对话的紧凑视图。Content 为该轮文本；ResultIDs 为该轮产生/引用的
// 结果信封 result_id；Conclusion 为该轮的关键结论（可选，用于摘要保真）。
type Turn struct {
	Role       string
	Content    string
	ResultIDs  []string
	Conclusion string
}

// WindowConfig 配置历史窗口。KeepRecentTurns 为保留原文的最近轮数；累计轮数超过它时，
// 更早的轮次被摘要成一条紧凑记忆。KeepRecentTurns<=0 表示不做窗口化。
type WindowConfig struct {
	KeepRecentTurns int
}

// WindowResult 是窗口化输出：Summary 为早期轮次的紧凑摘要（可能为空），Recent 为保留原文的
// 最近轮次，PreservedResultIDs 为被摘要轮次中出现、必须保留的 result_id 去重集合。
type WindowResult struct {
	Summary            string
	Recent             []Turn
	PreservedResultIDs []string
}

// Summarizer 把一组早期轮次压缩为一段紧凑摘要文本。实现方（LLM/抽取式）SHALL 在摘要中
// 保留 preservedResultIDs 列出的 result_id 与各轮结论。
type Summarizer interface {
	Summarize(ctx context.Context, older []Turn, preservedResultIDs []string) (string, error)
}

// Windower 施加历史窗口 + 摘要策略。
type Windower struct {
	cfg        WindowConfig
	summarizer Summarizer
}

// NewWindower 构造 Windower。summarizer 为 nil 时使用确定性的抽取式摘要（无需 LLM）。
func NewWindower(cfg WindowConfig, summarizer Summarizer) *Windower {
	if summarizer == nil {
		summarizer = ExtractiveSummarizer{}
	}
	return &Windower{cfg: cfg, summarizer: summarizer}
}

// Apply 对 turns 施加窗口化。累计轮数不超过窗口时原样返回；超过时把最早的
// len-KeepRecentTurns 轮摘要为 Summary，并保留其 result_id 到 PreservedResultIDs。
func (w *Windower) Apply(ctx context.Context, turns []Turn) (*WindowResult, error) {
	keep := w.cfg.KeepRecentTurns
	if keep <= 0 || len(turns) <= keep {
		return &WindowResult{Recent: turns}, nil
	}

	split := len(turns) - keep
	older := turns[:split]
	recent := turns[split:]

	preserved := collectResultIDs(older)
	summary, err := w.summarizer.Summarize(ctx, older, preserved)
	if err != nil {
		return nil, err
	}
	// 兜底不变量：无论摘要器如何实现，仍被引用的 result_id 必须出现在摘要文本里，
	// 否则后续轮次无法按引用取回完整结果集。
	summary = ensureResultIDsPresent(summary, preserved)

	return &WindowResult{
		Summary:            summary,
		Recent:             recent,
		PreservedResultIDs: preserved,
	}, nil
}

// resultIDPattern 匹配 result_id（rs_ 前缀 + uuid/别名）。
var resultIDPattern = regexp.MustCompile(`rs_[0-9a-zA-Z-]+`)

// collectResultIDs 汇总若干轮中出现的 result_id（显式字段 + 文本内嵌），去重且稳定排序。
func collectResultIDs(turns []Turn) []string {
	seen := map[string]struct{}{}
	for _, t := range turns {
		for _, id := range t.ResultIDs {
			if id != "" {
				seen[id] = struct{}{}
			}
		}
		for _, id := range resultIDPattern.FindAllString(t.Content+" "+t.Conclusion, -1) {
			seen[id] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// ensureResultIDsPresent 确保摘要文本包含所有 preserved result_id，缺失的补挂到末尾。
func ensureResultIDsPresent(summary string, ids []string) string {
	if len(ids) == 0 {
		return summary
	}
	var missing []string
	for _, id := range ids {
		if !strings.Contains(summary, id) {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return summary
	}
	tail := "可引用结果：" + strings.Join(missing, ", ")
	if strings.TrimSpace(summary) == "" {
		return tail
	}
	return summary + "\n" + tail
}

// ExtractiveSummarizer 是确定性的抽取式摘要器：不调用 LLM，把每轮的角色/结论（或截断内容）
// 逐条列出，并显式附上待保留的 result_id。适合作为默认实现与单测基线。
type ExtractiveSummarizer struct{}

// Summarize 实现 Summarizer。
func (ExtractiveSummarizer) Summarize(_ context.Context, older []Turn, preserved []string) (string, error) {
	var b strings.Builder
	b.WriteString("【早期对话摘要】\n")
	for i, t := range older {
		line := t.Conclusion
		if line == "" {
			line = collapse(t.Content, 120)
		}
		if line == "" {
			continue
		}
		role := t.Role
		if role == "" {
			role = "turn"
		}
		b.WriteString("- ")
		b.WriteString(role)
		b.WriteString(": ")
		b.WriteString(line)
		if i < len(older)-1 {
			b.WriteString("\n")
		}
	}
	if len(preserved) > 0 {
		b.WriteString("\n可引用结果：")
		b.WriteString(strings.Join(preserved, ", "))
	}
	return b.String(), nil
}

// collapse 折叠多余空白并截断到 max 字符。
func collapse(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}
