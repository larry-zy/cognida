package knowledge

import "testing"

func node(name string) *GraphNode { return &GraphNode{Name: name} }

func rel(src, tgt string, strength, weight float64) *GraphRelation {
	return &GraphRelation{Source: src, Target: tgt, Strength: strength, Weight: weight}
}

func TestCalculateDetailedStats_ComponentsAndDegrees(t *testing.T) {
	svc := NewGraphService()

	// 分量1: A-B-C 链；分量2: D-E；孤立: F
	graph := &GraphData{
		Node: []*GraphNode{node("A"), node("B"), node("C"), node("D"), node("E"), node("F")},
		Relation: []*GraphRelation{
			rel("A", "B", 2, 4),
			rel("B", "C", 4, 8),
			rel("D", "E", 6, 6),
		},
	}

	stats := svc.CalculateDetailedStats(graph)

	if stats.NodeCount != 6 {
		t.Errorf("NodeCount = %d, want 6", stats.NodeCount)
	}
	if stats.RelationCount != 3 {
		t.Errorf("RelationCount = %d, want 3", stats.RelationCount)
	}
	if stats.ComponentCount != 3 { // {A,B,C}, {D,E}, {F}
		t.Errorf("ComponentCount = %d, want 3", stats.ComponentCount)
	}
	if stats.IsolatedNodes != 1 { // F
		t.Errorf("IsolatedNodes = %d, want 1", stats.IsolatedNodes)
	}
	// B 度数为2（A-B, B-C），为最大度
	if stats.MaxDegree != 2 {
		t.Errorf("MaxDegree = %d, want 2", stats.MaxDegree)
	}
	// 平均权重 (4+8+6)/3 = 6
	if stats.AvgWeight < 5.99 || stats.AvgWeight > 6.01 {
		t.Errorf("AvgWeight = %f, want ~6", stats.AvgWeight)
	}
}

func TestCalculateDetailedStats_Empty(t *testing.T) {
	svc := NewGraphService()
	stats := svc.CalculateDetailedStats(&GraphData{})
	if stats.NodeCount != 0 || stats.ComponentCount != 0 || stats.IsolatedNodes != 0 {
		t.Errorf("empty graph stats not zeroed: %+v", stats)
	}
}

func TestCalculateDetailedStats_SingleComponent(t *testing.T) {
	svc := NewGraphService()
	graph := &GraphData{
		Node:     []*GraphNode{node("A"), node("B"), node("C")},
		Relation: []*GraphRelation{rel("A", "B", 1, 1), rel("B", "C", 1, 1), rel("C", "A", 1, 1)},
	}
	stats := svc.CalculateDetailedStats(graph)
	if stats.ComponentCount != 1 {
		t.Errorf("ComponentCount = %d, want 1", stats.ComponentCount)
	}
	if stats.IsolatedNodes != 0 {
		t.Errorf("IsolatedNodes = %d, want 0", stats.IsolatedNodes)
	}
}
