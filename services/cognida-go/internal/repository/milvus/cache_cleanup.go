package milvus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"

	"cognida/internal/model/cache"
	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type cacheCleanupClient interface {
	QueryIterator(context.Context, milvusclient.QueryIteratorOption, ...grpc.CallOption) (milvusclient.QueryIterator, error)
	Delete(context.Context, milvusclient.DeleteOption, ...grpc.CallOption) (milvusclient.DeleteResult, error)
}

func (r *CacheVectorRepository) collection() string {
	if r.collectionName != "" {
		return r.collectionName
	}
	return SemanticCacheCollectionName
}

// NewCacheVectorRepositoryForCollection binds an operator-selected collection
// to an explicit client. It does not create, load or otherwise mutate anything.
func NewCacheVectorRepositoryForCollection(client *milvusclient.Client, name string, dim int) (*CacheVectorRepository, error) {
	if client == nil || !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`).MatchString(name) || dim < 1 {
		return nil, cache.ErrCleanupInvalid
	}
	return &CacheVectorRepository{client: client, collectionName: name, dim: dim}, nil
}

func (r *CacheVectorRepository) cleanupBackend() cacheCleanupClient {
	if r.cleanupClient != nil {
		return r.cleanupClient
	}
	return r.client
}

// ListCleanupCandidates reads every page through EOF before returning rows.
// The cutoff excludes fresh in-flight writes; the Redis absent fence, rather
// than age alone, protects against late content publication.
func (r *CacheVectorRepository) ListCleanupCandidates(ctx context.Context, tenant, before int64, maximum, batch int) ([]cache.CacheVectorIdentity, error) {
	if tenant < 1 || before < 1 || maximum < 1 || maximum > 100000 || batch < 1 || batch > 1000 {
		return nil, cache.ErrCleanupInvalid
	}
	opt := milvusclient.NewQueryIteratorOption(r.collection()).WithBatchSize(batch).
		WithFilter(fmt.Sprintf("tenant_id == %d && created_at <= %d", tenant, before)).
		WithOutputFields("cache_id", "tenant_id", "agent_type", "created_at").WithConsistencyLevel(entity.ClStrong)
	iter, err := r.cleanupBackend().QueryIterator(ctx, opt)
	if err != nil {
		return nil, fmt.Errorf("create cache vector iterator: %w", err)
	}
	rows := make([]cache.CacheVectorIdentity, 0)
	last := ""
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := iter.Next(ctx)
		if page.Err != nil {
			return nil, fmt.Errorf("cache vector page result: %w", page.Err)
		}
		if errors.Is(err, io.EOF) {
			if page.ResultCount != 0 {
				return nil, cache.ErrCleanupIncomplete
			}
			return rows, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read cache vector page: %w", err)
		}
		if page.ResultCount < 1 || len(rows)+page.ResultCount > maximum {
			return nil, cache.ErrCleanupIncomplete
		}
		ids, okID := page.GetColumn("cache_id").(*column.ColumnVarChar)
		tenants, okTenant := page.GetColumn("tenant_id").(*column.ColumnInt64)
		agents, okAgent := page.GetColumn("agent_type").(*column.ColumnVarChar)
		created, okCreated := page.GetColumn("created_at").(*column.ColumnInt64)
		if !okID || !okTenant || !okAgent || !okCreated || ids.Len() != page.ResultCount || tenants.Len() != page.ResultCount || agents.Len() != page.ResultCount || created.Len() != page.ResultCount {
			return nil, cache.ErrCleanupIncomplete
		}
		for i := 0; i < page.ResultCount; i++ {
			id, e1 := ids.Value(i)
			tid, e2 := tenants.Value(i)
			agent, e3 := agents.Value(i)
			at, e4 := created.Value(i)
			row := cache.CacheVectorIdentity{CacheID: id, TenantID: tid, AgentType: agent, CreatedAt: at}
			if e1 != nil || e2 != nil || e3 != nil || e4 != nil || row.Validate() != nil || tid != tenant || at > before || id <= last {
				return nil, cache.ErrCleanupIncomplete
			}
			rows = append(rows, row)
			last = id
		}
	}
}

func (r *CacheVectorRepository) DeleteCleanupCandidate(ctx context.Context, v cache.CacheVectorIdentity) (int64, error) {
	if err := v.Validate(); err != nil {
		return 0, &cache.CacheDeleteError{Cause: err, NotApplied: true}
	}
	if err := ctx.Err(); err != nil {
		return 0, &cache.CacheDeleteError{Cause: err, NotApplied: true}
	}
	expr := fmt.Sprintf("tenant_id == %d && cache_id == %q && agent_type == %q && created_at == %d", v.TenantID, v.CacheID, v.AgentType, v.CreatedAt)
	result, err := r.cleanupBackend().Delete(ctx, milvusclient.NewDeleteOption(r.collection()).WithExpr(expr))
	if err != nil {
		code := status.Code(err)
		definite := code == codes.InvalidArgument || code == codes.PermissionDenied || code == codes.Unauthenticated || code == codes.FailedPrecondition
		return 0, &cache.CacheDeleteError{Cause: err, NotApplied: definite}
	}
	if result.DeleteCount < 0 || result.DeleteCount > 1 {
		return 0, &cache.CacheDeleteError{Cause: cache.ErrCleanupReconcile}
	}
	return result.DeleteCount, nil
}
