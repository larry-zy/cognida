package handler

import (
	agentctx "cognida/internal/model/agent"
	d "cognida/internal/model/governance"
	"github.com/gin-gonic/gin"
	"strconv"
)

func (h *GovernanceHandler) RegisterFeatureOperations(api *gin.RouterGroup) {
	api = api.Group("")
	api.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(agentctx.WithRequestID(c.Request.Context(), lifecycleRequestID(c)))
		c.Next()
	})
	api.GET("/feature-models", func(c *gin.Context) { c.Header("Cache-Control", "no-store"); OK(c, h.service.FeatureEmbeddingModel()) })
	api.GET("/features/:id/versions/:version/requirements", h.featureRequirements)
	api.POST("/feature-jobs", h.createFeatureJob)
	api.GET("/feature-jobs", h.listFeatureJobs)
	api.GET("/feature-jobs/:id", h.getFeatureJob)
	api.POST("/feature-jobs/:id/:action", h.controlFeatureJob)
	api.POST("/drift/baselines", h.createDriftBaseline)
	api.GET("/drift/records", h.listDrift)
	api.GET("/drift/records/:id", h.getDrift)
	api.GET("/drift/records/:id/events", h.driftEvents)
	api.POST("/drift/baselines/:id/approve", h.approveDriftBaseline)
	api.POST("/drift/compare", h.compareDrift)
	api.POST("/drift/monitors", h.createDriftMonitor)
	api.POST("/drift/monitors/:id/:action", h.updateDriftMonitor)
	api.POST("/drift/reports/:id/remediation", h.driftRemediation)
}
func (h *GovernanceHandler) createFeatureJob(c *gin.Context) {
	var in d.FeatureJobRequest
	if !readTaskJSON(c, &in) {
		return
	}
	v, e := h.service.CreateFeatureJob(c.Request.Context(), gp(c), in, c.GetHeader("Idempotency-Key"))
	if e != nil {
		governanceErr(c, e)
		return
	}
	OK(c, v)
}
func (h *GovernanceHandler) listFeatureJobs(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	v, next, e := h.service.ListFeatureJobs(c.Request.Context(), gp(c), c.Query("cursor"))
	if e != nil {
		governanceErr(c, e)
		return
	}
	OK(c, gin.H{"items": v, "next_cursor": next})
}
func (h *GovernanceHandler) getFeatureJob(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	v, e := h.service.GetFeatureJob(c.Request.Context(), gp(c), c.Param("id"))
	if e != nil {
		governanceErr(c, e)
		return
	}
	OK(c, v)
}
func (h *GovernanceHandler) controlFeatureJob(c *gin.Context) {
	var in struct {
		ExpectedVersion int `json:"expected_version"`
	}
	if !readTaskJSON(c, &in) {
		return
	}
	v, e := h.service.ControlFeatureJob(c.Request.Context(), gp(c), c.Param("id"), c.Param("action"), in.ExpectedVersion)
	if e != nil {
		governanceErr(c, e)
		return
	}
	OK(c, v)
}
func (h *GovernanceHandler) createDriftBaseline(c *gin.Context) {
	var in d.DriftBaseline
	if !readTaskJSON(c, &in) {
		return
	}
	v, e := h.service.CreateDriftBaseline(c.Request.Context(), gp(c), in, c.GetHeader("Idempotency-Key"))
	if e != nil {
		governanceErr(c, e)
		return
	}
	OK(c, v)
}
func (h *GovernanceHandler) getDrift(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	v, e := h.service.GetDrift(c.Request.Context(), gp(c), c.Param("id"))
	if e != nil {
		governanceErr(c, e)
		return
	}
	OK(c, v)
}
func (h *GovernanceHandler) listDrift(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	v, next, e := h.service.ListDrift(c.Request.Context(), gp(c), c.Query("kind"), c.Query("cursor"))
	if e != nil {
		governanceErr(c, e)
		return
	}
	OK(c, gin.H{"items": v, "next_cursor": next})
}
func (h *GovernanceHandler) approveDriftBaseline(c *gin.Context) {
	var in struct {
		ExpectedVersion int `json:"expected_version"`
	}
	if !readTaskJSON(c, &in) {
		return
	}
	v, e := h.service.ApproveDriftBaseline(c.Request.Context(), gp(c), c.Param("id"), in.ExpectedVersion)
	if e != nil {
		governanceErr(c, e)
		return
	}
	OK(c, v)
}
func (h *GovernanceHandler) compareDrift(c *gin.Context) {
	var in struct {
		BaselineID   string  `json:"baseline_id"`
		CurrentRefID string  `json:"current_ref_id"`
		Threshold    float64 `json:"threshold"`
	}
	if !readTaskJSON(c, &in) {
		return
	}
	v, e := h.service.CompareDrift(c.Request.Context(), gp(c), in.BaselineID, in.CurrentRefID, in.Threshold)
	if e != nil {
		governanceErr(c, e)
		return
	}
	OK(c, v)
}
func (h *GovernanceHandler) createDriftMonitor(c *gin.Context) {
	var in d.DriftMonitor
	if !readTaskJSON(c, &in) {
		return
	}
	v, e := h.service.CreateDriftMonitor(c.Request.Context(), gp(c), in, c.GetHeader("Idempotency-Key"))
	if e != nil {
		governanceErr(c, e)
		return
	}
	OK(c, v)
}
func (h *GovernanceHandler) updateDriftMonitor(c *gin.Context) {
	var in struct {
		ExpectedVersion int    `json:"expected_version"`
		BaselineID      string `json:"baseline_id"`
	}
	if !readTaskJSON(c, &in) {
		return
	}
	v, e := h.service.UpdateDriftMonitor(c.Request.Context(), gp(c), c.Param("id"), c.Param("action"), in.BaselineID, in.ExpectedVersion)
	if e != nil {
		governanceErr(c, e)
		return
	}
	OK(c, v)
}
func (h *GovernanceHandler) driftEvents(c *gin.Context) {
	v, e := h.service.DriftAudit(c.Request.Context(), gp(c), c.Param("id"))
	if e != nil {
		governanceErr(c, e)
		return
	}
	OK(c, v)
}
func (h *GovernanceHandler) driftRemediation(c *gin.Context) {
	var in struct {
		ExpectedVersion int `json:"expected_version"`
	}
	if !readTaskJSON(c, &in) {
		return
	}
	v, e := h.service.CreateDriftRemediation(c.Request.Context(), gp(c), c.Param("id"), in.ExpectedVersion)
	if e != nil {
		governanceErr(c, e)
		return
	}
	OK(c, v)
}

func (h *GovernanceHandler) featureRequirements(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	version, e := strconv.Atoi(c.Param("version"))
	if e != nil || version < 1 {
		governanceErr(c, d.ErrInvalid)
		return
	}
	calls, embedding, e := h.service.FeatureExecutionRequirementsForSource(c.Request.Context(), gp(c), c.Param("id"), version, c.Query("source_ref_id"))
	if e != nil {
		governanceErr(c, e)
		return
	}
	OK(c, gin.H{"feature_id": c.Param("id"), "version": version, "max_calls_per_attempt": calls, "embedding": embedding, "max_attempts": 1, "cost_limit_enforced": false, "requires_zero_cost_reservation": embedding})
}
