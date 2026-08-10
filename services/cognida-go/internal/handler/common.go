package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	agentctx "cognida/internal/model/agent"
	"cognida/internal/pkg/httputil"
)

// ========================================
// 统一响应结构
// ========================================

// Response 统一响应结构
type Response struct {
	Code    int         `json:"code"`           // 业务状态码
	Message string      `json:"message"`        // 提示信息
	Data    interface{} `json:"data,omitempty"` // 数据
}

// ========================================
// 响应构造函数
// ========================================

// NewResponse 创建响应
func NewResponse(code int, message string, data interface{}) *Response {
	return &Response{
		Code:    code,
		Message: message,
		Data:    data,
	}
}

// Success 成功响应
func Success(data interface{}) *Response {
	return &Response{
		Code:    0,
		Message: "success",
		Data:    data,
	}
}

// Fail 失败响应
func Fail(code int, message string) *Response {
	return &Response{
		Code:    code,
		Message: message,
	}
}

// PageData 分页数据
type PageData struct {
	Total int64       `json:"total"` // 总数
	Items interface{} `json:"items"` // 列表
	Page  int         `json:"page"`  // 当前页
	Size  int         `json:"size"`  // 每页数量
}

// PageSuccess 分页成功响应
func PageSuccess(total int64, items interface{}, page, size int) *Response {
	return Success(&PageData{
		Total: total,
		Items: items,
		Page:  page,
		Size:  size,
	})
}

// PageSuccessJSON 分页成功 JSON 响应
func PageSuccessJSON(c *gin.Context, total int64, items interface{}, page, size int) {
	c.JSON(http.StatusOK, &Response{
		Code:    0,
		Message: "success",
		Data: &PageData{
			Total: total,
			Items: items,
			Page:  page,
			Size:  size,
		},
	})
}

// ========================================
// HTTP 响应方法
// ========================================

// OK 成功响应
func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Success(data))
}

// Created 创建成功响应
func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, Success(data))
}

// NoContent 无内容响应
func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// BadRequest 错误请求响应
func BadRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, Fail(http.StatusBadRequest, message))
}

// Unauthorized 未授权响应
func Unauthorized(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, Fail(http.StatusUnauthorized, message))
}

// Forbidden 禁止访问响应
func Forbidden(c *gin.Context, message string) {
	c.JSON(http.StatusForbidden, Fail(http.StatusForbidden, message))
}

// NotFound 未找到响应
func NotFound(c *gin.Context, message string) {
	c.JSON(http.StatusNotFound, Fail(http.StatusNotFound, message))
}

// InternalError 内部错误响应
func InternalError(c *gin.Context, message string) {
	c.JSON(http.StatusInternalServerError, Fail(http.StatusInternalServerError, message))
}

// Conflict 资源冲突响应（如重复上传），data 携带冲突详情供前端处理
func Conflict(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusConflict, NewResponse(http.StatusConflict, message, data))
}

// ========================================
// 请求绑定
// ========================================

// BindJSON 绑定 JSON 请求，失败时自动返回 400 错误
func BindJSON(c *gin.Context, obj interface{}) bool {
	if err := c.ShouldBindJSON(obj); err != nil {
		BadRequest(c, err.Error())
		return false
	}
	return true
}

// BindQuery 绑定 Query 参数，失败时自动返回 400 错误
func BindQuery(c *gin.Context, obj interface{}) bool {
	if err := c.ShouldBindQuery(obj); err != nil {
		BadRequest(c, err.Error())
		return false
	}
	return true
}

// BindURI 绑定 URI 参数，失败时自动返回 400 错误
func BindURI(c *gin.Context, obj interface{}) bool {
	if err := c.ShouldBindUri(obj); err != nil {
		BadRequest(c, err.Error())
		return false
	}
	return true
}

// ========================================
// 参数提取
// ========================================

// GetUserID 获取用户 ID（使用默认值）
func GetUserID(c *gin.Context) int64 {
	return httputil.GetUserIDWithDefault(c.GetInt64("user_id"))
}

// GetTenantID 获取租户 ID（使用默认值）
func GetTenantID(c *gin.Context) int64 {
	return httputil.GetTenantIDWithDefault(c.GetInt64("tenant_id"))
}

// GetPageParams 获取分页参数
func GetPageParams(c *gin.Context) (page, pageSize int) {
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "20")

	pageRaw, _ := strconv.Atoi(pageStr)
	pageSizeRaw, _ := strconv.Atoi(pageSizeStr)

	page = httputil.NormalizePage(pageRaw)
	pageSize = httputil.NormalizePageSize(pageSizeRaw)

	return page, pageSize
}

// ========================================
// 上下文设置
// ========================================

// WithTenantID 在上下文中设置租户 ID
func WithTenantID(c *gin.Context, tenantID int64) {
	c.Set("tenant_id", tenantID)
}

// WithUserID 在上下文中设置用户 ID
func WithUserID(c *gin.Context, userID int64) {
	c.Set("user_id", userID)
}

// ========================================
// 参数验证
// ========================================

// RequireParam 验证必需的 URI 参数，为空时返回 400 错误
// 返回 true 表示验证通过，false 表示失败（已响应）
func RequireParam(c *gin.Context, paramValue, paramName string) bool {
	if paramValue == "" {
		BadRequest(c, paramName+" is required")
		return false
	}
	return true
}

// ========================================
// 上下文获取（带错误处理）
// ========================================

// MustGetTenantID 获取租户 ID，失败时返回 401 错误
// 返回 (tenantID, true) 表示成功，(0, false) 表示已响应错误
func MustGetTenantID(c *gin.Context) (int64, bool) {
	tenantID, exists := c.Get("tenant_id")
	if !exists {
		Unauthorized(c, "tenant not found")
		return 0, false
	}

	// 类型断言
	id, ok := tenantID.(int64)
	if !ok {
		BadRequest(c, "invalid tenant_id type")
		return 0, false
	}

	return id, true
}

// MustGetUserID 获取用户 ID，失败时返回 401 错误
// 返回 (userID, true) 表示成功，(0, false) 表示已响应错误
func MustGetUserID(c *gin.Context) (int64, bool) {
	userID, exists := c.Get("user_id")
	if !exists {
		Unauthorized(c, "user not found")
		return 0, false
	}

	// 类型断言
	id, ok := userID.(int64)
	if !ok {
		BadRequest(c, "invalid user_id type")
		return 0, false
	}

	return id, true
}

// MustGetTenantAndUserID 获取租户和用户 ID，任一失败返回 401 错误
// 返回 (tenantID, userID, true) 表示成功，(0, 0, false) 表示已响应错误
func MustGetTenantAndUserID(c *gin.Context) (int64, int64, bool) {
	tenantID, ok1 := MustGetTenantID(c)
	if !ok1 {
		return 0, 0, false
	}

	userID, ok2 := MustGetUserID(c)
	if !ok2 {
		return 0, 0, false
	}

	return tenantID, userID, true
}

// ========================================
// Context Helpers
// ========================================

// ContextWithTenantID 在 context 中设置租户 ID。
// 统一走 agentctx 的类型化 key，避免与内建字符串 key 冲突（go vet SA1029），
// 并与 service/repository 侧的 agentctx.GetTenantID 读取端保持同一把 key。
func ContextWithTenantID(ctx context.Context, tenantID int64) context.Context {
	return agentctx.WithTenantID(ctx, tenantID)
}

// ContextWithUserID 在 context 中设置用户 ID（同上，统一走 agentctx 类型化 key）。
func ContextWithUserID(ctx context.Context, userID int64) context.Context {
	return agentctx.WithUserID(ctx, userID)
}

// ========================================
// Time Helpers
// ========================================

// FormatTime 格式化时间指针，返回 Unix 时间戳
func FormatTime(t *time.Time) int64 {
	if t == nil {
		return 0
	}
	return t.Unix()
}
