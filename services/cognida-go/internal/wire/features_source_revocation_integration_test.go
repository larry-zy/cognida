//go:build integration

package wire

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	agent "cognida/internal/model/agent"
	c "cognida/internal/model/catalog"
	e "cognida/internal/model/evaluation"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/repository/mysql"
	"cognida/internal/service/agent/resultstore"
	"cognida/internal/service/lifecycle"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func sourceSnapshotAndReport(t *testing.T, f *sourceHTTPFixture, request lifecycle.SourceRequest) ([]l.DataAssetRef, string) {
	t.Helper()
	key := uuid.NewString()
	w := f.request(t, http.MethodPost, "/lifecycle/assets", &request, key, "")
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var out struct {
		Data l.DataAssetRef `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	report, err := f.life.ImportDerived(context.Background(), f.owner, l.Insight, "source-policy-test:"+uuid.NewString(), []l.RefID{out.Data.RefID}, json.RawMessage(`{"record_count":1}`), out.Data.Policy)
	require.NoError(t, err)
	refs := []l.DataAssetRef{out.Data, report}
	assertSourceReads(t, f, refs, http.StatusOK)
	return refs, key
}

func assertSourceReads(t *testing.T, f *sourceHTTPFixture, refs []l.DataAssetRef, code int) {
	t.Helper()
	for _, ref := range refs {
		w := f.request(t, http.MethodGet, fmt.Sprintf("/lifecycle/assets/%s?version=%d", ref.AssetID, ref.Version), nil, "", "")
		require.Equal(t, code, w.Code, w.Body.String())
	}
}

func TestDatasourceCatalogChangesInvalidateSnapshotAndReport(t *testing.T) {
	for _, scenario := range []string{"table classification", "column classification", "table deprecated", "catalog removed", "new catalog classification"} {
		t.Run(scenario, func(t *testing.T) {
			f := newSourceHTTPFixture(t)
			ctx := context.Background()
			source, err := f.sources.Get(ctx, f.owner.TenantID, f.dsID)
			require.NoError(t, err)
			repo := mysql.NewCatalogRepository(f.db)
			register := func() string {
				_, err = repo.Snapshot(ctx, f.owner.TenantID, f.dsID, []c.AssetTable{{Schema: source.DatabaseName, TableName: f.table, Type: "table", LastSyncedAt: time.Now().UTC(), Governance: c.Governance{Classification: "internal"}, Columns: []c.AssetColumn{{Name: "amount", DataType: "decimal", Governance: c.Governance{Classification: "internal"}}}}})
				require.NoError(t, err)
				id, err := repo.ResolveCoordinate(ctx, f.owner.TenantID, f.dsID, source.DatabaseName, f.table, "")
				require.NoError(t, err)
				return id
			}
			id := ""
			if scenario != "new catalog classification" {
				id = register()
			}
			request := lifecycle.SourceRequest{Kind: "datasource", DatasourceID: f.dsID, Table: f.table}
			refs, key := sourceSnapshotAndReport(t, f, request)
			// Acquired internal bytes remain readable for a normal member until the
			// catalog changes; the immutable old policy cannot grant sensitive access.
			require.NoError(t, f.db.Model(&mysql.TenantUserModel{}).Where("tenant_id=? AND user_id=?", f.owner.TenantID, f.owner.UserID).Update("role", "member").Error)
			assertSourceReads(t, f, refs, http.StatusOK)
			switch scenario {
			case "table classification":
				require.NoError(t, repo.UpdateGovernance(ctx, f.owner.TenantID, id, c.Governance{Classification: "secret"}))
			case "column classification":
				asset, err := repo.Get(ctx, f.owner.TenantID, id)
				require.NoError(t, err)
				require.NoError(t, repo.UpdateGovernance(ctx, f.owner.TenantID, asset.Columns[0].ID, c.Governance{Classification: "confidential"}))
			case "table deprecated":
				require.NoError(t, f.db.Table("asset_table").Where("id=? AND tenant_id=?", id, f.owner.TenantID).Update("deprecated", true).Error)
			case "catalog removed":
				require.NoError(t, f.db.Table("asset_column").Where("table_id=? AND tenant_id=?", id, f.owner.TenantID).Delete(map[string]any{}).Error)
				require.NoError(t, f.db.Table("asset_table").Where("id=? AND tenant_id=?", id, f.owner.TenantID).Delete(map[string]any{}).Error)
			case "new catalog classification":
				id = register()
				require.NoError(t, repo.UpdateGovernance(ctx, f.owner.TenantID, id, c.Governance{Classification: "secret"}))
			}
			assertSourceReads(t, f, refs, http.StatusForbidden)
			// Except physical catalog deletion (which creates an unregistered new
			// source), replay cannot silently return the previously authorized bytes.
			if scenario != "catalog removed" {
				require.Equal(t, http.StatusForbidden, f.request(t, http.MethodPost, "/lifecycle/assets", &request, key, "").Code)
			}
		})
	}
}

func TestResultSourceCurrentGuardProtectsSnapshotAndReport(t *testing.T) {
	for _, scenario := range []string{"inactive", "disabled", "need_credentials", "changed version", "deleted"} {
		t.Run(scenario, func(t *testing.T) {
			f := newSourceHTTPFixture(t)
			ctx := context.Background()
			session := uuid.NewString()
			require.NoError(t, f.db.Create(&mysql.SessionModel{ID: session, TenantID: f.owner.TenantID, UserID: f.owner.UserID, Title: "guard fixture", Status: 1}).Error)
			source, err := f.sources.Get(ctx, f.owner.TenantID, f.dsID)
			require.NoError(t, err)
			owner := resultstore.OwnerKey(f.owner.TenantID, session)
			id, err := f.sources.GuardResults(f.results).Put(ctx, &resultstore.Result{Owner: owner, DatasourceID: f.dsID, SourceVersion: source.UpdatedAt.UTC().Format(time.RFC3339Nano), Columns: []string{"amount"}, ColumnTypes: map[string]string{"amount": "DECIMAL"}, Rows: []map[string]any{{"amount": "9007199254740993.123456789"}}}, time.Minute)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, f.redis.Del(context.Background(), "agent:resultstore:"+id).Err()) })
			request := lifecycle.SourceRequest{Kind: "result", SessionID: session, ResultID: id}
			refs, key := sourceSnapshotAndReport(t, f, request)
			switch scenario {
			case "changed version":
				require.NoError(t, f.db.Model(&mysql.DataSourceModel{}).Where("id=?", f.dsID).Update("updated_at", source.UpdatedAt.Add(time.Second)).Error)
			case "deleted":
				require.NoError(t, f.db.Where("id=?", f.dsID).Delete(&mysql.DataSourceModel{}).Error)
			default:
				require.NoError(t, f.db.Model(&mysql.DataSourceModel{}).Where("id=?", f.dsID).Update("status", scenario).Error)
			}
			assertSourceReads(t, f, refs, http.StatusForbidden)
			require.Equal(t, http.StatusNotFound, f.request(t, http.MethodPost, "/lifecycle/assets", &request, key, "").Code)
		})
	}
}

func TestEvaluationSourceRechecksFixedAndOfflineParents(t *testing.T) {
	for _, kind := range []string{"fixed", "offline"} {
		t.Run(kind, func(t *testing.T) {
			f := newSourceHTTPFixture(t)
			ctx := context.Background()
			_, doc, chunk := f.knowledge(t)
			parents, _ := sourceSnapshotAndReport(t, f, lifecycle.SourceRequest{Kind: "knowledge", KnowledgeID: doc})
			taskID := uuid.NewString()
			task := mysql.EvaluationTaskModel{ID: taskID, TenantID: f.owner.TenantID, UserID: f.owner.UserID, DatasetID: uuid.NewString(), Type: "qa", Status: "completed", TotalCount: 1, SuccessCount: 1}
			if kind == "fixed" {
				version, err := mysql.NewDatasetVersionRepository(f.db).Put(ctx, f.owner, uuid.NewString(), &e.DatasetVersion{DatasetID: task.DatasetID, Profile: "qa", ManifestDigest: "fixture-manifest", SourceRefs: []string{parents[0].AssetRefID}}, nil)
				require.NoError(t, err)
				task.Snapshot, err = json.Marshal(e.EvaluationSnapshot{DatasetVersionID: version.ID, DatasetManifest: version.ManifestDigest, Configuration: json.RawMessage(`{}`)})
				require.NoError(t, err)
			} else {
				task.Type = string(e.OfflineType)
				// Same authorization port used by production OfflineService. The
				// source itself is real MySQL/object lifecycle data; no model executes.
				f.evaluations.WithOfflineReadAuthorizer(func(ctx context.Context, id string) error {
					require.Equal(t, taskID, id)
					require.Equal(t, f.owner.TenantID, agent.MustGetTenantID(ctx))
					require.Equal(t, f.owner.UserID, agent.MustGetUserID(ctx))
					_, _, err := f.life.Resolve(ctx, f.owner.TenantID, f.owner.UserID, parents[0].RefID)
					return err
				})
			}
			require.NoError(t, f.db.Create(&task).Error)
			require.NoError(t, f.db.Create(&mysql.EvaluationResultModel{TaskID: taskID, Question: "q", ReferenceAnswer: "a", GeneratedAnswer: "a", RetrievedPIDs: "[]", RelevantPIDs: "[]", Success: true}).Error)
			request := lifecycle.SourceRequest{Kind: "evaluation", RunID: taskID}
			refs, key := sourceSnapshotAndReport(t, f, request)
			require.NoError(t, f.db.Model(&mysql.ChunkModel{}).Where("id=?", chunk).Update("is_enabled", false).Error)
			assertSourceReads(t, f, refs, http.StatusForbidden)
			require.Equal(t, http.StatusNotFound, f.request(t, http.MethodPost, "/lifecycle/assets", &request, key, "").Code)
		})
	}
}
