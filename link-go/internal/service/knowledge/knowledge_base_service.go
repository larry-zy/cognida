// Package kb provides Knowledge Base service implementations
package knowledge

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"link/internal/infrastructure/id"
	domain_knowledge "link/internal/model/knowledge"
)

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// IsValidationError checks if error is a ValidationError
func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}

// ========================================
// Knowledge Base Use Case Implementation
// ========================================

// knowledgeBaseService implements KnowledgeBaseService
type knowledgeBaseService struct {
	kbRepo        domain_knowledge.KnowledgeBaseRepository
	kbSettingRepo domain_knowledge.KnowledgeBaseSettingRepository
	knowledgeRepo domain_knowledge.KnowledgeRepository
	chunkRepo     domain_knowledge.ChunkRepository
	statsQuerier  domain_knowledge.KnowledgeStatsQuerier
	vectorRepo    domain_knowledge.VectorRepository
	graphRepo     domain_knowledge.GraphRepository
	idGenerator   id.IDGenerator
}

// NewKnowledgeBaseService creates a new knowledge base service
func NewKnowledgeBaseService(
	kbRepo domain_knowledge.KnowledgeBaseRepository,
	kbSettingRepo domain_knowledge.KnowledgeBaseSettingRepository,
	knowledgeRepo domain_knowledge.KnowledgeRepository,
	chunkRepo domain_knowledge.ChunkRepository,
	statsQuerier domain_knowledge.KnowledgeStatsQuerier,
	vectorRepo domain_knowledge.VectorRepository,
	graphRepo domain_knowledge.GraphRepository,
) KnowledgeBaseService {
	return &knowledgeBaseService{
		kbRepo:        kbRepo,
		kbSettingRepo: kbSettingRepo,
		knowledgeRepo: knowledgeRepo,
		chunkRepo:     chunkRepo,
		statsQuerier:  statsQuerier,
		vectorRepo:    vectorRepo,
		graphRepo:     graphRepo,
		idGenerator:   id.NewIDGenerator(),
	}
}

// SetIDGenerator 设置 ID 生成器（主要用于测试）
func (s *knowledgeBaseService) SetIDGenerator(generator id.IDGenerator) {
	s.idGenerator = generator
}

// Create creates a new knowledge base
func (s *knowledgeBaseService) Create(ctx context.Context, kb *domain_knowledge.KnowledgeBase, setting *domain_knowledge.KnowledgeBaseSetting) error {
	// 先创建知识库
	if err := s.kbRepo.Create(ctx, kb); err != nil {
		return err
	}

	// 如果提供了设置，创建设置记录
	if setting != nil {
		setting.KnowledgeBaseID = kb.ID
		if err := s.kbSettingRepo.Create(ctx, setting); err != nil {
			return err
		}
	}

	return nil
}

// CreateFromRequest 从 DTO 创建知识库（生成 ID）
func (s *knowledgeBaseService) CreateFromRequest(
	ctx context.Context,
	req *CreateKnowledgeBaseRequest,
	tenantID, userID int64,
) (*domain_knowledge.KnowledgeBase, error) {
	// 生成 ID
	now := time.Now()
	kb := &domain_knowledge.KnowledgeBase{
		ID:            s.idGenerator.Generate(),
		TenantID:      tenantID,
		UserID:        userID,
		Name:          req.Name,
		Description:   req.Description,
		Avatar:        req.Avatar,
		IsPublic:      req.IsPublic,
		Status:        1,
		DocumentCount: 0,
		ChunkCount:    0,
		StorageSize:   0,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	// 创建知识库
	if err := s.kbRepo.Create(ctx, kb); err != nil {
		return nil, err
	}

	// 如果有检索配置，创建设置记录
	if req.ChunkSize != nil || req.ChunkOverlap != nil || req.GraphEnabled != nil ||
		req.BM25Enabled != nil || req.RetrievalMode != nil || req.TopK != nil || req.Alpha != nil {
		bm25Enabled := false
		if req.BM25Enabled != nil {
			bm25Enabled = *req.BM25Enabled
		}
		chunkingConfig := s.buildChunkingConfig(req)
		setting := &domain_knowledge.KnowledgeBaseSetting{
			KnowledgeBaseID: kb.ID,
			GraphEnabled:    req.GraphEnabled != nil && *req.GraphEnabled,
			BM25Enabled:     &bm25Enabled,
			ChunkingConfig:  &chunkingConfig,
		}
		if err := s.kbSettingRepo.Create(ctx, setting); err != nil {
			return nil, err
		}
		kb.Setting = setting
	}

	return kb, nil
}

// buildChunkingConfig 构建分块配置 JSON
func (s *knowledgeBaseService) buildChunkingConfig(req *CreateKnowledgeBaseRequest) string {
	// TODO: 构建 JSON 配置
	return "{}"
}

// FindByID finds a knowledge base by ID
func (s *knowledgeBaseService) FindByID(ctx context.Context, id string) (*domain_knowledge.KnowledgeBase, error) {
	return s.kbRepo.FindByID(ctx, id)
}

// FindByIDWithSettings finds a knowledge base with settings loaded
func (s *knowledgeBaseService) FindByIDWithSettings(ctx context.Context, id string) (*domain_knowledge.KnowledgeBase, error) {
	kb, err := s.kbRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 加载设置
	setting, err := s.kbSettingRepo.FindByKnowledgeBaseID(ctx, id)
	if err == nil {
		kb.Setting = setting
	}

	return kb, nil
}

// FindByTenantID lists knowledge bases for a tenant
func (s *knowledgeBaseService) FindByTenantID(
	ctx context.Context,
	tenantID int64,
	page, pageSize int,
) ([]*domain_knowledge.KnowledgeBase, int64, error) {
	return s.kbRepo.FindByTenantID(ctx, tenantID, page, pageSize)
}

// Update updates a knowledge base
func (s *knowledgeBaseService) Update(ctx context.Context, kb *domain_knowledge.KnowledgeBase) error {
	return s.kbRepo.Update(ctx, kb)
}

// UpdateFromRequest 从 DTO 更新知识库
func (s *knowledgeBaseService) UpdateFromRequest(
	ctx context.Context,
	id string,
	req *UpdateKnowledgeBaseRequest,
) (*domain_knowledge.KnowledgeBase, error) {
	// 获取现有实体
	kb, err := s.kbRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 应用更新
	now := time.Now()
	if req.Name != nil {
		kb.Name = *req.Name
	}
	if req.Description != nil {
		kb.Description = *req.Description
	}
	if req.Avatar != nil {
		kb.Avatar = *req.Avatar
	}
	if req.IsPublic != nil {
		kb.IsPublic = *req.IsPublic
	}
	if req.Status != nil {
		kb.Status = *req.Status
	}
	kb.UpdatedAt = now

	// 保存更新
	if err := s.kbRepo.Update(ctx, kb); err != nil {
		return nil, err
	}

	return kb, nil
}

// UpdateWithSettings updates a knowledge base and its settings
func (s *knowledgeBaseService) UpdateWithSettings(ctx context.Context, kb *domain_knowledge.KnowledgeBase, setting *domain_knowledge.KnowledgeBaseSetting) error {
	// 更新知识库
	if err := s.kbRepo.Update(ctx, kb); err != nil {
		return err
	}

	// 如果提供了设置，更新设置记录
	if setting != nil {
		setting.KnowledgeBaseID = kb.ID
		if setting.ID == 0 {
			// 检查设置是否存在
			exists, err := s.kbSettingRepo.Exists(ctx, kb.ID)
			if err != nil {
				return err
			}
			if !exists {
				return s.kbSettingRepo.Create(ctx, setting)
			}
		}
		if err := s.kbSettingRepo.Update(ctx, setting); err != nil {
			return err
		}
	}

	return nil
}

// Delete soft deletes a knowledge base
func (s *knowledgeBaseService) Delete(ctx context.Context, id string) error {
	return s.kbRepo.Delete(ctx, id)
}

// Exists checks if a knowledge base exists
func (s *knowledgeBaseService) Exists(ctx context.Context, id string) (bool, error) {
	return s.kbRepo.Exists(ctx, id)
}

// GetStats gets statistics for a knowledge base (delegates to KnowledgeStatsQuerier)
func (s *knowledgeBaseService) GetStats(ctx context.Context, kbID string) (*domain_knowledge.KnowledgeBaseStats, error) {
	if s.statsQuerier == nil {
		// Fallback: return empty stats
		return &domain_knowledge.KnowledgeBaseStats{
			KnowledgeBaseID: kbID,
		}, nil
	}
	return s.statsQuerier.GetStats(ctx, kbID)
}

// GetKnowledgeList gets knowledge list for a KB (delegates to KnowledgeStatsQuerier)
func (s *knowledgeBaseService) GetKnowledgeList(
	ctx context.Context,
	kbID string,
	page, pageSize int,
	status string,
) ([]*domain_knowledge.Knowledge, int64, error) {
	if s.statsQuerier == nil {
		// Fallback: use knowledge repository
		query := &domain_knowledge.KnowledgeListQuery{
			Page:     page,
			PageSize: pageSize,
		}
		return s.knowledgeRepo.FindByKnowledgeBaseID(ctx, kbID, query)
	}
	return s.statsQuerier.GetKnowledgeList(ctx, kbID, page, pageSize, status)
}

// GetKnowledgeListWithStatus gets knowledge list with multiple statuses for a KB
func (s *knowledgeBaseService) GetKnowledgeListWithStatus(
	ctx context.Context,
	kbID string,
	page, pageSize int,
	statuses []string,
) ([]*domain_knowledge.Knowledge, int64, error) {
	if s.statsQuerier == nil {
		// Fallback: use knowledge repository with filter
		query := &domain_knowledge.KnowledgeListQuery{
			Page:     page,
			PageSize: pageSize,
		}
		// 过滤多个状态
		allResults, _, err := s.knowledgeRepo.FindByKnowledgeBaseID(ctx, kbID, query)
		if err != nil {
			return nil, 0, err
		}
		// 手动过滤状态
		statusMap := make(map[string]bool)
		for _, s := range statuses {
			statusMap[s] = true
		}
		filtered := make([]*domain_knowledge.Knowledge, 0)
		for _, k := range allResults {
			if statusMap[k.ParseStatus] {
				filtered = append(filtered, k)
			}
		}
		total := int64(len(filtered))
		// 分页
		start := (page - 1) * pageSize
		end := start + pageSize
		if int64(start) > total {
			return []*domain_knowledge.Knowledge{}, total, nil
		}
		if int64(end) > total {
			end = int(total)
		}
		return filtered[start:end], total, nil
	}
	return s.statsQuerier.GetKnowledgeListWithStatus(ctx, kbID, page, pageSize, statuses)
}

// DeleteKnowledge deletes knowledge from a KB across all stores (MySQL + Milvus + Neo4j)
func (s *knowledgeBaseService) DeleteKnowledge(ctx context.Context, kbID, knowledgeID string, tenantID int64) error {
	if s.statsQuerier == nil {
		// Fallback: simple delete through knowledge repository
		// Note: This won't delete chunks, so statsQuerier should be provided
		return s.knowledgeRepo.Delete(ctx, knowledgeID)
	}

	// Delete from MySQL (knowledge and chunks)
	if err := s.statsQuerier.DeleteKnowledgeWithChunks(ctx, kbID, knowledgeID); err != nil {
		return err
	}

	// Delete from Milvus (vectors) —— 尽力而为，MySQL 已删除，向量删除失败不回滚。
	// 注意：collection 名由 kbID 的前导数字派生（与写入侧 fmt.Sscanf 一致），
	// KB ID 形如 "20260704...bfa0e3" 含十六进制字母，不能用 strconv.ParseInt（会整体失败）。
	if s.vectorRepo != nil {
		var kbIDInt int64
		if kbID != "" {
			fmt.Sscanf(kbID, "%d", &kbIDInt)
		}
		if err := s.vectorRepo.DeleteByKnowledgeID(ctx, kbIDInt, knowledgeID); err != nil {
			log.Printf("[KnowledgeBaseService] Warning: failed to delete vectors for knowledge %s: %v", knowledgeID, err)
		}
	}

	// Delete from Neo4j (graph nodes/relations) —— 尽力而为。namespace 与写入侧
	// document_processor_service.extractGraph 保持一致：TenantID 为字符串化的租户 ID。
	if s.graphRepo != nil {
		namespace := domain_knowledge.NameSpace{
			TenantID:        fmt.Sprintf("%d", tenantID),
			KnowledgeBaseID: kbID,
			Knowledge:       knowledgeID,
		}
		if err := s.graphRepo.DeleteByKnowledgeID(ctx, namespace, knowledgeID); err != nil {
			log.Printf("[KnowledgeBaseService] Warning: failed to delete graph data for knowledge %s: %v", knowledgeID, err)
		}
	}

	return nil
}

// GetChunks gets chunks for a KB
func (s *knowledgeBaseService) GetChunks(
	ctx context.Context,
	kbID string,
	page, pageSize int,
	knowledgeID string,
) ([]*domain_knowledge.Chunk, int64, error) {
	// 使用仓储查询
	query := &domain_knowledge.ChunkListQuery{
		KnowledgeBaseID: kbID,
		KnowledgeID:     knowledgeID,
		Page:            page,
		PageSize:        pageSize,
	}
	return s.chunkRepo.FindByKnowledgeBaseID(ctx, kbID, query)
}

// CreateChunk creates a chunk
func (s *knowledgeBaseService) CreateChunk(ctx context.Context, chunk *domain_knowledge.Chunk) error {
	return s.chunkRepo.Create(ctx, chunk)
}

// ========================================
// Document Upload and Processing
// ========================================

// UploadDocument 上传文档文件（同步创建记录，立即可在列表中显示）
func (s *knowledgeBaseService) UploadDocument(
	ctx context.Context,
	kbID string,
	tenantID, userID int64,
	fileName, fileType string,
	fileSize int64,
	filePath, fileHash string,
) (*domain_knowledge.Knowledge, error) {
	// 生成知识条目 ID
	knowledgeID := s.idGenerator.Generate()

	docType := fileType
	if docType == "" {
		docType = "document"
	}

	// 创建知识条目：状态置为 processing，前端上传后即可在列表看到并轮询进度
	now := time.Now()
	knowledge := &domain_knowledge.Knowledge{
		ID:              knowledgeID,
		TenantID:        tenantID,
		KnowledgeBaseID: kbID,
		UserID:          userID,
		Type:            docType,
		Title:           fileName,
		Source:          "upload",
		ParseStatus:     domain_knowledge.ParseStatusProcessing,
		EnableStatus:    domain_knowledge.EnableStatusDisabled,
		FilePath:        filePath,
		FileHash:        fileHash,
		StorageSize:     fileSize,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.knowledgeRepo.Create(ctx, knowledge); err != nil {
		return nil, err
	}

	return knowledge, nil
}

// CheckDuplicateByHash 在知识库范围内检查是否已存在相同文件（防止重传相同文件）
func (s *knowledgeBaseService) CheckDuplicateByHash(ctx context.Context, kbID, fileHash string) (*domain_knowledge.Knowledge, error) {
	if fileHash == "" {
		return nil, nil
	}
	return s.knowledgeRepo.FindByFileHashInKB(ctx, kbID, fileHash)
}

// UpdateParseStatus 更新文档解析状态
func (s *knowledgeBaseService) UpdateParseStatus(ctx context.Context, id, parseStatus, errorMessage string) error {
	return s.knowledgeRepo.UpdateParseStatus(ctx, id, parseStatus, errorMessage)
}

// GetKnowledgeDetail 获取文档详情
func (s *knowledgeBaseService) GetKnowledgeDetail(ctx context.Context, kbID, knowledgeID string) (*domain_knowledge.Knowledge, error) {
	knowledge, err := s.knowledgeRepo.FindByID(ctx, knowledgeID)
	if err != nil {
		return nil, err
	}

	// 验证是否属于指定的知识库
	if knowledge.KnowledgeBaseID != kbID {
		return nil, domain_knowledge.ErrKnowledgeNotFound
	}

	return knowledge, nil
}

// GetKnowledgeStatus 获取文档处理状态
func (s *knowledgeBaseService) GetKnowledgeStatus(ctx context.Context, kbID, knowledgeID string) (*domain_knowledge.Knowledge, error) {
	return s.GetKnowledgeDetail(ctx, kbID, knowledgeID)
}

// ========================================
// Chunk Operations
// ========================================

// GetChunkDetail 获取分块详情
func (s *knowledgeBaseService) GetChunkDetail(ctx context.Context, kbID, chunkID string) (*domain_knowledge.Chunk, error) {
	chunk, err := s.chunkRepo.FindByID(ctx, chunkID)
	if err != nil {
		return nil, err
	}

	// 验证是否属于指定的知识库
	if chunk.KnowledgeBaseID != kbID {
		return nil, domain_knowledge.ErrChunkNotFound
	}

	return chunk, nil
}

// UpdateChunk 更新分块
func (s *knowledgeBaseService) UpdateChunk(
	ctx context.Context,
	kbID, chunkID string,
	content *string,
) (*domain_knowledge.Chunk, error) {
	chunk, err := s.GetChunkDetail(ctx, kbID, chunkID)
	if err != nil {
		return nil, err
	}

	if content != nil {
		chunk.Content = *content
	}

	now := time.Now()
	chunk.UpdatedAt = now

	if err := s.chunkRepo.Update(ctx, chunk); err != nil {
		return nil, err
	}

	return chunk, nil
}

// DeleteChunk 删除分块
func (s *knowledgeBaseService) DeleteChunk(ctx context.Context, kbID, chunkID string) error {
	// 先验证分块是否存在且属于指定知识库
	_, err := s.GetChunkDetail(ctx, kbID, chunkID)
	if err != nil {
		return err
	}

	return s.chunkRepo.Delete(ctx, chunkID)
}

// ========================================
// Search Operations
// ========================================

// Search 搜索知识库
func (s *knowledgeBaseService) Search(
	ctx context.Context,
	kbIDs []string,
	query string,
	topK int,
	minScore float64,
) ([]*domain_knowledge.Chunk, error) {
	// 获取启用的分块
	var allChunks []*domain_knowledge.Chunk
	for _, kbID := range kbIDs {
		chunks, _, err := s.chunkRepo.FindByKnowledgeBaseID(ctx, kbID, &domain_knowledge.ChunkListQuery{
			KnowledgeBaseID: kbID,
			IsEnabled:       boolPtr(true),
			Page:            1,
			PageSize:        topK * len(kbIDs), // 获取足够的结果用于过滤
		})
		if err != nil {
			continue
		}
		allChunks = append(allChunks, chunks...)
	}

	// 简单的文本匹配（后续可以集成向量检索）
	var results []*domain_knowledge.Chunk
	for _, chunk := range allChunks {
		if containsIgnoreCase(chunk.Content, query) {
			results = append(results, chunk)
			if len(results) >= topK {
				break
			}
		}
	}

	return results, nil
}

// boolPtr 返回 bool 指针的辅助函数
func boolPtr(b bool) *bool {
	return &b
}

// containsIgnoreCase 忽略大小写的字符串包含检查
func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 &&
			(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
				containsMiddle(s, substr))))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ========================================
// DTO Conversion Methods (Domain Entity -> Response DTO)
// ========================================

// ToResponse converts a domain entity to response DTO
func (s *knowledgeBaseService) ToResponse(entity *domain_knowledge.KnowledgeBase) *KnowledgeBaseResponse {
	if entity == nil {
		return nil
	}

	resp := &KnowledgeBaseResponse{
		ID:            entity.ID,
		TenantID:      entity.TenantID,
		UserID:        entity.UserID,
		Name:          entity.Name,
		Description:   entity.Description,
		Avatar:        entity.Avatar,
		DocumentCount: entity.DocumentCount,
		ChunkCount:    entity.ChunkCount,
		StorageSize:   entity.StorageSize,
		Status:        entity.Status,
		IsPublic:      entity.IsPublic,
		CreatedAt:     entity.CreatedAt.Unix(),
		UpdatedAt:     entity.UpdatedAt.Unix(),
	}

	if entity.Setting != nil {
		resp.Setting = s.ToSettingResponse(entity.Setting)
	}

	return resp
}

// ToResponseList converts a list of domain entities to response DTOs
func (s *knowledgeBaseService) ToResponseList(entities []*domain_knowledge.KnowledgeBase) []*KnowledgeBaseResponse {
	if entities == nil {
		return nil
	}

	responses := make([]*KnowledgeBaseResponse, len(entities))
	for i, entity := range entities {
		responses[i] = s.ToResponse(entity)
	}
	return responses
}

// ToSettingResponse converts a domain setting entity to response DTO
func (s *knowledgeBaseService) ToSettingResponse(setting *domain_knowledge.KnowledgeBaseSetting) *KnowledgeBaseSettingResponse {
	if setting == nil {
		return nil
	}

	bm25Enabled := false
	if setting.BM25Enabled != nil {
		bm25Enabled = *setting.BM25Enabled
	}

	chunkingConfig := ""
	if setting.ChunkingConfig != nil {
		chunkingConfig = *setting.ChunkingConfig
	}

	settingsJSON := ""
	if setting.SettingsJSON != nil {
		settingsJSON = *setting.SettingsJSON
	}

	return &KnowledgeBaseSettingResponse{
		ID:              setting.ID,
		KnowledgeBaseID: setting.KnowledgeBaseID,
		GraphEnabled:    setting.GraphEnabled,
		BM25Enabled:     bm25Enabled,
		ChunkingConfig:  chunkingConfig,
		SettingsJSON:    settingsJSON,
		UpdatedAt:       setting.UpdatedAt.Unix(),
	}
}
