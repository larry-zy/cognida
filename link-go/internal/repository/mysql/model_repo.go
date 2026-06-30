// Package mysql 提供 MySQL 持久化层的 LLM 模型仓储实现
package mysql

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
	"link/internal/model/llm"
)

// modelRepo LLM 模型配置仓储实现
type modelRepo struct {
	db *gorm.DB
}

// NewModelRepository 创建模型配置仓储
func NewModelRepository(db *gorm.DB) llm.ModelRepository {
	return &modelRepo{db: db}
}

// Create 创建模型配置
func (r *modelRepo) Create(ctx context.Context, config *llm.ModelConfig) error {
	config.CreatedAt = 0 // 由数据库设置
	config.UpdatedAt = 0

	entity := &Model{
		TenantID:    config.TenantID,
		Name:        config.Name,
		DisplayName: config.DisplayName,
		Provider:    string(config.Provider),
		ModelType:   string(config.ModelType),
		APIKey:      config.APIKey,
		BaseURL:     config.BaseURL,
		ModelName:   config.ModelName,
		Enabled:     config.Enabled,
		IsDefault:   config.IsDefault,
	}

	// 处理额外配置
	if config.Config != "" {
		entity.Config = config.Config
	}

	// 如果是默认模型，取消其他默认模型
	if config.IsDefault {
		r.db.WithContext(ctx).Model(&Model{}).
			Where("tenant_id = ? AND model_type = ? AND is_default = ?",
				config.TenantID, config.ModelType, true).
			Update("is_default", false)
	}

	if err := r.db.WithContext(ctx).Create(entity).Error; err != nil {
		return fmt.Errorf("create model config failed: %w", err)
	}

	config.ID = entity.ID
	config.CreatedAt = entity.CreatedAt
	config.UpdatedAt = entity.UpdatedAt
	return nil
}

// Update 更新模型配置
func (r *modelRepo) Update(ctx context.Context, config *llm.ModelConfig) error {
	entity := &Model{
		ID:          config.ID,
		TenantID:    config.TenantID,
		Name:        config.Name,
		DisplayName: config.DisplayName,
		Provider:    string(config.Provider),
		ModelType:   string(config.ModelType),
		APIKey:      config.APIKey,
		BaseURL:     config.BaseURL,
		ModelName:   config.ModelName,
		Enabled:     config.Enabled,
		IsDefault:   config.IsDefault,
		Config:      config.Config,
	}

	// 如果是默认模型，取消其他默认模型
	if config.IsDefault {
		r.db.WithContext(ctx).Model(&Model{}).
			Where("tenant_id = ? AND model_type = ? AND is_default = ? AND id != ?",
				config.TenantID, config.ModelType, true, config.ID).
			Update("is_default", false)
	}

	if err := r.db.WithContext(ctx).Save(entity).Error; err != nil {
		return fmt.Errorf("update model config failed: %w", err)
	}

	config.UpdatedAt = entity.UpdatedAt
	return nil
}

// Delete 删除模型配置
func (r *modelRepo) Delete(ctx context.Context, id int64) error {
	if err := r.db.WithContext(ctx).Delete(&Model{}, id).Error; err != nil {
		return fmt.Errorf("delete model config failed: %w", err)
	}
	return nil
}

// GetByID 根据 ID 获取模型配置
func (r *modelRepo) GetByID(ctx context.Context, id int64) (*llm.ModelConfig, error) {
	var entity Model
	if err := r.db.WithContext(ctx).First(&entity, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("model config not found")
		}
		return nil, fmt.Errorf("get model config failed: %w", err)
	}

	return r.toDomainModel(&entity), nil
}

// GetByTenant 获取租户的所有模型配置
func (r *modelRepo) GetByTenant(ctx context.Context, tenantID int64) ([]*llm.ModelConfig, error) {
	var entities []Model
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Find(&entities).Error; err != nil {
		return nil, fmt.Errorf("get model configs by tenant failed: %w", err)
	}

	configs := make([]*llm.ModelConfig, len(entities))
	for i, e := range entities {
		configs[i] = r.toDomainModel(&e)
	}
	return configs, nil
}

// GetByType 根据类型获取模型配置
func (r *modelRepo) GetByType(ctx context.Context, tenantID int64, modelType llm.ModelType) ([]*llm.ModelConfig, error) {
	var entities []Model
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND model_type = ?", tenantID, modelType).
		Order("is_default DESC, created_at DESC").
		Find(&entities).Error; err != nil {
		return nil, fmt.Errorf("get model configs by type failed: %w", err)
	}

	configs := make([]*llm.ModelConfig, len(entities))
	for i, e := range entities {
		configs[i] = r.toDomainModel(&e)
	}
	return configs, nil
}

// GetDefault 获取默认模型配置
func (r *modelRepo) GetDefault(ctx context.Context, tenantID int64, modelType llm.ModelType) (*llm.ModelConfig, error) {
	var entity Model
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND model_type = ? AND is_default = ?", tenantID, modelType, true).
		First(&entity).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// 如果没有默认模型，返回第一个启用的模型
			err = r.db.WithContext(ctx).
				Where("tenant_id = ? AND model_type = ? AND enabled = ?", tenantID, modelType, true).
				Order("created_at ASC").
				First(&entity).Error
			if err != nil {
				return nil, fmt.Errorf("no default model config found")
			}
		} else {
			return nil, fmt.Errorf("get default model config failed: %w", err)
		}
	}

	return r.toDomainModel(&entity), nil
}

// List 列出模型配置
func (r *modelRepo) List(ctx context.Context, tenantID int64, offset, limit int) ([]*llm.ModelConfig, int64, error) {
	var entities []Model
	var total int64

	// 获取总数
	if err := r.db.WithContext(ctx).
		Model(&Model{}).
		Where("tenant_id = ?", tenantID).
		Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count model configs failed: %w", err)
	}

	// 获取分页数据
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&entities).Error; err != nil {
		return nil, 0, fmt.Errorf("list model configs failed: %w", err)
	}

	configs := make([]*llm.ModelConfig, len(entities))
	for i, e := range entities {
		configs[i] = r.toDomainModel(&e)
	}
	return configs, total, nil
}

// toDomainModel 转换为领域模型
func (r *modelRepo) toDomainModel(entity *Model) *llm.ModelConfig {
	return &llm.ModelConfig{
		ID:          entity.ID,
		TenantID:    entity.TenantID,
		Name:        entity.Name,
		DisplayName: entity.DisplayName,
		Provider:    llm.Provider(entity.Provider),
		ModelType:   llm.ModelType(entity.ModelType),
		APIKey:      entity.APIKey,
		BaseURL:     entity.BaseURL,
		ModelName:   entity.ModelName,
		Enabled:     entity.Enabled,
		IsDefault:   entity.IsDefault,
		Config:      entity.Config,
		CreatedAt:   entity.CreatedAt,
		UpdatedAt:   entity.UpdatedAt,
	}
}

// ========================================
// Model Entity (数据库表)
// ========================================

// Model LLM 模型配置表
type Model struct {
	ID          int64  `gorm:"column:id;primaryKey;autoIncrement"`
	TenantID    int64  `gorm:"column:tenant_id;not null;index:idx_tenant_model"`
	Name        string `gorm:"column:name;not null;size:64"`
	DisplayName string `gorm:"column:display_name;not null;size:128"`
	Provider    string `gorm:"column:provider;not null;size:32"`
	ModelType   string `gorm:"column:model_type;not null;size:32;index:idx_tenant_model"`
	APIKey      string `gorm:"column:api_key;size:256"`
	BaseURL     string `gorm:"column:base_url;size:512"`
	ModelName   string `gorm:"column:model_name;not null;size:128"`
	Enabled     bool   `gorm:"column:enabled;default:true"`
	IsDefault   bool   `gorm:"column:is_default;default:false"`
	Config      string `gorm:"column:config;type:text"`
	CreatedAt   int64  `gorm:"column:created_at;autoCreateTime:milli"`
	UpdatedAt   int64  `gorm:"column:updated_at;autoUpdateTime:milli"`
}

// TableName 指定表名
func (Model) TableName() string {
	return "llm_models"
}

// ========================================
// 辅助函数
// ========================================

// GetModelConfig 获取模型配置（带额外配置解析）
func (r *modelRepo) GetModelConfig(ctx context.Context, tenantID int64, name string) (*llm.ModelConfig, error) {
	var entity Model
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND name = ?", tenantID, name).
		First(&entity).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("model config not found: %s", name)
		}
		return nil, fmt.Errorf("get model config failed: %w", err)
	}

	return r.toDomainModel(&entity), nil
}

// ParseExtraConfig 解析额外配置
func ParseExtraConfig(configStr string) (map[string]interface{}, error) {
	if configStr == "" {
		return make(map[string]interface{}), nil
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(configStr), &config); err != nil {
		return nil, fmt.Errorf("parse extra config failed: %w", err)
	}

	return config, nil
}

// MarshalExtraConfig 序列化额外配置
func MarshalExtraConfig(config map[string]interface{}) (string, error) {
	if len(config) == 0 {
		return "", nil
	}

	data, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("marshal extra config failed: %w", err)
	}

	return string(data), nil
}
