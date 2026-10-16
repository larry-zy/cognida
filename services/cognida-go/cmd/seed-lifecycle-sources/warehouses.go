package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	agentctx "cognida/internal/model/agent"
	ds "cognida/internal/model/datasource"
	lifecycle "cognida/internal/model/lifecycle"
	"cognida/internal/model/semantic"
	"cognida/internal/repository/mysql"
	"cognida/internal/service/agent/resultstore"
	"cognida/internal/service/agent/tools"
	"cognida/internal/service/datasource"
	"cognida/internal/service/queryjob"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// This fixture uses only task-owned loopback containers and the previously
// authenticated isolated account. Source data is queried, never inserted into
// ResultStore by the fixture. The generated scenario goes through the real UI.
func seedWarehouse(api apiClient, db *gorm.DB, dir string, owner *fixture, engine string) error {
	ports := map[string]int{"mysql": 62301, "postgres": 62309, "clickhouse": 62305, "doris": 62306, "starrocks": 62307, "hive": 62308}
	port, ok := ports[engine]
	if !ok {
		return fmt.Errorf("unsupported isolated warehouse %q", engine)
	}
	input := datasource.SaveInput{Name: "warehouse-" + engine + "-" + uuid.NewString(), Type: engine, Host: "127.0.0.1", Port: port, DatabaseName: "cognida_query_test", Username: "acceptance", Password: "acceptance_local_only"}
	if engine == "hive" {
		input.Username = "hive"
		input.Password = ""
		input.Extra = json.RawMessage(`{"auth":"NONE","tls":false}`)
	}
	if engine == "postgres" {
		input.Extra = json.RawMessage(`{"sslmode":"disable"}`)
	}
	if engine == "clickhouse" {
		input.Extra = json.RawMessage(`{"secure":false}`)
	}
	var source datasource.Sanitized
	if e := api.call("POST", "/datasources", input, &source); e != nil {
		return e
	}
	if source.ID == "" {
		return fmt.Errorf("no registered warehouse ID")
	}
	var session struct {
		ID string `json:"id"`
	}
	if e := api.call("POST", "/sessions", map[string]any{"title": input.Name, "agent_type": "text2sql"}, &session); e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	ctx = agentctx.WithToolScope(agentctx.WithRequestID(agentctx.WithSessionID(agentctx.WithUserID(agentctx.WithTenantID(ctx, owner.TenantID), owner.UserID), session.ID), uuid.NewString()), "read")
	cipher, e := datasource.NewCipher(os.Getenv("DATASOURCE_SECRET_KEY"))
	if e != nil {
		return e
	}
	repo := mysql.NewDataSourceRepository(db)
	manager := datasource.NewConnectionManager(repo, cipher, datasource.DefaultPoolOptions())
	defer manager.Close()
	service := datasource.NewService(repo, cipher, manager, nil)
	jobs, e := queryjob.New(mysql.NewQueryJobRepository(db), service, func(c context.Context, p lifecycle.Principal, id string) (*ds.DataSource, error) {
		if p.TenantID != owner.TenantID || p.UserID != owner.UserID {
			return nil, fmt.Errorf("fixture owner mismatch")
		}
		return repo.Get(c, id, p.TenantID)
	}, cipher)
	if e != nil {
		return e
	}
	service.WithDurableQueries(jobs.Execute)
	metadata, e := service.DescribeTable(ctx, owner.TenantID, source.ID, "samples")
	if e != nil {
		return e
	}
	if metadata == nil {
		return fmt.Errorf("missing live warehouse metadata")
	}
	modelID, tableID := uuid.NewString(), uuid.NewString()
	name := "warehouse_" + engine + "_" + uuid.NewString()
	bundle := &semantic.ModelBundle{Model: &semantic.SemanticModel{ID: modelID, TenantID: owner.TenantID, Name: name, Version: 1, Status: semantic.ModelStatusActive}, LogicalTables: []*semantic.LogicalTable{{ID: tableID, ModelID: modelID, Name: "samples", PhysicalTable: "samples", DatabaseID: source.ID}}, Dimensions: []*semantic.Dimension{{ID: uuid.NewString(), ModelID: modelID, LogicalTableID: tableID, Name: "code", Expr: "code", DataType: "string"}}, Measures: []*semantic.Measure{{ID: uuid.NewString(), ModelID: modelID, LogicalTableID: tableID, Name: "amount", Expr: "amount", Aggregation: semantic.AggMax, DataType: "decimal"}}}
	semanticRepo := mysql.NewSemanticRepository(db)
	if e = semanticRepo.UpsertBundle(ctx, bundle); e != nil {
		return e
	}
	semanticTool := tools.NewSemanticQueryTool(semanticRepo, nil, nil, nil, service)
	request, _ := json.Marshal(tools.SemanticQueryRequest{Model: name, Metrics: []string{"amount"}, Dimensions: []string{"code"}, Limit: 10})
	compiled, e := semanticTool.InvokableRun(ctx, string(request))
	if e != nil {
		return e
	}
	var query tools.SemanticQueryResult
	if e = json.Unmarshal([]byte(compiled), &query); e != nil {
		return e
	}
	if !query.Covered || query.DatabaseID != source.ID {
		return fmt.Errorf("semantic query did not bind the exact source")
	}
	redisDB, _ := strconv.Atoi(os.Getenv("REDIS_DB"))
	client := redis.NewClient(&redis.Options{Addr: os.Getenv("REDIS_ADDR"), Password: os.Getenv("REDIS_PASSWORD"), DB: redisDB})
	defer client.Close()
	store := resultstore.NewRedisStore(client)
	sqlTool := tools.NewSQLExecuteTool(db, service, store, nil, resultstore.NewSessionRegistry(resultstore.DefaultTTL))
	request, _ = json.Marshal(tools.SQLExecuteRequest{SQL: query.SQL, DatabaseID: source.ID, MaxRows: 10})
	executed, e := sqlTool.InvokableRun(ctx, string(request))
	if e != nil {
		return e
	}
	var result tools.SQLExecuteResult
	if e = json.Unmarshal([]byte(executed), &result); e != nil {
		return e
	}
	if result.ResultID == "" || result.RowCount != 2 {
		return fmt.Errorf("warehouse query returned unexpected rows: %d", result.RowCount)
	}
	actual, e := store.GetExact(ctx, resultstore.OwnerKey(owner.TenantID, session.ID), result.ResultID)
	if e != nil {
		return e
	}
	found := false
	exactCells := []map[string]any{}
	for index, row := range actual.Rows {
		if row["code"] == "00123" {
			found = true
			if row["amount"] != "9007199254740993.123456789" {
				return fmt.Errorf("warehouse query lost decimal precision: %v", row["amount"])
			}
			exactCells = append(exactCells, map[string]any{"row": index, "column": "code", "value": "00123"}, map[string]any{"row": index, "column": "amount", "value": row["amount"]})
		}
	}
	if !found {
		return fmt.Errorf("warehouse query lost text code")
	}
	manifest := map[string]any{"engine": engine, "datasource_id": source.ID, "session_id": session.ID, "model": bundle, "query": query, "result": actual, "expires_at": time.Now().Add(resultstore.DefaultTTL)}
	body, _ := json.MarshalIndent(manifest, "", "  ")
	if e = os.WriteFile(filepath.Join(dir, "warehouse-"+engine+"-receipt.json"), body, 0600); e != nil {
		return e
	}
	scenarios := map[string]any{"scenarios": []any{map[string]any{"id": "warehouse-" + engine, "source": "result", "session_id": session.ID, "result_id": result.ResultID, "expected": map[string]any{"record_count": 2, "published_exact_strings": exactCells}}}}
	body, _ = json.MarshalIndent(scenarios, "", "  ")
	return os.WriteFile(filepath.Join(dir, "warehouse-"+engine+"-scenario.json"), body, 0600)
}
