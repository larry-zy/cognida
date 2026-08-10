// Package account provides authentication and tenant service interface definitions
package account

import "context"

// ========================================
// AuthService 认证服务接口
// ========================================

// AuthService 定义用户认证服务的契约
type AuthService interface {
	// Register 注册新用户
	Register(ctx context.Context, req *RegisterRequest) (*AuthResponse, error)

	// Login 用户登录认证
	Login(ctx context.Context, req *LoginRequest) (*AuthResponse, error)

	// Logout 用户登出
	Logout(ctx context.Context, userID int64) error

	// RefreshToken 刷新认证令牌
	RefreshToken(ctx context.Context, req *RefreshTokenRequest) (*AuthResponse, error)

	// ValidateToken 验证认证令牌
	ValidateToken(tokenString string) (*TokenClaims, error)
}

// ========================================
// ProfileService 用户资料服务接口
// ========================================

// ProfileService 定义用户资料管理服务的契约
type ProfileService interface {
	// GetProfile 获取用户资料
	GetProfile(ctx context.Context, userID int64) (*UserInfo, error)

	// UpdateProfile 更新用户资料
	UpdateProfile(ctx context.Context, userID int64, req *UpdateUserRequest) (*UserInfo, error)

	// ChangePassword 修改用户密码
	ChangePassword(ctx context.Context, userID int64, oldPassword, newPassword string) error
}

// ========================================
// TenantService 租户服务接口
// ========================================

// TenantService 定义租户管理服务的契约
type TenantService interface {
	// CreateTenant 创建新租户
	CreateTenant(ctx context.Context, req *CreateTenantRequest) (*TenantResponse, error)

	// GetTenant 根据ID获取租户
	GetTenant(ctx context.Context, id int64) (*TenantResponse, error)

	// GetTenantByAPIKey 根据API密钥获取租户
	GetTenantByAPIKey(ctx context.Context, apiKey string) (*TenantResponse, error)

	// ListTenants 列出所有租户
	ListTenants(ctx context.Context, page, pageSize int) ([]*TenantResponse, int64, error)

	// ListTenantsByCursor 游标（keyset）分页列出租户〔M5〕：仅在携带 cursor 时启用。
	// 返回本页租户、下一页游标（空=末页），不返回总数。
	ListTenantsByCursor(ctx context.Context, cursor string, pageSize int) ([]*TenantResponse, string, error)

	// UpdateTenant 更新租户
	UpdateTenant(ctx context.Context, id int64, req *UpdateTenantRequest) (*TenantResponse, error)

	// DeleteTenant 删除租户
	DeleteTenant(ctx context.Context, id int64) error

	// UpdateStorageUsed 更新租户存储使用量
	UpdateStorageUsed(ctx context.Context, tenantID int64, delta int64) error

	// RotateAPIKey 旋转租户API密钥
	RotateAPIKey(ctx context.Context, tenantID int64) (string, error)

	// GetStorageUsage 获取存储使用情况
	GetStorageUsage(ctx context.Context, id int64) (*StorageUsageResponse, error)

	// ValidateAPIKey 验证API密钥并返回租户
	ValidateAPIKey(ctx context.Context, apiKey string) (*TenantResponse, error)
}

