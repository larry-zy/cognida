//go:build integration

package handler

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cognida/internal/infrastructure/job"
	store "cognida/internal/infrastructure/objectstore"
	d "cognida/internal/model/lifecycle"
	repo "cognida/internal/repository/mysql"
	svc "cognida/internal/service/lifecycle"
	"github.com/gin-gonic/gin"
	mysqlDSN "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// This opt-in benchmark uses real HTTP, MySQL, Redis, object files and Python.
// Its injected principal authorizer is a test fixture, not a login/RBAC test.
func TestHFIngestionComputeHTTPBenchmark(t *testing.T) {
	dataRoot := os.Getenv("COGNIDA_HF_DATA_DIR")
	if dataRoot == "" {
		t.Skip("set COGNIDA_HF_DATA_DIR for pinned public-data benchmark")
	}
	dsn := os.Getenv("COGNIDA_HF_TEST_DSN")
	dsnConfig, err := mysqlDSN.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(dsnConfig.DBName, "cognida_review_"), "dedicated disposable migrated database required")
	redisAddr := os.Getenv("COGNIDA_HF_REDIS_ADDR")
	computeURL := os.Getenv("COGNIDA_HF_COMPUTE_URL")
	token := os.Getenv("INTERNAL_HTTP_AUTH_TOKEN")
	require.NotEmpty(t, redisAddr)
	require.NotEmpty(t, computeURL)
	require.True(t, token != "", "internal compute token required")
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	tenant := time.Now().UnixNano() & 0x1fffffffffffff
	t.Cleanup(func() {
		for _, table := range []string{"lifecycle_events", "lifecycle_approvals", "lifecycle_stages", "lifecycle_runs", "lifecycle_assets"} {
			require.NoError(t, db.Table(table).Where("tenant_id = ?", tenant).Delete(map[string]any{}).Error)
		}
		require.NoError(t, sqlDB.Close())
	})
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: redisAddr})
	require.NoError(t, client.Ping(ctx).Err())
	prefix := "cognida:hf-ingestion:" + uuid.NewString()
	t.Cleanup(func() {
		// Scan only this test's random namespace; never flush a shared Redis DB.
		iter := client.Scan(ctx, 0, prefix+":*", 100).Iterator()
		var keys []string
		for iter.Next(ctx) {
			keys = append(keys, iter.Val())
		}
		require.NoError(t, iter.Err())
		if len(keys) > 0 {
			require.NoError(t, client.Del(ctx, keys...).Err())
		}
		require.NoError(t, client.Close())
	})
	queue := job.NewRedisQueueWithPrefix(client, prefix)
	objects, err := store.NewLocal(t.TempDir())
	require.NoError(t, err)
	service, err := svc.NewService(repo.NewLifecycleRepository(db), objects, queue, svc.Config{
		Compute: svc.NewHTTPCompute(computeURL, token),
		Authorize: func(_ context.Context, p d.Principal, _ string) error {
			if (p.TenantID != tenant && p.TenantID != tenant+1) || p.UserID != 1 {
				return d.ErrForbidden
			}
			return nil
		},
	})
	require.NoError(t, err)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		id := tenant
		if c.GetHeader("X-Test-Other-Tenant") == "true" {
			id++
		}
		c.Set("tenant_id", id)
		c.Set("user_id", int64(1))
		c.Next()
	})
	NewLifecycleHandler(service).Register(router.Group("/lifecycle"))
	server := httptest.NewServer(router)
	defer server.Close()
	apiClient := &http.Client{Timeout: 30 * time.Second}
	request := func(method, path string, body any, key string, other bool, status int, target any) {
		raw, e := json.Marshal(body)
		require.NoError(t, e)
		req, e := http.NewRequestWithContext(ctx, method, server.URL+path, bytes.NewReader(raw))
		require.NoError(t, e)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		req.Header.Set("X-Request-ID", "hf-ingestion-"+key)
		if other {
			req.Header.Set("X-Test-Other-Tenant", "true")
		}
		resp, e := apiClient.Do(req)
		require.NoError(t, e)
		defer resp.Body.Close()
		payload, e := io.ReadAll(io.LimitReader(resp.Body, 9<<20))
		require.NoError(t, e)
		require.Equal(t, status, resp.StatusCode, "unexpected HTTP status: %s", string(payload))
		if target != nil {
			var envelope struct {
				Data json.RawMessage `json:"data"`
			}
			require.NoError(t, json.Unmarshal(payload, &envelope))
			require.NoError(t, json.Unmarshal(envelope.Data, target))
		}
	}
	invoke := func(agent string, refs []d.RefID, options map[string]any, key string) d.StageRun {
		body := map[string]any{"agent_id": agent, "input_refs": refs, "options": options}
		var first, retry d.StageRun
		request("POST", "/lifecycle/invocations", body, key, false, 201, &first)
		request("POST", "/lifecycle/invocations", body, key, false, 201, &retry)
		require.Equal(t, first.ID, retry.ID, "idempotent invocation must reuse the durable stage")
		delivery, e := queue.TryClaim(ctx)
		require.NoError(t, e)
		require.NotNil(t, delivery)
		require.NoError(t, service.Handle(ctx, *delivery))
		require.NoError(t, queue.Ack(ctx, *delivery))
		var resolved struct {
			Invocation d.StageRun `json:"invocation"`
		}
		request("GET", "/lifecycle/invocations/"+first.ID, nil, "", false, 200, &resolved)
		require.Equal(t, d.StatusSucceeded, resolved.Invocation.Status, resolved.Invocation.Error)
		require.NotNil(t, resolved.Invocation.Output)
		require.Equal(t, "hf-ingestion-"+key, resolved.Invocation.RequestID)
		return resolved.Invocation
	}
	readAsset := func(ref d.RefID) d.Input {
		var resolved struct {
			Content d.Input `json:"content"`
		}
		request("GET", fmt.Sprintf("/lifecycle/assets/%s?version=%d", ref.AssetID, ref.Version), nil, "", false, 200, &resolved)
		return resolved.Content
	}
	var results []map[string]any
	for _, dataset := range []string{"banking77", "massive_zh"} {
		manifestRaw, e := os.ReadFile(filepath.Join(dataRoot, dataset, "manifest.json"))
		require.NoError(t, e)
		var manifest struct {
			DatasetID string                                 `json:"dataset_id"`
			Revision  string                                 `json:"revision"`
			Files     []struct{ Split, Path, SHA256 string } `json:"files"`
		}
		require.NoError(t, json.Unmarshal(manifestRaw, &manifest))
		expectedDataset := map[string][2]string{
			"banking77":  {"mteb/banking77", "18072d2685ea682290f7b8924d94c62acc19c0b2"},
			"massive_zh": {"mteb/amazon_massive_intent", "940fd47a81eaa7f2cc7b129674d945d618ac38c2"},
		}[dataset]
		require.Equal(t, expectedDataset[0], manifest.DatasetID)
		require.Equal(t, expectedDataset[1], manifest.Revision)
		var testPath, expectedHash string
		for _, file := range manifest.Files {
			if file.Split == "test" {
				testPath, expectedHash = file.Path, file.SHA256
			}
		}
		require.NotEmpty(t, testPath)
		raw, e := os.ReadFile(testPath)
		require.NoError(t, e)
		checksum := sha256.Sum256(raw)
		require.Equal(t, expectedHash, hex.EncodeToString(checksum[:]))
		scanner := bufio.NewScanner(bytes.NewReader(raw))
		input := d.Input{Kind: "table", Columns: []string{"source_id", "text", "gold_label"}}
		// Fixed first 256 official test rows: contract sample, not a quality estimate.
		for scanner.Scan() && len(input.Rows) < 256 {
			var row struct{ ID, Text, Label string }
			require.NoError(t, json.Unmarshal(scanner.Bytes(), &row))
			require.Equal(t, fmt.Sprintf("test:%d", len(input.Rows)), row.ID)
			require.NotEmpty(t, row.Text)
			require.NotEmpty(t, row.Label)
			input.Rows = append(input.Rows, []any{row.ID, row.Text, row.Label})
		}
		require.NoError(t, scanner.Err())
		require.Len(t, input.Rows, 256)
		started := time.Now()
		var ref d.DataAssetRef
		request("POST", "/lifecycle/assets", map[string]any{"input": input, "name": "HF " + dataset}, dataset+"-import", false, 201, &ref)
		var duplicate d.DataAssetRef
		request("POST", "/lifecycle/assets", map[string]any{"input": input, "name": "HF " + dataset}, dataset+"-import", false, 201, &duplicate)
		require.Equal(t, ref.RefID, duplicate.RefID)
		require.Equal(t, input, readAsset(ref.RefID))
		request("GET", "/lifecycle/assets/"+ref.AssetID, nil, "", true, 404, nil)
		quality := invoke("data-quality", []d.RefID{ref.RefID}, nil, dataset+"-quality")
		clean := invoke("data-cleaning", []d.RefID{ref.RefID}, map[string]any{"operations": []string{"dedup"}}, dataset+"-clean")
		require.Equal(t, d.GatePassed, clean.Output.Gate.Status)
		var cleanedRef d.DataAssetRef
		for _, out := range clean.Output.Refs {
			if out.Kind == d.CleanedAsset {
				cleanedRef = out
			}
		}
		require.NotEmpty(t, cleanedRef.AssetID)
		require.Equal(t, []d.RefID{ref.RefID}, cleanedRef.Parents)
		require.Equal(t, input, readAsset(cleanedRef.RefID), "source IDs distinguish genuine repeated utterances")
		require.Equal(t, input, readAsset(ref.RefID), "source snapshot must remain immutable")
		results = append(results, map[string]any{"dataset": dataset, "revision": manifest.Revision, "source_sha256": expectedHash, "sampling": "first_256_official_test_rows", "input_rows": 256, "output_rows": 256, "quality_gate": quality.Output.Gate, "clean_gate": clean.Output.Gate, "duration_ms": time.Since(started).Milliseconds(), "source_immutable": true, "idempotency": true, "cross_tenant_hidden": true})
	}
	// Scalar control exercises the complete transport path and real dedup engine.
	// DECIMAL is preserved as a typed string; JSON float decimal precision is not promised.
	exact := d.Input{Kind: "table", Columns: []string{"value", "decimal"}, ColumnTypes: map[string]string{"decimal": "decimal"}, Rows: [][]any{
		{true, "1.2300000000000000001"}, {json.Number("1"), "1.2300000000000000001"}, {json.Number("1.0"), "1.2300000000000000001"}, {"1", "1.2300000000000000001"},
		{false, "1.2300000000000000001"}, {json.Number("0"), "1.2300000000000000001"}, {json.Number("0.0"), "1.2300000000000000001"}, {json.Number("9007199254740993"), "1.2300000000000000001"},
	}}
	var exactRef d.DataAssetRef
	request("POST", "/lifecycle/assets", map[string]any{"input": exact}, "types-import", false, 201, &exactRef)
	clean := invoke("data-cleaning", []d.RefID{exactRef.RefID}, map[string]any{"operations": []string{"dedup"}}, "types-clean")
	var typed d.Input
	for _, out := range clean.Output.Refs {
		if out.Kind == d.CleanedAsset {
			typed = readAsset(out.RefID)
		}
	}
	require.Equal(t, exact, typed)
	evidence := map[string]any{"schema_version": 1, "transport": "Go TCP HTTP -> MySQL + namespaced Redis jobs -> authenticated Python HTTP -> immutable objects", "authorization": "injected test principal; production login/RBAC not evaluated", "public_data": results, "scalar_control": map[string]any{"rows": len(exact.Rows), "output_rows": len(typed.Rows), "type_fidelity": true, "decimal_contract": "string plus column_types"}, "scope": "ingestion, quality, cleaning; no publication/search claims"}
	output := os.Getenv("COGNIDA_HF_INGESTION_OUTPUT")
	require.NotEmpty(t, output)
	require.NoError(t, os.MkdirAll(filepath.Dir(output), 0700))
	encoded, e := json.MarshalIndent(evidence, "", "  ")
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(output, append(encoded, '\n'), 0600))
	t.Log("verified 512 real HF rows and 8 exact-type controls over Go/Python HTTP with MySQL and Redis")
}
