package account

import (
	"cognida/internal/model/user"
	"context"
	"errors"
)

var ErrTenantAccessDenied = errors.New("无权访问该租户")

func (s *authService) authorizeTenant(ctx context.Context, userID, tenantID int64) error {
	repo, ok := s.userRepo.(user.MembershipRepository)
	if !ok {
		return ErrTenantAccessDenied
	}
	if _, err := repo.FindMembership(ctx, userID, tenantID); err != nil {
		return ErrTenantAccessDenied
	}
	return nil
}
func (s *authService) issueForTenant(ctx context.Context, usr *user.User, tenantID int64) (*AuthResponse, error) {
	scoped := *usr
	scoped.TenantID = tenantID
	access, refresh, expires, err := s.generateTokens(&scoped)
	if err != nil {
		return nil, err
	}
	if err := s.saveRefreshToken(ctx, usr.ID, refresh); err != nil {
		return nil, err
	}
	return &AuthResponse{AccessToken: access, RefreshToken: refresh, ExpiresAt: expires, TenantID: tenantID, User: user.UserInfo{ID: usr.ID, Username: usr.Username, Email: usr.Email, Avatar: usr.Avatar, Status: usr.Status, CreatedAt: usr.CreatedAt, TenantID: tenantID}}, nil
}
func (s *AccountService) AuthorizeTenant(ctx context.Context, userID, tenantID int64) error {
	auth, ok := s.authService.(*authService)
	if !ok {
		return ErrTenantAccessDenied
	}
	return auth.authorizeTenant(ctx, userID, tenantID)
}
func (s *AccountService) MemberTenants(ctx context.Context, userID int64, cursor string, limit int) ([]*user.Membership, string, error) {
	auth, ok := s.authService.(*authService)
	if !ok {
		return nil, "", ErrTenantAccessDenied
	}
	repo, ok := auth.userRepo.(user.MembershipRepository)
	if !ok {
		return nil, "", ErrTenantAccessDenied
	}
	return repo.ListMemberships(ctx, userID, cursor, limit)
}
func (s *AccountService) SwitchTenant(ctx context.Context, userID, tenantID int64) (*AuthResponse, error) {
	auth, ok := s.authService.(*authService)
	if !ok {
		return nil, ErrTenantAccessDenied
	}
	var response *AuthResponse
	err := auth.withTx(ctx, func(ctx context.Context) error {
		if err := auth.authorizeTenant(ctx, userID, tenantID); err != nil {
			return err
		}
		usr, err := auth.userRepo.FindByID(ctx, userID)
		if err != nil {
			return err
		}
		response, err = auth.issueForTenant(ctx, usr, tenantID)
		return err
	})
	return response, err
}

// CreateWorkspace creates the tenant and its owner in the same transaction.
func (s *AccountService) CreateWorkspace(ctx context.Context, userID int64, req *CreateTenantRequest) (*TenantResponse, error) {
	auth, ok := s.authService.(*authService)
	if !ok {
		return nil, ErrTenantAccessDenied
	}
	repo, ok := auth.userRepo.(user.MembershipRepository)
	if !ok {
		return nil, ErrTenantAccessDenied
	}
	var created *TenantResponse
	err := auth.withTx(ctx, func(ctx context.Context) error {
		usr, err := auth.userRepo.FindByID(ctx, userID)
		if err != nil || usr.Status != 1 {
			return ErrAccountDisabled
		}
		created, err = s.tenantService.CreateTenant(ctx, req)
		if err != nil {
			return err
		}
		return repo.AddMembership(ctx, userID, created.ID, "owner")
	})
	return created, err
}

func (s *AccountService) AuthorizeTenantManagement(ctx context.Context, userID, tenantID int64) error {
	auth, ok := s.authService.(*authService)
	if !ok {
		return ErrTenantAccessDenied
	}
	repo, ok := auth.userRepo.(user.MembershipRepository)
	if !ok {
		return ErrTenantAccessDenied
	}
	member, err := repo.FindMembership(ctx, userID, tenantID)
	if err != nil || (member.Role != "owner" && member.Role != "admin") {
		return ErrTenantAccessDenied
	}
	return nil
}
