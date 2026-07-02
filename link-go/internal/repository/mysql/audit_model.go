// Package mysql provides the MySQL persistence implementation for audit logs
package mysql

import (
	"time"

	domaintenant "link/internal/model/audit"
)

// AuditLogModel 审计日志数据库模型
//
// 包含 GORM 持久化细节，与 Domain 层的 AuditLog 实体分离。
type AuditLogModel struct {
	ID           int64     `gorm:"primaryKey"`
	TenantID     *int64    `gorm:"index"`
	UserID       *int64    `gorm:"index"`
	Module       string    `gorm:"index:idx_module_action;size:50;comment:模块"`
	Action       string    `gorm:"index:idx_module_action;size:100;comment:操作"`
	ResourceType string    `gorm:"size:50;comment:资源类型"`
	ResourceID   *string   `gorm:"size:100;comment:资源ID"`
	ResourceName *string   `gorm:"size:255;comment:资源名称"`
	Details      string    `gorm:"type:json;comment:详情"`
	Status       string    `gorm:"index;size:20;default:success;comment:状态"`
	ErrorMessage *string   `gorm:"type:text"`
	IPAddress    *string   `gorm:"size:45"`
	RequestID    *string   `gorm:"size:64;index"`
	DurationMs   *int      `gorm:"comment:耗时ms"`
	CreatedAt    time.Time `gorm:"autoCreateTime;index"`
}

// TableName 指定表名
func (AuditLogModel) TableName() string {
	return "audit_logs"
}

// ToDomain 转换为领域实体
func (m *AuditLogModel) ToDomain() *domaintenant.AuditLog {
	return &domaintenant.AuditLog{
		ID:           m.ID,
		TenantID:     m.TenantID,
		UserID:       m.UserID,
		Module:       m.Module,
		Action:       m.Action,
		ResourceType: m.ResourceType,
		ResourceID:   m.ResourceID,
		ResourceName: m.ResourceName,
		Details:      m.Details,
		Status:       m.Status,
		ErrorMessage: m.ErrorMessage,
		IPAddress:    m.IPAddress,
		RequestID:    m.RequestID,
		DurationMs:   m.DurationMs,
		CreatedAt:    m.CreatedAt,
	}
}

// FromDomain 从领域实体转换为数据库模型
func (AuditLogModel) FromDomain(entity *domaintenant.AuditLog) *AuditLogModel {
	return &AuditLogModel{
		ID:           entity.ID,
		TenantID:     entity.TenantID,
		UserID:       entity.UserID,
		Module:       entity.Module,
		Action:       entity.Action,
		ResourceType: entity.ResourceType,
		ResourceID:   entity.ResourceID,
		ResourceName: entity.ResourceName,
		Details:      entity.Details,
		Status:       entity.Status,
		ErrorMessage: entity.ErrorMessage,
		IPAddress:    entity.IPAddress,
		RequestID:    entity.RequestID,
		DurationMs:   entity.DurationMs,
		CreatedAt:    entity.CreatedAt,
	}
}

// ToDomainList 批量转换为领域实体
func ToDomainList(models []*AuditLogModel) []*domaintenant.AuditLog {
	result := make([]*domaintenant.AuditLog, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}
	return result
}
