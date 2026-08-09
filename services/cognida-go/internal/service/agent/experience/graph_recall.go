package experience

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"cognida/internal/model/knowledge"
)

// graphCacheTTL 是经验子图在召回热路径上的缓存有效期。召回是应答前的同步 BeforeHook，
// 若每个 turn 都 GetGraph 全量拉租户经验子图，繁忙租户会给 first-token 叠加一次 DB 往返；
// 用短 TTL 缓存把频率降到「每租户每 TTL 至多一次」。经验召回是增强项，≤TTL 的陈旧完全可接受。
const graphCacheTTL = 60 * time.Second

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

	// 经验子图缓存（按租户）。热路径只读，短 TTL 内复用，避免每 turn 一次全图 DB 拉取。
	mu    sync.Mutex
	cache map[int64]graphCacheEntry
}

// graphCacheEntry 是单租户经验子图的一份缓存快照及其过期时刻。
type graphCacheEntry struct {
	data      *knowledge.GraphData
	expiresAt time.Time
}

// NewGraphRecaller 创建召回器。repo 为 nil 时 Recall 返回空（视为未接线，静默降级）。
func NewGraphRecaller(repo knowledge.GraphRepository) *GraphRecaller {
	return &GraphRecaller{repo: repo, cache: make(map[int64]graphCacheEntry)}
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
	data := g.cachedGraph(ctx, tenantID, ns)
	if data == nil {
		return nil
	}
	return rankExperiences(data, query, limit)
}

// cachedGraph 返回该租户经验子图：命中未过期缓存即复用，否则拉取并回填。
// DB 拉取刻意放在锁外（避免持锁做 I/O），冷缓存下并发 turn 至多各拉一次，可接受。
// 返回的 *GraphData 仅供 rankExperiences 只读消费（不修改图），并发读安全。
func (g *GraphRecaller) cachedGraph(ctx context.Context, tenantID int64, ns knowledge.NameSpace) *knowledge.GraphData {
	now := time.Now()

	g.mu.Lock()
	if e, ok := g.cache[tenantID]; ok && now.Before(e.expiresAt) {
		g.mu.Unlock()
		return e.data
	}
	g.mu.Unlock()

	data, err := g.repo.GetGraph(ctx, ns)
	if err != nil || data == nil {
		return nil
	}

	g.mu.Lock()
	g.cache[tenantID] = graphCacheEntry{data: data, expiresAt: now.Add(graphCacheTTL)}
	g.mu.Unlock()
	return data
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
	// 按节点 name 索引 experience 节点（RELATED_TO 的端点用的是 name）。同名经验并存于同一桶，
	// 不再互相覆盖丢失——命中该 name 的边为桶内每条经验同时计分。
	exps := make(map[string][]*expAcc)
	all := make([]*expAcc, 0)
	for _, n := range data.Node {
		if n == nil || n.EntityType != "experience" {
			continue
		}
		acc := &expAcc{node: n, id: experienceIDFromNodeID(n.ID), conf: confidenceFromProps(n.Properties)}
		exps[n.Name] = append(exps[n.Name], acc)
		all = append(all, acc)
	}
	if len(all) == 0 {
		return nil
	}

	// 去重 (经验名, 关联名)：GraphSink 对同一 tool/tag 可能发多条 RELATED_TO，
	// 去重后每个关联名对同名经验桶只计一次，避免重复边把该经验分数重复累加。
	seen := make(map[string]struct{})
	for _, rel := range data.Relation {
		if rel == nil || rel.Type != string(knowledge.RelationTypeRelatedTo) {
			continue
		}
		// 一端是 experience 节点，另一端是话题/工具名。
		var bucket []*expAcc
		var expName, otherName string
		if b, ok := exps[rel.Source]; ok {
			bucket, expName, otherName = b, rel.Source, rel.Target
		} else if b, ok := exps[rel.Target]; ok {
			bucket, expName, otherName = b, rel.Target, rel.Source
		} else {
			continue
		}
		otherName = strings.TrimSpace(otherName)
		if otherName == "" {
			continue
		}
		otherLower := strings.ToLower(otherName)
		dedupKey := expName + "\x00" + otherLower
		if _, dup := seen[dedupKey]; dup {
			continue
		}
		seen[dedupKey] = struct{}{}
		if strings.Contains(qLower, otherLower) {
			w := len([]rune(otherName))
			for _, acc := range bucket {
				acc.score += w
			}
		}
	}

	out := make([]*expAcc, 0, len(all))
	for _, a := range all {
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
