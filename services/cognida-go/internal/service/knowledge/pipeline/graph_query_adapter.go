package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"cognida/internal/model/knowledge"
	domainrag "cognida/internal/model/rag"
)

type graphQueryAdapter struct {
	repo knowledge.GraphQueryRepository
}

func NewGraphQueryAdapter(repo knowledge.GraphQueryRepository) domainrag.GraphQueryRepository {
	if repo == nil {
		return nil
	}
	return &graphQueryAdapter{repo: repo}
}

func graphDocuments(chunks []*knowledge.Chunk) []*domainrag.Document {
	out := make([]*domainrag.Document, 0, len(chunks))
	for _, c := range chunks {
		if c == nil || !c.IsEnabled || c.DeletedAt != nil {
			continue
		}
		metadata := map[string]interface{}{}
		if c.Metadata != nil {
			_ = json.Unmarshal([]byte(*c.Metadata), &metadata)
		}
		out = append(out, &domainrag.Document{ChunkID: c.ID, KnowledgeID: c.KnowledgeID, KnowledgeBaseID: c.KnowledgeBaseID, Content: c.Content, ChunkIndex: c.ChunkIndex, Score: 1, MatchType: "graph", Metadata: metadata})
	}
	return out
}

func (a *graphQueryAdapter) GetChunksByGraphNodes(ctx context.Context, kb string, names []string) ([]*domainrag.Document, error) {
	chunks, err := a.repo.GetChunksByGraphNodes(ctx, kb, names)
	if err != nil {
		return nil, err
	}
	return graphDocuments(chunks), nil
}

// GetScopedChunks checks the relational owner even if graph metadata contains an
// obsolete or corrupt reference into a different namespace.
func (a *graphQueryAdapter) GetScopedChunks(ctx context.Context, tenant, kb string, ids []string) ([]*domainrag.Document, error) {
	tenantID, err := strconv.ParseInt(tenant, 10, 64)
	if err != nil || tenantID <= 0 || kb == "" {
		return nil, fmt.Errorf("graph retrieval requires valid tenant and KB")
	}
	chunks, err := a.repo.GetChunksByIDs(ctx, kb, ids)
	if err != nil {
		return nil, err
	}
	allowed := map[string]bool{}
	for _, id := range ids {
		allowed[id] = true
	}
	scoped := make([]*knowledge.Chunk, 0, len(chunks))
	for _, c := range chunks {
		if c != nil && c.TenantID == tenantID && c.KnowledgeBaseID == kb && allowed[c.ID] {
			scoped = append(scoped, c)
		}
	}
	return graphDocuments(scoped), nil
}

func (a *graphQueryAdapter) GetKnowledgeByGraphNodes(ctx context.Context, kb string, names []string) (interface{}, error) {
	return a.repo.GetKnowledgeByGraphNodes(ctx, kb, names)
}

func (a *graphQueryAdapter) GetGraphStats(ctx context.Context, kb string) (*domainrag.GraphStatistics, error) {
	stats, err := a.repo.GetGraphStats(ctx, kb)
	if err != nil {
		return nil, err
	}
	if stats == nil {
		return nil, fmt.Errorf("graph statistics unavailable")
	}
	return &domainrag.GraphStatistics{NodeCount: stats.NodeCount, RelationCount: stats.RelationCount, AvgDegree: stats.AvgDegree}, nil
}
