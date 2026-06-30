// Package memory 提供 Memory 管理用例
package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"link/internal/model/memory"
	domainllm "link/internal/model/llm"
	redisStore "link/internal/repository/redis"
)

// ========================================
// SaveMessageUseCase 保存消息用例
// ========================================

// SaveMessageUseCase 保存消息用例（写穿透：MySQL → Redis）
type SaveMessageUseCase struct {
	repo    memory.MemoryRepository
	cache   *redisStore.RedisMemoryStore
	counter domainllm.TokenCounter
}

// NewSaveMessageUseCase 创建保存消息用例
func NewSaveMessageUseCase(
	repo memory.MemoryRepository,
	cache *redisStore.RedisMemoryStore,
	counter domainllm.TokenCounter,
) *SaveMessageUseCase {
	return &SaveMessageUseCase{
		repo:    repo,
		cache:   cache,
		counter: counter,
	}
}

// Execute 执行保存消息
func (uc *SaveMessageUseCase) Execute(ctx context.Context, msg *memory.Message) error {
	// 1. 设置 token 数量（如果未设置）
	if msg.Tokens == 0 {
		msg.Tokens = uc.counter.CountTokens(msg.Content)
	}

	// 2. 保存到 MySQL（主存储）
	if err := uc.repo.SaveMessage(ctx, msg); err != nil {
		return fmt.Errorf("failed to save message to repo: %w", err)
	}

	// 3. 异步写 Redis（缓存）
	go func() {
		cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// 忽略缓存错误，不影响主流程
		_ = uc.cache.SaveMessage(cacheCtx, msg)
	}()

	return nil
}

// ========================================
// LoadHistoryUseCase 加载历史用例
// ========================================

// LoadHistoryUseCase 加载历史用例（Cache-Aside：Redis → MySQL）
type LoadHistoryUseCase struct {
	repo    memory.MemoryRepository
	cache   *redisStore.RedisMemoryStore
	config  *memory.MemoryConfig
}

// NewLoadHistoryUseCase 创建加载历史用例
func NewLoadHistoryUseCase(
	repo memory.MemoryRepository,
	cache *redisStore.RedisMemoryStore,
	config *memory.MemoryConfig,
) *LoadHistoryUseCase {
	return &LoadHistoryUseCase{
		repo:   repo,
		cache:  cache,
		config: config,
	}
}

// Execute 执行加载历史
func (uc *LoadHistoryUseCase) Execute(ctx context.Context, sessionID string, limit int) ([]*memory.Message, error) {
	// 1. 尝试从 Redis 加载
	if uc.config.StorageBackend == "redis" || uc.config.StorageBackend == "hybrid" {
		messages, err := uc.cache.LoadHistory(ctx, sessionID, limit)
		if err == nil && len(messages) > 0 {
			// 缓存命中，直接返回
			return messages, nil
		}
	}

	// 2. 从 MySQL 加载
	var messages []*memory.Message
	var err error

	if limit > 0 {
		messages, err = uc.repo.LoadHistoryWithLimit(ctx, sessionID, limit, false)
	} else {
		messages, err = uc.repo.LoadHistory(ctx, sessionID, false)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load history from repo: %w", err)
	}

	// 3. 异步预热缓存
	if (uc.config.StorageBackend == "redis" || uc.config.StorageBackend == "hybrid") && len(messages) > 0 {
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = uc.cache.WarmUpCache(cacheCtx, sessionID, messages)
		}()
	}

	return messages, nil
}

// ========================================
// ClearSessionUseCase 清空会话用例
// ========================================

// ClearSessionUseCase 清空会话用例
type ClearSessionUseCase struct {
	repo  memory.MemoryRepository
	cache *redisStore.RedisMemoryStore
}

// NewClearSessionUseCase 创建清空会话用例
func NewClearSessionUseCase(
	repo memory.MemoryRepository,
	cache *redisStore.RedisMemoryStore,
) *ClearSessionUseCase {
	return &ClearSessionUseCase{
		repo:  repo,
		cache: cache,
	}
}

// Execute 执行清空会话
func (uc *ClearSessionUseCase) Execute(ctx context.Context, sessionID string) error {
	// 1. 清空 MySQL
	if err := uc.repo.ClearSession(ctx, sessionID); err != nil {
		return fmt.Errorf("failed to clear session from repo: %w", err)
	}

	// 2. 清空 Redis 缓存
	go func() {
		cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = uc.cache.ClearSession(cacheCtx, sessionID)
	}()

	return nil
}

// ========================================
// CompressContextUseCase 压缩上下文用例
// ========================================

// CompressContextUseCase 压缩上下文用例
type CompressContextUseCase struct {
	compressionService memory.CompressionService
}

// NewCompressContextUseCase 创建压缩上下文用例
func NewCompressContextUseCase(
	compressionService memory.CompressionService,
) *CompressContextUseCase {
	return &CompressContextUseCase{
		compressionService: compressionService,
	}
}

// Execute 执行压缩上下文
func (uc *CompressContextUseCase) Execute(ctx context.Context, sessionID string) (*memory.CompressionResult, error) {
	return uc.compressionService.CheckAndCompress(ctx, sessionID)
}

// ========================================
// GetSessionStatsUseCase 获取会话统计用例
// ========================================

// GetSessionStatsUseCase 获取会话统计用例
type GetSessionStatsUseCase struct {
	repo  memory.MemoryRepository
	cache *redisStore.RedisMemoryStore
}

// NewGetSessionStatsUseCase 创建获取会话统计用例
func NewGetSessionStatsUseCase(
	repo memory.MemoryRepository,
	cache *redisStore.RedisMemoryStore,
) *GetSessionStatsUseCase {
	return &GetSessionStatsUseCase{
		repo:  repo,
		cache: cache,
	}
}

// SessionStats 会话统计信息
type SessionStats struct {
	SessionID     string
	MessageCount  int
	TokenCount    int
	CacheHit      bool
	LastMessageAt time.Time
}

// Execute 执行获取会话统计
func (uc *GetSessionStatsUseCase) Execute(ctx context.Context, sessionID string) (*SessionStats, error) {
	// 从 MySQL 获取统计
	usage, err := uc.repo.GetTokenUsage(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get token usage: %w", err)
	}

	// 检查缓存状态
	cacheStats, _ := uc.cache.GetCacheStats(ctx, sessionID)

	return &SessionStats{
		SessionID:    sessionID,
		MessageCount: usage.TotalMessages,
		TokenCount:   usage.TotalTokens,
		CacheHit:     cacheStats != nil && cacheStats.MessageCount > 0,
	}, nil
}

// ========================================
// MemoryService 记忆编排服务实现
// ========================================

// MemoryService 记忆编排服务实现
type MemoryService struct {
	repo              memory.MemoryRepository
	cache             *redisStore.RedisMemoryStore
	tokenCounter      domainllm.TokenCounter
	compressionService memory.CompressionService
	contextBuilder    memory.ContextBuilder
	config            *memory.MemoryConfig
}

// NewMemoryService 创建记忆编排服务
func NewMemoryService(
	repo memory.MemoryRepository,
	cache *redisStore.RedisMemoryStore,
	tokenCounter domainllm.TokenCounter,
	compressionService memory.CompressionService,
	contextBuilder memory.ContextBuilder,
	config *memory.MemoryConfig,
) memory.MemoryService {
	if config == nil {
		config = memory.DefaultMemoryConfig()
	}

	return &MemoryService{
		repo:              repo,
		cache:             cache,
		tokenCounter:      tokenCounter,
		compressionService: compressionService,
		contextBuilder:    contextBuilder,
		config:            config,
	}
}

// SaveMessage 保存消息（写穿透：MySQL → Redis）
func (s *MemoryService) SaveMessage(ctx context.Context, msg *memory.Message) error {
	uc := NewSaveMessageUseCase(s.repo, s.cache, s.tokenCounter)
	return uc.Execute(ctx, msg)
}

// LoadHistory 加载历史（Cache-Aside：Redis → MySQL）
func (s *MemoryService) LoadHistory(ctx context.Context, sessionID string, includeCompressed bool) ([]*memory.Message, error) {
	uc := NewLoadHistoryUseCase(s.repo, s.cache, s.config)
	return uc.Execute(ctx, sessionID, 0)
}

// LoadHistoryWithLimit 加载指定数量的最近消息
func (s *MemoryService) LoadHistoryWithLimit(ctx context.Context, sessionID string, limit int) ([]*memory.Message, error) {
	uc := NewLoadHistoryUseCase(s.repo, s.cache, s.config)
	return uc.Execute(ctx, sessionID, limit)
}

// ClearSession 清空会话
func (s *MemoryService) ClearSession(ctx context.Context, sessionID string) error {
	uc := NewClearSessionUseCase(s.repo, s.cache)
	return uc.Execute(ctx, sessionID)
}

// CompressContext 压缩上下文
func (s *MemoryService) CompressContext(ctx context.Context, sessionID string) (*memory.CompressionResult, error) {
	uc := NewCompressContextUseCase(s.compressionService)
	return uc.Execute(ctx, sessionID)
}

// BuildContext 构建完整上下文
func (s *MemoryService) BuildContext(ctx context.Context, req *memory.BuildContextRequest) (*memory.BuildContext, error) {
	return s.contextBuilder.Build(ctx, req)
}

// UpdateSummary 更新会话摘要
func (s *MemoryService) UpdateSummary(ctx context.Context, sessionID string, summary string) error {
	// 创建或更新摘要
	// 使用 Redis 存储摘要，设置与消息相同的 TTL
	key := s.buildSummaryKey(sessionID)
	return s.cache.Set(ctx, key, summary, 2*time.Hour)
}

// GetSummary 获取会话摘要
func (s *MemoryService) GetSummary(ctx context.Context, sessionID string) (string, error) {
	key := s.buildSummaryKey(sessionID)
	summary, err := s.cache.Get(ctx, key)
	if err != nil {
		return "", nil // 不存在返回空字符串，不报错
	}
	return summary, nil
}

// buildSummaryKey 构建摘要缓存键
func (s *MemoryService) buildSummaryKey(sessionID string) string {
	return "session:summary:" + sessionID
}

// ========================================
// RedisTokenCounter Redis 包装器
// ========================================

// RedisTokenCounter Redis Token 计数器包装器
// 这个类型用于将 infrastructure 层的类型转换为应用层可用
type RedisTokenCounter struct {
	counter interface{} // 实际的 TokenCounter 实现
}

// Count 计算 token 数量
func (c *RedisTokenCounter) Count(text string) int {
	// 这里需要调用实际的 token counter 实现
	// 为了避免循环依赖，使用接口
	if tc, ok := c.counter.(interface {
		Count(string) int
	}); ok {
		return tc.Count(text)
	}
	return 0
}

// CountMessages 计算消息列表的 token 数量
func (c *RedisTokenCounter) CountMessages(messages []*memory.Message) int {
	if tc, ok := c.counter.(interface {
		CountMessages([]*memory.Message) int
	}); ok {
		return tc.CountMessages(messages)
	}
	return 0
}

// CountApprox 粗略估算
func (c *RedisTokenCounter) CountApprox(text string) int {
	if tc, ok := c.counter.(interface {
		CountApprox(string) int
	}); ok {
		return tc.CountApprox(text)
	}
	return len(text) / 4
}

// ========================================
// RedisHealthCheck Redis 健康检查
// ========================================

// RedisHealthCheck Redis 健康检查
func RedisHealthCheck(client *redis.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	return client.Ping(ctx).Err()
}
