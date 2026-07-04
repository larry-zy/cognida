package tools

import (
	"context"
	"testing"

	agentctx "link/internal/model/agent"
)

// 有路由 holder 时：写入 holder 并返回 Applied=true，后续检索可读到聚焦范围。
func TestKbRoute_WritesHolder(t *testing.T) {
	sel := agentctx.NewRouteSelection()
	ctx := agentctx.WithRouteSelection(context.Background(), sel)

	res, err := kbRoute(ctx, &KbRouteRequest{KBIDs: []string{"kb1", "kb2"}, Reason: "最相关"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Applied {
		t.Fatalf("expected Applied=true when holder present")
	}
	got := sel.Get()
	if len(got) != 2 || got[0] != "kb1" || got[1] != "kb2" {
		t.Fatalf("holder not written correctly: %v", got)
	}
	if len(res.ScopedTo) != 2 {
		t.Fatalf("expected ScopedTo echo, got %v", res.ScopedTo)
	}
}

// 无路由 holder 时（防御路径）：不报错，Applied=false，不 panic。
func TestKbRoute_NoHolderDefensive(t *testing.T) {
	res, err := kbRoute(context.Background(), &KbRouteRequest{KBIDs: []string{"kb1"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Applied {
		t.Fatalf("expected Applied=false when holder absent")
	}
	if res.Note == "" {
		t.Fatalf("expected explanatory note on defensive path")
	}
}

// nil holder 指针也走防御路径（GetRouteSelection 返回非 nil pointer 才应用）。
func TestKbRoute_NilHolder(t *testing.T) {
	var sel *agentctx.RouteSelection
	ctx := agentctx.WithRouteSelection(context.Background(), sel)

	res, err := kbRoute(ctx, &KbRouteRequest{KBIDs: []string{"kb1"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Applied {
		t.Fatalf("expected Applied=false when holder is nil")
	}
}
