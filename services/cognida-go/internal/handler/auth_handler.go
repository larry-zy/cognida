// Package handler 提供用户认证的HTTP处理器
package handler

import (
	"github.com/gin-gonic/gin"

	appAccount "cognida/internal/service/account"
)

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
		InternalError(c, err.Error())
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
		Unauthorized(c, err.Error())
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
		Unauthorized(c, err.Error())
		return
	}

	OK(c, result)
}

// Logout 登出
func (h *AuthHandler) Logout(c *gin.Context) {
	userID := GetUserID(c)

	if err := h.accountService.Logout(c.Request.Context(), userID); err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, gin.H{"message": "登出成功"})
}

// GetProfile 获取用户信息
func (h *AuthHandler) GetProfile(c *gin.Context) {
	userID := GetUserID(c)

	result, err := h.accountService.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		NotFound(c, err.Error())
		return
	}

	OK(c, result)
}
