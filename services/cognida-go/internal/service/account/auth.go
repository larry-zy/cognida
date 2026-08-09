// Package account provides authentication service implementation
package account

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"cognida/internal/infrastructure/config"
	"cognida/internal/model/user"
)

// ========================================
// TenantServiceAdapter 租户服务适配器接口（避免循环依赖）
// ========================================

// TenantServiceAdapter 租户服务适配器（避免循环依赖）
type TenantServiceAdapter interface {
	CreateTenant(ctx context.Context, req interface{}) (*TenantInfo, error)
}

// TenantInfo 租户信息（简化版，避免循环依赖）
type TenantInfo struct {
	ID int64
}

// ========================================
// AuthService Implementation
// ========================================

// authService implements AuthService
type authService struct {
	userRepo        user.UserRepository
	refreshTokenRepo user.RefreshTokenRepository
	jwtConfig       *config.JWTConfig
	tenantService   TenantServiceAdapter
}

// NewAuthService creates a new auth service
func NewAuthService(
	userRepo user.UserRepository,
	refreshTokenRepo user.RefreshTokenRepository,
	jwtConfig *config.JWTConfig,
	tenantService TenantServiceAdapter,
) AuthService {
	return &authService{
		userRepo:        userRepo,
		refreshTokenRepo: refreshTokenRepo,
		jwtConfig:       jwtConfig,
		tenantService:   tenantService,
	}
}

// Register registers a new user
func (s *authService) Register(ctx context.Context, req *RegisterRequest) (*AuthResponse, error) {
	var tenantID int64
	var err error

	// 获取或创建租户
	if req.TenantID == 0 {
		// 自动创建默认租户
		tenantID, err = s.createDefaultTenant(ctx, req.Username)
		if err != nil {
			return nil, fmt.Errorf("创建默认租户失败: %w", err)
		}
	} else {
		tenantID = req.TenantID
	}

	// 检查邮箱是否已存在（在租户内）
	existingUser, err := s.userRepo.FindByEmail(ctx, tenantID, req.Email)
	if err == nil && existingUser != nil {
		return nil, errors.New("邮箱已被注册")
	}

	// 检查用户名是否已存在（在租户内）
	existingUser, err = s.userRepo.FindByUsername(ctx, tenantID, req.Username)
	if err == nil && existingUser != nil {
		return nil, errors.New("用户名已被使用")
	}

	// 加密密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("密码加密失败: %w", err)
	}

	// 创建用户（包含租户ID）
	newUser := &user.User{
		TenantID:     tenantID,
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		Avatar:       "",
		Status:       1, // 默认启用
	}

	err = s.userRepo.Create(ctx, newUser)
	if err != nil {
		return nil, fmt.Errorf("创建用户失败 (tenantID=%d, username=%s): %w", tenantID, req.Username, err)
	}

	// 生成Token（包含租户ID）
	accessToken, refreshToken, expiresAt, err := s.generateTokens(newUser)
	if err != nil {
		return nil, fmt.Errorf("生成Token失败: %w", err)
	}

	// 存储刷新Token
	err = s.saveRefreshToken(ctx, newUser.ID, refreshToken)
	if err != nil {
		return nil, fmt.Errorf("保存刷新Token失败: %w", err)
	}

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
		TenantID:     newUser.TenantID,
		User: user.UserInfo{
			ID:        newUser.ID,
			Username:  newUser.Username,
			Email:     newUser.Email,
			Avatar:    newUser.Avatar,
			Status:    newUser.Status,
			CreatedAt: newUser.CreatedAt,
			TenantID:  newUser.TenantID,
		},
	}, nil
}

// createDefaultTenant 创建默认租户
func (s *authService) createDefaultTenant(ctx context.Context, username string) (int64, error) {
	if s.tenantService == nil {
		return 0, errors.New("租户服务未初始化，请提供租户ID")
	}

	// 使用 map 传递请求，避免类型匹配问题
	req := map[string]interface{}{
		"name": fmt.Sprintf("%s的租户", username),
	}

	// 调用租户服务创建租户
	tenantInfo, err := s.tenantService.CreateTenant(ctx, req)
	if err != nil {
		return 0, err
	}

	return tenantInfo.ID, nil
}

// Login authenticates a user
func (s *authService) Login(ctx context.Context, req *LoginRequest) (*AuthResponse, error) {
	var usr *user.User
	var err error

	// 如果没有提供租户ID，则仅通过邮箱查找用户（自动获取租户ID）
	if req.TenantID == 0 {
		usr, err = s.userRepo.FindByEmailOnly(ctx, req.Email)
	} else {
		usr, err = s.userRepo.FindByEmail(ctx, req.TenantID, req.Email)
	}

	if err != nil {
		return nil, errors.New("邮箱或密码错误")
	}

	// 验证密码
	err = bcrypt.CompareHashAndPassword([]byte(usr.PasswordHash), []byte(req.Password))
	if err != nil {
		return nil, errors.New("邮箱或密码错误")
	}

	// 检查用户状态
	if usr.Status != 1 {
		return nil, errors.New("账号已被禁用")
	}

	// 更新最后登录时间
	err = s.userRepo.UpdateLastLogin(ctx, usr.ID)
	if err != nil {
		return nil, fmt.Errorf("更新登录时间失败: %w", err)
	}

	// 生成Token（包含租户ID）
	accessToken, refreshToken, expiresAt, err := s.generateTokens(usr)
	if err != nil {
		return nil, fmt.Errorf("生成Token失败: %w", err)
	}

	// 存储刷新Token
	err = s.saveRefreshToken(ctx, usr.ID, refreshToken)
	if err != nil {
		return nil, fmt.Errorf("保存刷新Token失败: %w", err)
	}

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
		TenantID:     usr.TenantID,
		User: user.UserInfo{
			ID:        usr.ID,
			Username:  usr.Username,
			Email:     usr.Email,
			Avatar:    usr.Avatar,
			Status:    usr.Status,
			CreatedAt: usr.CreatedAt,
			TenantID:  usr.TenantID,
		},
	}, nil
}

// Logout logs out a user
func (s *authService) Logout(ctx context.Context, userID int64) error {
	// 删除用户的所有刷新Token
	err := s.refreshTokenRepo.DeleteByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("登出失败: %w", err)
	}
	return nil
}

// RefreshToken refreshes an authentication token
func (s *authService) RefreshToken(ctx context.Context, req *RefreshTokenRequest) (*AuthResponse, error) {
	// 验证刷新Token
	claims, err := s.ValidateToken(req.RefreshToken)
	if err != nil {
		return nil, errors.New("无效的刷新Token")
	}

	if claims.TokenType != "refresh" {
		return nil, errors.New("Token类型错误")
	}

	// 获取用户信息
	usr, err := s.userRepo.FindByID(ctx, claims.UserID)
	if err != nil {
		return nil, errors.New("用户不存在")
	}

	// 检查用户状态
	if usr.Status != 1 {
		return nil, errors.New("账号已被禁用")
	}

	// 验证刷新Token是否在数据库中
	tokenHash := s.hashToken(req.RefreshToken)
	_, err = s.refreshTokenRepo.FindByHash(ctx, tokenHash)
	if err != nil {
		return nil, errors.New("刷新Token不存在或已失效")
	}

	// 生成新的Token（包含租户ID）
	accessToken, refreshToken, expiresAt, err := s.generateTokens(usr)
	if err != nil {
		return nil, fmt.Errorf("生成Token失败: %w", err)
	}

	// 删除旧的刷新Token
	err = s.refreshTokenRepo.DeleteByUserID(ctx, usr.ID)
	if err != nil {
		return nil, fmt.Errorf("删除旧Token失败: %w", err)
	}

	// 存储新的刷新Token
	err = s.saveRefreshToken(ctx, usr.ID, refreshToken)
	if err != nil {
		return nil, fmt.Errorf("保存刷新Token失败: %w", err)
	}

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
		TenantID:     usr.TenantID,
		User: user.UserInfo{
			ID:        usr.ID,
			Username:  usr.Username,
			Email:     usr.Email,
			Avatar:    usr.Avatar,
			Status:    usr.Status,
			CreatedAt: usr.CreatedAt,
			TenantID:  usr.TenantID,
		},
	}, nil
}

// ValidateToken validates an authentication token
func (s *authService) ValidateToken(tokenString string) (*user.TokenClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("无效的签名方法: %v", token.Header["alg"])
		}
		return []byte(s.jwtConfig.Secret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		// 逐字段 comma-ok 提取：缺失/类型不符的声明返回可控错误（上游 401），
		// 而非裸类型断言 panic 成 500。user_id 为身份主键，缺失即视为无效令牌。
		userID, ok := claims["user_id"].(float64)
		if !ok {
			return nil, errors.New("无效的Token：缺少 user_id")
		}
		tokenClaims := &user.TokenClaims{
			UserID: int64(userID),
		}
		if username, ok := claims["username"].(string); ok {
			tokenClaims.Username = username
		}
		if email, ok := claims["email"].(string); ok {
			tokenClaims.Email = email
		}
		if tokenType, ok := claims["token_type"].(string); ok {
			tokenClaims.TokenType = tokenType
		}
		// 尝试获取 tenant_id（如果存在）
		if tenantID, ok := claims["tenant_id"].(float64); ok {
			tokenClaims.TenantID = int64(tenantID)
		}
		return tokenClaims, nil
	}

	return nil, errors.New("无效的Token")
}

// generateTokens 生成访问令牌和刷新令牌
func (s *authService) generateTokens(usr *user.User) (string, string, int64, error) {
	now := time.Now()
	accessExpiresAt := now.Add(time.Duration(s.jwtConfig.AccessTokenExpire) * time.Second)
	refreshExpiresAt := now.Add(time.Duration(s.jwtConfig.RefreshTokenExpire) * time.Second)

	// 生成访问Token（包含租户ID）
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":    usr.ID,
		"tenant_id":  usr.TenantID,
		"username":   usr.Username,
		"email":      usr.Email,
		"token_type": "access",
		"exp":        accessExpiresAt.Unix(),
		"iat":        now.Unix(),
	})

	accessTokenString, err := accessToken.SignedString([]byte(s.jwtConfig.Secret))
	if err != nil {
		return "", "", 0, err
	}

	// 生成刷新Token（包含租户ID）
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":    usr.ID,
		"tenant_id":  usr.TenantID,
		"username":   usr.Username,
		"email":      usr.Email,
		"token_type": "refresh",
		"exp":        refreshExpiresAt.Unix(),
		"iat":        now.Unix(),
	})

	refreshTokenString, err := refreshToken.SignedString([]byte(s.jwtConfig.Secret))
	if err != nil {
		return "", "", 0, err
	}

	return accessTokenString, refreshTokenString, accessExpiresAt.Unix(), nil
}

// saveRefreshToken 保存刷新Token到数据库
func (s *authService) saveRefreshToken(ctx context.Context, userID int64, token string) error {
	tokenHash := s.hashToken(token)
	expiresAt := time.Now().Add(time.Duration(s.jwtConfig.RefreshTokenExpire) * time.Second)

	refreshToken := &user.RefreshToken{
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
	}

	return s.refreshTokenRepo.Create(ctx, refreshToken)
}

// hashToken 对Token进行哈希
func (s *authService) hashToken(token string) string {
	h := sha256.New()
	h.Write([]byte(token))
	return hex.EncodeToString(h.Sum(nil))
}
