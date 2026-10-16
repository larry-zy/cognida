package datasource

import (
	agent "cognida/internal/model/agent"
	m "cognida/internal/model/datasource"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

func (s *Service) WithDurableQueries(fn m.DurableQueryExecutor) *Service {
	s.durableQuery = fn
	return s
}
func (s *Service) ExecuteDurableQuery(ctx context.Context, tenant, user int64, id string, request m.QueryRequest, key string) (*m.QueryResult, error) {
	if s.durableQuery == nil || tenant < 1 || user < 1 || key == "" {
		return nil, fmt.Errorf("durable query requires authenticated owner and operation key")
	}
	source, e := s.repo.Get(ctx, id, tenant)
	if e != nil || source == nil {
		return nil, ErrNotFound
	}
	if source.Type == m.TypeBigQuery && request.MaxScanBytes == 0 {
		opts, e := parseBigQueryOptions(source)
		if e != nil {
			return nil, e
		}
		request.MaxScanBytes = opts.MaxScanBytes
	}
	return s.durableQuery(ctx, tenant, user, id, request, key)
}

// sampleREST uses live REST metadata and a durable query; it never opens a SQL
// connection or silently substitutes a different database for a remote engine.
func (s *Service) sampleREST(ctx context.Context, tenant int64, id, table string, limit int) (*m.QueryResult, bool, error) {
	source, e := s.repo.Get(ctx, id, tenant)
	if e != nil || source == nil {
		return nil, true, ErrNotFound
	}
	if source.Type != m.TypeBigQuery {
		return nil, false, nil
	}
	tables, e := s.ListTables(ctx, tenant, id)
	if e != nil {
		return nil, true, e
	}
	found := false
	for _, v := range tables {
		if v.Name == table {
			found = true
			break
		}
	}
	if !found {
		return nil, true, fmt.Errorf("datasource table is not currently authorized")
	}
	user, key := m.QueryIdentity(ctx)
	if user == 0 {
		user = agent.MustGetUserID(ctx)
	}
	if key == "" {
		key = agent.MustGetRequestID(ctx)
	}
	if user < 1 || key == "" {
		return nil, true, fmt.Errorf("remote sampling requires user identity and an idempotency key")
	}
	in := m.QueryRequest{SQL: "SELECT * FROM " + (bigQueryDriver{}).QuoteIdent(table), MaxRows: limit, MaxBytes: 8 << 20, Timeout: 30 * time.Second}
	raw, _ := json.Marshal([]any{"sample", key})
	digest := sha256.Sum256(raw)
	result, e := s.ExecuteDurableQuery(ctx, tenant, user, id, in, hex.EncodeToString(digest[:]))
	return result, true, e
}
