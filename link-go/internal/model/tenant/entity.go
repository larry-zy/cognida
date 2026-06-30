// Package tenant 提供租户领域的领域实体定义
package tenant

import "time"

// ========================================
// Tenant 租户实体
// ========================================

// Tenant 租户实体
// 对应 tenants 表
type Tenant struct {
	ID               int64
	Name             string
	Description      string
	APIKey           string
	RetrieverEngines *string
	Status           string
	Business         string
	StorageQuota     int64
	StorageUsed      int64
	AgentConfig      *string
	Settings         *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}

// ========================================
// TenantUser 租户用户关联
// ========================================

// TenantUser 租户用户关联
// 对应 tenant_users 表
type TenantUser struct {
	ID        int64
	TenantID  int64
	UserID    int64
	Role      string
	Status    string
	CreatedAt time.Time
}

// ========================================
// 租户状态常量
// ========================================

const (
	TenantStatusActive    = "active"
	TenantStatusSuspended = "suspended"
	TenantStatusDeleted   = "deleted"
)

// ========================================
// 租户用户角色常量
// ========================================

const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// ========================================
// 租户用户状态常量
// ========================================

const (
	TenantUserStatusActive   = "active"
	TenantUserStatusInactive = "inactive"
)

// ========================================
// TenantResponse 租户响应
// ========================================

// TenantResponse 租户响应
type TenantResponse struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	Business         string `json:"business"`
	Status           string `json:"status"`
	StorageQuota     int64  `json:"storage_quota"`
	StorageUsed      int64  `json:"storage_used"`
	RetrieverEngines *string `json:"retriever_engines,omitempty"`
	CreatedAt        int64  `json:"created_at"`
	UpdatedAt        int64  `json:"updated_at"`
}

// ToResponse 转换为响应格式
func (t *Tenant) ToResponse() *TenantResponse {
	return &TenantResponse{
		ID:               t.ID,
		Name:             t.Name,
		Description:      t.Description,
		Business:         t.Business,
		Status:           t.Status,
		StorageQuota:     t.StorageQuota,
		StorageUsed:      t.StorageUsed,
		RetrieverEngines: t.RetrieverEngines,
		CreatedAt:        t.CreatedAt.Unix(),
		UpdatedAt:        t.UpdatedAt.Unix(),
	}
}

// ========================================
// StorageStats 存储统计
// ========================================

// StorageStats 存储统计
type StorageStats struct {
	TotalSize int64 `json:"total_size"`
	KBCount   int64 `json:"kb_count"`
}
