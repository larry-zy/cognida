package handler

import (
	agentctx "cognida/internal/model/agent"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	d "cognida/internal/model/lifecycle"
	svc "cognida/internal/service/lifecycle"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type LifecycleHandler struct{ service *svc.Service }

func NewLifecycleHandler(service *svc.Service) *LifecycleHandler {
	return &LifecycleHandler{service: service}
}
func (h *LifecycleHandler) Register(g *gin.RouterGroup) {
	g.GET("/agents", h.Agents)
	g.GET("/templates", h.Templates)
	g.POST("/assets", h.Import)
	g.GET("/assets", h.Assets)
	g.GET("/assets/:id", h.Asset)
	g.POST("/assets/:id/prepare", h.Prepare)
	g.POST("/invocations", h.Invoke)
	g.GET("/invocations", h.Invocations)
	g.GET("/invocations/:id", h.Invocation)
	g.POST("/invocations/:id/cancel", h.CancelInvocation)
	g.POST("/runs", h.Start)
	g.GET("/runs", h.Runs)
	g.GET("/runs/:id", h.Run)
	g.POST("/runs/:id/cancel", h.Cancel)
	g.POST("/runs/:id/retry", h.Retry)
	g.GET("/runs/:id/events", h.Events)
	g.POST("/approvals/:id/resolve", h.Approve)
}
func lifecyclePrincipal(c *gin.Context) (d.Principal, bool) {
	tenant, user, ok := MustGetTenantAndUserID(c)
	if ok {
		c.Request = c.Request.WithContext(agentctx.WithRequestID(c.Request.Context(), lifecycleRequestID(c)))
	}
	return d.Principal{TenantID: tenant, UserID: user}, ok
}
func (h *LifecycleHandler) writeError(c *gin.Context, e error) {
	switch {
	case errors.Is(e, d.ErrNotFound):
		NotFound(c, e.Error())
	case errors.Is(e, d.ErrForbidden):
		c.JSON(403, gin.H{"code": 403, "message": e.Error()})
	case errors.Is(e, d.ErrInvalid):
		BadRequest(c, e.Error())
	case errors.Is(e, d.ErrConflict), errors.Is(e, d.ErrExpired):
		c.JSON(409, gin.H{"code": 409, "message": e.Error()})
	default:
		c.JSON(500, gin.H{"code": 500, "message": "lifecycle operation failed"})
	}
}
func (h *LifecycleHandler) Agents(c *gin.Context) { OK(c, gin.H{"agents": h.service.Agents()}) }
func (h *LifecycleHandler) Templates(c *gin.Context) {
	OK(c, gin.H{"templates": h.service.Templates()})
}
func (h *LifecycleHandler) Import(c *gin.Context) {
	p, ok := lifecyclePrincipal(c)
	if !ok {
		return
	}
	var req struct {
		Input  *d.Input           `json:"input"`
		Source *svc.SourceRequest `json:"source"`
		Name   string             `json:"name"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 9<<20)
	if !BindJSON(c, &req) {
		return
	}
	if (req.Input == nil) == (req.Source == nil) {
		BadRequest(c, "exactly one input or source is required")
		return
	}
	var ref d.DataAssetRef
	var e error
	if req.Source != nil {
		ref, e = h.service.Acquire(c.Request.Context(), p, *req.Source, req.Name, c.GetHeader("Idempotency-Key"))
	} else {
		ref, e = h.service.ImportWithKey(c.Request.Context(), p, *req.Input, req.Name, c.GetHeader("Idempotency-Key"))
	}
	if e != nil {
		h.writeError(c, e)
		return
	}
	Created(c, ref)
}
func pageOptions(c *gin.Context) (string, int, bool) { return parsedPageOptions(c, false) }
func parsedPageOptions(c *gin.Context, assets bool) (string, int, bool) {
	cursor := c.Query("cursor")
	if cursor != "" {
		var e error
		if assets {
			_, e = d.ParseRefID(cursor)
		} else {
			_, e = uuid.Parse(cursor)
		}
		if e != nil {
			BadRequest(c, "invalid cursor")
			return "", 0, false
		}
	}
	n := 50
	if raw := c.Query("page_size"); raw != "" {
		v, e := strconv.Atoi(raw)
		if e != nil || v < 1 || v > 100 {
			BadRequest(c, "page_size must be 1-100")
			return "", 0, false
		}
		n = v
	}
	return cursor, n, true
}
func (h *LifecycleHandler) Assets(c *gin.Context) {
	p, ok := lifecyclePrincipal(c)
	if !ok {
		return
	}
	cursor, n, ok := parsedPageOptions(c, true)
	if !ok {
		return
	}
	items, e := h.service.Repository().ListAssets(c.Request.Context(), p.TenantID, d.Kind(c.Query("kind")), cursor, n+1)
	if e != nil {
		h.writeError(c, e)
		return
	}
	more := len(items) > n
	if more {
		items = items[:n]
	}
	next := ""
	if more {
		next = items[len(items)-1].RefID.String()
	}
	OK(c, gin.H{"items": items, "next_cursor": next, "has_more": more})
}
func (h *LifecycleHandler) Asset(c *gin.Context) {
	p, ok := lifecyclePrincipal(c)
	if !ok {
		return
	}
	v, e := strconv.Atoi(c.DefaultQuery("version", "1"))
	if e != nil {
		BadRequest(c, "invalid version")
		return
	}
	ref, content, e := h.service.Resolve(c.Request.Context(), p.TenantID, p.UserID, d.RefID{AssetID: c.Param("id"), Version: v})
	if e != nil {
		h.writeError(c, e)
		return
	}
	OK(c, gin.H{"ref": ref, "content": content})
}

type lifecycleStartRequest struct {
	AgentID    string         `json:"agent_id"`
	TemplateID string         `json:"template_id"`
	InputRefs  []d.RefID      `json:"input_refs"`
	Options    map[string]any `json:"options"`
}

func (h *LifecycleHandler) Invoke(c *gin.Context) {
	p, ok := lifecyclePrincipal(c)
	if !ok {
		return
	}
	var req lifecycleStartRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128<<10)
	if !BindJSON(c, &req) {
		return
	}
	st, e := h.service.Invoke(c.Request.Context(), p, req.AgentID, req.InputRefs, req.Options, c.GetHeader("Idempotency-Key"), lifecycleRequestID(c))
	if e != nil {
		h.writeError(c, e)
		return
	}
	Created(c, st)
}
func (h *LifecycleHandler) Invocation(c *gin.Context) {
	p, ok := lifecyclePrincipal(c)
	if !ok {
		return
	}
	st, e := h.service.Repository().FindStage(c.Request.Context(), p.TenantID, c.Param("id"), false)
	if e != nil {
		h.writeError(c, e)
		return
	}
	approvals, e := h.service.Repository().ListApprovals(c.Request.Context(), p.TenantID, st.RunID)
	if e != nil {
		h.writeError(c, e)
		return
	}
	own := []d.Approval{}
	for _, a := range approvals {
		if a.StageID == st.ID {
			own = append(own, a)
		}
	}
	OK(c, gin.H{"invocation": st, "approvals": own})
}
func (h *LifecycleHandler) Invocations(c *gin.Context) {
	p, ok := lifecyclePrincipal(c)
	if !ok {
		return
	}
	cursor, n, ok := pageOptions(c)
	if !ok {
		return
	}
	items, e := h.service.Repository().ListInvocationsPage(c.Request.Context(), p.TenantID, cursor, n+1)
	if e != nil {
		h.writeError(c, e)
		return
	}
	more := len(items) > n
	if more {
		items = items[:n]
	}
	next := ""
	if more {
		next = items[len(items)-1].ID
	}
	OK(c, gin.H{"items": items, "next_cursor": next, "has_more": more})
}
func (h *LifecycleHandler) Start(c *gin.Context) {
	p, ok := lifecyclePrincipal(c)
	if !ok {
		return
	}
	var req lifecycleStartRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128<<10)
	if !BindJSON(c, &req) {
		return
	}
	run, e := h.service.StartRun(c.Request.Context(), p, req.TemplateID, req.InputRefs, req.Options, c.GetHeader("Idempotency-Key"), lifecycleRequestID(c))
	if e != nil {
		h.writeError(c, e)
		return
	}
	Created(c, run)
}
func (h *LifecycleHandler) Runs(c *gin.Context) {
	p, ok := lifecyclePrincipal(c)
	if !ok {
		return
	}
	cursor, n, ok := pageOptions(c)
	if !ok {
		return
	}
	items, e := h.service.Repository().ListRunsPage(c.Request.Context(), p.TenantID, cursor, n+1)
	if e != nil {
		h.writeError(c, e)
		return
	}
	more := len(items) > n
	if more {
		items = items[:n]
	}
	next := ""
	if more {
		next = items[len(items)-1].ID
	}
	OK(c, gin.H{"items": items, "next_cursor": next, "has_more": more})
}
func (h *LifecycleHandler) Run(c *gin.Context) {
	p, ok := lifecyclePrincipal(c)
	if !ok {
		return
	}
	run, e := h.service.Repository().FindRun(c.Request.Context(), p.TenantID, c.Param("id"), false)
	if e != nil {
		h.writeError(c, e)
		return
	}
	stages, e := h.service.Repository().ListStages(c.Request.Context(), p.TenantID, run.ID)
	if e != nil {
		h.writeError(c, e)
		return
	}
	approvals, e := h.service.Repository().ListApprovals(c.Request.Context(), p.TenantID, run.ID)
	if e != nil {
		h.writeError(c, e)
		return
	}
	OK(c, gin.H{"run": run, "stages": stages, "approvals": approvals})
}
func (h *LifecycleHandler) CancelInvocation(c *gin.Context) {
	p, ok := lifecyclePrincipal(c)
	if !ok {
		return
	}
	if e := h.service.CancelStage(c.Request.Context(), p, c.Param("id")); e != nil {
		h.writeError(c, e)
		return
	}
	OK(c, gin.H{"status": "cancelled"})
}
func (h *LifecycleHandler) Cancel(c *gin.Context) {
	p, ok := lifecyclePrincipal(c)
	if !ok {
		return
	}
	if e := h.service.CancelRun(c.Request.Context(), p, c.Param("id")); e != nil {
		h.writeError(c, e)
		return
	}
	OK(c, gin.H{"status": "cancelled"})
}
func (h *LifecycleHandler) Retry(c *gin.Context) {
	p, ok := lifecyclePrincipal(c)
	if !ok {
		return
	}
	if e := h.service.RetryRun(c.Request.Context(), p, c.Param("id")); e != nil {
		h.writeError(c, e)
		return
	}
	OK(c, gin.H{"status": "running"})
}
func (h *LifecycleHandler) Approve(c *gin.Context) {
	p, ok := lifecyclePrincipal(c)
	if !ok {
		return
	}
	var req struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if !BindJSON(c, &req) {
		return
	}
	if len(req.Reason) > 2000 {
		BadRequest(c, "reason too long")
		return
	}
	if e := h.service.ResolveApproval(c.Request.Context(), p, c.Param("id"), req.Decision, req.Reason); e != nil {
		h.writeError(c, e)
		return
	}
	OK(c, gin.H{"status": req.Decision})
}
func (h *LifecycleHandler) Events(c *gin.Context) {
	p, ok := lifecyclePrincipal(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if _, e := h.service.Repository().FindRun(c.Request.Context(), p.TenantID, id, false); e != nil {
		h.writeError(c, e)
		return
	}
	after := int64(0)
	if raw := c.GetHeader("Last-Event-ID"); raw != "" {
		v, e := strconv.ParseInt(raw, 10, 64)
		if e != nil || v < 0 {
			BadRequest(c, "invalid Last-Event-ID")
			return
		}
		after = v
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		events, e := h.service.Repository().Events(c.Request.Context(), p.TenantID, id, after)
		if e != nil {
			return
		}
		for _, event := range events {
			fmt.Fprintf(c.Writer, "id: %d\n", event.ID)
			c.SSEvent(event.Type, event)
			after = event.ID
		}
		c.Writer.Flush()
		run, e := h.service.Repository().FindRun(c.Request.Context(), p.TenantID, id, false)
		if e != nil {
			return
		}
		if d.Terminal(run.Status) && len(events) < 200 {
			return
		}
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func lifecycleRequestID(c *gin.Context) string {
	if id := c.GetString("request_id"); id != "" {
		return id
	}
	if id := c.GetHeader("X-Request-ID"); id != "" {
		return id
	}
	return uuid.NewString()
}
