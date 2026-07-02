// Package mysql 提供数据库持久化模型
// 这些模型仅用于基础设施层，包含 GORM 标签
package mysql

import (
	"encoding/json"
	"time"

	"link/internal/model/agent"
	domain_conversation "link/internal/model/conversation"
	domain_knowledge "link/internal/model/knowledge"
	domain_tenant "link/internal/model/tenant"
	domain_user "link/internal/model/user"
)

// ========================================
// KnowledgeBase 知识库模型
// ========================================

// KnowledgeBaseModel 知识库数据库模型
type KnowledgeBaseModel struct {
	ID            string     `gorm:"primaryKey;type:varchar(36)"`
	TenantID      int64      `gorm:"not null;index:idx_tenant_id"`
	UserID        int64      `gorm:"not null;index:idx_user_id"`
	Name          string     `gorm:"type:varchar(255);not null"`
	Description   string     `gorm:"type:text"`
	Avatar        string     `gorm:"type:varchar(500)"`
	Status        int8       `gorm:"type:tinyint;default:1;index:idx_status"`
	IsPublic      bool       `gorm:"default:false"`
	DocumentCount int        `gorm:"default:0"`
	ChunkCount    int        `gorm:"default:0"`
	StorageSize   int64      `gorm:"default:0"`
	CreatedAt     time.Time  `gorm:"autoCreateTime"`
	UpdatedAt     time.Time  `gorm:"autoUpdateTime"`
	DeletedAt     *time.Time `gorm:"index"`
}

// TableName 指定表名
func (KnowledgeBaseModel) TableName() string {
	return "knowledge_bases"
}

// ToDomain 转换为领域实体
func (m *KnowledgeBaseModel) ToDomain() *domain_knowledge.KnowledgeBase {
	return &domain_knowledge.KnowledgeBase{
		ID:            m.ID,
		TenantID:      m.TenantID,
		UserID:        m.UserID,
		Name:          m.Name,
		Description:   m.Description,
		Avatar:        m.Avatar,
		Status:        m.Status,
		IsPublic:      m.IsPublic,
		DocumentCount: m.DocumentCount,
		ChunkCount:    m.ChunkCount,
		StorageSize:   m.StorageSize,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
		DeletedAt:     m.DeletedAt,
	}
}

// FromDomain 从领域实体转换
func (m *KnowledgeBaseModel) FromDomain(entity *domain_knowledge.KnowledgeBase) *KnowledgeBaseModel {
	m.ID = entity.ID
	m.TenantID = entity.TenantID
	m.UserID = entity.UserID
	m.Name = entity.Name
	m.Description = entity.Description
	m.Avatar = entity.Avatar
	m.Status = entity.Status
	m.IsPublic = entity.IsPublic
	m.DocumentCount = entity.DocumentCount
	m.ChunkCount = entity.ChunkCount
	m.StorageSize = entity.StorageSize
	m.CreatedAt = entity.CreatedAt
	m.UpdatedAt = entity.UpdatedAt
	m.DeletedAt = entity.DeletedAt
	return m
}

// ========================================
// Knowledge 知识条目模型
// ========================================

// KnowledgeModel 知识条目数据库模型
type KnowledgeModel struct {
	ID           string     `gorm:"primaryKey;type:varchar(36)"`
	TenantID     int64      `gorm:"not null;index:idx_tenant_kb,priority:1"`
	TagID        *int64     `gorm:"default:NULL"`
	KnowledgeBaseID   string     `gorm:"not null;type:varchar(36);index:idx_tenant_kb,priority:2;index:idx_knowledge_base_id"`
	UserID       int64      `gorm:"not null;index:idx_user_id"`
	Type         string     `gorm:"type:varchar(50);not null"`
	Title        string     `gorm:"type:varchar(255);not null"`
	Description  string     `gorm:"type:text"`
	Source       string     `gorm:"type:varchar(128);not null"`
	ParseStatus  string     `gorm:"type:varchar(50);default:'unprocessed';index:idx_status"`
	EnableStatus string     `gorm:"type:varchar(50);default:'enabled';index:idx_status"`
	FilePath     string     `gorm:"type:text"`
	StorageSize  int64      `gorm:"default:0"`
	CreatedAt    time.Time  `gorm:"autoCreateTime"`
	UpdatedAt    time.Time  `gorm:"autoUpdateTime"`
	DeletedAt    *time.Time `gorm:"index"`
	ProcessedAt  *time.Time
	ChunkCount   *int `gorm:"default:NULL"`
}

// TableName 指定表名
func (KnowledgeModel) TableName() string {
	return "knowledge"
}

// ToDomain 转换为领域实体
func (m *KnowledgeModel) ToDomain() *domain_knowledge.Knowledge {
	return &domain_knowledge.Knowledge{
		ID:           m.ID,
		TenantID:     m.TenantID,
		TagID:        m.TagID,
		KnowledgeBaseID:         m.KnowledgeBaseID,
		UserID:       m.UserID,
		Type:         m.Type,
		Title:        m.Title,
		Description:  m.Description,
		Source:       m.Source,
		ParseStatus:  m.ParseStatus,
		EnableStatus: m.EnableStatus,
		FilePath:     m.FilePath,
		StorageSize:  m.StorageSize,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
		DeletedAt:    m.DeletedAt,
		ProcessedAt:  m.ProcessedAt,
		ChunkCount:   m.ChunkCount,
	}
}

// FromDomain 从领域实体转换（用于更新，会复制所有字段包括时间戳）
func (m *KnowledgeModel) FromDomain(entity *domain_knowledge.Knowledge) *KnowledgeModel {
	m.ID = entity.ID
	m.TenantID = entity.TenantID
	m.TagID = entity.TagID
	m.KnowledgeBaseID = entity.KnowledgeBaseID
	m.UserID = entity.UserID
	m.Type = entity.Type
	m.Title = entity.Title
	m.Description = entity.Description
	m.Source = entity.Source
	m.ParseStatus = entity.ParseStatus
	m.EnableStatus = entity.EnableStatus
	m.FilePath = entity.FilePath
	m.StorageSize = entity.StorageSize
	m.CreatedAt = entity.CreatedAt
	m.UpdatedAt = entity.UpdatedAt
	m.DeletedAt = entity.DeletedAt
	m.ProcessedAt = entity.ProcessedAt
	m.ChunkCount = entity.ChunkCount
	return m
}

// FromDomainForCreate 从领域实体转换（用于创建，跳过零时间戳让 GORM 自动设置）
func (m *KnowledgeModel) FromDomainForCreate(entity *domain_knowledge.Knowledge) *KnowledgeModel {
	m.ID = entity.ID
	m.TenantID = entity.TenantID
	m.TagID = entity.TagID
	m.KnowledgeBaseID = entity.KnowledgeBaseID
	m.UserID = entity.UserID
	m.Type = entity.Type
	m.Title = entity.Title
	m.Description = entity.Description
	m.Source = entity.Source
	m.ParseStatus = entity.ParseStatus
	m.EnableStatus = entity.EnableStatus
	m.FilePath = entity.FilePath
	m.StorageSize = entity.StorageSize
	// 只复制非零时间戳
	if !entity.CreatedAt.IsZero() {
		m.CreatedAt = entity.CreatedAt
	}
	if !entity.UpdatedAt.IsZero() {
		m.UpdatedAt = entity.UpdatedAt
	}
	if entity.DeletedAt != nil {
		m.DeletedAt = entity.DeletedAt
	}
	if entity.ProcessedAt != nil {
		m.ProcessedAt = entity.ProcessedAt
	}
	m.ChunkCount = entity.ChunkCount
	return m
}

// ========================================
// Chunk 文档分块模型
// ========================================

// ChunkModel 文档分块数据库模型
type ChunkModel struct {
	ID                     string     `gorm:"primaryKey;type:varchar(36)"`
	TenantID               int64      `gorm:"not null;index:idx_tenant_kb,priority:1"`
	TagID                  *int64     `gorm:"default:NULL"`
	KnowledgeBaseID   string     `gorm:"not null;type:varchar(36);index:idx_tenant_kb,priority:2;index:idx_knowledge_base_id"`
	KnowledgeID            string     `gorm:"not null;type:varchar(36);index:idx_knowledge_id"`
	Content                string     `gorm:"type:text;not null"`
	ChunkIndex             int        `gorm:"not null"`
	IsEnabled              bool       `gorm:"type:tinyint(1);not null;default:1"`
	StartAt                int        `gorm:"not null;default:0"`
	EndAt                  int        `gorm:"not null;default:0"`
	PreChunkID             *string    `gorm:"type:varchar(36)"`
	NextChunkID            *string    `gorm:"type:varchar(36)"`
	ChunkType              string     `gorm:"type:varchar(20);not null;default:'text';index:idx_chunk_type"`
	ParentChunkID          *string    `gorm:"type:varchar(36)"`
	ImageInfo              *string    `gorm:"type:text"`
	RelationChunks         *string    `gorm:"type:json"`
	IndirectRelationChunks *string    `gorm:"type:json"`
	EmbeddingID            *string    `gorm:"type:varchar(100);default:NULL;index:idx_embedding_id"`
	TokenCount             *int       `gorm:"default:NULL"`
	Metadata               *string    `gorm:"type:json"`
	CreatedAt              time.Time  `gorm:"autoCreateTime"`
	UpdatedAt              time.Time  `gorm:"autoUpdateTime"`
	DeletedAt              *time.Time `gorm:"index"`
}

// TableName 指定表名
func (ChunkModel) TableName() string {
	return "chunks"
}

// ToDomain 转换为领域实体
func (m *ChunkModel) ToDomain() *domain_knowledge.Chunk {
	return &domain_knowledge.Chunk{
		ID:                     m.ID,
		TenantID:               m.TenantID,
		TagID:                  m.TagID,
		KnowledgeBaseID:                   m.KnowledgeBaseID,
		KnowledgeID:            m.KnowledgeID,
		Content:                m.Content,
		ChunkIndex:             m.ChunkIndex,
		IsEnabled:              m.IsEnabled,
		StartAt:                m.StartAt,
		EndAt:                  m.EndAt,
		PreChunkID:             m.PreChunkID,
		NextChunkID:            m.NextChunkID,
		ChunkType:              m.ChunkType,
		ParentChunkID:          m.ParentChunkID,
		ImageInfo:              m.ImageInfo,
		RelationChunks:         m.RelationChunks,
		IndirectRelationChunks: m.IndirectRelationChunks,
		EmbeddingID:            m.EmbeddingID,
		TokenCount:             m.TokenCount,
		Metadata:               m.Metadata,
		CreatedAt:              m.CreatedAt,
		UpdatedAt:              m.UpdatedAt,
		DeletedAt:              m.DeletedAt,
	}
}

// FromDomain 从领域实体转换（用于更新，会复制所有字段包括时间戳）
func (m *ChunkModel) FromDomain(entity *domain_knowledge.Chunk) *ChunkModel {
	m.ID = entity.ID
	m.TenantID = entity.TenantID
	m.TagID = entity.TagID
	m.KnowledgeBaseID = entity.KnowledgeBaseID
	m.KnowledgeID = entity.KnowledgeID
	m.Content = entity.Content
	m.ChunkIndex = entity.ChunkIndex
	m.IsEnabled = entity.IsEnabled
	m.StartAt = entity.StartAt
	m.EndAt = entity.EndAt
	m.PreChunkID = entity.PreChunkID
	m.NextChunkID = entity.NextChunkID
	m.ChunkType = entity.ChunkType
	m.ParentChunkID = entity.ParentChunkID
	m.ImageInfo = entity.ImageInfo
	m.RelationChunks = entity.RelationChunks
	m.IndirectRelationChunks = entity.IndirectRelationChunks
	m.EmbeddingID = entity.EmbeddingID
	m.TokenCount = entity.TokenCount
	m.Metadata = entity.Metadata
	m.CreatedAt = entity.CreatedAt
	m.UpdatedAt = entity.UpdatedAt
	m.DeletedAt = entity.DeletedAt
	return m
}

// FromDomainForCreate 从领域实体转换（用于创建，跳过零时间戳让 GORM 自动设置）
func (m *ChunkModel) FromDomainForCreate(entity *domain_knowledge.Chunk) *ChunkModel {
	m.ID = entity.ID
	m.TenantID = entity.TenantID
	m.TagID = entity.TagID
	m.KnowledgeBaseID = entity.KnowledgeBaseID
	m.KnowledgeID = entity.KnowledgeID
	m.Content = entity.Content
	m.ChunkIndex = entity.ChunkIndex
	m.IsEnabled = entity.IsEnabled
	m.StartAt = entity.StartAt
	m.EndAt = entity.EndAt
	m.PreChunkID = entity.PreChunkID
	m.NextChunkID = entity.NextChunkID
	m.ChunkType = entity.ChunkType
	m.ParentChunkID = entity.ParentChunkID
	m.ImageInfo = entity.ImageInfo
	m.RelationChunks = entity.RelationChunks
	m.IndirectRelationChunks = entity.IndirectRelationChunks
	m.EmbeddingID = entity.EmbeddingID
	m.TokenCount = entity.TokenCount
	m.Metadata = entity.Metadata
	// 只复制非零时间戳
	if !entity.CreatedAt.IsZero() {
		m.CreatedAt = entity.CreatedAt
	}
	if !entity.UpdatedAt.IsZero() {
		m.UpdatedAt = entity.UpdatedAt
	}
	if entity.DeletedAt != nil {
		m.DeletedAt = entity.DeletedAt
	}
	return m
}

// ========================================
// KnowledgeBaseSetting 知识库设置模型
// ========================================

// KnowledgeBaseSettingModel 知识库设置数据库模型
type KnowledgeBaseSettingModel struct {
	ID                int64      `gorm:"primaryKey;autoIncrement"`
	KnowledgeBaseID   string     `gorm:"not null;type:varchar(36);uniqueIndex:uk_knowledge_base_id;column:knowledge_base_id"`
	GraphEnabled      bool       `gorm:"type:tinyint(1);default:0"`
	BM25Enabled       *bool      `gorm:"type:tinyint(1)"`
	ChunkingConfig    *string    `gorm:"type:json"`
	SettingsJSON      *string    `gorm:"type:json"`
	CreatedAt         time.Time  `gorm:"autoCreateTime"`
	UpdatedAt         time.Time  `gorm:"autoUpdateTime"`
}

// TableName 指定表名
func (KnowledgeBaseSettingModel) TableName() string {
	return "knowledge_base_settings"
}

// ToDomain 转换为领域实体
func (m *KnowledgeBaseSettingModel) ToDomain() *domain_knowledge.KnowledgeBaseSetting {
	return &domain_knowledge.KnowledgeBaseSetting{
		ID:              m.ID,
		KnowledgeBaseID: m.KnowledgeBaseID,
		GraphEnabled:    m.GraphEnabled,
		BM25Enabled:     m.BM25Enabled,
		ChunkingConfig:  m.ChunkingConfig,
		SettingsJSON:    m.SettingsJSON,
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
	}
}

// FromDomain 从领域实体转换
func (m *KnowledgeBaseSettingModel) FromDomain(entity *domain_knowledge.KnowledgeBaseSetting) *KnowledgeBaseSettingModel {
	m.ID = entity.ID
	m.KnowledgeBaseID = entity.KnowledgeBaseID
	m.GraphEnabled = entity.GraphEnabled
	m.BM25Enabled = entity.BM25Enabled
	m.ChunkingConfig = entity.ChunkingConfig
	m.SettingsJSON = entity.SettingsJSON
	m.CreatedAt = entity.CreatedAt
	m.UpdatedAt = entity.UpdatedAt
	return m
}

// ========================================
// Tag 知识标签模型
// ========================================

// TagModel 知识标签数据库模型
type TagModel struct {
	ID              int64      `gorm:"primaryKey;autoIncrement"`
	TenantID        string     `gorm:"not null;type:varchar(36);index:idx_tenant_kb,priority:1"`
	KnowledgeBaseID int64      `gorm:"not null;index:idx_tenant_kb,priority:2"`
	Name            string     `gorm:"type:varchar(255);not null;index:idx_name"`
	Color           *string    `gorm:"type:varchar(7)"`
	SortOrder       int        `gorm:"default:0"`
	CreatedAt       time.Time  `gorm:"autoCreateTime"`
	UpdatedAt       time.Time  `gorm:"autoUpdateTime"`
	DeletedAt       *time.Time `gorm:"index"`
}

// TableName 指定表名
func (TagModel) TableName() string {
	return "knowledge_tags"
}

// ToDomain 转换为领域实体
func (m *TagModel) ToDomain() *domain_knowledge.Tag {
	return &domain_knowledge.Tag{
		ID:              m.ID,
		TenantID:        m.TenantID,
		KnowledgeBaseID: m.KnowledgeBaseID,
		Name:            m.Name,
		Color:           m.Color,
		SortOrder:       m.SortOrder,
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
		DeletedAt:       m.DeletedAt,
	}
}

// FromDomain 从领域实体转换
func (m *TagModel) FromDomain(entity *domain_knowledge.Tag) *TagModel {
	m.ID = entity.ID
	m.TenantID = entity.TenantID
	m.KnowledgeBaseID = entity.KnowledgeBaseID
	m.Name = entity.Name
	m.Color = entity.Color
	m.SortOrder = entity.SortOrder
	m.CreatedAt = entity.CreatedAt
	m.UpdatedAt = entity.UpdatedAt
	m.DeletedAt = entity.DeletedAt
	return m
}

// ========================================
// RetrievalSetting 检索设置模型
// ========================================

// RetrievalSettingModel 检索设置数据库模型
type RetrievalSettingModel struct {
	ID        int64      `gorm:"primaryKey;autoIncrement"`
	TenantID  int64      `gorm:"not null;index:idx_tenant_id"`
	SessionID *string    `gorm:"type:varchar(36);index:idx_session_id"`
	// 向量检索配置
	VectorTopK      *int     `gorm:"default:5"`
	VectorThreshold *float64 `gorm:"default:0.7"`
	VectorModelID   *string  `gorm:"type:varchar(64)"`
	// BM25检索配置
	BM25Enable *bool `gorm:"type:tinyint(1)"`
	BM25TopK   *int  `gorm:"default:5"`
	// 图谱检索配置
	GraphEnabled     *bool    `gorm:"type:tinyint(1);default:0"`
	GraphTopK        *int     `gorm:"default:5"`
	GraphMinStrength *float64 `gorm:"default:1"`
	// 混合检索配置
	HybridAlpha         *domain_knowledge.Number `gorm:"default:0.5"`
	HybridRerankEnabled *bool             `gorm:"type:tinyint(1);default:0"`
	// 网络搜索配置
	WebEnabled     *bool `gorm:"type:tinyint(1);default:0"`
	WebSearchDepth *int  `gorm:"default:1"`
	// 重排序配置
	RerankEnabled *bool   `gorm:"type:tinyint(1);default:0"`
	AdvancedConfig *string `gorm:"type:json"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime"`
}

// TableName 指定表名
func (RetrievalSettingModel) TableName() string {
	return "retrieval_settings"
}

// ToDomain 转换为领域实体
func (m *RetrievalSettingModel) ToDomain() *domain_knowledge.RetrievalSetting {
	return &domain_knowledge.RetrievalSetting{
		ID:                  m.ID,
		TenantID:            m.TenantID,
		SessionID:           m.SessionID,
		VectorTopK:          m.VectorTopK,
		VectorThreshold:     m.VectorThreshold,
		VectorModelID:       m.VectorModelID,
		BM25Enable:          m.BM25Enable,
		BM25TopK:            m.BM25TopK,
		GraphEnabled:        m.GraphEnabled,
		GraphTopK:           m.GraphTopK,
		GraphMinStrength:    m.GraphMinStrength,
		HybridAlpha:         m.HybridAlpha,
		HybridRerankEnabled: m.HybridRerankEnabled,
		WebEnabled:          m.WebEnabled,
		WebSearchDepth:      m.WebSearchDepth,
		RerankEnabled:       m.RerankEnabled,
		AdvancedConfig:      m.AdvancedConfig,
		CreatedAt:           m.CreatedAt,
		UpdatedAt:           m.UpdatedAt,
	}
}

// FromDomain 从领域实体转换
func (m *RetrievalSettingModel) FromDomain(entity *domain_knowledge.RetrievalSetting) *RetrievalSettingModel {
	m.ID = entity.ID
	m.TenantID = entity.TenantID
	m.SessionID = entity.SessionID
	m.VectorTopK = entity.VectorTopK
	m.VectorThreshold = entity.VectorThreshold
	m.VectorModelID = entity.VectorModelID
	m.BM25Enable = entity.BM25Enable
	m.BM25TopK = entity.BM25TopK
	m.GraphEnabled = entity.GraphEnabled
	m.GraphTopK = entity.GraphTopK
	m.GraphMinStrength = entity.GraphMinStrength
	m.HybridAlpha = entity.HybridAlpha
	m.HybridRerankEnabled = entity.HybridRerankEnabled
	m.WebEnabled = entity.WebEnabled
	m.WebSearchDepth = entity.WebSearchDepth
	m.RerankEnabled = entity.RerankEnabled
	m.AdvancedConfig = entity.AdvancedConfig
	m.CreatedAt = entity.CreatedAt
	m.UpdatedAt = entity.UpdatedAt
	return m
}

// ========================================
// Session 会话模型
// ========================================

// SessionModel 会话数据库模型
type SessionModel struct {
	ID           string     `gorm:"primaryKey;type:varchar(36)"`
	TenantID     int64      `gorm:"not null;index:idx_tenant_id"`
	UserID       int64      `gorm:"not null;index:idx_user_id"`
	AgentType    string     `gorm:"type:varchar(50);default:'default'"`
	Title        string     `gorm:"type:varchar(255);not null"`
	Description  string     `gorm:"type:text"`
	Status       int8       `gorm:"type:tinyint;default:1;index:idx_status"`
	MessageCount int        `gorm:"default:0"`
	CreatedAt    time.Time  `gorm:"autoCreateTime"`
	UpdatedAt    time.Time  `gorm:"autoUpdateTime"`
	DeletedAt    *time.Time `gorm:"index"`
}

// TableName 指定表名
func (SessionModel) TableName() string {
	return "sessions"
}

// ToDomain 转换为领域实体
func (m *SessionModel) ToDomain() *domain_conversation.Session {
	return &domain_conversation.Session{
		ID:           m.ID,
		TenantID:     m.TenantID,
		UserID:       m.UserID,
		AgentType:    m.AgentType,
		Title:        m.Title,
		Description:  m.Description,
		Status:       m.Status,
		MessageCount: m.MessageCount,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
		DeletedAt:    m.DeletedAt,
	}
}

// FromDomain 从领域实体转换
func (m *SessionModel) FromDomain(entity *domain_conversation.Session) *SessionModel {
	m.ID = entity.ID
	m.TenantID = entity.TenantID
	m.UserID = entity.UserID
	m.AgentType = entity.AgentType
	m.Title = entity.Title
	m.Description = entity.Description
	m.Status = entity.Status
	m.MessageCount = entity.MessageCount
	m.CreatedAt = entity.CreatedAt
	m.UpdatedAt = entity.UpdatedAt
	m.DeletedAt = entity.DeletedAt
	return m
}

// ========================================
// Message 消息模型
// ========================================

// MessageModel 消息数据库模型
type MessageModel struct {
	ID                  string     `gorm:"primaryKey;type:varchar(36)"`
	RequestID           string     `gorm:"type:varchar(36);index:idx_request_id"`
	SessionID           string     `gorm:"not null;type:varchar(36);index:idx_session_id"`
	Role                string     `gorm:"type:varchar(50);not null"`
	Content             string     `gorm:"type:text;not null"`
	KnowledgeReferences string     `gorm:"type:json"`
	AgentSteps          string     `gorm:"type:json"`
	ToolCalls           string     `gorm:"type:json"`
	IsCompleted         bool       `gorm:"type:tinyint(1);default:0"`
	TokenCount          int        `gorm:"default:0"`
	CreatedAt           time.Time  `gorm:"autoCreateTime"`
	UpdatedAt           time.Time  `gorm:"autoUpdateTime"`
	DeletedAt           *time.Time `gorm:"index"`
}

// TableName 指定表名
func (MessageModel) TableName() string {
	return "messages"
}

// ToDomain 转换为领域实体
func (m *MessageModel) ToDomain() *domain_conversation.Message {
	return &domain_conversation.Message{
		ID:                  m.ID,
		RequestID:           m.RequestID,
		SessionID:           m.SessionID,
		Role:                m.Role,
		Content:             m.Content,
		KnowledgeReferences: m.KnowledgeReferences,
		AgentSteps:          m.AgentSteps,
		ToolCalls:           m.ToolCalls,
		IsCompleted:         m.IsCompleted,
		TokenCount:          m.TokenCount,
		CreatedAt:           m.CreatedAt,
		UpdatedAt:           m.UpdatedAt,
		DeletedAt:           m.DeletedAt,
	}
}

// FromDomain 从领域实体转换
func (m *MessageModel) FromDomain(entity *domain_conversation.Message) *MessageModel {
	m.ID = entity.ID
	m.RequestID = entity.RequestID
	m.SessionID = entity.SessionID
	m.Role = entity.Role
	m.Content = entity.Content

	// JSON 类型列需要处理空字符串：空字符串 -> "null" 或 "[]"
	// knowledge_references: 空时设为 null
	if entity.KnowledgeReferences == "" {
		m.KnowledgeReferences = "null"
	} else {
		m.KnowledgeReferences = entity.KnowledgeReferences
	}

	// agent_steps: 空时设为 "[]"
	if entity.AgentSteps == "" {
		m.AgentSteps = "[]"
	} else {
		m.AgentSteps = entity.AgentSteps
	}

	// tool_calls: 空时设为 "[]"
	if entity.ToolCalls == "" {
		m.ToolCalls = "[]"
	} else {
		m.ToolCalls = entity.ToolCalls
	}

	m.IsCompleted = entity.IsCompleted
	m.TokenCount = entity.TokenCount
	m.CreatedAt = entity.CreatedAt
	m.UpdatedAt = entity.UpdatedAt
	m.DeletedAt = entity.DeletedAt
	return m
}

// ========================================
// User 用户模型
// ========================================

// UserModel 用户数据库模型
type UserModel struct {
	ID           int64      `gorm:"primaryKey;autoIncrement"`
	TenantID     int64      `gorm:"not null;uniqueIndex:uk_tenant_username,priority:1;uniqueIndex:uk_tenant_email,priority:1"`
	Username     string     `gorm:"type:varchar(50);not null;uniqueIndex:uk_tenant_username,priority:2"`
	Email        string     `gorm:"type:varchar(100);not null;uniqueIndex:uk_tenant_email,priority:2"`
	PasswordHash string     `gorm:"type:varchar(255);not null"`
	Avatar       string     `gorm:"type:varchar(500)"`
	Status       int8       `gorm:"type:tinyint;default:1;index:idx_status"`
	CreatedAt    time.Time  `gorm:"autoCreateTime"`
	UpdatedAt    time.Time  `gorm:"autoUpdateTime"`
	LastLoginAt  *time.Time `gorm:"index"`
	DeletedAt    *time.Time `gorm:"index"`
}

// TableName 指定表名
func (UserModel) TableName() string {
	return "users"
}

// ToDomain 转换为领域实体
func (m *UserModel) ToDomain() *domain_user.User {
	return &domain_user.User{
		ID:           m.ID,
		TenantID:     m.TenantID,
		Username:     m.Username,
		Email:        m.Email,
		PasswordHash: m.PasswordHash,
		Avatar:       m.Avatar,
		Status:       m.Status,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
		LastLoginAt:  m.LastLoginAt,
	}
}

// FromDomain 从领域实体转换
func (m *UserModel) FromDomain(entity *domain_user.User) *UserModel {
	m.ID = entity.ID
	m.TenantID = entity.TenantID
	m.Username = entity.Username
	m.Email = entity.Email
	m.PasswordHash = entity.PasswordHash
	m.Avatar = entity.Avatar
	m.Status = entity.Status
	m.CreatedAt = entity.CreatedAt
	m.UpdatedAt = entity.UpdatedAt
	m.LastLoginAt = entity.LastLoginAt
	return m
}

// ========================================
// RefreshToken 刷新令牌模型
// ========================================

// RefreshTokenModel 刷新令牌数据库模型
type RefreshTokenModel struct {
	ID        int64     `gorm:"primaryKey;autoIncrement"`
	UserID    int64     `gorm:"not null;index"`
	TokenHash string    `gorm:"type:varchar(64);not null;uniqueIndex"`
	ExpiresAt time.Time `gorm:"not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

// TableName 指定表名
func (RefreshTokenModel) TableName() string {
	return "refresh_tokens"
}

// ToDomain 转换为领域实体
func (m *RefreshTokenModel) ToDomain() *domain_user.RefreshToken {
	return &domain_user.RefreshToken{
		ID:        m.ID,
		UserID:    m.UserID,
		TokenHash: m.TokenHash,
		ExpiresAt: m.ExpiresAt,
		CreatedAt: m.CreatedAt,
	}
}

// FromDomain 从领域实体转换
func (m *RefreshTokenModel) FromDomain(entity *domain_user.RefreshToken) *RefreshTokenModel {
	m.ID = entity.ID
	m.UserID = entity.UserID
	m.TokenHash = entity.TokenHash
	m.ExpiresAt = entity.ExpiresAt
	m.CreatedAt = entity.CreatedAt
	return m
}

// ========================================
// Tenant 租户模型
// ========================================

// TenantModel 租户数据库模型
type TenantModel struct {
	ID               int64      `gorm:"primaryKey;autoIncrement"`
	Name             string     `gorm:"type:varchar(255);not null"`
	Description      string     `gorm:"type:text"`
	APIKey           string     `gorm:"type:varchar(64);not null;uniqueIndex"`
	RetrieverEngines *string    `gorm:"type:json"`
	Status           string     `gorm:"type:varchar(50);default:'active'"`
	Business         string     `gorm:"type:varchar(255);not null"`
	StorageQuota     int64      `gorm:"not null;default:10737418240"`
	StorageUsed      int64      `gorm:"not null;default:0"`
	AgentConfig      *string    `gorm:"type:json"`
	Settings         *string    `gorm:"type:json"`
	CreatedAt        time.Time  `gorm:"autoCreateTime"`
	UpdatedAt        time.Time  `gorm:"autoUpdateTime"`
	DeletedAt        *time.Time `gorm:"index"`
}

// TableName 指定表名
func (TenantModel) TableName() string {
	return "tenants"
}

// ToDomain 转换为领域实体
func (m *TenantModel) ToDomain() *domain_tenant.Tenant {
	return &domain_tenant.Tenant{
		ID:               m.ID,
		Name:             m.Name,
		Description:      m.Description,
		APIKey:           m.APIKey,
		RetrieverEngines: m.RetrieverEngines,
		Status:           m.Status,
		Business:         m.Business,
		StorageQuota:     m.StorageQuota,
		StorageUsed:      m.StorageUsed,
		AgentConfig:      m.AgentConfig,
		Settings:         m.Settings,
		CreatedAt:        m.CreatedAt,
		UpdatedAt:        m.UpdatedAt,
		DeletedAt:        m.DeletedAt,
	}
}

// FromDomain 从领域实体转换（用于更新，会复制所有字段包括时间戳）
func (m *TenantModel) FromDomain(entity *domain_tenant.Tenant) *TenantModel {
	m.ID = entity.ID
	m.Name = entity.Name
	m.Description = entity.Description
	m.APIKey = entity.APIKey
	m.RetrieverEngines = entity.RetrieverEngines
	m.Status = entity.Status
	m.Business = entity.Business
	m.StorageQuota = entity.StorageQuota
	m.StorageUsed = entity.StorageUsed
	m.AgentConfig = entity.AgentConfig
	m.Settings = entity.Settings
	m.CreatedAt = entity.CreatedAt
	m.UpdatedAt = entity.UpdatedAt
	m.DeletedAt = entity.DeletedAt
	return m
}

// FromDomainForCreate 从领域实体转换（用于创建，跳过零时间戳让 GORM 自动设置）
func (m *TenantModel) FromDomainForCreate(entity *domain_tenant.Tenant) *TenantModel {
	m.ID = entity.ID
	m.Name = entity.Name
	m.Description = entity.Description
	m.APIKey = entity.APIKey
	m.RetrieverEngines = entity.RetrieverEngines
	m.Status = entity.Status
	m.Business = entity.Business
	m.StorageQuota = entity.StorageQuota
	m.StorageUsed = entity.StorageUsed
	m.AgentConfig = entity.AgentConfig
	m.Settings = entity.Settings
	// 只复制非零时间戳
	if !entity.CreatedAt.IsZero() {
		m.CreatedAt = entity.CreatedAt
	}
	if !entity.UpdatedAt.IsZero() {
		m.UpdatedAt = entity.UpdatedAt
	}
	if entity.DeletedAt != nil {
		m.DeletedAt = entity.DeletedAt
	}
	return m
}

// ========================================
// TenantUser 租户用户关联模型
// ========================================

// TenantUserModel 租户用户关联数据库模型
type TenantUserModel struct {
	ID        int64     `gorm:"primaryKey;autoIncrement"`
	TenantID  int64     `gorm:"not null;index:idx_tenant_user;uniqueIndex:uk_tenant_user"`
	UserID    int64     `gorm:"not null;index:idx_user_id;uniqueIndex:uk_tenant_user"`
	Role      string    `gorm:"type:varchar(50);not null;default:'member';index:idx_role"`
	Status    string    `gorm:"type:varchar(50);default:'active'"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

// TableName 指定表名
func (TenantUserModel) TableName() string {
	return "tenant_users"
}

// ToDomain 转换为领域实体
func (m *TenantUserModel) ToDomain() *domain_tenant.TenantUser {
	return &domain_tenant.TenantUser{
		ID:        m.ID,
		TenantID:  m.TenantID,
		UserID:    m.UserID,
		Role:      m.Role,
		Status:    m.Status,
		CreatedAt: m.CreatedAt,
	}
}

// FromDomain 从领域实体转换
func (m *TenantUserModel) FromDomain(entity *domain_tenant.TenantUser) *TenantUserModel {
	m.ID = entity.ID
	m.TenantID = entity.TenantID
	m.UserID = entity.UserID
	m.Role = entity.Role
	m.Status = entity.Status
	m.CreatedAt = entity.CreatedAt
	return m
}

// ========================================
// GraphNode 图谱节点模型
// ========================================

// GraphNodeModel 图谱节点数据库模型
type GraphNodeModel struct {
	ID         string    `gorm:"primaryKey;type:varchar(36)"`
	Name       string    `gorm:"type:varchar(255);not null;index:idx_node_name"`
	EntityType string    `gorm:"type:varchar(100);not null;index:idx_entity_type"`
	Attributes string    `gorm:"type:json"`
	Chunks     string    `gorm:"type:json"`
	Properties string    `gorm:"type:json"`
	TenantID   string    `gorm:"type:varchar(36);index:idx_tenant_kb"`
	KnowledgeBaseID       string    `gorm:"type:varchar(36);index:idx_tenant_kb"`
	CreatedAt  time.Time `gorm:"autoCreateTime"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime"`
}

// TableName 指定表名
func (GraphNodeModel) TableName() string {
	return "graph_nodes"
}

// ToDomain 转换为领域实体
func (m *GraphNodeModel) ToDomain() *domain_knowledge.GraphNode {
	return &domain_knowledge.GraphNode{
		ID:         m.ID,
		Name:       m.Name,
		EntityType: m.EntityType,
		Attributes: parseStringArray(m.Attributes),
		Chunks:     parseStringArray(m.Chunks),
		Properties: parseJSONMap(m.Properties),
	}
}

// FromDomain 从领域实体转换
func (m *GraphNodeModel) FromDomain(entity *domain_knowledge.GraphNode) *GraphNodeModel {
	m.ID = entity.ID
	m.Name = entity.Name
	m.EntityType = entity.EntityType
	m.Attributes = stringifyStringArray(entity.Attributes)
	m.Chunks = stringifyStringArray(entity.Chunks)
	m.Properties = stringifyJSONMap(entity.Properties)
	return m
}

// ========================================
// GraphRelation 图谱关系模型
// ========================================

// GraphRelationModel 图谱关系数据库模型
type GraphRelationModel struct {
	ID             string    `gorm:"primaryKey;type:varchar(36)"`
	Source         string    `gorm:"type:varchar(255);not null;index:idx_source"`
	Target         string    `gorm:"type:varchar(255);not null;index:idx_target"`
	Type           string    `gorm:"type:varchar(100);not null"`
	Strength       float64   `gorm:"type:double;default:0"`
	Weight         float64   `gorm:"type:double;default:0"`
	ChunkIDs       string    `gorm:"type:json"`
	Properties     string    `gorm:"type:json"`
	CombinedDegree int       `gorm:"default:0"`
	PMI            float64   `gorm:"type:double;default:0"`
	TenantID       string    `gorm:"type:varchar(36);index:idx_tenant_kb"`
	KnowledgeBaseID           string    `gorm:"type:varchar(36);index:idx_tenant_kb"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime"`
}

// TableName 指定表名
func (GraphRelationModel) TableName() string {
	return "graph_relations"
}

// ToDomain 转换为领域实体
func (m *GraphRelationModel) ToDomain() *domain_knowledge.GraphRelation {
	return &domain_knowledge.GraphRelation{
		ID:             m.ID,
		Source:         m.Source,
		Target:         m.Target,
		Type:           m.Type,
		Strength:       m.Strength,
		Weight:         m.Weight,
		ChunkIDs:       parseStringArray(m.ChunkIDs),
		Properties:     parseJSONMap(m.Properties),
		CombinedDegree: m.CombinedDegree,
		PMI:            m.PMI,
	}
}

// FromDomain 从领域实体转换
func (m *GraphRelationModel) FromDomain(entity *domain_knowledge.GraphRelation) *GraphRelationModel {
	m.ID = entity.ID
	m.Source = entity.Source
	m.Target = entity.Target
	m.Type = entity.Type
	m.Strength = entity.Strength
	m.Weight = entity.Weight
	m.ChunkIDs = stringifyStringArray(entity.ChunkIDs)
	m.Properties = stringifyJSONMap(entity.Properties)
	m.CombinedDegree = entity.CombinedDegree
	m.PMI = entity.PMI
	return m
}

// ========================================
// Agent相关模型
// ========================================

// AgentModel Agent数据库模型
type AgentModel struct {
	ID          string    `gorm:"primaryKey;type:varchar(36)"`
	Name        string    `gorm:"type:varchar(255);not null"`
	Description string    `gorm:"type:text"`
	Type        string    `gorm:"type:varchar(50);not null"`
	Status      string    `gorm:"type:varchar(50);default:'idle'"`
	Config      string    `gorm:"type:json"`
	TenantID    int64     `gorm:"not null;index:idx_tenant_id"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}

// TableName 指定表名
func (AgentModel) TableName() string {
	return "agents"
}

// ToDomain 转换为领域实体
func (m *AgentModel) ToDomain() *agent.Agent {
	return &agent.Agent{
		ID:          m.ID,
		Name:        m.Name,
		Description: m.Description,
		Type:        agent.AgentType(m.Type),
		Status:      agent.AgentStatus(m.Status),
		Config:      parseAgentConfig(m.Config),
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

// FromDomain 从领域实体转换
func (m *AgentModel) FromDomain(entity *agent.Agent) *AgentModel {
	m.ID = entity.ID
	m.Name = entity.Name
	m.Description = entity.Description
	m.Type = string(entity.Type)
	m.Status = string(entity.Status)
	m.Config = stringifyAgentConfig(entity.Config)
	m.CreatedAt = entity.CreatedAt
	m.UpdatedAt = entity.UpdatedAt
	return m
}

// ========================================
// 辅助函数
// ========================================

// parseStringArray 解析 JSON 字符串数组
func parseStringArray(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return nil
	}
	return result
}

// stringifyStringArray 将字符串数组转换为 JSON 字符串
func stringifyStringArray(arr []string) string {
	if arr == nil {
		return ""
	}
	data, err := json.Marshal(arr)
	if err != nil {
		return ""
	}
	return string(data)
}

// parseJSONMap 解析 JSON 对象
func parseJSONMap(s string) map[string]string {
	if s == "" {
		return nil
	}
	var result map[string]string
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return nil
	}
	return result
}

// stringifyJSONMap 将对象转换为 JSON 字符串
func stringifyJSONMap(m map[string]string) string {
	if m == nil {
		return ""
	}
	data, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(data)
}

// parseAgentConfig 解析 Agent 配置
func parseAgentConfig(s string) *agent.AgentConfig {
	if s == "" {
		return nil
	}
	var result agent.AgentConfig
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return nil
	}
	return &result
}

// stringifyAgentConfig 将 Agent 配置转换为 JSON 字符串
func stringifyAgentConfig(cfg *agent.AgentConfig) string {
	if cfg == nil {
		return ""
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return ""
	}
	return string(data)
}

