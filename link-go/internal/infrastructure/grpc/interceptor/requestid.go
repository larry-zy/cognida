package interceptor

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	agentctx "link/internal/model/agent"
)

// RequestIDMetadataKey 是 request_id 在 gRPC metadata 中的键。
// gRPC 会将 metadata key 统一小写，Python 侧按此键读取。
const RequestIDMetadataKey = "x-request-id"

// requestIDFromContext 从 Go context 提取 request_id（由 HTTP TraceMiddleware 注入）。
func requestIDFromContext(ctx context.Context) string {
	if id, ok := agentctx.GetRequestID(ctx); ok {
		return id
	}
	return ""
}

// RequestIDUnary 将 request_id 透传到下游（Python）gRPC 服务的出站 metadata。
// 使下游可将同一 request_id 绑定到自身日志，实现跨进程链路追踪。
func RequestIDUnary() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if rid := requestIDFromContext(ctx); rid != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, RequestIDMetadataKey, rid)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// RequestIDStream 是 RequestIDUnary 的流式版本。
func RequestIDStream() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		if rid := requestIDFromContext(ctx); rid != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, RequestIDMetadataKey, rid)
		}
		return streamer(ctx, desc, cc, method, opts...)
	}
}
