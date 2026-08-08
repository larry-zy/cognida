package experience

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"link/internal/model/knowledge"
)

// RecalledExperience 是从图谱召回的一条历史经验（只读投影，供首答注入）。
type RecalledExperience struct {
	Title    string
	Problem  string
	Solution string
	// Score 为匹配得分（命中的话题/工具名的字符权重和），用于排序与召回质量观测/调参。
	Score int
	// Confidence 为该经验沉淀时的质量置信度 0~100（缺失按 100 中性处理），参与排序降权。
	Confidence int
}

// GraphRecaller 从知识图谱召回与当前问题相关的历史经验，是 GraphSink 的读侧对偶。
//
// 召回口径与沉淀严格同源：同一租户全域命名空间（kb_id=""），沿 RELATED_TO 邻接
// 把「问题里出现的话题/工具名」接回其关联的 experience 节点。整幅经验子图由 GetGraph
// 一次取回（该命名空间仅有经验沉淀写入，天然有界），匹配与排序均在内存内完成——
// 纯函数、可单测，不依赖向量、不触碰 experiences 表，与「语义经验只进图谱简单沉淀」一致。
type GraphRecaller struct {
	repo knowledge.GraphRepository
}

// NewGraphRecaller 创建召回器。repo 为 nil 时 Recall 返回空（视为未接线，静默降级）。
func NewGraphRecaller(repo knowledge.GraphRepository) *GraphRecaller {
	return &GraphRecaller{repo: repo}
}

// Recall 返回与 query 最相关的至多 limit 条历史经验，按匹配得分降序、同分按新近（ID 大）优先。
// limit <= 0 时回落默认 3。任何图谱错误/空图都返回空切片而非报错——召回是增强项，绝不阻断主流程。
func (g *GraphRecaller) Recall(ctx context.Context, tenantID int64, query string, limit int) []RecalledExperience {
	if g == nil || g.repo == nil {
		return nil
	}
	if strings.TrimSpace(query) == "" {
		return nil
	}
	if limit <= 0 {
		limit = 3
	}
	ns := knowledge.NameSpace{TenantID: strconv.FormatInt(tenantID, 10), KnowledgeBaseID: ""}
	data, err := g.repo.GetGraph(ctx, ns)
	if err != nil || data == nil {
		return nil
	}
	return rankExperiences(data, query, limit)
}

// rankExperiences 在经验子图内为每个 experience 节点打分并取 topN。纯函数：不触网、可单测。
//
// 打分口径：沿 RELATED_TO 邻接，若某 experience 关联的话题/工具名作为子串出现在问题里，
// 则为该经验加分，权重取该名的 rune 长度——奖励更具体的话题（如「季节性大促分析」优先于「SQL」）。
func rankExperiences(data *knowledge.GraphData, query string, limit int) []RecalledExperience {
	qLower := strings.ToLower(query)

	type expAcc struct {
		node  *knowledge.GraphNode
		id    int64
		score int
		conf  int
	}
	// 按节点 name 索引 experience 节点（RELATED_TO 的端点用的是 name）。
	exps := make(map[string]*expAcc)
	for _, n := range data.Node {
		if n == nil || n.EntityType != "experience" {
			continue
		}
		exps[n.Name] = &expAcc{node: n, id: experienceIDFromNodeID(n.ID), conf: confidenceFromProps(n.Properties)}
	}
	if len(exps) == 0 {
		return nil
	}

	for _, rel := range data.Relation {
		if rel == nil || rel.Type != string(knowledge.RelationTypeRelatedTo) {
			continue
		}
		// 一端是 experience 节点，另一端是话题/工具名。
		var acc *expAcc
		var otherName string
		if a, ok := exps[rel.Source]; ok {
			acc, otherName = a, rel.Target
		} else if a, ok := exps[rel.Target]; ok {
			acc, otherName = a, rel.Source
		} else {
			continue
		}
		otherName = strings.TrimSpace(otherName)
		if otherName == "" {
			continue
		}
		if strings.Contains(qLower, strings.ToLower(otherName)) {
			acc.score += len([]rune(otherName))
		}
	}

	out := make([]*expAcc, 0, len(exps))
	for _, a := range exps {
		if a.score > 0 {
			out = append(out, a)
		}
	}
	// 排序键 = 匹配得分 × 置信度：先按质量加权得分降序，相等再按新近（ID 大）优先。
	// 匹配（score>0）作为入选硬门（上面已过滤），置信度只影响相对排序，不把命中项直接淘汰出局。
	sort.SliceStable(out, func(i, j int) bool {
		wi, wj := out[i].score*out[i].conf, out[j].score*out[j].conf
		if wi != wj {
			return wi > wj
		}
		return out[i].id > out[j].id // 同分：新近沉淀优先
	})
	if len(out) > limit {
		out = out[:limit]
	}

	res := make([]RecalledExperience, 0, len(out))
	for _, a := range out {
		res = append(res, RecalledExperience{
			Title:      strings.TrimPrefix(a.node.Name, "经验："),
			Problem:    a.node.Properties["problem"],
			Solution:   a.node.Properties["solution"],
			Score:      a.score,
			Confidence: a.conf,
		})
	}
	return res
}

// confidenceFromProps 从节点属性解析置信度 0~100；缺失/非法/越界一律按 100（中性，不降权）处理，
// 保证「置信度写入前沉淀的历史经验」不因缺该属性而被误降权。
func confidenceFromProps(props map[string]string) int {
	v, ok := props["confidence"]
	if !ok {
		return 100
	}
	c, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || c < 0 || c > 100 {
		return 100
	}
	return c
}

// experienceIDFromNodeID 从 "exp_<id>" 解析经验 ID（用于同分时新近优先）；解析失败回落 0。
func experienceIDFromNodeID(nodeID string) int64 {
	id, _ := strconv.ParseInt(strings.TrimPrefix(nodeID, "exp_"), 10, 64)
	return id
}
