package experience

import (
	"strings"

	domain_experience "link/internal/model/agent/experience"
)

// Deduplicator 依据经验的 tools+tags 特征集做「近重复」判定：同类套路（工具/标签高度重合）
// 只保留一条 skill，后来者复用既有 skill 而非反复生成一堆内容雷同的 SKILL.md。
//
// 相似度用 Jaccard（交集/并集），阈值可配；threshold<=0 时关闭去重。
type Deduplicator struct {
	threshold float64
}

// NewDeduplicator 创建去重器。threshold 为 tools+tags 的 Jaccard 相似度阈值（如 0.8）。
func NewDeduplicator(threshold float64) *Deduplicator {
	return &Deduplicator{threshold: threshold}
}

// Enabled 是否启用去重（阈值>0 且大于 0 时才生效）。
func (d *Deduplicator) Enabled() bool {
	return d != nil && d.threshold > 0
}

// FindDuplicate 在候选经验中找与 exp 特征相似度达到阈值、且相似度最高者；无则返回 nil。
// 会跳过同一会话与自身；exp 无 tools/tags 特征时不做合并（信息不足，避免误并）。
func (d *Deduplicator) FindDuplicate(
	exp *domain_experience.Experience, candidates []*domain_experience.Experience,
) *domain_experience.Experience {
	if !d.Enabled() || exp == nil {
		return nil
	}
	target := featureSet(exp)
	if len(target) == 0 {
		return nil
	}
	var best *domain_experience.Experience
	bestSim := 0.0
	for _, c := range candidates {
		if c == nil || c.SessionID == exp.SessionID {
			continue
		}
		sim := jaccard(target, featureSet(c))
		if sim >= d.threshold && sim > bestSim {
			bestSim = sim
			best = c
		}
	}
	return best
}

// featureSet 把 tools+tags 归一化（小写去空白）为去重集合。
func featureSet(exp *domain_experience.Experience) map[string]struct{} {
	set := make(map[string]struct{}, len(exp.Tools)+len(exp.Tags))
	add := func(items []string) {
		for _, it := range items {
			it = strings.ToLower(strings.TrimSpace(it))
			if it != "" {
				set[it] = struct{}{}
			}
		}
	}
	add(exp.Tools)
	add(exp.Tags)
	return set
}

// jaccard 计算两个集合的 Jaccard 相似度 |A∩B|/|A∪B|；任一为空返回 0。
func jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for k := range a {
		if _, ok := b[k]; ok {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}
