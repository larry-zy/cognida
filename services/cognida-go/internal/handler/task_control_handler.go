package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	d "cognida/internal/model/taskcontrol"
	svc "cognida/internal/service/taskcontrol"
	"github.com/gin-gonic/gin"
)

type TaskControlHandler struct {
	service    *svc.Service
	supervisor *svc.Supervisor
}

func NewTaskControlHandler(s *svc.Service) *TaskControlHandler {
	return &TaskControlHandler{service: s}
}
func (h *TaskControlHandler) Register(api *gin.RouterGroup) {
	g := api.Group("/governed-tasks")
	g.Use(func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if e := taskControlScope(c).Validate(); e != nil {
			Unauthorized(c, "需要有效的用户和租户身份")
			c.Abort()
			return
		}
		timeout := 30 * time.Second
		if c.FullPath() == g.BasePath()+"/:id/supervisor/propose" {
			// Leave time for the bounded model request and post-call validation.
			timeout = 60 * time.Second
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	g.GET("/capabilities", h.Capabilities)
	g.POST("/supervisor/assess", h.AssessTask)
	g.POST("/:id/supervisor/propose", h.ProposeTask)
	g.GET("/:id/supervisor/summary", h.SummarizeTask)
	g.POST("/validate", h.Validate)
	g.POST("", h.Create)
	g.GET("", h.List)
	g.GET("/:id", h.Get)
	g.POST("/:id/plans", h.AppendPlan)
	g.GET("/:id/plans/:version", h.GetPlan)
}
func taskControlScope(c *gin.Context) d.Scope {
	return d.Scope{TenantID: c.GetInt64("tenant_id"), UserID: c.GetInt64("user_id")}
}

// readTaskJSON rejects unknown and duplicate keys as well as trailing values.
// Neither identity, approval nor arbitrary executor parameters belong in a draft.
func readTaskJSON(c *gin.Context, out any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10)
	body, e := io.ReadAll(c.Request.Body)
	if e != nil {
		BadRequest(c, "任务请求超过大小限制")
		return false
	}
	check := json.NewDecoder(bytes.NewReader(body))
	check.UseNumber()
	if e = uniqueTaskJSON(check, 0); e != nil {
		BadRequest(c, "任务请求 JSON 不合法或包含重复字段")
		return false
	}
	if _, e = check.Token(); !errors.Is(e, io.EOF) {
		BadRequest(c, "请求只能包含一个 JSON 对象")
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if len(bytes.TrimSpace(body)) == 0 || bytes.TrimSpace(body)[0] != '{' {
		BadRequest(c, "请求必须为 JSON 对象")
		return false
	}
	if e = decoder.Decode(out); e != nil {
		BadRequest(c, "任务请求格式或字段不正确")
		return false
	}
	return true
}
func uniqueTaskJSON(dec *json.Decoder, depth int) error {
	if depth > 24 {
		return d.ErrInvalid
	}
	token, e := dec.Token()
	if e != nil {
		return e
	}
	if delim, ok := token.(json.Delim); ok {
		switch delim {
		case '{':
			seen := map[string]bool{}
			for dec.More() {
				k, e := dec.Token()
				if e != nil {
					return e
				}
				key, ok := k.(string)
				if !ok || !asciiTaskKey(key) || seen[key] {
					return d.ErrInvalid
				}
				seen[key] = true
				if e = uniqueTaskJSON(dec, depth+1); e != nil {
					return e
				}
			}
			end, e := dec.Token()
			if e != nil || end != json.Delim('}') {
				return d.ErrInvalid
			}
		case '[':
			for dec.More() {
				if e := uniqueTaskJSON(dec, depth+1); e != nil {
					return e
				}
			}
			end, e := dec.Token()
			if e != nil || end != json.Delim(']') {
				return d.ErrInvalid
			}
		default:
			return d.ErrInvalid
		}
	}
	return nil
}

// encoding/json accepts Unicode case-fold aliases (e.g. long s for s).
// Contract field names are exact lower-case ASCII; reject aliases before decoding.
func asciiTaskKey(key string) bool {
	if key == "" {
		return false
	}
	for _, r := range key {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}
func taskControlError(c *gin.Context, e error) {
	var validation *svc.ValidationError
	switch {
	case errors.Is(e, svc.ErrPlannerUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "Supervisor 模型暂不可用，请检查模型配置后重试"})
	case errors.Is(e, svc.ErrPlannerOutput):
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "message": "Supervisor 未返回有效的受约束计划，请补充目标后重试"})
	case errors.Is(e, context.DeadlineExceeded), errors.Is(e, context.Canceled):
		c.JSON(http.StatusGatewayTimeout, gin.H{"code": 504, "message": "任务规划已超时或取消"})
	case errors.As(e, &validation):
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "任务计划校验未通过", "issues": validation.Issues})
	case errors.Is(e, d.ErrForbidden):
		c.JSON(403, gin.H{"code": 403, "message": "任务或输入来源不可访问"})
	case errors.Is(e, d.ErrInvalid):
		BadRequest(c, "任务参数不合法")
	case errors.Is(e, d.ErrNotFound):
		NotFound(c, "任务或计划不存在")
	case errors.Is(e, d.ErrConflict):
		Conflict(c, "任务已变更，或幂等键已用于其他内容", nil)
	default:
		InternalError(c, "任务操作失败，请重试")
	}
}
func (h *TaskControlHandler) Capabilities(c *gin.Context) {
	items, e := h.service.Capabilities(c.Request.Context(), taskControlScope(c))
	if e != nil {
		taskControlError(c, e)
		return
	}
	OK(c, gin.H{"capabilities": items, "execution_enabled": false})
}
func (h *TaskControlHandler) Validate(c *gin.Context) {
	var in svc.PlanSpec
	if !readTaskJSON(c, &in) {
		return
	}
	result, e := h.service.Validate(c.Request.Context(), taskControlScope(c), in)
	if e != nil {
		taskControlError(c, e)
		return
	}
	OK(c, result)
}
func (h *TaskControlHandler) Create(c *gin.Context) {
	var in svc.DraftInput
	if !readTaskJSON(c, &in) {
		return
	}
	task, e := h.service.CreateDraft(c.Request.Context(), taskControlScope(c), in, c.GetHeader("Idempotency-Key"), c.GetString("request_id"))
	if e != nil {
		taskControlError(c, e)
		return
	}
	Created(c, task)
}
func (h *TaskControlHandler) Get(c *gin.Context) {
	task, e := h.service.Get(c.Request.Context(), taskControlScope(c), c.Param("id"))
	if e != nil {
		taskControlError(c, e)
		return
	}
	OK(c, task)
}
func (h *TaskControlHandler) List(c *gin.Context) {
	limit, e := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if e != nil || limit < 1 || limit > 100 {
		BadRequest(c, "page_size 必须为 1-100")
		return
	}
	page, e := h.service.List(c.Request.Context(), taskControlScope(c), d.ListFilter{Cursor: c.Query("cursor"), Limit: limit})
	if e != nil {
		taskControlError(c, e)
		return
	}
	OK(c, page)
}
func (h *TaskControlHandler) AppendPlan(c *gin.Context) {
	var in svc.AppendPlanInput
	if !readTaskJSON(c, &in) {
		return
	}
	result, e := h.service.AppendPlan(c.Request.Context(), taskControlScope(c), c.Param("id"), in)
	if e != nil {
		taskControlError(c, e)
		return
	}
	Created(c, result)
}
func (h *TaskControlHandler) GetPlan(c *gin.Context) {
	version, e := strconv.ParseInt(c.Param("version"), 10, 64)
	if e != nil || version < 1 {
		BadRequest(c, "计划版本不合法")
		return
	}
	plan, e := h.service.GetPlan(c.Request.Context(), taskControlScope(c), c.Param("id"), version)
	if e != nil {
		taskControlError(c, e)
		return
	}
	OK(c, plan)
}
