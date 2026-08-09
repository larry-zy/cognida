package interceptor

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type TokenAuth struct {
	token string
}

func NewTokenAuth(token string) *TokenAuth {
	return &TokenAuth{token: token}
}

func (t *TokenAuth) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + t.token}, nil
}

func (t *TokenAuth) RequireTransportSecurity() bool {
	return false
}

type TenantConfig struct {
	Header      string
	GetTenantID func(context.Context) string
}

func TenantUnary(cfg *TenantConfig) grpc.UnaryClientInterceptor {
	if cfg == nil {
		cfg = &TenantConfig{Header: "x-tenant-id"}
	}
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		tenantID := cfg.GetTenantID(ctx)
		if tenantID != "" {
			md := metadata.Pairs(cfg.Header, tenantID)
			ctx = metadata.NewOutgoingContext(ctx, md)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func TenantStream(cfg *TenantConfig) grpc.StreamClientInterceptor {
	if cfg == nil {
		cfg = &TenantConfig{Header: "x-tenant-id"}
	}
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		tenantID := cfg.GetTenantID(ctx)
		if tenantID != "" {
			md := metadata.Pairs(cfg.Header, tenantID)
			ctx = metadata.NewOutgoingContext(ctx, md)
		}
		return streamer(ctx, desc, cc, method, opts...)
	}
}
