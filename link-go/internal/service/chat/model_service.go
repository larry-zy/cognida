// Package llm provides LLM service layer implementations
package chat

import (
	"context"
	"fmt"

	"link/internal/model/llm"
)

// ========================================
// Model 管理 Service
// ========================================

// ModelService 模型管理服务
type ModelService struct {
	modelRepo    llm.ModelRepository
	modelFactory llm.ModelFactory
}

// NewModelService 创建模型管理服务
func NewModelService(
	modelRepo llm.ModelRepository,
	modelFactory llm.ModelFactory,
) *ModelService {
	return &ModelService{
		modelRepo:    modelRepo,
		modelFactory: modelFactory,
	}
}

// CreateModel 创建模型配置
func (s *ModelService) CreateModel(ctx context.Context, tenantID int64, req *CreateModelRequestDTO) (*ModelResponseDTO, error) {
	config := ToDomainModelConfig(tenantID, req)
	if err := s.modelRepo.Create(ctx, config); err != nil {
		return nil, fmt.Errorf("创建模型配置失败: %w", err)
	}

	return FromDomainModelConfig(config), nil
}

// UpdateModel 更新模型配置
func (s *ModelService) UpdateModel(ctx context.Context, id int64, req *UpdateModelRequestDTO) (*ModelResponseDTO, error) {
	config, err := s.modelRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("获取模型配置失败: %w", err)
	}

	// 更新字段
	if req.DisplayName != "" {
		config.DisplayName = req.DisplayName
	}
	if req.APIKey != "" {
		config.APIKey = req.APIKey
	}
	if req.BaseURL != "" {
		config.BaseURL = req.BaseURL
	}
	if req.ModelName != "" {
		config.ModelName = req.ModelName
	}
	if req.Enabled != nil {
		config.Enabled = *req.Enabled
	}
	if req.IsDefault != nil {
		config.IsDefault = *req.IsDefault
	}

	if err := s.modelRepo.Update(ctx, config); err != nil {
		return nil, fmt.Errorf("更新模型配置失败: %w", err)
	}

	return FromDomainModelConfig(config), nil
}

// DeleteModel 删除模型配置
func (s *ModelService) DeleteModel(ctx context.Context, id int64) error {
	return s.modelRepo.Delete(ctx, id)
}

// GetModel 获取模型配置
func (s *ModelService) GetModel(ctx context.Context, id int64) (*ModelResponseDTO, error) {
	config, err := s.modelRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("获取模型配置失败: %w", err)
	}

	return FromDomainModelConfig(config), nil
}

// ListModels 列出模型配置
func (s *ModelService) ListModels(ctx context.Context, tenantID int64, req *ListModelsRequestDTO) (*ListModelsResponseDTO, error) {
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize

	var configs []*llm.ModelConfig
	var total int64
	var err error

	if req.ModelType != "" {
		configs, err = s.modelRepo.GetByType(ctx, tenantID, llm.ModelType(req.ModelType))
		total = int64(len(configs))
	} else {
		configs, total, err = s.modelRepo.List(ctx, tenantID, offset, pageSize)
	}

	if err != nil {
		return nil, fmt.Errorf("列出模型配置失败: %w", err)
	}

	// 应用过滤
	if req.Enabled != nil {
		filtered := make([]*llm.ModelConfig, 0)
		for _, c := range configs {
			if c.Enabled == *req.Enabled {
				filtered = append(filtered, c)
			}
		}
		configs = filtered
	}

	models := make([]*ModelResponseDTO, len(configs))
	for i, c := range configs {
		models[i] = FromDomainModelConfig(c)
	}

	return &ListModelsResponseDTO{
		Models:   models,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// GetDefaultModel 获取默认模型
func (s *ModelService) GetDefaultModel(ctx context.Context, tenantID int64, modelType string) (*ModelResponseDTO, error) {
	config, err := s.modelRepo.GetDefault(ctx, tenantID, llm.ModelType(modelType))
	if err != nil {
		return nil, fmt.Errorf("获取默认模型失败: %w", err)
	}

	return FromDomainModelConfig(config), nil
}

// CreateChatModel 创建聊天模型实例
func (s *ModelService) CreateChatModel(ctx context.Context, tenantID, modelID int64) (llm.LLMClient, error) {
	config, err := s.modelRepo.GetByID(ctx, modelID)
	if err != nil {
		return nil, fmt.Errorf("获取模型配置失败: %w", err)
	}

	if config.TenantID != tenantID {
		return nil, fmt.Errorf("无权访问该模型")
	}

	if config.ModelType != llm.ModelTypeChat {
		return nil, fmt.Errorf("模型类型不匹配")
	}

	model, err := s.modelFactory.CreateChatModel(config)
	if err != nil {
		return nil, fmt.Errorf("创建聊天模型失败: %w", err)
	}

	return model, nil
}

// CreateEmbeddingModel 创建嵌入向量模型实例
func (s *ModelService) CreateEmbeddingModel(ctx context.Context, tenantID, modelID int64) (llm.EmbeddingRepository, error) {
	config, err := s.modelRepo.GetByID(ctx, modelID)
	if err != nil {
		return nil, fmt.Errorf("获取模型配置失败: %w", err)
	}

	if config.TenantID != tenantID {
		return nil, fmt.Errorf("无权访问该模型")
	}

	if config.ModelType != llm.ModelTypeEmbedding {
		return nil, fmt.Errorf("模型类型不匹配")
	}

	model, err := s.modelFactory.CreateEmbeddingModel(config)
	if err != nil {
		return nil, fmt.Errorf("创建嵌入向量模型失败: %w", err)
	}

	return model, nil
}

// CreateRerankModel 创建重排模型实例
func (s *ModelService) CreateRerankModel(ctx context.Context, tenantID, modelID int64) (llm.RerankRepository, error) {
	config, err := s.modelRepo.GetByID(ctx, modelID)
	if err != nil {
		return nil, fmt.Errorf("获取模型配置失败: %w", err)
	}

	if config.TenantID != tenantID {
		return nil, fmt.Errorf("无权访问该模型")
	}

	if config.ModelType != llm.ModelTypeRerank {
		return nil, fmt.Errorf("模型类型不匹配")
	}

	model, err := s.modelFactory.CreateRerankModel(config)
	if err != nil {
		return nil, fmt.Errorf("创建重排模型失败: %w", err)
	}

	return model, nil
}
