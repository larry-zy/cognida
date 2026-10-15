//go:build integration

package governance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cognida/internal/handler"
	"cognida/internal/infrastructure/job"
	store "cognida/internal/infrastructure/objectstore"
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	repo "cognida/internal/repository/mysql"
	svc "cognida/internal/service/governance"
	"cognida/internal/service/lifecycle"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// These tests use a real migrated MySQL database and actual local object bytes.
// Feature execution is synchronous; the queue is an unused explicit test double.
type featureQueue struct{ job.Queue }

func (featureQueue) Enqueue(context.Context, job.Job) error {
	return errors.New("unexpected queue operation in synchronous feature materialization")
}

type featureDBFixture struct {
	s       *svc.Service
	assets  *lifecycle.Service
	db      *gorm.DB
	repo    d.Repository
	p       l.Principal
	revoked atomic.Bool
}

func featureDB(t *testing.T) *featureDBFixture {
	t.Helper()
	dsn := os.Getenv("COGNIDA_DOMAIN_TEST_DSN")
	if dsn == "" {
		t.Skip("COGNIDA_DOMAIN_TEST_DSN required")
	}
	cfg, err := driver.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(cfg.DBName, "cognida_review_"), "isolated review database required")
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sql, err := db.DB()
	require.NoError(t, err)
	f := &featureDBFixture{db: db, repo: repo.NewGovernanceRepository(db), p: l.Principal{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 9123}}
	t.Cleanup(func() {
		for _, table := range []string{"feature_materializations", "feature_values", "governance_records", "lifecycle_assets"} {
			require.NoError(t, db.Table(table).Where("tenant_id=?", f.p.TenantID).Delete(map[string]any{}).Error)
		}
		require.NoError(t, sql.Close())
	})
	objects, err := store.NewLocal(t.TempDir())
	require.NoError(t, err)
	auth := func(_ context.Context, p l.Principal, _ string) error {
		if p != f.p || f.revoked.Load() {
			return l.ErrForbidden
		}
		return nil
	}
	f.assets, err = lifecycle.NewService(repo.NewLifecycleRepository(db), objects, featureQueue{}, lifecycle.Config{Authorize: auth, ValidateGovernedOrigin: func(ctx context.Context, p l.Principal, ref l.DataAssetRef) error {
		return f.s.ValidateFeatureMaterializationOrigin(ctx, p, ref)
	}})
	require.NoError(t, err)
	f.s = svc.NewService(f.repo, f.assets, nil, featureQueue{}, auth)
	return f
}
func (f *featureDBFixture) feature(t *testing.T, key string) (*d.Record, l.DataAssetRef) {
	t.Helper()
	ref, err := f.assets.ImportWithKey(context.Background(), f.p, l.Input{Kind: "table", Columns: []string{"entity", "amount", "event"}, Rows: [][]any{{"00123", "9007199254740993.123456789012345678", "2026-01-01T01:00:00Z"}, {"b", "0.2", "2026-01-01T02:00:00Z"}}}, "feature fixture", uuid.NewString())
	require.NoError(t, err)
	input := d.Feature{Name: "exact add", Type: "numerical", Definition: []byte(`{"kind":"expression","entity_field":"entity","event_time_field":"event","expression":{"op":"add","args":[{"op":"column","field":"amount"},{"op":"literal","value":0.1}]}}`), SourceRefID: ref.RefID.String()}
	head, err := f.s.CreateFeature(context.Background(), f.p, input, key)
	require.NoError(t, err)
	return head, ref
}
func (f *featureDBFixture) api(t *testing.T, p l.Principal, method, path, key string, body any) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Set("tenant_id", p.TenantID); c.Set("user_id", p.UserID); c.Next() })
	handler.NewGovernanceHandler(f.s).Register(engine.Group("/api"))
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(method, "/api"+path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	var out map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return w.Code, out
}
func TestFeatureEngineeringRealMySQLHTTPAndAvailability(t *testing.T) {
	f := featureDB(t)
	ctx := context.Background()
	head, _ := f.feature(t, "feature")
	definition, err := f.s.GetFeatureVersion(ctx, f.p, head.ID, 1)
	require.NoError(t, err)
	code, out := f.api(t, f.p, "POST", "/features/preview", "", map[string]any{"feature": definition, "limit": 1})
	require.Equal(t, 200, code, out)
	require.EqualValues(t, 2, out["data"].(map[string]any)["total_count"])
	var beforeMicros int64
	require.NoError(t, f.db.Raw("SELECT CAST(UNIX_TIMESTAMP(NOW(6))*1000000 AS SIGNED)").Scan(&beforeMicros).Error)
	before := time.UnixMicro(beforeMicros).UTC()
	code, out = f.api(t, f.p, "POST", "/features/"+head.ID+"/materialize", "mat", map[string]any{"version": 1})
	require.Equal(t, 200, code, out)
	receipt, err := f.s.LookupFeatureMaterialization(ctx, f.p, "mat")
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.Equal(t, 2, receipt.Count)
	require.True(t, receipt.AvailableAt.After(before))
	filter := []d.FeatureValue{{FeatureID: head.ID, Version: 1}}
	values, err := f.s.QueryFeatures(ctx, f.p, []string{"00123", "b"}, filter, before)
	require.NoError(t, err)
	require.Empty(t, values, "later materialization must not appear in an earlier as_of")
	values, err = f.s.QueryFeatures(ctx, f.p, []string{"00123", "b"}, filter, receipt.AvailableAt.Add(time.Second))
	require.NoError(t, err)
	require.Len(t, values, 2)
	require.Equal(t, "9007199254740993.223456789012345678", string(values[0].Value))
	require.Equal(t, receipt.AvailableAt, values[0].AvailableAt.UTC())
	outputID, err := l.ParseRefID(receipt.OutputRefID)
	require.NoError(t, err)
	_, body, err := f.assets.Resolve(ctx, f.p.TenantID, f.p.UserID, outputID)
	require.NoError(t, err)
	require.Contains(t, string(body), "9007199254740993.223456789012345678")
	replay, err := f.s.MaterializeFeature(ctx, f.p, head.ID, 1, "mat")
	require.NoError(t, err)
	require.Equal(t, *receipt, *replay)
	// Callers cannot forge availability or overwrite a historic entity/event cell.
	late := d.FeatureValue{FeatureID: head.ID, Version: 1, EntityID: "late", Value: []byte(`12.3`), Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), AvailableAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	require.NoError(t, f.s.WriteFeatures(ctx, f.p, []d.FeatureValue{late}))
	values, err = f.s.QueryFeatures(ctx, f.p, []string{"late"}, filter, before)
	require.NoError(t, err)
	require.Empty(t, values)
	late.Value = []byte(`99`)
	require.ErrorIs(t, f.s.WriteFeatures(ctx, f.p, []d.FeatureValue{late}), d.ErrConflict)
	code, out = f.api(t, f.p, "POST", "/feature-values", "manual", map[string]any{"values": []any{map[string]any{"feature_id": head.ID, "version": 1, "entity_id": "manual", "timestamp": "2026-01-01T00:00:00Z", "value": nil, "value_exact": "9007199254740993.123456789012345678", "available_at": "2025-01-01T00:00:00Z"}}})
	require.Equal(t, 200, code, out)
	manual, err := f.s.QueryFeatures(ctx, f.p, []string{"manual"}, filter, time.Now().Add(time.Minute))
	require.NoError(t, err)
	require.Len(t, manual, 1)
	require.Equal(t, "9007199254740993.123456789012345678", manual[0].ValueExact)
	require.True(t, manual[0].AvailableAt.After(before))
	code, _ = f.api(t, f.p, "POST", "/feature-values", "bad", map[string]any{"values": []any{map[string]any{"feature_id": head.ID, "version": 1, "entity_id": "manual-conflict", "timestamp": "2026-01-01T00:00:00Z", "value": 1, "value_exact": "2"}}})
	require.Equal(t, 400, code)
	code, _ = f.api(t, l.Principal{TenantID: f.p.TenantID, UserID: f.p.UserID + 1}, "GET", "/features/"+head.ID+"/versions/1", "", nil)
	require.Equal(t, 403, code)
	f.revoked.Store(true)
	_, err = f.s.MaterializeFeature(ctx, f.p, head.ID, 1, "mat")
	require.Error(t, err)
	_, _, err = f.assets.Resolve(ctx, f.p.TenantID, f.p.UserID, outputID)
	require.Error(t, err)
	receipt, err = f.s.LookupFeatureMaterialization(ctx, f.p, "mat")
	require.NoError(t, err)
	require.NotNil(t, receipt, "revocation does not erase committed facts")
}
func TestFeatureEngineeringConcurrentReplayAndAtomicRollback(t *testing.T) {
	f := featureDB(t)
	ctx := context.Background()
	head, _ := f.feature(t, "one")
	var wg sync.WaitGroup
	results := make(chan *d.FeatureMaterialization, 6)
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := f.s.MaterializeFeature(ctx, f.p, head.ID, 1, "same")
			results <- r
			errs <- e
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	id := ""
	for r := range results {
		if id == "" {
			id = r.ReceiptID
		}
		require.Equal(t, id, r.ReceiptID)
	}
	var count int64
	require.NoError(t, f.db.Table("feature_materializations").Where("tenant_id=?", f.p.TenantID).Count(&count).Error)
	require.EqualValues(t, 1, count)
	head2, _ := f.feature(t, "two")
	_, err := f.s.MaterializeFeature(ctx, f.p, head2.ID, 1, "same")
	require.ErrorIs(t, err, d.ErrConflict)
	at := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	err = f.s.WriteFeatures(ctx, f.p, []d.FeatureValue{{FeatureID: head.ID, Version: 1, EntityID: "new", Timestamp: at, Value: []byte(`1`)}, {FeatureID: head.ID, Version: 1, EntityID: "00123", Timestamp: at, Value: []byte(`0`)}})
	require.ErrorIs(t, err, d.ErrConflict)
	require.NoError(t, f.db.Table("feature_values").Where("tenant_id=? AND entity_id='new'", f.p.TenantID).Count(&count).Error)
	require.Zero(t, count, "a conflicting batch must roll back all prior inserts")
}
func TestFeatureEngineeringGovernedCancellationCommitFence(t *testing.T) {
	f := featureDB(t)
	ctx := context.Background()
	g := f.repo.(d.GovernedRepository)
	for n := 0; n < 6; n++ {
		head, source := f.feature(t, fmt.Sprintf("case-%d", n))
		in := d.GovernedInvocation{OperationKey: strings.Repeat(fmt.Sprintf("%x", n+1), 64), PayloadHash: strings.Repeat("f", 64), Profile: d.FeatureProfile, FeatureID: head.ID, FeatureVersion: 1, InputRef: source.RefID, Deadline: time.Now().UTC().Add(time.Minute), TimeoutSeconds: 60, CallsReserved: 3}
		op, err := g.SubmitGoverned(ctx, f.p, in, false)
		require.NoError(t, err)
		op, err = g.UpdateGoverned(ctx, f.p, op.ID, func(record *d.Record, _ *d.GovernedExecution, _ d.Repository) error {
			record.Status = "running"
			return nil
		})
		require.NoError(t, err)
		var wg sync.WaitGroup
		var materializeErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, materializeErr = f.s.MaterializeGovernedFeature(ctx, f.p, head.ID, 1, op.ID)
		}()
		go func() { defer wg.Done(); _, e := g.SubmitGoverned(ctx, f.p, in, true); require.NoError(t, e) }()
		wg.Wait()
		receipt, e := f.s.LookupFeatureMaterialization(ctx, f.p, "governed:"+op.ID)
		require.NoError(t, e)
		var count int64
		require.NoError(t, f.db.Table("feature_values").Where("tenant_id=? AND feature_id=?", f.p.TenantID, head.ID).Count(&count).Error)
		if receipt == nil {
			require.Error(t, materializeErr)
			require.Zero(t, count)
			_, e = f.s.MaterializeGovernedFeature(ctx, f.p, head.ID, 1, op.ID)
			require.Error(t, e)
			require.NoError(t, f.db.Table("feature_values").Where("tenant_id=? AND feature_id=?", f.p.TenantID, head.ID).Count(&count).Error)
			require.Zero(t, count)
		} else {
			require.NoError(t, materializeErr)
			require.EqualValues(t, 2, count)
			replay, e := f.s.MaterializeGovernedFeature(ctx, f.p, head.ID, 1, op.ID)
			require.NoError(t, e)
			require.Equal(t, receipt.ReceiptID, replay.ReceiptID)
		}
		_, e = f.s.MaterializeFeature(ctx, f.p, head.ID, 1, "governed:"+op.ID)
		require.ErrorIs(t, e, d.ErrInvalid)
	}
}

func TestFeatureEngineeringDefinitionNumericLiteralPreserved(t *testing.T) {
	f := featureDB(t)
	ctx := context.Background()
	_, source := f.feature(t, "source")
	input := d.Feature{Name: "literal", Type: "numerical", SourceRefID: source.RefID.String(), Definition: []byte(`{"kind":"expression","entity_field":"entity","expression":{"op":"literal","value":9007199254740993.123456789012345678}}`), DefinitionExact: `{"kind":"embedding"}`}
	head, err := f.s.CreateFeature(ctx, f.p, input, "literal")
	require.NoError(t, err)
	stored, err := f.s.GetFeatureVersion(ctx, f.p, head.ID, 1)
	require.NoError(t, err)
	require.JSONEq(t, string(input.Definition), string(stored.Definition))
	require.Contains(t, string(stored.Definition), "9007199254740993.123456789012345678")
	preview, err := f.s.PreviewFeature(ctx, f.p, stored, 20)
	require.NoError(t, err)
	require.Equal(t, "9007199254740993.123456789012345678", string(preview.Values[0].Value))
	_, err = f.s.MaterializeFeature(ctx, f.p, head.ID, 1, "literal-materialize")
	require.NoError(t, err)
	got, err := f.s.QueryFeatures(ctx, f.p, []string{"00123"}, []d.FeatureValue{{FeatureID: head.ID, Version: 1}}, time.Now().Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, "9007199254740993.123456789012345678", string(got[0].Value))
}
