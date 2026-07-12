// Package kb provides Knowledge Base service interfaces
package knowledge

import (
	"context"

	domain_knowledge "link/internal/model/knowledge"
)

// ========================================
// Knowledge Base Service
// ========================================

// KnowledgeBaseService defines the contract for knowledge base management
type KnowledgeBaseService interface {
	// Create creates a new knowledge base
	Create(ctx context.Context, kb *domain_knowledge.KnowledgeBase, setting *domain_knowledge.KnowledgeBaseSetting) error

	// CreateFromRequest 从 DTO 创建知识库（ID 在服务层生成）
	CreateFromRequest(ctx context.Context, req *CreateKnowledgeBaseRequest, tenantID, userID int64) (*domain_knowledge.KnowledgeBase, error)

	// FindByID finds a knowledge base by ID（tenantID 强制归属校验，跨租户返回 not found）
	FindByID(ctx context.Context, id string, tenantID int64) (*domain_knowledge.KnowledgeBase, error)

	// FindByIDWithSettings finds a knowledge base with settings loaded（tenantID 强制归属校验）
	FindByIDWithSettings(ctx context.Context, id string, tenantID int64) (*domain_knowledge.KnowledgeBase, error)

	// FindByTenantID lists knowledge bases for a tenant
	FindByTenantID(ctx context.Context, tenantID int64, page, pageSize int) ([]*domain_knowledge.KnowledgeBase, int64, error)

	// Update updates a knowledge base
	Update(ctx context.Context, kb *domain_knowledge.KnowledgeBase) error

	// UpdateFromRequest 从 DTO 更新知识库（tenantID 强制归属校验）
	UpdateFromRequest(ctx context.Context, id string, tenantID int64, req *UpdateKnowledgeBaseRequest) (*domain_knowledge.KnowledgeBase, error)

	// UpdateWithSettings updates a knowledge base and its settings
	UpdateWithSettings(ctx context.Context, kb *domain_knowledge.KnowledgeBase, setting *domain_knowledge.KnowledgeBaseSetting) error

	// Delete soft deletes a knowledge base（tenantID 强制归属校验）
	Delete(ctx context.Context, id string, tenantID int64) error

	// Exists checks if a knowledge base exists
	Exists(ctx context.Context, id string) (bool, error)

	// GetStats gets statistics for a knowledge base（tenantID 强制归属校验）
	GetStats(ctx context.Context, kbID string, tenantID int64) (*domain_knowledge.KnowledgeBaseStats, error)

	// GetKnowledgeList gets knowledge list for a KB（tenantID 强制归属校验）
	GetKnowledgeList(ctx context.Context, kbID string, tenantID int64, page, pageSize int, status string) ([]*domain_knowledge.Knowledge, int64, error)

	// DeleteKnowledge deletes knowledge from a KB
	DeleteKnowledge(ctx context.Context, kbID, knowledgeID string, tenantID int64) error

	// GetChunks gets chunks for a KB（tenantID 强制归属校验）
	GetChunks(ctx context.Context, kbID string, tenantID int64, page, pageSize int, knowledgeID string) ([]*domain_knowledge.Chunk, int64, error)

	// CreateChunk creates a chunk
	CreateChunk(ctx context.Context, chunk *domain_knowledge.Chunk) error

	// UploadDocument 上传文档文件（同步创建记录，fileHash 用于防止重传相同文件）
	UploadDocument(ctx context.Context, kbID string, tenantID, userID int64, fileName, fileType string, fileSize int64, filePath, fileHash string) (*domain_knowledge.Knowledge, error)

	// CheckDuplicateByHash 在知识库范围内检查是否已存在相同文件（防止重传）；存在则返回已有文档（tenantID 强制归属校验）
	CheckDuplicateByHash(ctx context.Context, kbID string, tenantID int64, fileHash string) (*domain_knowledge.Knowledge, error)

	// UpdateParseStatus 更新文档解析状态（如处理失败时标记 failed）
	UpdateParseStatus(ctx context.Context, id, parseStatus, errorMessage string) error

	// GetKnowledgeDetail 获取文档详情（tenantID 强制归属校验）
	GetKnowledgeDetail(ctx context.Context, kbID string, tenantID int64, knowledgeID string) (*domain_knowledge.Knowledge, error)

	// GetKnowledgeStatus 获取文档处理状态（tenantID 强制归属校验）
	GetKnowledgeStatus(ctx context.Context, kbID string, tenantID int64, knowledgeID string) (*domain_knowledge.Knowledge, error)

	// GetKnowledgeListWithStatus 获取指定状态列表的知识条目（tenantID 强制归属校验）
	GetKnowledgeListWithStatus(ctx context.Context, kbID string, tenantID int64, page, pageSize int, statuses []string) ([]*domain_knowledge.Knowledge, int64, error)

	// GetChunkDetail 获取分块详情（tenantID 强制归属校验）
	GetChunkDetail(ctx context.Context, kbID string, tenantID int64, chunkID string) (*domain_knowledge.Chunk, error)

	// UpdateChunk 更新分块（tenantID 强制归属校验）
	UpdateChunk(ctx context.Context, kbID string, tenantID int64, chunkID string, content *string) (*domain_knowledge.Chunk, error)

	// DeleteChunk 删除分块（tenantID 强制归属校验）
	DeleteChunk(ctx context.Context, kbID string, tenantID int64, chunkID string) error

	// Search 搜索知识库（仅在属于 tenantID 的知识库内检索，越权 kbID 被静默剔除）
	Search(ctx context.Context, kbIDs []string, tenantID int64, query string, topK int, minScore float64) ([]*domain_knowledge.Chunk, error)

	// ToResponse converts a domain entity to response DTO
	ToResponse(entity *domain_knowledge.KnowledgeBase) *KnowledgeBaseResponse
	// ToResponseList converts a list of domain entities to response DTOs
	ToResponseList(entities []*domain_knowledge.KnowledgeBase) []*KnowledgeBaseResponse
}

// ========================================
// Knowledge Service
// ========================================

// KnowledgeService defines the contract for knowledge management
type KnowledgeService interface {
	// Create creates a knowledge entry
	Create(ctx context.Context, knowledge *domain_knowledge.Knowledge) error

	// CreateBatch creates knowledge entries in batch
	CreateBatch(ctx context.Context, knowledgeList []*domain_knowledge.Knowledge) error

	// FindByID finds knowledge by ID
	FindByID(ctx context.Context, id string) (*domain_knowledge.Knowledge, error)

	// FindByKnowledgeBaseID finds knowledge by KB ID
	FindByKnowledgeBaseID(ctx context.Context, kbID string, query *domain_knowledge.KnowledgeListQuery) ([]*domain_knowledge.Knowledge, int64, error)

	// FindByTenantID finds knowledge by tenant ID
	FindByTenantID(ctx context.Context, tenantID int64, page, pageSize int) ([]*domain_knowledge.Knowledge, int64, error)

	// Update updates knowledge
	Update(ctx context.Context, knowledge *domain_knowledge.Knowledge) error

	// UpdateParseStatus updates parse status
	UpdateParseStatus(ctx context.Context, id string, parseStatus string, errorMessage string) error

	// UpdateChunkCount updates chunk count
	UpdateChunkCount(ctx context.Context, id string, chunkCount int) error

	// Delete soft deletes knowledge
	Delete(ctx context.Context, id string) error

	// DeleteBatch batch deletes knowledge
	DeleteBatch(ctx context.Context, ids []string) error

	// HardDelete hard deletes knowledge
	HardDelete(ctx context.Context, id string) error

	// FindByStatus finds knowledge by status
	FindByStatus(ctx context.Context, tenantID int64, parseStatus string, limit int) ([]*domain_knowledge.Knowledge, error)

	// FindByFileHash finds knowledge by file hash
	FindByFileHash(ctx context.Context, tenantID int64, fileHash string) (*domain_knowledge.Knowledge, error)

	// Chunk operations
	FindChunkByID(ctx context.Context, id string) (*domain_knowledge.Chunk, error)
	FindChunkByKnowledgeBaseID(ctx context.Context, kbID string, page, pageSize int) ([]*domain_knowledge.Chunk, int64, error)
	FindChunkByKnowledgeID(ctx context.Context, knowledgeID string, enabledOnly bool) ([]*domain_knowledge.Chunk, error)
	CreateChunk(ctx context.Context, chunk *domain_knowledge.Chunk) error
	CreateChunkBatch(ctx context.Context, chunks []*domain_knowledge.Chunk) error
	UpdateChunk(ctx context.Context, chunk *domain_knowledge.Chunk) error
	DeleteChunk(ctx context.Context, id string) error
	DeleteChunkByKnowledgeID(ctx context.Context, knowledgeID string) error

	// KnowledgeBaseSetting operations
	FindKnowledgeBaseSettingByKnowledgeBaseID(ctx context.Context, kbID string) (*domain_knowledge.KnowledgeBaseSetting, error)
	FindKnowledgeBaseSettingByKnowledgeBaseIDAndKey(ctx context.Context, kbID string, key string) (*domain_knowledge.KnowledgeBaseSetting, error)
	CreateKnowledgeBaseSetting(ctx context.Context, setting *domain_knowledge.KnowledgeBaseSetting) error
	UpdateKnowledgeBaseSetting(ctx context.Context, setting *domain_knowledge.KnowledgeBaseSetting) error
	UpdateRetrievalConfig(ctx context.Context, kbID string, mode string, threshold float64, topK int) error

	// Utility methods
	CountByKnowledgeBaseID(ctx context.Context, kbID string) (int64, error)
	CountByTenantID(ctx context.Context, tenantID int64) (int64, error)

	// Transaction methods
	WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// ========================================
// Document Processor Service
// ========================================

// DocumentProcessorService defines the contract for document processing pipeline
type DocumentProcessorService interface {
	// ProcessDocument processes a document (parse + chunk + store + vectorize + graph extract)
	ProcessDocument(ctx context.Context, tenantID int64, userID int64, req *ProcessDocumentRequest) (*ProcessDocumentResponse, error)

	// RebuildKnowledgeBaseGraph re-extracts the knowledge graph for all completed documents in a KB,
	// reusing stored chunks (no re-parse). Idempotent per document.
	RebuildKnowledgeBaseGraph(ctx context.Context, tenantID int64, kbID string) (*RebuildGraphResponse, error)
}
