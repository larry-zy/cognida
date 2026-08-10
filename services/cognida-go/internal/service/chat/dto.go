// Package chat provides chat application layer DTOs
package chat

import (
	"time"

	"cognida/internal/model/conversation"
	"cognida/internal/model/llm"
)

// ========================================
// Model 配置相关 DTO
// ========================================

// CreateModelRequestDTO 创建模型请求 DTO
type CreateModelRequestDTO struct {
	Name        string                 `json:"name" binding:"required"`
	DisplayName string                 `json:"display_name" binding:"required"`
	Provider    string                 `json:"provider" binding:"required"`
	ModelType   string                 `json:"model_type" binding:"required"`
	APIKey      string                 `json:"api_key,omitempty"`
	BaseURL     string                 `json:"base_url,omitempty"`
	ModelName   string                 `json:"model_name" binding:"required"`
	Enabled     bool                   `json:"enabled"`
	IsDefault   bool                   `json:"is_default"`
	Config      map[string]interface{} `json:"config,omitempty"`
}

// UpdateModelRequestDTO 更新模型请求 DTO
type UpdateModelRequestDTO struct {
	DisplayName string                 `json:"display_name,omitempty"`
	APIKey      string                 `json:"api_key,omitempty"`
	BaseURL     string                 `json:"base_url,omitempty"`
	ModelName   string                 `json:"model_name,omitempty"`
	Enabled     *bool                  `json:"enabled,omitempty"`
	IsDefault   *bool                  `json:"is_default,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`
}

// ModelResponseDTO 模型响应 DTO
type ModelResponseDTO struct {
	ID          int64                  `json:"id"`
	TenantID    int64                  `json:"tenant_id"`
	Name        string                 `json:"name"`
	DisplayName string                 `json:"display_name"`
	Provider    string                 `json:"provider"`
	ModelType   string                 `json:"model_type"`
	BaseURL     string                 `json:"base_url,omitempty"`
	ModelName   string                 `json:"model_name"`
	Enabled     bool                   `json:"enabled"`
	IsDefault   bool                   `json:"is_default"`
	CreatedAt   int64                  `json:"created_at"`
	UpdatedAt   int64                  `json:"updated_at"`
}

// ListModelsRequestDTO 列出模型请求 DTO
type ListModelsRequestDTO struct {
	ModelType string `json:"model_type,omitempty"`
	Enabled   *bool  `json:"enabled,omitempty"`
	Page      int    `json:"page,omitempty"`
	PageSize  int    `json:"page_size,omitempty"`
}

// ListModelsResponseDTO 列出模型响应 DTO
type ListModelsResponseDTO struct {
	Models    []*ModelResponseDTO `json:"models"`
	Total     int64               `json:"total"`
	Page      int                 `json:"page"`
	PageSize  int                 `json:"page_size"`
}

// ListModelsCursorResponseDTO 游标分页列出模型响应 DTO〔M5〕
type ListModelsCursorResponseDTO struct {
	Models     []*ModelResponseDTO `json:"models"`
	NextCursor string              `json:"next_cursor"`
	HasMore    bool                `json:"has_more"`
	PageSize   int                 `json:"page_size"`
}

// ========================================
// Embedding 相关 DTO
// ========================================

// EmbeddingRequestDTO 嵌入向量请求 DTO
type EmbeddingRequestDTO struct {
	Texts   []string              `json:"texts" binding:"required"`
	Model   string                `json:"model,omitempty"`
	Options *EmbeddingOptionsDTO  `json:"options,omitempty"`
}

// EmbeddingOptionsDTO 嵌入向量选项 DTO
type EmbeddingOptionsDTO struct {
	Dimensions     int                    `json:"dimensions,omitempty"`
	EncodingFormat string                 `json:"encoding_format,omitempty"`
	Extra          map[string]interface{} `json:"extra,omitempty"`
}

// UsageDTO 使用量统计 DTO
type UsageDTO struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// EmbeddingResponseDTO 嵌入向量响应 DTO
type EmbeddingResponseDTO struct {
	Embeddings [][]float32            `json:"embeddings"`
	Usage      *UsageDTO              `json:"usage,omitempty"`
	Model      string                 `json:"model,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// ========================================
// Rerank 相关 DTO
// ========================================

// RerankRequestDTO 重排请求 DTO
type RerankRequestDTO struct {
	Query   string             `json:"query" binding:"required"`
	Docs    []string           `json:"docs" binding:"required"`
	Model   string             `json:"model,omitempty"`
	Options *RerankOptionsDTO  `json:"options,omitempty"`
}

// RerankOptionsDTO 重排选项 DTO
type RerankOptionsDTO struct {
	TopK  int                    `json:"top_k,omitempty"`
	Extra map[string]interface{} `json:"extra,omitempty"`
}

// RerankResponseDTO 重排响应 DTO
type RerankResponseDTO struct {
	Results  []*RerankResultDTO    `json:"results"`
	Model    string                `json:"model,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// RerankResultDTO 重排结果 DTO
type RerankResultDTO struct {
	Index    int     `json:"index"`
	Score    float64 `json:"score"`
	Document string  `json:"document,omitempty"`
}

// ========================================
// 转换函数
// ========================================

// ToDomainEmbeddingRequest 转换为领域层嵌入请求
func ToDomainEmbeddingRequest(req *EmbeddingRequestDTO) *llm.EmbeddingRequest {
	if req == nil {
		return nil
	}

	var opts *llm.EmbeddingOptions
	if req.Options != nil {
		opts = &llm.EmbeddingOptions{
			Dimensions:      req.Options.Dimensions,
			EncodingFormat:  req.Options.EncodingFormat,
			Extra:           req.Options.Extra,
		}
	}

	return &llm.EmbeddingRequest{
		Texts:   req.Texts,
		Model:   req.Model,
		Options: opts,
	}
}

// FromDomainEmbeddingResponse 转换领域层嵌入响应
func FromDomainEmbeddingResponse(resp *llm.EmbeddingResponse) *EmbeddingResponseDTO {
	if resp == nil {
		return nil
	}

	var usage *UsageDTO
	if resp.Usage != nil {
		usage = &UsageDTO{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}

	return &EmbeddingResponseDTO{
		Embeddings: resp.Embeddings,
		Usage:      usage,
		Model:      resp.Model,
		Metadata:   resp.Metadata,
	}
}

// ToDomainRerankRequest 转换为领域层重排请求
func ToDomainRerankRequest(req *RerankRequestDTO) *llm.RerankRequest {
	if req == nil {
		return nil
	}

	var opts *llm.RerankOptions
	if req.Options != nil {
		opts = &llm.RerankOptions{
			TopK:  req.Options.TopK,
			Extra: req.Options.Extra,
		}
	}

	return &llm.RerankRequest{
		Query:   req.Query,
		Docs:    req.Docs,
		Model:   req.Model,
		Options: opts,
	}
}

// FromDomainRerankResponse 转换领域层重排响应
func FromDomainRerankResponse(resp *llm.RerankResponse) *RerankResponseDTO {
	if resp == nil {
		return nil
	}

	results := make([]*RerankResultDTO, len(resp.Results))
	for i, r := range resp.Results {
		results[i] = &RerankResultDTO{
			Index:    r.Index,
			Score:    r.Score,
			Document: r.Document,
		}
	}

	return &RerankResponseDTO{
		Results:  results,
		Model:    resp.Model,
		Metadata: resp.Metadata,
	}
}

// ToDomainModelConfig 转换为领域层模型配置
func ToDomainModelConfig(tenantID int64, req *CreateModelRequestDTO) *llm.ModelConfig {
	return &llm.ModelConfig{
		TenantID:    tenantID,
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Provider:    llm.Provider(req.Provider),
		ModelType:   llm.ModelType(req.ModelType),
		APIKey:      req.APIKey,
		BaseURL:     req.BaseURL,
		ModelName:   req.ModelName,
		Enabled:     req.Enabled,
		IsDefault:   req.IsDefault,
		// Config 需要序列化为 JSON
	}
}

// FromDomainModelConfig 转换领域层模型配置为 DTO
func FromDomainModelConfig(config *llm.ModelConfig) *ModelResponseDTO {
	if config == nil {
		return nil
	}

	return &ModelResponseDTO{
		ID:          config.ID,
		TenantID:    config.TenantID,
		Name:        config.Name,
		DisplayName: config.DisplayName,
		Provider:    string(config.Provider),
		ModelType:   string(config.ModelType),
		BaseURL:     config.BaseURL,
		ModelName:   config.ModelName,
		Enabled:     config.Enabled,
		IsDefault:   config.IsDefault,
		CreatedAt:   config.CreatedAt,
		UpdatedAt:   config.UpdatedAt,
	}
}

// ========================================
// Message/Session DTO
// ========================================

// CreateMessageRequest 创建消息请求
type CreateMessageRequest struct {
	SessionID           string `json:"session_id" binding:"required"`
	Role                string `json:"role" binding:"required,oneof=system user assistant tool"`
	Content             string `json:"content" binding:"required"`
	KnowledgeReferences string `json:"knowledge_references"`
	AgentSteps          string `json:"agent_steps"`
	ToolCalls           string `json:"tool_calls"`
	TokenCount          int    `json:"token_count"`
}

// UpdateMessageRequest 更新消息请求
type UpdateMessageRequest struct {
	Content             *string `json:"content"`
	IsCompleted         *bool   `json:"is_completed"`
	TokenCount          *int    `json:"token_count"`
	KnowledgeReferences *string `json:"knowledge_references"`
	AgentSteps          *string `json:"agent_steps"`
}

// MessageResponse 消息响应
type MessageResponse struct {
	ID                  string    `json:"id"`
	RequestID           string    `json:"request_id"`
	SessionID           string    `json:"session_id"`
	Role                string    `json:"role"`
	Content             string    `json:"content"`
	KnowledgeReferences string    `json:"knowledge_references"`
	AgentSteps          string    `json:"agent_steps"`
	ToolCalls           string    `json:"tool_calls"`
	IsCompleted         bool      `json:"is_completed"`
	TokenCount          int       `json:"token_count"`
	CreatedAt           time.Time `json:"created_at"`
}

// MessageListResponse 消息列表响应
type MessageListResponse struct {
	Messages []*MessageResponse `json:"messages"`
	Total    int64              `json:"total"`
	Page     int                `json:"page"`
	Size     int                `json:"size"`
}

// ListMessagesRequest 消息列表请求
type ListMessagesRequest struct {
	SessionID string `json:"session_id"`
	Page      int    `json:"page"`
	Size      int    `json:"size"`
	Role      string `json:"role"`
}

// CreateSessionRequest 创建会话请求
type CreateSessionRequest struct {
	AgentType   string     `json:"agent_type,omitempty"`   // Agent类型: rag, text2sql, default
	Title       string     `json:"title" binding:"max=255"`
	Description string     `json:"description"`
	RAGConfig   *RAGConfig `json:"rag_config,omitempty"`
}

// UpdateSessionRequest 更新会话请求
type UpdateSessionRequest struct {
	Title       *string    `json:"title" binding:"omitempty,max=255"`
	Description *string    `json:"description"`
	Status      *int8      `json:"status" binding:"omitempty,oneof=0 1"`
	RAGConfig   *RAGConfig `json:"rag_config,omitempty"`
}

// SessionResponse 会话响应
type SessionResponse struct {
	ID           string     `json:"id"`
	TenantID     int64      `json:"tenant_id"`
	UserID       int64      `json:"user_id"`
	AgentType    string     `json:"agent_type,omitempty"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Status       int8       `json:"status"`
	MessageCount int        `json:"message_count"`
	RAGConfig    *RAGConfig `json:"rag_config,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// SessionListResponse 会话列表响应
type SessionListResponse struct {
	Sessions []*SessionResponse `json:"sessions"`
	Total    int64              `json:"total"`
	Page     int                `json:"page"`
	Size     int                `json:"size"`
}

// RAGConfig RAG 配置
type RAGConfig struct {
	Enabled             bool     `json:"enabled"`
	KnowledgeBaseID     string   `json:"knowledge_base_id"`
	RetrievalModes      []string `json:"retrieval_modes"`
	VectorTopK          int      `json:"vector_top_k"`
	KeywordTopK         int      `json:"keyword_top_k"`
	GraphTopK           int      `json:"graph_top_k"`
	SimilarityThreshold float64  `json:"similarity_threshold"`
	Alpha               float32  `json:"alpha"`
}

// ListSessionsRequest 会话列表请求
type ListSessionsRequest struct {
	Page   int   `json:"page"`
	Size   int   `json:"size"`
	Status *int8 `json:"status"`
}

// SessionDetailResponse 会话详情响应
type SessionDetailResponse struct {
	Session  *conversation.Session   `json:"session"`
	Messages []*conversation.Message `json:"messages"`
}
