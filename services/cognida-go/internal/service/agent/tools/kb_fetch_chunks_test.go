package tools

import (
	"context"
	"encoding/json"
	"testing"
)

// stubChunkFetcher 记录收到的（归一化后的）请求，回放固定结果。
type stubChunkFetcher struct {
	got  *FetchChunksRequest
	resp *FetchChunksResult
}

func (s *stubChunkFetcher) FetchChunks(_ interface{}, req *FetchChunksRequest) (*FetchChunksResult, error) {
	s.got = req
	if s.resp != nil {
		return s.resp, nil
	}
	return &FetchChunksResult{Chunks: []FetchedChunk{}, Count: 0}, nil
}

func runFetchTool(t *testing.T, fetcher ChunkFetcher, req *FetchChunksRequest) (*FetchChunksResult, error) {
	t.Helper()
	tool := NewKbFetchChunksTool(fetcher)
	if tool == nil {
		t.Fatal("tool must not be nil")
	}
	args, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal req: %v", err)
	}
	out, err := tool.InvokableRun(context.Background(), string(args))
	if err != nil {
		return nil, err
	}
	var res FetchChunksResult
	if uerr := json.Unmarshal([]byte(out), &res); uerr != nil {
		t.Fatalf("unmarshal response: %v (raw=%s)", uerr, out)
	}
	return &res, nil
}

// TestKbFetchChunks_RejectsEmptyFilter 验证：三种过滤维度全空时拒绝执行（避免无边界拉取）。
func TestKbFetchChunks_RejectsEmptyFilter(t *testing.T) {
	s := &stubChunkFetcher{}
	_, err := runFetchTool(t, s, &FetchChunksRequest{})
	if err == nil {
		t.Fatal("expected rejection when no filter dimension provided")
	}
	if s.got != nil {
		t.Fatal("fetcher must not be called on empty filter")
	}
}

// TestKbFetchChunks_NilFetcherDegrades 验证：未接线时返回降级提示而非 panic。
func TestKbFetchChunks_NilFetcherDegrades(t *testing.T) {
	_, err := runFetchTool(t, nil, &FetchChunksRequest{ChunkIDs: []string{"c1"}})
	if err == nil {
		t.Fatal("expected degraded error when fetcher is nil")
	}
}

// TestKbFetchChunks_NormalizesLimitOffset 验证：limit 默认/上限、offset 下限在委派前归一化。
func TestKbFetchChunks_NormalizesLimitOffset(t *testing.T) {
	cases := []struct {
		name      string
		inLimit   int
		inOffset  int
		wantLimit int
		wantOff   int
	}{
		{"default", 0, 0, 20, 0},
		{"cap", 999, -5, 200, 0},
		{"passthrough", 33, 7, 33, 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &stubChunkFetcher{}
			_, err := runFetchTool(t, s, &FetchChunksRequest{
				ChunkIDs: []string{"c1"}, Limit: tc.inLimit, Offset: tc.inOffset,
			})
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if s.got == nil {
				t.Fatal("fetcher should have been called")
			}
			if s.got.Limit != tc.wantLimit || s.got.Offset != tc.wantOff {
				t.Fatalf("limit/offset not normalized: got limit=%d offset=%d, want %d/%d",
					s.got.Limit, s.got.Offset, tc.wantLimit, tc.wantOff)
			}
		})
	}
}

// TestKbFetchChunks_RejectsDeepPagination 验证：offset+limit 超过 Milvus 窗口硬上限时
// fail-closed，且不委派底层（宁拒不闯，避免落到驱动才报错）。
func TestKbFetchChunks_RejectsDeepPagination(t *testing.T) {
	s := &stubChunkFetcher{}
	// limit 归一化后为 20，offset 16370 → 16390 > 16384。
	_, err := runFetchTool(t, s, &FetchChunksRequest{
		ChunkIDs: []string{"c1"}, Offset: 16370, Limit: 20,
	})
	if err == nil {
		t.Fatal("expected rejection when offset+limit exceeds milvus window cap")
	}
	if s.got != nil {
		t.Fatal("fetcher must not be called on deep pagination")
	}
}

// TestKbFetchChunks_DeepPaginationBoundaryOK 验证：恰好等于上限时放行（边界不误伤）。
func TestKbFetchChunks_DeepPaginationBoundaryOK(t *testing.T) {
	s := &stubChunkFetcher{}
	// offset 16364 + limit 20 = 16384 == 上限，应放行。
	_, err := runFetchTool(t, s, &FetchChunksRequest{
		ChunkIDs: []string{"c1"}, Offset: 16364, Limit: 20,
	})
	if err != nil {
		t.Fatalf("boundary (== cap) should pass, got err: %v", err)
	}
	if s.got == nil {
		t.Fatal("fetcher should have been called at boundary")
	}
}

// TestKbFetchChunks_DelegatesResult 验证：底层结果原样透传。
func TestKbFetchChunks_DelegatesResult(t *testing.T) {
	s := &stubChunkFetcher{resp: &FetchChunksResult{
		Chunks: []FetchedChunk{{ChunkID: "c1", Content: "x"}},
		Count:  1,
	}}
	res, err := runFetchTool(t, s, &FetchChunksRequest{KnowledgeIDs: []string{"k1"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Count != 1 || len(res.Chunks) != 1 || res.Chunks[0].ChunkID != "c1" {
		t.Fatalf("result not passed through: %+v", res)
	}
}
