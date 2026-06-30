// Package mysql: unit tests for the pure-Go portions of the MySQL graph store
// (JSON helpers, set operations, and in-memory graph algorithms). These tests
// require no database. End-to-end persistence is covered by the integration
// test (graph_store_integration_test.go, build tag `integration`).
package mysql

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	domain_knowledge "link/internal/model/knowledge"
)

func TestMarshalUnmarshalSlice(t *testing.T) {
	in := []string{"a", "b", "c"}
	got := unmarshalSlice(marshalSlice(in))
	if !reflect.DeepEqual(got, in) {
		t.Errorf("round-trip mismatch: got %v, want %v", got, in)
	}
	if marshalSlice(nil) != "[]" {
		t.Errorf("expected [] for nil slice, got %q", marshalSlice(nil))
	}
	if unmarshalSlice("") != nil {
		t.Errorf("expected nil for empty string")
	}
	if unmarshalSlice("not-json") != nil {
		t.Errorf("expected nil for invalid json")
	}
}

func TestMarshalUnmarshalStringMap(t *testing.T) {
	in := map[string]string{"k1": "v1", "k2": "v2"}
	got := unmarshalStringMap(marshalStringMap(in))
	if !reflect.DeepEqual(got, in) {
		t.Errorf("round-trip mismatch: got %v, want %v", got, in)
	}
	if marshalStringMap(nil) != "{}" {
		t.Errorf("expected {} for nil map")
	}
	if unmarshalStringMap("") != nil {
		t.Errorf("expected nil for empty string")
	}
}

func TestUnionStrings(t *testing.T) {
	got := unionStrings([]string{"a", "b", ""}, []string{"b", "c", ""})
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	// Order preserved: a's elements first, then new ones from b.
	if got[0] != "a" || got[2] != "c" {
		t.Errorf("order not preserved: %v", got)
	}
}

func TestNodeModelDomainRoundTrip(t *testing.T) {
	ns := domain_knowledge.NameSpace{TenantID: "t1", KnowledgeBaseID: "kb1"}
	n := &domain_knowledge.GraphNode{
		ID:         "n1",
		Name:       "Alice",
		EntityType: "Person",
		Attributes: []string{"engineer"},
		Chunks:     []string{"c1", "c2"},
		Properties: map[string]string{"role": "lead"},
	}
	m := nodeModelFromDomain(ns, n)
	if m.TenantID != "t1" || m.KBID != "kb1" {
		t.Errorf("namespace not applied: %+v", m)
	}
	got := m.toDomain()
	if got.ID != n.ID || got.Name != n.Name || got.EntityType != n.EntityType {
		t.Errorf("scalar mismatch: %+v", got)
	}
	if !reflect.DeepEqual(got.Chunks, n.Chunks) || !reflect.DeepEqual(got.Attributes, n.Attributes) {
		t.Errorf("slice mismatch: %+v", got)
	}
	if !reflect.DeepEqual(got.Properties, n.Properties) {
		t.Errorf("properties mismatch: %+v", got)
	}
}

func TestRelationModelDomainRoundTrip(t *testing.T) {
	ns := domain_knowledge.NameSpace{TenantID: "t1", KnowledgeBaseID: "kb1"}
	r := &domain_knowledge.GraphRelation{
		ID:             "r1",
		Source:         "Alice",
		Target:         "Bob",
		Type:           "KNOWS",
		Strength:       0.7,
		Weight:         1.2,
		ChunkIDs:       []string{"c1"},
		Properties:     map[string]string{"since": "2020"},
		CombinedDegree: 5,
		PMI:            0.3,
		Description:    "colleagues",
	}
	got := relationModelFromDomain(ns, r).toDomain()
	if got.ID != r.ID || got.Source != r.Source || got.Target != r.Target || got.Type != r.Type {
		t.Errorf("scalar mismatch: %+v", got)
	}
	if got.Strength != r.Strength || got.Weight != r.Weight || got.PMI != r.PMI || got.CombinedDegree != r.CombinedDegree {
		t.Errorf("numeric mismatch: %+v", got)
	}
	if got.Description != r.Description {
		t.Errorf("description mismatch: %q", got.Description)
	}
}

// buildTestAdjacency constructs an in-memory undirected adjacency for graph
// algorithm tests without touching a database.
//   A - B - C - D    (path)   plus  B - D  (shortcut) and isolated E
func buildTestAdjacency() *adjacency {
	nodes := []string{"A", "B", "C", "D", "E"}
	adj := &adjacency{
		nodesByName: make(map[string]*domain_knowledge.GraphNode),
		edges:       make(map[string][]adjEdge),
	}
	for _, n := range nodes {
		adj.nodesByName[n] = &domain_knowledge.GraphNode{Name: n}
	}
	addEdge := func(id, s, tgt string, w float64) {
		rel := &domain_knowledge.GraphRelation{ID: id, Source: s, Target: tgt, Weight: w}
		adj.edges[s] = append(adj.edges[s], adjEdge{to: tgt, rel: rel})
		adj.edges[tgt] = append(adj.edges[tgt], adjEdge{to: s, rel: rel})
	}
	addEdge("e1", "A", "B", 1)
	addEdge("e2", "B", "C", 1)
	addEdge("e3", "C", "D", 1)
	addEdge("e4", "B", "D", 1)
	return adj
}

func TestBFSShortestPath(t *testing.T) {
	adj := buildTestAdjacency()

	names, edges, ok := bfsShortestPath(adj, "A", "D", 0)
	if !ok {
		t.Fatal("expected path A->D")
	}
	// Shortest is A-B-D (length 2) thanks to the B-D shortcut.
	if len(edges) != 2 {
		t.Errorf("expected path length 2, got %d (%v)", len(edges), names)
	}
	if names[0] != "A" || names[len(names)-1] != "D" {
		t.Errorf("path endpoints wrong: %v", names)
	}

	// Same node.
	if _, _, ok := bfsShortestPath(adj, "A", "A", 0); !ok {
		t.Error("expected trivial path for same node")
	}
	// Unreachable.
	if _, _, ok := bfsShortestPath(adj, "A", "E", 0); ok {
		t.Error("expected no path to isolated node E")
	}
	// Depth limit cuts off reachable node.
	if _, _, ok := bfsShortestPath(adj, "A", "C", 1); ok {
		t.Error("expected depth limit to block A->C at maxDepth 1")
	}
}

func TestEnumerateSimplePaths(t *testing.T) {
	adj := buildTestAdjacency()
	paths := enumerateSimplePaths(adj, "A", "D", 6)
	if len(paths) < 2 {
		t.Fatalf("expected at least 2 simple paths A->D, got %d", len(paths))
	}
	// All paths must start at A and end at D, and be simple (no repeated nodes).
	for _, p := range paths {
		if p.Nodes[0].Name != "A" || p.Nodes[len(p.Nodes)-1].Name != "D" {
			t.Errorf("path endpoints wrong: %v", p.Nodes)
		}
		seen := map[string]bool{}
		for _, n := range p.Nodes {
			if seen[n.Name] {
				t.Errorf("path not simple, repeated node %s", n.Name)
			}
			seen[n.Name] = true
		}
	}
}

func TestSortPaths(t *testing.T) {
	paths := []*domain_knowledge.PathQueryResult{
		{Length: 3, Weight: 1},
		{Length: 1, Weight: 1},
		{Length: 2, Weight: 5},
		{Length: 2, Weight: 9},
	}
	sortPaths(paths)
	// length asc; within equal length, weight desc.
	if paths[0].Length != 1 {
		t.Errorf("expected shortest first, got %d", paths[0].Length)
	}
	if paths[1].Length != 2 || paths[1].Weight != 9 {
		t.Errorf("expected higher-weight length-2 path second, got len=%d w=%f", paths[1].Length, paths[1].Weight)
	}
}

func TestDegreeMap(t *testing.T) {
	nodes := []graphNodeModel{{Name: "A"}, {Name: "B"}, {Name: "C"}}
	rels := []graphRelationModel{
		{Source: "A", Target: "B"},
		{Source: "B", Target: "C"},
	}
	deg := degreeMap(nodes, rels)
	if deg["A"] != 1 || deg["B"] != 2 || deg["C"] != 1 {
		t.Errorf("unexpected degrees: %v", deg)
	}
}

func TestConnectedComponents(t *testing.T) {
	// Two components: {A,B,C} and {D}; E isolated.
	nodes := []graphNodeModel{{Name: "A"}, {Name: "B"}, {Name: "C"}, {Name: "D"}, {Name: "E"}}
	neighborSet := map[string]map[string]struct{}{
		"A": {"B": {}}, "B": {"A": {}, "C": {}}, "C": {"B": {}},
		"D": {}, "E": {},
	}
	count, largest := connectedComponents(nodes, neighborSet)
	if count != 3 {
		t.Errorf("expected 3 components, got %d", count)
	}
	if largest != 3 {
		t.Errorf("expected largest component size 3, got %d", largest)
	}
}

func TestAverageClusteringCoefficient(t *testing.T) {
	// Triangle A-B-C fully connected -> each node clustering = 1.
	triangle := map[string]map[string]struct{}{
		"A": {"B": {}, "C": {}},
		"B": {"A": {}, "C": {}},
		"C": {"A": {}, "B": {}},
	}
	if got := averageClusteringCoefficient(triangle); got < 0.999 || got > 1.001 {
		t.Errorf("expected clustering 1.0 for triangle, got %f", got)
	}
	// Path A-B-C: B has 2 neighbors with no link between them -> 0.
	path := map[string]map[string]struct{}{
		"A": {"B": {}},
		"B": {"A": {}, "C": {}},
		"C": {"B": {}},
	}
	if got := averageClusteringCoefficient(path); got != 0 {
		t.Errorf("expected clustering 0 for path, got %f", got)
	}
	if got := averageClusteringCoefficient(map[string]map[string]struct{}{}); got != 0 {
		t.Errorf("expected 0 for empty graph, got %f", got)
	}
}

func TestTopTypeCounts(t *testing.T) {
	dist := map[string]int{"Person": 5, "Org": 3, "Place": 8, "Event": 1}
	top := topTypeCounts(dist, 2)
	if len(top) != 2 {
		t.Fatalf("expected 2, got %d", len(top))
	}
	if top[0].Type != "Place" || top[0].Count != 8 {
		t.Errorf("expected Place(8) first, got %s(%d)", top[0].Type, top[0].Count)
	}
	if top[1].Type != "Person" || top[1].Count != 5 {
		t.Errorf("expected Person(5) second, got %s(%d)", top[1].Type, top[1].Count)
	}

	// Stable tie-breaking by type name when counts equal.
	tie := map[string]int{"b": 2, "a": 2, "c": 2}
	all := topTypeCounts(tie, 10)
	names := []string{all[0].Type, all[1].Type, all[2].Type}
	if !sort.StringsAreSorted(names) {
		t.Errorf("expected alphabetical tie-break, got %v", names)
	}
}

func TestEscapeLike(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "chunk1", "chunk1"},
		{"percent", "a%b", "a\\%b"},
		{"underscore", "a_b", "a\\_b"},
		{"backslash", "a\\b", "a\\\\b"},
		{"mixed", "a%_\\b", "a\\%\\_\\\\b"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := escapeLike(c.in); got != c.want {
				t.Errorf("escapeLike(%q)=%q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestJSONElemPattern(t *testing.T) {
	// The value must be wrapped in quotes so "c1" cannot match "c12".
	if got, want := jsonElemPattern("c1"), `%"c1"%`; got != want {
		t.Errorf("jsonElemPattern(c1)=%q, want %q", got, want)
	}
	// Wildcards inside the id are escaped so they match literally, not broaden.
	if got, want := jsonElemPattern("a%b"), `%"a\%b"%`; got != want {
		t.Errorf("jsonElemPattern=%q, want %q", got, want)
	}
}

// TestJSONElemPatternAgainstMarshaledSlice cross-checks that the LIKE pattern's
// literal core actually appears in the JSON produced by marshalSlice for the
// matching element, and crucially that a prefix-sharing sibling ("c12") does NOT
// contain it — this is what makes the LIKE pre-filter in DeleteByChunkID safe
// against false negatives while the closing quote prevents over-broad matches.
func TestJSONElemPatternAgainstMarshaledSlice(t *testing.T) {
	core := `"c1"` // the unescaped literal core of jsonElemPattern("c1")
	withTarget := marshalSlice([]string{"c12", "c1", "c3"})
	if !strings.Contains(withTarget, core) {
		t.Errorf("expected %q to contain element marker %q", withTarget, core)
	}
	onlySibling := marshalSlice([]string{"c12"})
	if strings.Contains(onlySibling, core) {
		t.Errorf("element marker %q must not match prefix-sharing %q", core, onlySibling)
	}
}

func TestPathToResult(t *testing.T) {
	adj := buildTestAdjacency()
	rel := &domain_knowledge.GraphRelation{ID: "e1", Source: "A", Target: "B", Weight: 2.5}
	res := adj.pathToResult([]string{"A", "B"}, []*domain_knowledge.GraphRelation{rel})
	if res.Length != 1 {
		t.Errorf("expected length 1, got %d", res.Length)
	}
	if res.Weight != 2.5 {
		t.Errorf("expected weight 2.5, got %f", res.Weight)
	}
	if len(res.Nodes) != 2 || res.Nodes[0].Name != "A" {
		t.Errorf("unexpected nodes: %v", res.Nodes)
	}
}
