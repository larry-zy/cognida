// Package knowledge provides Knowledge service type definitions
package knowledge

// ========================================
// RAG 聊天请求/响应
// ========================================

// ChatRequest RAG 聊天请求
type ChatRequest struct {
	KnowledgeBaseID      string                `json:"knowledge_base_id"`
	Query                 string                `json:"query" binding:"required"`
	SessionID             string                `json:"session_id,omitempty"`
	ConversationHistory   []ConversationMessage  `json:"conversation_history,omitempty"`
	Options               *ChatOptions         `json:"options,omitempty"`
	Stream                bool                  `json:"stream,omitempty"`
}

// ChatOptions 聊天选项
type ChatOptions struct {
	// 检索配置
	TopK                int     `json:"top_k,omitempty"`
	SimilarityThreshold float64 `json:"similarity_threshold,omitempty"`
	RetrievalMode       string  `json:"retrieval_mode,omitempty"`
	EnableRerank        bool    `json:"enable_rerank,omitempty"`

	// LLM 配置
	Temperature float64 `json:"temperature,omitempty"`
	MaxTokens   int    `json:"max_tokens,omitempty"`

	// 图谱配置
	GraphEnabled bool `json:"graph_enabled,omitempty"`

	// 查询增强配置
	EnableQueryRewrite bool `json:"enable_query_rewrite,omitempty"`
	EnableQuerySplit   bool `json:"enable_query_split,omitempty"`
}

// ConversationMessage 对话消息
type ConversationMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatResponse RAG 聊天响应
type ChatResponse struct {
	Answer     string          `json:"answer"`
	Documents  []DocumentDTO   `json:"documents,omitempty"`
	SessionID  string          `json:"session_id,omitempty"`
	MessageID  string          `json:"message_id,omitempty"`
	Metadata   *ChatMetadata   `json:"metadata,omitempty"`
}

// DocumentDTO 文档 DTO
type DocumentDTO struct {
	ChunkID          string                 `json:"chunk_id"`
	KnowledgeID      string                 `json:"knowledge_id,omitempty"`
	KnowledgeBaseID  string                 `json:"knowledge_base_id,omitempty"`
	Content          string                 `json:"content"`
	Score            float64                `json:"score"`
	MatchType        string                 `json:"match_type,omitempty"`
	ChunkIndex       int                    `json:"chunk_index,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// ChatMetadata 聊天元数据
type ChatMetadata struct {
	ProcessingTime    int64               `json:"processing_time_ms"`
	RetrievalCount   int                 `json:"retrieval_count"`
	VectorCount      int                 `json:"vector_count"`
	BM25Count        int                 `json:"bm25_count"`
	GraphCount       int                 `json:"graph_count"`
	QueryRewritten   bool                `json:"query_rewritten,omitempty"`
	OriginalQuery    string              `json:"original_query,omitempty"`
	RewrittenQuery   string              `json:"rewritten_query,omitempty"`
	SubQueries       []string            `json:"sub_queries,omitempty"`
	RetrievalTrace   *RetrievalTraceDTO  `json:"retrieval_trace,omitempty"`
}

// RetrievalTraceDTO 检索追踪 DTO
type RetrievalTraceDTO struct {
	VectorLatency int64               `json:"vector_latency_ms,omitempty"`
	BM25Latency   int64               `json:"bm25_latency_ms,omitempty"`
	GraphLatency  int64               `json:"graph_latency_ms,omitempty"`
	RerankLatency int64               `json:"rerank_latency_ms,omitempty"`
	Steps         []RetrievalStepDTO  `json:"steps,omitempty"`
}

// RetrievalStepDTO 检索步骤 DTO
type RetrievalStepDTO struct {
	StepType    string                 `json:"step_type"`
	Description string                 `json:"description"`
	Latency     int64                  `json:"latency_ms"`
	ResultCount int                    `json:"result_count"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

// ========================================
// 流式事件
// ========================================

// StreamEvent 流式事件
type StreamEvent struct {
	Event    string                 `json:"event"` // retrieve/rerank/generate/chunk/done/error
	Content  string                 `json:"content,omitempty"`
	Document *DocumentDTO           `json:"document,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Error    string                 `json:"error,omitempty"`
	Done     bool                   `json:"done,omitempty"`
}

// ========================================
// 检索请求/响应
// ========================================

// RetrieveRequest 检索请求
type RetrieveRequest struct {
	KnowledgeBaseID                string   `json:"knowledge_base_id"`
	Query                          string   `json:"query" binding:"required"`
	TopK                           int      `json:"top_k,omitempty"`
	SimilarityThreshold            float64  `json:"similarity_threshold,omitempty"`
	RetrievalMode                  string   `json:"retrieval_mode,omitempty"`
	EnableRerank                   bool     `json:"enable_rerank,omitempty"`
}

// RetrieveResponse 检索响应
type RetrieveResponse struct {
	Documents   []DocumentDTO        `json:"documents"`
	Query       string                `json:"query"`
	TotalCount  int                  `json:"total_count"`
	HasMore     bool                 `json:"has_more"`
	Latency     int64                `json:"latency_ms"`
	SearchTrace *RetrievalTraceDTO   `json:"search_trace,omitempty"`
}

// ========================================
// 图谱请求/响应
// ========================================

// GraphExtractRequest 图谱提取请求
type GraphExtractRequest struct {
	KnowledgeBaseID string `json:"knowledge_base_id"`
	ChunkID         string `json:"chunk_id"`
	Document        string `json:"document"`
	Query           string `json:"query,omitempty"`
	Mode            string `json:"mode,omitempty"` // document/query
}

// GraphExtractResponse 图谱提取响应
type GraphExtractResponse struct {
	ChunkID   string            `json:"chunk_id"`
	Nodes     []GraphNodeDTO    `json:"nodes"`
	Relations []GraphRelationDTO `json:"relations"`
}


// ========================================
// 查询增强请求/响应
// ========================================

// StrengthenQueryRequest 查询增强请求
type StrengthenQueryRequest struct {
	Query               string `json:"query" binding:"required"`
	ConversationHistory string `json:"conversation_history,omitempty"`
	EnableRewrite       bool   `json:"enable_rewrite,omitempty"`
	EnableSplit         bool   `json:"enable_split,omitempty"`
}

// StrengthenQueryResponse 查询增强响应
type StrengthenQueryResponse struct {
	OriginalQuery      string   `json:"original_query"`
	RewrittenQuery     string   `json:"rewritten_query,omitempty"`
	SubQueries         []string `json:"sub_queries,omitempty"`
	RewriteApplied     bool     `json:"rewrite_applied"`
	SplitApplied       bool     `json:"split_applied"`
	ProcessingTime     int64    `json:"processing_time_ms"`
	QueriesForRetrieve []string `json:"queries_for_retrieve,omitempty"`
}

// ========================================
// 批量检索请求/响应
// ========================================

// MultiKBRetrieveRequest 多知识库检索请求
type MultiKBRetrieveRequest struct {
	KnowledgeBaseIDs    []string `json:"knowledge_base_ids" binding:"required"`
	Query               string   `json:"query" binding:"required"`
	TopK                int      `json:"top_k,omitempty"`
	SimilarityThreshold float64  `json:"similarity_threshold,omitempty"`
	RetrievalMode       string   `json:"retrieval_mode,omitempty"`
	EnableRerank        bool     `json:"enable_rerank,omitempty"`
}

// MultiKBRetrieveResponse 多知识库检索响应
type MultiKBRetrieveResponse struct {
	Results    []KBRetrieveResultDTO `json:"results"`
	Query      string                 `json:"query"`
	TotalCount int                    `json:"total_count"`
	Latency    int64                  `json:"latency_ms"`
}

// KBRetrieveResultDTO 知识库检索结果 DTO
type KBRetrieveResultDTO struct {
	KnowledgeBaseID string        `json:"knowledge_base_id"`
	Documents       []DocumentDTO  `json:"documents"`
	Count           int           `json:"count"`
}

// ========================================
// Knowledge Base 请求/响应
// ========================================

// CreateKnowledgeBaseRequest 创建知识库请求
type CreateKnowledgeBaseRequest struct {
	Name          string  `json:"name" binding:"required"`
	Description   string  `json:"description"`
	Avatar        string  `json:"avatar"`
	EmbodiedID    string  `json:"embodied_id"`
	Type          string  `json:"type"`
	IsPublic      bool    `json:"is_public"`
	ChunkSize     *int    `json:"chunk_size"`
	ChunkOverlap  *int    `json:"chunk_overlap"`
	GraphEnabled  *bool   `json:"graph_enabled"`
	BM25Enabled   *bool   `json:"bm25_enabled"`
	RetrievalMode *string `json:"retrieval_mode"`
	TopK          *int    `json:"top_k"`
	Alpha         *float64 `json:"alpha"`
}

// UpdateKnowledgeBaseRequest 更新知识库请求
type UpdateKnowledgeBaseRequest struct {
	Name        *string  `json:"name"`
	Description *string  `json:"description"`
	Avatar      *string  `json:"avatar"`
	IsPublic    *bool    `json:"is_public"`
	Status      *int8    `json:"status"`
	// GraphEnabled 库级图谱提取开关：非 nil 时更新到 kb_settings，允许建库后在设置页开关
	GraphEnabled *bool   `json:"graph_enabled"`
}

// KnowledgeBaseResponse 知识库响应
type KnowledgeBaseResponse struct {
	ID             string                    `json:"id"`
	Name           string                    `json:"name"`
	Description    string                    `json:"description"`
	Avatar         string                    `json:"avatar"`
	Type           string                    `json:"type"`
	TenantID       int64                     `json:"tenant_id"`
	UserID         int64                     `json:"user_id"`
	DocumentCount  int                       `json:"document_count"`
	ChunkCount     int                       `json:"chunk_count"`
	StorageSize    int64                     `json:"storage_size"`
	Status         int8                      `json:"status"`
	IsPublic       bool                      `json:"is_public"`
	CreatedAt      int64                     `json:"created_at"`
	UpdatedAt      int64                     `json:"updated_at"`
	Setting        *KnowledgeBaseSettingResponse `json:"setting,omitempty"`
}

// KnowledgeBaseSettingResponse 知识库设置响应
type KnowledgeBaseSettingResponse struct {
	ID             int64  `json:"id"`
	KnowledgeBaseID string `json:"kb_id"`
	GraphEnabled   bool   `json:"graph_enabled"`
	BM25Enabled    bool   `json:"bm25_enabled"`
	ChunkingConfig string `json:"chunking_config"`
	SettingsJSON   string `json:"settings_json"`
	UpdatedAt      int64  `json:"updated_at"`
}

// ========================================
// Document Processor 请求/响应
// ========================================

// ProcessDocumentRequest 文档处理请求
type ProcessDocumentRequest struct {
	KnowledgeBaseID string  `json:"kb_id" binding:"required"`
	KnowledgeID     string  `json:"knowledge_id"`
	FilePath        string  `json:"file_path"`
	FileName        string  `json:"file_name"`
	FileType        string  `json:"file_type"`
	Title           string  `json:"title"`
	DocumentID      string  `json:"document_id"`
	GraphEnabled    bool    `json:"graph_enabled"`
	URL             string  `json:"url"`
	Content         []byte  `json:"content"`
	ChunkSize       int     `json:"chunk_size"`
	ChunkOverlap    int     `json:"chunk_overlap"`
	ChunkStrategy   string  `json:"chunk_strategy"`
}

// ProcessDocumentResponse 文档处理响应
type ProcessDocumentResponse struct {
	DocumentID      string   `json:"document_id"`
	KnowledgeID     string   `json:"knowledge_id"`
	Status          string   `json:"status"`
	ParseStatus     string   `json:"parse_status"`
	ChunkCount      int      `json:"chunk_count"`
	ChunkIDs        []string `json:"chunk_ids"`
	ProcessTime     int64    `json:"process_time_ms"`
	StorageSize     int64    `json:"storage_size"`
	Vectorized      bool     `json:"vectorized"`
	GraphExtracted  bool     `json:"graph_extracted"`
	Message         string   `json:"message"`
}

// RebuildGraphResponse 知识库图谱补建结果
type RebuildGraphResponse struct {
	TotalDocuments     int `json:"total_documents"`     // 已完成解析的文档总数
	ProcessedDocuments int `json:"processed_documents"` // 成功重建图谱的文档数
	SkippedDocuments   int `json:"skipped_documents"`   // 无可用分块而跳过的文档数
	FailedDocuments    int `json:"failed_documents"`    // 重建失败的文档数
	TotalNodes         int `json:"total_nodes"`         // 提取的节点总数
	TotalRelations     int `json:"total_relations"`     // 提取的关系总数
}
