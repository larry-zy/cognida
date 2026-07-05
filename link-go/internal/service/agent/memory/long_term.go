// Package agent memory provides long-term memory use cases
package memory

import (
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"time"

	"link/internal/model/memory"
)

// ========================================
// StoreMemoryUseCase 存储长期记忆用例
// ========================================

// StoreMemoryUseCase 存储长期记忆用例
type StoreMemoryUseCase struct {
	repo    memory.LongTermMemoryRepository
	embedder EmbeddingGenerator // 向量生成器
}

// EmbeddingGenerator 向量生成器接口
type EmbeddingGenerator interface {
	GenerateEmbedding(ctx context.Context, text string) ([]float32, error)
}

// NewStoreMemoryUseCase 创建存储长期记忆用例
func NewStoreMemoryUseCase(
	repo memory.LongTermMemoryRepository,
	embedder EmbeddingGenerator,
) *StoreMemoryUseCase {
	return &StoreMemoryUseCase{
		repo:    repo,
		embedder: embedder,
	}
}

// Execute 执行存储记忆
func (uc *StoreMemoryUseCase) Execute(ctx context.Context, mem *memory.LongTermMemory) error {
	// 1. 生成向量嵌入（如果需要）
	if len(mem.Embedding) == 0 && uc.embedder != nil {
		embedding, err := uc.embedder.GenerateEmbedding(ctx, mem.Content)
		if err != nil {
			return fmt.Errorf("failed to generate embedding: %w", err)
		}
		mem.Embedding = embedding
	}

	// 2. 设置默认重要性（如果未设置）
	if mem.Importance == 0 {
		mem.Importance = 0.5
	}

	// 3. 设置时间戳
	now := time.Now()
	mem.CreatedAt = now
	mem.UpdatedAt = now

	// 4. 存储到仓库
	if err := uc.repo.Store(ctx, mem); err != nil {
		return fmt.Errorf("failed to store memory: %w", err)
	}

	return nil
}

// ========================================
// RetrieveMemoryUseCase 检索长期记忆用例
// ========================================

// RetrieveMemoryUseCase 检索长期记忆用例
type RetrieveMemoryUseCase struct {
	repo memory.LongTermMemoryRepository
}

// NewRetrieveMemoryUseCase 创建检索长期记忆用例
func NewRetrieveMemoryUseCase(
	repo memory.LongTermMemoryRepository,
) *RetrieveMemoryUseCase {
	return &RetrieveMemoryUseCase{
		repo: repo,
	}
}

// Execute 执行检索记忆
func (uc *RetrieveMemoryUseCase) Execute(ctx context.Context, id string) (*memory.LongTermMemory, error) {
	mem, err := uc.repo.Retrieve(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve memory: %w", err)
	}

	// 异步更新访问统计：repo 层原子自增（UPDATE ... access_count=access_count+1），
	// 不在 goroutine 里改写已返回给调用方的 *mem（data race）
	memID := mem.ID
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_ = uc.repo.RecordAccess(bgCtx, memID)
	}()

	return mem, nil
}

// ========================================
// SearchMemoriesUseCase 搜索长期记忆用例
// ========================================

// SearchMemoriesUseCase 搜索长期记忆用例
type SearchMemoriesUseCase struct {
	repo    memory.LongTermMemoryRepository
	embedder EmbeddingGenerator
}

// NewSearchMemoriesUseCase 创建搜索长期记忆用例
func NewSearchMemoriesUseCase(
	repo memory.LongTermMemoryRepository,
	embedder EmbeddingGenerator,
) *SearchMemoriesUseCase {
	return &SearchMemoriesUseCase{
		repo:    repo,
		embedder: embedder,
	}
}

// Execute 执行搜索记忆
func (uc *SearchMemoriesUseCase) Execute(ctx context.Context, query *memory.MemorySearchQuery) ([]*memory.LongTermMemory, error) {
	// 如果提供了查询文本但没有向量，先生成向量
	if len(query.QueryEmbedding) == 0 && uc.embedder != nil {
		// 这里简化处理，实际应该有查询文本字段
		return []*memory.LongTermMemory{}, fmt.Errorf("query embedding required")
	}

	memories, err := uc.repo.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to search memories: %w", err)
	}

	return memories, nil
}

// ========================================
// RecallMemoriesUseCase 智能回忆用例
// ========================================

// RecallMemoriesUseCase 智能回忆用例
type RecallMemoriesUseCase struct {
	searchUC *SearchMemoriesUseCase
}

// NewRecallMemoriesUseCase 创建智能回忆用例
func NewRecallMemoriesUseCase(
	searchUC *SearchMemoriesUseCase,
) *RecallMemoriesUseCase {
	return &RecallMemoriesUseCase{
		searchUC: searchUC,
	}
}

// Execute 执行智能回忆
func (uc *RecallMemoriesUseCase) Execute(
	ctx context.Context,
	query *memory.MemorySearchQuery,
	opts *memory.RecallOptions,
) ([]*memory.LongTermMemory, error) {
	if opts == nil {
		opts = memory.DefaultRecallOptions()
	}

	// 设置默认值
	if query.MaxResults == 0 {
		query.MaxResults = opts.MaxResults
	}
	if query.MinImportance == 0 {
		query.MinImportance = opts.MinImportance
	}

	// 执行搜索
	memories, err := uc.searchUC.Execute(ctx, query)
	if err != nil {
		return nil, err
	}

	// 计算综合评分并排序
	// 这里简化处理，实际应该根据相似度、时间、重要性计算
	return memories, nil
}

// ========================================
// ExtractFactsUseCase 提取原子事实用例
// ========================================

// ExtractFactsUseCase 提取原子事实用例
type ExtractFactsUseCase struct {
	storeUC *StoreMemoryUseCase
	llmClient FactExtractor
}

// FactExtractor 事实提取器接口
type FactExtractor interface {
	ExtractFacts(ctx context.Context, messages []*memory.Message) ([]*Fact, error)
}

// Fact 原子事实
type Fact struct {
	Content    string
	Category   string
	Importance float64
}

// NewExtractFactsUseCase 创建提取原子事实用例
func NewExtractFactsUseCase(
	storeUC *StoreMemoryUseCase,
	llmClient FactExtractor,
) *ExtractFactsUseCase {
	return &ExtractFactsUseCase{
		storeUC:  storeUC,
		llmClient: llmClient,
	}
}

// Execute 执行提取原子事实
func (uc *ExtractFactsUseCase) Execute(
	ctx context.Context,
	sessionID string,
	messages []*memory.Message,
) ([]*memory.LongTermMemory, error) {
	// 1. 调用 LLM 提取事实
	facts, err := uc.llmClient.ExtractFacts(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to extract facts: %w", err)
	}

	// 2. 转换为长期记忆
	memories := make([]*memory.LongTermMemory, len(facts))
	for i, fact := range facts {
		memories[i] = &memory.LongTermMemory{
			ID:         generateID(),
			Content:    fact.Content,
			Category:   fact.Category,
			Importance: fact.Importance,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
	}

	// 3. 存储到仓库
	for _, mem := range memories {
		if err := uc.storeUC.Execute(ctx, mem); err != nil {
			// 继续存储其他的，不中断
			continue
		}
	}

	return memories, nil
}

// ========================================
// LongTermMemoryService 长期记忆服务实现
// ========================================

// LongTermMemoryService 长期记忆服务实现
type LongTermMemoryService struct {
	storeUC   *StoreMemoryUseCase
	retrieveUC *RetrieveMemoryUseCase
	searchUC  *SearchMemoriesUseCase
	recallUC  *RecallMemoriesUseCase
	extractUC *ExtractFactsUseCase
}

// NewLongTermMemoryService 创建长期记忆服务
func NewLongTermMemoryService(
	repo memory.LongTermMemoryRepository,
	embedder EmbeddingGenerator,
	factExtractor FactExtractor,
) memory.LongTermMemoryService {
	storeUC := NewStoreMemoryUseCase(repo, embedder)
	retrieveUC := NewRetrieveMemoryUseCase(repo)
	searchUC := NewSearchMemoriesUseCase(repo, embedder)
	recallUC := NewRecallMemoriesUseCase(searchUC)
	extractUC := NewExtractFactsUseCase(storeUC, factExtractor)

	return &LongTermMemoryService{
		storeUC:   storeUC,
		retrieveUC: retrieveUC,
		searchUC:  searchUC,
		recallUC:  recallUC,
		extractUC: extractUC,
	}
}

// Store 存储长期记忆
func (s *LongTermMemoryService) Store(ctx context.Context, memory *memory.LongTermMemory) error {
	return s.storeUC.Execute(ctx, memory)
}

// Retrieve 根据 ID 获取记忆
func (s *LongTermMemoryService) Retrieve(ctx context.Context, id string) (*memory.LongTermMemory, error) {
	return s.retrieveUC.Execute(ctx, id)
}

// Search 搜索记忆
func (s *LongTermMemoryService) Search(ctx context.Context, query *memory.MemorySearchQuery) ([]*memory.LongTermMemory, error) {
	return s.searchUC.Execute(ctx, query)
}

// Recall 智能回忆
func (s *LongTermMemoryService) Recall(
	ctx context.Context,
	query *memory.MemorySearchQuery,
	opts *memory.RecallOptions,
) ([]*memory.LongTermMemory, error) {
	return s.recallUC.Execute(ctx, query, opts)
}

// Update 更新记忆
func (s *LongTermMemoryService) Update(ctx context.Context, memory *memory.LongTermMemory) error {
	return s.retrieveUC.repo.Update(ctx, memory)
}

// Delete 删除记忆
func (s *LongTermMemoryService) Delete(ctx context.Context, id string) error {
	return s.retrieveUC.repo.Delete(ctx, id)
}

// ExtractFacts 从对话中提取原子事实
func (s *LongTermMemoryService) ExtractFacts(
	ctx context.Context,
	sessionID string,
	messages []*memory.Message,
) ([]*memory.LongTermMemory, error) {
	return s.extractUC.Execute(ctx, sessionID, messages)
}

// ========================================
// 辅助函数
// ========================================

// generateID 生成唯一 ID
func generateID() string {
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), randomString(8))
}

// randomString 生成随机字符串（crypto/rand：同一纳秒内的并发调用也不会重复）
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	if _, err := cryptorand.Read(b); err != nil {
		// crypto/rand 不可用属于系统级故障，直接 panic 比静默重复 ID 安全
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b)
}
