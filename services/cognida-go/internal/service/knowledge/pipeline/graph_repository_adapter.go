package rag

import (
	"cognida/internal/model/knowledge"
	domainrag "cognida/internal/model/rag"
	"context"
	"fmt"
)

// GraphSearcher is the read-only graph capability used by retrieval.
type GraphSearcher interface {
	SearchNode(context.Context, domainrag.NameSpace, []string) (*domainrag.GraphData, error)
}
type graphRepositoryAdapter struct{ repo knowledge.GraphRepository }

func NewGraphRepositoryAdapter(repo knowledge.GraphRepository) GraphSearcher {
	if repo == nil {
		return nil
	}
	return &graphRepositoryAdapter{repo: repo}
}
func (a *graphRepositoryAdapter) SearchNode(ctx context.Context, ns domainrag.NameSpace, names []string) (*domainrag.GraphData, error) {
	if ns.TenantID == "" || ns.KnowledgeBaseID == "" {
		return nil, fmt.Errorf("graph search requires tenant and knowledge base")
	}
	scope := knowledge.NameSpace{TenantID: ns.TenantID, KnowledgeBaseID: ns.KnowledgeBaseID}
	data := &knowledge.GraphData{}
	seen := map[string]bool{}
	if len(names) > 12 {
		names = names[:12]
	}
	for _, name := range names {
		nodes, err := a.repo.SearchNodes(ctx, scope, name, &knowledge.NodeQueryOptions{Limit: 5, WithChunks: true})
		if err != nil {
			return nil, err
		}
		for _, node := range nodes {
			if node == nil || seen[node.ID] {
				continue
			}
			seen[node.ID] = true
			data.Node = append(data.Node, node)
			// One-hop expansion is capped and retains the exact namespace.
			neighbors, err := a.repo.GetNeighbors(ctx, scope, node.Name, &knowledge.RelationQueryOptions{Limit: 5})
			if err != nil {
				return nil, err
			}
			if neighbors != nil {
				data.Node = append(data.Node, neighbors.Neighbors...)
				data.Relation = append(data.Relation, neighbors.Relations...)
			}
		}
	}
	out := &domainrag.GraphData{}
	if data == nil {
		return out, nil
	}
	for _, n := range data.Node {
		if n != nil {
			out.Node = append(out.Node, &domainrag.GraphNode{ID: n.ID, Name: n.Name, EntityType: n.EntityType, Attributes: n.Attributes, Chunks: n.Chunks, Properties: n.Properties})
		}
	}
	for _, r := range data.Relation {
		if r != nil {
			out.Relation = append(out.Relation, &domainrag.GraphRelation{ID: r.ID, Source: r.Source, Target: r.Target, Type: r.Type, Strength: r.Strength, Weight: r.Weight, ChunkIDs: r.ChunkIDs, Properties: r.Properties, CombinedDegree: r.CombinedDegree})
		}
	}
	return out, nil
}
