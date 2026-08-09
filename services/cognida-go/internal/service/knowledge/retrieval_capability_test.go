// Package knowledge: RetrievalCapability（检索能力封装，单一事实源）的白盒单元测试。
// 覆盖收敛后独有的治理逻辑：模式分发、跨库去重保留高分、混合相对质量下限、
// 可插拔重排（默认关/开）、单片截断、出处标签、TopK 兜底、HasAnswer、未接线哨兵错误。
package knowledge

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"cognida/internal/model/rag"
)

// capFakeRetriever 按检索模式返回预置文档，并记录最近一次收到的 opts。
// 未预置的模式返回 nil（模拟单库无命中）。逐库检索现为并发扇出，故内部状态用 mu 保护，
// 避免多库测试下 lastOpts/calls 被并发写触发 -race。
type capFakeRetriever struct {
	vector []*rag.Document
	bm25   []*rag.Document
	hybrid []*rag.Document
	// perKB 若非空，则按 kbID 返回不同结果（用于多库合并/去重测试），优先于 hybrid。
	perKB map[string][]*rag.Document

	mu       sync.Mutex
	lastOpts *rag.RetrieveOptions
	calls    int // 累计被调用次数（= 实际扇出的库数）
}

func (r *capFakeRetriever) respond(docs []*rag.Document, opts *rag.RetrieveOptions) (*rag.RetrieveResponse, error) {
	r.mu.Lock()
	r.lastOpts = opts
	r.calls++
	r.mu.Unlock()
	return &rag.RetrieveResponse{Results: docs}, nil
}

func (r *capFakeRetriever) Retrieve(_ context.Context, _, _, _ string, opts *rag.RetrieveOptions) (*rag.RetrieveResponse, error) {
	return r.respond(r.hybrid, opts)
}
func (r *capFakeRetriever) RetrieveWithEmbedding(_ context.Context, _, _, _ string, _ []float32, opts *rag.RetrieveOptions) (*rag.RetrieveResponse, error) {
	return r.respond(r.hybrid, opts)
}
func (r *capFakeRetriever) VectorRetrieve(_ context.Context, _, _, _ string, opts *rag.RetrieveOptions) (*rag.RetrieveResponse, error) {
	return r.respond(r.vector, opts)
}
func (r *capFakeRetriever) BM25Retrieve(_ context.Context, _, _, _ string, opts *rag.RetrieveOptions) (*rag.RetrieveResponse, error) {
	return r.respond(r.bm25, opts)
}
func (r *capFakeRetriever) HybridRetrieve(_ context.Context, _, kbID, _ string, opts *rag.RetrieveOptions) (*rag.RetrieveResponse, error) {
	if r.perKB != nil {
		return r.respond(r.perKB[kbID], opts)
	}
	return r.respond(r.hybrid, opts)
}
func (r *capFakeRetriever) GraphRetrieve(_ context.Context, _, _, _ string, opts *rag.RetrieveOptions) (*rag.RetrieveResponse, error) {
	return r.respond(nil, opts)
}

// reverseReranker 把结果倒序，作为「可插拔重排改变顺序」的可观测替身。
type reverseReranker struct{ called bool }

func (r *reverseReranker) Rerank(_ context.Context, results []*rag.Document, _ string) ([]*rag.Document, error) {
	r.called = true
	out := make([]*rag.Document, len(results))
	for i := range results {
		out[i] = results[len(results)-1-i]
	}
	return out, nil
}
func (r *reverseReranker) SetStrategy(string) error { return nil }
func (r *reverseReranker) GetStrategy() string      { return "reverse" }

func doc(chunkID, content string, score float32) *rag.Document {
	return &rag.Document{ChunkID: chunkID, Content: content, Score: score}
}

func capBaseQuery() GovernedQuery {
	return GovernedQuery{Query: "问题", Mode: "hybrid", TopK: 20}
}

// 未注入底层检索器 → 返回哨兵错误，不静默成空结果。
func TestCapability_NotWired_ReturnsSentinel(t *testing.T) {
	c := NewRetrievalCapability(nil, nil)
	_, err := c.Retrieve(context.Background(), 1, []string{"kb1"}, capBaseQuery())
	if !errors.Is(err, errRetrieverNotWired) {
		t.Fatalf("期望 errRetrieverNotWired, got %v", err)
	}
}

// 空 kbIDs 或空查询 → 空结果 + HasAnswer=false，不报错。
func TestCapability_EmptyScopeOrQuery_NoAnswer(t *testing.T) {
	c := NewRetrievalCapability(&capFakeRetriever{}, nil)

	res, err := c.Retrieve(context.Background(), 1, nil, capBaseQuery())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.HasAnswer || len(res.Chunks) != 0 {
		t.Fatalf("空范围应无答案, got %+v", res)
	}

	q := capBaseQuery()
	q.Query = "   "
	res, err = c.Retrieve(context.Background(), 1, []string{"kb1"}, q)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.HasAnswer || len(res.Chunks) != 0 {
		t.Fatalf("空查询应无答案, got %+v", res)
	}
}

// 底层逐库检索的 opts.RerankEnabled 恒被强制关闭（重排集中在跨库合并后）。
func TestCapability_ForcesPipelineRerankOff(t *testing.T) {
	ret := &capFakeRetriever{hybrid: []*rag.Document{doc("a", "x", 0.9)}}
	c := NewRetrievalCapability(ret, &reverseReranker{})

	q := capBaseQuery()
	q.EnableRerank = true // 即便会话开重排，底层逐库检索也不得开
	if _, err := c.Retrieve(context.Background(), 1, []string{"kb1"}, q); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ret.lastOpts == nil {
		t.Fatal("retriever 未被调用")
	}
	if ret.lastOpts.RerankEnabled {
		t.Fatal("底层 opts.RerankEnabled 应恒为 false")
	}
}

// 模式分发：vector/bm25 走对应方法；未知模式回落 hybrid。
func TestCapability_ModeDispatch(t *testing.T) {
	ret := &capFakeRetriever{
		vector: []*rag.Document{doc("v", "vec", 0.8)},
		bm25:   []*rag.Document{doc("b", "bm", 0.7)},
		hybrid: []*rag.Document{doc("h", "hyb", 0.6)},
	}
	c := NewRetrievalCapability(ret, nil)

	cases := map[string]string{"vector": "vec", "bm25": "bm", "hybrid": "hyb", "unknown-mode": "hyb"}
	for mode, wantContent := range cases {
		q := capBaseQuery()
		q.Mode = mode
		res, err := c.Retrieve(context.Background(), 1, []string{"kb1"}, q)
		if err != nil {
			t.Fatalf("mode=%s err: %v", mode, err)
		}
		if len(res.Chunks) != 1 || res.Chunks[0].Content != wantContent {
			t.Fatalf("mode=%s 期望命中 %q, got %+v", mode, wantContent, res.Chunks)
		}
		if ret.lastOpts.RetrievalMode != mode {
			t.Fatalf("mode=%s 未透传给 opts.RetrievalMode, got %q", mode, ret.lastOpts.RetrievalMode)
		}
	}
}

// 跨库合并去重：同 ChunkID 命中多库时保留更高分。
func TestCapability_DedupByChunkIDKeepsHigher(t *testing.T) {
	ret := &capFakeRetriever{perKB: map[string][]*rag.Document{
		"kb1": {doc("dup", "低分版本", 0.30)},
		"kb2": {doc("dup", "高分版本", 0.80), doc("uniq", "独占", 0.50)},
	}}
	c := NewRetrievalCapability(ret, nil)

	res, err := c.Retrieve(context.Background(), 1, []string{"kb1", "kb2"}, capBaseQuery())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res.Chunks) != 2 {
		t.Fatalf("去重后应剩 2 片, got %d: %+v", len(res.Chunks), res.Chunks)
	}
	// 排序降序 → 第一片应是保留下来的高分版本（float32→float64 有精度差，比内容/ID 即可）。
	if res.Chunks[0].ChunkID != "dup" || res.Chunks[0].Content != "高分版本" {
		t.Fatalf("去重应保留高分版本, got %+v", res.Chunks[0])
	}
}

// 多库并发扇出：库数超过并发上限时全部被检索，跨库合并结果确定（与调度顺序无关）。
func TestCapability_ConcurrentMultiKBGather(t *testing.T) {
	// 造 capMaxConcurrentKB+4 个库，每库一片、分数各异（降序可断言最终顺序确定）。
	kbCount := capMaxConcurrentKB + 4
	perKB := make(map[string][]*rag.Document, kbCount)
	kbIDs := make([]string, 0, kbCount)
	for i := 0; i < kbCount; i++ {
		id := "kb" + string(rune('a'+i))
		kbIDs = append(kbIDs, id)
		// 分数随 i 递减但收在 [0.79,0.90] 紧带内（均 > 0.3×max，不触混合下限），
		// 只考察扇出与确定性排序：kba 最高、kbb 次之……最终降序可预期。
		perKB[id] = []*rag.Document{doc("chunk-"+id, id, 0.90-float32(i)*0.01)}
	}
	ret := &capFakeRetriever{perKB: perKB}
	c := NewRetrievalCapability(ret, nil)

	res, err := c.Retrieve(context.Background(), 1, kbIDs, capBaseQuery())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ret.calls != kbCount {
		t.Fatalf("应对全部 %d 个库扇出检索, 实际调用 %d 次", kbCount, ret.calls)
	}
	if len(res.Chunks) != kbCount {
		t.Fatalf("应汇集全部 %d 库各 1 片, got %d", kbCount, len(res.Chunks))
	}
	// 与调度顺序无关的确定性：按分数降序即 kba, kbb, kbc……
	for i := 0; i < kbCount; i++ {
		wantID := "chunk-kb" + string(rune('a'+i))
		if res.Chunks[i].ChunkID != wantID {
			t.Fatalf("第 %d 片应确定为 %q, got %q", i, wantID, res.Chunks[i].ChunkID)
		}
	}
}

// 混合相对质量下限：丢弃 < 0.3×maxScore 的弱命中；vector/bm25 模式不套此下限。
func TestCapability_HybridRelativeFloor(t *testing.T) {
	// maxScore=1.0 → floor=0.3；0.2 应被丢弃，0.35 保留。
	docs := []*rag.Document{doc("top", "强", 1.0), doc("mid", "中", 0.35), doc("weak", "弱", 0.2)}

	// hybrid：弱命中被剔除。
	cHybrid := NewRetrievalCapability(&capFakeRetriever{hybrid: docs}, nil)
	res, err := cHybrid.Retrieve(context.Background(), 1, []string{"kb1"}, capBaseQuery())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res.Chunks) != 2 {
		t.Fatalf("hybrid 下限应剔除弱命中剩 2 片, got %d: %+v", len(res.Chunks), res.Chunks)
	}
	for _, ch := range res.Chunks {
		if ch.ChunkID == "weak" {
			t.Fatalf("弱命中 weak 应被相对下限剔除, got %+v", res.Chunks)
		}
	}

	// vector：同样的分布不套 hybrid 下限，弱命中保留（底层已按余弦阈值过滤）。
	cVec := NewRetrievalCapability(&capFakeRetriever{vector: docs}, nil)
	q := capBaseQuery()
	q.Mode = "vector"
	res, err = cVec.Retrieve(context.Background(), 1, []string{"kb1"}, q)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res.Chunks) != 3 {
		t.Fatalf("vector 模式不套 hybrid 下限, 应保留 3 片, got %d", len(res.Chunks))
	}
}

// 混合下限守卫：分数量纲异常（全 0）时不过滤，避免误删全部命中。
func TestCapability_HybridFloorGuardsZeroScores(t *testing.T) {
	docs := []*rag.Document{doc("a", "x", 0), doc("b", "y", 0)}
	c := NewRetrievalCapability(&capFakeRetriever{hybrid: docs}, nil)
	res, err := c.Retrieve(context.Background(), 1, []string{"kb1"}, capBaseQuery())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res.Chunks) != 2 {
		t.Fatalf("全 0 分不应被过滤, got %d", len(res.Chunks))
	}
}

// 可插拔重排（默认关）：未开重排时按分数降序，reranker 不被调用。
func TestCapability_RerankDisabledByDefault(t *testing.T) {
	rr := &reverseReranker{}
	ret := &capFakeRetriever{hybrid: []*rag.Document{doc("a", "A", 0.9), doc("b", "B", 0.5)}}
	c := NewRetrievalCapability(ret, rr)

	res, err := c.Retrieve(context.Background(), 1, []string{"kb1"}, capBaseQuery()) // EnableRerank 默认 false
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if rr.called {
		t.Fatal("默认不应触发重排")
	}
	if res.Chunks[0].ChunkID != "a" {
		t.Fatalf("默认应按分数降序, got %+v", res.Chunks)
	}
}

// 可插拔重排（开）：注入 reranker 且 EnableRerank=true 时由重排决定顺序。
func TestCapability_RerankEnabledReordersByReranker(t *testing.T) {
	rr := &reverseReranker{}
	// 分数降序本应 a,b；reverse 重排后应 b,a。
	ret := &capFakeRetriever{hybrid: []*rag.Document{doc("a", "A", 0.9), doc("b", "B", 0.5)}}
	c := NewRetrievalCapability(ret, rr)

	q := capBaseQuery()
	q.EnableRerank = true
	res, err := c.Retrieve(context.Background(), 1, []string{"kb1"}, q)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !rr.called {
		t.Fatal("EnableRerank=true 且注入 reranker 时应触发重排")
	}
	if res.Chunks[0].ChunkID != "b" || res.Chunks[1].ChunkID != "a" {
		t.Fatalf("应由 reranker 决定顺序(倒序), got %+v", res.Chunks)
	}
}

// EnableRerank=true 但未注入 reranker（nil）→ 退回分数降序，不 panic（可插拔缺省）。
func TestCapability_RerankEnabledButNilReranker(t *testing.T) {
	ret := &capFakeRetriever{hybrid: []*rag.Document{doc("a", "A", 0.9), doc("b", "B", 0.5)}}
	c := NewRetrievalCapability(ret, nil)

	q := capBaseQuery()
	q.EnableRerank = true
	res, err := c.Retrieve(context.Background(), 1, []string{"kb1"}, q)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Chunks[0].ChunkID != "a" {
		t.Fatalf("无 reranker 应退回分数降序, got %+v", res.Chunks)
	}
}

// 单片按 rune 截断（保护 CJK），并追加省略标记。
func TestCapability_TruncatesLongContentByRune(t *testing.T) {
	long := strings.Repeat("字", capMaxChunkContentChars+50)
	ret := &capFakeRetriever{hybrid: []*rag.Document{doc("a", long, 0.9)}}
	c := NewRetrievalCapability(ret, nil)

	res, err := c.Retrieve(context.Background(), 1, []string{"kb1"}, capBaseQuery())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	got := []rune(res.Chunks[0].Content)
	if len(got) <= capMaxChunkContentChars {
		t.Fatalf("超长内容应被截断, 实际 rune 数 %d", len(got))
	}
	if !strings.HasSuffix(res.Chunks[0].Content, "…（内容已截断）") {
		t.Fatalf("截断应追加省略标记, got 尾部 %q", string(got[len(got)-10:]))
	}
	// 主体应恰为 max 个 rune。
	if string(got[:capMaxChunkContentChars]) != strings.Repeat("字", capMaxChunkContentChars) {
		t.Fatal("截断主体应保留前 max 个 rune")
	}
}

// 出处标签：优先文档标题元数据，退回 KnowledgeID，再退回 ChunkID。
func TestCapability_SourceLabelPrefersTitle(t *testing.T) {
	withTitle := &rag.Document{ChunkID: "c1", KnowledgeID: "k1", Content: "x", Score: 0.9,
		Metadata: map[string]interface{}{"title": "季度财报.pdf"}}
	withKnowledge := &rag.Document{ChunkID: "c2", KnowledgeID: "k2", Content: "y", Score: 0.8}
	onlyChunk := &rag.Document{ChunkID: "c3", Content: "z", Score: 0.7}

	ret := &capFakeRetriever{hybrid: []*rag.Document{withTitle, withKnowledge, onlyChunk}}
	c := NewRetrievalCapability(ret, nil)

	res, err := c.Retrieve(context.Background(), 1, []string{"kb1"}, capBaseQuery())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := map[string]string{"c1": "季度财报.pdf", "c2": "k2", "c3": "c3"}
	for _, ch := range res.Chunks {
		if want[ch.ChunkID] != ch.Source {
			t.Fatalf("chunk %s 出处 = %q, 期望 %q", ch.ChunkID, ch.Source, want[ch.ChunkID])
		}
	}
}

// TopK 兜底：TopK≤0 时用 capDefaultMaxChunks 硬上限截断合并结果。
func TestCapability_DefaultChunkCap(t *testing.T) {
	docs := make([]*rag.Document, 0, capDefaultMaxChunks+10)
	for i := 0; i < capDefaultMaxChunks+10; i++ {
		docs = append(docs, doc("c"+string(rune('a'+i%26))+string(rune('0'+i/26)), "内容", 0.9))
	}
	ret := &capFakeRetriever{vector: docs}
	c := NewRetrievalCapability(ret, nil)

	q := capBaseQuery()
	q.Mode = "vector" // 避免 hybrid 下限干扰计数（同分下限不删）
	q.TopK = 0
	res, err := c.Retrieve(context.Background(), 1, []string{"kb1"}, q)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res.Chunks) != capDefaultMaxChunks {
		t.Fatalf("TopK≤0 应用硬上限 %d 截断, got %d", capDefaultMaxChunks, len(res.Chunks))
	}
	// 兜底须同时下达给底层检索器 opts.TopK：否则真实检索器会 results[:0]/topK=0 直接返回空。
	if ret.lastOpts == nil || ret.lastOpts.TopK != capDefaultMaxChunks {
		t.Fatalf("TopK≤0 应把硬上限 %d 透传给底层 opts.TopK, got %+v", capDefaultMaxChunks, ret.lastOpts)
	}
}

// 显式 TopK 生效：合并结果超过 TopK 时截断到 TopK。
func TestCapability_ExplicitTopKCaps(t *testing.T) {
	docs := []*rag.Document{doc("a", "A", 0.9), doc("b", "B", 0.8), doc("c", "C", 0.7)}
	ret := &capFakeRetriever{vector: docs}
	c := NewRetrievalCapability(ret, nil)

	q := capBaseQuery()
	q.Mode = "vector"
	q.TopK = 2
	res, err := c.Retrieve(context.Background(), 1, []string{"kb1"}, q)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res.Chunks) != 2 || res.Chunks[0].ChunkID != "a" || res.Chunks[1].ChunkID != "b" {
		t.Fatalf("应截断到 TopK=2 保留高分, got %+v", res.Chunks)
	}
}
