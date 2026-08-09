// Package account provides DTO aliases for backward compatibility
package account

import (
	"cognida/internal/model/tenant"
	"cognida/internal/model/user"
)

// ========================================
// Auth DTOs (aliases to model layer)
// ========================================

// RegisterRequest 用户注册请求
type RegisterRequest = user.RegisterRequest

// LoginRequest 用户登录请求
type LoginRequest = user.LoginRequest

// RefreshTokenRequest 刷新Token请求
type RefreshTokenRequest = user.RefreshTokenRequest

// AuthResponse 认证响应
type AuthResponse = user.AuthResponse

// UserInfo 用户信息（不含敏感信息）
type UserInfo = user.UserInfo

// UpdateUserRequest 更新用户请求
type UpdateUserRequest = user.UpdateUserRequest

// TokenClaims JWT Token声明
type TokenClaims = user.TokenClaims

// ========================================
// Tenant DTOs (aliases to model layer)
// ========================================

// CreateTenantRequest 创建租户请求
type CreateTenantRequest = tenant.CreateTenantRequest

// UpdateTenantRequest 更新租户请求
type UpdateTenantRequest = tenant.UpdateTenantRequest

// TenantResponse 租户响应（不含敏感信息）
type TenantResponse = tenant.TenantResponse

// StorageUsageResponse 存储使用情况响应
type StorageUsageResponse struct {
	Quota      int64   `json:"quota"`
	Used       int64   `json:"used"`
	Percentage float64 `json:"percentage"`
}
