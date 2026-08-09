// Package audit provides the audit log domain repository interfaces
package audit

import (
	"context"
	"time"
)

// Repository 审计日志仓储接口
type Repository interface {
	// Create 创建日志记录
	Create(ctx context.Context, log *AuditLog) error

	// FindByID 根据ID查找
	FindByID(ctx context.Context, id int64) (*AuditLog, error)

	// Query 查询日志列表
	Query(ctx context.Context, q *Query) ([]*AuditLog, int64, error)

	// GetStats 获取统计数据
	GetStats(ctx context.Context, tenantID *int64, days int) (*Stats, error)

	// DeleteBefore 删除指定日期之前的日志（归档用）
	DeleteBefore(ctx context.Context, date time.Time) (int64, error)
}
