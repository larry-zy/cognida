// Package mysql 指标语义层仓储实现。
package mysql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"cognida/internal/model/semantic"
)

var _ semantic.Repository = (*SemanticRepository)(nil)

// SemanticRepository 指标语义模型仓储实现。
type SemanticRepository struct {
	db *gorm.DB
}

// NewSemanticRepository 创建语义模型仓储。
func NewSemanticRepository(db *gorm.DB) *SemanticRepository {
	return &SemanticRepository{db: db}
}

// GetActiveModel 按租户 + 逻辑名加载「生效」语义模型的完整 bundle。
func (r *SemanticRepository) GetActiveModel(ctx context.Context, tenantID int64, name string) (*semantic.ModelBundle, error) {
	var head SemanticModelModel
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND name = ? AND status = ? AND deleted_at IS NULL",
			tenantID, name, string(semantic.ModelStatusActive)).
		First(&head).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, semantic.ErrModelNotFound
		}
		return nil, fmt.Errorf("%w: %v", semantic.ErrRepository, err)
	}
	return r.loadBundle(ctx, &head)
}

// ListActiveModels 列出租户下全部生效语义模型（仅模型头）。
func (r *SemanticRepository) ListActiveModels(ctx context.Context, tenantID int64) ([]*semantic.SemanticModel, error) {
	var heads []*SemanticModelModel
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND status = ? AND deleted_at IS NULL",
			tenantID, string(semantic.ModelStatusActive)).
		Order("name ASC").
		Find(&heads).Error
	if err != nil {
		return nil, fmt.Errorf("%w: %v", semantic.ErrRepository, err)
	}
	out := make([]*semantic.SemanticModel, len(heads))
	for i, h := range heads {
		out[i] = h.ToDomain()
	}
	return out, nil
}

// GetModelBundle 按模型 ID 加载完整 bundle。
func (r *SemanticRepository) GetModelBundle(ctx context.Context, modelID string) (*semantic.ModelBundle, error) {
	var head SemanticModelModel
	err := r.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", modelID).
		First(&head).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, semantic.ErrModelNotFound
		}
		return nil, fmt.Errorf("%w: %v", semantic.ErrRepository, err)
	}
	return r.loadBundle(ctx, &head)
}

// loadBundle 载入一个模型头的全部构件，组装为 ModelBundle。
func (r *SemanticRepository) loadBundle(ctx context.Context, head *SemanticModelModel) (*semantic.ModelBundle, error) {
	db := r.db.WithContext(ctx)

	var lts []*SemanticLogicalTableModel
	if err := db.Where("model_id = ?", head.ID).Order("name ASC").Find(&lts).Error; err != nil {
		return nil, fmt.Errorf("%w: load logical tables: %v", semantic.ErrRepository, err)
	}
	var dims []*SemanticDimensionModel
	if err := db.Where("model_id = ?", head.ID).Order("name ASC").Find(&dims).Error; err != nil {
		return nil, fmt.Errorf("%w: load dimensions: %v", semantic.ErrRepository, err)
	}
	var meas []*SemanticMeasureModel
	if err := db.Where("model_id = ?", head.ID).Order("name ASC").Find(&meas).Error; err != nil {
		return nil, fmt.Errorf("%w: load measures: %v", semantic.ErrRepository, err)
	}
	var mets []*SemanticMetricModel
	if err := db.Where("model_id = ?", head.ID).Order("name ASC").Find(&mets).Error; err != nil {
		return nil, fmt.Errorf("%w: load metrics: %v", semantic.ErrRepository, err)
	}
	var rels []*SemanticRelationModel
	if err := db.Where("model_id = ?", head.ID).Find(&rels).Error; err != nil {
		return nil, fmt.Errorf("%w: load relations: %v", semantic.ErrRepository, err)
	}

	bundle := &semantic.ModelBundle{
		Model:         head.ToDomain(),
		LogicalTables: make([]*semantic.LogicalTable, len(lts)),
		Dimensions:    make([]*semantic.Dimension, len(dims)),
		Measures:      make([]*semantic.Measure, len(meas)),
		Metrics:       make([]*semantic.Metric, len(mets)),
		Relations:     make([]*semantic.Relation, len(rels)),
	}
	for i, x := range lts {
		bundle.LogicalTables[i] = x.ToDomain()
	}
	for i, x := range dims {
		bundle.Dimensions[i] = x.ToDomain()
	}
	for i, x := range meas {
		bundle.Measures[i] = x.ToDomain()
	}
	for i, x := range mets {
		bundle.Metrics[i] = x.ToDomain()
	}
	for i, x := range rels {
		bundle.Relations[i] = x.ToDomain()
	}
	return bundle, nil
}

// UpsertBundle 全量写入/更新一个语义模型及其构件（事务内幂等：先删旧构件再插新）。
func (r *SemanticRepository) UpsertBundle(ctx context.Context, bundle *semantic.ModelBundle) error {
	if bundle == nil || bundle.Model == nil || bundle.Model.ID == "" {
		return fmt.Errorf("%w: bundle 或 model.id 为空", semantic.ErrRepository)
	}
	now := time.Now()
	modelID := bundle.Model.ID

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 模型头：Save 即 upsert（主键存在则更新）。
		head := FromDomainSemanticModel(bundle.Model)
		if head.CreatedAt.IsZero() {
			head.CreatedAt = now
		}
		head.UpdatedAt = now
		if head.Version <= 0 {
			head.Version = 1
		}
		if head.Status == "" {
			head.Status = string(semantic.ModelStatusDraft)
		}
		if err := tx.Save(head).Error; err != nil {
			return fmt.Errorf("%w: save model: %v", semantic.ErrRepository, err)
		}

		// 构件：先按 model_id 清空再全量插入，保证幂等且无残留。
		wipes := []interface{}{
			&SemanticLogicalTableModel{}, &SemanticDimensionModel{},
			&SemanticMeasureModel{}, &SemanticMetricModel{}, &SemanticRelationModel{},
		}
		for _, m := range wipes {
			if err := tx.Where("model_id = ?", modelID).Delete(m).Error; err != nil {
				return fmt.Errorf("%w: wipe components: %v", semantic.ErrRepository, err)
			}
		}

		for _, e := range bundle.LogicalTables {
			m := FromDomainSemanticLogicalTable(e)
			stampTimes(&m.CreatedAt, &m.UpdatedAt, now)
			if err := tx.Create(m).Error; err != nil {
				return fmt.Errorf("%w: create logical table: %v", semantic.ErrRepository, err)
			}
		}
		for _, e := range bundle.Dimensions {
			m := FromDomainSemanticDimension(e)
			stampTimes(&m.CreatedAt, &m.UpdatedAt, now)
			if err := tx.Create(m).Error; err != nil {
				return fmt.Errorf("%w: create dimension: %v", semantic.ErrRepository, err)
			}
		}
		for _, e := range bundle.Measures {
			m := FromDomainSemanticMeasure(e)
			stampTimes(&m.CreatedAt, &m.UpdatedAt, now)
			if err := tx.Create(m).Error; err != nil {
				return fmt.Errorf("%w: create measure: %v", semantic.ErrRepository, err)
			}
		}
		for _, e := range bundle.Metrics {
			m := FromDomainSemanticMetric(e)
			stampTimes(&m.CreatedAt, &m.UpdatedAt, now)
			if err := tx.Create(m).Error; err != nil {
				return fmt.Errorf("%w: create metric: %v", semantic.ErrRepository, err)
			}
		}
		for _, e := range bundle.Relations {
			m := FromDomainSemanticRelation(e)
			stampTimes(&m.CreatedAt, &m.UpdatedAt, now)
			if err := tx.Create(m).Error; err != nil {
				return fmt.Errorf("%w: create relation: %v", semantic.ErrRepository, err)
			}
		}
		return nil
	})
}

// BumpVersion 递增模型版本（语义变更后调用，驱动 Verified Query 缓存失效）。
func (r *SemanticRepository) BumpVersion(ctx context.Context, modelID string) (int, error) {
	var newVersion int
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 行级锁读取，避免并发 Bump 丢失递增（两事务同读 v3 各写 v4）。
		var head SemanticModelModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND deleted_at IS NULL", modelID).
			First(&head).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return semantic.ErrModelNotFound
			}
			return fmt.Errorf("%w: %v", semantic.ErrRepository, err)
		}
		newVersion = head.Version + 1
		res := tx.Model(&SemanticModelModel{}).
			Where("id = ?", modelID).
			Updates(map[string]interface{}{
				"version":    newVersion,
				"updated_at": time.Now(),
			})
		if res.Error != nil {
			return fmt.Errorf("%w: %v", semantic.ErrRepository, res.Error)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return newVersion, nil
}

// stampTimes 为构件补齐时间戳（创建时间缺省则用 now，更新时间恒为 now）。
func stampTimes(createdAt, updatedAt *time.Time, now time.Time) {
	if createdAt.IsZero() {
		*createdAt = now
	}
	*updatedAt = now
}
