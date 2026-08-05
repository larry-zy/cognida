// Package mysql 提供列画像（column_profiles）的 MySQL 持久化实现。
package mysql

import (
	"encoding/json"
	"time"

	"link/internal/model/dataprofile"
)

// ColumnProfileModel 对应 column_profiles 表，承载单列数据事实的快照。
//
// 唯一键取物理坐标（tenant_id + datasource_id + schema_name + table_name + column_name），
// 写入以此为幂等键覆盖式 upsert——非累积，一列一行，重复画像只更新快照。datasource_id
// 空串表示当前业务库（会话未选外部数据源）。top_values 以 JSON 列留存枚举分布。
type ColumnProfileModel struct {
	ID           int64  `gorm:"primaryKey;autoIncrement"`
	TenantID     int64  `gorm:"not null;uniqueIndex:uk_col_profile,priority:1;index:idx_col_profile_table,priority:1"`
	DatasourceID string `gorm:"type:varchar(128);not null;default:'';uniqueIndex:uk_col_profile,priority:2;index:idx_col_profile_table,priority:2"`
	SchemaName   string `gorm:"type:varchar(128);not null;uniqueIndex:uk_col_profile,priority:3;index:idx_col_profile_table,priority:3"`
	// Table 存被画像表的物理表名（DB 列 table_name）。字段不叫 TableName 是为了
	// 避让下方 GORM Tabler 接口要求的 TableName() 方法——两者同名会编译冲突。
	Table string `gorm:"column:table_name;type:varchar(128);not null;uniqueIndex:uk_col_profile,priority:4;index:idx_col_profile_table,priority:4"`
	ColumnName   string `gorm:"type:varchar(128);not null;uniqueIndex:uk_col_profile,priority:5"`

	RowCount  int64   `gorm:"not null;default:0;comment:画像时表总行数"`
	NullCount int64   `gorm:"not null;default:0;comment:空值行数"`
	NullRate  float64 `gorm:"not null;default:0;comment:空值率0~1"`
	Distinct  int64   `gorm:"column:distinct_count;not null;default:0;comment:基数(不同取值个数)"`

	// TopValues 枚举列的取值-频次分布（JSON 数组），高基数/连续列为空。
	TopValues  string    `gorm:"type:json"`
	ProfiledAt time.Time `gorm:"not null;comment:本次画像完成时刻"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime"`
}

// TableName 指定表名。
func (ColumnProfileModel) TableName() string {
	return "column_profiles"
}

// ToDomain 转换为领域实体（top_values 反序列化失败时按空处理，不阻断读回）。
func (m *ColumnProfileModel) ToDomain() *dataprofile.ColumnProfile {
	var top []dataprofile.ValueFrequency
	if m.TopValues != "" {
		_ = json.Unmarshal([]byte(m.TopValues), &top)
	}
	return &dataprofile.ColumnProfile{
		TenantID:     m.TenantID,
		DatasourceID: m.DatasourceID,
		SchemaName:   m.SchemaName,
		TableName:    m.Table,
		ColumnName:   m.ColumnName,
		RowCount:     m.RowCount,
		NullCount:    m.NullCount,
		NullRate:     m.NullRate,
		Distinct:     m.Distinct,
		TopValues:    top,
		ProfiledAt:   m.ProfiledAt,
	}
}

// columnProfileModelFromDomain 从领域实体构造持久化模型。
// top_values 是 MySQL JSON 列，空数组必须写成 "[]" 而非空串——空串不是合法 JSON，
// 会被拒（Error 3140）；序列化失败同样回退到 "[]" 兜底。
func columnProfileModelFromDomain(p *dataprofile.ColumnProfile) *ColumnProfileModel {
	top := "[]"
	if len(p.TopValues) > 0 {
		if b, err := json.Marshal(p.TopValues); err == nil {
			top = string(b)
		}
	}
	return &ColumnProfileModel{
		TenantID:     p.TenantID,
		DatasourceID: p.DatasourceID,
		SchemaName:   p.SchemaName,
		Table:        p.TableName,
		ColumnName:   p.ColumnName,
		RowCount:     p.RowCount,
		NullCount:    p.NullCount,
		NullRate:     p.NullRate,
		Distinct:     p.Distinct,
		TopValues:    top,
		ProfiledAt:   p.ProfiledAt,
	}
}
