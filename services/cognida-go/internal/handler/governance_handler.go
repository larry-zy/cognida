package handler

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	svc "cognida/internal/service/governance"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"time"
)

type GovernanceHandler struct{ service *svc.Service }

func NewGovernanceHandler(s *svc.Service) *GovernanceHandler { return &GovernanceHandler{s} }
func gp(c *gin.Context) l.Principal {
	return l.Principal{TenantID: GetTenantID(c), UserID: GetUserID(c)}
}
func governanceErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, d.ErrNotFound), errors.Is(err, l.ErrNotFound):
		NotFound(c, "记录不存在")
	case errors.Is(err, d.ErrForbidden), errors.Is(err, l.ErrForbidden):
		Forbidden(c, "没有该操作权限")
	case errors.Is(err, d.ErrInvalid), errors.Is(err, l.ErrInvalid):
		BadRequest(c, err.Error())
	case errors.Is(err, d.ErrConflict), errors.Is(err, l.ErrConflict):
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "记录已变更，请刷新后重试"})
	default:
		InternalError(c, "操作失败，请检查来源资产与处理服务")
	}
}
func recordOK(c *gin.Context, r *d.Record, err error) {
	if err != nil {
		governanceErr(c, err)
		return
	}
	view, e := svc.View(r)
	if e != nil {
		governanceErr(c, e)
		return
	}
	OK(c, view)
}
func (h *GovernanceHandler) Register(api *gin.RouterGroup) {
	api = api.Group("")
	api.Use(func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<20)
		}
		c.Next()
	})
	for _, route := range []struct{ path, kind string }{{"subscriptions", "subscription"}, {"subscription-runs", "subscription_run"}, {"annotation-tasks", "annotation_task"}, {"tagging-tasks", "tagging_task"}, {"features", "feature"}, {"improvement-suggestions", "suggestion"}} {
		kind := route.kind
		api.GET("/"+route.path, h.list(kind, ""))
		api.GET("/"+route.path+"/:id", h.get(kind))
	}
	api.POST("/subscriptions", h.saveSubscription)
	api.PUT("/subscriptions/:id", h.saveSubscription)
	api.POST("/subscriptions/:id/run", h.runSubscription)
	api.GET("/subscriptions/:id/runs", h.list("subscription_run", "id"))
	api.GET("/notifications", h.list("notification", ""))
	api.POST("/notifications/:id/read", func(c *gin.Context) {
		v, e := h.service.ReadNotification(c.Request.Context(), gp(c), c.Param("id"))
		recordOK(c, v, e)
	})
	api.POST("/annotation-tasks", h.createAnnotation)
	api.GET("/annotation-tasks/:id/items", h.list("annotation_item", "id"))
	api.POST("/annotation-tasks/:id/preannotate", h.preannotate)
	api.PUT("/annotation-tasks/:id/items/:itemId", h.updateAnnotation)
	api.POST("/annotation-tasks/:id/items/:itemId/review", h.reviewAnnotation)
	api.GET("/annotation-tasks/:id/export", h.exportAnnotations)
	api.POST("/tagging-tasks", h.createTagging)
	api.GET("/tagging-tasks/:id/results", h.list("tagging_result", "id"))
	api.PUT("/tagging-tasks/:id/results/:itemId", h.updateTagging)
	api.POST("/tagging-results/:id/review", h.reviewTagging)
	api.POST("/features", h.saveFeature)
	api.POST("/features/preview", h.previewFeature)
	api.GET("/features/:id/versions/:version", h.getFeatureVersion)
	api.POST("/features/:id/materialize", h.materializeFeature)
	api.GET("/governance-executions/:id", h.getGovernedExecution)
	api.GET("/features/:id/versions", h.list("feature_version", "id"))
	api.POST("/features/:id/versions", h.saveFeature)
	api.POST("/features/:id/compute", h.computeFeature)
	api.POST("/feature-values", h.writeFeatures)
	api.POST("/feature-values/query", h.queryFeatures)
	h.RegisterSuggestionRoutes(api)
}
func (h *GovernanceHandler) get(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		v, e := h.service.Get(c.Request.Context(), gp(c), kind, c.Param("id"))
		recordOK(c, v, e)
	}
}
func (h *GovernanceHandler) list(kind, parent string) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
		status := c.Query("status")
		if c.Query("unread") == "true" {
			status = "unread"
		}
		filter := d.Filter{Kind: kind, Status: status, Cursor: c.Query("cursor"), Limit: limit}
		if parent != "" {
			filter.ParentID = c.Param(parent)
		}
		page, e := h.service.List(c.Request.Context(), gp(c), filter)
		if e != nil {
			governanceErr(c, e)
			return
		}
		items := []map[string]any{}
		for _, r := range page.Items {
			v, e := svc.View(&r)
			if e != nil {
				governanceErr(c, e)
				return
			}
			items = append(items, v)
		}
		OK(c, d.Page[map[string]any]{Items: items, NextCursor: page.NextCursor, HasMore: page.HasMore})
	}
}
func (h *GovernanceHandler) saveSubscription(c *gin.Context) {
	var in struct {
		d.Subscription
		ExpectedVersion int `json:"expected_version"`
	}
	if !BindJSON(c, &in) {
		return
	}
	v, e := h.service.SaveSubscription(c.Request.Context(), gp(c), c.Param("id"), c.GetHeader("Idempotency-Key"), in.Subscription, in.ExpectedVersion)
	recordOK(c, v, e)
}
func (h *GovernanceHandler) runSubscription(c *gin.Context) {
	key := c.GetHeader("Idempotency-Key")
	if key == "" {
		BadRequest(c, "Idempotency-Key 必填")
		return
	}
	v, e := h.service.RunSubscription(c.Request.Context(), gp(c), c.Param("id"), key)
	recordOK(c, v, e)
}
func (h *GovernanceHandler) createAnnotation(c *gin.Context) {
	var in d.AnnotationTask
	if !BindJSON(c, &in) {
		return
	}
	v, e := h.service.CreateAnnotation(c.Request.Context(), gp(c), in, c.GetHeader("Idempotency-Key"))
	recordOK(c, v, e)
}
func (h *GovernanceHandler) preannotate(c *gin.Context) {
	if e := h.service.Preannotate(c.Request.Context(), gp(c), c.Param("id")); e != nil {
		governanceErr(c, e)
		return
	}
	OK(c, gin.H{"status": "completed"})
}
func (h *GovernanceHandler) updateAnnotation(c *gin.Context) {
	var in struct {
		Annotation      d.Annotation `json:"annotation"`
		ExpectedVersion int          `json:"expected_version"`
	}
	if !BindJSON(c, &in) {
		return
	}
	v, e := h.service.UpdateAnnotation(c.Request.Context(), gp(c), c.Param("id"), c.Param("itemId"), in.Annotation, in.ExpectedVersion)
	recordOK(c, v, e)
}
func (h *GovernanceHandler) reviewAnnotation(c *gin.Context) {
	var in struct {
		Decision        string `json:"decision"`
		Comment         string `json:"comment"`
		ExpectedVersion int    `json:"expected_version"`
	}
	if !BindJSON(c, &in) {
		return
	}
	v, e := h.service.ReviewAnnotation(c.Request.Context(), gp(c), c.Param("id"), c.Param("itemId"), in.Decision, in.Comment, in.ExpectedVersion)
	recordOK(c, v, e)
}
func (h *GovernanceHandler) exportAnnotations(c *gin.Context) {
	if format := c.DefaultQuery("format", "jsonl"); format != "jsonl" {
		BadRequest(c, "只支持 jsonl 导出")
		return
	}
	if _, err := h.service.Get(c.Request.Context(), gp(c), "annotation_task", c.Param("id")); err != nil {
		governanceErr(c, err)
		return
	}
	page, err := h.service.List(c.Request.Context(), gp(c), d.Filter{Kind: "annotation_item", ParentID: c.Param("id"), Status: "approved", Limit: 100})
	if err != nil {
		governanceErr(c, err)
		return
	}
	c.Header("Content-Type", "application/x-ndjson")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=annotations-%s.jsonl", c.Param("id")))
	enc := json.NewEncoder(c.Writer)
	for {
		for _, r := range page.Items {
			value, e := svc.View(&r)
			if e != nil || enc.Encode(value) != nil {
				return
			}
		}
		if !page.HasMore {
			return
		}
		page, err = h.service.List(c.Request.Context(), gp(c), d.Filter{Kind: "annotation_item", ParentID: c.Param("id"), Status: "approved", Limit: 100, Cursor: page.NextCursor})
		if err != nil {
			return
		}
	}
}
func (h *GovernanceHandler) createTagging(c *gin.Context) {
	var in d.TaggingTask
	if !BindJSON(c, &in) {
		return
	}
	v, e := h.service.CreateTagging(c.Request.Context(), gp(c), in, c.GetHeader("Idempotency-Key"))
	recordOK(c, v, e)
}
func (h *GovernanceHandler) updateTagging(c *gin.Context) {
	var in struct {
		d.TaggingResult
		ExpectedVersion int `json:"expected_version"`
	}
	if !BindJSON(c, &in) {
		return
	}
	v, e := h.service.UpdateTagging(c.Request.Context(), gp(c), c.Param("id"), c.Param("itemId"), in.TaggingResult, in.ExpectedVersion)
	recordOK(c, v, e)
}
func (h *GovernanceHandler) saveFeature(c *gin.Context) {
	var in struct {
		d.Feature
		ExpectedVersion int `json:"expected_version"`
	}
	if !BindJSON(c, &in) {
		return
	}
	var v *d.Record
	var e error
	if c.Param("id") == "" {
		v, e = h.service.CreateFeature(c.Request.Context(), gp(c), in.Feature, c.GetHeader("Idempotency-Key"))
	} else {
		v, e = h.service.NewFeatureVersion(c.Request.Context(), gp(c), c.Param("id"), in.Feature, in.ExpectedVersion)
	}
	recordOK(c, v, e)
}
func (h *GovernanceHandler) computeFeature(c *gin.Context) {
	var in struct {
		Version int `json:"version"`
	}
	if !BindJSON(c, &in) {
		return
	}
	n, e := h.service.ComputeFeatures(c.Request.Context(), gp(c), c.Param("id"), in.Version)
	if e != nil {
		governanceErr(c, e)
		return
	}
	OK(c, gin.H{"count": n})
}
func (h *GovernanceHandler) writeFeatures(c *gin.Context) {
	var in struct {
		Values []d.FeatureValue `json:"values"`
	}
	if !BindJSON(c, &in) {
		return
	}
	if e := h.service.WriteFeatures(c.Request.Context(), gp(c), in.Values); e != nil {
		governanceErr(c, e)
		return
	}
	OK(c, gin.H{"count": len(in.Values)})
}
func (h *GovernanceHandler) queryFeatures(c *gin.Context) {
	var in struct {
		EntityIDs []string         `json:"entity_ids"`
		Features  []d.FeatureValue `json:"features"`
		AsOf      time.Time        `json:"as_of"`
	}
	if !BindJSON(c, &in) {
		return
	}
	v, e := h.service.QueryFeatures(c.Request.Context(), gp(c), in.EntityIDs, in.Features, in.AsOf)
	if e != nil {
		governanceErr(c, e)
		return
	}
	OK(c, gin.H{"items": v})
}

func (h *GovernanceHandler) reviewTagging(c *gin.Context) {
	var in struct {
		Decision        string `json:"decision"`
		Comment         string `json:"comment"`
		ExpectedVersion int    `json:"expected_version"`
	}
	if !BindJSON(c, &in) {
		return
	}
	v, e := h.service.ReviewTagging(c.Request.Context(), gp(c), c.Param("id"), in.Decision, in.Comment, in.ExpectedVersion)
	recordOK(c, v, e)
}
