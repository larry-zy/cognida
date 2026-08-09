package context

import "strings"

// Layer 是分层动态提示词的一层。Priority 越大越重要（超预算时越晚被裁剪）；Pinned 标记
// 系统约束/安全策略层，MUST 永不被裁剪或截断。
type Layer struct {
	Name     string
	Content  string
	Priority int
	Pinned   bool
}

// TokenCounter 估算文本 token 数。基础设施层的 tokenCounter 满足本接口。
type TokenCounter interface {
	Count(text string) int
}

// BudgetConfig 配置单轮注入的 token 预算。Separator 为层间分隔符（默认双换行）。
type BudgetConfig struct {
	MaxTokens int
	Separator string
}

// AssembleResult 是预算治理后的组装结果，附带可观测的裁剪决策。
type AssembleResult struct {
	Prompt      string
	TotalTokens int
	Kept        []string // 完整保留的层名（按显示顺序）
	Truncated   []string // 被截断保留的层名
	Dropped     []string // 被整层丢弃的层名
	OverBudget  bool     // 仅 Pinned 层即已超预算，无法进一步裁剪
}

// Assemble 按 token 预算组装分层提示词：
//   - Pinned 层（系统/安全）无条件保留，即使这会超出预算（此时 OverBudget=true）。
//   - 其余层按 Priority 从高到低填充剩余预算；跨越边界的那一层被截断，之后的低优先层被丢弃。
//   - 输出按层的原始显示顺序拼接，只包含被保留（完整或截断）的层。
func Assemble(layers []Layer, cfg BudgetConfig, counter TokenCounter) AssembleResult {
	sep := cfg.Separator
	if sep == "" {
		sep = "\n\n"
	}

	// 决定每层保留多少内容：index -> 保留后的内容（"" 表示丢弃）。
	kept := make([]string, len(layers))
	for i := range layers {
		kept[i] = layers[i].Content
	}

	res := AssembleResult{}

	// 1) Pinned 层无条件占用预算。
	pinnedTokens := 0
	for i, l := range layers {
		if l.Pinned {
			pinnedTokens += counter.Count(l.Content)
		} else {
			kept[i] = "" // 先清空非 pinned，稍后按优先级回填
		}
	}

	remaining := cfg.MaxTokens - pinnedTokens
	if remaining < 0 {
		remaining = 0
		res.OverBudget = true
	}

	// 2) 非 pinned 层按 Priority 从高到低填充剩余预算。
	order := nonPinnedByPriority(layers)
	for _, i := range order {
		content := layers[i].Content
		cost := counter.Count(content)
		if cost <= remaining {
			kept[i] = content
			remaining -= cost
			continue
		}
		if remaining > 0 {
			// 截断到剩余预算：能塞多少塞多少。
			truncated := truncateToTokens(content, remaining, counter)
			if strings.TrimSpace(truncated) != "" {
				kept[i] = truncated
				remaining = 0
				continue
			}
		}
		kept[i] = "" // 预算耗尽，整层丢弃
	}

	// 3) 按原始显示顺序拼接，记录裁剪决策。
	var parts []string
	for i, l := range layers {
		switch {
		case kept[i] == "":
			res.Dropped = append(res.Dropped, l.Name)
		case kept[i] != l.Content:
			res.Truncated = append(res.Truncated, l.Name)
			parts = append(parts, kept[i])
		default:
			res.Kept = append(res.Kept, l.Name)
			parts = append(parts, kept[i])
		}
	}

	res.Prompt = strings.Join(parts, sep)
	res.TotalTokens = counter.Count(res.Prompt)
	return res
}

// nonPinnedByPriority 返回非 pinned 层的下标，按 Priority 降序（同优先级按原始顺序稳定）。
func nonPinnedByPriority(layers []Layer) []int {
	idx := make([]int, 0, len(layers))
	for i, l := range layers {
		if !l.Pinned {
			idx = append(idx, i)
		}
	}
	// 稳定插入排序：保持同优先级的原始相对顺序。
	for i := 1; i < len(idx); i++ {
		for j := i; j > 0 && layers[idx[j]].Priority > layers[idx[j-1]].Priority; j-- {
			idx[j], idx[j-1] = idx[j-1], idx[j]
		}
	}
	return idx
}

// truncateToTokens 返回 content 的最长前缀，使其估算 token 数不超过 maxTokens。
// 通过对 rune 前缀长度二分查找实现（token 估算随长度单调不减）。
func truncateToTokens(content string, maxTokens int, counter TokenCounter) string {
	if maxTokens <= 0 {
		return ""
	}
	runes := []rune(content)
	if counter.Count(content) <= maxTokens {
		return content
	}
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if counter.Count(string(runes[:mid])) <= maxTokens {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo == 0 {
		return ""
	}
	return string(runes[:lo]) + "…"
}
