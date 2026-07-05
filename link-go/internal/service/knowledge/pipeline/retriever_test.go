// Package rag: unit tests for the pure-Go retrieval helpers (keyword scoring,
// document sorting, RRF fusion). These require no database or external service.
package rag

import (
	"testing"

	domainrag "link/internal/model/rag"
)

func TestKeywordScore(t *testing.T) {
	cases := []struct {
		name    string
		content string
		query   string
		want    float32
	}{
		{"empty query", "hello world", "", 0},
		{"empty content", "", "hello", 0},
		{"single term hit", "the quick brown fox", "fox", 1},
		{"single term miss", "the quick brown fox", "cat", 0},
		{"all terms hit", "the quick brown fox", "quick fox", 1},
		{"half terms hit", "the quick brown fox", "quick cat", 0.5},
		{"case insensitive", "The Quick Brown Fox", "FOX", 1},
		{"chinese substring hit", "知识图谱与向量检索", "向量检索", 1},
		{"chinese substring miss", "知识图谱与向量检索", "关系数据库", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := keywordScore(c.content, c.query)
			if got != c.want {
				t.Errorf("keywordScore(%q,%q)=%v, want %v", c.content, c.query, got, c.want)
			}
		})
	}
}

func TestContainsKeyword(t *testing.T) {
	// Regression guard for the former stub that returned true for any non-empty input.
	if containsKeyword("the quick brown fox", "cat") {
		t.Error("expected no match for unrelated query")
	}
	if !containsKeyword("the quick brown fox", "fox") {
		t.Error("expected match for present term")
	}
	if containsKeyword("anything", "") {
		t.Error("expected no match for empty query")
	}
}

func TestSortDocsByScoreDesc(t *testing.T) {
	docs := []*domainrag.Document{
		{ChunkID: "a", Score: 0.2},
		{ChunkID: "b", Score: 0.9},
		{ChunkID: "c", Score: 0.5},
	}
	sortDocsByScoreDesc(docs)
	if docs[0].ChunkID != "b" || docs[1].ChunkID != "c" || docs[2].ChunkID != "a" {
		t.Errorf("unexpected order: %s,%s,%s", docs[0].ChunkID, docs[1].ChunkID, docs[2].ChunkID)
	}
}

func TestRRFusionOrdersByFusedScore(t *testing.T) {
	r := &RetrieverImpl{}
	// docA ranks high in both lists; docC only in BM25; docB only in vector.
	vector := []*domainrag.Document{
		{ChunkID: "A", Content: "a"},
		{ChunkID: "B", Content: "b"},
	}
	bm25 := []*domainrag.Document{
		{ChunkID: "A", Content: "a"},
		{ChunkID: "C", Content: "c"},
	}
	out := r.rRFusion(vector, bm25, 10, 0.5)
	if len(out) != 3 {
		t.Fatalf("expected 3 fused docs, got %d", len(out))
	}
	// A appears in both lists at rank 0 -> strictly highest fused score.
	if out[0].ChunkID != "A" {
		t.Errorf("expected A first (present in both lists), got %s", out[0].ChunkID)
	}
	// Output must be sorted descending by score.
	for i := 1; i < len(out); i++ {
		if out[i-1].Score < out[i].Score {
			t.Errorf("not sorted desc at %d: %v < %v", i, out[i-1].Score, out[i].Score)
		}
	}
}

func TestRRFusionRespectsTopK(t *testing.T) {
	r := &RetrieverImpl{}
	vector := []*domainrag.Document{{ChunkID: "A"}, {ChunkID: "B"}, {ChunkID: "C"}}
	out := r.rRFusion(vector, nil, 2, 0.5)
	if len(out) != 2 {
		t.Errorf("expected topK=2 results, got %d", len(out))
	}
}

// TestRRFusionScoresIncompatibleWithCosineThreshold locks in the root cause of the
// "hybrid retrieval returns empty" bug: RRF fused scores (~1/(60+rank)) live on a
// completely different scale than cosine similarity. Applying a cosine threshold
// (0.5~0.7) to fused scores drops every hit. HybridRetrieve therefore must NOT run
// filterByThreshold on fused results — it filters the vector component instead.
func TestRRFusionScoresIncompatibleWithCosineThreshold(t *testing.T) {
	r := &RetrieverImpl{}
	vector := []*domainrag.Document{{ChunkID: "A"}, {ChunkID: "B"}}
	bm25 := []*domainrag.Document{{ChunkID: "A"}, {ChunkID: "C"}}
	fused := r.rRFusion(vector, bm25, 10, 0.5)
	if len(fused) == 0 {
		t.Fatal("fusion produced no docs")
	}
	// Top fused score must be far below any sane cosine threshold.
	if fused[0].Score >= 0.5 {
		t.Fatalf("unexpected: top RRF score %v is on cosine scale; assumptions changed", fused[0].Score)
	}
	// Proof that the old post-fusion filter wiped everything: a 0.5 cosine threshold
	// removes 100% of fused results.
	if got := r.filterByThreshold(fused, 0.5); len(got) != 0 {
		t.Fatalf("expected cosine threshold to drop all RRF-scored docs, kept %d", len(got))
	}
	// The correct behavior (no cosine filter on fused output) keeps them.
	if len(fused) != 3 {
		t.Fatalf("expected 3 fused docs retained without cosine filtering, got %d", len(fused))
	}
}
