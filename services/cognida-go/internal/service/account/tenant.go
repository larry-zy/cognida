// Package account provides tenant service implementation
package account

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"cognida/internal/model/tenant"
)

// ========================================
// TenantService Implementation
// ========================================

// tenantService implements TenantService
type tenantService struct {
	tenantRepo tenant.TenantRepository
}

// NewTenantService creates a new tenant service
func NewTenantService(tenantRepo tenant.TenantRepository) TenantService {
	return &tenantService{
		tenantRepo: tenantRepo,
	}
}

// CreateTenant creates a new tenant
func (s *tenantService) CreateTenant(ctx context.Context, req *CreateTenantRequest) (*TenantResponse, error) {
	// 检查租户名称是否已存在
	exists, err := s.tenantRepo.ExistsByName(ctx, strings.TrimSpace(req.Name))
	if err != nil {
		return nil, fmt.Errorf("检查租户名称失败: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("租户名称已存在")
	}

	// 设置默认存储配额 (10GB)
	storageQuota := req.StorageQuota
	if storageQuota == 0 {
		storageQuota = 10 * 1024 * 1024 * 1024 // 10GB
	}

	// 生成 API Key
	apiKey, err := s.generateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("生成 API Key 失败: %w", err)
	}

	tenantEnt := &tenant.Tenant{
		Name:         strings.TrimSpace(req.Name),
		Description:  strings.TrimSpace(req.Description),
		Business:     strings.TrimSpace(req.Business),
		APIKey:       apiKey,
		Status:       tenant.TenantStatusActive,
		StorageQuota: storageQuota,
		StorageUsed:  0,
	}

	err = s.tenantRepo.Create(ctx, tenantEnt)
	if err != nil {
		return nil, fmt.Errorf("创建租户失败: %w", err)
	}

	return s.toTenantResponse(tenantEnt), nil
}

// GetTenant retrieves a tenant by ID
func (s *tenantService) GetTenant(ctx context.Context, id int64) (*TenantResponse, error) {
	tenantEnt, err := s.tenantRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.toTenantResponse(tenantEnt), nil
}

// GetTenantByAPIKey retrieves a tenant by API key
func (s *tenantService) GetTenantByAPIKey(ctx context.Context, apiKey string) (*TenantResponse, error) {
	tenantEnt, err := s.tenantRepo.FindByAPIKey(ctx, apiKey)
	if err != nil {
		return nil, err
	}
	return s.toTenantResponse(tenantEnt), nil
}

// ListTenants lists all tenants
func (s *tenantService) ListTenants(ctx context.Context, page, pageSize int) ([]*TenantResponse, int64, error) {
	tenants, total, err := s.tenantRepo.FindAll(ctx, page, pageSize)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*TenantResponse, len(tenants))
	for i, t := range tenants {
		responses[i] = s.toTenantResponse(t)
	}

	return responses, total, nil
}

// UpdateTenant updates a tenant
func (s *tenantService) UpdateTenant(ctx context.Context, id int64, req *UpdateTenantRequest) (*TenantResponse, error) {
	tenantEnt, err := s.tenantRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 更新字段
	if req.Name != "" {
		// 检查新名称是否已被其他租户使用
		if req.Name != tenantEnt.Name {
			exists, err := s.tenantRepo.ExistsByName(ctx, req.Name)
			if err != nil {
				return nil, fmt.Errorf("检查租户名称失败: %w", err)
			}
			if exists {
				return nil, fmt.Errorf("租户名称已存在")
			}
			tenantEnt.Name = strings.TrimSpace(req.Name)
		}
	}
	if req.Description != "" {
		tenantEnt.Description = strings.TrimSpace(req.Description)
	}
	if req.Business != "" {
		tenantEnt.Business = strings.TrimSpace(req.Business)
	}
	if req.Status != "" {
		tenantEnt.Status = req.Status
	}

	err = s.tenantRepo.Update(ctx, tenantEnt)
	if err != nil {
		return nil, fmt.Errorf("更新租户失败: %w", err)
	}

	return s.toTenantResponse(tenantEnt), nil
}

// DeleteTenant deletes a tenant
func (s *tenantService) DeleteTenant(ctx context.Context, id int64) error {
	return s.tenantRepo.Delete(ctx, id)
}

// UpdateStorageUsed updates the storage usage for a tenant
func (s *tenantService) UpdateStorageUsed(ctx context.Context, tenantID int64, delta int64) error {
	// 获取当前租户
	tenantEnt, err := s.tenantRepo.FindByID(ctx, tenantID)
	if err != nil {
		return err
	}

	// 更新存储使用量
	newStorageUsed := tenantEnt.StorageUsed + delta
	if newStorageUsed < 0 {
		newStorageUsed = 0
	}

	return s.tenantRepo.UpdateStorage(ctx, tenantID, newStorageUsed)
}

// RotateAPIKey rotates the API key for a tenant
func (s *tenantService) RotateAPIKey(ctx context.Context, tenantID int64) (string, error) {
	apiKey, err := s.generateAPIKey()
	if err != nil {
		return "", fmt.Errorf("生成 API Key 失败: %w", err)
	}

	err = s.tenantRepo.UpdateAPIKey(ctx, tenantID, apiKey)
	if err != nil {
		return "", fmt.Errorf("更新 API Key 失败: %w", err)
	}

	return apiKey, nil
}

// GetStorageUsage gets storage usage for a tenant
func (s *tenantService) GetStorageUsage(ctx context.Context, id int64) (*StorageUsageResponse, error) {
	tenantEnt, err := s.tenantRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	quota := tenantEnt.StorageQuota
	used := tenantEnt.StorageUsed
	percentage := 0.0
	if quota > 0 {
		percentage = float64(used) / float64(quota) * 100
	}

	return &StorageUsageResponse{
		Quota:      quota,
		Used:       used,
		Percentage: percentage,
	}, nil
}

// ValidateAPIKey validates API key and returns tenant
func (s *tenantService) ValidateAPIKey(ctx context.Context, apiKey string) (*TenantResponse, error) {
	tenantEnt, err := s.tenantRepo.FindByAPIKey(ctx, apiKey)
	if err != nil {
		return nil, err
	}

	// 检查租户状态
	if tenantEnt.Status != tenant.TenantStatusActive {
		return nil, fmt.Errorf("租户已被暂停或删除")
	}

	return s.toTenantResponse(tenantEnt), nil
}

// generateAPIKey generates an API key
func (s *tenantService) generateAPIKey() (string, error) {
	// 生成随机字节
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}

	// 计算哈希
	hash := sha256.Sum256(randomBytes)
	hashStr := hex.EncodeToString(hash[:])

	// 生成 API Key 格式: tenant_<hex>
	apiKey := fmt.Sprintf("tenant_%s", hashStr[:32])

	return apiKey, nil
}

// toTenantResponse converts to response format
func (s *tenantService) toTenantResponse(tenantEnt *tenant.Tenant) *TenantResponse {
	return &TenantResponse{
		ID:               tenantEnt.ID,
		Name:             tenantEnt.Name,
		Description:      tenantEnt.Description,
		Business:         tenantEnt.Business,
		Status:           tenantEnt.Status,
		StorageQuota:     tenantEnt.StorageQuota,
		StorageUsed:      tenantEnt.StorageUsed,
		RetrieverEngines: tenantEnt.RetrieverEngines,
		CreatedAt:        tenantEnt.CreatedAt.Unix(),
		UpdatedAt:        tenantEnt.UpdatedAt.Unix(),
	}
}
