package retriever

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/milvus-io/milvus-proto/go-api/v2/commonpb"
	"github.com/milvus-io/milvus-proto/go-api/v2/milvuspb"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type loadServer struct {
	milvuspb.UnimplementedMilvusServiceServer
	progress int64
	fail     bool
	deadline atomic.Int64
}

func (s *loadServer) LoadCollection(ctx context.Context, _ *milvuspb.LoadCollectionRequest) (*commonpb.Status, error) {
	if deadline, ok := ctx.Deadline(); ok {
		s.deadline.Store(time.Until(deadline).Nanoseconds())
	}
	return &commonpb.Status{}, nil
}
func (s *loadServer) GetLoadingProgress(context.Context, *milvuspb.GetLoadingProgressRequest) (*milvuspb.GetLoadingProgressResponse, error) {
	if s.fail {
		return nil, status.Error(codes.Internal, "missing segment")
	}
	return &milvuspb.GetLoadingProgressResponse{Status: &commonpb.Status{}, Progress: s.progress}, nil
}

func TestLoadKnowledgeBaseWaitIsBoundedAndPropagatesFailure(t *testing.T) {
	for _, tc := range []struct {
		name        string
		progress    int64
		fail, async bool
		timeout     time.Duration
		wantError   bool
	}{
		{"loading_timeout", 0, false, false, 25 * time.Millisecond, true},
		{"load_failure", 0, true, false, time.Second, true},
		{"loaded", 100, false, false, time.Second, false},
		{"async_has_default_rpc_deadline", 0, false, true, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			server := grpc.NewServer()
			fixture := &loadServer{progress: tc.progress, fail: tc.fail}
			milvuspb.RegisterMilvusServiceServer(server, fixture)
			go func() { _ = server.Serve(listener) }()
			t.Cleanup(server.Stop)
			client, err := milvusclient.New(context.Background(), &milvusclient.ClientConfig{Address: listener.Addr().String(), DisableConn: true})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, client.Close(context.Background())) })
			ctx := context.Background()
			if tc.timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tc.timeout)
				defer cancel()
			}
			err = (&VectorRetriever{client: client}).LoadKnowledgeBase(ctx, 0, tc.async)
			if tc.wantError {
				require.Error(t, err)
				require.Contains(t, err.Error(), "await collection link load")
			} else {
				require.NoError(t, err)
			}
			require.Positive(t, fixture.deadline.Load())
			require.LessOrEqual(t, fixture.deadline.Load(), int64(90*time.Second))
		})
	}
}
