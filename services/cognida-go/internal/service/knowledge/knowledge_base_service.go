// Package kb provides Knowledge Base service implementations
package knowledge

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"cognida/internal/infrastructure/id"
	domain_knowledge "cognida/internal/model/knowledge"
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

// KBCacheInvalidator 是 RAG 硬语义缓存对知识库服务暴露的窄失效接口
// （由 infrastructure/cache.SemanticCacheService 实现）。知识库内容一旦变更（删库/删知识/删分块），
// 该库沉淀的硬缓存答案即失效，须按 kbID 精确清除，避免旧知识以缓存形式续命。
// 可选依赖：未接线（nil）时所有失效点恒等跳过，零回归。
type KBCacheInvalidator interface {
	InvalidateByKB(ctx context.Context, tenantID int64, kbID string) error
}

// knowledgeBaseService implements KnowledgeBaseService
type knowledgeBaseService struct {
	kbRepo           domain_knowledge.KnowledgeBaseRepository
	kbSettingRepo    domain_knowledge.KnowledgeBaseSettingRepository
	knowledgeRepo    domain_knowledge.KnowledgeRepository
	chunkRepo        domain_knowledge.ChunkRepository
	statsQuerier     domain_knowledge.KnowledgeStatsQuerier
	vectorRepo       domain_knowledge.VectorRepository
	graphRepo        domain_knowledge.GraphRepository
	idGenerator      id.IDGenerator
	cacheInvalidator KBCacheInvalidator // 可选：RAG 硬缓存按库失效；nil 时跳过
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

// SetCacheInvalidator 注入 RAG 硬缓存按库失效器（组合根接线，可选）。
// 传 nil 或不调用时，内容变更后不做缓存失效——零回归。
func (s *knowledgeBaseService) SetCacheInvalidator(inv KBCacheInvalidator) {
	s.cacheInvalidator = inv
}

// invalidateKBCache 在知识库内容变更后清除该库的 RAG 硬缓存（尽力而为，失败仅告警不阻断主流程）。
// 未注入失效器（nil）时恒等跳过。
func (s *knowledgeBaseService) invalidateKBCache(ctx context.Context, tenantID int64, kbID string) {
	if s.cacheInvalidator == nil || kbID == "" {
		return
	}
	if err := s.cacheInvalidator.InvalidateByKB(ctx, tenantID, kbID); err != nil {
		log.Printf("[KnowledgeBaseService] Warning: failed to invalidate RAG cache for kb %s: %v", kbID, err)
	}
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

// requireKB 强制校验知识库归属（防跨租户 IDOR）：
// SQL 层 WHERE id=? AND tenant_id=?，跨租户与不存在统一返回“知识库不存在”，不泄露资源存在性。
func (s *knowledgeBaseService) requireKB(ctx context.Context, kbID string, tenantID int64) (*domain_knowledge.KnowledgeBase, error) {
	return s.kbRepo.FindByIDForTenant(ctx, kbID, tenantID)
}

// FindByID finds a knowledge base by ID（tenantID 强制归属校验）
func (s *knowledgeBaseService) FindByID(ctx context.Context, id string, tenantID int64) (*domain_knowledge.KnowledgeBase, error) {
	return s.requireKB(ctx, id, tenantID)
}

// FindByIDWithSettings finds a knowledge base with settings loaded（tenantID 强制归属校验）
func (s *knowledgeBaseService) FindByIDWithSettings(ctx context.Context, id string, tenantID int64) (*domain_knowledge.KnowledgeBase, error) {
	kb, err := s.requireKB(ctx, id, tenantID)
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

// UpdateFromRequest 从 DTO 更新知识库（tenantID 强制归属校验）
func (s *knowledgeBaseService) UpdateFromRequest(
	ctx context.Context,
	id string,
	tenantID int64,
	req *UpdateKnowledgeBaseRequest,
) (*domain_knowledge.KnowledgeBase, error) {
	// 获取现有实体（强制归属校验）
	kb, err := s.requireKB(ctx, id, tenantID)
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

	// 图谱开关属于库级数据处理配置，允许建库后在设置页开关，持久化到 kb_settings
	if req.GraphEnabled != nil {
		if err := s.applyGraphEnabled(ctx, id, *req.GraphEnabled, now); err != nil {
			return nil, err
		}
	}

	// 回填最新设置，保证响应包含 setting（图谱开关等），供前端设置页回显
	if setting, ferr := s.kbSettingRepo.FindByKnowledgeBaseID(ctx, id); ferr == nil {
		kb.Setting = setting
	}

	return kb, nil
}

// applyGraphEnabled 更新（或在缺失时创建）知识库的图谱提取开关
func (s *knowledgeBaseService) applyGraphEnabled(ctx context.Context, kbID string, enabled bool, now time.Time) error {
	setting, err := s.kbSettingRepo.FindByKnowledgeBaseID(ctx, kbID)
	if err == nil && setting != nil {
		setting.GraphEnabled = enabled
		setting.UpdatedAt = now
		return s.kbSettingRepo.Update(ctx, setting)
	}
	// 设置不存在则创建
	return s.kbSettingRepo.Create(ctx, &domain_knowledge.KnowledgeBaseSetting{
		KnowledgeBaseID: kbID,
		GraphEnabled:    enabled,
		CreatedAt:       now,
		UpdatedAt:       now,
	})
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

// Delete soft deletes a knowledge base（tenantID 强制归属校验）
func (s *knowledgeBaseService) Delete(ctx context.Context, id string, tenantID int64) error {
	if _, err := s.requireKB(ctx, id, tenantID); err != nil {
		return err
	}
	if err := s.kbRepo.Delete(ctx, id); err != nil {
		return err
	}
	s.invalidateKBCache(ctx, tenantID, id) // 库删除 → 清该库硬缓存
	return nil
}

// Exists checks if a knowledge base exists
func (s *knowledgeBaseService) Exists(ctx context.Context, id string) (bool, error) {
	return s.kbRepo.Exists(ctx, id)
}

// GetStats gets statistics for a knowledge base (delegates to KnowledgeStatsQuerier)（tenantID 强制归属校验）
func (s *knowledgeBaseService) GetStats(ctx context.Context, kbID string, tenantID int64) (*domain_knowledge.KnowledgeBaseStats, error) {
	if _, err := s.requireKB(ctx, kbID, tenantID); err != nil {
		return nil, err
	}
	if s.statsQuerier == nil {
		// Fallback: return empty stats
		return &domain_knowledge.KnowledgeBaseStats{
			KnowledgeBaseID: kbID,
		}, nil
	}
	return s.statsQuerier.GetStats(ctx, kbID)
}

// GetKnowledgeList gets knowledge list for a KB (delegates to KnowledgeStatsQuerier)（tenantID 强制归属校验）
func (s *knowledgeBaseService) GetKnowledgeList(
	ctx context.Context,
	kbID string,
	tenantID int64,
	page, pageSize int,
	status string,
) ([]*domain_knowledge.Knowledge, int64, error) {
	if _, err := s.requireKB(ctx, kbID, tenantID); err != nil {
		return nil, 0, err
	}
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

// GetKnowledgeListWithStatus gets knowledge list with multiple statuses for a KB（tenantID 强制归属校验）
func (s *knowledgeBaseService) GetKnowledgeListWithStatus(
	ctx context.Context,
	kbID string,
	tenantID int64,
	page, pageSize int,
	statuses []string,
) ([]*domain_knowledge.Knowledge, int64, error) {
	if _, err := s.requireKB(ctx, kbID, tenantID); err != nil {
		return nil, 0, err
	}
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
	// 强制归属校验：知识库必须属于当前租户
	if _, err := s.requireKB(ctx, kbID, tenantID); err != nil {
		return err
	}
	if s.statsQuerier == nil {
		// Fallback: simple delete through knowledge repository
		// Note: This won't delete chunks, so statsQuerier should be provided
		if err := s.knowledgeRepo.Delete(ctx, knowledgeID); err != nil {
			return err
		}
		s.invalidateKBCache(ctx, tenantID, kbID) // 知识删除 → 清该库硬缓存
		return nil
	}

	// 一致性策略（KB-6/KB-7）：MySQL 是权威库，Milvus/Neo4j 是可从 MySQL 重建的投影。
	// 旧代码"先删 MySQL，投影删除失败仅 log warning 不回滚" → 孤儿向量/图谱仍被检索命中，
	// 却对外报删除成功。改为：① 先取删除图谱所需的 chunk_id 集合（MySQL 删除后即不可得）；
	// ② 先删投影（Milvus + Neo4j），任一失败即中止并返回错误——此时 MySQL 记录仍在、
	// 文档可见，可安全重试（各步幂等）；③ 投影删净后再删 MySQL 权威记录。
	// 彻底的跨存储原子性需 outbox + 对账（见 review 参考模式 B），此处为低风险收敛版。

	// ① 取该文档全部分块 id（含禁用），用于按 chunk_id 精确清理图谱
	var chunkIDs []string
	if s.graphRepo != nil && s.chunkRepo != nil {
		chunks, cerr := s.chunkRepo.FindByKnowledgeID(ctx, knowledgeID, false)
		if cerr != nil {
			return fmt.Errorf("加载分块以清理图谱失败: %w", cerr)
		}
		chunkIDs = make([]string, 0, len(chunks))
		for _, c := range chunks {
			chunkIDs = append(chunkIDs, c.ID)
		}
	}

	// ② 先删投影 · Milvus 向量（按 knowledge_id，向量行带该字段，可靠且幂等）。
	// collection 名由 kbID 前导数字派生（与写入侧 fmt.Sscanf 一致），KB ID 形如
	// "20260704...bfa0e3" 含十六进制字母，不能用 strconv.ParseInt（会整体失败）。
	if s.vectorRepo != nil {
		var kbIDInt int64
		if kbID != "" {
			_, _ = fmt.Sscanf(kbID, "%d", &kbIDInt) // 解析失败保持零值
		}
		if err := retryTransientDelete(ctx, func() error {
			return s.vectorRepo.DeleteByKnowledgeID(ctx, kbIDInt, knowledgeID)
		}); err != nil {
			return fmt.Errorf("删除向量失败，已中止（MySQL 未删，可重试）: %w", err)
		}
	}

	// ② 先删投影 · Neo4j 图谱（按 chunk_id 集合——节点存 chunk_id 而非 knowledge_id，
	// 旧的 DeleteByKnowledgeID 恒不匹配、永远清不掉，KB-7）。namespace 与写入侧
	// document_processor_service.extractGraph 一致：TenantID 为字符串化的租户 ID。
	if s.graphRepo != nil && len(chunkIDs) > 0 {
		namespace := domain_knowledge.NameSpace{
			TenantID:        fmt.Sprintf("%d", tenantID),
			KnowledgeBaseID: kbID,
			Knowledge:       knowledgeID,
		}
		if err := retryTransientDelete(ctx, func() error {
			return s.graphRepo.DeleteByChunkIDs(ctx, namespace, chunkIDs)
		}); err != nil {
			return fmt.Errorf("删除图谱失败，已中止（MySQL 未删，可重试）: %w", err)
		}
	}

	// ③ 投影删净后再删 MySQL 权威记录（knowledge + chunks）
	if err := s.statsQuerier.DeleteKnowledgeWithChunks(ctx, kbID, knowledgeID); err != nil {
		return err
	}
	s.invalidateKBCache(ctx, tenantID, kbID) // 知识删除 → 清该库硬缓存

	return nil
}

// SetKnowledgeEnabled 启用/停用某文档。MySQL 是启用状态的唯一权威源：更新 knowledge.enable_status
// 并级联 chunk.is_enabled，随后失效该库检索缓存。停用不触碰向量库/图谱——检索侧按 MySQL 权威后过滤
// 剔除停用命中（见 RetrievalCapability.filterByEnabled），无需回写向量或重嵌入。
func (s *knowledgeBaseService) SetKnowledgeEnabled(ctx context.Context, kbID, knowledgeID string, tenantID int64, enabled bool) error {
	// 强制归属校验：知识库必须属于当前租户
	if _, err := s.requireKB(ctx, kbID, tenantID); err != nil {
		return err
	}

	// 文档必须存在且属于本知识库（纵深防御：避免跨库改写他库文档状态）
	knowledge, err := s.knowledgeRepo.FindByID(ctx, knowledgeID)
	if err != nil {
		return err
	}
	if knowledge.KnowledgeBaseID != kbID {
		return fmt.Errorf("知识条目不属于该知识库")
	}

	enableStatus := domain_knowledge.EnableStatusDisabled
	if enabled {
		enableStatus = domain_knowledge.EnableStatusEnabled
	}

	// ① 更新文档权威启用状态
	if err := s.knowledgeRepo.UpdateEnableStatus(ctx, knowledgeID, enableStatus); err != nil {
		return fmt.Errorf("更新文档启用状态失败: %w", err)
	}

	// ② 级联更新分块 is_enabled（含当前禁用分块，enabledOnly=false 取全量 id）。
	// 分块状态是文档状态的从属投影，保持二者一致；检索后过滤按文档 knowledge_id 判定，
	// 分块 is_enabled 供片段级检索/统计使用。
	if s.chunkRepo != nil {
		chunks, cerr := s.chunkRepo.FindByKnowledgeID(ctx, knowledgeID, false)
		if cerr != nil {
			return fmt.Errorf("加载分块以级联启用状态失败: %w", cerr)
		}
		if len(chunks) > 0 {
			ids := make([]string, 0, len(chunks))
			for _, c := range chunks {
				ids = append(ids, c.ID)
			}
			if err := s.chunkRepo.UpdateBatchStatus(ctx, ids, enabled); err != nil {
				return fmt.Errorf("级联分块启用状态失败: %w", err)
			}
		}
	}

	s.invalidateKBCache(ctx, tenantID, kbID) // 启用状态变更 → 清该库硬缓存，避免命中旧结果
	return nil
}

// deleteRetryAttempts / deleteRetryBaseBackoff 控制投影删除的有界重试〔#8〕。
const (
	deleteRetryAttempts    = 3
	deleteRetryBaseBackoff = 100 * time.Millisecond
)

// retryTransientDelete 对幂等的投影删除（Milvus / Neo4j）做有界指数退避重试，吸收瞬时抖动
// （网络闪断、下游 GC 暂停）——一次短暂不可用不至于让「先删投影再删 MySQL」的删除彻底失败〔#8〕。
// 这是在「一致性优先」前提下改善可用性：投影删净前绝不删 MySQL 权威记录，故不会产生可被检索命中
// 的孤儿向量/图谱（检索直接读 Milvus/Neo4j、不回查 MySQL）；重试仍不成功才硬失败，此时 MySQL
// 未删、各步幂等、可整体安全重试。ctx 取消时立即返回，不再空等退避。
func retryTransientDelete(ctx context.Context, op func() error) error {
	var err error
	for attempt := 0; attempt < deleteRetryAttempts; attempt++ {
		if err = op(); err == nil {
			return nil
		}
		if attempt == deleteRetryAttempts-1 {
			break
		}
		backoff := deleteRetryBaseBackoff * time.Duration(1<<attempt) // 100ms、200ms…
		log.Printf("[KnowledgeBase] 投影删除瞬时失败（第 %d/%d 次），%v 后重试: %v",
			attempt+1, deleteRetryAttempts, backoff, err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
	return err
}

// GetChunks gets chunks for a KB（tenantID 强制归属校验）
func (s *knowledgeBaseService) GetChunks(
	ctx context.Context,
	kbID string,
	tenantID int64,
	page, pageSize int,
	knowledgeID string,
) ([]*domain_knowledge.Chunk, int64, error) {
	if _, err := s.requireKB(ctx, kbID, tenantID); err != nil {
		return nil, 0, err
	}
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
	// 强制归属校验：知识库必须属于当前租户
	if _, err := s.requireKB(ctx, kbID, tenantID); err != nil {
		return nil, err
	}

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
		// 新入库文档默认启用，可被检索命中；停用是显式管理动作（见 SetKnowledgeEnabled）。
		EnableStatus:    domain_knowledge.EnableStatusEnabled,
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

// CheckDuplicateByHash 在知识库范围内检查是否已存在相同文件（防止重传相同文件）（tenantID 强制归属校验）
func (s *knowledgeBaseService) CheckDuplicateByHash(ctx context.Context, kbID string, tenantID int64, fileHash string) (*domain_knowledge.Knowledge, error) {
	if _, err := s.requireKB(ctx, kbID, tenantID); err != nil {
		return nil, err
	}
	if fileHash == "" {
		return nil, nil
	}
	return s.knowledgeRepo.FindByFileHashInKB(ctx, kbID, fileHash)
}

// UpdateParseStatus 更新文档解析状态
func (s *knowledgeBaseService) UpdateParseStatus(ctx context.Context, id, parseStatus, errorMessage string) error {
	return s.knowledgeRepo.UpdateParseStatus(ctx, id, parseStatus, errorMessage)
}

// GetKnowledgeDetail 获取文档详情（tenantID 强制归属校验）
func (s *knowledgeBaseService) GetKnowledgeDetail(ctx context.Context, kbID string, tenantID int64, knowledgeID string) (*domain_knowledge.Knowledge, error) {
	if _, err := s.requireKB(ctx, kbID, tenantID); err != nil {
		return nil, err
	}

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

// GetKnowledgeStatus 获取文档处理状态（tenantID 强制归属校验）
func (s *knowledgeBaseService) GetKnowledgeStatus(ctx context.Context, kbID string, tenantID int64, knowledgeID string) (*domain_knowledge.Knowledge, error) {
	return s.GetKnowledgeDetail(ctx, kbID, tenantID, knowledgeID)
}

// ========================================
// Chunk Operations
// ========================================

// GetChunkDetail 获取分块详情（tenantID 强制归属校验）
func (s *knowledgeBaseService) GetChunkDetail(ctx context.Context, kbID string, tenantID int64, chunkID string) (*domain_knowledge.Chunk, error) {
	if _, err := s.requireKB(ctx, kbID, tenantID); err != nil {
		return nil, err
	}

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

// UpdateChunk 更新分块（tenantID 强制归属校验）
func (s *knowledgeBaseService) UpdateChunk(
	ctx context.Context,
	kbID string,
	tenantID int64,
	chunkID string,
	content *string,
) (*domain_knowledge.Chunk, error) {
	chunk, err := s.GetChunkDetail(ctx, kbID, tenantID, chunkID)
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

// DeleteChunk 删除分块（tenantID 强制归属校验）
func (s *knowledgeBaseService) DeleteChunk(ctx context.Context, kbID string, tenantID int64, chunkID string) error {
	// 先验证分块是否存在且属于指定知识库（含租户归属校验）
	_, err := s.GetChunkDetail(ctx, kbID, tenantID, chunkID)
	if err != nil {
		return err
	}

	if err := s.chunkRepo.Delete(ctx, chunkID); err != nil {
		return err
	}
	s.invalidateKBCache(ctx, tenantID, kbID) // 分块删除 → 清该库硬缓存
	return nil
}

// ========================================
// Search Operations
// ========================================

// FilterAccessibleKBIDs 过滤出属于该租户的知识库 ID（越权/陌生 ID 被静默剔除，保持入参顺序去重）。
// 检索本体已收敛到 knowledge.RetrievalCapability（向量/BM25/混合 + 去重 + 阈值 + 可插拔重排），
// 本方法只负责把「请求的知识库范围」压到「租户可访问范围」，供 REST 检索 handler 在调用能力前强制归属边界。
func (s *knowledgeBaseService) FilterAccessibleKBIDs(ctx context.Context, kbIDs []string, tenantID int64) []string {
	out := make([]string, 0, len(kbIDs))
	seen := make(map[string]bool, len(kbIDs))
	for _, kbID := range kbIDs {
		if seen[kbID] {
			continue
		}
		if _, err := s.requireKB(ctx, kbID, tenantID); err != nil {
			continue // 不属于当前租户的 kbID 直接剔除，不返回其内容
		}
		seen[kbID] = true
		out = append(out, kbID)
	}
	return out
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
