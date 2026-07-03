// Package semantic 定义指标语义层（NL2Semantics）的领域实体与仓储接口。
//
// 指标语义层是「查询地基」：把受治理的逻辑表/维度/度量/指标/关系集中建模，
// 让查询从「LLM 裸拼 SQL」升级为「按既定口径经指标引擎生成 SQL」。
// 语义模型经 GORM 落 MySQL（agent_semantic_* 前缀），由 cmd/migrate-db 同步。
package semantic

import "time"

// ModelStatus 语义模型状态。
type ModelStatus string

const (
	// ModelStatusDraft 草稿：仍在建模，不参与线上查询。
	ModelStatusDraft ModelStatus = "draft"
	// ModelStatusActive 生效：作为查询主路径的受治理语义模型。
	ModelStatusActive ModelStatus = "active"
	// ModelStatusDeprecated 弃用：保留历史但不再选用。
	ModelStatusDeprecated ModelStatus = "deprecated"
)

// Aggregation 度量的默认聚合方式。
type Aggregation string

const (
	AggSum   Aggregation = "sum"
	AggCount Aggregation = "count"
	AggAvg   Aggregation = "avg"
	AggMax   Aggregation = "max"
	AggMin   Aggregation = "min"
	// AggNone 表示该度量本身即为可直接使用的表达式，不再外套聚合。
	AggNone Aggregation = "none"
)

// SemanticModel 是一个受治理的语义模型容器（一个业务域一套）。
//
// Version 是缓存失效的锚点：任何对该模型下逻辑表/维度/度量/指标/关系的
// 语义变更都应 bump 版本，Verified/Golden Query 缓存键含版本，版本变更即失效。
type SemanticModel struct {
	ID          string      `json:"id"`
	TenantID    int64       `json:"tenant_id"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Version     int         `json:"version"`
	Status      ModelStatus `json:"status"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
	DeletedAt   *time.Time  `json:"deleted_at,omitempty"`
}

// LogicalTable 逻辑表：语义名到物理表的映射。
type LogicalTable struct {
	ID            string    `json:"id"`
	ModelID       string    `json:"model_id"`
	Name          string    `json:"name"`           // 逻辑表名（业务语义）
	PhysicalTable string    `json:"physical_table"` // 物理表名
	DatabaseID    string    `json:"database_id,omitempty"`
	PrimaryKey    string    `json:"primary_key,omitempty"`
	Description   string    `json:"description,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Dimension 维度：可用于分组/过滤的语义列。
type Dimension struct {
	ID             string    `json:"id"`
	ModelID        string    `json:"model_id"`
	LogicalTableID string    `json:"logical_table_id"`
	Name           string    `json:"name"`           // 维度语义名（如"区域"）
	Expr           string    `json:"expr"`           // 物理列名或表达式
	DataType       string    `json:"data_type,omitempty"`
	Description    string    `json:"description,omitempty"`
	Synonyms       []string  `json:"synonyms,omitempty"` // 业务同义词，供术语 grounding
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Measure 度量：可聚合的原子列 + 默认聚合方式，是指标的构件。
type Measure struct {
	ID             string      `json:"id"`
	ModelID        string      `json:"model_id"`
	LogicalTableID string      `json:"logical_table_id"`
	Name           string      `json:"name"`        // 度量语义名（如"订单金额"）
	Expr           string      `json:"expr"`        // 物理列名或表达式
	Aggregation    Aggregation `json:"aggregation"` // 默认聚合
	DataType       string      `json:"data_type,omitempty"`
	Description    string      `json:"description,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// Metric 指标：受治理的、口径集中定义的可复用计算（如 revenue/gmv/churn）。
//
// Caliber（口径）是指标的核心资产：一处定义、处处一致，禁止 LLM 每次自行推断。
type Metric struct {
	ID          string    `json:"id"`
	ModelID     string    `json:"model_id"`
	Name        string    `json:"name"`             // 指标名（如"营收"）
	Expr        string    `json:"expr"`             // 计算表达式（可引用度量/列）
	Caliber     string    `json:"caliber,omitempty"` // 口径说明（自然语言，人读）
	Format      string    `json:"format,omitempty"`  // 展示格式（如 "currency" / "percent"）
	Description string    `json:"description,omitempty"`
	Synonyms    []string  `json:"synonyms,omitempty"` // 业务同义词，供术语 grounding
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// JoinType 表间关系的连接类型。
type JoinType string

const (
	JoinInner JoinType = "inner"
	JoinLeft  JoinType = "left"
	JoinRight JoinType = "right"
)

// Relation 表间关系：逻辑表之间的 JOIN 定义，供指标引擎跨表组装。
type Relation struct {
	ID            string    `json:"id"`
	ModelID       string    `json:"model_id"`
	LeftTableID   string    `json:"left_table_id"`
	RightTableID  string    `json:"right_table_id"`
	JoinType      JoinType  `json:"join_type"`
	JoinCondition string    `json:"join_condition"` // ON 表达式
	Description   string    `json:"description,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ModelBundle 是一个语义模型及其全部构件的聚合视图，供指标引擎一次性装配。
type ModelBundle struct {
	Model         *SemanticModel  `json:"model"`
	LogicalTables []*LogicalTable `json:"logical_tables"`
	Dimensions    []*Dimension    `json:"dimensions"`
	Measures      []*Measure      `json:"measures"`
	Metrics       []*Metric       `json:"metrics"`
	Relations     []*Relation     `json:"relations"`
}
