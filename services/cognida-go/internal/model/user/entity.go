// Package user 提供用户领域的领域实体定义
package user

import "time"

// ========================================
// User 用户实体
// ========================================

// User 用户实体
// 对应 users 表
type User struct {
	ID           int64
	TenantID     int64
	Username     string
	Email        string
	PasswordHash string
	Avatar       string
	Status       int8
	CreatedAt    time.Time
	UpdatedAt    time.Time
	LastLoginAt  *time.Time
}

// ========================================
// RefreshToken 刷新令牌实体
// ========================================

// RefreshToken 刷新令牌实体
// 对应 refresh_tokens 表
type RefreshToken struct {
	ID        int64
	UserID    int64
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
}


// RefreshTokenEntity 别名，保持兼容性
type RefreshTokenEntity = RefreshToken

// ========================================
// UserPreference 用户偏好设置
// ========================================

// UserPreference 用户偏好设置
type UserPreference struct {
	ID                  int64
	UserID              int64
	Language            string
	Theme               string
	NotificationEnabled bool
	PreferenceJSON      string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}


// ========================================
// APIKey API密钥实体
// ========================================

// APIKey API密钥实体
type APIKey struct {
	ID         int64
	UserID     int64
	Name       string
	KeyHash    string
	KeyPrefix  string
	Scopes     string
	LastUsedAt *time.Time
	ExpiresAt  *time.Time
	Status     int8
	CreatedAt  time.Time
}


// ========================================
// TokenClaims JWT令牌声明
// ========================================

// TokenClaims JWT Token声明
type TokenClaims struct {
	UserID    int64  `json:"user_id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	TenantID  int64  `json:"tenant_id,omitempty"`
	TokenType string `json:"token_type"` // access or refresh
}

// ========================================
// UserInfo 用户信息（不含敏感信息）
// ========================================

// UserInfo 用户信息（不含敏感信息）
type UserInfo struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Avatar    string    `json:"avatar"`
	Status    int8      `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	TenantID  int64     `json:"tenant_id,omitempty"`
}
