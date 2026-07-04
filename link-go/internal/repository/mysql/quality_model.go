package mysql

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	domain_quality "link/internal/model/quality"
)

// ========================================
// QualityCheckRecordModel 质检记录模型
// ========================================

// QualityCheckRecordModel 质检记录 GORM 模型
type QualityCheckRecordModel struct {
	ID           string    `gorm:"column:id;primaryKey;type:varchar(64)" json:"id"`
	TenantID     int64     `gorm:"column:tenant_id;not null;index:idx_qc_tenant_type" json:"tenant_id"`
	UserID       int64     `gorm:"column:user_id;not null" json:"user_id"`
	Type         string    `gorm:"column:type;not null;type:varchar(32);index:idx_qc_tenant_type" json:"type"`
	SourceName   string    `gorm:"column:source_name;type:varchar(255)" json:"source_name"`
	OverallScore float64   `gorm:"column:overall_score;not null;default:0" json:"overall_score"`
	RecordCount  int32     `gorm:"column:record_count;not null;default:0" json:"record_count"`
	Summary      string    `gorm:"column:summary;type:varchar(512)" json:"summary"`
	ReportJSON   []byte    `gorm:"column:report_json;type:json" json:"report_json,omitempty"`
	CreatedAt    time.Time `gorm:"column:created_at;not null;index:idx_qc_created_at" json:"created_at"`
}

// TableName 指定表名
func (QualityCheckRecordModel) TableName() string {
	return "quality_check_records"
}

// toDomain 转换为领域实体
func (m *QualityCheckRecordModel) toDomain() *domain_quality.CheckRecord {
	return &domain_quality.CheckRecord{
		ID:           m.ID,
		TenantID:     m.TenantID,
		UserID:       m.UserID,
		Type:         domain_quality.CheckType(m.Type),
		SourceName:   m.SourceName,
		OverallScore: m.OverallScore,
		RecordCount:  m.RecordCount,
		Summary:      m.Summary,
		ReportJSON:   m.ReportJSON,
		CreatedAt:    m.CreatedAt,
	}
}

// fromDomain 从领域实体构建模型
func qualityModelFromDomain(r *domain_quality.CheckRecord) *QualityCheckRecordModel {
	return &QualityCheckRecordModel{
		ID:           r.ID,
		TenantID:     r.TenantID,
		UserID:       r.UserID,
		Type:         string(r.Type),
		SourceName:   r.SourceName,
		OverallScore: r.OverallScore,
		RecordCount:  r.RecordCount,
		Summary:      r.Summary,
		ReportJSON:   r.ReportJSON,
		CreatedAt:    r.CreatedAt,
	}
}

// ========================================
// qualityCheckRecordRepository 仓储实现
// ========================================

type qualityCheckRecordRepository struct {
	db *gorm.DB
}

// NewQualityCheckRecordRepository 创建质检记录仓储
func NewQualityCheckRecordRepository(db *gorm.DB) domain_quality.CheckRecordRepository {
	return &qualityCheckRecordRepository{db: db}
}

func (r *qualityCheckRecordRepository) Create(ctx context.Context, record *domain_quality.CheckRecord) error {
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}
	return r.db.WithContext(ctx).Create(qualityModelFromDomain(record)).Error
}

func (r *qualityCheckRecordRepository) Get(ctx context.Context, id string, tenantID int64) (*domain_quality.CheckRecord, error) {
	var m QualityCheckRecordModel
	err := r.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return m.toDomain(), nil
}

func (r *qualityCheckRecordRepository) List(ctx context.Context, filter domain_quality.ListFilter) ([]*domain_quality.CheckRecord, int64, error) {
	query := r.db.WithContext(ctx).Model(&QualityCheckRecordModel{}).
		Where("tenant_id = ?", filter.TenantID)
	if filter.Type != "" {
		query = query.Where("type = ?", string(filter.Type))
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	var models []QualityCheckRecordModel
	err := query.Order("created_at DESC").
		Limit(limit).Offset(filter.Offset).
		Find(&models).Error
	if err != nil {
		return nil, 0, err
	}

	records := make([]*domain_quality.CheckRecord, 0, len(models))
	for i := range models {
		records = append(records, models[i].toDomain())
	}
	return records, total, nil
}

func (r *qualityCheckRecordRepository) Delete(ctx context.Context, id string, tenantID int64) error {
	return r.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Delete(&QualityCheckRecordModel{}).Error
}
