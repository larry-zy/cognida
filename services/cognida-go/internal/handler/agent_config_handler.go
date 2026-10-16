package handler

import (
	d "cognida/internal/model/agent"
	service "cognida/internal/service/agent"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
)

func configIdentity(c *gin.Context) (d.ConfigScope, bool) {
	p := d.ConfigScope{TenantID: c.GetInt64("tenant_id"), UserID: c.GetInt64("user_id")}
	if p.Validate() != nil {
		Unauthorized(c, "需要登录后管理个人 Agent 配置")
		return p, false
	}
	return p, true
}
func configError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, d.ErrConfigInvalid):
		BadRequest(c, err.Error())
	case errors.Is(err, d.ErrConfigConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "配置版本已变化或幂等键与请求不匹配，请刷新后重试"})
	default:
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Agent 配置存储暂不可用"})
	}
}

// GetConfig returns the caller's private overrides. Null means use runtime defaults.
func (h *AgentHandler) GetConfig(c *gin.Context) {
	p, ok := configIdentity(c)
	if !ok {
		return
	}
	snapshot, err := h.configUseCase.Read(c.Request.Context(), p)
	if err != nil {
		configError(c, err)
		return
	}
	OK(c, snapshot)
}

// UpdateConfig atomically patches only implemented settings, with durable CAS/replay.
func (h *AgentHandler) UpdateConfig(c *gin.Context) {
	p, ok := configIdentity(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	var req service.ConfigUpdate
	if err := decoder.Decode(&req); err != nil {
		BadRequest(c, "配置必须为有效 JSON，且只包含支持的字段")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		BadRequest(c, "配置只能包含一个 JSON 对象")
		return
	}
	snapshot, err := h.configUseCase.Update(c.Request.Context(), p, req, c.GetHeader("Idempotency-Key"))
	if err != nil {
		configError(c, err)
		return
	}
	OK(c, snapshot)
}
