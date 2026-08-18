// Package audit provides the audit log domain entity definitions
package audit

import "time"

// AuditLog 审计日志实体
//
// 纯粹的领域实体，不包含持久化细节。
// 持久化相关（GORM 标签、表名）在 Infrastructure 层定义。
type AuditLog struct {
	ID           int64
	TenantID     *int64
	UserID       *int64
	Module       string
	Action       string
	ResourceType string
	ResourceID   *string
	ResourceName *string
	Details      string
	Status       string
	ErrorMessage *string
	IPAddress    *string
	RequestID    *string
	DurationMs   *int
	CreatedAt    time.Time
	// HasTrace 该 request_id 是否在 trace_spans 有调用链。非持久化，查询时由 handler 填充。
	HasTrace bool
}

// NewAuditLog 创建新的审计日志
func NewAuditLog(tenantID, userID *int64, module, action string) *AuditLog {
	return &AuditLog{
		TenantID:  tenantID,
		UserID:    userID,
		Module:    module,
		Action:    action,
		Status:    "success",
		CreatedAt: time.Now(),
	}
}

// WithResource 设置资源信息
func (a *AuditLog) WithResource(resourceType string, resourceID *string, resourceName *string) *AuditLog {
	a.ResourceType = resourceType
	a.ResourceID = resourceID
	a.ResourceName = resourceName
	return a
}

// WithDetails 设置详情
func (a *AuditLog) WithDetails(details string) *AuditLog {
	a.Details = details
	return a
}

// WithError 设置错误信息
func (a *AuditLog) WithError(err error) *AuditLog {
	if err != nil {
		a.Status = "error"
		errMsg := err.Error()
		a.ErrorMessage = &errMsg
	}
	return a
}

// WithRequest 设置请求信息
func (a *AuditLog) WithRequest(requestID, ipAddress *string) *AuditLog {
	a.RequestID = requestID
	a.IPAddress = ipAddress
	return a
}

// WithDuration 设置耗时
func (a *AuditLog) WithDuration(durationMs int) *AuditLog {
	a.DurationMs = &durationMs
	return a
}

// IsSuccess 判断操作是否成功
func (a *AuditLog) IsSuccess() bool {
	return a.Status == "success"
}

// ========================================
// 值对象（查询参数、统计等）
// ========================================

// Query 查询参数
type Query struct {
	TenantID     *int64
	UserID       *int64
	Module       string
	Action       string
	ResourceType string
	Status       string
	StartTime    *time.Time
	EndTime      *time.Time
	Keywords     *string
	RequestID    *string

	Page     int
	PageSize int

	// Cursor 不透明 keyset 游标〔M5〕：非空时走游标翻页（按 created_at,id 倒序），
	// 忽略 Page/offset；为空则维持既有 offset 分页。空串即首页。
	Cursor string
}

// Validate 验证查询参数
func (q *Query) Validate() error {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 1000 {
		q.PageSize = 20
	}
	return nil
}

// Stats 统计数据
type Stats struct {
	Total    int64
	ByModule map[string]int64
	ByDay    []DayCount
}

// DayCount 每日统计
type DayCount struct {
	Date  string
	Count int64
}
