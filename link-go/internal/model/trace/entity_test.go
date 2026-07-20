package trace

import (
	"testing"
	"time"
)

func ts(sec int) time.Time { return time.Unix(int64(sec), 0) }

// TestBuildTree_Forest 验证多根、父子挂接与同层按开始时间排序。
func TestBuildTree_Forest(t *testing.T) {
	spans := []*Span{
		{SpanID: "root", ParentSpanID: "", Name: "agent.chat", StartTime: ts(10)},
		{SpanID: "c2", ParentSpanID: "root", Name: "tool.b", StartTime: ts(30)},
		{SpanID: "c1", ParentSpanID: "root", Name: "tool.a", StartTime: ts(20)},
		{SpanID: "gc", ParentSpanID: "c1", Name: "sub", StartTime: ts(25)},
		{SpanID: "orphanRoot", ParentSpanID: "missing", Name: "orphan", StartTime: ts(5)},
	}

	roots := BuildTree(spans)

	// 两个根：真正的 root 和父不存在的 orphanRoot。
	if len(roots) != 2 {
		t.Fatalf("期望 2 个根，得到 %d", len(roots))
	}
	// 根按开始时间升序：orphan(5) 在 root(10) 前。
	if roots[0].Span.SpanID != "orphanRoot" || roots[1].Span.SpanID != "root" {
		t.Fatalf("根排序错误: %s, %s", roots[0].Span.SpanID, roots[1].Span.SpanID)
	}

	rootNode := roots[1]
	if len(rootNode.Children) != 2 {
		t.Fatalf("root 期望 2 个子节点，得到 %d", len(rootNode.Children))
	}
	// 子节点按开始时间升序：c1(20) 先于 c2(30)。
	if rootNode.Children[0].Span.SpanID != "c1" || rootNode.Children[1].Span.SpanID != "c2" {
		t.Fatalf("子节点排序错误")
	}
	// 孙节点挂在 c1 下。
	if len(rootNode.Children[0].Children) != 1 || rootNode.Children[0].Children[0].Span.SpanID != "gc" {
		t.Fatalf("孙节点挂接错误")
	}
}

// TestBuildTree_SelfParentGuard 自引用 parent 不应造成环/丢失，应作为根。
func TestBuildTree_SelfParentGuard(t *testing.T) {
	spans := []*Span{
		{SpanID: "x", ParentSpanID: "x", Name: "self", StartTime: ts(1)},
	}
	roots := BuildTree(spans)
	if len(roots) != 1 || roots[0].Span.SpanID != "x" {
		t.Fatalf("自引用 span 应作为根，得到 %+v", roots)
	}
	if len(roots[0].Children) != 0 {
		t.Fatalf("自引用不应挂成自身子节点")
	}
}

// TestBuildTree_Empty 空输入返回空森林。
func TestBuildTree_Empty(t *testing.T) {
	if roots := BuildTree(nil); len(roots) != 0 {
		t.Fatalf("空输入应返回空森林")
	}
}
