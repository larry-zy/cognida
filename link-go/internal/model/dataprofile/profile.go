// Package dataprofile 定义「列画像/数据事实」的领域实体与仓储端口。
//
// 数据事实（data facts）是只有扫描真实数据行才能知道的信息：某列的空值率、
// 基数（不同取值个数）、枚举实际取值分布等。语义层只覆盖被建模的表，自进化只覆盖
// 被问过的问题；而 Agent 首次探查一张未建模、未触达的冷表时，这些事实无从得知，
// 只能靠 LLM 现场猜测（如把 status='已完成' 直接拼进 SQL，匹配 0 行）。
//
// 本层以「物理坐标」（tenant + datasource + schema + table + column）为键，把画像
// 惰性写通（lazy write-through）沉淀进一张精简的 MySQL 表，再由 get_schema 读回拼给
// Agent，用真实事实替代猜测，提升 Text2SQL 准确率。非累积、带时效：每次探查覆盖式
// upsert 快照，过期或缺失才触发后台重算，只为被探查到的表买单。
package dataprofile

import (
	"context"
	"time"
)

// ValueFrequency 是枚举列的一个「取值-频次」样本，用于把低基数列的实际取值分布
// （如订单状态 completed/shipped/paid）暴露给 Agent，供其拼对过滤条件。
type ValueFrequency struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// ColumnProfile 是单列在某一时刻的数据事实快照，以物理坐标为主键唯一。
//
// DatasourceID 为空表示「当前业务库」（会话未选外部数据源）；SchemaName 存实际库名，
// 与 DatasourceID 一并消歧（同一租户下不同库的同名表互不覆盖）。
type ColumnProfile struct {
	TenantID     int64  `json:"tenant_id"`
	DatasourceID string `json:"datasource_id,omitempty"` // 空=当前业务库
	SchemaName   string `json:"schema_name"`             // 物理库名
	TableName    string `json:"table_name"`
	ColumnName   string `json:"column_name"`

	// RowCount 画像时的表总行数；NullCount 该列空值行数；NullRate 为二者派生比率（0~1），
	// 冗余存储以便读回直接展示，无需二次计算。
	RowCount  int64   `json:"row_count"`
	NullCount int64   `json:"null_count"`
	NullRate  float64 `json:"null_rate"`

	// Distinct 该列不同取值个数（基数）。高基数≈标识列/自由文本，低基数≈枚举/分类。
	Distinct int64 `json:"distinct"`

	// TopValues 仅对低基数（枚举型）列填充：按频次降序的实际取值分布。
	// 高基数或连续型列留空——枚举清单对它们无意义且体积大。
	TopValues []ValueFrequency `json:"top_values,omitempty"`

	// ProfiledAt 本次画像的完成时刻，用于时效判定（过期即重算）。
	ProfiledAt time.Time `json:"profiled_at"`
}

// Store 是列画像的持久化端口（写通目标）。实现落在 repository/mysql，
// 由组合根注入 agent 工具层；缺失（nil）时 get_schema 不带事实、零回归。
type Store interface {
	// Upsert 以物理坐标为幂等键覆盖式写入一批列画像（同一列重复画像即更新快照）。
	Upsert(ctx context.Context, profiles []*ColumnProfile) error
	// ListByTable 读回某表全部列的画像（缺失返回空切片，非错误）。
	ListByTable(ctx context.Context, tenantID int64, datasourceID, schema, table string) ([]*ColumnProfile, error)
}
