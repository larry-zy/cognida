// Package memory 提供 Memory 模块的领域服务接口
package memory

import "context"

// ========================================
// CompressionService 上下文压缩服务
// ========================================

// CompressionService 上下文压缩服务接口
// 负责智能上下文压缩和卸载
type CompressionService interface {
	// CheckAndCompress 检查并执行压缩
	// 根据当前 token 使用情况和配置策略，决定是否需要压缩
	CheckAndCompress(ctx context.Context, sessionID string) (*CompressionResult, error)

	// CompressHistory 执行历史消息压缩
	CompressHistory(ctx context.Context, sessionID string) (*CompressionResult, error)

	// OffloadLargeContent 卸载大内容到外部存储
	OffloadLargeContent(ctx context.Context, sessionID string) (*OffloadResult, error)

	// OffloadToolInputs 卸载工具调用输入
	OffloadToolInputs(ctx context.Context, sessionID string) (*OffloadResult, error)

	// GetCompressionStatus 获取压缩状态
	GetCompressionStatus(ctx context.Context, sessionID string) (*CompressionStatus, error)
}

// CompressionResult 压缩结果
type CompressionResult struct {
	SessionID          string
	Strategy           CompressionStrategy
	OriginalMessages   int
	OriginalTokens     int
	CompressedTokens   int
	TokensSaved        int
	CompressionRatio   float64
	SummaryID          string
	Duration           int64 // 毫秒
}

// OffloadResult 卸载结果
type OffloadResult struct {
	SessionID     string
	OffloadedCount int
	OffloadedSize  int64 // 字节
	OffloadedPaths []string
	Duration       int64 // 毫秒
}

// CompressionStatus 压缩状态
type CompressionStatus struct {
	SessionID         string
	TotalMessages     int
	TotalTokens       int
	CompressedCount   int
	OffloadedCount    int
	CompressionRatio  float64
	NeedsCompression  bool
	CompressionInProg bool
}

// ========================================
// ContextBuilder 上下文构建服务
// ========================================

// ContextBuilder 上下文构建服务接口
// 负责动态构建 Agent 上下文
type ContextBuilder interface {
	// Build 构建完整上下文
	Build(ctx context.Context, req *BuildContextRequest) (*BuildContext, error)

	// BuildSystemPrompt 构建系统提示词
	BuildSystemPrompt(ctx context.Context, templateID string, variables map[string]interface{}) (string, error)

	// BuildWithFewShot 构建 Few-shot 示例上下文
	BuildWithFewShot(ctx context.Context, req *BuildContextRequest, examples []*Message) (*BuildContext, error)

	// ValidateContext 验证上下文是否在 token 限制内
	ValidateContext(ctx context.Context, context *BuildContext, config *ContextBuilderConfig) error
}

// BuildContextRequest 上下文构建请求
type BuildContextRequest struct {
	SessionID          string
	TenantID           int64
	UserID             int64
	CurrentMessage     string
	Config             *ContextBuilderConfig

	// 可选：指定要包含的历史消息数量
	HistoryLimit       int

	// 可选：特定的长期记忆查询
	MemoryQuery        *MemorySearchQuery
}

// BuildContext 构建的上下文
type BuildContext struct {
	// 系统提示词
	SystemPrompt string

	// Few-shot 示例
	FewShots     []*Message

	// 对话历史
	History      []*Message

	// 摘要
	Summaries    []*Summary

	// 长期记忆
	LongTermMemories []*LongTermMemory

	// 元数据
	Metadata     *ContextBuildMetadata

	// 最终的 messages 列表（可直接传给 LLM）
	Messages     []*Message
}

// ========================================
// TokenCounter Token 计数器
// ========================================

// TokenCounter Token 计数器接口
type TokenCounter interface {
	// Count 计算 token 数量
	Count(text string) int

	// CountMessages 计算消息列表的 token 数量
	CountMessages(messages []*Message) int

	// CountApprox 粗略估算（更快，不精确）
	CountApprox(text string) int
}

// ========================================
// MemoryService 记忆编排服务
// ========================================

// MemoryService 记忆编排服务接口
// 协调短期记忆、长期记忆和上下文构建
type MemoryService interface {
	// SaveMessage 保存消息（写穿透：MySQL → Redis）
	SaveMessage(ctx context.Context, msg *Message) error

	// LoadHistory 加载历史（Cache-Aside：Redis → MySQL）
	LoadHistory(ctx context.Context, sessionID string, includeCompressed bool) ([]*Message, error)

	// LoadHistoryWithLimit 加载指定数量的最近消息
	LoadHistoryWithLimit(ctx context.Context, sessionID string, limit int) ([]*Message, error)

	// ClearSession 清空会话
	ClearSession(ctx context.Context, sessionID string) error

	// CompressContext 压缩上下文
	CompressContext(ctx context.Context, sessionID string) (*CompressionResult, error)

	// BuildContext 构建完整上下文
	BuildContext(ctx context.Context, req *BuildContextRequest) (*BuildContext, error)

	// UpdateSummary 更新会话摘要
	UpdateSummary(ctx context.Context, sessionID string, summary string) error

	// GetSummary 获取会话摘要
	GetSummary(ctx context.Context, sessionID string) (string, error)
}

// ========================================
// LongTermMemoryService 长期记忆服务
// ========================================

// LongTermMemoryService 长期记忆服务接口
type LongTermMemoryService interface {
	// Store 存储长期记忆
	Store(ctx context.Context, memory *LongTermMemory) error

	// Retrieve 根据 ID 获取记忆
	Retrieve(ctx context.Context, id string) (*LongTermMemory, error)

	// Search 搜索记忆
	Search(ctx context.Context, query *MemorySearchQuery) ([]*LongTermMemory, error)

	// Recall 智能回忆（综合评分排序）
	Recall(ctx context.Context, query *MemorySearchQuery, opts *RecallOptions) ([]*LongTermMemory, error)

	// Update 更新记忆
	Update(ctx context.Context, memory *LongTermMemory) error

	// Delete 删除记忆
	Delete(ctx context.Context, id string) error

	// ExtractFacts 从对话中提取原子事实
	ExtractFacts(ctx context.Context, sessionID string, messages []*Message) ([]*LongTermMemory, error)
}
