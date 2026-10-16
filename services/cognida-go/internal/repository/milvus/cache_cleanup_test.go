package milvus

import (
	"context"
	"errors"
	"io"
	"testing"

	d "cognida/internal/model/cache"
	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type cleanupIteratorFixture struct {
	pages []milvusclient.ResultSet
	tail  error
	at    int
}

func (i *cleanupIteratorFixture) Next(context.Context) (milvusclient.ResultSet, error) {
	if i.at < len(i.pages) {
		p := i.pages[i.at]
		i.at++
		return p, nil
	}
	if i.tail != nil {
		return milvusclient.ResultSet{}, i.tail
	}
	return milvusclient.ResultSet{}, io.EOF
}

type cleanupClientFixture struct {
	iter     milvusclient.QueryIterator
	opt      milvusclient.QueryIteratorOption
	deletion milvusclient.DeleteOption
	count    int64
	err      error
}

func (c *cleanupClientFixture) QueryIterator(_ context.Context, o milvusclient.QueryIteratorOption, _ ...grpc.CallOption) (milvusclient.QueryIterator, error) {
	c.opt = o
	return c.iter, nil
}
func (c *cleanupClientFixture) Delete(_ context.Context, o milvusclient.DeleteOption, _ ...grpc.CallOption) (milvusclient.DeleteResult, error) {
	c.deletion = o
	return milvusclient.DeleteResult{DeleteCount: c.count}, c.err
}
func cleanupPage(ids ...string) milvusclient.ResultSet {
	tids, ats, agents := make([]int64, len(ids)), make([]int64, len(ids)), make([]string, len(ids))
	for i := range ids {
		tids[i] = 7
		ats[i] = 100
		agents[i] = "rag"
	}
	return milvusclient.ResultSet{ResultCount: len(ids), Fields: milvusclient.DataSet{column.NewColumnVarChar("cache_id", ids), column.NewColumnInt64("tenant_id", tids), column.NewColumnVarChar("agent_type", agents), column.NewColumnInt64("created_at", ats)}}
}
func TestCacheCleanupEnumeratesAllPagesOrFailsClosed(t *testing.T) {
	f := &cleanupClientFixture{iter: &cleanupIteratorFixture{pages: []milvusclient.ResultSet{cleanupPage("a", "b"), cleanupPage("c")}}}
	r := &CacheVectorRepository{cleanupClient: f, collectionName: "scoped"}
	rows, err := r.ListCleanupCandidates(context.Background(), 7, 200, 3, 2)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	req, err := f.opt.Request()
	require.NoError(t, err)
	require.Equal(t, "scoped", req.CollectionName)
	require.Equal(t, "tenant_id == 7 && created_at <= 200", req.Expr)
	require.EqualValues(t, entity.ClStrong, req.ConsistencyLevel)
	require.False(t, req.UseDefaultConsistency)
	require.Equal(t, 2, f.opt.BatchSize())
	require.Less(t, f.opt.Limit(), int64(0))
	for _, mode := range []string{"late failure", "over limit", "duplicate", "wrong owner", "wrong type", "empty page", "missing column", "invalid identity", "page embedded error"} {
		t.Run(mode, func(t *testing.T) {
			p := cleanupPage("c")
			it := &cleanupIteratorFixture{pages: []milvusclient.ResultSet{cleanupPage("a", "b"), p}}
			maximum := 3
			switch mode {
			case "late failure":
				it.tail = errors.New("late page failure")
			case "over limit":
				maximum = 2
			case "duplicate":
				it.pages[1] = cleanupPage("b")
			case "wrong owner":
				it.pages[1].Fields[1] = column.NewColumnInt64("tenant_id", []int64{8})
			case "wrong type":
				it.pages[1].Fields[1] = column.NewColumnVarChar("tenant_id", []string{"7"})
			case "empty page":
				it.pages[1] = milvusclient.ResultSet{}
			case "missing column":
				it.pages[1].Fields = it.pages[1].Fields[:3]
			case "invalid identity":
				it.pages[1] = cleanupPage("z\" || true")
			case "page embedded error":
				it.pages[1].Err = errors.New("partial result error")
			}
			f.iter = it
			rows, err := r.ListCleanupCandidates(context.Background(), 7, 200, maximum, 2)
			require.Error(t, err)
			require.Nil(t, rows)
		})
	}
}
func TestCacheCleanupDeleteHasExactFenceAndHonestOutcome(t *testing.T) {
	f := &cleanupClientFixture{count: 1}
	r := &CacheVectorRepository{cleanupClient: f, collectionName: "scoped"}
	v := d.CacheVectorIdentity{TenantID: 7, CacheID: "abc", AgentType: "rag", CreatedAt: 100}
	n, err := r.DeleteCleanupCandidate(context.Background(), v)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	require.Equal(t, `tenant_id == 7 && cache_id == "abc" && agent_type == "rag" && created_at == 100`, f.deletion.Request().Expr)
	for _, code := range []codes.Code{codes.InvalidArgument, codes.PermissionDenied, codes.DeadlineExceeded, codes.Unavailable} {
		f.err = status.Error(code, "test error")
		n, err = r.DeleteCleanupCandidate(context.Background(), v)
		var de *d.CacheDeleteError
		require.ErrorAs(t, err, &de)
		require.Equal(t, code == codes.InvalidArgument || code == codes.PermissionDenied, de.NotApplied)
		require.Zero(t, n)
	}
	f.err = nil
	f.count = 0
	n, err = r.DeleteCleanupCandidate(context.Background(), v)
	require.NoError(t, err)
	require.Zero(t, n)
	f.count = 2
	_, err = r.DeleteCleanupCandidate(context.Background(), v)
	require.ErrorIs(t, err, d.ErrCleanupReconcile)
	f.deletion = nil
	v.CacheID = "bad\""
	_, err = r.DeleteCleanupCandidate(context.Background(), v)
	require.Error(t, err)
	require.Nil(t, f.deletion)
}
