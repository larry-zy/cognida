// Package account provides profile service implementation
package account

import (
	"context"

	"golang.org/x/crypto/bcrypt"

	"cognida/internal/model/user"
)

// ========================================
// ProfileService Implementation
// ========================================

// profileService implements ProfileService
type profileService struct {
	userRepo user.UserRepository
}

// NewProfileService creates a new profile service
func NewProfileService(userRepo user.UserRepository) ProfileService {
	return &profileService{
		userRepo: userRepo,
	}
}

// GetProfile retrieves a user's profile
func (s *profileService) GetProfile(ctx context.Context, userID int64) (*user.UserInfo, error) {
	usr, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &user.UserInfo{
		ID:        usr.ID,
		Username:  usr.Username,
		Email:     usr.Email,
		Avatar:    usr.Avatar,
		Status:    usr.Status,
		CreatedAt: usr.CreatedAt,
		TenantID:  usr.TenantID,
	}, nil
}

// UpdateProfile updates a user's profile
func (s *profileService) UpdateProfile(ctx context.Context, userID int64, req *UpdateUserRequest) (*user.UserInfo, error) {
	usr, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// 更新字段
	if req.Username != "" {
		usr.Username = req.Username
	}
	if req.Email != "" {
		usr.Email = req.Email
	}
	if req.Avatar != "" {
		usr.Avatar = req.Avatar
	}

	// 保存更新
	err = s.userRepo.Update(ctx, usr)
	if err != nil {
		return nil, err
	}

	return &user.UserInfo{
		ID:        usr.ID,
		Username:  usr.Username,
		Email:     usr.Email,
		Avatar:    usr.Avatar,
		Status:    usr.Status,
		CreatedAt: usr.CreatedAt,
		TenantID:  usr.TenantID,
	}, nil
}

// ChangePassword changes a user's password
func (s *profileService) ChangePassword(ctx context.Context, userID int64, oldPassword, newPassword string) error {
	usr, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return ErrUserNotFound
	}

	// 验证旧密码
	err = bcrypt.CompareHashAndPassword([]byte(usr.PasswordHash), []byte(oldPassword))
	if err != nil {
		return ErrOldPasswordIncorrect
	}

	// 加密新密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	return s.userRepo.UpdatePassword(ctx, userID, string(hashedPassword))
}
