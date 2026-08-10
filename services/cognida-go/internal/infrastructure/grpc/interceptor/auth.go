package interceptor

import (
	"context"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	agentctx "cognida/internal/model/agent"
)

// TenantMetadataKey 是租户 ID 在 gRPC metadata 中的键。
// gRPC 会将 metadata key 统一小写，Python 侧按此键读取并绑定租户上下文（〔H8〕）。
const TenantMetadataKey = "x-tenant-id"

// TokenAuth 以 per-RPC 凭证形式为出站调用附加 Bearer 令牌，用于 Go↔Python
// 内部服务间的共享密钥鉴权（〔H8〕）。requireTLS 与所在信道的 TLS 状态保持一致：
// 明文回环（127.0.0.1）下为 false 以允许附带令牌，启用 TLS 时为 true 强制加密传输。
type TokenAuth struct {
	token      string
	requireTLS bool
}

// NewTokenAuth 创建 Bearer 令牌凭证。requireTLS 应传入信道是否启用 TLS，
// 使 gRPC 的传输安全校验与实际信道一致（明文信道不会因要求 TLS 而拒发）。
func NewTokenAuth(token string, requireTLS bool) *TokenAuth {
	return &TokenAuth{token: token, requireTLS: requireTLS}
}

// GetRequestMetadata 返回随每次 RPC 发送的鉴权元数据。
func (t *TokenAuth) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + t.token}, nil
}

// RequireTransportSecurity 声明是否要求信道启用传输安全。
// 与信道 TLS 状态绑定：明文回环下返回 false（允许携带令牌），TLS 下返回 true。
func (t *TokenAuth) RequireTransportSecurity() bool {
	return t.requireTLS
}

// tenantIDFromContext 从 Go context 提取租户 ID（由 HTTP 租户中间件注入），
// 转为十进制字符串。无租户时返回空串。
func tenantIDFromContext(ctx context.Context) string {
	if id, ok := agentctx.GetTenantID(ctx); ok && id > 0 {
		return strconv.FormatInt(id, 10)
	}
	return ""
}

// TenantUnary 将租户 ID 透传到下游（Python）gRPC 服务的出站 metadata，
// 使下游可据此做租户隔离与审计。无租户时不附加，保持行为不变。
func TenantUnary() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if tid := tenantIDFromContext(ctx); tid != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, TenantMetadataKey, tid)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// TenantStream 是 TenantUnary 的流式版本。
func TenantStream() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		if tid := tenantIDFromContext(ctx); tid != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, TenantMetadataKey, tid)
		}
		return streamer(ctx, desc, cc, method, opts...)
	}
}
