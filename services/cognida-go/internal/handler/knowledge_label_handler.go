package handler

import (
	d "cognida/internal/model/governance"
	"github.com/gin-gonic/gin"
	"strconv"
)

func (h *GovernanceHandler) RegisterKnowledgeLabelRoutes(api *gin.RouterGroup) {
	api.GET("/tagging-results/:id/apply-target", func(c *gin.Context) {
		version, err := strconv.Atoi(c.Query("expected_version"))
		if err != nil {
			governanceErr(c, d.ErrInvalid)
			return
		}
		target, err := h.service.KnowledgeLabelPublicationTarget(c.Request.Context(), gp(c), c.Param("id"), version, c.Query("publication_ref_id"))
		if err != nil {
			governanceErr(c, err)
			return
		}
		OK(c, target)
	})
	api.POST("/tagging-results/:id/apply", func(c *gin.Context) {
		var in d.ApplyKnowledgeLabels
		if !BindJSON(c, &in) {
			return
		}
		in.ResultID = c.Param("id")
		v, e := h.service.ApplyKnowledgeLabels(c.Request.Context(), gp(c), in, c.GetHeader("Idempotency-Key"))
		if e != nil {
			governanceErr(c, e)
			return
		}
		OK(c, v)
	})
	api.GET("/knowledge/:id/applied-labels", func(c *gin.Context) {
		v, e := h.service.KnowledgeLabels(c.Request.Context(), gp(c), c.Param("id"))
		if e != nil {
			governanceErr(c, e)
			return
		}
		OK(c, gin.H{"labels": v})
	})
}
