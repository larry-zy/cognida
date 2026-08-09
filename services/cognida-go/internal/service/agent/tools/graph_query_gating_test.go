package tools

import (
	"context"
	"strings"
	"testing"

	agentctx "cognida/internal/model/agent"
)

// 图谱增强未开启时，graph_query 直接返回提示且不调用图谱服务、不报错。
func TestGraphQuery_GatedWhenDisabled(t *testing.T) {
	// 故意传入 nil 图谱服务；若门控失效会走到 nil 检查报错，从而暴露问题。
	ctx := agentctx.WithGraphEnabled(context.Background(), false)
	res, err := graphQuery(ctx, &GraphQueryRequest{Query: "张三负责哪些项目"}, nil)
	if err != nil {
		t.Fatalf("gated call should not error, got %v", err)
	}
	if res.HasAnswer || res.Count != 0 {
		t.Fatalf("expected empty gated result, got %+v", res)
	}
	if !strings.Contains(res.Answer, "图谱增强未开启") {
		t.Fatalf("expected gating hint in answer, got %q", res.Answer)
	}
}

// 图谱开启时正常调用图谱服务。
func TestGraphQuery_PassthroughWhenEnabled(t *testing.T) {
	ctx := agentctx.WithGraphEnabled(context.Background(), true)
	res, err := graphQuery(ctx, &GraphQueryRequest{Query: "张三与李四的关联"}, &mockGraphService{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Answer != "mock answer" {
		t.Fatalf("expected passthrough to graph service, got %q", res.Answer)
	}
}
