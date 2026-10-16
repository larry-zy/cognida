package handler

import (
	d "cognida/internal/model/huggingface"
	l "cognida/internal/model/lifecycle"
	hf "cognida/internal/service/huggingface"
	"context"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"strconv"
	"time"
)

type HuggingFaceHandler struct{ service *hf.Service }

func NewHuggingFaceHandler(service *hf.Service) *HuggingFaceHandler {
	return &HuggingFaceHandler{service}
}
func (h *HuggingFaceHandler) Register(api *gin.RouterGroup) {
	scope := func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if hfPrincipal(c).Validate() != nil {
			Unauthorized(c, "需要有效用户与租户身份")
			c.Abort()
			return
		}
		if h.service == nil {
			hfError(c, d.ErrUnavailable, nil)
			c.Abort()
			return
		}
		ctx, stop := context.WithTimeout(c.Request.Context(), 125*time.Second)
		defer stop()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
	api.GET("/data-connectors", scope, func(c *gin.Context) {
		OK(c, gin.H{"scope": "huggingface_connector", "items": []any{h.service.Capabilities()}})
	})
	routes := api.Group("/huggingface", scope)
	routes.GET("/datasets", h.Search)
	routes.GET("/dataset", h.Dataset)
	routes.POST("/preview", h.Preview)
	routes.POST("/imports", h.Import)
	routes.GET("/imports/:id", h.GetImport)
}
func hfPrincipal(c *gin.Context) l.Principal {
	return l.Principal{TenantID: c.GetInt64("tenant_id"), UserID: c.GetInt64("user_id")}
}
func hfError(c *gin.Context, e error, receipt *d.Import) {
	status, code := 503, "unavailable"
	switch {
	case errors.Is(e, d.ErrInvalid):
		status, code = 400, "invalid_request"
	case errors.Is(e, d.ErrForbidden), errors.Is(e, l.ErrForbidden):
		status, code = 403, "forbidden"
	case errors.Is(e, d.ErrNotFound):
		status, code = 404, "not_found"
	case errors.Is(e, d.ErrConflict):
		status, code = 409, "conflict"
	case errors.Is(e, d.ErrLimit):
		status, code = 413, "limit_exceeded"
	case errors.Is(e, d.ErrDecode):
		status, code = 422, "decode_error"
	case errors.Is(e, d.ErrUpstream):
		status, code = 502, "upstream_error"
	case errors.Is(e, context.DeadlineExceeded):
		status, code = 504, "deadline_exceeded"
	case errors.Is(e, context.Canceled):
		status, code = 408, "cancelled"
	}
	c.JSON(status, gin.H{"error": code, "code": code, "data": receipt})
}
func hfBody(c *gin.Context) (d.Request, error) {
	var in d.Request
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if e := decoder.Decode(&in); e != nil {
		return in, d.ErrInvalid
	}
	if e := decoder.Decode(new(any)); e != io.EOF {
		return in, d.ErrInvalid
	}
	return in, in.Validate()
}
func (h *HuggingFaceHandler) Search(c *gin.Context) {
	limit, e := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if e != nil {
		hfError(c, d.ErrInvalid, nil)
		return
	}
	out, e := h.service.Search(c.Request.Context(), hfPrincipal(c), c.Query("q"), c.Query("cursor"), limit, c.Query("credential_id"))
	if e != nil {
		hfError(c, e, nil)
		return
	}
	OK(c, out)
}
func (h *HuggingFaceHandler) Dataset(c *gin.Context) {
	out, e := h.service.Dataset(c.Request.Context(), hfPrincipal(c), c.Query("repo_id"), c.DefaultQuery("revision", "main"), c.Query("credential_id"))
	if e != nil {
		hfError(c, e, nil)
		return
	}
	OK(c, out)
}
func (h *HuggingFaceHandler) Preview(c *gin.Context) {
	in, e := hfBody(c)
	if e != nil {
		hfError(c, e, nil)
		return
	}
	out, e := h.service.Preview(c.Request.Context(), hfPrincipal(c), in)
	if e != nil {
		hfError(c, e, nil)
		return
	}
	OK(c, out)
}
func (h *HuggingFaceHandler) Import(c *gin.Context) {
	in, e := hfBody(c)
	if e != nil {
		hfError(c, e, nil)
		return
	}
	out, e := h.service.Import(c.Request.Context(), hfPrincipal(c), in, c.GetHeader("Idempotency-Key"))
	if e != nil {
		hfError(c, e, out)
		return
	}
	OK(c, out)
}
func (h *HuggingFaceHandler) GetImport(c *gin.Context) {
	out, e := h.service.GetImport(c.Request.Context(), hfPrincipal(c), c.Param("id"))
	if e != nil {
		hfError(c, e, nil)
		return
	}
	OK(c, out)
}
