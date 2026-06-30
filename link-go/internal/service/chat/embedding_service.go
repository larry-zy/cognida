// Package llm provides LLM service layer implementations
package chat

import (
	"context"
	"fmt"

	"link/internal/model/llm"
)

// ========================================
// Embedding Service
// ========================================

// EmbeddingService 嵌入向量服务
type EmbeddingService struct {
	embeddingRepo llm.EmbeddingRepository
	modelRepo     llm.ModelRepository
	modelFactory  llm.ModelFactory
}

// NewEmbeddingService 创建嵌入向量服务
func NewEmbeddingService(
	embeddingRepo llm.EmbeddingRepository,
	modelRepo llm.ModelRepository,
	modelFactory llm.ModelFactory,
) *EmbeddingService {
	return &EmbeddingService{
		embeddingRepo: embeddingRepo,
		modelRepo:     modelRepo,
		modelFactory:  modelFactory,
	}
}

// SetEmbeddingModel 设置嵌入向量模型
func (s *EmbeddingService) SetEmbeddingModel(repo llm.EmbeddingRepository) {
	s.embeddingRepo = repo
}

// Embed 生成文本嵌入向量
func (s *EmbeddingService) Embed(ctx context.Context, req *EmbeddingRequestDTO) (*EmbeddingResponseDTO, error) {
	if s.embeddingRepo == nil {
		return nil, fmt.Errorf("嵌入向量模型未初始化")
	}

	domainReq := ToDomainEmbeddingRequest(req)
	resp, err := s.embeddingRepo.Embed(ctx, domainReq)
	if err != nil {
		return nil, fmt.Errorf("生成嵌入向量失败: %w", err)
	}

	return FromDomainEmbeddingResponse(resp), nil
}

// EmbedBatch 批量生成嵌入向量
func (s *EmbeddingService) EmbedBatch(ctx context.Context, req *EmbeddingRequestDTO) (*EmbeddingResponseDTO, error) {
	if s.embeddingRepo == nil {
		return nil, fmt.Errorf("嵌入向量模型未初始化")
	}

	domainReq := ToDomainEmbeddingRequest(req)
	resp, err := s.embeddingRepo.EmbedBatch(ctx, domainReq)
	if err != nil {
		return nil, fmt.Errorf("批量生成嵌入向量失败: %w", err)
	}

	return FromDomainEmbeddingResponse(resp), nil
}

// GetDimension 获取向量维度
func (s *EmbeddingService) GetDimension(ctx context.Context) (int, error) {
	if s.embeddingRepo == nil {
		return 0, fmt.Errorf("嵌入向量模型未初始化")
	}

	return s.embeddingRepo.GetDimension(ctx)
}
