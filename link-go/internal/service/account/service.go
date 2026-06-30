// Package account provides account and tenant management services
package account

import (
	"context"

	"link/internal/model/tenant"
	"link/internal/model/user"
	"link/internal/infrastructure/config"
)

// ========================================
// Account Service
// ========================================

// AccountService combines user and tenant services
type AccountService struct {
	authService    AuthService
	profileService ProfileService
	tenantService  TenantService
}

// NewAccountService creates a new account service
func NewAccountService(
	userRepo user.UserRepository,
	refreshTokenRepo user.RefreshTokenRepository,
	jwtConfig *config.JWTConfig,
	tenantRepo tenant.TenantRepository,
) *AccountService {
	// Create tenant service
	svc := NewTenantService(tenantRepo)

	// Create tenant service adapter for auth
	tenantAdapter := NewTenantServiceAdapter(tenantRepo)

	// Create user services
	authService := NewAuthService(userRepo, refreshTokenRepo, jwtConfig, tenantAdapter)
	profileService := NewProfileService(userRepo)

	return &AccountService{
		authService:    authService,
		profileService: profileService,
		tenantService:  svc,
	}
}

// ========================================
// User Operations (Auth & Profile)
// ========================================

// Register registers a new user
func (s *AccountService) Register(ctx context.Context, req *RegisterRequest) (*AuthResponse, error) {
	return s.authService.Register(ctx, req)
}

// Login authenticates a user
func (s *AccountService) Login(ctx context.Context, req *LoginRequest) (*AuthResponse, error) {
	return s.authService.Login(ctx, req)
}

// Logout logs out a user
func (s *AccountService) Logout(ctx context.Context, userID int64) error {
	return s.authService.Logout(ctx, userID)
}

// RefreshToken refreshes an authentication token
func (s *AccountService) RefreshToken(ctx context.Context, req *RefreshTokenRequest) (*AuthResponse, error) {
	return s.authService.RefreshToken(ctx, req)
}

// GetUserByID retrieves a user by ID
func (s *AccountService) GetUserByID(ctx context.Context, userID int64) (*UserInfo, error) {
	return s.profileService.GetProfile(ctx, userID)
}

// ValidateToken validates an authentication token
func (s *AccountService) ValidateToken(tokenString string) (*TokenClaims, error) {
	return s.authService.ValidateToken(tokenString)
}

// GetProfile retrieves a user's profile
func (s *AccountService) GetProfile(ctx context.Context, userID int64) (*UserInfo, error) {
	return s.profileService.GetProfile(ctx, userID)
}

// UpdateProfile updates a user's profile
func (s *AccountService) UpdateProfile(ctx context.Context, userID int64, req *UpdateUserRequest) (*UserInfo, error) {
	return s.profileService.UpdateProfile(ctx, userID, req)
}

// ChangePassword changes a user's password
func (s *AccountService) ChangePassword(ctx context.Context, userID int64, oldPassword, newPassword string) error {
	return s.profileService.ChangePassword(ctx, userID, oldPassword, newPassword)
}

// ========================================
// Tenant Operations
// ========================================

// CreateTenant creates a new tenant
func (s *AccountService) CreateTenant(ctx context.Context, req *CreateTenantRequest) (*TenantResponse, error) {
	return s.tenantService.CreateTenant(ctx, req)
}

// GetTenantByID gets tenant by ID
func (s *AccountService) GetTenantByID(ctx context.Context, id int64) (*TenantResponse, error) {
	return s.tenantService.GetTenant(ctx, id)
}

// ListTenants lists all tenants
func (s *AccountService) ListTenants(ctx context.Context, page, pageSize int) ([]*TenantResponse, int64, error) {
	return s.tenantService.ListTenants(ctx, page, pageSize)
}

// UpdateTenant updates a tenant
func (s *AccountService) UpdateTenant(ctx context.Context, id int64, req *UpdateTenantRequest) (*TenantResponse, error) {
	return s.tenantService.UpdateTenant(ctx, id, req)
}

// DeleteTenant deletes a tenant
func (s *AccountService) DeleteTenant(ctx context.Context, id int64) error {
	return s.tenantService.DeleteTenant(ctx, id)
}

// RegenerateAPIKey regenerates API key
func (s *AccountService) RegenerateAPIKey(ctx context.Context, id int64) (string, error) {
	return s.tenantService.RotateAPIKey(ctx, id)
}

// ValidateAPIKey validates API key
func (s *AccountService) ValidateAPIKey(ctx context.Context, apiKey string) (*TenantResponse, error) {
	return s.tenantService.ValidateAPIKey(ctx, apiKey)
}

// UpdateStorageUsed updates storage usage
func (s *AccountService) UpdateStorageUsed(ctx context.Context, tenantID int64, delta int64) error {
	return s.tenantService.UpdateStorageUsed(ctx, tenantID, delta)
}

// GetStorageUsage gets storage usage
func (s *AccountService) GetStorageUsage(ctx context.Context, id int64) (*StorageUsageResponse, error) {
	return s.tenantService.GetStorageUsage(ctx, id)
}

// ========================================
// TenantServiceAdapter 租户服务适配器实现
// ========================================

// tenantServiceAdapterImpl implements TenantServiceAdapter interface
type tenantServiceAdapterImpl struct {
	tenantService TenantService
}

// NewTenantServiceAdapter creates a new tenant service adapter
func NewTenantServiceAdapter(tenantRepo tenant.TenantRepository) TenantServiceAdapter {
	return &tenantServiceAdapterImpl{
		tenantService: NewTenantService(tenantRepo),
	}
}

// CreateTenant implements TenantServiceAdapter
func (a *tenantServiceAdapterImpl) CreateTenant(ctx context.Context, req interface{}) (*TenantInfo, error) {
	// Convert map to CreateTenantRequest
	createReq, ok := req.(*CreateTenantRequest)
	if !ok {
		// Try map[string]interface{}
		m, ok := req.(map[string]interface{})
		if !ok {
			return nil, nil
		}
		createReq = &CreateTenantRequest{}
		if name, ok := m["name"].(string); ok {
			createReq.Name = name
		}
	}

	resp, err := a.tenantService.CreateTenant(ctx, createReq)
	if err != nil {
		return nil, err
	}
	return &TenantInfo{ID: resp.ID}, nil
}

