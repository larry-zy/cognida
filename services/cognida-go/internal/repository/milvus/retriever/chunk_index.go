package retriever

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cognida/internal/model/knowledge"
)

var _ knowledge.ChunkIndexReader = (*VectorRetriever)(nil)

func (r *VectorRetriever) QueryChunks(ctx context.Context, query knowledge.ChunkIndexQuery) ([]*knowledge.IndexedChunk, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("chunk index reader is unavailable")
	}
	opts, err := chunkIndexOptions(query)
	if err != nil {
		return nil, err
	}
	docs, err := r.Query(ctx, 0, opts)
	if err != nil {
		return nil, err
	}
	return indexedChunks(docs), nil
}

func indexedChunks(docs []*DocumentData) []*knowledge.IndexedChunk {
	result := make([]*knowledge.IndexedChunk, 0, len(docs))
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		result = append(result, &knowledge.IndexedChunk{
			ChunkID: doc.ChunkID, KnowledgeID: doc.KnowledgeID,
			KnowledgeBaseID: doc.KnowledgeBaseID, ChunkIndex: doc.ChunkIndex,
			Content: doc.Content, IsEnabled: doc.IsEnabled, TokenCount: doc.TokenCount,
		})
	}
	return result
}

func chunkIndexOptions(q knowledge.ChunkIndexQuery) (*QueryOptions, error) {
	if q.TenantID <= 0 || len(q.KBIDs) == 0 {
		return nil, fmt.Errorf("chunk index query requires tenant and authorized knowledge bases")
	}
	literal := func(ids []string) string {
		data, _ := json.Marshal(ids) // []string always marshals; escapes quotes/control characters.
		return string(data)
	}
	clauses := []string{fmt.Sprintf("tenant_id == %d", q.TenantID), "kb_id in " + literal(q.KBIDs)}
	if len(q.ChunkIDs) > 0 {
		clauses = append(clauses, "chunk_id in "+literal(q.ChunkIDs))
	}
	if len(q.KnowledgeIDs) > 0 {
		clauses = append(clauses, "knowledge_id in "+literal(q.KnowledgeIDs))
	}
	if q.EnabledOnly {
		clauses = append(clauses, "is_enabled == true")
	}
	return &QueryOptions{Expr: []string{strings.Join(clauses, " and ")}, Limit: q.Limit, Offset: q.Offset}, nil
}
