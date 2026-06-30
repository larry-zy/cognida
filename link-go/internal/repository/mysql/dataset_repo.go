// Package persistence MySQL 数据集仓储实现
package mysql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"link/internal/model/evaluation"
)

var (
	_ evaluation.DatasetRepository = (*DatasetRepository)(nil)
)

// DatasetRepository 数据集仓储实现
type DatasetRepository struct {
	db *gorm.DB
}

// NewDatasetRepository 创建数据集仓储
func NewDatasetRepository(db *gorm.DB) *DatasetRepository {
	return &DatasetRepository{db: db}
}

// Create 创建数据集
func (r *DatasetRepository) Create(ctx context.Context, dataset *evaluation.Dataset) error {
	model := FromDomainDataset(dataset)
	if err := r.getDB(ctx).Create(model).Error; err != nil {
		// 检查是否是唯一键冲突
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return evaluation.ErrDatasetAlreadyExists
		}
		return fmt.Errorf("%w: %v", evaluation.ErrRepository, err)
	}
	// 更新 ID
	dataset.ID = model.ID
	return nil
}

// FindByID 根据 dataset_id 查找数据集
func (r *DatasetRepository) FindByID(ctx context.Context, datasetID string) (*evaluation.Dataset, error) {
	var model DatasetModel
	err := r.getDB(ctx).Where("dataset_id = ? AND deleted_at IS NULL", datasetID).First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, evaluation.ErrDatasetNotFound
		}
		return nil, fmt.Errorf("%w: %v", evaluation.ErrRepository, err)
	}
	return model.ToDomain(), nil
}

// FindByIDWithTenant 根据 dataset_id 和 tenant_id 查找数据集
func (r *DatasetRepository) FindByIDWithTenant(ctx context.Context, datasetID string, tenantID int64) (*evaluation.Dataset, error) {
	var model DatasetModel
	err := r.getDB(ctx).Where("dataset_id = ? AND tenant_id = ? AND deleted_at IS NULL", datasetID, tenantID).First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, evaluation.ErrDatasetNotFound
		}
		return nil, fmt.Errorf("%w: %v", evaluation.ErrRepository, err)
	}
	return model.ToDomain(), nil
}

// List 根据过滤条件列出数据集
func (r *DatasetRepository) List(ctx context.Context, filter *evaluation.DatasetFilter) ([]*evaluation.Dataset, int64, error) {
	query := r.getDB(ctx).Model(&DatasetModel{}).Where("deleted_at IS NULL")

	// 应用过滤条件
	if filter.TenantID != nil {
		query = query.Where("tenant_id = ?", *filter.TenantID)
	}
	if filter.UserID != nil {
		query = query.Where("user_id = ?", *filter.UserID)
	}
	if filter.Type != nil {
		query = query.Where("type = ?", string(*filter.Type))
	}
	if filter.EvaluationType != nil {
		query = query.Where("evaluation_type = ?", string(*filter.EvaluationType))
	}
	if filter.Search != "" {
		query = query.Where("name LIKE ?", "%"+filter.Search+"%")
	}

	// 获取总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("%w: %v", evaluation.ErrRepository, err)
	}

	// 分页和排序
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}

	offset := (filter.Page - 1) * filter.PageSize

	var models []*DatasetModel
	err := query.Order("created_at DESC").
		Limit(filter.PageSize).
		Offset(offset).
		Find(&models).Error

	if err != nil {
		return nil, 0, fmt.Errorf("%w: %v", evaluation.ErrRepository, err)
	}

	return ToDomainDatasetList(models), total, nil
}

// Update 更新数据集元数据
func (r *DatasetRepository) Update(ctx context.Context, dataset *evaluation.Dataset) error {
	model := FromDomainDataset(dataset)
	result := r.getDB(ctx).
		Model(&DatasetModel{}).
		Where("dataset_id = ? AND deleted_at IS NULL", dataset.DatasetID).
		Updates(map[string]interface{}{
			"name":            model.Name,
			"description":     model.Description,
			"evaluation_type": model.EvaluationType,
			"qa_count":        model.QACount,
			"updated_at":      time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("%w: %v", evaluation.ErrRepository, result.Error)
	}
	if result.RowsAffected == 0 {
		return evaluation.ErrDatasetNotFound
	}
	return nil
}

// SoftDelete 软删除数据集
func (r *DatasetRepository) SoftDelete(ctx context.Context, datasetID string, tenantID int64) error {
	result := r.getDB(ctx).
		Model(&DatasetModel{}).
		Where("dataset_id = ? AND tenant_id = ?", datasetID, tenantID).
		Update("deleted_at", time.Now())

	if result.Error != nil {
		return fmt.Errorf("%w: %v", evaluation.ErrRepository, result.Error)
	}
	if result.RowsAffected == 0 {
		return evaluation.ErrDatasetNotFound
	}
	return nil
}

// AddSamples 批量添加样本
func (r *DatasetRepository) AddSamples(ctx context.Context, datasetID string, samples []*evaluation.DatasetRecord) error {
	if len(samples) == 0 {
		return nil
	}

	// 转换为模型
	models := make([]*DatasetRecordModel, len(samples))
	for i, s := range samples {
		models[i] = FromDomainDatasetRecord(s)
	}

	// 批量插入（每批 100 条）
	batchSize := 100
	if err := r.getDB(ctx).CreateInBatches(models, batchSize).Error; err != nil {
		return fmt.Errorf("%w: %v", evaluation.ErrRepository, err)
	}

	// 更新数据集的 qa_count
	count, err := r.CountSamples(ctx, datasetID)
	if err != nil {
		return err
	}

	return r.getDB(ctx).
		Model(&DatasetModel{}).
		Where("dataset_id = ?", datasetID).
		Update("qa_count", count).Error
}

// ListSamples 查询样本列表（分页）
func (r *DatasetRepository) ListSamples(ctx context.Context, datasetID string, tenantID int64, page, pageSize int) ([]*evaluation.DatasetRecord, int64, error) {
	query := r.getDB(ctx).Model(&DatasetRecordModel{}).Where("dataset_id = ?", datasetID)

	// 租户隔离
	if tenantID > 0 {
		query = query.Where("tenant_id = ?", tenantID)
	}

	// 获取总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("%w: %v", evaluation.ErrRepository, err)
	}

	// 分页
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 1000 {
		pageSize = 1000
	}

	offset := (page - 1) * pageSize

	var models []*DatasetRecordModel
	err := query.Order("id ASC").
		Limit(pageSize).
		Offset(offset).
		Find(&models).Error

	if err != nil {
		return nil, 0, fmt.Errorf("%w: %v", evaluation.ErrRepository, err)
	}

	return ToDomainDatasetRecordList(models), total, nil
}

// DeleteSample 删除单个样本
func (r *DatasetRepository) DeleteSample(ctx context.Context, datasetID string, tenantID int64, sampleID int64) error {
	query := r.getDB(ctx).Where("id = ? AND dataset_id = ?", sampleID, datasetID)

	// 租户隔离
	if tenantID > 0 {
		query = query.Where("tenant_id = ?", tenantID)
	}

	result := query.Delete(&DatasetRecordModel{})
	if result.Error != nil {
		return fmt.Errorf("%w: %v", evaluation.ErrRepository, result.Error)
	}
	if result.RowsAffected == 0 {
		return evaluation.ErrDatasetSampleNotFound
	}

	// 更新数据集的 qa_count
	count, err := r.CountSamples(ctx, datasetID)
	if err != nil {
		return err
	}

	return r.getDB(ctx).
		Model(&DatasetModel{}).
		Where("dataset_id = ?", datasetID).
		Update("qa_count", count).Error
}

// CountSamples 统计样本数量
func (r *DatasetRepository) CountSamples(ctx context.Context, datasetID string) (int, error) {
	var count int64
	err := r.getDB(ctx).
		Model(&DatasetRecordModel{}).
		Where("dataset_id = ?", datasetID).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("%w: %v", evaluation.ErrRepository, err)
	}
	return int(count), nil
}

// getDB 获取数据库连接（支持事务）
func (r *DatasetRepository) getDB(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value("tx").(*gorm.DB); ok {
		return tx
	}
	return r.db.WithContext(ctx)
}
