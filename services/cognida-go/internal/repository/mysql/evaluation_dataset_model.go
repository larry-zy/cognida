// Package persistence MySQL 持久化模型 - 数据集相关
package mysql

import (
	"time"

	"cognida/internal/model/evaluation"
)

// ========================================
// DatasetModel 数据集模型
// ========================================

// DatasetModel 数据集 GORM 模型
type DatasetModel struct {
	ID             int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	DatasetID      string    `gorm:"column:dataset_id;not null;type:varchar(64);uniqueIndex:uk_dataset_id" json:"dataset_id"`
	TenantID       int64     `gorm:"column:tenant_id;not null;index:idx_tenant_id,idx_tenant_deleted" json:"tenant_id"`
	UserID         int64     `gorm:"column:user_id;not null;index" json:"user_id"`
	Name           string    `gorm:"column:name;not null;type:varchar(255)" json:"name"`
	Description    string    `gorm:"column:description;type:text" json:"description"`
	Type           string    `gorm:"column:type;not null;type:varchar(32);index:idx_type" json:"type"`
	EvaluationType string    `gorm:"column:evaluation_type;not null;type:varchar(32);index:idx_evaluation_type" json:"evaluation_type"`
	QACount        int       `gorm:"column:qa_count;not null;default:0" json:"qa_count"`
	CreatedAt      time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null" json:"updated_at"`
	DeletedAt      *time.Time `gorm:"column:deleted_at;index:idx_deleted_at,idx_tenant_deleted" json:"deleted_at,omitempty"`
}

// TableName 指定表名
func (DatasetModel) TableName() string {
	return "evaluation_datasets"
}

// ToDomain 转换为领域实体
func (m *DatasetModel) ToDomain() *evaluation.Dataset {
	return &evaluation.Dataset{
		ID:             m.ID,
		DatasetID:      m.DatasetID,
		TenantID:       m.TenantID,
		UserID:         m.UserID,
		Name:           m.Name,
		Description:    m.Description,
		Type:           evaluation.DatasetType(m.Type),
		EvaluationType: evaluation.EvaluationType(m.EvaluationType),
		QACount:        m.QACount,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
		DeletedAt:      m.DeletedAt,
	}
}

// FromDomain 从领域实体转换为模型
func FromDomainDataset(dataset *evaluation.Dataset) *DatasetModel {
	return &DatasetModel{
		ID:             dataset.ID,
		DatasetID:      dataset.DatasetID,
		TenantID:       dataset.TenantID,
		UserID:         dataset.UserID,
		Name:           dataset.Name,
		Description:    dataset.Description,
		Type:           string(dataset.Type),
		EvaluationType: string(dataset.EvaluationType),
		QACount:        dataset.QACount,
		CreatedAt:      dataset.CreatedAt,
		UpdatedAt:      dataset.UpdatedAt,
		DeletedAt:      dataset.DeletedAt,
	}
}

// ToDomainList 批量转换为领域实体
func ToDomainDatasetList(models []*DatasetModel) []*evaluation.Dataset {
	datasets := make([]*evaluation.Dataset, len(models))
	for i, m := range models {
		datasets[i] = m.ToDomain()
	}
	return datasets
}
