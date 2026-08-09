// Package mysql 提供聊天领域的 MySQL 持久化实现
package mysql

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	domain_conversation "cognida/internal/model/conversation"
)

// ========================================
// SessionRepository 会话仓储实现
// ========================================

// sessionRepository 会话仓储实现
type sessionRepository struct {
	db *gorm.DB
}

// NewSessionRepository 创建会话仓储
func NewSessionRepository(db *gorm.DB) domain_conversation.SessionRepository {
	return &sessionRepository{db: db}
}

// Create 创建会话
func (r *sessionRepository) Create(ctx context.Context, session *domain_conversation.Session) error {
	var model SessionModel
	model.FromDomain(session)
	return r.db.WithContext(ctx).Create(&model).Error
}

// FindByID 根据ID查找会话
func (r *sessionRepository) FindByID(ctx context.Context, id string) (*domain_conversation.Session, error) {
	var model SessionModel
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&model).Error

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("会话不存在")
	}
	if err != nil {
		return nil, fmt.Errorf("查询会话失败: %w", err)
	}

	return model.ToDomain(), nil
}

// FindByUserID 根据用户ID查找会话列表
func (r *sessionRepository) FindByUserID(ctx context.Context, userID int64, req *domain_conversation.ListSessionsRequest) ([]*domain_conversation.Session, int64, error) {
	var models []*SessionModel
	var total int64

	db := r.db.WithContext(ctx).Model(&SessionModel{}).Where("user_id = ?", userID)

	// 状态过滤
	if req.Status != nil {
		db = db.Where("status = ?", *req.Status)
	}

	// 统计总数
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计会话数量失败: %w", err)
	}

	// 分页查询
	offset := (req.Page - 1) * req.Size
	err := db.Order("created_at DESC").
		Limit(req.Size).
		Offset(offset).
		Find(&models).Error

	if err != nil {
		return nil, 0, fmt.Errorf("查询会话列表失败: %w", err)
	}

	result := make([]*domain_conversation.Session, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}

	return result, total, nil
}

// FindByTenantID 根据租户ID查找会话列表
func (r *sessionRepository) FindByTenantID(ctx context.Context, tenantID int64, req *domain_conversation.ListSessionsRequest) ([]*domain_conversation.Session, int64, error) {
	var models []*SessionModel
	var total int64

	db := r.db.WithContext(ctx).Model(&SessionModel{}).Where("tenant_id = ?", tenantID)

	// 状态过滤
	if req.Status != nil {
		db = db.Where("status = ?", *req.Status)
	}

	// 统计总数
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计会话数量失败: %w", err)
	}

	// 分页查询
	offset := (req.Page - 1) * req.Size
	err := db.Order("created_at DESC").
		Limit(req.Size).
		Offset(offset).
		Find(&models).Error

	if err != nil {
		return nil, 0, fmt.Errorf("查询会话列表失败: %w", err)
	}

	result := make([]*domain_conversation.Session, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}

	return result, total, nil
}

// Update 更新会话
func (r *sessionRepository) Update(ctx context.Context, session *domain_conversation.Session) error {
	var model SessionModel
	model.FromDomain(session)
	return r.db.WithContext(ctx).Save(&model).Error
}

// UpdateStatus 更新会话状态
func (r *sessionRepository) UpdateStatus(ctx context.Context, id string, status int8) error {
	return r.db.WithContext(ctx).Model(&SessionModel{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// UpdateMessageCount 更新消息数量
func (r *sessionRepository) UpdateMessageCount(ctx context.Context, id string, count int) error {
	return r.db.WithContext(ctx).Model(&SessionModel{}).
		Where("id = ?", id).
		Update("message_count", count).Error
}

// Delete 删除会话（软删除）
func (r *sessionRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&SessionModel{}).Error
}

// DeleteByUserID 删除用户的所有会话
func (r *sessionRepository) DeleteByUserID(ctx context.Context, userID int64) error {
	return r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&SessionModel{}).Error
}

// ArchiveSession 归档会话
func (r *sessionRepository) ArchiveSession(ctx context.Context, id string) error {
	return r.UpdateStatus(ctx, id, 0)
}

// ActivateSession 激活会话
func (r *sessionRepository) ActivateSession(ctx context.Context, id string) error {
	return r.UpdateStatus(ctx, id, 1)
}

// Exists 检查会话是否存在
func (r *sessionRepository) Exists(ctx context.Context, id string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&SessionModel{}).
		Where("id = ?", id).
		Count(&count).Error

	return count > 0, err
}

// CountByUserID 统计用户的会话数量
func (r *sessionRepository) CountByUserID(ctx context.Context, userID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&SessionModel{}).
		Where("user_id = ?", userID).
		Count(&count).Error

	return count, err
}

// ========================================
// MessageRepository 消息仓储实现
// ========================================

// messageRepository 消息仓储实现
type messageRepository struct {
	db *gorm.DB
}

// NewMessageRepository 创建消息仓储
func NewMessageRepository(db *gorm.DB) domain_conversation.MessageRepository {
	return &messageRepository{db: db}
}

// Create 创建消息
func (r *messageRepository) Create(ctx context.Context, message *domain_conversation.Message) error {
	var model MessageModel
	model.FromDomain(message)
	return r.db.WithContext(ctx).Create(&model).Error
}

// CreateBatch 批量创建消息
func (r *messageRepository) CreateBatch(ctx context.Context, messages []*domain_conversation.Message) error {
	if len(messages) == 0 {
		return nil
	}
	models := make([]*MessageModel, len(messages))
	for i, m := range messages {
		var model MessageModel
		model.FromDomain(m)
		models[i] = &model
	}
	return r.db.WithContext(ctx).Create(&models).Error
}

// FindByID 根据ID查找消息
func (r *messageRepository) FindByID(ctx context.Context, id string) (*domain_conversation.Message, error) {
	var model MessageModel
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&model).Error

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("消息不存在")
	}
	if err != nil {
		return nil, fmt.Errorf("查询消息失败: %w", err)
	}

	return model.ToDomain(), nil
}

// FindBySessionID 根据会话ID查找消息列表
func (r *messageRepository) FindBySessionID(ctx context.Context, sessionID string, req *domain_conversation.ListMessagesRequest) ([]*domain_conversation.Message, int64, error) {
	var models []*MessageModel
	var total int64

	db := r.db.WithContext(ctx).Model(&MessageModel{}).Where("session_id = ?", sessionID)

	// 角色过滤
	if req.Role != "" {
		db = db.Where("role = ?", req.Role)
	}

	// 统计总数
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计消息数量失败: %w", err)
	}

	// 分页查询
	offset := (req.Page - 1) * req.Size
	err := db.Order("created_at ASC").
		Limit(req.Size).
		Offset(offset).
		Find(&models).Error

	if err != nil {
		return nil, 0, fmt.Errorf("查询消息列表失败: %w", err)
	}

	result := make([]*domain_conversation.Message, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}

	return result, total, nil
}

// FindRecentBySessionID 取会话最近 limit 条消息：按时间倒序查库取最近 N 条，
// 再反转为时间正序（最早在前）返回，供多轮对话记忆按顺序回放。
func (r *messageRepository) FindRecentBySessionID(ctx context.Context, sessionID string, limit int) ([]*domain_conversation.Message, error) {
	if limit <= 0 {
		limit = 20
	}
	var models []*MessageModel
	err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at DESC").
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("查询最近消息失败: %w", err)
	}

	// 反转为时间正序（最早在前）
	result := make([]*domain_conversation.Message, len(models))
	for i, m := range models {
		result[len(models)-1-i] = m.ToDomain()
	}
	return result, nil
}

// FindByRequestID 根据请求ID查找消息
func (r *messageRepository) FindByRequestID(ctx context.Context, requestID string) (*domain_conversation.Message, error) {
	var model MessageModel
	err := r.db.WithContext(ctx).
		Where("request_id = ?", requestID).
		First(&model).Error

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("消息不存在")
	}
	if err != nil {
		return nil, fmt.Errorf("查询消息失败: %w", err)
	}

	return model.ToDomain(), nil
}

// Update 更新消息
func (r *messageRepository) Update(ctx context.Context, message *domain_conversation.Message) error {
	var model MessageModel
	model.FromDomain(message)
	return r.db.WithContext(ctx).Save(&model).Error
}

// UpdateContent 更新消息内容
func (r *messageRepository) UpdateContent(ctx context.Context, id string, content string) error {
	return r.db.WithContext(ctx).Model(&MessageModel{}).
		Where("id = ?", id).
		Update("content", content).Error
}

// UpdateCompleted 更新消息完成状态
func (r *messageRepository) UpdateCompleted(ctx context.Context, id string, isCompleted bool) error {
	return r.db.WithContext(ctx).Model(&MessageModel{}).
		Where("id = ?", id).
		Update("is_completed", isCompleted).Error
}

// Delete 删除消息（软删除）
func (r *messageRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&MessageModel{}).Error
}

// DeleteBySessionID 删除会话的所有消息
func (r *messageRepository) DeleteBySessionID(ctx context.Context, sessionID string) error {
	return r.db.WithContext(ctx).Where("session_id = ?", sessionID).Delete(&MessageModel{}).Error
}

// CountBySessionID 统计会话的消息数量
func (r *messageRepository) CountBySessionID(ctx context.Context, sessionID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&MessageModel{}).
		Where("session_id = ?", sessionID).
		Count(&count).Error

	return count, err
}

// GetTokenCountBySessionID 获取会话的 Token 总数
func (r *messageRepository) GetTokenCountBySessionID(ctx context.Context, sessionID string) (int, error) {
	var total int
	err := r.db.WithContext(ctx).
		Model(&MessageModel{}).
		Where("session_id = ?", sessionID).
		Select("COALESCE(SUM(token_count), 0)").
		Scan(&total).Error

	return total, err
}
