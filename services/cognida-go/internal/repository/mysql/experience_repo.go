// Package mysql 提供会话经验沉淀（experience）的 MySQL 持久化实现。
package mysql

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	domain_experience "cognida/internal/model/agent/experience"
)

// ========================================
// AgentExperienceModel 会话经验数据库模型
// ========================================

// AgentExperienceModel 对应 agent_experiences 表。
// 该表同时充当「会话已沉淀」的幂等标记：session_id 唯一，一条会话至多一条记录。
// Tools/Tags 为轻量字符串数组，落库为 JSON 列。
type AgentExperienceModel struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	SessionID string `gorm:"type:varchar(36);not null;uniqueIndex:uk_session_id"`
	TenantID  int64  `gorm:"not null;index:idx_exp_tenant"`
	UserID    int64  `gorm:"not null"`
	AgentType string `gorm:"type:varchar(50)"`

	Title             string `gorm:"type:varchar(512)"`
	Problem           string `gorm:"type:text"`
	Solution          string `gorm:"type:text"`
	Tools             string `gorm:"type:json"`                                   // JSON 数组：用到的工具名
	Tags              string `gorm:"type:json"`                                   // JSON 数组：主题标签
	Confidence        int    `gorm:"type:int;default:0;index:idx_exp_confidence"` // 蒸馏置信度 0~100
	SkillWorthy       bool   `gorm:"type:tinyint(1);default:0"`
	SkillInstructions string `gorm:"type:text"`

	Status    string `gorm:"type:varchar(20);default:'pending';index:idx_exp_status"`
	SkillName string `gorm:"type:varchar(255)"`
	Error     string `gorm:"type:text"`

	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

// TableName 指定表名
func (AgentExperienceModel) TableName() string {
	return "agent_experiences"
}

// ========================================
// experienceRepository 实现
// ========================================

type experienceRepository struct {
	db *gorm.DB
}

// NewExperienceRepository 创建会话经验仓储
func NewExperienceRepository(db *gorm.DB) domain_experience.ExperienceRepository {
	return &experienceRepository{db: db}
}

// FindIdleUnsummarizedSessions 挑出空闲已结束且未沉淀的会话。
// 用 NOT EXISTS 关联 agent_experiences 作幂等过滤，避免改动会话领域模型。
// createdAfter 非零时追加 created_at 下限，只处理该时刻之后新建的会话（历史存量不回溯）。
func (r *experienceRepository) FindIdleUnsummarizedSessions(
	ctx context.Context, idleBefore, createdAfter time.Time, minMessages, limit int,
) ([]*domain_experience.IdleSession, error) {
	if limit <= 0 {
		limit = 20
	}
	type row struct {
		ID           string
		TenantID     int64
		UserID       int64
		AgentType    string
		MessageCount int
	}
	q := r.db.WithContext(ctx).
		Table("sessions AS s").
		Select("s.id, s.tenant_id, s.user_id, s.agent_type, s.message_count").
		Where("s.status = ?", 1). // SessionStatusActive
		Where("s.deleted_at IS NULL").
		Where("s.updated_at < ?", idleBefore).
		Where("s.message_count >= ?", minMessages).
		Where("NOT EXISTS (SELECT 1 FROM agent_experiences e WHERE e.session_id = s.id)")
	if !createdAfter.IsZero() {
		q = q.Where("s.created_at >= ?", createdAfter)
	}
	var rows []row
	err := q.
		Order("s.updated_at ASC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("查询待沉淀会话失败: %w", err)
	}

	result := make([]*domain_experience.IdleSession, len(rows))
	for i, x := range rows {
		result[i] = &domain_experience.IdleSession{
			SessionID:    x.ID,
			TenantID:     x.TenantID,
			UserID:       x.UserID,
			AgentType:    x.AgentType,
			MessageCount: x.MessageCount,
		}
	}
	return result, nil
}

// Save 按 session_id 幂等 upsert 一条经验记录。
func (r *experienceRepository) Save(ctx context.Context, exp *domain_experience.Experience) error {
	model := &AgentExperienceModel{
		ID:                exp.ID,
		SessionID:         exp.SessionID,
		TenantID:          exp.TenantID,
		UserID:            exp.UserID,
		AgentType:         exp.AgentType,
		Title:             exp.Title,
		Problem:           exp.Problem,
		Solution:          exp.Solution,
		Tools:             marshalJSONArray(exp.Tools),
		Tags:              marshalJSONArray(exp.Tags),
		Confidence:        exp.Confidence,
		SkillWorthy:       exp.SkillWorthy,
		SkillInstructions: exp.SkillInstructions,
		Status:            exp.Status,
		SkillName:         exp.SkillName,
		Error:             exp.Error,
	}
	// 冲突（同一 session_id）时更新全部业务字段，实现 pending→done 的推进。
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "session_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"title", "problem", "solution", "tools", "tags", "confidence",
				"skill_worthy", "skill_instructions", "status", "skill_name", "error", "updated_at",
			}),
		}).
		Create(model).Error
	if err != nil {
		return fmt.Errorf("保存会话经验失败: %w", err)
	}
	// 回填自增 ID 与时间戳（图谱节点 ID / SKILL.md 落款依赖）。
	exp.ID = model.ID
	if !model.CreatedAt.IsZero() {
		exp.CreatedAt = model.CreatedAt
	}
	if !model.UpdatedAt.IsZero() {
		exp.UpdatedAt = model.UpdatedAt
	}
	return nil
}

// FindSkillWorthyByTenant 返回某租户下已生成 skill 的经验（供近重复合并判定）。
func (r *experienceRepository) FindSkillWorthyByTenant(
	ctx context.Context, tenantID int64, limit int,
) ([]*domain_experience.Experience, error) {
	if limit <= 0 {
		limit = 200
	}
	var models []AgentExperienceModel
	err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Where("status = ?", domain_experience.StatusDone).
		Where("skill_worthy = ?", true).
		Where("skill_name <> ''").
		Order("updated_at DESC").
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("查询已生成技能的经验失败: %w", err)
	}
	out := make([]*domain_experience.Experience, len(models))
	for i := range models {
		out[i] = models[i].toDomain()
	}
	return out, nil
}

// ExistsBySession 是否已存在该会话的沉淀记录。
func (r *experienceRepository) ExistsBySession(ctx context.Context, sessionID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&AgentExperienceModel{}).
		Where("session_id = ?", sessionID).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("检查会话经验存在性失败: %w", err)
	}
	return count > 0, nil
}

// toDomain 把持久化模型映射回领域经验（Tools/Tags 由 JSON 列解析回切片）。
func (m *AgentExperienceModel) toDomain() *domain_experience.Experience {
	return &domain_experience.Experience{
		ID:                m.ID,
		SessionID:         m.SessionID,
		TenantID:          m.TenantID,
		UserID:            m.UserID,
		AgentType:         m.AgentType,
		Title:             m.Title,
		Problem:           m.Problem,
		Solution:          m.Solution,
		Tools:             unmarshalJSONArray(m.Tools),
		Tags:              unmarshalJSONArray(m.Tags),
		Confidence:        m.Confidence,
		SkillWorthy:       m.SkillWorthy,
		SkillInstructions: m.SkillInstructions,
		Status:            m.Status,
		SkillName:         m.SkillName,
		Error:             m.Error,
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}
}

// marshalJSONArray 把字符串数组序列化为 JSON 文本（nil/空 → "[]"），供 JSON 列存储。
func marshalJSONArray(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	b, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// unmarshalJSONArray 把 JSON 列文本解析回字符串数组（空/非法 → nil）。
func unmarshalJSONArray(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" || s == "[]" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}
