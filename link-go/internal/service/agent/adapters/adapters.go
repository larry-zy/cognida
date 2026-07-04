// Package adapters 把已接线的领域服务（检索器 / 图谱服务 / 知识库仓储）适配为
// Agent 工具层（service/agent/tools）声明的服务接口，供组合根（main.go）注入。
//
// 关键设计：检索范围（哪些知识库）不由 LLM 决定，而由用户在会话入口选定并经
// ctx（AllowedKBIDs）透传；图谱增强是否开启同样经 ctx（GraphEnabled）透传。
// 适配器在此读取 ctx 完成 scope 强制，替代旧的“把知识库范围塞进 query 文本前缀”hack。
package adapters

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	agentctx "link/internal/model/agent"
	"link/internal/model/knowledge"
	"link/internal/model/rag"
	svcknowledge "link/internal/service/knowledge"
	"link/internal/service/agent/tools"
)

// ========================================
// RAG 检索适配器
// ========================================

// RAGRetrieverAdapter 将基础检索器 rag.Retriever 适配为 tools.RAGQueryService。
// 检索范围来自 ctx AllowedKBIDs（用户选定），为空时回退为“全部已启用知识库”。
type RAGRetrieverAdapter struct {
	retriever rag.Retriever
	kbRepo    knowledge.KnowledgeBaseRepository
}

// NewRAGRetrieverAdapter 创建 RAG 检索适配器
func NewRAGRetrieverAdapter(retriever rag.Retriever, kbRepo knowledge.KnowledgeBaseRepository) *RAGRetrieverAdapter {
	return &RAGRetrieverAdapter{retriever: retriever, kbRepo: kbRepo}
}

// Query 实现 tools.RAGQueryService。ctxAny 由工具层透传，断言回 context.Context 读取 scope。
func (a *RAGRetrieverAdapter) Query(ctxAny interface{}, req *tools.RAGQueryRequest) (*tools.RAGQueryResult, error) {
	ctx := asContext(ctxAny)
	if a.retriever == nil {
		return nil, fmt.Errorf("检索器未接线")
	}

	tenantID := agentctx.MustGetTenantID(ctx)
	tenantStr := strconv.FormatInt(tenantID, 10)

	// 检索范围：用户选定的 KB 与租户内已启用 KB 求交（租户边界强制）；未选定则取全部已启用 KB。
	kbIDs := resolveKBScope(ctx, a.kbRepo, tenantID)
	if len(kbIDs) == 0 {
		return &tools.RAGQueryResult{
			Query:     req.Query,
			Chunks:    []tools.DocumentChunk{},
			Count:     0,
			HasAnswer: false,
		}, nil
	}

	opts := &rag.RetrieveOptions{
		TopK:                req.TopK,
		SimilarityThreshold: req.MinScore,
		RerankEnabled:       req.EnableRerank,
		GraphEnabled:        agentctx.IsGraphEnabled(ctx),
		RetrievalMode:       req.RetrievalMode,
	}

	var all []tools.DocumentChunk
	for _, kbID := range kbIDs {
		resp, err := a.retrieveByMode(ctx, tenantStr, kbID, req.Query, req.RetrievalMode, opts)
		if err != nil || resp == nil {
			continue // 单个知识库失败不阻断整体检索
		}
		for _, d := range resp.Results {
			all = append(all, tools.DocumentChunk{
				Content:    d.Content,
				Score:      float64(d.Score),
				Source:     d.KnowledgeBaseID,
				ChunkIndex: d.ChunkIndex,
				Metadata:   d.Metadata,
			})
		}
	}

	// 跨知识库合并后按分数降序，取 TopK。
	sort.Slice(all, func(i, j int) bool { return all[i].Score > all[j].Score })
	if req.TopK > 0 && len(all) > req.TopK {
		all = all[:req.TopK]
	}

	return &tools.RAGQueryResult{
		Query:           req.Query,
		Chunks:          all,
		Count:           len(all),
		KnowledgeBaseID: strings.Join(kbIDs, ","),
		HasAnswer:       len(all) > 0,
	}, nil
}

// retrieveByMode 按检索模式选择基础检索器方法。
func (a *RAGRetrieverAdapter) retrieveByMode(ctx context.Context, tenantID, kbID, query, mode string, opts *rag.RetrieveOptions) (*rag.RetrieveResponse, error) {
	switch mode {
	case "vector":
		return a.retriever.VectorRetrieve(ctx, tenantID, kbID, query, opts)
	case "bm25":
		return a.retriever.BM25Retrieve(ctx, tenantID, kbID, query, opts)
	default:
		return a.retriever.HybridRetrieve(ctx, tenantID, kbID, query, opts)
	}
}

// ========================================
// 图谱查询适配器
// ========================================

// GraphSearchAdapter 将 knowledge.GraphService 适配为 tools.GraphQueryService。
// 同样以 ctx AllowedKBIDs 限定命名空间；工具层已按 ctx GraphEnabled 做开关门控。
type GraphSearchAdapter struct {
	graphService *svcknowledge.GraphService
	kbRepo       knowledge.KnowledgeBaseRepository
}

// NewGraphSearchAdapter 创建图谱查询适配器
func NewGraphSearchAdapter(graphService *svcknowledge.GraphService, kbRepo knowledge.KnowledgeBaseRepository) *GraphSearchAdapter {
	return &GraphSearchAdapter{graphService: graphService, kbRepo: kbRepo}
}

// GraphQuery 实现 tools.GraphQueryService。
func (a *GraphSearchAdapter) GraphQuery(ctxAny interface{}, req *tools.GraphQueryRequest) (*tools.GraphQueryResult, error) {
	ctx := asContext(ctxAny)
	if a.graphService == nil {
		return nil, fmt.Errorf("图谱服务未接线")
	}

	tenantID := agentctx.MustGetTenantID(ctx)
	tenantStr := strconv.FormatInt(tenantID, 10)

	kbIDs := resolveKBScope(ctx, a.kbRepo, tenantID)

	nodes := extractQueryTerms(req.Query)

	entities := make([]tools.GraphEntity, 0)
	relations := make([]tools.GraphRelation, 0)
	seenEntity := map[string]bool{}
	seenRel := map[string]bool{}

	for _, kbID := range kbIDs {
		ns := knowledge.NameSpace{TenantID: tenantStr, KnowledgeBaseID: kbID}
		data, err := a.graphService.SearchNode(ctx, ns, nodes)
		if err != nil || data == nil {
			continue
		}
		for _, n := range data.Node {
			if n == nil || seenEntity[n.Name] {
				continue
			}
			seenEntity[n.Name] = true
			entities = append(entities, tools.GraphEntity{
				ID:         n.ID,
				Name:       n.Name,
				Type:       n.EntityType,
				Properties: stringMapToAny(n.Properties),
			})
		}
		for _, r := range data.Relation {
			if r == nil {
				continue
			}
			key := r.Source + "->" + r.Type + "->" + r.Target
			if seenRel[key] {
				continue
			}
			seenRel[key] = true
			relations = append(relations, tools.GraphRelation{
				Source:      r.Source,
				Target:      r.Target,
				Type:        r.Type,
				Description: r.Description,
				Strength:    r.Strength,
			})
		}
	}

	// 应用 top_k：截断实体，并只保留两端仍在保留集合内的关系。
	if req.TopK > 0 && len(entities) > req.TopK {
		entities = entities[:req.TopK]
		kept := make(map[string]bool, len(entities))
		for _, e := range entities {
			kept[e.Name] = true
		}
		filtered := relations[:0]
		for _, r := range relations {
			if kept[r.Source] && kept[r.Target] {
				filtered = append(filtered, r)
			}
		}
		relations = filtered
	}

	return &tools.GraphQueryResult{
		Query:           req.Query,
		Entities:        entities,
		Relations:       relations,
		Count:           len(entities),
		KnowledgeBaseID: strings.Join(kbIDs, ","),
		HasAnswer:       len(entities) > 0,
	}, nil
}

// ========================================
// 公共辅助
// ========================================

// asContext 将工具层透传的 interface{} 断言回 context.Context，兜底 Background。
func asContext(ctxAny interface{}) context.Context {
	if ctx, ok := ctxAny.(context.Context); ok && ctx != nil {
		return ctx
	}
	return context.Background()
}

// resolveKBScope 计算本次检索的知识库范围，按「知识库选择模式」分派，并始终强制租户边界。
//
// 三种模式（ctx KBScopeMode，默认 manual）：
//   - manual：范围完全由用户勾选决定，忽略 Agent 的 kb_route 路由；等价历史行为。
//   - hybrid：以「用户勾选 ∩ 租户已启用」为允许上界，Agent 经 kb_route 在上界内自选。
//   - auto  ：以「租户全部已启用」为允许上界（忽略用户勾选），Agent 经 kb_route 在上界内自选。
//
// 无论何种模式，最终范围都由 resolveKBScope 与允许上界求交——Agent 越权/超范围/禁用/陌生的
// KB ID 一律被丢弃，这是重新引入「让 AI 选库」而仍保持租户边界安全的关键。
//
// 注：本应用 KB 授权边界为租户级（列表接口 ListKnowledgeBases 即按租户返回），跨租户由
// 向量库/图谱的 tenant 过滤兜底；此处再做一层显式校验，避免对越权/陌生 ID 发起无效检索。
func resolveKBScope(ctx context.Context, kbRepo knowledge.KnowledgeBaseRepository, tenantID int64) []string {
	if tenantID <= 0 {
		return nil // 缺少有效租户信息时不检索，避免落到 tenant=0
	}
	mode := agentctx.MustGetKBScopeMode(ctx)
	upper := upperBoundKBScope(ctx, kbRepo, tenantID, mode)

	// 手动模式：用户全权，忽略 Agent 路由，直接返回上界。
	if mode == agentctx.KBScopeModeManual {
		return upper
	}

	// 结合 / 智能：Agent 经 kb_route 声明的聚焦范围，与允许上界求交。
	var route []string
	if sel, ok := agentctx.GetRouteSelection(ctx); ok {
		route = sel.Get()
	}
	if len(route) == 0 {
		return upper // Agent 未路由 → 检索整个上界
	}
	return intersect(route, upper)
}

// upperBoundKBScope 计算某模式下的「允许上界」知识库集合：
//   - auto：租户全部已启用 KB（忽略用户勾选）；
//   - 其他（manual/hybrid）：用户勾选 ∩ 已启用；未勾选 → 全部已启用；
//     勾选非空但枚举为空时，仓储缺失则信任勾选、仓储可用则视为无可检索 KB（沿用历史退化规则）。
func upperBoundKBScope(ctx context.Context, kbRepo knowledge.KnowledgeBaseRepository, tenantID int64, mode string) []string {
	enabled := enabledKBIDs(ctx, kbRepo, tenantID)
	if mode == agentctx.KBScopeModeAuto {
		return enabled
	}
	selected := agentctx.MustGetAllowedKBIDs(ctx)
	if len(selected) == 0 {
		return enabled // 未勾选 → 全部已启用
	}
	if len(enabled) == 0 {
		if kbRepo == nil {
			return selected // 无法校验，信任用户勾选
		}
		return nil // 仓储可用但无已启用 KB
	}
	return intersect(selected, enabled)
}

// intersect 返回 a 中同时出现在 b 的元素（保持 a 的顺序、去重）。
func intersect(a, b []string) []string {
	allowed := make(map[string]bool, len(b))
	for _, id := range b {
		allowed[id] = true
	}
	seen := make(map[string]bool, len(a))
	out := make([]string, 0, len(a))
	for _, id := range a {
		if allowed[id] && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// enabledKBIDs 枚举租户下全部已启用（未删除且 Status!=0）的知识库 ID。
func enabledKBIDs(ctx context.Context, kbRepo knowledge.KnowledgeBaseRepository, tenantID int64) []string {
	if kbRepo == nil {
		return nil
	}
	// 取足够大的单页覆盖租户全部 KB，避免选定的 KB 落在分页之外被静默丢弃。
	kbs, _, err := kbRepo.FindByTenantID(ctx, tenantID, 1, 1000)
	if err != nil {
		return nil
	}
	var ids []string
	for _, kb := range kbs {
		if kb.DeletedAt == nil && kb.Status != 0 {
			ids = append(ids, kb.ID)
		}
	}
	return ids
}

// extractQueryTerms 从查询里拆出候选实体名（按空白与常见标点切分），无有效词则退回整句。
func extractQueryTerms(query string) []string {
	fields := strings.FieldsFunc(query, func(r rune) bool {
		switch r {
		case ' ', '\t', '\n', '，', ',', '。', '、', '?', '？', '!', '！', '；', ';', '：', ':':
			return true
		}
		return false
	})
	terms := make([]string, 0, len(fields)+1)
	seen := map[string]bool{}
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" || seen[f] {
			continue
		}
		seen[f] = true
		terms = append(terms, f)
	}
	if len(terms) == 0 {
		if q := strings.TrimSpace(query); q != "" {
			terms = append(terms, q)
		}
	}
	return terms
}

// stringMapToAny 将 map[string]string 转为 map[string]interface{}（工具层属性类型）。
func stringMapToAny(m map[string]string) map[string]interface{} {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
