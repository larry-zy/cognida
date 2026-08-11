// Package knowledge provides the knowledge management domain entity definitions
package knowledge

import "time"

// ========================================
// KnowledgeBase 知识库实体
// ========================================

// KnowledgeBase knowledge base entity
// Corresponds to knowledge_bases table
type KnowledgeBase struct {
	ID            string
	TenantID      int64
	UserID        int64
	Name          string
	Description   string
	Avatar        string
	Status        int8
	IsPublic      bool
	DocumentCount int
	ChunkCount    int
	StorageSize   int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     *time.Time
	// Associated setting (non-persistent field)
	Setting *KnowledgeBaseSetting
}

// ========================================
// KnowledgeBase Business Behavior
// ========================================

// IsActive checks if the knowledge base is active
func (kb *KnowledgeBase) IsActive() bool {
	return kb.Status == 1
}

// Activate activates the knowledge base
func (kb *KnowledgeBase) Activate() {
	kb.Status = 1
}

// Deactivate deactivates the knowledge base
func (kb *KnowledgeBase) Deactivate() {
	kb.Status = 0
}

// CanBeDeleted checks if the knowledge base can be deleted
func (kb *KnowledgeBase) CanBeDeleted() bool {
	return kb.DocumentCount == 0 && kb.ChunkCount == 0
}

// HasDocuments returns true if the knowledge base has documents
func (kb *KnowledgeBase) HasDocuments() bool {
	return kb.DocumentCount > 0
}

// HasChunks returns true if the knowledge base has chunks
func (kb *KnowledgeBase) HasChunks() bool {
	return kb.ChunkCount > 0
}

// GetStorageSizeMB returns storage size in MB
func (kb *KnowledgeBase) GetStorageSizeMB() float64 {
	return float64(kb.StorageSize) / (1024 * 1024)
}

// IsOwnedBy checks if the knowledge base is owned by the given user
func (kb *KnowledgeBase) IsOwnedBy(userID int64) bool {
	return kb.UserID == userID
}

// BelongsToTenant checks if the knowledge base belongs to the given tenant
func (kb *KnowledgeBase) BelongsToTenant(tenantID int64) bool {
	return kb.TenantID == tenantID
}

// UpdateStats updates the statistics
func (kb *KnowledgeBase) UpdateStats(documentCount, chunkCount int, storageSize int64) {
	kb.DocumentCount = documentCount
	kb.ChunkCount = chunkCount
	kb.StorageSize = storageSize
}

// IncrementDocumentCount increments the document count
func (kb *KnowledgeBase) IncrementDocumentCount() {
	kb.DocumentCount++
}

// DecrementDocumentCount decrements the document count
func (kb *KnowledgeBase) DecrementDocumentCount() {
	if kb.DocumentCount > 0 {
		kb.DocumentCount--
	}
}

// AddChunks adds chunks to the knowledge base
func (kb *KnowledgeBase) AddChunks(count int, size int64) {
	kb.ChunkCount += count
	kb.StorageSize += size
}

// RemoveChunks removes chunks from the knowledge base
func (kb *KnowledgeBase) RemoveChunks(count int, size int64) {
	kb.ChunkCount -= count
	if kb.ChunkCount < 0 {
		kb.ChunkCount = 0
	}
	kb.StorageSize -= size
	if kb.StorageSize < 0 {
		kb.StorageSize = 0
	}
}

// ========================================
// Knowledge 知识条目实体
// ========================================

// Knowledge knowledge entry entity
// Corresponds to knowledges table
type Knowledge struct {
	ID                string
	TenantID          int64
	TagID             *int64
	KnowledgeBaseID   string
	UserID            int64
	Type         string
	Title        string
	Description  string
	Source       string
	ParseStatus  string
	EnableStatus string
	FilePath     string
	FileHash     string
	StorageSize  int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
	ProcessedAt  *time.Time
	ChunkCount   *int
}

// ========================================
// Knowledge Business Behavior
// ========================================

// Parse status constants
const (
	ParseStatusPending    = "pending"
	ParseStatusProcessing = "processing"
	ParseStatusCompleted  = "completed"
	ParseStatusFailed     = "failed"
)

// Enable status constants
const (
	EnableStatusEnabled  = "enabled"
	EnableStatusDisabled = "disabled"
)

// IsPending checks if the knowledge entry is pending parsing
func (k *Knowledge) IsPending() bool {
	return k.ParseStatus == ParseStatusPending
}

// IsProcessing checks if the knowledge entry is being processed
func (k *Knowledge) IsProcessing() bool {
	return k.ParseStatus == ParseStatusProcessing
}

// IsCompleted checks if the knowledge entry parsing is completed
func (k *Knowledge) IsCompleted() bool {
	return k.ParseStatus == ParseStatusCompleted
}

// IsFailed checks if the knowledge entry parsing failed
func (k *Knowledge) IsFailed() bool {
	return k.ParseStatus == ParseStatusFailed
}

// IsEnabled checks if the knowledge entry is enabled
func (k *Knowledge) IsEnabled() bool {
	return k.EnableStatus == EnableStatusEnabled
}

// MarkAsPending marks the knowledge entry as pending
func (k *Knowledge) MarkAsPending() {
	k.ParseStatus = ParseStatusPending
}

// MarkAsProcessing marks the knowledge entry as processing
func (k *Knowledge) MarkAsProcessing() {
	k.ParseStatus = ParseStatusProcessing
}

// MarkAsCompleted marks the knowledge entry as completed
func (k *Knowledge) MarkAsCompleted() {
	k.ParseStatus = ParseStatusCompleted
	now := time.Now()
	k.ProcessedAt = &now
}

// MarkAsFailed marks the knowledge entry as failed
func (k *Knowledge) MarkAsFailed(errorMessage string) {
	k.ParseStatus = ParseStatusFailed
}

// Enable enables the knowledge entry
func (k *Knowledge) Enable() {
	k.EnableStatus = EnableStatusEnabled
}

// Disable disables the knowledge entry
func (k *Knowledge) Disable() {
	k.EnableStatus = EnableStatusDisabled
}

// SetChunkCount sets the chunk count
func (k *Knowledge) SetChunkCount(count int) {
	k.ChunkCount = &count
}

// GetChunkCount returns the chunk count
func (k *Knowledge) GetChunkCount() int {
	if k.ChunkCount == nil {
		return 0
	}
	return *k.ChunkCount
}

// HasChunks returns true if the knowledge entry has chunks
func (k *Knowledge) HasChunks() bool {
	return k.GetChunkCount() > 0
}

// IsOwnedBy checks if the knowledge entry is owned by the given user
func (k *Knowledge) IsOwnedBy(userID int64) bool {
	return k.UserID == userID
}

// BelongsToKnowledgeBase checks if the knowledge entry belongs to the given KB
func (k *Knowledge) BelongsToKnowledgeBase(knowledgeBaseID string) bool {
	return k.KnowledgeBaseID == knowledgeBaseID
}

// BelongsToTenant checks if the knowledge entry belongs to the given tenant
func (k *Knowledge) BelongsToTenant(tenantID int64) bool {
	return k.TenantID == tenantID
}

// ========================================
// Chunk 文档分块实体
// ========================================

// Chunk document chunk entity
// Corresponds to chunks table
type Chunk struct {
	ID                string
	TenantID          int64
	TagID             *int64
	KnowledgeBaseID   string
	KnowledgeID       string
	Content                string
	ChunkIndex             int
	IsEnabled              bool
	StartAt                int
	EndAt                  int
	PreChunkID             *string
	NextChunkID            *string
	ChunkType              string
	ParentChunkID          *string
	ImageInfo              *string
	RelationChunks         *string
	IndirectRelationChunks *string
	EmbeddingID            *string
	TokenCount             *int
	Metadata               *string
	CreatedAt              time.Time
	UpdatedAt              time.Time
	DeletedAt              *time.Time
}

// ========================================
// Chunk Business Behavior
// ========================================

// Chunk type constants
const (
	ChunkTypeText     = "text"
	ChunkTypeImage    = "image"
	ChunkTypeTable    = "table"
	ChunkTypeMixed    = "mixed"
	ChunkTypeCode     = "code"
	ChunkTypeFormula  = "formula"
)

// IsEnabled checks if the chunk is enabled
func (c *Chunk) IsEnabledCheck() bool {
	return c.IsEnabled
}

// Enable enables the chunk
func (c *Chunk) Enable() {
	c.IsEnabled = true
}

// Disable disables the chunk
func (c *Chunk) Disable() {
	c.IsEnabled = false
}

// IsTextChunk checks if this is a text chunk
func (c *Chunk) IsTextChunk() bool {
	return c.ChunkType == ChunkTypeText
}

// IsImageChunk checks if this is an image chunk
func (c *Chunk) IsImageChunk() bool {
	return c.ChunkType == ChunkTypeImage
}

// GetContentLength returns the content length
func (c *Chunk) GetContentLength() int {
	return len(c.Content)
}

// HasEmbedding checks if the chunk has an embedding
func (c *Chunk) HasEmbedding() bool {
	return c.EmbeddingID != nil && *c.EmbeddingID != ""
}

// GetTokenCount returns the token count
func (c *Chunk) GetTokenCount() int {
	if c.TokenCount == nil {
		return 0
	}
	return *c.TokenCount
}

// BelongsToKnowledgeBase checks if the chunk belongs to the given KB
func (c *Chunk) BelongsToKnowledgeBase(knowledgeBaseID string) bool {
	return c.KnowledgeBaseID == knowledgeBaseID
}

// BelongsToKnowledge checks if the chunk belongs to the given knowledge
func (c *Chunk) BelongsToKnowledge(knowledgeID string) bool {
	return c.KnowledgeID == knowledgeID
}

// GetPosition returns the position (start, end) of the chunk
func (c *Chunk) GetPosition() (int, int) {
	return c.StartAt, c.EndAt
}

// SetPosition sets the position of the chunk
func (c *Chunk) SetPosition(start, end int) {
	c.StartAt = start
	c.EndAt = end
}

// HasPreviousChunk checks if there's a previous chunk
func (c *Chunk) HasPreviousChunk() bool {
	return c.PreChunkID != nil && *c.PreChunkID != ""
}

// HasNextChunk checks if there's a next chunk
func (c *Chunk) HasNextChunk() bool {
	return c.NextChunkID != nil && *c.NextChunkID != ""
}

// HasParentChunk checks if there's a parent chunk
func (c *Chunk) HasParentChunk() bool {
	return c.ParentChunkID != nil && *c.ParentChunkID != ""
}

// ========================================
// KnowledgeBaseSetting 知识库设置实体
// ========================================

// KnowledgeBaseSetting knowledge base setting entity
// Corresponds to knowledge_base_settings table
type KnowledgeBaseSetting struct {
	ID                int64
	KnowledgeBaseID   string
	GraphEnabled      bool
	BM25Enabled       *bool
	ChunkingConfig    *string
	SettingsJSON      *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ========================================
// KnowledgeBaseSetting Business Behavior
// ========================================

// IsGraphEnabled checks if graph is enabled
func (s *KnowledgeBaseSetting) IsGraphEnabled() bool {
	return s.GraphEnabled
}

// EnableGraph enables graph
func (s *KnowledgeBaseSetting) EnableGraph() {
	s.GraphEnabled = true
}

// DisableGraph disables graph
func (s *KnowledgeBaseSetting) DisableGraph() {
	s.GraphEnabled = false
}

// IsBM25Enabled checks if BM25 is enabled
func (s *KnowledgeBaseSetting) IsBM25Enabled() bool {
	if s.BM25Enabled == nil {
		return false
	}
	return *s.BM25Enabled
}

// SetBM25Enabled sets BM25 enabled status
func (s *KnowledgeBaseSetting) SetBM25Enabled(enabled bool) {
	s.BM25Enabled = &enabled
}

// GetChunkingConfig returns the chunking config
func (s *KnowledgeBaseSetting) GetChunkingConfig() string {
	if s.ChunkingConfig == nil {
		return "{}"
	}
	return *s.ChunkingConfig
}

// SetChunkingConfig sets the chunking config
func (s *KnowledgeBaseSetting) SetChunkingConfig(config string) {
	s.ChunkingConfig = &config
}

// ========================================
// Factory Functions
// ========================================

// NewKnowledgeBase creates a new knowledge base
func NewKnowledgeBase(id string, tenantID, userID int64, name string) (*KnowledgeBase, error) {
	if name == "" {
		return nil, ErrInvalidName
	}
	if tenantID <= 0 {
		return nil, ErrInvalidTenantID
	}
	if userID <= 0 {
		return nil, ErrInvalidUserID
	}

	now := time.Now()
	return &KnowledgeBase{
		ID:            id,
		TenantID:      tenantID,
		UserID:        userID,
		Name:          name,
		Status:        1,
		DocumentCount: 0,
		ChunkCount:    0,
		StorageSize:   0,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

// NewKnowledge creates a new knowledge entry
func NewKnowledge(id string, tenantID, userID int64, knowledgeBaseID string, knowledgeType string, title string) (*Knowledge, error) {
	if title == "" {
		return nil, ErrInvalidTitle
	}
	if knowledgeBaseID == "" {
		return nil, ErrInvalidKnowledgeBaseID
	}
	if tenantID <= 0 {
		return nil, ErrInvalidTenantID
	}

	now := time.Now()
	return &Knowledge{
		ID:                id,
		TenantID:          tenantID,
		UserID:            userID,
		KnowledgeBaseID:   knowledgeBaseID,
		Type:              knowledgeType,
		Title:             title,
		ParseStatus:       ParseStatusPending,
		// 新建文档默认启用；停用是显式管理动作（见 SetKnowledgeEnabled）。
		EnableStatus:      EnableStatusEnabled,
		StorageSize:       0,
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

// NewChunk creates a new chunk
func NewChunk(id string, tenantID int64, knowledgeBaseID, knowledgeID string, content string, index int) (*Chunk, error) {
	if content == "" {
		return nil, ErrInvalidContent
	}
	if knowledgeBaseID == "" {
		return nil, ErrInvalidKnowledgeBaseID
	}
	if knowledgeID == "" {
		return nil, ErrInvalidKnowledgeID
	}

	now := time.Now()
	return &Chunk{
		ID:                id,
		TenantID:          tenantID,
		KnowledgeBaseID:   knowledgeBaseID,
		KnowledgeID:       knowledgeID,
		Content:           content,
		ChunkIndex:        index,
		IsEnabled:         true,
		ChunkType:         ChunkTypeText,
		StartAt:           0,
		EndAt:             len(content),
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

// ========================================
// Tag 知识标签实体
// ========================================

// Tag knowledge tag entity
// Corresponds to knowledge_tags table
type Tag struct {
	ID              int64
	TenantID        string
	KnowledgeBaseID int64
	Name            string
	Color           *string
	SortOrder       int
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
}

// ========================================
// RetrievalSetting 检索设置实体
// ========================================

// RetrievalSetting retrieval setting entity
// Corresponds to retrieval_settings table
type RetrievalSetting struct {
	ID        int64
	TenantID  int64
	SessionID *string
	// Vector retrieval config
	VectorTopK      *int
	VectorThreshold *float64
	VectorModelID   *string
	// BM25 retrieval config
	BM25Enable *bool
	BM25TopK   *int
	// Graph retrieval config
	GraphEnabled     *bool
	GraphTopK        *int
	GraphMinStrength *float64
	// Hybrid retrieval config
	HybridAlpha         *Number
	HybridRerankEnabled *bool
	// Web search config
	WebEnabled     *bool
	WebSearchDepth *int
	// Rerank config
	RerankEnabled *bool
	// Advanced config
	AdvancedConfig *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Number represents numeric type (compatible with float64 and int)
type Number float64

// ========================================
// Query Parameters
// ========================================

// KnowledgeListQuery knowledge entry query parameters
type KnowledgeListQuery struct {
	KnowledgeBaseID   string
	Type              string
	ParseStatus       string
	EnableStatus      string
	Page              int
	PageSize          int
}

// ChunkListQuery chunk query parameters
type ChunkListQuery struct {
	KnowledgeBaseID   string
	KnowledgeID       string
	IsEnabled   *bool
	Page        int
	PageSize    int
}

// TagListQuery tag query parameters
type TagListQuery struct {
	Name     string
	Page     int
	PageSize int
}

// GraphStats graph statistics
type GraphStats struct {
	NodeCount     int64
	RelationCount int64
	ChunkCount    int64
	ChunkIDs      []string
}

// KnowledgeBaseStats knowledge base statistics
type KnowledgeBaseStats struct {
	KnowledgeBaseID   string
	KnowledgeCount    int64
	ChunkCount        int64
	TotalSize         int64
}

// ========================================
// NameSpace 命名空间
// ========================================

// NameSpace namespace for knowledge and graph
type NameSpace struct {
	TenantID         string
	KnowledgeBaseID   string
	Knowledge         string
	Type              string
}

// String returns namespace string
func (n NameSpace) String() string {
	return n.TenantID + ":" + n.KnowledgeBaseID
}
