// Package reflection provides the domain interfaces for Agent self-reflection capability.
package reflection

import "context"

// ========================================
// Memory 反思记忆接口
// ========================================

// ReflectionMemory 定义了反思经验的存储和检索接口。
// 可插拔设计支持不同的存储后端（Milvus、Redis 等）。
type ReflectionMemory interface {
	// Store 存储反思记录。
	Store(ctx context.Context, record *ReflectionRecord) error

	// Retrieve 检索与任务相关的历史反思记录。
	// 返回按相似度排序的记录，优先返回失败经验。
	Retrieve(ctx context.Context, agentID, task string, limit int) ([]*ReflectionRecord, error)

	// UpdateSuccess 更新记录为成功状态。
	UpdateSuccess(ctx context.Context, id string) error

	// Cleanup 清理过期的反思记录。
	Cleanup(ctx context.Context) error
}

// Vectorizer 向量化接口，用于 Memory 的语义检索
type Vectorizer interface {
	// Embed 将文本转换为向量。
	Embed(ctx context.Context, text string) ([]float32, error)
}
