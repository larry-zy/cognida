// Package llm provides LLM service layer implementations
package chat

import (
	"context"
	"fmt"

	"link/internal/model/llm"
)

// ========================================
// Rerank Service
// ========================================

// RerankService 重排服务
type RerankService struct {
	rerankRepo    llm.RerankRepository
	modelRepo     llm.ModelRepository
	modelFactory  llm.ModelFactory
}

// NewRerankService 创建重排服务
func NewRerankService(
	rerankRepo llm.RerankRepository,
	modelRepo llm.ModelRepository,
	modelFactory llm.ModelFactory,
) *RerankService {
	return &RerankService{
		rerankRepo:   rerankRepo,
		modelRepo:    modelRepo,
		modelFactory: modelFactory,
	}
}

// SetRerankModel 设置重排模型
func (s *RerankService) SetRerankModel(repo llm.RerankRepository) {
	s.rerankRepo = repo
}

// Rerank 重排文档列表
func (s *RerankService) Rerank(ctx context.Context, req *RerankRequestDTO) (*RerankResponseDTO, error) {
	if s.rerankRepo == nil {
		return nil, fmt.Errorf("重排模型未初始化")
	}

	domainReq := ToDomainRerankRequest(req)
	resp, err := s.rerankRepo.Rerank(ctx, domainReq)
	if err != nil {
		return nil, fmt.Errorf("重排失败: %w", err)
	}

	return FromDomainRerankResponse(resp), nil
}

// GetMaxDocs 获取支持的最大文档数
func (s *RerankService) GetMaxDocs() int {
	if s.rerankRepo == nil {
		return 0
	}
	return s.rerankRepo.GetMaxDocs()
}
