// Package mysql provides MySQL persistence implementation for audit logs
package mysql

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"cognida/internal/model/audit"
)

// auditRepository 审计日志仓储
type auditRepository struct {
	*BaseRepository
}

// NewAuditRepository 创建审计日志仓储
func NewAuditRepository(db *gorm.DB) audit.Repository {
	return &auditRepository{
		BaseRepository: NewBaseRepository(db, false),
	}
}

// Create 创建日志记录
func (r *auditRepository) Create(ctx context.Context, log *audit.AuditLog) error {
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}
	model := AuditLogModel{}.FromDomain(log)
	return r.WithContext(ctx).Create(model).Error
}

// FindByID 根据ID查找
func (r *auditRepository) FindByID(ctx context.Context, id int64) (*audit.AuditLog, error) {
	var model AuditLogModel
	err := r.WithContext(ctx).Where("id = ?", id).First(&model).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("日志不存在")
		}
		return nil, fmt.Errorf("查询失败: %w", err)
	}
	return model.ToDomain(), nil
}

// Query 查询日志列表
func (r *auditRepository) Query(ctx context.Context, q *audit.Query) ([]*audit.AuditLog, int64, error) {
	var models []*AuditLogModel
	var total int64

	db := r.WithContext(ctx).Model(&AuditLogModel{})

	// 租户过滤
	if q.TenantID != nil {
		db = db.Where("tenant_id = ?", *q.TenantID)
	}

	// 用户过滤
	if q.UserID != nil {
		db = db.Where("user_id = ?", *q.UserID)
	}

	// 模块过滤
	if q.Module != "" {
		db = db.Where("module = ?", q.Module)
	}

	// 操作过滤
	if q.Action != "" {
		db = db.Where("action = ?", q.Action)
	}

	// 资源类型过滤
	if q.ResourceType != "" {
		db = db.Where("resource_type = ?", q.ResourceType)
	}

	// 状态过滤
	if q.Status != "" {
		db = db.Where("status = ?", q.Status)
	}

	// 请求ID过滤
	if q.RequestID != nil {
		db = db.Where("request_id = ?", *q.RequestID)
	}

	// 关键词搜索
	if q.Keywords != nil {
		keyword := "%" + *q.Keywords + "%"
		db = db.Where("resource_name LIKE ? OR error_message LIKE ?", keyword, keyword)
	}

	// 时间范围
	if q.StartTime != nil {
		db = db.Where("created_at >= ?", *q.StartTime)
	}
	if q.EndTime != nil {
		db = db.Where("created_at <= ?", *q.EndTime)
	}

	// 统计
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计失败: %w", err)
	}

	// 分页（使用 Query 的 Validate 方法已验证）
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 {
		q.PageSize = 20
	}
	if q.PageSize > 100 {
		q.PageSize = 100
	}

	offset := (q.Page - 1) * q.PageSize
	err := db.Order("created_at DESC").Offset(offset).Limit(q.PageSize).Find(&models).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询失败: %w", err)
	}

	return ToDomainList(models), total, nil
}

// GetStats 获取统计数据
func (r *auditRepository) GetStats(ctx context.Context, tenantID *int64, days int) (*audit.Stats, error) {
	stats := &audit.Stats{
		ByModule: make(map[string]int64),
	}

	startTime := time.Now().AddDate(0, 0, -days)

	db := r.WithContext(ctx).Model(&AuditLogModel{}).
		Where("created_at >= ?", startTime)

	if tenantID != nil {
		db = db.Where("tenant_id = ?", *tenantID)
	}

	// 总数
	if err := db.Count(&stats.Total).Error; err != nil {
		return nil, fmt.Errorf("统计失败: %w", err)
	}

	// 按模块统计
	var moduleStats []struct {
		Module string
		Count  int64
	}
	if err := db.Select("module, count(*) as count").Group("module").
		Order("count DESC").Scan(&moduleStats).Error; err == nil {
		for _, s := range moduleStats {
			stats.ByModule[s.Module] = s.Count
		}
	}

	// 按日统计
	var dayStats []struct {
		Date  string
		Count int64
	}
	if err := db.Select("DATE(created_at) as date, count(*) as count").
		Group("DATE(created_at)").
		Order("date DESC").
		Scan(&dayStats).Error; err == nil {
		for _, s := range dayStats {
			stats.ByDay = append(stats.ByDay, audit.DayCount{
				Date:  s.Date,
				Count: s.Count,
			})
		}
	}

	return stats, nil
}

// DeleteBefore 删除指定日期之前的日志（归档用）
func (r *auditRepository) DeleteBefore(ctx context.Context, date time.Time) (int64, error) {
	result := r.WithContext(ctx).
		Where("created_at < ?", date).
		Delete(&AuditLogModel{})
	if result.Error != nil {
		return 0, fmt.Errorf("删除失败: %w", result.Error)
	}
	return result.RowsAffected, nil
}
