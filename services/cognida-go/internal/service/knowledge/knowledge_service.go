// Package kb provides Knowledge service implementations
package knowledge

import (
	"context"
	"fmt"

	domain_knowledge "cognida/internal/model/knowledge"
)

// ========================================
// Knowledge Service Implementation
// ========================================

// knowledgeService implements KnowledgeService
type knowledgeService struct {
	knowledgeRepo      domain_knowledge.KnowledgeRepository
	chunkRepo          domain_knowledge.ChunkRepository
	kbSettingRepo      domain_knowledge.KnowledgeBaseSettingRepository
	transactionManager domain_knowledge.TransactionManager
}

// NewKnowledgeService creates a new knowledge service
func NewKnowledgeService(
	knowledgeRepo domain_knowledge.KnowledgeRepository,
	chunkRepo domain_knowledge.ChunkRepository,
	kbSettingRepo domain_knowledge.KnowledgeBaseSettingRepository,
	transactionManager domain_knowledge.TransactionManager,
) KnowledgeService {
	return &knowledgeService{
		knowledgeRepo:      knowledgeRepo,
		chunkRepo:          chunkRepo,
		kbSettingRepo:      kbSettingRepo,
		transactionManager: transactionManager,
	}
}

// Create creates a knowledge entry
func (s *knowledgeService) Create(ctx context.Context, knowledge *domain_knowledge.Knowledge) error {
	return s.knowledgeRepo.Create(ctx, knowledge)
}

// CreateBatch creates knowledge entries in batch
func (s *knowledgeService) CreateBatch(ctx context.Context, knowledgeList []*domain_knowledge.Knowledge) error {
	return s.knowledgeRepo.CreateBatch(ctx, knowledgeList)
}

// FindByID finds knowledge by ID
func (s *knowledgeService) FindByID(ctx context.Context, id string) (*domain_knowledge.Knowledge, error) {
	return s.knowledgeRepo.FindByID(ctx, id)
}

// FindByKnowledgeBaseID finds knowledge by KB ID
func (s *knowledgeService) FindByKnowledgeBaseID(
	ctx context.Context,
	kbID string,
	query *domain_knowledge.KnowledgeListQuery,
) ([]*domain_knowledge.Knowledge, int64, error) {
	return s.knowledgeRepo.FindByKnowledgeBaseID(ctx, kbID, query)
}

// FindByTenantID finds knowledge by tenant ID
func (s *knowledgeService) FindByTenantID(
	ctx context.Context,
	tenantID int64,
	page, pageSize int,
) ([]*domain_knowledge.Knowledge, int64, error) {
	return s.knowledgeRepo.FindByTenantID(ctx, tenantID, page, pageSize)
}

// Update updates knowledge
func (s *knowledgeService) Update(ctx context.Context, knowledge *domain_knowledge.Knowledge) error {
	return s.knowledgeRepo.Update(ctx, knowledge)
}

// UpdateParseStatus updates parse status
func (s *knowledgeService) UpdateParseStatus(
	ctx context.Context,
	id string,
	parseStatus string,
	errorMessage string,
) error {
	return s.knowledgeRepo.UpdateParseStatus(ctx, id, parseStatus, errorMessage)
}

// UpdateChunkCount updates chunk count
func (s *knowledgeService) UpdateChunkCount(ctx context.Context, id string, chunkCount int) error {
	return s.knowledgeRepo.UpdateChunkCount(ctx, id, chunkCount)
}

// Delete soft deletes knowledge
func (s *knowledgeService) Delete(ctx context.Context, id string) error {
	return s.knowledgeRepo.Delete(ctx, id)
}

// DeleteBatch batch deletes knowledge
func (s *knowledgeService) DeleteBatch(ctx context.Context, ids []string) error {
	return s.knowledgeRepo.DeleteBatch(ctx, ids)
}

// HardDelete hard deletes knowledge
func (s *knowledgeService) HardDelete(ctx context.Context, id string) error {
	return s.knowledgeRepo.HardDelete(ctx, id)
}

// FindByStatus finds knowledge by status
func (s *knowledgeService) FindByStatus(
	ctx context.Context,
	tenantID int64,
	parseStatus string,
	limit int,
) ([]*domain_knowledge.Knowledge, error) {
	return s.knowledgeRepo.FindByStatus(ctx, tenantID, parseStatus, limit)
}

// FindByFileHash finds knowledge by file hash
func (s *knowledgeService) FindByFileHash(
	ctx context.Context,
	tenantID int64,
	fileHash string,
) (*domain_knowledge.Knowledge, error) {
	return s.knowledgeRepo.FindByFileHash(ctx, tenantID, fileHash)
}

// ========================================
// Chunk operations
// ========================================

// FindChunkByID finds chunk by ID
func (s *knowledgeService) FindChunkByID(ctx context.Context, id string) (*domain_knowledge.Chunk, error) {
	return s.chunkRepo.FindByID(ctx, id)
}

// FindChunkByKnowledgeBaseID finds chunks by KB ID
func (s *knowledgeService) FindChunkByKnowledgeBaseID(
	ctx context.Context,
	kbID string,
	page, pageSize int,
) ([]*domain_knowledge.Chunk, int64, error) {
	query := &domain_knowledge.ChunkListQuery{
		Page:     page,
		PageSize: pageSize,
	}
	return s.chunkRepo.FindByKnowledgeBaseID(ctx, kbID, query)
}

// FindChunkByKnowledgeID finds chunks by knowledge ID
func (s *knowledgeService) FindChunkByKnowledgeID(
	ctx context.Context,
	knowledgeID string,
	enabledOnly bool,
) ([]*domain_knowledge.Chunk, error) {
	return s.chunkRepo.FindByKnowledgeID(ctx, knowledgeID, enabledOnly)
}

// CreateChunk creates a chunk
func (s *knowledgeService) CreateChunk(ctx context.Context, chunk *domain_knowledge.Chunk) error {
	return s.chunkRepo.Create(ctx, chunk)
}

// CreateChunkBatch creates chunks in batch
func (s *knowledgeService) CreateChunkBatch(ctx context.Context, chunks []*domain_knowledge.Chunk) error {
	return s.chunkRepo.CreateBatch(ctx, chunks)
}

// UpdateChunk updates a chunk
func (s *knowledgeService) UpdateChunk(ctx context.Context, chunk *domain_knowledge.Chunk) error {
	return s.chunkRepo.Update(ctx, chunk)
}

// DeleteChunk deletes a chunk
func (s *knowledgeService) DeleteChunk(ctx context.Context, id string) error {
	return s.chunkRepo.Delete(ctx, id)
}

// DeleteChunkByKnowledgeID deletes chunks by knowledge ID
func (s *knowledgeService) DeleteChunkByKnowledgeID(ctx context.Context, knowledgeID string) error {
	return s.chunkRepo.DeleteByKnowledgeID(ctx, knowledgeID)
}

// ========================================
// KnowledgeBaseSetting operations
// ========================================

// FindKnowledgeBaseSettingByKnowledgeBaseID finds KB setting by KB ID
func (s *knowledgeService) FindKnowledgeBaseSettingByKnowledgeBaseID(
	ctx context.Context,
	kbID string,
) (*domain_knowledge.KnowledgeBaseSetting, error) {
	return s.kbSettingRepo.FindByKnowledgeBaseID(ctx, kbID)
}

// FindKnowledgeBaseSettingByKnowledgeBaseIDAndKey finds KB setting by KB ID and key
func (s *knowledgeService) FindKnowledgeBaseSettingByKnowledgeBaseIDAndKey(
	ctx context.Context,
	kbID string,
	key string,
) (*domain_knowledge.KnowledgeBaseSetting, error) {
	return s.kbSettingRepo.FindByKnowledgeBaseID(ctx, kbID)
}

// CreateKnowledgeBaseSetting creates a KB setting
func (s *knowledgeService) CreateKnowledgeBaseSetting(ctx context.Context, setting *domain_knowledge.KnowledgeBaseSetting) error {
	return s.kbSettingRepo.Create(ctx, setting)
}

// UpdateKnowledgeBaseSetting updates a KB setting
func (s *knowledgeService) UpdateKnowledgeBaseSetting(ctx context.Context, setting *domain_knowledge.KnowledgeBaseSetting) error {
	return s.kbSettingRepo.Update(ctx, setting)
}

// UpdateRetrievalConfig updates retrieval config
func (s *knowledgeService) UpdateRetrievalConfig(
	ctx context.Context,
	kbID string,
	mode string,
	threshold float64,
	topK int,
) error {
	return s.kbSettingRepo.UpdateRetrievalConfig(ctx, kbID, mode, threshold, topK)
}

// ========================================
// Utility methods
// ========================================

// GetTransactionManager returns the transaction manager
func (s *knowledgeService) GetTransactionManager() domain_knowledge.TransactionManager {
	return s.transactionManager
}

// CountByKnowledgeBaseID counts knowledge by KB ID
func (s *knowledgeService) CountByKnowledgeBaseID(ctx context.Context, kbID string) (int64, error) {
	knowledges, _, err := s.FindByKnowledgeBaseID(ctx, kbID, &domain_knowledge.KnowledgeListQuery{})
	if err != nil {
		return 0, err
	}
	return int64(len(knowledges)), nil
}

// CountByTenantID counts knowledge by tenant ID
func (s *knowledgeService) CountByTenantID(ctx context.Context, tenantID int64) (int64, error) {
	knowledges, _, err := s.FindByTenantID(ctx, tenantID, 1, 1000)
	if err != nil {
		return 0, err
	}
	return int64(len(knowledges)), nil
}

// ========================================
// Legacy adapter methods for backward compatibility
// ========================================

// GetKnowledgeByID gets knowledge by ID (alias)
func (s *knowledgeService) GetKnowledgeByID(ctx context.Context, id string) (*domain_knowledge.Knowledge, error) {
	return s.FindByID(ctx, id)
}

// GetChunksByKnowledgeID gets chunks by knowledge ID (alias)
func (s *knowledgeService) GetChunksByKnowledgeID(
	ctx context.Context,
	knowledgeID string,
) ([]*domain_knowledge.Chunk, error) {
	return s.FindChunkByKnowledgeID(ctx, knowledgeID, false)
}

// CountChunksByKnowledgeID counts chunks by knowledge ID
func (s *knowledgeService) CountChunksByKnowledgeID(
	ctx context.Context,
	knowledgeID string,
) (int, error) {
	chunks, err := s.FindChunkByKnowledgeID(ctx, knowledgeID, false)
	if err != nil {
		return 0, err
	}
	return len(chunks), nil
}

// WithTransaction executes a function within a transaction
func (s *knowledgeService) WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.transactionManager == nil {
		// No transaction manager, execute directly
		return fn(ctx)
	}
	return s.transactionManager.WithTransaction(ctx, fn)
}

// GetPendingKnowledge gets pending knowledge
func (s *knowledgeService) GetPendingKnowledge(
	ctx context.Context,
	tenantID int64,
	limit int,
) ([]*domain_knowledge.Knowledge, error) {
	return s.FindByStatus(ctx, tenantID, "pending", limit)
}

// GetProcessingKnowledge gets processing knowledge
func (s *knowledgeService) GetProcessingKnowledge(
	ctx context.Context,
	tenantID int64,
	limit int,
) ([]*domain_knowledge.Knowledge, error) {
	return s.FindByStatus(ctx, tenantID, "processing", limit)
}

// MarkAsProcessing marks as processing
func (s *knowledgeService) MarkAsProcessing(ctx context.Context, id string) error {
	return s.UpdateParseStatus(ctx, id, "processing", "")
}

// MarkAsCompleted marks as completed
func (s *knowledgeService) MarkAsCompleted(ctx context.Context, id string, chunkCount int) error {
	if err := s.UpdateChunkCount(ctx, id, chunkCount); err != nil {
		return fmt.Errorf("failed to update chunk count: %w", err)
	}
	return s.UpdateParseStatus(ctx, id, "completed", "")
}

// MarkAsFailed marks as failed
func (s *knowledgeService) MarkAsFailed(ctx context.Context, id string, errMsg string) error {
	return s.UpdateParseStatus(ctx, id, "failed", errMsg)
}
