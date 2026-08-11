// Package handler 提供用户认证的HTTP处理器
package handler

import (
	"errors"

	"github.com/gin-gonic/gin"

	appAccount "cognida/internal/service/account"
)

// respondAuthError 将账号域业务错误映射为恰当的 HTTP 语义，未识别错误委托中心化
// RespondError（context/gorm/游标 → 对应码，其余 → 500 且不回显内部细节）〔R2-1/M7〕。
// 对外文案统一取 sentinel 自身 message，即便 service 侧对 sentinel 做了 %w 包裹，
// 也只暴露安全文案、不泄漏包裹链中的内部细节。
func respondAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, appAccount.ErrEmailExists):
		Conflict(c, appAccount.ErrEmailExists.Error(), nil)
	case errors.Is(err, appAccount.ErrUsernameExists):
		Conflict(c, appAccount.ErrUsernameExists.Error(), nil)
	case errors.Is(err, appAccount.ErrInvalidCredential):
		Unauthorized(c, appAccount.ErrInvalidCredential.Error())
	case errors.Is(err, appAccount.ErrInvalidToken):
		Unauthorized(c, appAccount.ErrInvalidToken.Error())
	case errors.Is(err, appAccount.ErrAccountDisabled):
		Forbidden(c, appAccount.ErrAccountDisabled.Error())
	case errors.Is(err, appAccount.ErrUserNotFound):
		NotFound(c, appAccount.ErrUserNotFound.Error())
	case errors.Is(err, appAccount.ErrOldPasswordIncorrect):
		BadRequest(c, appAccount.ErrOldPasswordIncorrect.Error())
	default:
		RespondError(c, err)
	}
}

// ========================================
// AuthHandler 认证处理器
// ========================================

// AuthHandler 认证处理器
type AuthHandler struct {
	accountService *appAccount.AccountService
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(accountService *appAccount.AccountService) *AuthHandler {
	return &AuthHandler{
		accountService: accountService,
	}
}

// Register 用户注册
func (h *AuthHandler) Register(c *gin.Context) {
	var req appAccount.RegisterRequest
	if !BindJSON(c, &req) {
		return
	}

	result, err := h.accountService.Register(c.Request.Context(), &req)
	if err != nil {
		respondAuthError(c, err)
		return
	}

	Created(c, result)
}

// Login 用户登录
func (h *AuthHandler) Login(c *gin.Context) {
	var req appAccount.LoginRequest
	if !BindJSON(c, &req) {
		return
	}

	result, err := h.accountService.Login(c.Request.Context(), &req)
	if err != nil {
		respondAuthError(c, err)
		return
	}

	OK(c, result)
}

// RefreshToken 刷新令牌
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req appAccount.RefreshTokenRequest
	if !BindJSON(c, &req) {
		return
	}

	result, err := h.accountService.RefreshToken(c.Request.Context(), &req)
	if err != nil {
		respondAuthError(c, err)
		return
	}

	OK(c, result)
}

// Logout 登出
func (h *AuthHandler) Logout(c *gin.Context) {
	userID := GetUserID(c)

	if err := h.accountService.Logout(c.Request.Context(), userID); err != nil {
		respondAuthError(c, err)
		return
	}

	OK(c, gin.H{"message": "登出成功"})
}

// GetProfile 获取用户信息
func (h *AuthHandler) GetProfile(c *gin.Context) {
	userID := GetUserID(c)

	result, err := h.accountService.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		respondAuthError(c, err)
		return
	}

	OK(c, result)
}
