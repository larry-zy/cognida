package interceptor

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	agentctx "cognida/internal/model/agent"
)

// TestTokenAuth_Metadata 验证 Bearer 令牌以正确格式附加，
// 且 RequireTransportSecurity 与信道 TLS 状态一致（〔H8〕）。
func TestTokenAuth_Metadata(t *testing.T) {
	ta := NewTokenAuth("s3cr3t", false)
	md, err := ta.GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatalf("GetRequestMetadata err: %v", err)
	}
	if got := md["authorization"]; got != "Bearer s3cr3t" {
		t.Fatalf("authorization = %q, want %q", got, "Bearer s3cr3t")
	}
	if ta.RequireTransportSecurity() {
		t.Fatal("明文信道下 RequireTransportSecurity 应为 false")
	}
	if !NewTokenAuth("x", true).RequireTransportSecurity() {
		t.Fatal("TLS 信道下 RequireTransportSecurity 应为 true")
	}
}

// TestTenantUnary_AppendsTenant 验证 context 携带租户时，出站 metadata 附带 x-tenant-id；
// 无租户时不附带。
func TestTenantUnary_AppendsTenant(t *testing.T) {
	interceptor := TenantUnary()

	capture := func(ctx context.Context) []string {
		var out []string
		invoker := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			if md, ok := metadata.FromOutgoingContext(ctx); ok {
				out = md.Get(TenantMetadataKey)
			}
			return nil
		}
		_ = interceptor(ctx, "/svc/M", nil, nil, nil, invoker)
		return out
	}

	// 有租户
	got := capture(agentctx.WithTenantID(context.Background(), 42))
	if len(got) != 1 || got[0] != "42" {
		t.Fatalf("x-tenant-id = %v, want [42]", got)
	}

	// 无租户：不应附带
	if got := capture(context.Background()); len(got) != 0 {
		t.Fatalf("无租户时不应附带 x-tenant-id, got %v", got)
	}
}
