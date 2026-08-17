package knowledge

import (
	"context"
	"fmt"
	"log"

	domain_knowledge "cognida/internal/model/knowledge"
)

type vectorProjectionStore interface {
	HasCollection(ctx context.Context, kbID int64) (bool, error)
	DeleteByKnowledgeID(ctx context.Context, kbID int64, knowledgeID string) error
}

type vectorCollectionStore interface {
	HasCollection(ctx context.Context, kbID int64) (bool, error)
	CreateCollection(ctx context.Context, kbID int64, dimension int, opts *domain_knowledge.CollectionOptions) error
	CreateIndex(ctx context.Context, kbID int64, fieldName string, indexType domain_knowledge.IndexType, metricType domain_knowledge.MetricType, params map[string]string) error
	LoadCollection(ctx context.Context, kbID int64, async bool) error
}

// deleteVectorProjectionByKnowledgeID 删除文档向量投影。collection 不存在表示目标数据已不存在，
// 按删除的幂等语义直接成功；真实的 Milvus 查询或删除错误仍返回给调用方重试。
func deleteVectorProjectionByKnowledgeID(ctx context.Context, repo vectorProjectionStore, kbID int64, knowledgeID string) error {
	exists, err := repo.HasCollection(ctx, kbID)
	if err != nil {
		return fmt.Errorf("check vector collection before delete: %w", err)
	}
	if !exists {
		log.Printf("[KnowledgeBase] Milvus collection 不存在，跳过文档向量删除: knowledge_id=%s", knowledgeID)
		return nil
	}
	return repo.DeleteByKnowledgeID(ctx, kbID, knowledgeID)
}

// ensureVectorCollection 创建首次向量写入所需的统一 collection、稠密/稀疏索引并同步加载。
// 调用方负责串行化首次初始化，避免并发上传同时创建相同 collection。
func ensureVectorCollection(ctx context.Context, repo vectorCollectionStore, kbID int64, dimension int) error {
	if dimension <= 0 {
		return fmt.Errorf("invalid embedding dimension: %d", dimension)
	}

	exists, err := repo.HasCollection(ctx, kbID)
	if err != nil {
		return fmt.Errorf("check vector collection: %w", err)
	}
	if exists {
		return nil
	}

	if err := repo.CreateCollection(ctx, kbID, dimension, domain_knowledge.DefaultCollectionOptions()); err != nil {
		return fmt.Errorf("create vector collection: %w", err)
	}
	if err := repo.CreateIndex(
		ctx,
		kbID,
		"dense_vector",
		domain_knowledge.IndexTypeHnsw,
		domain_knowledge.MetricTypeL2,
		map[string]string{"M": "16", "efConstruction": "256"},
	); err != nil {
		return fmt.Errorf("create dense vector index: %w", err)
	}
	if err := repo.LoadCollection(ctx, kbID, false); err != nil {
		return fmt.Errorf("load vector collection: %w", err)
	}

	log.Printf("[DocumentProcessor] Milvus collection 初始化完成: dimension=%d", dimension)
	return nil
}
