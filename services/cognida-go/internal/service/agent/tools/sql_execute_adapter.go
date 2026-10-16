package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	agentctx "cognida/internal/model/agent"
	ds "cognida/internal/model/datasource"
	"cognida/internal/service/agent/resultstore"
)

func sqlExecuteAdapter(ctx context.Context, req *SQLExecuteRequest, id string, provider ds.QueryProvider, store resultstore.Store, registry *resultstore.SessionRegistry) (*SQLExecuteResult, error) {
	start := time.Now()
	if err := validateSQL(req.SQL); err != nil {
		return nil, err
	}
	adapter, source, err := provider.AcquireQuery(ctx, agentctx.MustGetTenantID(ctx), id)
	if err != nil {
		return nil, err
	}
	maxRows := req.MaxRows
	if maxRows <= 0 {
		maxRows = 100
	}
	if maxRows > 1000 {
		maxRows = 1000
	}
	request := ds.QueryRequest{SQL: req.SQL, MaxRows: maxRows, MaxBytes: 8 << 20, Timeout: 30 * time.Second}
	var result *ds.QueryResult
	if adapter.Capabilities().Async {
		durable, ok := provider.(ds.DurableQueryProvider)
		if !ok || agentctx.MustGetUserID(ctx) < 1 || agentctx.MustGetRequestID(ctx) == "" {
			return nil, fmt.Errorf("async SQL requires a durable query provider and authenticated request")
		}
		raw, _ := json.Marshal([]any{"agent-sql", agentctx.MustGetRequestID(ctx), agentctx.MustGetSessionID(ctx), id, request})
		digest := sha256.Sum256(raw)
		result, err = durable.ExecuteDurableQuery(ctx, agentctx.MustGetTenantID(ctx), agentctx.MustGetUserID(ctx), id, request, hex.EncodeToString(digest[:]))
	} else {
		result, err = adapter.Query(ctx, request)
	}
	if err != nil {
		return nil, fmt.Errorf("external query failed: %w", err)
	}
	// The acquisition above rechecks current source authorization. Do not reuse a
	// cache across source versions; every successful run records its exact scope.
	owner := resultstore.OwnerKey(agentctx.MustGetTenantID(ctx), agentctx.MustGetSessionID(ctx))
	data := &resultstore.Result{Owner: owner, Columns: result.Columns, ColumnTypes: result.ColumnTypes, Rows: result.Rows, SourceSQL: result.ExecutedSQL, DatasourceID: source.ID, SourceVersion: result.SourceVersion}
	warning := ""
	if result.Truncated {
		warning = fmt.Sprintf("查询结果超过 %d 行；保存的是有界快照", maxRows)
	}
	if store != nil {
		data.ResultID, err = store.Put(ctx, data, resultstore.DefaultTTL)
		if err != nil {
			return nil, fmt.Errorf("query result persistence failed: %w", err)
		}
	}
	if registry != nil && data.ResultID != "" {
		registry.RecordFetch(owner, resultstore.SQLSignature(source.ID+":"+result.SourceVersion, result.ExecutedSQL), data.ResultID)
	}
	envelope := resultstore.BuildEnvelope(data, resultstore.DefaultSampleRows)
	return &SQLExecuteResult{ResultID: data.ResultID, Columns: envelope.Columns, Dtypes: envelope.Dtypes, RowCount: envelope.RowCount, Samples: envelope.Samples, Aggregates: envelope.Aggregates, Truncated: envelope.Truncated || result.Truncated, LatencyMs: time.Since(start).Milliseconds(), Warning: warning, ExecutedSQL: result.ExecutedSQL}, nil
}
