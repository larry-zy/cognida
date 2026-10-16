//go:build integration

package handler

import (
	"cognida/internal/handler/middleware"
	d "cognida/internal/model/agent"
	repo "cognida/internal/repository/mysql"
	service "cognida/internal/service/agent"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type configConsumer struct{ seen context.Context }

func (c *configConsumer) Execute(ctx context.Context, _ string, _ string) (string, error) {
	c.seen = ctx
	return "answer", nil
}
func (c *configConsumer) ExecuteStream(ctx context.Context, _ string, _ string) (<-chan *d.StreamEvent, error) {
	c.seen = ctx
	out := make(chan *d.StreamEvent)
	close(out)
	return out, nil
}
func (c *configConsumer) GetStatus(string) (d.AgentStatus, error) { return d.AgentStatusIdle, nil }

// Actual TCP HTTP + MySQL + runtime consumer. Authentication identity is an
// explicit fixture; these tests do not claim to validate login or model quality.
func TestAgentConfigMySQLHTTPPersistenceIsolationReplayAndRuntime(t *testing.T) {
	dsn := os.Getenv("COGNIDA_AGENT_CONFIG_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated agent config DSN required")
	}
	cfg, err := driver.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(cfg.DBName, "cognida_review_"))
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sql, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sql.Close()) })
	tenant := time.Now().UnixNano() & 0x1fffffffffffff
	user := int64(109)
	t.Cleanup(func() {
		for _, table := range []string{"agent_preference_updates", "agent_preferences"} {
			require.NoError(t, db.Table(table).Where("tenant_id IN ?", []int64{tenant, tenant + 1}).Delete(map[string]any{}).Error)
		}
	})
	config := service.NewConfigService(repo.NewAgentPreferencesRepository(db))
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-Identity") != "missing" {
			p := tenant
			u := user
			if c.GetHeader("X-Test-Identity") == "other-user" {
				u++
			}
			if c.GetHeader("X-Test-Identity") == "other-tenant" {
				p++
			}
			c.Set("tenant_id", p)
			c.Set("user_id", u)
		}
		c.Next()
	})
	r.Use(middleware.NewTenantMiddleware().Apply())
	r.Use(middleware.NewIdempotency(nil).Apply())
	h := NewAgentHandler(nil, nil, config, nil, nil, nil)
	r.GET("/api/v1/agent/config", h.GetConfig)
	r.POST("/api/v1/agent/config", h.UpdateConfig)
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)
	call := func(method, body, key, identity string) (int, d.Preferences) {
		req, e := http.NewRequest(method, server.URL+"/api/v1/agent/config", strings.NewReader(body))
		require.NoError(t, e)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		req.Header.Set("X-Test-Identity", identity)
		response, e := server.Client().Do(req)
		require.NoError(t, e)
		defer response.Body.Close()
		raw, e := io.ReadAll(response.Body)
		require.NoError(t, e)
		var data struct {
			Data d.Preferences `json:"data"`
		}
		require.NoError(t, json.Unmarshal(raw, &data), string(raw))
		return response.StatusCode, data.Data
	}
	status, initial := call("GET", "", "", "")
	require.Equal(t, 200, status)
	require.Zero(t, initial.Version)
	status, _ = call("GET", "", "", "missing")
	require.Equal(t, 400, status) // TenantMiddleware rejects missing fixture identity before the handler.
	body := `{"expected_version":0,"agentic_rag":{"max_iterations":2,"default_top_k":7,"rerank_enabled":false},"deep_research":{"max_sources":9}}`
	status, first := call("POST", body, "Private-Key", "")
	require.Equal(t, 200, status)
	require.Equal(t, int64(1), first.Version)
	for _, identity := range []string{"other-user", "other-tenant"} {
		status, p := call("GET", "", "", identity)
		require.Equal(t, 200, status)
		require.Zero(t, p.Version)
		require.Nil(t, p.AgenticRAG.DefaultTopK)
	}
	status, replay := call("POST", body, "Private-Key", "")
	require.Equal(t, 200, status)
	require.Equal(t, first, replay)
	status, _ = call("POST", strings.Replace(body, `"default_top_k":7`, `"default_top_k":8`, 1), "Private-Key", "")
	require.Equal(t, 409, status)
	status, _ = call("POST", body, "new-stale-key", "")
	require.Equal(t, 409, status)
	for _, bad := range []string{`{}`, `null`, body + ` {}`, `{"expected_version":1,"admin":true}`, `{"expected_version":1,"agentic_rag":{"max_iterations":21}}`, `{"expected_version":1,"agentic_rag":{"search_config":{"api_key":"do-not-echo"}}}`} {
		status, _ = call("POST", bad, "bad", "")
		require.Equal(t, 400, status, bad)
	}
	status, _ = call("POST", body, "", "")
	require.Equal(t, 400, status)
	// Simultaneous edits from the same visible revision: exactly one survives.
	statuses := make(chan int, 8)
	var wg sync.WaitGroup
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			code, _ := call("POST", fmt.Sprintf(`{"expected_version":1,"agentic_rag":{"default_top_k":%d}}`, n+10), "race-"+strconv.Itoa(n), "")
			statuses <- code
		}(n)
	}
	wg.Wait()
	close(statuses)
	winners := 0
	for code := range statuses {
		if code == 200 {
			winners++
		} else {
			require.Equal(t, 409, code)
		}
	}
	require.Equal(t, 1, winners)
	// Reconstructing service/repository is the same persistence boundary as restart.
	restarted := service.NewConfigService(repo.NewAgentPreferencesRepository(db))
	current, err := restarted.Read(context.Background(), d.ConfigScope{TenantID: tenant, UserID: user})
	require.NoError(t, err)
	require.Equal(t, int64(2), current.Version)
	status, replay = call("POST", body, "Private-Key", "")
	require.Equal(t, 200, status)
	require.Equal(t, first, replay)
	// Effective consumer reads the persisted defaults once, not just GET echo.
	consumer := &configConsumer{}
	executor := service.NewExecuteService(consumer).WithConfig(restarted)
	ctx := d.NewAgentContext(context.Background(), "", tenant, user, "query")
	_, err = executor.Execute(ctx, &service.AgenticRAGRequest{AgentID: "agent-rag-001", Query: "query"})
	require.NoError(t, err)
	params, ok := d.GetRetrievalParams(consumer.seen)
	require.True(t, ok)
	require.Equal(t, *current.AgenticRAG.DefaultTopK, *params.TopK)
	require.Equal(t, 2, d.EffectiveIterationLimit(consumer.seen, 8))
	stream, err := executor.ExecuteStream(ctx, &service.AgenticRAGRequest{AgentID: "agent-rag-001", Query: "query"})
	require.NoError(t, err)
	for range stream {
	}
	require.Equal(t, 2, d.EffectiveIterationLimit(consumer.seen, 8))
	status, reset := call("POST", `{"expected_version":2,"agentic_rag":{"max_iterations":null,"default_top_k":null}}`, "reset", "")
	require.Equal(t, 200, status)
	require.Equal(t, int64(3), reset.Version)
	require.Nil(t, reset.AgenticRAG.DefaultTopK)
	require.Equal(t, *current.AgenticRAG.DefaultTopK, *params.TopK, "in-flight snapshot stays fixed")
	status, own := call("POST", body, "Private-Key", "other-user")
	require.Equal(t, 200, status)
	require.Equal(t, int64(1), own.Version)
	t.Log("real HTTP/MySQL: scope isolation, CAS race, payload-bound replay, restart persistence, runtime defaults and reset verified")
}
