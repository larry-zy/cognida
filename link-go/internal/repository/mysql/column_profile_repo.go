package mysql

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"link/internal/model/dataprofile"
)

// columnProfileRepository 列画像仓储。
type columnProfileRepository struct {
	db *gorm.DB
}

// NewColumnProfileRepository 创建列画像仓储（Store 端口的 MySQL 实现）。
func NewColumnProfileRepository(db *gorm.DB) dataprofile.Store {
	return &columnProfileRepository{db: db}
}

// upsertColumns 覆盖式 upsert 时按物理坐标冲突要更新的字段（统计值 + 画像时刻）。
var upsertColumns = clause.OnConflict{
	Columns: []clause.Column{
		{Name: "tenant_id"}, {Name: "datasource_id"},
		{Name: "schema_name"}, {Name: "table_name"}, {Name: "column_name"},
	},
	DoUpdates: clause.AssignmentColumns([]string{
		"row_count", "null_count", "null_rate", "distinct_count", "top_values", "profiled_at",
	}),
}

// Upsert 以物理坐标为幂等键覆盖式写入一批列画像（重复画像即更新快照，不累积）。
func (r *columnProfileRepository) Upsert(ctx context.Context, profiles []*dataprofile.ColumnProfile) error {
	if len(profiles) == 0 {
		return nil
	}
	models := make([]*ColumnProfileModel, 0, len(profiles))
	for _, p := range profiles {
		models = append(models, columnProfileModelFromDomain(p))
	}
	if err := r.db.WithContext(ctx).
		Clauses(upsertColumns).
		CreateInBatches(models, 100).Error; err != nil {
		return fmt.Errorf("写入列画像失败: %w", err)
	}
	return nil
}

// ListByTable 读回某表全部列的画像（缺失返回空切片，非错误）。
func (r *columnProfileRepository) ListByTable(ctx context.Context, tenantID int64, datasourceID, schema, table string) ([]*dataprofile.ColumnProfile, error) {
	var models []*ColumnProfileModel
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND datasource_id = ? AND schema_name = ? AND table_name = ?",
			tenantID, datasourceID, schema, table).
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("查询列画像失败: %w", err)
	}
	out := make([]*dataprofile.ColumnProfile, 0, len(models))
	for _, m := range models {
		out = append(out, m.ToDomain())
	}
	return out, nil
}
