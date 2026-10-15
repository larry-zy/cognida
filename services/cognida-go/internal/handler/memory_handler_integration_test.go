//go:build integration

package handler

import (
	"bytes"
	"cognida/internal/handler/middleware"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	agent "cognida/internal/model/agent"
	d "cognida/internal/model/memory"
	repository "cognida/internal/repository/mysql"
	svc "cognida/internal/service/memory"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type integrationMemoryEmbedder struct{}

func (integrationMemoryEmbedder) EmbedStrings(context.Context, []string, ...embedding.Option) ([][]float64, error) {
	return [][]float64{{0.75, 0.25}}, nil
}

func TestMemoryHTTPIntegrationPersistenceIsolationAndForgetting(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	cfg, err := driver.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(cfg.DBName, "cognida_review_"), "isolated review database required")
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	tenant := time.Now().UnixNano() & 0x1fffffffffffff
	t.Cleanup(func() {
		require.NoError(t, db.Table("user_memories").Where("tenant_id IN ?", []int64{tenant, tenant + 1}).Delete(map[string]any{}).Error)
		require.NoError(t, sqlDB.Close())
	})
	repo := repository.NewLongTermMemoryRepository(db)
	service := svc.NewService(repo, integrationMemoryEmbedder{}, "test-provider/model-v1")
	r := gin.New()
	r.Use(func(c *gin.Context) {
		tenantID, userID := tenant, int64(41)
		if c.GetHeader("X-Other-Tenant") == "true" {
			tenantID++
		}
		if c.GetHeader("X-Other-User") == "true" {
			userID++
		}
		c.Set("tenant_id", tenantID)
		c.Set("user_id", userID)
	})
	r.Use(middleware.NewIdempotency(nil).Apply())
	NewMemoryHandler(service).Register(r.Group("/api/v1"))
	request := func(method, path string, body any, key, other string) *httptest.ResponseRecorder {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		if other != "" {
			req.Header.Set(other, "true")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	readMemory := func(w *httptest.ResponseRecorder) d.LongTermMemory {
		var result struct {
			Data d.LongTermMemory `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		return result.Data
	}
	input := d.WriteInput{Content: "我喜欢中文回答", Category: "preference", Importance: .8}
	w := request("POST", "/api/v1/memories", input, "", "")
	require.Equal(t, 400, w.Code)
	key := uuid.NewString()
	w = request("POST", "/api/v1/memories", input, key, "")
	require.Equal(t, 201, w.Code, w.Body.String())
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	m := readMemory(w)
	require.NotEmpty(t, m.ID)
	require.EqualValues(t, 1, m.Version)
	persisted, err := repository.NewLongTermMemoryRepository(db).Get(context.Background(), d.Scope{TenantID: tenant, UserID: 41}, m.ID)
	require.NoError(t, err)
	require.Equal(t, []float32{0.75, 0.25}, persisted.Embedding)
	require.Equal(t, "test-provider/model-v1", persisted.EmbeddingModel)
	vectorService := svc.NewService(repository.NewLongTermMemoryRepository(db), integrationMemoryEmbedder{}, "test-provider/model-v1")
	recalled, err := vectorService.Recall(context.Background(), d.Scope{TenantID: tenant, UserID: 41}, d.RecallInput{Query: "language preference"})
	require.NoError(t, err)
	require.Len(t, recalled.Items, 1)
	require.Equal(t, "cosine", recalled.Items[0].Method)
	again := request("POST", "/api/v1/memories", input, key, "")
	require.Equal(t, 201, again.Code)
	require.Equal(t, m.ID, readMemory(again).ID)
	require.Empty(t, again.Header().Get("Idempotency-Replayed"))
	require.Equal(t, "no-store", again.Header().Get("Cache-Control"))
	changed := input
	changed.Content = "changed"
	require.Equal(t, 409, request("POST", "/api/v1/memories", changed, key, "").Code)
	path := "/api/v1/memories/" + m.ID
	for _, other := range []string{"X-Other-User", "X-Other-Tenant"} {
		require.Equal(t, 404, request("GET", path, nil, "", other).Code)
		require.Equal(t, 404, request("PUT", path, d.UpdateInput{WriteInput: input, ExpectedVersion: 1}, "", other).Code)
		require.Equal(t, 404, request("DELETE", path+"?expected_version=1", nil, "", other).Code)
		w = request("GET", "/api/v1/memories", nil, "", other)
		require.Equal(t, 200, w.Code)
		require.Contains(t, w.Body.String(), `"items":[]`)
		w = request("POST", "/api/v1/memories/recall", d.RecallInput{Query: "中文"}, "", other)
		require.Equal(t, 200, w.Code)
		require.Contains(t, w.Body.String(), `"items":[]`)
	}
	// A fresh service instance simulates process restart; persisted preferences reach a new session.
	fresh := svc.NewService(repository.NewLongTermMemoryRepository(db), nil, "")
	scope := d.Scope{TenantID: tenant, UserID: 41}
	ctx := agent.WithSessionID(agent.WithUserID(agent.WithTenantID(context.Background(), tenant), 41), uuid.NewString())
	_, message, err := svc.NewRecallHook(fresh)(ctx, "介绍这个项目")
	require.NoError(t, err)
	require.Contains(t, message, input.Content)
	updatedInput := d.UpdateInput{WriteInput: d.WriteInput{Content: "我喜欢英语回答", Category: "preference", Importance: .9}, ExpectedVersion: 1}
	w = request("PUT", path, updatedInput, "", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	updated := readMemory(w)
	require.EqualValues(t, 2, updated.Version)
	require.Equal(t, 409, request("PUT", path, updatedInput, "", "").Code)
	_, message, err = svc.NewRecallHook(fresh)(ctx, "介绍这个项目")
	require.NoError(t, err)
	require.Contains(t, message, updatedInput.Content)
	require.NotContains(t, message, input.Content)
	require.Equal(t, 409, request("DELETE", path+"?expected_version=1", nil, "", "").Code)
	require.Equal(t, 200, request("DELETE", path+"?expected_version=2", nil, "", "").Code)
	require.Equal(t, 200, request("DELETE", path+"?expected_version=2", nil, "", "").Code)
	require.Equal(t, 404, request("GET", path, nil, "", "").Code)
	require.Equal(t, 409, request("POST", "/api/v1/memories", input, key, "").Code)
	_, message, err = svc.NewRecallHook(fresh)(ctx, "介绍这个项目")
	require.NoError(t, err)
	require.Equal(t, "介绍这个项目", message)
	var tombstone struct {
		Content   string
		Embedding []byte
		DeletedAt *time.Time
	}
	require.NoError(t, db.Table("user_memories").Where("id=?", m.ID).Take(&tombstone).Error)
	require.Empty(t, tombstone.Content)
	require.Empty(t, tombstone.Embedding)
	require.NotNil(t, tombstone.DeletedAt)
	// Idempotency keys remain case-sensitive under the database's Unicode CI collation.
	upper := request("POST", "/api/v1/memories", input, "Save-Case-Key", "")
	lower := request("POST", "/api/v1/memories", input, "save-case-key", "")
	require.Equal(t, 201, upper.Code)
	require.Equal(t, 201, lower.Code)
	require.NotEqual(t, readMemory(upper).ID, readMemory(lower).ID)
	// Concurrent retries create one durable row, even across independently constructed callers.
	concurrentKey := uuid.NewString()
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := fresh.Create(context.Background(), scope, d.WriteInput{Content: "保留100%精度", Category: "fact"}, concurrentKey, "user")
			if e != nil {
				errs <- e
				return
			}
			ids <- v.ID
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	oneID := ""
	for id := range ids {
		if oneID == "" {
			oneID = id
		}
		require.Equal(t, oneID, id)
	}
	for i := 0; i < 3; i++ {
		_, err := fresh.Create(context.Background(), scope, d.WriteInput{Content: fmt.Sprintf("事实%d", i), Category: "fact"}, uuid.NewString(), "user")
		require.NoError(t, err)
	}
	seen := map[string]bool{}
	cursor := ""
	for {
		w = request("GET", "/api/v1/memories?page_size=2&cursor="+cursor, nil, "", "")
		require.Equal(t, 200, w.Code, w.Body.String())
		var envelope struct {
			Data d.MemoryPage `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
		for _, row := range envelope.Data.Items {
			require.False(t, seen[row.ID])
			seen[row.ID] = true
		}
		cursor = envelope.Data.NextCursor
		if !envelope.Data.HasMore {
			break
		}
	}
	require.Len(t, seen, 6)
	w = request("GET", "/api/v1/memories?q=%25", nil, "", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "100%")
	require.NotContains(t, w.Body.String(), "事实0")
	require.Equal(t, 400, request("GET", "/api/v1/memories?cursor=bad", nil, "", "").Code)
	// A version fence allows one concurrent correction and rejects the loser.
	var success, conflict int
	var mu sync.Mutex
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := fresh.Update(context.Background(), scope, oneID, d.UpdateInput{WriteInput: d.WriteInput{Content: "修正" + strconv.Itoa(i), Category: "fact"}, ExpectedVersion: 1})
			mu.Lock()
			defer mu.Unlock()
			if e == nil {
				success++
			} else if e == d.ErrConflict {
				conflict++
			} else {
				t.Errorf("unexpected update: %v", e)
			}
		}(i)
	}
	wg.Wait()
	require.Equal(t, 1, success)
	require.Equal(t, 1, conflict)
}
