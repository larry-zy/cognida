package termgrounding

import (
	"context"
	"strconv"

	"link/internal/model/knowledge"
)

// graphSearchLimit 是接地时图谱节点检索的候选上限（术语接地只需少量近义节点）。
const graphSearchLimit = 5

// similarRelationTypes 是接地视为「同义/别名」的关系类型：命中这些边的邻居才作为别名候选，
// 避免把弱相关邻居（如 DEPENDS_ON）误当同义词注入。
var similarRelationTypes = []string{
	string(knowledge.RelationTypeSimilarTo),
	string(knowledge.RelationTypeRelatedTo),
	string(knowledge.RelationTypeBelongsTo),
}

// GraphAdapter 用既有 knowledge.GraphRepository 适配 GraphPort，
// 复用血缘/知识图谱做术语接地，而非另建一套术语表。
type GraphAdapter struct {
	repo knowledge.GraphRepository
	kbID string // 可选：限定知识库；空则按租户全域检索
}

var _ GraphPort = (*GraphAdapter)(nil)

// NewGraphAdapter 构造图谱适配器；repo 为 nil 时返回 nil（调用方据此退化为仅模型内接地）。
func NewGraphAdapter(repo knowledge.GraphRepository, kbID string) *GraphAdapter {
	if repo == nil {
		return nil
	}
	return &GraphAdapter{repo: repo, kbID: kbID}
}

// ResolveAliases 实现 GraphPort：全文检索命中术语的节点，返回节点自身名与其
// 同义/相关邻居名，作为回落模型内索引的别名候选。
func (a *GraphAdapter) ResolveAliases(ctx context.Context, tenantID int64, term string) ([]string, error) {
	ns := knowledge.NameSpace{
		TenantID:        strconv.FormatInt(tenantID, 10),
		KnowledgeBaseID: a.kbID,
	}
	nodes, err := a.repo.SearchNodes(ctx, ns, term, &knowledge.NodeQueryOptions{Limit: graphSearchLimit})
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var aliases []string
	push := func(name string) {
		n := norm(name)
		if n == "" || seen[n] {
			return
		}
		seen[n] = true
		aliases = append(aliases, name)
	}

	for _, node := range nodes {
		push(node.Name)
		// 沿同义/相关边取邻居名，作为进一步的别名候选。
		res, nerr := a.repo.GetNeighbors(ctx, ns, node.Name, &knowledge.RelationQueryOptions{
			Types: similarRelationTypes,
			Limit: graphSearchLimit,
		})
		if nerr != nil || res == nil {
			continue
		}
		for _, nb := range res.Neighbors {
			push(nb.Name)
		}
	}
	return aliases, nil
}
