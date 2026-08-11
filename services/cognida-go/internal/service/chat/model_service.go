// Package llm provides LLM service layer implementations
package chat

import (
	"context"
	"fmt"
	"sort"

	"cognida/internal/model/llm"
)

// chatChainFactory 是模型工厂的可选能力：按有序配置装配一条 chat 弹性降级链。
// 工厂未实现时退化为单目标创建。
type chatChainFactory interface {
	CreateChatModelChain(configs []*llm.ModelConfig) (llm.LLMClient, error)
}

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

	// 类型/启用状态过滤与分页、总数统计全部下推仓储（同一 SQL 过滤口径），
	// 避免旧实现「类型分支不分页」「Enabled 内存过滤致 total 失真」两处缺陷（〔R2-2〕）。
	configs, total, err := s.modelRepo.List(ctx, tenantID, req.ModelType, req.Enabled, offset, pageSize)
	if err != nil {
		return nil, fmt.Errorf("列出模型配置失败: %w", err)
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

// ListModelsByCursor 游标（keyset）分页列出模型配置〔M5〕。
// 与 ListModels 的偏移路径共享排序（created_at DESC），仅在请求携带 cursor 时启用。
func (s *ModelService) ListModelsByCursor(ctx context.Context, tenantID int64, req *ListModelsRequestDTO, cursor string) (*ListModelsCursorResponseDTO, error) {
	pageSize := req.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	configs, nextCursor, err := s.modelRepo.ListByCursor(ctx, tenantID, req.ModelType, req.Enabled, cursor, pageSize)
	if err != nil {
		return nil, fmt.Errorf("列出模型配置失败: %w", err)
	}

	models := make([]*ModelResponseDTO, len(configs))
	for i, c := range configs {
		models[i] = FromDomainModelConfig(c)
	}

	return &ListModelsCursorResponseDTO{
		Models:     models,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
		PageSize:   pageSize,
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

	// 工厂支持链式时，装配「主模型 + 同租户其它可用 chat 模型」降级链。
	if chainFactory, ok := s.modelFactory.(chatChainFactory); ok {
		model, err := chainFactory.CreateChatModelChain(s.buildChatChain(ctx, tenantID, config))
		if err != nil {
			return nil, fmt.Errorf("创建聊天模型失败: %w", err)
		}
		return model, nil
	}

	model, err := s.modelFactory.CreateChatModel(config)
	if err != nil {
		return nil, fmt.Errorf("创建聊天模型失败: %w", err)
	}

	return model, nil
}

// buildChatChain 组装 chat 降级链：主模型置首，其余同租户可用 chat 模型按
// 「默认优先、再按 ID 升序」排在后备位。查询失败时退化为仅主模型。
func (s *ModelService) buildChatChain(ctx context.Context, tenantID int64, primary *llm.ModelConfig) []*llm.ModelConfig {
	chain := []*llm.ModelConfig{primary}

	others, err := s.modelRepo.GetByType(ctx, tenantID, llm.ModelTypeChat)
	if err != nil {
		return chain
	}

	candidates := make([]*llm.ModelConfig, 0, len(others))
	for _, c := range others {
		if c == nil || c.ID == primary.ID || !c.Enabled {
			continue
		}
		candidates = append(candidates, c)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].IsDefault != candidates[j].IsDefault {
			return candidates[i].IsDefault
		}
		return candidates[i].ID < candidates[j].ID
	})

	return append(chain, candidates...)
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
