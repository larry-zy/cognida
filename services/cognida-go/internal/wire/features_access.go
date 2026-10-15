package wire

import (
	catalogmodel "cognida/internal/model/catalog"
	collectionmodel "cognida/internal/model/collection"
	dsmodel "cognida/internal/model/datasource"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/model/objectstore"
	"cognida/internal/repository/mysql"
	"cognida/internal/service/agent/resultstore"
	"cognida/internal/service/catalog"
	"cognida/internal/service/collection"
	"cognida/internal/service/collection/fetcher"
	"cognida/internal/service/datasource"
	"cognida/internal/service/knowledge"
	"cognida/internal/service/lifecycle"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"io"
	"strings"
	"time"
)

func featureAuthorizer(db *gorm.DB) lifecycle.Authorizer {
	members := mysql.NewTenantUserRepository(db)
	tenants := mysql.NewTenantRepository(db)
	users := mysql.NewUserRepository(db)
	return func(ctx context.Context, p l.Principal, scope string) error {
		user, e := users.FindByID(ctx, p.UserID)
		if e != nil || user == nil || user.Status != 1 {
			return l.ErrForbidden
		}
		member, e := members.Find(ctx, p.TenantID, p.UserID)
		if e != nil || member == nil || member.Status != "active" {
			return l.ErrForbidden
		}
		tenant, e := tenants.FindByID(ctx, p.TenantID)
		if e != nil || tenant == nil || tenant.Status != "active" {
			return l.ErrForbidden
		}
		if scope == "read" {
			return nil
		}
		if member.Role != "owner" && member.Role != "admin" {
			return l.ErrForbidden
		}
		return nil
	}
}
func featureAccess(db *gorm.DB, source *datasource.Service, catalogService *catalog.Service, fetcher collectionmodel.Fetcher, store objectstore.Store, results resultstore.Store) lifecycle.AccessFunc {
	return func(ctx context.Context, p l.Principal, req lifecycle.SourceRequest) (l.Input, l.Provenance, l.PolicySnapshot, error) {
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		input := l.Input{}
		origin := l.Provenance{SourceType: req.Kind}
		policy := l.PolicySnapshot{Sensitivity: "unknown", Scopes: []string{"read"}, Version: "1"}
		switch req.Kind {
		case "web":
			response, e := fetcher.Fetch(ctx, collectionmodel.FetchRequest{URL: req.URL, MaxResponseBytes: 2 << 20})
			if e != nil {
				if invalidWebSource(e) {
					return input, origin, policy, fmt.Errorf("%w: %v", l.ErrInvalid, e)
				}
				return input, origin, policy, e
			}
			content, e := (collection.TextNormalizer{}).Normalize(response.ContentType, response.Body)
			if e != nil {
				return input, origin, policy, e
			}
			input = l.Input{Kind: "text", Text: content}
			origin.SourceID = response.CanonicalURL
			origin.URI = response.CanonicalURL
		case "datasource":
			if _, e := source.Get(ctx, p.TenantID, req.DatasourceID); e != nil {
				if errors.Is(e, datasource.ErrNotFound) {
					return input, origin, policy, l.ErrNotFound
				}
				return input, origin, policy, e
			}
			conn, ds, e := source.ConnectionManager().Acquire(ctx, p.TenantID, req.DatasourceID)
			if e != nil {
				return input, origin, policy, e
			}
			schema := ds.DatabaseName
			if ds.Type == dsmodel.TypePostgres {
				if e = conn.QueryRowContext(ctx, "SELECT current_schema()").Scan(&schema); e != nil {
					return input, origin, policy, e
				}
			}
			if req.Schema != "" && req.Schema != schema {
				return input, origin, policy, l.ErrInvalid
			}
			id, e := catalogService.Repo.ResolveCoordinate(ctx, p.TenantID, req.DatasourceID, schema, req.Table, "")
			if e != nil && !errors.Is(e, catalogmodel.ErrNotFound) {
				return input, origin, policy, e
			}
			origin.SourceID = req.DatasourceID + "/" + schema + "/" + req.Table
			if req.CatalogAssetID != "" && req.CatalogAssetID != id {
				return input, origin, policy, l.ErrNotFound
			}
			if id != "" {
				asset, e := catalogService.Get(ctx, p.TenantID, id)
				if e != nil {
					return input, origin, policy, e
				}
				if asset.Deprecated {
					return input, origin, policy, l.ErrNotFound
				}
				policy.Sensitivity = asset.Classification
				for _, col := range asset.Columns {
					if !col.Deprecated && (col.Classification == "secret" || (col.Classification == "confidential" && policy.Sensitivity != "secret")) {
						policy.Sensitivity = col.Classification
					}
				}
				origin.SourceID = asset.ID
			}
			if policy.Sensitivity == "secret" || policy.Sensitivity == "confidential" {
				if e := featureAuthorizer(db)(ctx, p, "read_sensitive"); e != nil {
					return input, origin, policy, e
				}
			}
			columns, rows, columnTypes, e := source.SampleRecordsWithTypes(ctx, p.TenantID, req.DatasourceID, req.Table, 1000)
			if e != nil {
				return input, origin, policy, e
			}
			input = l.Input{Kind: "table", Columns: columns, Rows: rows, ColumnTypes: columnTypes}
		case "knowledge":
			document, e := mysql.NewKnowledgeRepository(db).FindByID(ctx, req.KnowledgeID)
			if e != nil || document == nil || document.TenantID != p.TenantID || document.DeletedAt != nil || !document.IsEnabled() {
				return input, origin, policy, l.ErrNotFound
			}
			kb, e := mysql.NewKnowledgeBaseRepository(db, true).FindByIDForTenant(ctx, document.KnowledgeBaseID, p.TenantID)
			if e != nil || kb == nil || kb.DeletedAt != nil || !kb.IsActive() {
				return input, origin, policy, l.ErrNotFound
			}
			chunks, e := mysql.NewChunkRepository(db).FindByKnowledgeID(ctx, document.ID, true)
			if e != nil {
				return input, origin, policy, e
			}
			var text strings.Builder
			for _, chunk := range chunks {
				if chunk == nil || chunk.DeletedAt != nil || chunk.TenantID != p.TenantID || chunk.KnowledgeBaseID != kb.ID {
					continue
				}
				if text.Len()+len(chunk.Content) > 800000 {
					return input, origin, policy, l.ErrInvalid
				}
				text.WriteString(chunk.Content)
				text.WriteString("\n")
			}
			input = l.Input{Kind: "text", Text: text.String()}
			origin.SourceID = document.ID
		case "collection":
			items, e := mysql.NewCollectionRepository(db).ListItems(ctx, p.TenantID, req.RunID)
			if e != nil {
				return input, origin, policy, e
			}
			var chosen *collectionmodel.Item
			for _, item := range items {
				if item.ID == req.ItemID {
					chosen = item
					break
				}
			}
			if chosen == nil || chosen.NormalizedObjectKey == "" {
				return input, origin, policy, l.ErrNotFound
			}
			reader, _, e := store.Get(ctx, chosen.NormalizedObjectKey)
			if e != nil {
				return input, origin, policy, e
			}
			defer reader.Close()
			body, e := io.ReadAll(io.LimitReader(reader, 800001))
			if e != nil {
				return input, origin, policy, e
			}
			if len(body) > 800000 {
				return input, origin, policy, l.ErrInvalid
			}
			input = l.Input{Kind: "text", Text: string(body)}
			origin.SourceID = chosen.ID
			origin.URI = chosen.CanonicalURL
		case "result":
			session, e := mysql.NewSessionRepository(db).FindByID(ctx, req.SessionID)
			if e != nil || session == nil || session.TenantID != p.TenantID || session.UserID != p.UserID {
				return input, origin, policy, l.ErrNotFound
			}
			result, e := exactLifecycleResult(ctx, results, resultstore.OwnerKey(p.TenantID, req.SessionID), req.ResultID)
			if e != nil {
				if errors.Is(e, resultstore.ErrNotFound) || errors.Is(e, resultstore.ErrUnauthorized) {
					return input, origin, policy, l.ErrNotFound
				}
				return input, origin, policy, e
			}
			input = l.Input{Kind: "table", Columns: result.Columns, Rows: [][]any{}, ColumnTypes: result.ColumnTypes}
			for _, row := range result.Rows {
				values := make([]any, len(result.Columns))
				for i, col := range result.Columns {
					values[i] = row[col]
				}
				input.Rows = append(input.Rows, values)
			}
			origin.SourceID = req.ResultID
		case "evaluation":
			task, e := mysql.NewEvaluationTaskRepository(db).FindByID(ctx, req.RunID)
			if e != nil || task == nil || task.TenantID != p.TenantID || task.UserID != p.UserID {
				return input, origin, policy, l.ErrNotFound
			}
			input = l.Input{Kind: "table", Columns: []string{"question", "reference_answer", "generated_answer", "success"}, Rows: [][]any{}}
			cursor := ""
			repo := mysql.NewEvaluationResultRepository(db)
			for {
				rows, next, err := repo.FindByTaskIDByCursor(ctx, task.ID, cursor, 100)
				if err != nil {
					return input, origin, policy, err
				}
				if len(input.Rows)+len(rows) > 10000 {
					return l.Input{}, origin, policy, fmt.Errorf("%w: evaluation source exceeds 10000 rows", l.ErrInvalid)
				}
				for _, row := range rows {
					input.Rows = append(input.Rows, []any{row.Question, row.ReferenceAnswer, row.GeneratedAnswer, row.Success})
				}
				if next == "" {
					break
				}
				cursor = next
			}
			origin.SourceID = task.ID
		default:
			return input, origin, policy, l.ErrInvalid
		}
		return input, origin, policy, input.Validate()
	}
}

func invalidWebSource(err error) bool {
	return errors.Is(err, fetcher.ErrInvalidURL) || errors.Is(err, fetcher.ErrUnsafeDestination) ||
		errors.Is(err, fetcher.ErrUnsupportedType) || errors.Is(err, fetcher.ErrResponseTooLarge)
}

func exactLifecycleResult(ctx context.Context, store resultstore.Store, owner, id string) (*resultstore.Result, error) {
	if store == nil {
		return nil, resultstore.ErrNotFound
	}
	if exact, ok := store.(interface {
		GetExact(context.Context, string, string) (*resultstore.Result, error)
	}); ok {
		return exact.GetExact(ctx, owner, id)
	}
	return store.Get(ctx, owner, id)
}

func featurePublication(processor knowledge.DocumentProcessorService, catalogService *catalog.Service) lifecycle.PublishFunc {
	return func(ctx context.Context, p l.Principal, ref l.DataAssetRef, payload json.RawMessage, options map[string]any) (map[string]any, error) {
		var input l.Input
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, err
		}
		if err := input.Validate(); err != nil {
			return nil, err
		}
		if input.Kind == "text" {
			kb, _ := options["knowledge_base_id"].(string)
			if kb == "" {
				return nil, fmt.Errorf("knowledge_base_id required for text publication")
			}
			title := strings.TrimSpace(ref.Name)
			if title == "" {
				title = "Data asset " + ref.AssetID
			}
			result, err := processor.ProcessDocument(ctx, p.TenantID, p.UserID, &knowledge.ProcessDocumentRequest{KnowledgeBaseID: kb, IdempotencyKey: "lifecycle:" + ref.AssetRefID, Title: title, FileName: ref.AssetID + ".txt", FileType: "txt", Content: []byte(input.Text)})
			if err != nil {
				return nil, err
			}
			if result == nil || result.ParseStatus != "completed" || result.KnowledgeID == "" {
				return nil, fmt.Errorf("knowledge publication incomplete")
			}
			return map[string]any{"publication_status": "published", "knowledge_id": result.KnowledgeID, "knowledge_base_id": kb}, nil
		}
		result := map[string]any{"publication_status": "published", "storage": "versioned_object_store", "source_ref": ref.AssetRefID}
		if id, ok := options["catalog_asset_id"].(string); ok && id != "" {
			asset, err := catalogService.Get(ctx, p.TenantID, id)
			if err != nil {
				return nil, err
			}
			if asset.Deprecated {
				return nil, l.ErrInvalid
			}
			columns := map[string]bool{}
			for _, col := range asset.Columns {
				if !col.Deprecated {
					columns[col.Name] = true
				}
			}
			for _, col := range input.Columns {
				if !columns[col] {
					return nil, fmt.Errorf("published columns disagree with catalog")
				}
			}
			result["catalog_asset_id"] = id
		}
		return result, nil
	}
}
