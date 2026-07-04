// Package handler 提供评测管理的HTTP处理器
package handler

import (
	"errors"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	domeval "link/internal/model/evaluation"
	"link/internal/service/evaluation"
)

// ========================================
// EvaluationHandler 评测处理器
// ========================================

// EvaluationHandler 评测处理器
type EvaluationHandler struct {
	service         *evaluation.Service
	datasetManager  *evaluation.DatasetManager
	progressCache   domeval.ProgressReader
}

// NewEvaluationHandler 创建评测处理器
func NewEvaluationHandler(
	service *evaluation.Service,
	datasetManager *evaluation.DatasetManager,
	progressCache domeval.ProgressReader,
) *EvaluationHandler {
	return &EvaluationHandler{
		service:        service,
		datasetManager: datasetManager,
		progressCache:  progressCache,
	}
}

// ========================================
// 请求结构
// ========================================

// CreateEvaluationTaskRequest 创建评测任务请求
type CreateEvaluationTaskRequest struct {
	DatasetID string                 `json:"dataset_id" binding:"required"`
	Type      string                 `json:"type" binding:"required"`
	KnowledgeBaseID      string                 `json:"kb_id,omitempty"`
	AgentID   string                 `json:"agent_id,omitempty"`
	ModelID   string                 `json:"model_id,omitempty"`
	Config    map[string]interface{} `json:"config,omitempty"`
}

// ListDatasetsRequest 列出数据集请求参数
type ListDatasetsRequest struct {
	Type string `form:"type"` // 数据集类型过滤
}

// ========================================
// HTTP 处理方法
// ========================================

// CreateTask 创建评测任务
// POST /api/v1/evaluation/tasks
func (h *EvaluationHandler) CreateTask(c *gin.Context) {
	// 获取租户和用户信息
	tenantIDInt, userIDInt, ok := MustGetTenantAndUserID(c)
	if !ok {
		return
	}

	// 解析请求
	var req CreateEvaluationTaskRequest
	if !BindJSON(c, &req) {
		return
	}

	// 构建配置
	evalConfig := &evaluation.CreateEvaluationTaskRequest{
		DatasetID: req.DatasetID,
		Type:      evaluation.EvaluationType(req.Type),
		KnowledgeBaseID:      req.KnowledgeBaseID,
		AgentID:   req.AgentID,
		ModelID:   req.ModelID,
		Config:    req.Config,
	}

	// 创建任务
	task, err := h.service.CreateEvaluation(c.Request.Context(), tenantIDInt, userIDInt, evalConfig)
	if err != nil {
		h.handleError(c, err)
		return
	}

	Created(c, gin.H{
		"task_id":    task.ID,
		"type":       task.Type,
		"status":     task.Status,
		"dataset_id": req.DatasetID,
		"created_at": task.CreatedAt,
	})
}

// GetTask 获取评测任务状态
// GET /api/v1/evaluation/tasks/:id
func (h *EvaluationHandler) GetTask(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		BadRequest(c, "task id is required")
		return
	}

	result, err := h.service.GetEvaluationResult(c.Request.Context(), taskID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	OK(c, result)
}

// ListTasks 列出评测任务
// GET /api/v1/evaluation/tasks
func (h *EvaluationHandler) ListTasks(c *gin.Context) {
	// 获取租户信息
	tenantID, exists := c.Get("tenant_id")
	if !exists {
		Unauthorized(c, "tenant not found")
		return
	}

	// 安全的类型断言
	tenantIDInt, ok := tenantID.(int64)
	if !ok {
		BadRequest(c, "invalid tenant_id type")
		return
	}

	// 解析查询参数
	status := c.Query("status")
	evalType := c.Query("type")
	page, pageSize := GetPageParams(c)

	// 调用服务
	resultList, err := h.service.ListEvaluationResults(c.Request.Context(), tenantIDInt, page, pageSize)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// 过滤结果（基于查询参数）
	items := resultList.Items
	if status != "" || evalType != "" {
		filtered := make([]*evaluation.EvaluationTaskSummary, 0)
		for _, item := range items {
			if status != "" && string(item.Status) != status {
				continue
			}
			if evalType != "" && string(item.Type) != evalType {
				continue
			}
			filtered = append(filtered, item)
		}
		items = filtered
		resultList.Total = int64(len(filtered))
	}

	OK(c, gin.H{
		"items":       items,
		"total":       resultList.Total,
		"page":        resultList.Page,
		"page_size":   resultList.PageSize,
		"total_pages": resultList.TotalPages,
	})
}

// StreamTask SSE 流式推送任务进度
// GET /api/v1/evaluation/tasks/:id/stream
func (h *EvaluationHandler) StreamTask(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		BadRequest(c, "task id is required")
		return
	}

	// 设置 SSE 头
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")

	// 创建 ticker 用于轮询
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// 心跳 ticker（每30秒发送一次）
	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer heartbeatTicker.Stop()

	// 设置最大超时时间（5分钟）
	timeout := time.NewTimer(5 * time.Minute)
	defer timeout.Stop()

	// 客户端断开检测
	ctx := c.Request.Context()

	// 上次进度（用于去重）
	var lastProgress string

	for {
		select {
		case <-ctx.Done():
			return
		case <-timeout.C:
			// 超时后发送超时事件
			c.SSEvent("timeout", `{"message":"Stream timeout after 5 minutes"}`)
			c.Writer.Flush()
			return
		case <-heartbeatTicker.C:
			// 发送心跳（注释行，不会触发客户端事件）
			fmt.Fprintf(c.Writer, ": heartbeat\n\n")
			c.Writer.Flush()
		case <-ticker.C:
			// 从 Redis 获取进度
			progress, err := h.progressCache.GetProgress(ctx, taskID)
			if err != nil {
				// 出错时发送错误事件并结束
				c.SSEvent("error", `{"message":"failed to get progress"}`)
				c.Writer.Flush()
				return
			}

			// 如果没有进度信息，跳过
			if progress == nil {
				continue
			}

			// 序列化进度
			data := fmt.Sprintf(`{"stage":"%s","current":%d,"total":%d,"message":"%s","percentage":%d}`,
				progress.Stage, progress.Current, progress.Total, progress.Message, progress.Percentage)

			// 去重：如果进度没变化，不发送
			if data == lastProgress {
				continue
			}
			lastProgress = data

			// 发送 SSE 数据
			c.SSEvent("progress", data)
			c.Writer.Flush()

			// 检查任务是否完成或失败
			if progress.Stage == domeval.StageCompleted {
				c.SSEvent("done", `{"message":"Evaluation completed"}`)
				c.Writer.Flush()
				return
			}
			if progress.Stage == domeval.StageFailed {
				c.SSEvent("failed", fmt.Sprintf(`{"message":"%s"}`, progress.Error))
				c.Writer.Flush()
				return
			}
		}
	}
}

// GetResults 获取评测结果
// GET /api/v1/evaluation/tasks/:id/results
func (h *EvaluationHandler) GetResults(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		BadRequest(c, "task id is required")
		return
	}

	result, err := h.service.GetEvaluationResult(c.Request.Context(), taskID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	OK(c, result)
}

// GetQAResults 获取 QA 级别评测结果（分页）
// GET /api/v1/evaluation/results/:task_id/qa
func (h *EvaluationHandler) GetQAResults(c *gin.Context) {
	taskID := c.Param("task_id")
	if taskID == "" {
		BadRequest(c, "task id is required")
		return
	}

	// 解析分页参数
	page, pageSize := GetPageParams(c)

	// 获取 QA 结果
	result, err := h.service.GetQAResults(c.Request.Context(), taskID, page, pageSize)
	if err != nil {
		h.handleError(c, err)
		return
	}

	OK(c, result)
}

// ListResults 列出评测结果
// GET /api/v1/evaluation/results
func (h *EvaluationHandler) ListResults(c *gin.Context) {
	// 获取租户信息
	tenantID, exists := c.Get("tenant_id")
	if !exists {
		Unauthorized(c, "tenant not found")
		return
	}

	// 解析查询参数
	page, pageSize := GetPageParams(c)
	status := c.Query("status")
	evalType := c.Query("type")

	// 调用服务
	resultList, err := h.service.ListEvaluationResults(c.Request.Context(), tenantID.(int64), page, pageSize)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// 过滤结果（基于查询参数）
	items := resultList.Items
	if status != "" || evalType != "" {
		filtered := make([]*evaluation.EvaluationTaskSummary, 0)
		for _, item := range items {
			if status != "" && string(item.Status) != status {
				continue
			}
			if evalType != "" && string(item.Type) != evalType {
				continue
			}
			filtered = append(filtered, item)
		}
		items = filtered
		resultList.Total = int64(len(filtered))
	}

	OK(c, gin.H{
		"items":       items,
		"total":       resultList.Total,
		"page":        resultList.Page,
		"page_size":   resultList.PageSize,
		"total_pages": resultList.TotalPages,
	})
}

// DeleteTask 删除评测任务
// DELETE /api/v1/evaluation/tasks/:id
func (h *EvaluationHandler) DeleteTask(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		BadRequest(c, "task id is required")
		return
	}

	if err := h.service.DeleteEvaluationTask(c.Request.Context(), taskID); err != nil {
		h.handleError(c, err)
		return
	}

	OK(c, gin.H{
		"message": "Task deleted successfully",
		"task_id": taskID,
	})
}

// ListDatasets 列出可用的数据集
// GET /api/v1/evaluation/datasets
func (h *EvaluationHandler) ListDatasets(c *gin.Context) {
	// 解析查询参数
	var req ListDatasetsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	// 获取数据集列表
	datasets, err := h.service.GetDatasetList(c.Request.Context())
	if err != nil {
		h.handleError(c, err)
		return
	}

	// 按类型过滤
	if req.Type != "" {
		filtered := make([]*evaluation.DatasetMetadata, 0)
		for _, ds := range datasets {
			if string(ds.EvalType) == req.Type {
				filtered = append(filtered, ds)
			}
		}
		datasets = filtered
	}

	OK(c, datasets)
}

// GetDatasetInfo 获取数据集信息
// GET /api/v1/evaluation/datasets/:id
func (h *EvaluationHandler) GetDatasetInfo(c *gin.Context) {
	datasetID := c.Param("id")
	if datasetID == "" {
		BadRequest(c, "dataset id is required")
		return
	}

	info, err := h.service.GetDatasetInfo(c.Request.Context(), datasetID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	OK(c, info)
}

// ========================================
// Helper Methods
// ========================================

// handleError 处理错误并返回适当的 HTTP 响应
func (h *EvaluationHandler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domeval.ErrTaskNotFound):
		NotFound(c, err.Error())
	case errors.Is(err, domeval.ErrDatasetNotFound):
		NotFound(c, err.Error())
	case errors.Is(err, domeval.ErrDatasetSampleNotFound):
		NotFound(c, err.Error())
	case errors.Is(err, domeval.ErrInvalidConfig), errors.Is(err, domeval.ErrInvalidEvalType), errors.Is(err, domeval.ErrDatasetNameEmpty):
		BadRequest(c, err.Error())
	default:
		InternalError(c, err.Error())
	}
}


// ========================================
// 数据集管理 HTTP Handlers
// ========================================

// CreateDataset 创建数据集
// POST /api/v1/evaluation/datasets
func (h *EvaluationHandler) CreateDataset(c *gin.Context) {
	// 获取租户和用户信息
	tenantID, exists := c.Get("tenant_id")
	if !exists {
		Unauthorized(c, "tenant not found")
		return
	}
	userID, exists := c.Get("user_id")
	if !exists {
		Unauthorized(c, "user not found")
		return
	}

	// 解析请求
	var req evaluation.CreateDatasetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	// 创建数据集
	result, err := h.datasetManager.CreateDataset(c.Request.Context(), tenantID.(int64), userID.(int64), &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	Created(c, result)
}

// GetDatasetDetail 获取数据集详情
// GET /api/v1/evaluation/datasets/:id
func (h *EvaluationHandler) GetDatasetDetail(c *gin.Context) {
	datasetID := c.Param("id")
	if datasetID == "" {
		BadRequest(c, "dataset id is required")
		return
	}

	tenantID, exists := c.Get("tenant_id")
	if !exists {
		Unauthorized(c, "tenant not found")
		return
	}

	result, err := h.datasetManager.GetDataset(c.Request.Context(), datasetID, tenantID.(int64))
	if err != nil {
		h.handleError(c, err)
		return
	}

	OK(c, result)
}

// UpdateDataset 更新数据集
// PUT /api/v1/evaluation/datasets/:id
func (h *EvaluationHandler) UpdateDataset(c *gin.Context) {
	datasetID := c.Param("id")
	if datasetID == "" {
		BadRequest(c, "dataset id is required")
		return
	}

	tenantID, exists := c.Get("tenant_id")
	if !exists {
		Unauthorized(c, "tenant not found")
		return
	}

	var req evaluation.UpdateDatasetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	result, err := h.datasetManager.UpdateDataset(c.Request.Context(), datasetID, tenantID.(int64), &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	OK(c, result)
}

// DeleteDataset 删除数据集
// DELETE /api/v1/evaluation/datasets/:id
func (h *EvaluationHandler) DeleteDataset(c *gin.Context) {
	datasetID := c.Param("id")
	if datasetID == "" {
		BadRequest(c, "dataset id is required")
		return
	}

	tenantID, exists := c.Get("tenant_id")
	if !exists {
		Unauthorized(c, "tenant not found")
		return
	}

	if err := h.datasetManager.DeleteDataset(c.Request.Context(), datasetID, tenantID.(int64)); err != nil {
		h.handleError(c, err)
		return
	}

	OK(c, gin.H{
		"message":    "Dataset deleted successfully",
		"dataset_id": datasetID,
	})
}

// ListSamples 列出数据集样本
// GET /api/v1/evaluation/datasets/:id/samples
func (h *EvaluationHandler) ListSamples(c *gin.Context) {
	datasetID := c.Param("id")
	if datasetID == "" {
		BadRequest(c, "dataset id is required")
		return
	}

	tenantID, exists := c.Get("tenant_id")
	if !exists {
		Unauthorized(c, "tenant not found")
		return
	}

	// 解析分页参数
	var req evaluation.ListSamplesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	result, err := h.datasetManager.ListSamples(c.Request.Context(), datasetID, tenantID.(int64), &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	OK(c, result)
}

// AddSamples 添加样本
// POST /api/v1/evaluation/datasets/:id/samples
func (h *EvaluationHandler) AddSamples(c *gin.Context) {
	datasetID := c.Param("id")
	if datasetID == "" {
		BadRequest(c, "dataset id is required")
		return
	}

	tenantID, exists := c.Get("tenant_id")
	if !exists {
		Unauthorized(c, "tenant not found")
		return
	}

	var req evaluation.AddSamplesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	if err := h.datasetManager.AddSamples(c.Request.Context(), datasetID, tenantID.(int64), &req); err != nil {
		h.handleError(c, err)
		return
	}

	OK(c, gin.H{
		"message":    "Samples added successfully",
		"dataset_id": datasetID,
		"count":      len(req.QAPairs),
	})
}

// DeleteSample 删除样本
// DELETE /api/v1/evaluation/datasets/:id/samples/:sample_id
func (h *EvaluationHandler) DeleteSample(c *gin.Context) {
	datasetID := c.Param("id")
	if datasetID == "" {
		BadRequest(c, "dataset id is required")
		return
	}

	sampleID := c.Param("sample_id")
	if sampleID == "" {
		BadRequest(c, "sample id is required")
		return
	}

	tenantID, exists := c.Get("tenant_id")
	if !exists {
		Unauthorized(c, "tenant not found")
		return
	}

	// 转换 sample_id
	var sampleIDInt int64
	if _, err := fmt.Sscanf(sampleID, "%d", &sampleIDInt); err != nil {
		BadRequest(c, "invalid sample id")
		return
	}

	if err := h.datasetManager.DeleteSample(c.Request.Context(), datasetID, tenantID.(int64), sampleIDInt); err != nil {
		h.handleError(c, err)
		return
	}

	OK(c, gin.H{
		"message":   "Sample deleted successfully",
		"sample_id": sampleID,
	})
}

// ========================================
// 可用指标目录（注册表驱动，按评测类型过滤）
// ========================================

// ListGraders 返回某评测类型下的可用指标目录。
// GET /api/v1/evaluation/graders?eval_type=llm|qa|rag|agent
// 目录来源于 Python 注册表（唯一事实来源），前端据此渲染可勾选指标，消除写死漂移。
func (h *EvaluationHandler) ListGraders(c *gin.Context) {
	evalType := c.Query("eval_type")
	if evalType == "" {
		BadRequest(c, "eval_type is required")
		return
	}

	catalog, err := h.service.ListAvailableGraders(c.Request.Context(), evalType)
	if err != nil {
		h.handleError(c, err)
		return
	}

	OK(c, catalog)
}
