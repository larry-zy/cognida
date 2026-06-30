// Package errors provides domain error definitions and business error types
package errors

import (
	"errors"
	"fmt"
)

// ========================================
// 领域错误定义 (标准库 errors)
// ========================================

var (
	// ========================================
	// 通用错误
	// ========================================

	// ErrNotFound 资源未找到
	ErrNotFound = errors.New("resource not found")

	// ErrAlreadyExists 资源已存在
	ErrAlreadyExists = errors.New("resource already exists")

	// ErrInvalidInput 无效输入
	ErrInvalidInput = errors.New("invalid input")

	// ErrUnauthorized 未授权
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden 禁止访问
	ErrForbidden = errors.New("forbidden")

	// ErrInternal 内部错误
	ErrInternal = errors.New("internal error")

	// ErrTimeout 超时
	ErrTimeout = errors.New("operation timeout")

	// ========================================
	// 租户相关错误
	// ========================================

	// ErrTenantNotFound 租户未找到
	ErrTenantNotFound = errors.New("tenant not found")

	// ErrTenantSuspended 租户已暂停
	ErrTenantSuspended = errors.New("tenant is suspended")

	// ErrTenantStorageExceeded 租户存储超额
	ErrTenantStorageExceeded = errors.New("tenant storage quota exceeded")

	// ========================================
	// 用户相关错误
	// ========================================

	// ErrUserNotFound 用户未找到
	ErrUserNotFound = errors.New("user not found")

	// ErrUserDisabled 用户已禁用
	ErrUserDisabled = errors.New("user is disabled")

	// ErrInvalidCredentials 无效凭证
	ErrInvalidCredentials = errors.New("invalid credentials")

	// ErrInvalidToken 无效令牌
	ErrInvalidToken = errors.New("invalid token")

	// ErrTokenExpired 令牌已过期
	ErrTokenExpired = errors.New("token expired")

	// ========================================
	// 知识库相关错误
	// ========================================

	// ErrKnowledgeBaseNotFound 知识库未找到
	ErrKnowledgeBaseNotFound = errors.New("knowledge base not found")

	// ErrKnowledgeNotFound 知识条目未找到
	ErrKnowledgeNotFound = errors.New("knowledge not found")

	// ErrChunkNotFound 分块未找到
	ErrChunkNotFound = errors.New("chunk not found")

	// ErrInvalidKnowledgeBaseType 无效知识库类型
	ErrInvalidKnowledgeBaseType = errors.New("invalid knowledge base type")

	// ========================================
	// 会话相关错误
	// ========================================

	// ErrSessionNotFound 会话未找到
	ErrSessionNotFound = errors.New("session not found")

	// ErrSessionArchived 会话已归档
	ErrSessionArchived = errors.New("session is archived")

	// ========================================
	// Agent相关错误
	// ========================================

	// ErrAgentNotFound Agent未找到
	ErrAgentNotFound = errors.New("agent not found")

	// ErrToolNotFound 工具未找到
	ErrToolNotFound = errors.New("tool not found")

	// ErrToolExecutionFailed 工具执行失败
	ErrToolExecutionFailed = errors.New("tool execution failed")

	// ErrMaxIterationsExceeded 超过最大迭代次数
	ErrMaxIterationsExceeded = errors.New("max iterations exceeded")

	// ========================================
	// 检索相关错误
	// ========================================

	// ErrRetrievalFailed 检索失败
	ErrRetrievalFailed = errors.New("retrieval failed")

	// ErrNoResultsFound 没有找到结果
	ErrNoResultsFound = errors.New("no results found")

	// ErrInvalidRetrievalMode 无效的检索模式
	ErrInvalidRetrievalMode = errors.New("invalid retrieval mode")

	// ========================================
	// 外部服务相关错误
	// ========================================

	// ErrExternalServiceUnavailable 外部服务不可用
	ErrExternalServiceUnavailable = errors.New("external service unavailable")

	// ErrRateLimitExceeded 超过速率限制
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
)

// ========================================
// 领域错误类型
// ========================================

// DomainError 领域错误
type DomainError struct {
	Code    string
	Message string
	Err     error
}

// Error 实现error接口
func (e *DomainError) Error() string {
	if e.Err != nil {
		return e.Code + ": " + e.Message + " (" + e.Err.Error() + ")"
	}
	return e.Code + ": " + e.Message
}

// Unwrap 实现errors.Unwrap接口
func (e *DomainError) Unwrap() error {
	return e.Err
}

// NewDomainError 创建领域错误
func NewDomainError(code, message string, err error) *DomainError {
	return &DomainError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// ========================================
// 业务错误类型 (带数字错误码)
// ========================================

// BizError 业务错误
type BizError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

// Error 实现 error 接口
func (e *BizError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// Unwrap 支持 errors.Unwrap
func (e *BizError) Unwrap() error {
	return e.Err
}

// New 创建业务错误
func New(code int, message string) *BizError {
	return &BizError{
		Code:    code,
		Message: message,
	}
}

// Wrap 包装错误
func Wrap(code int, message string, err error) *BizError {
	return &BizError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// ========================================
// 预定义业务错误 (数字错误码)
// ========================================

// 通用错误 1-999
var (
	ErrSuccess          = New(0, "success")
	ErrUnknown          = New(1, "未知错误")
	ErrInvalidParam     = New(2, "参数错误")
	ErrInvalidRequest   = New(3, "请求格式错误")
	ErrMethodNotAllowed = New(4, "请求方法不允许")
	ErrUnauthorizedBiz  = New(5, "未授权")
	ErrForbiddenBiz     = New(6, "禁止访问")
	ErrNotFoundBiz      = New(7, "资源不存在")
	ErrDuplicate        = New(8, "数据已存在")
	ErrInternalBiz      = New(9, "内部错误")
	ErrTimeoutBiz       = New(10, "请求超时")
	ErrRateLimit        = New(11, "请求过于频繁")
)

// 用户相关 1000-1999
var (
	ErrUserNotFoundBiz    = New(1001, "用户不存在")
	ErrUserDisabledBiz    = New(1002, "用户已被禁用")
	ErrUserExists         = New(1003, "用户已存在")
	ErrPasswordWrong      = New(1004, "密码错误")
	ErrPasswordTooWeak    = New(1005, "密码强度不足")
	ErrOldPasswordWrong   = New(1006, "原密码错误")
	ErrPasswordSame       = New(1007, "新密码不能与原密码相同")
	ErrEmailExists        = New(1008, "邮箱已被使用")
	ErrPhoneExists        = New(1009, "手机号已被使用")
	ErrCaptchaWrong       = New(1010, "验证码错误")
	ErrCaptchaExpired     = New(1011, "验证码已过期")
	ErrTokenExpiredBiz    = New(1012, "Token 已过期")
	ErrTokenInvalidBiz    = New(1013, "Token 无效")
)

// 租户相关 2000-2999
var (
	ErrTenantNotFoundBiz    = New(2001, "租户不存在")
	ErrTenantDisabledBiz    = New(2002, "租户已被禁用")
	ErrTenantExists         = New(2003, "租户已存在")
	ErrTenantLimitExceed    = New(2004, "租户数量限制")
	ErrTenantUserLimit      = New(2005, "租户用户数限制")
)

// 知识库相关 3000-3999
var (
	ErrKBNotFound         = New(3001, "知识库不存在")
	ErrKBExists           = New(3002, "知识库已存在")
	ErrKBNameDuplicate    = New(3003, "知识库名称重复")
	ErrKBDisabled         = New(3004, "知识库已被禁用")
	ErrKBReadOnly         = New(3005, "知识库只读")
	ErrKBFileNotFound     = New(3006, "文件不存在")
	ErrKBFileUploadFailed = New(3007, "文件上传失败")
	ErrKBFileTypeInvalid  = New(3008, "不支持的文件类型")
	ErrKBFileSizeExceed   = New(3009, "文件大小超限")
)

// 文档/向量相关 4000-4999
var (
	ErrDocNotFound      = New(4001, "文档不存在")
	ErrDocIndexFailed   = New(4002, "文档索引失败")
	ErrDocSearchFailed  = New(4003, "文档搜索失败")
	ErrVectorNotFound   = New(4004, "向量不存在")
	ErrVectorDimInvalid = New(4005, "向量维度不匹配")
)

// 图谱相关 5000-5999
var (
	ErrGraphNotFound    = New(5001, "图谱不存在")
	ErrGraphExists      = New(5002, "图谱已存在")
	ErrEntityNotFound   = New(5003, "实体不存在")
	ErrRelationNotFound = New(5004, "关系不存在")
	ErrGraphParseFailed = New(5005, "图谱解析失败")
)

// 对话相关 6000-6999
var (
	ErrSessionNotFoundBiz     = New(6001, "会话不存在")
	ErrSessionExpired         = New(6002, "会话已过期")
	ErrMessageNotFound        = New(6003, "消息不存在")
	ErrLLMRequestFailed       = New(6004, "大模型请求失败")
	ErrLLMResponseInvalid     = New(6005, "大模型响应无效")
	ErrContextLengthExceed    = New(6006, "上下文长度超限")
	ErrRAGFailed              = New(6007, "知识检索失败")
)

// Agent 相关 7000-7999
var (
	ErrAgentNotFoundBiz    = New(7001, "Agent 不存在")
	ErrAgentDisabled       = New(7002, "Agent 已被禁用")
	ErrAgentToolNotFound   = New(7003, "工具不存在")
	ErrAgentToolFailed     = New(7004, "工具执行失败")
)

// ========================================
// 错误码常量
// ========================================

const (
	ErrorCodeNotFound           = "NOT_FOUND"
	ErrorCodeAlreadyExists      = "ALREADY_EXISTS"
	ErrorCodeInvalidInput       = "INVALID_INPUT"
	ErrorCodeUnauthorized       = "UNAUTHORIZED"
	ErrorCodeForbidden          = "FORBIDDEN"
	ErrorCodeInternal           = "INTERNAL_ERROR"
	ErrorCodeTimeout            = "TIMEOUT"
	ErrorCodeTenantNotFound     = "TENANT_NOT_FOUND"
	ErrorCodeTenantSuspended    = "TENANT_SUSPENDED"
	ErrorCodeUserNotFound       = "USER_NOT_FOUND"
	ErrorCodeInvalidCredentials = "INVALID_CREDENTIALS"
	ErrorCodeKBNotFound         = "KB_NOT_FOUND"
	ErrorCodeSessionNotFound    = "SESSION_NOT_FOUND"
	ErrorCodeRetrievalFailed    = "RETRIEVAL_FAILED"
	ErrorCodeAgentNotFound      = "AGENT_NOT_FOUND"
)

// ========================================
// 常用错误构造函数
// ========================================

// NotFoundError 创建未找到错误
func NotFoundError(resource string, err error) *DomainError {
	return NewDomainError(ErrorCodeNotFound, resource+" not found", err)
}

// AlreadyExistsError 创建已存在错误
func AlreadyExistsError(resource string, err error) *DomainError {
	return NewDomainError(ErrorCodeAlreadyExists, resource+" already exists", err)
}

// InvalidInputError 创建无效输入错误
func InvalidInputError(message string, err error) *DomainError {
	return NewDomainError(ErrorCodeInvalidInput, message, err)
}

// UnauthorizedError 创建未授权错误
func UnauthorizedError(message string) *DomainError {
	return NewDomainError(ErrorCodeUnauthorized, message, nil)
}

// ForbiddenError 创建禁止访问错误
func ForbiddenError(message string) *DomainError {
	return NewDomainError(ErrorCodeForbidden, message, nil)
}

// InternalError 创建内部错误
func InternalError(message string, err error) *DomainError {
	return NewDomainError(ErrorCodeInternal, message, err)
}

// ========================================
// 辅助函数 (BizError)
// ========================================

// IsBizError 是否是业务错误
func IsBizError(err error) bool {
	_, ok := err.(*BizError)
	return ok
}

// GetBizError 获取业务错误
func GetBizError(err error) (*BizError, bool) {
	bizErr, ok := err.(*BizError)
	return bizErr, ok
}

// MustBizError 获取业务错误，如果不是则 panic
func MustBizError(err error) *BizError {
	bizErr, ok := err.(*BizError)
	if !ok {
		panic(fmt.Sprintf("not a BizError: %v", err))
	}
	return bizErr
}

// GetCode 获取错误码
func GetCode(err error) int {
	if bizErr, ok := err.(*BizError); ok {
		return bizErr.Code
	}
	return 1 // 默认未知错误
}

// GetMessage 获取错误信息
func GetMessage(err error) string {
	if bizErr, ok := err.(*BizError); ok {
		return bizErr.Message
	}
	return "未知错误"
}
