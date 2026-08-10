// Package cache provides semantic cache service implementation
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/google/uuid"

	domaincache "cognida/internal/model/cache"
	"cognida/internal/pkg/safego"
	milvuscache "cognida/internal/repository/milvus"
	rediscache "cognida/internal/repository/redis"
)

// ========================================
// Semantic Cache Service
// ========================================

// SemanticCacheService 语义缓存服务
type SemanticCacheService struct {
	vectorRepo  domaincache.CacheVectorRepository
	contentRepo domaincache.CacheContentRepository
	strategy    *domaincache.AgentCacheStrategy
	embedder    embedding.Embedder
}

// NewSemanticCacheService 创建语义缓存服务
func NewSemanticCacheService(
	vectorRepo domaincache.CacheVectorRepository,
	contentRepo domaincache.CacheContentRepository,
	strategy *domaincache.AgentCacheStrategy,
	embedder embedding.Embedder,
) *SemanticCacheService {
	return &SemanticCacheService{
		vectorRepo:  vectorRepo,
		contentRepo: contentRepo,
		strategy:    strategy,
		embedder:    embedder,
	}
}

// ========================================
// Core Cache Operations
// ========================================

// IsEnabled 检查缓存是否启用
func (s *SemanticCacheService) IsEnabled(agentType string) bool {
	config := s.strategy.GetAgentConfig(agentType)
	return config.Enabled
}

// Get 获取缓存
func (s *SemanticCacheService) Get(ctx context.Context, req *domaincache.CacheRequest) (*domaincache.CacheResponse, error) {
	// 1. 检查是否启用
	if !s.IsEnabled(req.AgentType) {
		return &domaincache.CacheResponse{FromCache: false}, nil
	}

	config := s.strategy.GetAgentConfig(req.AgentType)

	// 2. 生成查询向量
	vector, err := s.embedVector(ctx, req.Query)
	if err != nil {
		log.Printf("[Cache] Embedding failed, skipping cache: %v", err)
		return &domaincache.CacheResponse{FromCache: false}, nil
	}

	// 3. Milvus 向量检索
	searchResults, err := s.vectorRepo.Search(ctx, vector, req.TenantID, req.AgentType, config.TopK)
	if err != nil {
		log.Printf("[Cache] Vector search failed: %v", err)
		return &domaincache.CacheResponse{FromCache: false}, nil
	}

	if len(searchResults) == 0 {
		_ = s.contentRepo.RecordMiss(ctx)
		return &domaincache.CacheResponse{FromCache: false}, nil
	}

	// 4. 提取 cache_id 列表
	cacheIDs := make([]string, len(searchResults))
	for i, result := range searchResults {
		cacheIDs[i] = result.CacheID
	}

	// 5. Redis 批量获取内容
	entries, err := s.contentRepo.MGetContents(ctx, cacheIDs)
	if err != nil {
		log.Printf("[Cache] Content fetch failed: %v", err)
		return &domaincache.CacheResponse{FromCache: false}, nil
	}

	// 6. 计算 Prompt hash
	promptHash := s.promptHash(req.PromptTemplate)

	// 7. 按相似度降序检查候选结果
	for _, searchResult := range searchResults {
		entry, exists := entries[searchResult.CacheID]
		if !exists {
			continue // Redis 中不存在，可能是孤儿向量
		}

		// 检查 Prompt 模板是否匹配
		if entry.PromptHash != promptHash {
			continue
		}

		// 检查相似度阈值
		if searchResult.Similarity >= config.Threshold {
			// 命中缓存
			_ = s.contentRepo.UpdateHitCount(ctx, entry.CacheID)
			_ = s.contentRepo.RecordHit(ctx)

			// 记录相似度分布
			_ = s.contentRepo.IncrementSimilarityBucket(ctx, s.similarityBucket(searchResult.Similarity))

			log.Printf("[Cache] Hit: cache_id=%s, similarity=%.4f", entry.CacheID, searchResult.Similarity)

			return &domaincache.CacheResponse{
				Response:   entry.Response,
				CacheID:    entry.CacheID,
				Similarity: searchResult.Similarity,
				FromCache:  true,
				CreatedAt:  time.Unix(entry.CreatedAt, 0),
			}, nil
		}
	}

	// 所有候选都不满足条件
	_ = s.contentRepo.RecordMiss(ctx)
	return &domaincache.CacheResponse{FromCache: false}, nil
}

// RecallSimilar 软语义缓存（few-shot）召回：检索与当前查询相似的历史问答，
// 但**不短路返回**、**不校验 PromptHash**——仅作为参考示例交由上层注入上下文，
// 由 LLM 结合实时数据重新生成。用于 data agent 等"数据会变、必须重算"的场景。
//
// 与 Get 的差异：Get 是硬缓存（命中即返回、需 PromptHash 一致）；本方法是软缓存
// （返回多条示例、忽略 PromptHash、由调用方决定如何使用）。二者共用同一向量检索底座。
func (s *SemanticCacheService) RecallSimilar(ctx context.Context, req *domaincache.CacheRequest, topN int) ([]domaincache.RecalledExample, error) {
	if !s.IsEnabled(req.AgentType) {
		return nil, nil
	}
	config := s.strategy.GetAgentConfig(req.AgentType)
	if topN <= 0 {
		topN = config.TopK
	}
	if topN <= 0 {
		topN = 3
	}

	vector, err := s.embedVector(ctx, req.Query)
	if err != nil {
		log.Printf("[Cache] RecallSimilar embedding failed: %v", err)
		return nil, nil
	}

	searchResults, err := s.vectorRepo.Search(ctx, vector, req.TenantID, req.AgentType, topN)
	if err != nil {
		log.Printf("[Cache] RecallSimilar search failed: %v", err)
		return nil, nil
	}
	if len(searchResults) == 0 {
		return nil, nil
	}

	cacheIDs := make([]string, len(searchResults))
	for i, r := range searchResults {
		cacheIDs[i] = r.CacheID
	}
	entries, err := s.contentRepo.MGetContents(ctx, cacheIDs)
	if err != nil {
		log.Printf("[Cache] RecallSimilar content fetch failed: %v", err)
		return nil, nil
	}

	// searchResults 已按相似度降序；按序过阈值组装（不校验 PromptHash）。
	examples := make([]domaincache.RecalledExample, 0, len(searchResults))
	for _, sr := range searchResults {
		entry, ok := entries[sr.CacheID]
		if !ok || entry.Response == "" {
			continue // 孤儿向量或空内容
		}
		if sr.Similarity < config.Threshold {
			continue // 低于软阈值，作参考价值不足
		}
		examples = append(examples, domaincache.RecalledExample{
			Query:      entry.Query,
			Response:   entry.Response,
			Similarity: sr.Similarity,
		})
	}
	return examples, nil
}

// InvalidateByKB 按知识库失效硬缓存：删除该租户下归属 kbID 的全部缓存条目（向量+内容）。
// 供 RAG 知识库文档写入/删除后调用，避免命中陈旧答案。
func (s *SemanticCacheService) InvalidateByKB(ctx context.Context, tenantID int64, kbID string) error {
	vecRepo, ok := s.vectorRepo.(*milvuscache.CacheVectorRepository)
	if !ok {
		return fmt.Errorf("invalidate by kb: vector repo does not support kb-scoped delete")
	}
	if err := vecRepo.DeleteByKB(ctx, tenantID, kbID); err != nil {
		return fmt.Errorf("invalidate by kb failed: %w", err)
	}
	// 内容侧（Redis）无 kb 索引，靠 TTL 自然过期；向量删除后即不会再被检索命中。
	log.Printf("[Cache] Invalidated KB cache: tenant_id=%d, kb_id=%s", tenantID, kbID)
	return nil
}

// Set 设置缓存（异步）
func (s *SemanticCacheService) Set(ctx context.Context, req *domaincache.CacheRequest, resp string, tokensUsed int) error {
	config := s.strategy.GetAgentConfig(req.AgentType)
	if !config.Enabled {
		return nil
	}

	// 异步写入，不阻塞响应
	go func() {
		defer safego.Recover("semantic-cache-write")
		writeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := s.writeCache(writeCtx, req, resp, config.TTL); err != nil {
			log.Printf("[Cache] Write failed: %v", err)
		}
	}()

	return nil
}

// Clear 清除缓存
func (s *SemanticCacheService) Clear(ctx context.Context, tenantID int64, agentType, model string) error {
	var count int64
	var err error

	// 根据条件清除
	if agentType != "" && model != "" {
		// 按 Agent 和 Model 清除（需要遍历）
		count, err = s.clearByAgentAndModel(ctx, tenantID, agentType, model)
	} else if agentType != "" {
		// 按 Agent 清除 - 使用类型断言访问具体方法
		if repo, ok := s.contentRepo.(*rediscache.CacheContentRepository); ok {
			count, err = repo.ClearByAgent(ctx, tenantID, agentType)
		}
		if vecRepo, ok := s.vectorRepo.(*milvuscache.CacheVectorRepository); ok {
			_ = vecRepo.DeleteByAgent(ctx, tenantID, agentType)
		}
	} else {
		// 按租户清除全部
		if repo, ok := s.contentRepo.(*rediscache.CacheContentRepository); ok {
			count, err = repo.ClearByTenant(ctx, tenantID)
		}
		if vecRepo, ok := s.vectorRepo.(*milvuscache.CacheVectorRepository); ok {
			_ = vecRepo.DeleteByTenant(ctx, tenantID)
		}
	}

	if err != nil {
		return fmt.Errorf("clear cache failed: %w", err)
	}

	log.Printf("[Cache] Cleared %d entries", count)
	return nil
}

// GetStats 获取统计信息
func (s *SemanticCacheService) GetStats(ctx context.Context) (*domaincache.CacheStats, error) {
	return s.contentRepo.GetStats(ctx)
}

// ========================================
// Helper Methods
// ========================================

// embedVector 生成查询向量
func (s *SemanticCacheService) embedVector(ctx context.Context, text string) ([]float32, error) {
	// 检查 embedder 是否可用
	if s.embedder == nil {
		return nil, fmt.Errorf("embedder not configured")
	}

	embeddings, err := s.embedder.EmbedStrings(ctx, []string{text})
	if err != nil {
		return nil, fmt.Errorf("embedding failed: %w", err)
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings generated")
	}

	// 转换为 float32
	vector := make([]float32, len(embeddings[0]))
	for i, v := range embeddings[0] {
		vector[i] = float32(v)
	}
	return vector, nil
}

// promptHash 计算 Prompt 模板 hash
func (s *SemanticCacheService) promptHash(prompt string) string {
	hash := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(hash[:])[:16] // 取前 16 位
}

// similarityBucket 计算相似度区间
func (s *SemanticCacheService) similarityBucket(similarity float32) string {
	switch {
	case similarity >= 0.95:
		return "95-100"
	case similarity >= 0.90:
		return "90-95"
	case similarity >= 0.85:
		return "85-90"
	case similarity >= 0.80:
		return "80-85"
	case similarity >= 0.75:
		return "75-80"
	default:
		return "0-75"
	}
}

// writeCache 写入缓存（同步方法，用于 goroutine 调用）
func (s *SemanticCacheService) writeCache(ctx context.Context, req *domaincache.CacheRequest, resp string, ttl time.Duration) error {
	// 1. 生成 cache_id 和向量
	cacheID := generateCacheID(req.TenantID, req.AgentType)
	vector, err := s.embedVector(ctx, req.Query)
	if err != nil {
		return fmt.Errorf("embed vector failed: %w", err)
	}

	// 2. 构建 CacheEntry
	now := time.Now().Unix()
	entry := &domaincache.CacheEntry{
		CacheID:    cacheID,
		Query:      req.Query,
		Response:   resp,
		Model:      req.Model,
		AgentType:  req.AgentType,
		TenantID:   req.TenantID,
		PromptHash: s.promptHash(req.PromptTemplate),
		KBScope:    req.KBScope,
		Vector:     vector,
		CreatedAt:  now,
		UpdatedAt:  now,
		HitCount:   0,
		TTL:        ttl,
		Metadata:   req.Metadata,
	}

	// 3. 写入 Redis
	if err := s.contentRepo.SetContent(ctx, entry, ttl); err != nil {
		return fmt.Errorf("write to redis failed: %w", err)
	}

	// 4. 写入 Milvus
	if err := s.vectorRepo.Insert(ctx, entry); err != nil {
		// Milvus 写入失败不影响 Redis，但记录错误
		log.Printf("[Cache] Milvus insert failed: %v", err)
	}

	log.Printf("[Cache] Written: cache_id=%s", cacheID)
	return nil
}

// clearByAgentAndModel 按 Agent 和 Model 清除缓存
func (s *SemanticCacheService) clearByAgentAndModel(ctx context.Context, tenantID int64, agentType, model string) (int64, error) {
	// 先获取 Agent 类型下所有缓存 ID
	_ = fmt.Sprintf("tenant:%d:agent:%s:caches", tenantID, agentType) // 预留的 key 格式

	// 需要访问 Redis 客户端来获取 SMembers
	// 这里使用 contentRepo 的具体实现
	if repo, ok := s.contentRepo.(*rediscache.CacheContentRepository); ok {
		cacheIDs, err := repo.GetAgentCacheIDs(ctx, tenantID, agentType)
		if err != nil {
			return 0, err
		}

		// 筛选匹配模型的缓存
		targetIDs := make([]string, 0)
		for _, cacheID := range cacheIDs {
			entry, err := s.contentRepo.GetContent(ctx, cacheID)
			if err != nil {
				continue
			}
			if entry.Model == model {
				targetIDs = append(targetIDs, cacheID)
			}
		}

		// 批量删除
		for _, cacheID := range targetIDs {
			_ = s.contentRepo.Delete(ctx, cacheID)
			_ = s.vectorRepo.Delete(ctx, cacheID)
		}

		return int64(len(targetIDs)), nil
	}

	return 0, fmt.Errorf("content repo type assertion failed")
}

// ========================================
// Configuration Management
// ========================================

// UpdateStrategy 更新缓存策略
func (s *SemanticCacheService) UpdateStrategy(strategy *domaincache.AgentCacheStrategy) {
	s.strategy = strategy
	log.Printf("[Cache] Strategy updated")
}

// GetStrategy 获取当前缓存策略
func (s *SemanticCacheService) GetStrategy() *domaincache.AgentCacheStrategy {
	return s.strategy
}

// ========================================
// Package Helper Functions
// ========================================

// generateCacheID 生成缓存 ID
func generateCacheID(tenantID int64, agentType string) string {
	return fmt.Sprintf("%s_%d_%s", uuid.New().String()[:8], tenantID, agentType)
}
