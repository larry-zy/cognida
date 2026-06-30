// Package persistence MySQL 持久化模型 - 数据集样本记录相关
package mysql

import (
	"encoding/json"
	"time"

	"link/internal/model/evaluation"
)

// ========================================
// DatasetRecordModel 数据集样本记录模型
// ========================================

// DatasetRecordModel 数据集样本记录 GORM 模型
type DatasetRecordModel struct {
	ID              int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	DatasetID       string    `gorm:"column:dataset_id;not null;index:idx_dataset_id,idx_tenant_dataset" json:"dataset_id"`
	TenantID        int64     `gorm:"column:tenant_id;not null;index:idx_tenant_id,idx_tenant_dataset" json:"tenant_id"`
	Question        string    `gorm:"column:question;not null;type:text" json:"question"`
	ReferenceAnswer string    `gorm:"column:reference_answer;not null;type:text" json:"reference_answer"`
	RelevantPIDs    string    `gorm:"column:relevant_pids;type:json" json:"relevant_pids,omitempty"`
	Context         string    `gorm:"column:context;type:text" json:"context,omitempty"`
	CreatedAt       time.Time `gorm:"column:created_at;not null" json:"created_at"`
}

// TableName 指定表名
func (DatasetRecordModel) TableName() string {
	return "evaluation_dataset_records"
}

// ToDomain 转换为领域实体
func (m *DatasetRecordModel) ToDomain() *evaluation.DatasetRecord {
	record := &evaluation.DatasetRecord{
		ID:              m.ID,
		DatasetID:       m.DatasetID,
		TenantID:        m.TenantID,
		Question:        m.Question,
		ReferenceAnswer: m.ReferenceAnswer,
		Context:         m.Context,
		CreatedAt:       m.CreatedAt,
	}

	// 解析 relevant_pids JSON 字段
	if m.RelevantPIDs != "" {
		json.Unmarshal([]byte(m.RelevantPIDs), &record.RelevantPIDs)
	}

	return record
}

// ToDomainWithQAPair 转换为 QAPair（用于评测）
func (m *DatasetRecordModel) ToDomainWithQAPair() *evaluation.QAPair {
	pair := &evaluation.QAPair{
		Question:        m.Question,
		ReferenceAnswer: m.ReferenceAnswer,
		Context:         m.Context,
	}

	// 解析 relevant_pids JSON 字段
	if m.RelevantPIDs != "" {
		json.Unmarshal([]byte(m.RelevantPIDs), &pair.RelevantPIDs)
	}

	return pair
}

// FromDomainDatasetRecord 从领域实体转换为模型
func FromDomainDatasetRecord(record *evaluation.DatasetRecord) *DatasetRecordModel {
	model := &DatasetRecordModel{
		ID:              record.ID,
		DatasetID:       record.DatasetID,
		TenantID:        record.TenantID,
		Question:        record.Question,
		ReferenceAnswer: record.ReferenceAnswer,
		Context:         record.Context,
		CreatedAt:       record.CreatedAt,
	}

	// 序列化 relevant_pids 为 JSON
	if len(record.RelevantPIDs) > 0 {
		if data, err := json.Marshal(record.RelevantPIDs); err == nil {
			model.RelevantPIDs = string(data)
		}
	}

	return model
}

// FromQAPair 从 QAPair 转换为模型（用于批量导入）
func FromQAPair(datasetID string, tenantID int64, pair *evaluation.QAPair) *DatasetRecordModel {
	model := &DatasetRecordModel{
		DatasetID:       datasetID,
		TenantID:        tenantID,
		Question:        pair.Question,
		ReferenceAnswer: pair.ReferenceAnswer,
		Context:         pair.Context,
	}

	// 序列化 relevant_pids 为 JSON
	if len(pair.RelevantPIDs) > 0 {
		if data, err := json.Marshal(pair.RelevantPIDs); err == nil {
			model.RelevantPIDs = string(data)
		}
	}

	return model
}

// ToDomainList 批量转换为领域实体
func ToDomainDatasetRecordList(models []*DatasetRecordModel) []*evaluation.DatasetRecord {
	records := make([]*evaluation.DatasetRecord, len(models))
	for i, m := range models {
		records[i] = m.ToDomain()
	}
	return records
}

// ToDomainQAPairList 批量转换为 QAPair
func ToDomainQAPairList(models []*DatasetRecordModel) []*evaluation.QAPair {
	pairs := make([]*evaluation.QAPair, len(models))
	for i, m := range models {
		pairs[i] = m.ToDomainWithQAPair()
	}
	return pairs
}
