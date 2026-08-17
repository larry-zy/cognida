package knowledge

import (
	"context"
	"fmt"
	"testing"

	"github.com/cloudwego/eino/components/embedding"

	domain_knowledge "cognida/internal/model/knowledge"
)

type fakeVectorProjectionStore struct {
	exists      bool
	hasErr      error
	deleteErr   error
	hasCalls    int
	deleteCalls int
}

func (f *fakeVectorProjectionStore) HasCollection(context.Context, int64) (bool, error) {
	f.hasCalls++
	return f.exists, f.hasErr
}

func (f *fakeVectorProjectionStore) DeleteByKnowledgeID(context.Context, int64, string) error {
	f.deleteCalls++
	return f.deleteErr
}

type fakeVectorCollectionStore struct {
	exists      bool
	hasCalls    int
	createCalls int
	indexCalls  int
	loadCalls   int
	dimension   int
	opts        *domain_knowledge.CollectionOptions
}

type recordingEmbedder struct {
	batches [][]string
	count   int
}

func (e *recordingEmbedder) EmbedStrings(_ context.Context, texts []string, _ ...embedding.Option) ([][]float64, error) {
	batchCopy := append([]string(nil), texts...)
	e.batches = append(e.batches, batchCopy)
	result := make([][]float64, len(texts))
	for i := range texts {
		result[i] = []float64{float64(e.count)}
		e.count++
	}
	return result, nil
}

func (f *fakeVectorCollectionStore) HasCollection(context.Context, int64) (bool, error) {
	f.hasCalls++
	return f.exists, nil
}

func (f *fakeVectorCollectionStore) CreateCollection(_ context.Context, _ int64, dimension int, opts *domain_knowledge.CollectionOptions) error {
	f.createCalls++
	f.dimension = dimension
	f.opts = opts
	return nil
}

func (f *fakeVectorCollectionStore) CreateIndex(context.Context, int64, string, domain_knowledge.IndexType, domain_knowledge.MetricType, map[string]string) error {
	f.indexCalls++
	return nil
}

func (f *fakeVectorCollectionStore) LoadCollection(context.Context, int64, bool) error {
	f.loadCalls++
	return nil
}

func TestDeleteVectorProjectionByKnowledgeIDMissingCollectionIsSuccess(t *testing.T) {
	repo := &fakeVectorProjectionStore{exists: false}

	if err := deleteVectorProjectionByKnowledgeID(context.Background(), repo, 0, "knowledge-1"); err != nil {
		t.Fatalf("collection 不存在时删除应幂等成功: %v", err)
	}
	if repo.hasCalls != 1 {
		t.Fatalf("HasCollection 调用次数 = %d, want 1", repo.hasCalls)
	}
	if repo.deleteCalls != 0 {
		t.Fatalf("collection 不存在时不应调用 DeleteByKnowledgeID，实际 %d 次", repo.deleteCalls)
	}
}

func TestDeleteVectorProjectionByKnowledgeIDExistingCollectionDeletes(t *testing.T) {
	repo := &fakeVectorProjectionStore{exists: true}

	if err := deleteVectorProjectionByKnowledgeID(context.Background(), repo, 0, "knowledge-1"); err != nil {
		t.Fatalf("删除已有 collection 中的投影失败: %v", err)
	}
	if repo.deleteCalls != 1 {
		t.Fatalf("DeleteByKnowledgeID 调用次数 = %d, want 1", repo.deleteCalls)
	}
}

func TestEnsureVectorCollectionExistingCollectionIsFastPath(t *testing.T) {
	repo := &fakeVectorCollectionStore{exists: true}

	if err := ensureVectorCollection(context.Background(), repo, 0, 1536); err != nil {
		t.Fatalf("已有 collection 的快速路径失败: %v", err)
	}
	if repo.createCalls != 0 || repo.indexCalls != 0 || repo.loadCalls != 0 {
		t.Fatalf("已有 collection 不应重复初始化: create=%d index=%d load=%d", repo.createCalls, repo.indexCalls, repo.loadCalls)
	}
}

func TestEnsureVectorCollectionInitializesMissingCollection(t *testing.T) {
	repo := &fakeVectorCollectionStore{exists: false}

	if err := ensureVectorCollection(context.Background(), repo, 0, 1024); err != nil {
		t.Fatalf("初始化缺失 collection 失败: %v", err)
	}
	if repo.createCalls != 1 || repo.indexCalls != 1 || repo.loadCalls != 1 {
		t.Fatalf("初始化调用次数不正确: create=%d index=%d load=%d", repo.createCalls, repo.indexCalls, repo.loadCalls)
	}
	if repo.dimension != 1024 {
		t.Fatalf("collection dimension = %d, want 1024", repo.dimension)
	}
	if repo.opts == nil || !repo.opts.EnableBM25 {
		t.Fatal("首次初始化必须启用默认 BM25 collection 配置")
	}
}

func TestEmbedTextsInBatchesRespectsLimitAndOrder(t *testing.T) {
	embedder := &recordingEmbedder{}
	texts := make([]string, 38)
	for i := range texts {
		texts[i] = fmt.Sprintf("chunk-%d", i)
	}

	embeddings, err := embedTextsInBatches(context.Background(), embedder, texts)
	if err != nil {
		t.Fatalf("分批 embedding 失败: %v", err)
	}
	wantBatchSizes := []int{10, 10, 10, 8}
	if len(embedder.batches) != len(wantBatchSizes) {
		t.Fatalf("批次数 = %d, want %d", len(embedder.batches), len(wantBatchSizes))
	}
	for i, want := range wantBatchSizes {
		if got := len(embedder.batches[i]); got != want {
			t.Fatalf("第 %d 批大小 = %d, want %d", i, got, want)
		}
	}
	for i, vector := range embeddings {
		if len(vector) != 1 || vector[0] != float64(i) {
			t.Fatalf("第 %d 个结果顺序错误: %v", i, vector)
		}
	}
}
