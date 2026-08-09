// Package user 提供用户领域的仓储接口定义
package user

import "context"

// ========================================
// UserRepository 用户仓储接口
// ========================================

// UserRepository 用户仓储接口
type UserRepository interface {
	// Create 创建用户
	Create(ctx context.Context, user *User) error

	// FindByID 根据ID查找用户
	FindByID(ctx context.Context, id int64) (*User, error)

	// FindByEmail 根据邮箱查找用户（租户内唯一）
	FindByEmail(ctx context.Context, tenantID int64, email string) (*User, error)

	// FindByEmailOnly 根据邮箱查找用户（跨所有租户）
	FindByEmailOnly(ctx context.Context, email string) (*User, error)

	// FindByUsername 根据用户名查找用户（租户内唯一）
	FindByUsername(ctx context.Context, tenantID int64, username string) (*User, error)

	// FindByTenantID 根据租户ID查找用户列表
	FindByTenantID(ctx context.Context, tenantID int64, page, pageSize int) ([]*User, int64, error)

	// Update 更新用户
	Update(ctx context.Context, user *User) error

	// UpdatePassword 更新密码
	UpdatePassword(ctx context.Context, userID int64, passwordHash string) error

	// UpdateAvatar 更新头像
	UpdateAvatar(ctx context.Context, userID int64, avatar string) error

	// UpdateLastLogin 更新最后登录时间
	UpdateLastLogin(ctx context.Context, userID int64) error

	// UpdateStatus 更新用户状态
	UpdateStatus(ctx context.Context, userID int64, status int8) error

	// Delete 删除用户（软删除）
	Delete(ctx context.Context, userID int64) error

	// Exists 检查用户是否存在
	Exists(ctx context.Context, userID int64) (bool, error)

	// ExistsByEmail 检查邮箱是否已存在
	ExistsByEmail(ctx context.Context, tenantID int64, email string) (bool, error)

	// ExistsByUsername 检查用户名是否已存在
	ExistsByUsername(ctx context.Context, tenantID int64, username string) (bool, error)

	// CountByTenantID 统计租户的用户数量
	CountByTenantID(ctx context.Context, tenantID int64) (int64, error)
}

// ========================================
// RefreshTokenRepository 刷新令牌仓储接口
// ========================================

// RefreshTokenRepository 刷新令牌仓储接口
type RefreshTokenRepository interface {
	// Create 创建刷新令牌
	Create(ctx context.Context, token *RefreshToken) error

	// FindByHash 根据哈希查找令牌
	FindByHash(ctx context.Context, tokenHash string) (*RefreshToken, error)

	// FindByUserID 根据用户ID查找令牌列表
	FindByUserID(ctx context.Context, userID int64) ([]*RefreshToken, error)

	// Delete 删除令牌
	Delete(ctx context.Context, id int64) error

	// DeleteByUserID 删除用户的所有令牌
	DeleteByUserID(ctx context.Context, userID int64) error

	// DeleteExpired 删除过期令牌
	DeleteExpired(ctx context.Context) error

	// RevokeUserTokens 撤销用户的所有令牌
	RevokeUserTokens(ctx context.Context, userID int64) error
}

// ========================================
// UserPreferenceRepository 用户偏好仓储接口
// ========================================

// UserPreferenceRepository 用户偏好仓储接口
type UserPreferenceRepository interface {
	// FindByUserID 根据用户ID查找偏好设置
	FindByUserID(ctx context.Context, userID int64) (*UserPreference, error)

	// Create 创建偏好设置
	Create(ctx context.Context, preference *UserPreference) error

	// Update 更新偏好设置
	Update(ctx context.Context, preference *UserPreference) error

	// UpdateLanguage 更新语言设置
	UpdateLanguage(ctx context.Context, userID int64, language string) error

	// UpdateTheme 更新主题设置
	UpdateTheme(ctx context.Context, userID int64, theme string) error

	// Delete 删除偏好设置
	Delete(ctx context.Context, userID int64) error
}

// ========================================
// APIKeyRepository API密钥仓储接口
// ========================================

// APIKeyRepository API密钥仓储接口
type APIKeyRepository interface {
	// Create 创建API密钥
	Create(ctx context.Context, key *APIKey) error

	// FindByID 根据ID查找API密钥
	FindByID(ctx context.Context, id int64) (*APIKey, error)

	// FindByHash 根据哈希查找API密钥
	FindByHash(ctx context.Context, keyHash string) (*APIKey, error)

	// FindByUserID 根据用户ID查找API密钥列表
	FindByUserID(ctx context.Context, userID int64) ([]*APIKey, error)

	// Update 更新API密钥
	Update(ctx context.Context, key *APIKey) error

	// UpdateLastUsed 更新最后使用时间
	UpdateLastUsed(ctx context.Context, id int64) error

	// UpdateStatus 更新状态
	UpdateStatus(ctx context.Context, id int64, status int8) error

	// Delete 删除API密钥
	Delete(ctx context.Context, id int64) error

	// DeleteByUserID 删除用户的所有API密钥
	DeleteByUserID(ctx context.Context, userID int64) error

	// Revoke 撤销API密钥
	Revoke(ctx context.Context, id int64) error
}
