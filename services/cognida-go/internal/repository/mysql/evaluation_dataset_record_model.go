// Package persistence MySQL 持久化模型 - 数据集样本记录相关
package mysql

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"cognida/internal/model/evaluation"
)

// nullableJSON 让 type:json 列在空值时写 NULL 而非非法空串——MySQL 的 JSON 列
// 拒绝 ”（Error 3140），而 GORM 对普通 string 零值会写入 ”。用作 relevant_pids /
// expected_tools / expected_steps 的存储类型，空切片场景下统一落 NULL。
type nullableJSON string

// Value 实现 driver.Valuer：空串落 NULL，否则原样写入。
func (j nullableJSON) Value() (driver.Value, error) {
	if j == "" {
		return nil, nil
	}
	return string(j), nil
}

// Scan 实现 sql.Scanner：NULL/[]byte/string 统一归一为字符串。
func (j *nullableJSON) Scan(v any) error {
	switch s := v.(type) {
	case nil:
		*j = ""
	case []byte:
		*j = nullableJSON(s)
	case string:
		*j = nullableJSON(s)
	}
	return nil
}

// ========================================
// DatasetRecordModel 数据集样本记录模型
// ========================================

// DatasetRecordModel 数据集样本记录 GORM 模型
type DatasetRecordModel struct {
	ID              int64        `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	DatasetID       string       `gorm:"column:dataset_id;not null;type:varchar(64);index:idx_dataset_id,idx_tenant_dataset" json:"dataset_id"`
	TenantID        int64        `gorm:"column:tenant_id;not null;index:idx_tenant_id,idx_tenant_dataset" json:"tenant_id"`
	Question        string       `gorm:"column:question;not null;type:text" json:"question"`
	ReferenceAnswer string       `gorm:"column:reference_answer;not null;type:text" json:"reference_answer"`
	RelevantPIDs    nullableJSON `gorm:"column:relevant_pids;type:json" json:"relevant_pids,omitempty"`
	Context         string       `gorm:"column:context;type:text" json:"context,omitempty"`
	// Agent 评测期望标注（JSON 数组；QA/RAG 样本为空 → NULL）
	ExpectedTools nullableJSON `gorm:"column:expected_tools;type:json" json:"expected_tools,omitempty"`
	ExpectedSteps nullableJSON `gorm:"column:expected_steps;type:json" json:"expected_steps,omitempty"`
	CreatedAt     time.Time    `gorm:"column:created_at;not null" json:"created_at"`
}

// decodeJSONStringSlice 解析 JSON 数组字符串到 []string（空串安全跳过）。
func decodeJSONStringSlice(raw nullableJSON, dst *[]string) {
	if raw != "" {
		json.Unmarshal([]byte(raw), dst)
	}
}

// encodeJSONStringSlice 序列化 []string 为 JSON 字符串（空切片返回空串 → 落 NULL）。
func encodeJSONStringSlice(src []string) nullableJSON {
	if len(src) == 0 {
		return ""
	}
	if data, err := json.Marshal(src); err == nil {
		return nullableJSON(data)
	}
	return ""
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
	decodeJSONStringSlice(m.ExpectedTools, &record.ExpectedTools)
	decodeJSONStringSlice(m.ExpectedSteps, &record.ExpectedSteps)

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
	decodeJSONStringSlice(m.ExpectedTools, &pair.ExpectedTools)
	decodeJSONStringSlice(m.ExpectedSteps, &pair.ExpectedSteps)

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
			model.RelevantPIDs = nullableJSON(data)
		}
	}
	model.ExpectedTools = encodeJSONStringSlice(record.ExpectedTools)
	model.ExpectedSteps = encodeJSONStringSlice(record.ExpectedSteps)

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
			model.RelevantPIDs = nullableJSON(data)
		}
	}
	model.ExpectedTools = encodeJSONStringSlice(pair.ExpectedTools)
	model.ExpectedSteps = encodeJSONStringSlice(pair.ExpectedSteps)

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
