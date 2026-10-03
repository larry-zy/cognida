//go:build integration

package handler

import (
	"bytes"
	"cognida/internal/infrastructure/job"
	store "cognida/internal/infrastructure/objectstore"
	d "cognida/internal/model/lifecycle"
	repo "cognida/internal/repository/mysql"
	svc "cognida/internal/service/lifecycle"
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestLifecycleHTTPIntegrationTenantIdempotencyAndSSE(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	require.NotEmpty(t, dsn)
	db, e := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	sql, e := db.DB()
	require.NoError(t, e)
	tenant := time.Now().UnixNano() & 0x1fffffffffffff
	t.Cleanup(func() {
		for _, table := range []string{"lifecycle_events", "lifecycle_approvals", "lifecycle_stages", "lifecycle_runs", "lifecycle_assets"} {
			require.NoError(t, db.Table(table).Where("tenant_id=?", tenant).Delete(map[string]any{}).Error)
		}
		require.NoError(t, sql.Close())
	})
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", Password: os.Getenv("REDIS_PASSWORD")})
	require.NoError(t, client.Ping(context.Background()).Err())
	prefix := "cognida:lifecycle-api:" + uuid.NewString()
	t.Cleanup(func() {
		keys, e := client.Keys(context.Background(), prefix+":*").Result()
		require.NoError(t, e)
		if len(keys) > 0 {
			require.NoError(t, client.Del(context.Background(), keys...).Err())
		}
		require.NoError(t, client.Close())
	})
	q := job.NewRedisQueueWithPrefix(client, prefix)
	objects, e := store.NewLocal(t.TempDir())
	require.NoError(t, e)
	s, e := svc.NewService(repo.NewLifecycleRepository(db), objects, q, svc.Config{})
	require.NoError(t, e)
	g := gin.New()
	g.Use(func(c *gin.Context) {
		id := tenant
		if c.GetHeader("X-Test-Other-Tenant") == "true" {
			id++
		}
		c.Set("tenant_id", id)
		c.Set("user_id", int64(1))
		c.Next()
	})
	NewLifecycleHandler(s).Register(g.Group("/lifecycle"))
	request := func(method, path string, body any, key, other string) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", key)
		r.Header.Set("X-Test-Other-Tenant", other)
		w := httptest.NewRecorder()
		g.ServeHTTP(w, r)
		return w
	}
	decode := func(w *httptest.ResponseRecorder) map[string]any {
		var v struct {
			Data map[string]any `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &v))
		return v.Data
	}
	w := request("POST", "/lifecycle/assets", map[string]any{"input": map[string]any{"kind": "text", "text": "snapshot"}}, "asset", "")
	require.Equal(t, 201, w.Code, w.Body.String())
	asset := decode(w)
	refs := []map[string]any{{"asset_id": asset["asset_id"], "version": asset["version"]}}
	path := "/lifecycle/assets/" + asset["asset_id"].(string)
	require.Equal(t, 404, request("GET", path, nil, "", "true").Code)
	w = request("GET", path, nil, "", "")
	require.Equal(t, 200, w.Code)
	require.Equal(t, "snapshot", decode(w)["content"].(map[string]any)["text"])
	// Exercise the real JSON binding boundary with a number beyond float64's
	// exact integer range, then read the stored snapshot through HTTP.
	w = request("POST", "/lifecycle/assets", map[string]any{"input": d.Input{Kind: "table", Columns: []string{"id"}, Rows: [][]any{{json.Number("9007199254740993")}}}}, "exact-asset", "")
	require.Equal(t, 201, w.Code, w.Body.String())
	exactAsset := decode(w)
	w = request("GET", "/lifecycle/assets/"+exactAsset["asset_id"].(string), nil, "", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "9007199254740993")
	require.NotContains(t, w.Body.String(), "9007199254740992")

	payload := map[string]any{"agent_id": "data-access", "input_refs": refs}
	require.Equal(t, 400, request("POST", "/lifecycle/invocations", payload, "", "").Code)
	w = request("POST", "/lifecycle/invocations", payload, "invoke", "")
	require.Equal(t, 201, w.Code, w.Body.String())
	inv := decode(w)
	again := request("POST", "/lifecycle/invocations", payload, "invoke", "")
	require.Equal(t, 201, again.Code)
	require.Equal(t, inv["id"], decode(again)["id"])
	j, e := q.TryClaim(context.Background())
	require.NoError(t, e)
	require.NotNil(t, j)
	require.NoError(t, s.Handle(context.Background(), *j))
	require.NoError(t, q.Ack(context.Background(), *j))
	w = request("GET", "/lifecycle/invocations/"+inv["id"].(string), nil, "", "")
	require.Equal(t, 200, w.Code)
	require.Equal(t, d.StatusSucceeded, decode(w)["invocation"].(map[string]any)["status"])
	w = request("POST", "/lifecycle/runs", map[string]any{"template_id": "web-knowledge-to-insight", "input_refs": refs}, "run", "")
	require.Equal(t, 201, w.Code)
	run := decode(w)
	id := run["id"].(string)
	require.Equal(t, 200, request("POST", "/lifecycle/runs/"+id+"/cancel", nil, "", "").Code)
	w = request("GET", "/lifecycle/runs/"+id+"/events", nil, "", "")
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "text/event-stream")
	require.Contains(t, w.Body.String(), "stage.cancelled")
	w = request("GET", "/lifecycle/assets?page_size=1", nil, "", "")
	require.Equal(t, 200, w.Code)
	require.True(t, decode(w)["has_more"].(bool))
	cursor := decode(w)["next_cursor"].(string)
	w = request("GET", "/lifecycle/assets?page_size=1&cursor="+cursor, nil, "", "")
	require.Equal(t, http.StatusOK, w.Code)
	require.Len(t, decode(w)["items"].([]any), 1)
}
