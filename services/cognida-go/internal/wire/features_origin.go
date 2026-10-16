package wire

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"sort"
	"strings"
	"time"

	collectionmodel "cognida/internal/model/collection"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/repository/mysql"
	"cognida/internal/service/agent/resultstore"
	"cognida/internal/service/datasource"
	"cognida/internal/service/lifecycle"
	"gorm.io/gorm"
)

func validateCollectionSourceOwner(ctx context.Context, db *gorm.DB, p l.Principal, run string) error {
	var count int64
	err := db.WithContext(ctx).Table("collection_runs r").Joins("JOIN collection_tasks t ON t.id=r.task_id AND t.tenant_id=r.tenant_id").Where("r.id=? AND r.tenant_id=? AND t.user_id=?", run, p.TenantID, p.UserID).Count(&count).Error
	if err != nil || count != 1 {
		return l.ErrForbidden
	}
	return nil
}

// Immutable bytes do not confer permanent access. This checks the source
// handle saved at acquisition, without silently refetching a different snapshot.
func validateFeatureSourceOrigin(ctx context.Context, db *gorm.DB, source *datasource.Service, results resultstore.Store, p l.Principal, ref l.DataAssetRef, authorizeEvaluation evaluationSourceAuthorizer, feedbackGuards ...knowledgeFeedbackGuard) error {
	if ref.Provenance.SourceOwnerID != p.UserID || len(ref.Provenance.AccessRequest) == 0 || len(ref.Provenance.AccessRequest) > 8192 {
		return l.ErrForbidden
	}
	var req lifecycle.SourceRequest
	if json.Unmarshal(ref.Provenance.AccessRequest, &req) != nil || req.Kind != ref.Provenance.SourceType {
		return l.ErrForbidden
	}
	switch req.Kind {
	case "knowledge":
		doc, err := mysql.NewKnowledgeRepository(db).FindByID(ctx, req.KnowledgeID)
		if err != nil || doc == nil || doc.TenantID != p.TenantID || doc.DeletedAt != nil || !doc.IsEnabled() {
			return l.ErrForbidden
		}
		kb, err := mysql.NewKnowledgeBaseRepository(db, true).FindByIDForTenant(ctx, doc.KnowledgeBaseID, p.TenantID)
		if err != nil || kb == nil || kb.DeletedAt != nil || !kb.IsActive() || (!kb.IsPublic && kb.UserID != p.UserID) {
			return l.ErrForbidden
		}
		if err = knowledgeFeedbackOrigin(ctx, db, p, doc.KnowledgeBaseID, doc.ID, ref.Provenance.SourceVersion, feedbackGuards...); err != nil {
			return err
		}
		input, err := knowledgeSourceInput(ctx, db, p, kb.ID, doc.ID)
		if err != nil {
			return l.ErrForbidden
		}
		body, err := json.Marshal(input)
		if err != nil {
			return l.ErrForbidden
		}
		digest := sha256.Sum256(body)
		if hex.EncodeToString(digest[:]) != ref.Provenance.Digest {
			return l.ErrForbidden
		}
	case "datasource":
		ds, err := source.Get(ctx, p.TenantID, req.DatasourceID)
		if err != nil || ds == nil || !availableFeatureDatasource(ds.Status) || ds.UpdatedAt.UTC().Format(time.RFC3339Nano) != ref.Provenance.SourceVersion {
			return l.ErrForbidden
		}
		if err = validateDatasourceCatalogOrigin(ctx, db, p, req, ref.Provenance.SourceID); err != nil {
			return err
		}
	case "collection":
		if err := validateCollectionSourceOwner(ctx, db, p, req.RunID); err != nil {
			return err
		}
		items, err := mysql.NewCollectionRepository(db).ListItems(ctx, p.TenantID, req.RunID)
		if err != nil {
			return l.ErrForbidden
		}
		found := false
		for _, item := range items {
			if item.ID == req.ItemID && item.NormalizedObjectKey != "" && item.ID == ref.Provenance.SourceID && item.ContentHash == ref.Provenance.SourceVersion && (item.Status == collectionmodel.ItemStatusCandidate || item.Status == collectionmodel.ItemStatusAccepted) {
				found = true
			}
		}
		if !found {
			return l.ErrForbidden
		}
	case "result":
		session, err := mysql.NewSessionRepository(db).FindByID(ctx, req.SessionID)
		if err != nil || session == nil || session.TenantID != p.TenantID || session.UserID != p.UserID {
			return l.ErrForbidden
		}
		if _, err = exactLifecycleResult(ctx, results, resultstore.OwnerKey(p.TenantID, req.SessionID), req.ResultID); err != nil {
			return l.ErrForbidden
		}
	case "evaluation":
		if authorizeEvaluation == nil || authorizeEvaluation(ctx, p, req.RunID) != nil {
			return l.ErrForbidden
		}
		task, err := mysql.NewEvaluationTaskRepository(db).FindByID(ctx, req.RunID)
		if err != nil || task == nil || task.TenantID != p.TenantID || task.UserID != p.UserID {
			return l.ErrForbidden
		}
	case "web":
		u, err := url.Parse(req.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return l.ErrForbidden
		}
	default:
		return l.ErrForbidden
	}
	return nil
}

// Acquisition and later reads use the same ordered source body. An edited,
// disabled or removed chunk invalidates descendants of the old snapshot.
func knowledgeSourceInput(ctx context.Context, db *gorm.DB, p l.Principal, kb, document string) (l.Input, error) {
	chunks, err := mysql.NewChunkRepository(db).FindByKnowledgeID(ctx, document, true)
	if err != nil {
		return l.Input{}, err
	}
	// Index alone is not unique after edits/imports; use an opaque ID tie-break.
	sort.SliceStable(chunks, func(i, j int) bool {
		if chunks[i] == nil {
			return false
		}
		if chunks[j] == nil {
			return true
		}
		if chunks[i].ChunkIndex == chunks[j].ChunkIndex {
			return chunks[i].ID < chunks[j].ID
		}
		return chunks[i].ChunkIndex < chunks[j].ChunkIndex
	})
	var body strings.Builder
	for _, chunk := range chunks {
		if chunk == nil || chunk.DeletedAt != nil || chunk.TenantID != p.TenantID || chunk.KnowledgeBaseID != kb {
			continue
		}
		if body.Len()+len(chunk.Content)+1 > 800000 {
			return l.Input{}, l.ErrInvalid
		}
		body.WriteString(chunk.Content)
		body.WriteByte('\n')
	}
	return l.Input{Kind: "text", Text: body.String()}, nil
}
