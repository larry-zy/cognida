// Package mysql 提供操作审计仓储的 MySQL 实现。
package mysql

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"link/internal/model/agent/operations"
)

// ========================================
// OperationAuditModel 操作审计（agent_operation_audit）
// ========================================

// OperationAuditModel 操作审计 GORM 模型：记录所有写/ETL/导出与被拦截操作。
type OperationAuditModel struct {
	ID             int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TenantID       int64     `gorm:"column:tenant_id;not null;index:idx_opaudit_tenant_key" json:"tenant_id"`
	SessionID      string    `gorm:"column:session_id;type:varchar(64);index:idx_opaudit_session" json:"session_id"`
	RequestID      string    `gorm:"column:request_id;type:varchar(64);index:idx_opaudit_request" json:"request_id"`
	OperationType  string    `gorm:"column:operation_type;not null;type:varchar(16)" json:"operation_type"`
	Target         string    `gorm:"column:target;type:varchar(255)" json:"target"`
	SQLText        string    `gorm:"column:sql_text;type:text" json:"sql_text"`
	Params         string    `gorm:"column:params;type:text" json:"params"`
	IdempotencyKey string    `gorm:"column:idempotency_key;type:varchar(128);index:idx_opaudit_tenant_key" json:"idempotency_key"`
	Status         string    `gorm:"column:status;not null;type:varchar(24);index:idx_opaudit_status" json:"status"`
	Result         string    `gorm:"column:result;type:text" json:"result"`
	CreatedAt      time.Time `gorm:"column:created_at;not null;autoCreateTime" json:"created_at"`
}

// TableName 指定表名。
func (OperationAuditModel) TableName() string { return "agent_operation_audit" }

// ToDomain 转换为领域实体。
func (m *OperationAuditModel) ToDomain() *operations.OperationAudit {
	return &operations.OperationAudit{
		ID:             m.ID,
		TenantID:       m.TenantID,
		SessionID:      m.SessionID,
		RequestID:      m.RequestID,
		OperationType:  m.OperationType,
		Target:         m.Target,
		SQLText:        m.SQLText,
		Params:         m.Params,
		IdempotencyKey: m.IdempotencyKey,
		Status:         m.Status,
		Result:         m.Result,
		CreatedAt:      m.CreatedAt,
	}
}

// fromDomainOperationAudit 从领域实体转换为模型。
func fromDomainOperationAudit(e *operations.OperationAudit) *OperationAuditModel {
	return &OperationAuditModel{
		ID:             e.ID,
		TenantID:       e.TenantID,
		SessionID:      e.SessionID,
		RequestID:      e.RequestID,
		OperationType:  e.OperationType,
		Target:         e.Target,
		SQLText:        e.SQLText,
		Params:         e.Params,
		IdempotencyKey: e.IdempotencyKey,
		Status:         e.Status,
		Result:         e.Result,
		CreatedAt:      e.CreatedAt,
	}
}

// OperationAuditRepository 操作审计仓储 MySQL 实现。
type OperationAuditRepository struct {
	db *gorm.DB
}

// NewOperationAuditRepository 创建操作审计仓储。
func NewOperationAuditRepository(db *gorm.DB) *OperationAuditRepository {
	return &OperationAuditRepository{db: db}
}

// Record 追加一条审计记录（append-only）。
func (r *OperationAuditRepository) Record(ctx context.Context, audit *operations.OperationAudit) error {
	m := fromDomainOperationAudit(audit)
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return err
	}
	audit.ID = m.ID
	audit.CreatedAt = m.CreatedAt
	return nil
}

// FindByIdempotencyKey 按租户 + 幂等键查最近一条成功记录；无则返回 (nil, nil)。
func (r *OperationAuditRepository) FindByIdempotencyKey(ctx context.Context, tenantID int64, key string) (*operations.OperationAudit, error) {
	var m OperationAuditModel
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND idempotency_key = ? AND status = ?", tenantID, key, operations.StatusSuccess).
		Order("id DESC").
		First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return m.ToDomain(), nil
}
