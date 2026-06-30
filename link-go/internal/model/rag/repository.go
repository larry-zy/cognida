// Package rag 提供 RAG 领域仓储接口定义
package rag

import (
	"context"
)

// ========================================
// Retriever 检索器接口
// ========================================

// Retriever 检索器接口
type Retriever interface {
	// Retrieve 执行检索
	Retrieve(ctx context.Context, tenantID, kbID, query string, opts *RetrieveOptions) (*RetrieveResponse, error)

	// RetrieveWithEmbedding 使用提供的向量执行检索
	RetrieveWithEmbedding(ctx context.Context, tenantID, kbID, query string, embedding []float32, opts *RetrieveOptions) (*RetrieveResponse, error)

	// VectorRetrieve 向量检索
	VectorRetrieve(ctx context.Context, tenantID, kbID, query string, opts *RetrieveOptions) (*RetrieveResponse, error)

	// BM25Retrieve BM25 关键词检索
	BM25Retrieve(ctx context.Context, tenantID, kbID, query string, opts *RetrieveOptions) (*RetrieveResponse, error)

	// HybridRetrieve 混合检索（向量+BM25）
	HybridRetrieve(ctx context.Context, tenantID, kbID, query string, opts *RetrieveOptions) (*RetrieveResponse, error)

	// GraphRetrieve 图谱检索
	GraphRetrieve(ctx context.Context, tenantID, kbID, query string, opts *RetrieveOptions) (*RetrieveResponse, error)
}

// ========================================
// Reranker 重排器接口
// ========================================

// RerankerStrategy 重排策略接口
type RerankerStrategy interface {
	// Rerank 执行重排
	Rerank(ctx context.Context, results []*Document, query string) ([]*Document, error)
}

// Reranker 重排器接口
type Reranker interface {
	// Rerank 重排检索结果
	Rerank(ctx context.Context, results []*Document, query string) ([]*Document, error)

	// SetStrategy 设置重排策略
	SetStrategy(strategy string) error

	// GetStrategy 获取当前策略
	GetStrategy() string
}

// ========================================
// Pipeline 管道接口
// ========================================

// Pipeline RAG 处理管道接口
type Pipeline interface {
	// Execute 执行 RAG 管道
	Execute(ctx context.Context, req *PipelineRequest) (*PipelineResponse, error)

	// ExecuteStream 流式执行 RAG 管道
	ExecuteStream(ctx context.Context, req *PipelineRequest) (<-chan *PipelineEvent, error)
}

// PipelineRequest 管道请求
type PipelineRequest struct {
	TenantID        string
	KnowledgeBaseID            string
	Query           string
	Options         *PipelineConfig
	Context         *RAGContext
	StreamEnabled   bool
}

// PipelineResponse 管道响应
type PipelineResponse struct {
	Answer          string
	Documents       []*Document
	Context         *RAGContext
	Metadata        map[string]interface{}
	ProcessingTime  int64
}

// PipelineEvent 管道事件（流式）
type PipelineEvent struct {
	Event       string // retrieve/rerank/generate/done/error
	Content     string
	Document    *Document
	Metadata    map[string]interface{}
	Error       error
}

// ========================================
// QueryStrengthener 查询增强器接口
// ========================================

// QueryStrengthener 查询增强器接口
type QueryStrengthener interface {
	// StrengthenQuery 增强查询
	StrengthenQuery(ctx context.Context, query string, conversationHistory string, opts *StrengthOptions) (*StrengthenedQuery, error)

	// RewriteQuery 重写查询
	RewriteQuery(ctx context.Context, query string, conversationHistory string, opts *StrengthOptions) (string, error)

	// SplitQuery 拆分查询
	SplitQuery(ctx context.Context, query string, conversationHistory string, opts *StrengthOptions) ([]string, error)
}

// StrengthOptions 增强选项
type StrengthOptions struct {
	EnableRewrite bool    // 是否启用查询重写
	EnableSplit   bool    // 是否启用查询拆分
	Temperature   float64 // LLM 温度参数
	MaxTokens     int     // 最大 token 数
}

// DefaultStrengthOptions 默认增强选项
func DefaultStrengthOptions() *StrengthOptions {
	return &StrengthOptions{
		EnableRewrite: true,
		EnableSplit:   true,
		Temperature:   0.1,
		MaxTokens:     2000,
	}
}

// ========================================
// GraphRepository 图谱仓储接口
// ========================================

// GraphRepository 图谱仓储接口
type GraphRepository interface {
	// AddGraph 添加图谱数据
	AddGraph(ctx context.Context, namespace NameSpace, graphs []*GraphData) error

	// DeleteGraph 删除图谱数据
	DeleteGraph(ctx context.Context, namespaces []NameSpace) error

	// GetGraph 获取完整图谱数据
	GetGraph(ctx context.Context, namespace NameSpace) (*GraphData, error)

	// SearchNode 搜索节点
	SearchNode(ctx context.Context, namespace NameSpace, nodes []string) (*GraphData, error)

	// SearchPath 搜索路径
	SearchPath(ctx context.Context, namespace NameSpace, startNode, endNode string, maxDepth int) ([]*GraphData, error)

	// CheckHealth 检查图谱服务健康状态
	CheckHealth(ctx context.Context) error

	// UpdateNode 更新节点属性
	UpdateNode(ctx context.Context, namespace NameSpace, node *GraphNode) error

	// AddRelation 添加单个关系
	AddRelation(ctx context.Context, namespace NameSpace, relation *GraphRelation) (*GraphRelation, error)

	// AddNode 添加单个节点
	AddNode(ctx context.Context, namespace NameSpace, node *GraphNode) error

	// UpdateRelation 更新关系属性
	UpdateRelation(ctx context.Context, namespace NameSpace, relation *GraphRelation) (*GraphRelation, error)

	// DeleteNode 删除单个节点
	DeleteNode(ctx context.Context, namespace NameSpace, nodeID string) error

	// DeleteRelation 删除单个关系
	DeleteRelation(ctx context.Context, namespace NameSpace, relationID string) error

	// DeleteByChunkID 删除与指定 chunk_id 相关的所有图谱数据
	DeleteByChunkID(ctx context.Context, namespace NameSpace, chunkID string) error

	// DeleteByKnowledgeID 删除与指定 knowledge_id 相关的所有图谱数据
	DeleteByKnowledgeID(ctx context.Context, namespace NameSpace, knowledgeID string) error
}

// GraphQueryRepository 图谱与知识库关联查询仓储接口
type GraphQueryRepository interface {
	// GetChunksByGraphNodes 根据图谱节点名称获取关联的分片
	GetChunksByGraphNodes(ctx context.Context, kbID string, nodeNames []string) ([]*Document, error)

	// GetKnowledgeByGraphNodes 根据图谱节点名称获取关联的知识条目
	GetKnowledgeByGraphNodes(ctx context.Context, kbID string, nodeNames []string) (interface{}, error)

	// GetGraphStats 获取图谱统计信息
	GetGraphStats(ctx context.Context, kbID string) (*GraphStatistics, error)
}

// NameSpace 命名空间
type NameSpace struct {
	TenantID string
	KnowledgeBaseID     string
}

// GraphStatistics 图谱统计信息
type GraphStatistics struct {
	NodeCount     int    // 节点数量
	RelationCount int    // 关系数量
	AvgDegree     float64 // 平均度数
}

// ========================================
// Embedder 向量化器接口
// ========================================

// Embedder 向量化器接口
type Embedder interface {
	// Embed 将文本转换为向量
	Embed(ctx context.Context, texts []string) ([][]float32, error)

	// EmbedQuery 将查询转换为向量
	EmbedQuery(ctx context.Context, query string) ([]float32, error)
}

// ========================================
// LLMChat LLM 聊天接口
// ========================================

// LLMChat LLM 聊天接口
type LLMChat interface {
	// Chat 聊天
	Chat(ctx context.Context, messages []LLMMessage, opts *ChatOptions) (*LLMResponse, error)

	// ChatStream 流式聊天
	ChatStream(ctx context.Context, messages []LLMMessage, opts *ChatOptions) (<-chan *LLMStreamEvent, error)
}

// LLMMessage LLM 消息
type LLMMessage struct {
	Role    string // system/user/assistant
	Content string
}

// ChatOptions 聊天选项
type ChatOptions struct {
	Temperature float64
	MaxTokens   int
	TopP        float64
}

// LLMResponse LLM 响应
type LLMResponse struct {
	Content    string
	MessageID  string
	TokenCount int
	FinishReason string
}

// LLMStreamEvent LLM 流式事件
type LLMStreamEvent struct {
	Content   string
	Done      bool
	Error     error
}

// ========================================
// VectorRepository 向量仓储接口
// ========================================

// VectorRepository 向量仓储接口
type VectorRepository interface {
	// CreateCollection 创建集合
	CreateCollection(ctx context.Context, kbID int64, dimension int, opts *CollectionOptions) error

	// DropCollection 删除集合
	DropCollection(ctx context.Context, kbID int64) error

	// HasCollection 检查集合是否存在
	HasCollection(ctx context.Context, kbID int64) (bool, error)

	// Insert 插入向量数据
	Insert(ctx context.Context, kbID int64, docs []*VectorDocument) error

	// Search 向量搜索
	Search(ctx context.Context, kbID int64, vector []float32, opts *SearchOptions) ([]*VectorDocument, error)

	// Delete 删除向量数据
	Delete(ctx context.Context, kbID int64, ids []int64) error

	// DeleteByChunkID 按 chunk_id 删除
	DeleteByChunkID(ctx context.Context, kbID int64, chunkID string) error

	// DeleteByKnowledgeID 按 knowledge_id 删除
	DeleteByKnowledgeID(ctx context.Context, kbID int64, knowledgeID string) error

	// CreateIndex 创建索引
	CreateIndex(ctx context.Context, kbID int64, fieldName string, indexType IndexType, metricType MetricType, params map[string]string) error

	// LoadCollection 加载集合到内存
	LoadCollection(ctx context.Context, kbID int64, async bool) error

	// ReleaseCollection 释放集合内存
	ReleaseCollection(ctx context.Context, kbID int64) error

	// GetStats 获取集合统计信息
	GetStats(ctx context.Context, kbID int64) (*CollectionStats, error)

	// CheckHealth 检查连接健康状态
	CheckHealth(ctx context.Context) error
}

// VectorDocument 向量文档
type VectorDocument struct {
	ID           int64                  // 主键ID
	DenseVector  []float32              // 稠密向量
	SparseVector SparseEmbedding        // 稀疏向量
	ChunkID      string                 // 分块ID
	KnowledgeID  string                 // 知识条目ID
	KnowledgeBaseID         string                 // 知识库ID
	TenantID     int64                  // 租户ID
	ChunkIndex   int                    // 分块索引
	Content      string                 // 分块内容
	IsEnabled    bool                   // 是否启用
	StartAt      int64                  // 起始位置
	EndAt        int64                  // 结束位置
	TokenCount   int64                  // Token数量
	Score        float32                // 相似度分数
	Metadata     map[string]interface{} // 元数据
}

// SparseEmbedding 稀疏向量
type SparseEmbedding map[uint32]float32

// CollectionOptions 集合选项
type CollectionOptions struct {
	Description  string // 描述
	AutoID       bool   // 是否自动生成ID
	EnableDynamic bool   // 是否启用动态字段
}

// SearchOptions 搜索选项
type SearchOptions struct {
	TopK          int     // 返回结果数量
	ScoreThreshold float32 // 相似度阈值
	Filter        string  // 过滤表达式
}

// IndexType 索引类型
type IndexType string

const (
	IndexTypeFlat           IndexType = "FLAT"
	IndexTypeIvfFlat        IndexType = "IVF_FLAT"
	IndexTypeIvfSq8         IndexType = "IVF_SQ8"
	IndexTypeIvfPq          IndexType = "IVF_PQ"
	IndexTypeHnsw           IndexType = "HNSW"
	IndexTypeDiskAnn        IndexType = "DISKANN"
	IndexTypeAutoIndex      IndexType = "AUTOINDEX"
	IndexTypeScalar         IndexType = "SCALAR"
	IndexTypeSparseInverted IndexType = "SPARSE_INVERTED"
)

// MetricType 距离度量类型
type MetricType string

const (
	MetricTypeL2     MetricType = "L2"
	MetricTypeIP     MetricType = "IP"
	MetricTypeCOSINE MetricType = "COSINE"
	MetricTypeBM25   MetricType = "BM25"
)

// CollectionStats 集合统计信息
type CollectionStats struct {
	Name       string              // 集合名称
	FieldCount int                 // 字段数量
	Statistics map[string]string   // 统计信息
}
