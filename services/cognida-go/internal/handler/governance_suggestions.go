package handler

import (
	d "cognida/internal/model/governance"
	svc "cognida/internal/service/governance"
	"github.com/gin-gonic/gin"
)

func (h *GovernanceHandler) RegisterSuggestionRoutes(api *gin.RouterGroup) {
	api.POST("/improvement-suggestions", h.createSuggestion)
	api.POST("/improvement-suggestions/discover", h.discoverSuggestions)
	api.POST("/improvement-suggestions/:id/review", h.reviewSuggestion)
}
func (h *GovernanceHandler) createSuggestion(c *gin.Context) {
	var in d.Suggestion
	if !BindJSON(c, &in) {
		return
	}
	record, e := h.service.CreateSuggestion(c.Request.Context(), gp(c), in, c.GetHeader("Idempotency-Key"))
	recordOK(c, record, e)
}
func (h *GovernanceHandler) reviewSuggestion(c *gin.Context) {
	var in struct {
		Decision        string `json:"decision"`
		Comment         string `json:"comment"`
		ExpectedVersion int    `json:"expected_version"`
	}
	if !BindJSON(c, &in) {
		return
	}
	record, e := h.service.ReviewSuggestion(c.Request.Context(), gp(c), c.Param("id"), in.Decision, in.Comment, in.ExpectedVersion)
	recordOK(c, record, e)
}
func (h *GovernanceHandler) discoverSuggestions(c *gin.Context) {
	var in struct {
		SourceTypes []string `json:"source_types"`
		Limit       int      `json:"limit"`
	}
	if !BindJSON(c, &in) {
		return
	}
	records, e := h.service.DiscoverSuggestions(c.Request.Context(), gp(c), in.SourceTypes, in.Limit)
	if e != nil {
		governanceErr(c, e)
		return
	}
	items := make([]map[string]any, 0, len(records))
	for _, record := range records {
		view, e := svc.View(record)
		if e != nil {
			governanceErr(c, e)
			return
		}
		items = append(items, view)
	}
	OK(c, gin.H{"items": items})
}
