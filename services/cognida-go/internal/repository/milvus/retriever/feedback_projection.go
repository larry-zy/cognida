package retriever

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"regexp"
)

// FeedbackProjection queries actual index rows; a MySQL completed flag alone
// is insufficient evidence that a publication can be consumed.
func (a *VectorRepositoryAdapter) FeedbackProjection(ctx context.Context, tenant int64, kb, id string) (map[string]string, error) {
	if tenant <= 0 {
		return nil, fmt.Errorf("invalid tenant")
	}
	if _, e := uuid.Parse(id); e != nil {
		return nil, e
	}
	if len(kb) > 128 {
		return nil, fmt.Errorf("invalid kb")
	}
	// strconv-style quoted literals are not needed: these IDs are validated UUIDs.
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`).MatchString(kb) {
		return nil, fmt.Errorf("invalid kb")
	}
	var numeric int64
	_, _ = fmt.Sscanf(kb, "%d", &numeric)
	docs, e := a.retriever.Query(ctx, numeric, &QueryOptions{Expr: []string{fmt.Sprintf("tenant_id == %d and kb_id == '%s' and knowledge_id == '%s'", tenant, kb, id)}, Limit: 65})
	if e != nil {
		return nil, e
	}
	out := map[string]string{}
	for _, doc := range docs {
		if doc.TenantID != tenant || doc.KnowledgeID != id || doc.KnowledgeBaseID != kb {
			return nil, fmt.Errorf("projection scope mismatch")
		}
		out[doc.ChunkID] = doc.Content
	}
	return out, nil
}
