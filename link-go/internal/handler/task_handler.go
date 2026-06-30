// Package handler 提供任务管理的HTTP处理器
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	appEvaluation "link/internal/service/evaluation"
"link/internal/infrastructure/config"
)

// ========================================
// TaskHandler 任务处理器
// ========================================

// TaskHandler 任务处理器
type TaskHandler struct {
	taskService *appEvaluation.TaskService
}

// NewTaskHandler 创建任务处理器
func NewTaskHandler(taskService *appEvaluation.TaskService) *TaskHandler {
	return &TaskHandler{
		taskService: taskService,
	}
}

// ========================================
// 请求结构
// ========================================

// CreateTaskRequest 创建任务请求
type CreateTaskRequest struct {
	Type     string                 `json:"type" binding:"required"`     // 任务类型
	TargetID string                 `json:"target_id"`                  // 目标资源ID
	Payload  map[string]interface{} `json:"payload"`                    // 任务参数
	MaxRetries int                  `json:"max_retries"`                // 最大重试次数
	TimeoutSeconds int              `json:"timeout_seconds"`            // 超时时间（秒）
	ParentID string                 `json:"parent_id"`                  // 父任务ID
}

// ListTasksRequest 列出任务请求参数
type ListTasksRequest struct {
	Type   string `form:"type"`   // 任务类型过滤
	Status string `form:"status"` // 状态过滤
	Page   int    `form:"page"`   // 页码
	Size   int    `form:"size"`   // 每页数量
}

// RetryTaskRequest 重试任务请求
type RetryTaskRequest struct{}

// ========================================
// HTTP 处理方法
// ========================================

// CreateTask 创建任务
// POST /api/v1/tasks
func (h *TaskHandler) CreateTask(c *gin.Context) {
	// 获取租户和用户信息
	tenantIDInt, userIDInt, ok := MustGetTenantAndUserID(c)
	if !ok {
		return
	}

	// 解析请求
	var req CreateTaskRequest
	if !BindJSON(c, &req) {
		return
	}

	// 构建任务选项
	var options []appEvaluation.TaskOption
	if req.MaxRetries > 0 {
		options = append(options, appEvaluation.WithMaxRetries(req.MaxRetries))
	}
	if req.TimeoutSeconds > 0 {
		options = append(options, appEvaluation.WithTimeout(req.TimeoutSeconds))
	}
	if req.ParentID != "" {
		options = append(options, appEvaluation.WithParentID(req.ParentID))
	}

	// 创建任务
	task, err := h.taskService.CreateWithQueue(
		c.Request.Context(),
		tenantIDInt,
		userIDInt,
		req.Type,
		req.TargetID,
		req.Payload,
		options...,
	)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Created(c, task)
}

// GetTask 获取任务详情
// GET /api/v1/tasks/:id
func (h *TaskHandler) GetTask(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		BadRequest(c, "task id is required")
		return
	}

	task, err := h.taskService.GetTask(c.Request.Context(), taskID)
	if err != nil {
		NotFound(c, err.Error())
		return
	}

	OK(c, task)
}

// ListTasks 列出任务
// GET /api/v1/tasks
func (h *TaskHandler) ListTasks(c *gin.Context) {
	// 获取租户信息
	tenantIDInt, ok := MustGetTenantID(c)
	if !ok {
		return
	}

	// 解析查询参数
	var req ListTasksRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	// 使用配置的默认值
	req.Page = config.NormalizePage(req.Page)
	req.Size = config.NormalizePageSize(req.Size)

	// 查询任务列表
	tasks, total, err := h.taskService.ListTasks(
		c.Request.Context(),
		tenantIDInt,
		req.Page,
		req.Size,
	)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	PageSuccessJSON(c, total, tasks, req.Page, req.Size)
}

// ListTasksByType 按类型列出任务
// GET /api/v1/tasks/type/:type
func (h *TaskHandler) ListTasksByType(c *gin.Context) {
	// 获取租户信息
	tenantIDInt, ok := MustGetTenantID(c)
	if !ok {
		return
	}

	taskType := c.Param("type")
	status := c.Query("status")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	limit = config.NormalizePageSize(limit)

	tasks, err := h.taskService.ListTasksByType(
		c.Request.Context(),
		tenantIDInt,
		taskType,
		status,
		limit,
	)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, tasks)
}

// CancelTask 取消任务
// POST /api/v1/tasks/:id/cancel
func (h *TaskHandler) CancelTask(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		BadRequest(c, "task id is required")
		return
	}

	if err := h.taskService.CancelTask(c.Request.Context(), taskID); err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, gin.H{"message": "task cancelled"})
}

// RetryTask 重试任务
// POST /api/v1/tasks/:id/retry
func (h *TaskHandler) RetryTask(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		BadRequest(c, "task id is required")
		return
	}

	if err := h.taskService.RetryTask(c.Request.Context(), taskID); err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, gin.H{"message": "task queued for retry"})
}
