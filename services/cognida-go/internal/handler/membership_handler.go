package handler

import (
	appAccount "cognida/internal/service/account"
	"github.com/gin-gonic/gin"
	"strconv"
)

func (h *AuthHandler) MemberTenants(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	items, next, err := h.accountService.MemberTenants(c.Request.Context(), GetUserID(c), c.Query("cursor"), limit)
	if err != nil {
		respondAuthError(c, err)
		return
	}
	OK(c, gin.H{"items": items, "next_cursor": next, "has_more": next != ""})
}
func (h *AuthHandler) SwitchTenant(c *gin.Context) {
	var req struct {
		TenantID int64 `json:"tenant_id" binding:"required,gt=0"`
	}
	if !BindJSON(c, &req) {
		return
	}
	response, err := h.accountService.SwitchTenant(c.Request.Context(), GetUserID(c), req.TenantID)
	if err != nil {
		respondAuthError(c, err)
		return
	}
	OK(c, response)
}

func (h *AuthHandler) CreateWorkspace(c *gin.Context) {
	var req appAccount.CreateTenantRequest
	if !BindJSON(c, &req) {
		return
	}
	result, err := h.accountService.CreateWorkspace(c.Request.Context(), GetUserID(c), &req)
	if err != nil {
		respondAuthError(c, err)
		return
	}
	Created(c, result)
}
