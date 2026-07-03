// Package mysql 指标语义层持久化模型（agent_semantic_* 表）。
package mysql

import (
	"encoding/json"
	"time"

	"link/internal/model/semantic"
)

// ========================================
// SemanticModelModel 语义模型（容器 + 版本锚点）
// ========================================

// SemanticModelModel 语义模型 GORM 模型。
type SemanticModelModel struct {
	ID          string     `gorm:"column:id;primaryKey;type:varchar(64)" json:"id"`
	TenantID    int64      `gorm:"column:tenant_id;not null;index:idx_sem_tenant_name" json:"tenant_id"`
	Name        string     `gorm:"column:name;not null;type:varchar(128);index:idx_sem_tenant_name" json:"name"`
	Description string     `gorm:"column:description;type:text" json:"description,omitempty"`
	Version     int        `gorm:"column:version;not null;default:1" json:"version"`
	Status      string     `gorm:"column:status;not null;type:varchar(32);index:idx_sem_status" json:"status"`
	CreatedAt   time.Time  `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at;not null" json:"updated_at"`
	DeletedAt   *time.Time `gorm:"column:deleted_at;index" json:"deleted_at,omitempty"`
}

// TableName 指定表名。
func (SemanticModelModel) TableName() string { return "agent_semantic_models" }

// ToDomain 转换为领域实体。
func (m *SemanticModelModel) ToDomain() *semantic.SemanticModel {
	return &semantic.SemanticModel{
		ID:          m.ID,
		TenantID:    m.TenantID,
		Name:        m.Name,
		Description: m.Description,
		Version:     m.Version,
		Status:      semantic.ModelStatus(m.Status),
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
		DeletedAt:   m.DeletedAt,
	}
}

// FromDomainSemanticModel 从领域实体转换为模型。
func FromDomainSemanticModel(e *semantic.SemanticModel) *SemanticModelModel {
	return &SemanticModelModel{
		ID:          e.ID,
		TenantID:    e.TenantID,
		Name:        e.Name,
		Description: e.Description,
		Version:     e.Version,
		Status:      string(e.Status),
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
		DeletedAt:   e.DeletedAt,
	}
}

// ========================================
// SemanticLogicalTableModel 逻辑表
// ========================================

// SemanticLogicalTableModel 逻辑表 GORM 模型。
type SemanticLogicalTableModel struct {
	ID            string    `gorm:"column:id;primaryKey;type:varchar(64)" json:"id"`
	ModelID       string    `gorm:"column:model_id;not null;type:varchar(64);index:idx_sem_lt_model" json:"model_id"`
	Name          string    `gorm:"column:name;not null;type:varchar(128)" json:"name"`
	PhysicalTable string    `gorm:"column:physical_table;not null;type:varchar(128)" json:"physical_table"`
	DatabaseID    string    `gorm:"column:database_id;type:varchar(128)" json:"database_id,omitempty"`
	PrimaryKey    string    `gorm:"column:primary_key;type:varchar(128)" json:"primary_key,omitempty"`
	Description   string    `gorm:"column:description;type:text" json:"description,omitempty"`
	CreatedAt     time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at;not null" json:"updated_at"`
}

// TableName 指定表名。
func (SemanticLogicalTableModel) TableName() string { return "agent_semantic_logical_tables" }

// ToDomain 转换为领域实体。
func (m *SemanticLogicalTableModel) ToDomain() *semantic.LogicalTable {
	return &semantic.LogicalTable{
		ID:            m.ID,
		ModelID:       m.ModelID,
		Name:          m.Name,
		PhysicalTable: m.PhysicalTable,
		DatabaseID:    m.DatabaseID,
		PrimaryKey:    m.PrimaryKey,
		Description:   m.Description,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
}

// FromDomainSemanticLogicalTable 从领域实体转换为模型。
func FromDomainSemanticLogicalTable(e *semantic.LogicalTable) *SemanticLogicalTableModel {
	return &SemanticLogicalTableModel{
		ID:            e.ID,
		ModelID:       e.ModelID,
		Name:          e.Name,
		PhysicalTable: e.PhysicalTable,
		DatabaseID:    e.DatabaseID,
		PrimaryKey:    e.PrimaryKey,
		Description:   e.Description,
		CreatedAt:     e.CreatedAt,
		UpdatedAt:     e.UpdatedAt,
	}
}

// ========================================
// SemanticDimensionModel 维度
// ========================================

// SemanticDimensionModel 维度 GORM 模型。
type SemanticDimensionModel struct {
	ID             string    `gorm:"column:id;primaryKey;type:varchar(64)" json:"id"`
	ModelID        string    `gorm:"column:model_id;not null;type:varchar(64);index:idx_sem_dim_model" json:"model_id"`
	LogicalTableID string    `gorm:"column:logical_table_id;not null;type:varchar(64)" json:"logical_table_id"`
	Name           string    `gorm:"column:name;not null;type:varchar(128)" json:"name"`
	Expr           string    `gorm:"column:expr;not null;type:varchar(512)" json:"expr"`
	DataType       string    `gorm:"column:data_type;type:varchar(64)" json:"data_type,omitempty"`
	Description    string    `gorm:"column:description;type:text" json:"description,omitempty"`
	Synonyms       []byte    `gorm:"column:synonyms;type:json" json:"synonyms,omitempty"`
	CreatedAt      time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null" json:"updated_at"`
}

// TableName 指定表名。
func (SemanticDimensionModel) TableName() string { return "agent_semantic_dimensions" }

// ToDomain 转换为领域实体。
func (m *SemanticDimensionModel) ToDomain() *semantic.Dimension {
	return &semantic.Dimension{
		ID:             m.ID,
		ModelID:        m.ModelID,
		LogicalTableID: m.LogicalTableID,
		Name:           m.Name,
		Expr:           m.Expr,
		DataType:       m.DataType,
		Description:    m.Description,
		Synonyms:       decodeStringSlice(m.Synonyms),
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
}

// FromDomainSemanticDimension 从领域实体转换为模型。
func FromDomainSemanticDimension(e *semantic.Dimension) *SemanticDimensionModel {
	return &SemanticDimensionModel{
		ID:             e.ID,
		ModelID:        e.ModelID,
		LogicalTableID: e.LogicalTableID,
		Name:           e.Name,
		Expr:           e.Expr,
		DataType:       e.DataType,
		Description:    e.Description,
		Synonyms:       encodeStringSlice(e.Synonyms),
		CreatedAt:      e.CreatedAt,
		UpdatedAt:      e.UpdatedAt,
	}
}

// ========================================
// SemanticMeasureModel 度量
// ========================================

// SemanticMeasureModel 度量 GORM 模型。
type SemanticMeasureModel struct {
	ID             string    `gorm:"column:id;primaryKey;type:varchar(64)" json:"id"`
	ModelID        string    `gorm:"column:model_id;not null;type:varchar(64);index:idx_sem_mea_model" json:"model_id"`
	LogicalTableID string    `gorm:"column:logical_table_id;not null;type:varchar(64)" json:"logical_table_id"`
	Name           string    `gorm:"column:name;not null;type:varchar(128)" json:"name"`
	Expr           string    `gorm:"column:expr;not null;type:varchar(512)" json:"expr"`
	Aggregation    string    `gorm:"column:aggregation;not null;type:varchar(32);default:sum" json:"aggregation"`
	DataType       string    `gorm:"column:data_type;type:varchar(64)" json:"data_type,omitempty"`
	Description    string    `gorm:"column:description;type:text" json:"description,omitempty"`
	CreatedAt      time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null" json:"updated_at"`
}

// TableName 指定表名。
func (SemanticMeasureModel) TableName() string { return "agent_semantic_measures" }

// ToDomain 转换为领域实体。
func (m *SemanticMeasureModel) ToDomain() *semantic.Measure {
	return &semantic.Measure{
		ID:             m.ID,
		ModelID:        m.ModelID,
		LogicalTableID: m.LogicalTableID,
		Name:           m.Name,
		Expr:           m.Expr,
		Aggregation:    semantic.Aggregation(m.Aggregation),
		DataType:       m.DataType,
		Description:    m.Description,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
}

// FromDomainSemanticMeasure 从领域实体转换为模型。
func FromDomainSemanticMeasure(e *semantic.Measure) *SemanticMeasureModel {
	return &SemanticMeasureModel{
		ID:             e.ID,
		ModelID:        e.ModelID,
		LogicalTableID: e.LogicalTableID,
		Name:           e.Name,
		Expr:           e.Expr,
		Aggregation:    string(e.Aggregation),
		DataType:       e.DataType,
		Description:    e.Description,
		CreatedAt:      e.CreatedAt,
		UpdatedAt:      e.UpdatedAt,
	}
}

// ========================================
// SemanticMetricModel 指标（口径集中定义）
// ========================================

// SemanticMetricModel 指标 GORM 模型。
type SemanticMetricModel struct {
	ID          string    `gorm:"column:id;primaryKey;type:varchar(64)" json:"id"`
	ModelID     string    `gorm:"column:model_id;not null;type:varchar(64);index:idx_sem_met_model" json:"model_id"`
	Name        string    `gorm:"column:name;not null;type:varchar(128)" json:"name"`
	Expr        string    `gorm:"column:expr;not null;type:varchar(1024)" json:"expr"`
	Caliber     string    `gorm:"column:caliber;type:text" json:"caliber,omitempty"`
	Format      string    `gorm:"column:format;type:varchar(64)" json:"format,omitempty"`
	Description string    `gorm:"column:description;type:text" json:"description,omitempty"`
	Synonyms    []byte    `gorm:"column:synonyms;type:json" json:"synonyms,omitempty"`
	CreatedAt   time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at;not null" json:"updated_at"`
}

// TableName 指定表名。
func (SemanticMetricModel) TableName() string { return "agent_semantic_metrics" }

// ToDomain 转换为领域实体。
func (m *SemanticMetricModel) ToDomain() *semantic.Metric {
	return &semantic.Metric{
		ID:          m.ID,
		ModelID:     m.ModelID,
		Name:        m.Name,
		Expr:        m.Expr,
		Caliber:     m.Caliber,
		Format:      m.Format,
		Description: m.Description,
		Synonyms:    decodeStringSlice(m.Synonyms),
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

// FromDomainSemanticMetric 从领域实体转换为模型。
func FromDomainSemanticMetric(e *semantic.Metric) *SemanticMetricModel {
	return &SemanticMetricModel{
		ID:          e.ID,
		ModelID:     e.ModelID,
		Name:        e.Name,
		Expr:        e.Expr,
		Caliber:     e.Caliber,
		Format:      e.Format,
		Description: e.Description,
		Synonyms:    encodeStringSlice(e.Synonyms),
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
}

// ========================================
// SemanticRelationModel 表间关系（JOIN）
// ========================================

// SemanticRelationModel 表间关系 GORM 模型。
type SemanticRelationModel struct {
	ID            string    `gorm:"column:id;primaryKey;type:varchar(64)" json:"id"`
	ModelID       string    `gorm:"column:model_id;not null;type:varchar(64);index:idx_sem_rel_model" json:"model_id"`
	LeftTableID   string    `gorm:"column:left_table_id;not null;type:varchar(64)" json:"left_table_id"`
	RightTableID  string    `gorm:"column:right_table_id;not null;type:varchar(64)" json:"right_table_id"`
	JoinType      string    `gorm:"column:join_type;not null;type:varchar(16);default:inner" json:"join_type"`
	JoinCondition string    `gorm:"column:join_condition;not null;type:varchar(512)" json:"join_condition"`
	Description   string    `gorm:"column:description;type:text" json:"description,omitempty"`
	CreatedAt     time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at;not null" json:"updated_at"`
}

// TableName 指定表名。
func (SemanticRelationModel) TableName() string { return "agent_semantic_relations" }

// ToDomain 转换为领域实体。
func (m *SemanticRelationModel) ToDomain() *semantic.Relation {
	return &semantic.Relation{
		ID:            m.ID,
		ModelID:       m.ModelID,
		LeftTableID:   m.LeftTableID,
		RightTableID:  m.RightTableID,
		JoinType:      semantic.JoinType(m.JoinType),
		JoinCondition: m.JoinCondition,
		Description:   m.Description,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
}

// FromDomainSemanticRelation 从领域实体转换为模型。
func FromDomainSemanticRelation(e *semantic.Relation) *SemanticRelationModel {
	return &SemanticRelationModel{
		ID:            e.ID,
		ModelID:       e.ModelID,
		LeftTableID:   e.LeftTableID,
		RightTableID:  e.RightTableID,
		JoinType:      string(e.JoinType),
		JoinCondition: e.JoinCondition,
		Description:   e.Description,
		CreatedAt:     e.CreatedAt,
		UpdatedAt:     e.UpdatedAt,
	}
}

// decodeStringSlice 把 JSON 列解码为字符串切片（空/非法返回 nil）。
func decodeStringSlice(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

// encodeStringSlice 把字符串切片编码为 JSON 列（空返回 nil，不落 "[]" 噪声）。
func encodeStringSlice(in []string) []byte {
	if len(in) == 0 {
		return nil
	}
	data, err := json.Marshal(in)
	if err != nil {
		return nil
	}
	return data
}
