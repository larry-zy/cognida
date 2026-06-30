// Package llm provides LLM use case implementations with semantic cache support
package chat

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	domaincache "link/internal/model/cache"
	domainllm "link/internal/model/llm"
)

// ========================================
// Cached Chat Model Wrapper
// ========================================

// CachedChatModel 带语义缓存的聊天模型包装器
type CachedChatModel struct {
	chatModel   model.ChatModel
	cacheUseCase domaincache.SemanticCache
	agentType    string
}

// NewCachedChatModel 创建带缓存的聊天模型
func NewCachedChatModel(
	chatModel model.ChatModel,
	cacheUseCase domaincache.SemanticCache,
	agentType string,
) *CachedChatModel {
	return &CachedChatModel{
		chatModel:   chatModel,
		cacheUseCase: cacheUseCase,
		agentType:    agentType,
	}
}

// Generate 生成响应（带缓存）
func (m *CachedChatModel) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	// 1. 提取查询和 Prompt 模板
	query, promptTemplate := extractQueryAndTemplate(messages)

	// 2. 构建缓存请求
	cacheReq := &domaincache.CacheRequest{
		Query:          query,
		PromptTemplate: promptTemplate,
		TenantID:       getTenantID(ctx),
		AgentType:      m.agentType,
		Model:         getModelName(opts...),
	}

	// 3. 尝试从缓存获取
	cacheResp, err := m.cacheUseCase.Get(ctx, cacheReq)
	if err != nil {
		log.Printf("[Cache] Get failed: %v", err)
		// 继续调用 LLM
	} else if cacheResp.FromCache {
		log.Printf("[Cache] Hit: cache_id=%s, similarity=%.4f", cacheResp.CacheID, cacheResp.Similarity)
		// 返回缓存响应
		return &schema.Message{
			Role:      schema.Assistant,
			Content:   cacheResp.Response,
			Extra:     map[string]interface{}{
				"from_cache":  true,
				"cache_id":    cacheResp.CacheID,
				"similarity":  cacheResp.Similarity,
				"created_at":  cacheResp.CreatedAt,
			},
		}, nil
	}

	// 4. 缓存未命中，调用 LLM
	llmResp, err := m.chatModel.Generate(ctx, messages, opts...)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// 5. 异步写入缓存
	go func() {
		writeCtx, cancel := context.WithTimeout(context.Background(), contextTimeout)
		defer cancel()

		response := extractContent(llmResp)
		tokensUsed := extractTokenUsage(llmResp)

		if err := m.cacheUseCase.Set(writeCtx, cacheReq, response, tokensUsed); err != nil {
			log.Printf("[Cache] Set failed: %v", err)
		}
	}()

	return llmResp, nil
}

// Stream 流式生成（暂不缓存）
func (m *CachedChatModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	// 流式响应暂不缓存，直接调用底层模型
	return m.chatModel.Stream(ctx, messages, opts...)
}

// ========================================
// Helper Functions
// ========================================

const contextTimeout = 30 * time.Second

// extractQueryAndTemplate 从消息中提取查询和 Prompt 模板
func extractQueryAndTemplate(messages []*schema.Message) (query, template string) {
	if len(messages) == 0 {
		return "", ""
	}

	// 最后一条用户消息作为查询
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == schema.User {
			query = extractContent(messages[i])
			break
		}
	}

	// 构建模板（包含系统提示和历史对话）
	template = buildPromptTemplate(messages)

	return query, template
}

// extractContent 提取消息内容
func extractContent(msg *schema.Message) string {
	if msg == nil {
		return ""
	}
	// 在 eino v0.7.32 中，Content 是 string 类型
	return msg.Content
}

// extractTokenUsage 提取 token 使用量
func extractTokenUsage(msg *schema.Message) int {
	if msg == nil || msg.Extra == nil {
		return 0
	}
	if usage, ok := msg.Extra["usage"].(map[string]interface{}); ok {
		if totalTokens, ok := usage["total_tokens"].(int); ok {
			return totalTokens
		}
		if totalTokens, ok := usage["total_tokens"].(int64); ok {
			return int(totalTokens)
		}
	}
	return 0
}

// buildPromptTemplate 构建 Prompt 模板
func buildPromptTemplate(messages []*schema.Message) string {
	if len(messages) == 0 {
		return ""
	}

	var template string
	for _, msg := range messages {
		template += fmt.Sprintf("[%s] %s\n", msg.Role, extractContent(msg))
	}
	return template
}

// getTenantID 从上下文获取租户 ID
func getTenantID(ctx context.Context) int64 {
	if tenantID, ok := ctx.Value("tenant_id").(int64); ok {
		return tenantID
	}
	return 0 // 默认租户
}

// getModelName 从选项获取模型名称
// 注意：由于 eino 的 Option 是未导出的，这里简化处理，返回默认值
func getModelName(opts ...model.Option) string {
	// TODO: 如果需要支持动态模型配置，可以通过其他方式传递模型名称
	return "default"
}

// ========================================
// LLM Client Wrapper (Domain Interface)
// ========================================

// CachedLLMClient 带缓存的 LLM 客户端包装器
type CachedLLMClient struct {
	llmClient    domainllm.LLMClient
	cacheUseCase domaincache.SemanticCache
	agentType    string
}

// NewCachedLLMClient 创建带缓存的 LLM 客户端
func NewCachedLLMClient(
	llmClient domainllm.LLMClient,
	cacheUseCase domaincache.SemanticCache,
	agentType string,
) *CachedLLMClient {
	return &CachedLLMClient{
		llmClient:    llmClient,
		cacheUseCase: cacheUseCase,
		agentType:    agentType,
	}
}

// Chat 聊天（带缓存）
func (r *CachedLLMClient) Chat(ctx context.Context, req *domainllm.ChatRequest) (*domainllm.ChatResponse, error) {
	// 1. 从 Messages 中提取查询（最后一条用户消息）
	query := extractQueryFromMessages(req.Messages)
	promptTemplate := buildPromptTemplateFromPtrMessages(req.Messages)
	tenantID := getTenantID(ctx)

	// 2. 构建缓存请求
	cacheReq := &domaincache.CacheRequest{
		Query:          query,
		PromptTemplate: promptTemplate,
		TenantID:       tenantID,
		AgentType:      r.agentType,
		Model:          req.Model,
	}

	// 3. 尝试从缓存获取
	cacheResp, err := r.cacheUseCase.Get(ctx, cacheReq)
	if err == nil && cacheResp.FromCache {
		return &domainllm.ChatResponse{
			Content:   cacheResp.Response,
			Message:   &domainllm.Message{Role: "assistant", Content: cacheResp.Response},
			Metadata: map[string]interface{}{
				"from_cache": true,
				"cache_id":   cacheResp.CacheID,
				"similarity": cacheResp.Similarity,
			},
		}, nil
	}

	// 4. 缓存未命中，调用 LLM
	llmResp, err := r.llmClient.Chat(ctx, req)
	if err != nil {
		return nil, err
	}

	// 5. 异步写入缓存
	go func() {
		writeCtx, cancel := context.WithTimeout(context.Background(), contextTimeout)
		defer cancel()
		tokensUsed := 0
		if llmResp.Usage != nil {
			tokensUsed = llmResp.Usage.TotalTokens
		}
		_ = r.cacheUseCase.Set(writeCtx, cacheReq, llmResp.Content, tokensUsed)
	}()

	return llmResp, nil
}

// ChatStream 流式聊天（暂不缓存）
func (r *CachedLLMClient) ChatStream(ctx context.Context, req *domainllm.ChatRequest) (<-chan *domainllm.ChatChunk, error) {
	// 流式响应暂不缓存
	return r.llmClient.ChatStream(ctx, req)
}

// ========================================
// Domain LLMClient Interface Methods
// ========================================

// GetModelInfo 获取模型信息
func (r *CachedLLMClient) GetModelInfo(ctx context.Context) (*domainllm.ModelInfo, error) {
	return r.llmClient.GetModelInfo(ctx)
}

// SupportsTools 检查是否支持工具
func (r *CachedLLMClient) SupportsTools() bool {
	return r.llmClient.SupportsTools()
}

// SupportsStreaming 检查是否支持流式
func (r *CachedLLMClient) SupportsStreaming() bool {
	return r.llmClient.SupportsStreaming()
}

// ========================================
// 向后兼容别名
// ========================================

// CachedChatRepository 带缓存的 LLM 聊天仓储（别名，向后兼容）
// @Deprecated: 使用 CachedLLMClient
type CachedChatRepository = CachedLLMClient

// NewCachedChatRepository 创建带缓存的 LLM 聊天仓储（别名，向后兼容）
// @Deprecated: 使用 NewCachedLLMClient
func NewCachedChatRepository(
	llmClient domainllm.LLMClient,
	cacheUseCase domaincache.SemanticCache,
	agentType string,
) *CachedLLMClient {
	return NewCachedLLMClient(llmClient, cacheUseCase, agentType)
}

// buildPromptTemplateFromPtrMessages 从指针消息列表构建模板
func buildPromptTemplateFromPtrMessages(messages []*domainllm.Message) string {
	var template string
	for _, msg := range messages {
		if msg != nil {
			template += fmt.Sprintf("[%s] %s\n", msg.Role, msg.Content)
		}
	}
	return template
}

// extractQueryFromMessages 从消息中提取查询（最后一条用户消息）
func extractQueryFromMessages(messages []*domainllm.Message) string {
	// 从后往前找最后一条用户消息
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i] != nil && messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
}
