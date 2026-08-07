// Package handler 提供知识库的 HTTP 处理器
package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"link/internal/infrastructure/config"
	domain_knowledge "link/internal/model/knowledge"
	app_kb "link/internal/service/knowledge"
)

// ========================================
// KnowledgeBaseHandler 知识库处理器
// ========================================

// KnowledgeBaseHandler 知识库处理器
type KnowledgeBaseHandler struct {
	knowledgeBaseService app_kb.KnowledgeBaseService
	documentProcessor    app_kb.DocumentProcessorService
	retrieval            *app_kb.RetrievalCapability
	uploadConfig         *config.UploadConfig
}

// NewKnowledgeBaseHandler 创建知识库处理器
func NewKnowledgeBaseHandler(
	knowledgeBaseService app_kb.KnowledgeBaseService,
	documentProcessor app_kb.DocumentProcessorService,
	retrieval *app_kb.RetrievalCapability,
) *KnowledgeBaseHandler {
	return &KnowledgeBaseHandler{
		knowledgeBaseService: knowledgeBaseService,
		documentProcessor:    documentProcessor,
		retrieval:            retrieval,
		uploadConfig:         config.LoadUploadConfig(),
	}
}

// SetUploadConfig 设置上传配置（用于测试）
func (h *KnowledgeBaseHandler) SetUploadConfig(uploadConfig *config.UploadConfig) {
	h.uploadConfig = uploadConfig
}

// ========================================
// KnowledgeBase CRUD 接口
// ========================================

// CreateKnowledgeBase 创建知识库
// @Summary 创建知识库
// @Description 创建新的知识库
// @Tags knowledge-base
// @Accept json
// @Produce json
// @Router /api/v1/knowledge-bases [post]
func (h *KnowledgeBaseHandler) CreateKnowledgeBase(c *gin.Context) {
	var req app_kb.CreateKnowledgeBaseRequest
	if !BindJSON(c, &req) {
		return
	}

	// 获取租户ID和用户ID
	tenantID := GetTenantID(c)
	userID := GetUserID(c)

	// 通过 Use Case 创建知识库（ID 在服务层生成）
	knowledgeBaseEntity, err := h.knowledgeBaseService.CreateFromRequest(c.Request.Context(), &req, tenantID, userID)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	// 使用 Service 层的转换方法
	resp := h.knowledgeBaseService.ToResponse(knowledgeBaseEntity)
	Created(c, resp)
}

// GetKnowledgeBase 获取知识库详情
// @Summary 获取知识库详情
// @Description 根据ID获取知识库详情
// @Tags knowledge-base
// @Router /api/v1/knowledge-bases/{id} [get]
func (h *KnowledgeBaseHandler) GetKnowledgeBase(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		BadRequest(c, "知识库ID不能为空")
		return
	}

	// 详情需带上 setting（图谱开关等库级数据处理配置），供前端设置页展示与编辑
	result, err := h.knowledgeBaseService.FindByIDWithSettings(c.Request.Context(), id, GetTenantID(c))
	if err != nil {
		NotFound(c, err.Error())
		return
	}

	// 使用 Service 层的转换方法
	resp := h.knowledgeBaseService.ToResponse(result)
	OK(c, resp)
}

// ListKnowledgeBases 列出知识库
// @Summary 列出知识库
// @Description 获取当前租户的知识库列表
// @Tags knowledge-base
// @Router /api/v1/knowledge-bases [get]
func (h *KnowledgeBaseHandler) ListKnowledgeBases(c *gin.Context) {
	page, pageSize := GetPageParams(c)

	tenantID := GetTenantID(c)

	result, total, err := h.knowledgeBaseService.FindByTenantID(c.Request.Context(), tenantID, page, pageSize)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	// 使用 Service 层的转换方法
	items := h.knowledgeBaseService.ToResponseList(result)
	PageSuccessJSON(c, total, items, page, pageSize)
}

// UpdateKnowledgeBase 更新知识库
// @Summary 更新知识库
// @Description 更新知识库信息
// @Tags knowledge-base
// @Router /api/v1/knowledge-bases/{id} [put]
func (h *KnowledgeBaseHandler) UpdateKnowledgeBase(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		BadRequest(c, "知识库ID不能为空")
		return
	}

	var req app_kb.UpdateKnowledgeBaseRequest
	if !BindJSON(c, &req) {
		return
	}

	// 通过 Use Case 更新知识库
	updatedKB, err := h.knowledgeBaseService.UpdateFromRequest(c.Request.Context(), id, GetTenantID(c), &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	// 使用 Service 层的转换方法
	resp := h.knowledgeBaseService.ToResponse(updatedKB)
	OK(c, resp)
}

// DeleteKnowledgeBase 删除知识库
// @Summary 删除知识库
// @Description 删除指定的知识库
// @Tags knowledge-base
// @Router /api/v1/knowledge-bases/{id} [delete]
func (h *KnowledgeBaseHandler) DeleteKnowledgeBase(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		BadRequest(c, "知识库ID不能为空")
		return
	}

	err := h.knowledgeBaseService.Delete(c.Request.Context(), id, GetTenantID(c))
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, map[string]interface{}{"message": "删除成功"})
}

// GetStats 获取知识库统计
// @Summary 获取知识库统计
// @Description 获取知识库的统计信息
// @Tags knowledge-base
// @Router /api/v1/knowledge-bases/{id}/stats [get]
func (h *KnowledgeBaseHandler) GetStats(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		BadRequest(c, "知识库ID不能为空")
		return
	}

	stats, err := h.knowledgeBaseService.GetStats(c.Request.Context(), id, GetTenantID(c))
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, map[string]interface{}{
		"kb_id":           stats.KnowledgeBaseID,
		"knowledge_count": stats.KnowledgeCount,
		"chunk_count":     stats.ChunkCount,
		"total_size":      stats.TotalSize,
	})
}

// RebuildGraph 为知识库补建知识图谱
// @Summary 为知识库补建知识图谱
// @Description 复用已存储分块，为库内所有已完成解析的历史文档重新提取知识图谱（幂等）
// @Tags knowledge-base
// @Router /api/v1/knowledge-bases/{id}/graph/rebuild [post]
func (h *KnowledgeBaseHandler) RebuildGraph(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		BadRequest(c, "知识库ID不能为空")
		return
	}

	tenantID := GetTenantID(c)

	// 校验归属并确认库级图谱开关已开启
	kb, err := h.knowledgeBaseService.FindByIDWithSettings(c.Request.Context(), id, tenantID)
	if err != nil {
		NotFound(c, err.Error())
		return
	}
	if kb.Setting == nil || !kb.Setting.GraphEnabled {
		BadRequest(c, "请先在设置页开启「构建知识图谱」后再补建")
		return
	}

	result, err := h.documentProcessor.RebuildKnowledgeBaseGraph(c.Request.Context(), tenantID, id)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, result)
}

// GetKnowledgeList 获取知识库文档列表
// @Summary 获取知识库文档列表
// @Description 获取指定知识库的文档列表
// @Tags knowledge-base
// @Router /api/v1/knowledge-bases/{id}/knowledge [get]
func (h *KnowledgeBaseHandler) GetKnowledgeList(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		BadRequest(c, "知识库ID不能为空")
		return
	}

	page, pageSize := GetPageParams(c)
	status := c.DefaultQuery("status", "")

	result, total, err := h.knowledgeBaseService.GetKnowledgeList(c.Request.Context(), id, GetTenantID(c), page, pageSize, status)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	// 转换为响应格式
	items := make([]map[string]interface{}, len(result))
	for i, k := range result {
		items[i] = map[string]interface{}{
			"id":            k.ID,
			"kb_id":         k.KnowledgeBaseID,
			"title":         k.Title,
			"type":          k.Type,
			"storage_size":  k.StorageSize,
			"file_path":     k.FilePath,
			"parse_status":  k.ParseStatus,
			"enable_status": k.EnableStatus,
			"chunk_count":   k.ChunkCount,
			"created_at":    k.CreatedAt.Unix(),
			"processed_at":  FormatTime(k.ProcessedAt),
		}
	}

	PageSuccessJSON(c, total, items, page, pageSize)
}

// DeleteKnowledge 删除知识库文档
// @Summary 删除知识库文档
// @Description 删除指定知识库的文档
// @Tags knowledge-base
// @Router /api/v1/knowledge-bases/{kb_id}/knowledge/{knowledge_id} [delete]
func (h *KnowledgeBaseHandler) DeleteKnowledge(c *gin.Context) {
	knowledgeBaseID := c.Param("id")
	knowledgeID := c.Param("knowledge_id")

	if knowledgeBaseID == "" {
		BadRequest(c, "知识库ID不能为空")
		return
	}
	if knowledgeID == "" {
		BadRequest(c, "文档ID不能为空")
		return
	}

	tenantID := GetTenantID(c)
	err := h.knowledgeBaseService.DeleteKnowledge(c.Request.Context(), knowledgeBaseID, knowledgeID, tenantID)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, map[string]interface{}{"message": "删除成功"})
}

// UploadKnowledgeFile 上传知识库文件
// @Summary 上传知识库文件
// @Description 上传文档文件到知识库
// @Tags knowledge-base
// @Router /api/v1/knowledge-bases/{id}/knowledge/file [post]
func (h *KnowledgeBaseHandler) UploadKnowledgeFile(c *gin.Context) {
	knowledgeBaseID := c.Param("id")
	if knowledgeBaseID == "" {
		BadRequest(c, "知识库ID不能为空")
		return
	}

	// 获取租户ID和用户ID
	tenantID := GetTenantID(c)
	userID := GetUserID(c)

	// 解析表单
	fileHeader, err := c.FormFile("file")
	if err != nil {
		BadRequest(c, "文件不能为空: "+err.Error())
		return
	}

	// 验证文件大小 (50MB)
	const maxFileSize = 50 * 1024 * 1024
	if fileHeader.Size > maxFileSize {
		BadRequest(c, "文件大小超过限制(50MB)")
		return
	}

	// 计算文件内容哈希（SHA-256），用于防止重传相同文件
	fileHash, err := hashUploadedFile(fileHeader)
	if err != nil {
		InternalError(c, "计算文件哈希失败: "+err.Error())
		return
	}

	// 服务端权威去重校验：同一知识库内已存在相同内容的文件则拒绝
	if dup, err := h.knowledgeBaseService.CheckDuplicateByHash(c.Request.Context(), knowledgeBaseID, tenantID, fileHash); err != nil {
		InternalError(c, "去重校验失败: "+err.Error())
		return
	} else if dup != nil {
		Conflict(c, "该文件已存在于知识库中，请勿重复上传", map[string]interface{}{
			"duplicate":    true,
			"knowledge_id": dup.ID,
			"title":        dup.Title,
		})
		return
	}

	// 生成唯一的文件名
	ext := filepath.Ext(fileHeader.Filename)
	savedFileName := fmt.Sprintf("%d_%d_%s%s", time.Now().Unix(), userID, knowledgeBaseID[:8], ext)
	savedFilePath := filepath.Join(h.uploadConfig.UploadDir, savedFileName)

	// 保存文件到磁盘
	if err := c.SaveUploadedFile(fileHeader, savedFilePath); err != nil {
		InternalError(c, "保存文件失败: "+err.Error())
		return
	}

	// 转换为绝对路径，确保 Python gRPC 服务能够找到文件
	absFilePath, err := filepath.Abs(savedFilePath)
	if err != nil {
		InternalError(c, "获取文件绝对路径失败: "+err.Error())
		return
	}

	// 同步创建知识条目记录（状态 processing），上传成功后前端即可在列表中看到并轮询进度
	knowledge, err := h.knowledgeBaseService.UploadDocument(
		c.Request.Context(),
		knowledgeBaseID,
		tenantID,
		userID,
		fileHeader.Filename,
		detectFileType(ext),
		fileHeader.Size,
		absFilePath,
		fileHash,
	)
	if err != nil {
		InternalError(c, "创建文档记录失败: "+err.Error())
		return
	}

	// 使用 DocumentProcessorService 异步处理文档
	// 创建独立的 context，避免 HTTP 请求返回后 context 被取消
	asyncCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	go func() {
		defer cancel()
		// goroutine 中的 panic 不会被 gin recovery 中间件捕获，必须自行兜底，否则整个进程崩溃
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[KnowledgeBaseHandler] Document processing panicked: %v\n%s", r, debug.Stack())
				// panic 后同样将记录标记为失败，避免永远停留在 processing
				h.markDocumentFailed(context.Background(), knowledge.ID)
			}
		}()
		h.processDocumentWithUseCase(asyncCtx, knowledgeBaseID, tenantID, userID, fileHeader.Filename, absFilePath, knowledge.ID)
	}()

	OK(c, map[string]interface{}{
		"knowledge_id": knowledge.ID,
		"status":       knowledge.ParseStatus,
		"title":        knowledge.Title,
		"message":      "文档正在处理中",
	})
}

// CheckKnowledgeFile 预检文件是否已存在于知识库（防止重传相同文件）
// 前端在真正上传字节前调用，命中则不再传输文件内容
// @Router /api/v1/knowledge-bases/{id}/knowledge/check [post]
func (h *KnowledgeBaseHandler) CheckKnowledgeFile(c *gin.Context) {
	knowledgeBaseID := c.Param("id")
	if knowledgeBaseID == "" {
		BadRequest(c, "知识库ID不能为空")
		return
	}

	var body struct {
		FileHash string `json:"file_hash"`
	}
	if !BindJSON(c, &body) {
		return
	}
	if body.FileHash == "" {
		BadRequest(c, "file_hash 不能为空")
		return
	}

	dup, err := h.knowledgeBaseService.CheckDuplicateByHash(c.Request.Context(), knowledgeBaseID, GetTenantID(c), body.FileHash)
	if err != nil {
		InternalError(c, "去重校验失败: "+err.Error())
		return
	}

	result := map[string]interface{}{"duplicate": dup != nil}
	if dup != nil {
		result["knowledge_id"] = dup.ID
		result["title"] = dup.Title
	}
	OK(c, result)
}

// hashUploadedFile 流式计算上传文件内容的 SHA-256 十六进制哈希
func hashUploadedFile(fileHeader *multipart.FileHeader) (string, error) {
	f, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// detectFileType 从扩展名推断文档类型
func detectFileType(ext string) string {
	e := strings.TrimPrefix(strings.ToLower(ext), ".")
	if e == "" {
		return "document"
	}
	return e
}

// markDocumentFailed 将文档记录标记为解析失败
func (h *KnowledgeBaseHandler) markDocumentFailed(ctx context.Context, knowledgeID string) {
	if knowledgeID == "" {
		return
	}
	if err := h.knowledgeBaseService.UpdateParseStatus(ctx, knowledgeID, domain_knowledge.ParseStatusFailed, ""); err != nil {
		log.Printf("[KnowledgeBaseHandler] Failed to mark document failed: %v", err)
	}
}

// processDocumentWithUseCase 使用 UseCase 处理文档
func (h *KnowledgeBaseHandler) processDocumentWithUseCase(ctx context.Context, knowledgeBaseID string, tenantID, userID int64, fileName, filePath, documentID string) {
	// 获取知识库配置以确定分块策略和是否启用图谱
	kb, err := h.knowledgeBaseService.FindByID(ctx, knowledgeBaseID, tenantID)
	if err != nil {
		log.Printf("[KnowledgeBaseHandler] Failed to get KB: %v", err)
		h.markDocumentFailed(ctx, documentID)
		return
	}

	chunkingConfig := getChunkingConfig(kb)
	graphEnabled := kb.Setting != nil && kb.Setting.GraphEnabled

	// 构建处理请求（复用同步创建的记录，避免重复建档）
	req := &app_kb.ProcessDocumentRequest{
		DocumentID:      documentID,
		FilePath:        filePath,
		KnowledgeBaseID: knowledgeBaseID,
		Title:           fileName,
		ChunkSize:       chunkingConfig.ChunkSize,
		ChunkOverlap:    chunkingConfig.ChunkOverlap,
		ChunkStrategy:   chunkingConfig.Strategy,
		GraphEnabled:    graphEnabled,
	}

	// 调用 UseCase 处理
	resp, err := h.documentProcessor.ProcessDocument(ctx, tenantID, userID, req)
	if err != nil {
		log.Printf("[KnowledgeBaseHandler] Document processing failed: %v", err)
		h.markDocumentFailed(ctx, documentID)
		return
	}

	log.Printf("[KnowledgeBaseHandler] Document processed: DocumentID=%s, Chunks=%d, Vectorized=%v, GraphExtracted=%v",
		resp.DocumentID, resp.ChunkCount, resp.Vectorized, resp.GraphExtracted)
}

// ChunkingConfig 分块配置
type ChunkingConfig struct {
	Strategy     string `json:"strategy"`
	ChunkSize    int    `json:"chunk_size"`
	ChunkOverlap int    `json:"chunk_overlap"`
}

// getChunkingConfig 从知识库设置中获取分块配置
func getChunkingConfig(kb *domain_knowledge.KnowledgeBase) ChunkingConfig {
	config := ChunkingConfig{
		Strategy:     "paragraph",
		ChunkSize:    500,
		ChunkOverlap: 50,
	}

	// 如果没有设置，返回默认值
	if kb.Setting == nil {
		return config
	}

	// 获取 JSON 配置字符串
	chunkingConfigJSON := kb.Setting.GetChunkingConfig()
	if chunkingConfigJSON == "" || chunkingConfigJSON == "{}" {
		return config
	}

	// 解析 JSON 配置
	var parsedConfig ChunkingConfig
	if err := json.Unmarshal([]byte(chunkingConfigJSON), &parsedConfig); err != nil {
		// 解析失败，使用默认值
		return config
	}

	// 验证和修正值
	if parsedConfig.Strategy == "" {
		parsedConfig.Strategy = "paragraph"
	}
	if parsedConfig.ChunkSize <= 0 {
		parsedConfig.ChunkSize = 500
	}
	if parsedConfig.ChunkOverlap < 0 {
		parsedConfig.ChunkOverlap = 50
	}

	return parsedConfig
}

// GetKnowledgeDetail 获取文档详情
// @Summary 获取文档详情
// @Description 获取指定知识库文档的详细信息
// @Tags knowledge-base
// @Router /api/v1/knowledge-bases/{id}/knowledge/{kid} [get]
func (h *KnowledgeBaseHandler) GetKnowledgeDetail(c *gin.Context) {
	knowledgeBaseID := c.Param("id")
	knowledgeID := c.Param("kid")

	if knowledgeBaseID == "" {
		BadRequest(c, "知识库ID不能为空")
		return
	}
	if knowledgeID == "" {
		BadRequest(c, "文档ID不能为空")
		return
	}

	knowledge, err := h.knowledgeBaseService.GetKnowledgeDetail(c.Request.Context(), knowledgeBaseID, GetTenantID(c), knowledgeID)
	if err != nil {
		NotFound(c, err.Error())
		return
	}

	OK(c, map[string]interface{}{
		"id":            knowledge.ID,
		"kb_id":         knowledge.KnowledgeBaseID,
		"title":         knowledge.Title,
		"type":          knowledge.Type,
		"description":   knowledge.Description,
		"source":        knowledge.Source,
		"parse_status":  knowledge.ParseStatus,
		"enable_status": knowledge.EnableStatus,
		"storage_size":  knowledge.StorageSize,
		"chunk_count": func() int {
			if knowledge.ChunkCount == nil {
				return 0
			}
			return *knowledge.ChunkCount
		}(),
		"file_path":    knowledge.FilePath,
		"file_name":    knowledge.Title,
		"file_type":    knowledge.Type,
		"created_at":   knowledge.CreatedAt.Unix(),
		"updated_at":   knowledge.UpdatedAt.Unix(),
		"processed_at": FormatTime(knowledge.ProcessedAt),
	})
}

// GetKnowledgeStatus 获取文档处理状态
// @Summary 获取文档处理状态
// @Description 获取文档的解析状态和处理进度
// @Tags knowledge-base
// @Router /api/v1/knowledge-bases/{id}/knowledge/{kid}/status [get]
func (h *KnowledgeBaseHandler) GetKnowledgeStatus(c *gin.Context) {
	knowledgeBaseID := c.Param("id")
	knowledgeID := c.Param("kid")

	if knowledgeBaseID == "" {
		BadRequest(c, "知识库ID不能为空")
		return
	}
	if knowledgeID == "" {
		BadRequest(c, "文档ID不能为空")
		return
	}

	knowledge, err := h.knowledgeBaseService.GetKnowledgeStatus(c.Request.Context(), knowledgeBaseID, GetTenantID(c), knowledgeID)
	if err != nil {
		NotFound(c, err.Error())
		return
	}

	OK(c, map[string]interface{}{
		"knowledge_id":  knowledge.ID,
		"parse_status":  knowledge.ParseStatus,
		"enable_status": knowledge.EnableStatus,
		"chunk_count": func() int {
			if knowledge.ChunkCount == nil {
				return 0
			}
			return *knowledge.ChunkCount
		}(),
		"processed_at": FormatTime(knowledge.ProcessedAt),
	})
}

// GetPendingKnowledgeList 获取待处理文档列表
// @Summary 获取待处理文档列表
// @Description 获取知识库中待处理（pending/processing）状态的文档列表
// @Tags knowledge-base
// @Router /api/v1/knowledge-bases/{id}/knowledge/pending [get]
func (h *KnowledgeBaseHandler) GetPendingKnowledgeList(c *gin.Context) {
	knowledgeBaseID := c.Param("id")
	if knowledgeBaseID == "" {
		BadRequest(c, "知识库ID不能为空")
		return
	}

	page, pageSize := GetPageParams(c)

	// 查询 pending 和 processing 状态的文档
	result, total, err := h.knowledgeBaseService.GetKnowledgeListWithStatus(c.Request.Context(), knowledgeBaseID, GetTenantID(c), page, pageSize, []string{"pending", "processing"})
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	items := make([]map[string]interface{}, len(result))
	for i, k := range result {
		items[i] = map[string]interface{}{
			"id":            k.ID,
			"kb_id":         k.KnowledgeBaseID,
			"title":         k.Title,
			"type":          k.Type,
			"description":   k.Description,
			"source":        k.Source,
			"parse_status":  k.ParseStatus,
			"enable_status": k.EnableStatus,
			"chunk_count": func() int {
				if k.ChunkCount == nil {
					return 0
				}
				return *k.ChunkCount
			}(),
			"storage_size": k.StorageSize,
			"file_path":    k.FilePath,
			"created_at":   k.CreatedAt.Unix(),
			"updated_at":   k.UpdatedAt.Unix(),
		}
	}

	OK(c, map[string]interface{}{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// GetChunks 获取分块列表
// @Summary 获取分块列表
// @Description 获取知识库的分块列表
// @Tags knowledge-base
// @Router /api/v1/knowledge-bases/{id}/chunks [get]
func (h *KnowledgeBaseHandler) GetChunks(c *gin.Context) {
	knowledgeBaseID := c.Param("id")
	if knowledgeBaseID == "" {
		BadRequest(c, "知识库ID不能为空")
		return
	}

	page, size := GetPageParams(c)
	knowledgeID := c.Query("knowledge_id")

	chunks, total, err := h.knowledgeBaseService.GetChunks(c.Request.Context(), knowledgeBaseID, GetTenantID(c), page, size, knowledgeID)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	// 转换为响应格式
	items := make([]map[string]interface{}, len(chunks))
	for i, chunk := range chunks {
		items[i] = map[string]interface{}{
			"id":           chunk.ID,
			"knowledge_id": chunk.KnowledgeID,
			"content":      chunk.Content,
			"chunk_index":  chunk.ChunkIndex,
			"is_enabled":   chunk.IsEnabled,
			"start_at":     chunk.StartAt,
			"end_at":       chunk.EndAt,
			"chunk_type":   chunk.ChunkType,
			"token_count": func() int {
				if chunk.TokenCount == nil {
					return 0
				}
				return *chunk.TokenCount
			}(),
			"created_at": chunk.CreatedAt.Unix(),
		}
	}

	PageSuccessJSON(c, total, items, page, size)
}

// GetChunkDetail 获取分块详情
// @Summary 获取分块详情
// @Description 获取指定分块的详细信息
// @Tags knowledge-base
// @Router /api/v1/knowledge-bases/{id}/chunks/{cid} [get]
func (h *KnowledgeBaseHandler) GetChunkDetail(c *gin.Context) {
	knowledgeBaseID := c.Param("id")
	chunkID := c.Param("cid")

	if knowledgeBaseID == "" {
		BadRequest(c, "知识库ID不能为空")
		return
	}
	if chunkID == "" {
		BadRequest(c, "分块ID不能为空")
		return
	}

	chunk, err := h.knowledgeBaseService.GetChunkDetail(c.Request.Context(), knowledgeBaseID, GetTenantID(c), chunkID)
	if err != nil {
		NotFound(c, err.Error())
		return
	}

	OK(c, map[string]interface{}{
		"id":           chunk.ID,
		"knowledge_id": chunk.KnowledgeID,
		"content":      chunk.Content,
		"chunk_index":  chunk.ChunkIndex,
		"is_enabled":   chunk.IsEnabled,
		"start_at":     chunk.StartAt,
		"end_at":       chunk.EndAt,
		"chunk_type":   chunk.ChunkType,
		"token_count": func() int {
			if chunk.TokenCount == nil {
				return 0
			}
			return *chunk.TokenCount
		}(),
		"created_at": chunk.CreatedAt.Unix(),
		"updated_at": chunk.UpdatedAt.Unix(),
	})
}

// UpdateChunk 更新分块
// @Summary 更新分块
// @Description 更新指定分块的内容
// @Tags knowledge-base
// @Router /api/v1/knowledge-bases/{id}/chunks/{cid} [put]
func (h *KnowledgeBaseHandler) UpdateChunk(c *gin.Context) {
	knowledgeBaseID := c.Param("id")
	chunkID := c.Param("cid")

	if knowledgeBaseID == "" {
		BadRequest(c, "知识库ID不能为空")
		return
	}
	if chunkID == "" {
		BadRequest(c, "分块ID不能为空")
		return
	}

	var req struct {
		Content *string  `json:"content"`
		Tags    []string `json:"tags"`
	}
	if !BindJSON(c, &req) {
		return
	}

	chunk, err := h.knowledgeBaseService.UpdateChunk(c.Request.Context(), knowledgeBaseID, GetTenantID(c), chunkID, req.Content)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, map[string]interface{}{
		"id":           chunk.ID,
		"knowledge_id": chunk.KnowledgeID,
		"content":      chunk.Content,
		"chunk_index":  chunk.ChunkIndex,
		"is_enabled":   chunk.IsEnabled,
		"start_at":     chunk.StartAt,
		"end_at":       chunk.EndAt,
		"chunk_type":   chunk.ChunkType,
		"token_count": func() int {
			if chunk.TokenCount == nil {
				return 0
			}
			return *chunk.TokenCount
		}(),
		"created_at": chunk.CreatedAt.Unix(),
		"updated_at": chunk.UpdatedAt.Unix(),
	})
}

// DeleteChunk 删除分块
// @Summary 删除分块
// @Description 删除指定的分块
// @Tags knowledge-base
// @Router /api/v1/knowledge-bases/{id}/chunks/{cid} [delete]
func (h *KnowledgeBaseHandler) DeleteChunk(c *gin.Context) {
	knowledgeBaseID := c.Param("id")
	chunkID := c.Param("cid")

	if knowledgeBaseID == "" {
		BadRequest(c, "知识库ID不能为空")
		return
	}
	if chunkID == "" {
		BadRequest(c, "分块ID不能为空")
		return
	}

	err := h.knowledgeBaseService.DeleteChunk(c.Request.Context(), knowledgeBaseID, GetTenantID(c), chunkID)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, map[string]interface{}{"message": "删除成功"})
}

// SearchKnowledge 搜索知识库
// @Summary 搜索知识库
// @Description 在知识库中搜索相关内容
// @Tags knowledge-base
// @Router /api/v1/knowledge/search [post]
func (h *KnowledgeBaseHandler) SearchKnowledge(c *gin.Context) {
	var req struct {
		KnowledgeBaseIDs []string `json:"kb_ids" binding:"required"`
		Query            string   `json:"query" binding:"required"`
		TopK             int      `json:"top_k"`
		MinScore         float64  `json:"min_score"`
		RetrievalMode    string   `json:"retrieval_mode"` // vector / bm25 / hybrid（缺省 hybrid）
		EnableRerank     bool     `json:"enable_rerank"`  // 可插拔重排开关，默认关
	}
	if !BindJSON(c, &req) {
		return
	}

	// 设置默认值
	if req.TopK <= 0 {
		req.TopK = 10
	}

	if h.retrieval == nil {
		InternalError(c, "检索能力未接线")
		return
	}

	tenantID := GetTenantID(c)
	// 租户边界强制：只保留归属本租户的知识库，越权/陌生 ID 被静默剔除，再交由统一检索能力。
	kbIDs := h.knowledgeBaseService.FilterAccessibleKBIDs(c.Request.Context(), req.KnowledgeBaseIDs, tenantID)

	result, err := h.retrieval.Retrieve(c.Request.Context(), tenantID, kbIDs, app_kb.GovernedQuery{
		Query:        req.Query,
		Mode:         req.RetrievalMode,
		TopK:         req.TopK,
		MinScore:     req.MinScore,
		EnableRerank: req.EnableRerank,
	})
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	// 转换为响应格式：分数/出处来自统一检索能力（替代旧裸子串匹配硬编码的固定分数 1.0 与空标题）。
	items := make([]map[string]interface{}, len(result.Chunks))
	for i, chunk := range result.Chunks {
		items[i] = map[string]interface{}{
			"knowledge_id":    chunk.KnowledgeID,
			"knowledge_title": chunk.Source,
			"chunk_id":        chunk.ChunkID,
			"content":         chunk.Content,
			"score":           chunk.Score,
			"highlight":       "", // 高亮功能需要在检索结果中标记匹配的关键词位置
		}
	}

	OK(c, map[string]interface{}{
		"total":      int64(len(items)),
		"items":      items,
		"has_answer": result.HasAnswer,
	})
}
