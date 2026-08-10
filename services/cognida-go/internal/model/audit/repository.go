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

	// Query 查询日志列表（offset 分页，返回总数）
	Query(ctx context.Context, q *Query) ([]*AuditLog, int64, error)

	// QueryByCursor 游标（keyset）分页查询〔M5〕：按 created_at,id 倒序，
	// 以 q.Cursor 为锚点取下一页。返回本页记录与「下一页游标」；
	// nextCursor 为空表示已到末页。不返回总数（游标翻页不依赖 total）。
	QueryByCursor(ctx context.Context, q *Query) (logs []*AuditLog, nextCursor string, err error)

	// GetStats 获取统计数据
	GetStats(ctx context.Context, tenantID *int64, days int) (*Stats, error)

	// DeleteBefore 删除指定日期之前的日志（归档用）
	DeleteBefore(ctx context.Context, date time.Time) (int64, error)
}
