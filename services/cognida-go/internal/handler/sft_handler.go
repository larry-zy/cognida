package handler

import (
	d "cognida/internal/model/governance"
	svc "cognida/internal/service/governance"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

type SFTHandler struct{ service *svc.Service }

func NewSFTHandler(s *svc.Service) *SFTHandler { return &SFTHandler{s} }
func (h *SFTHandler) Register(api *gin.RouterGroup) {
	common := NewGovernanceHandler(h.service)
	api.GET("/sft-datasets", common.list(d.SFTDatasetKind, ""))
	api.GET("/sft-datasets/:id", common.get(d.SFTDatasetKind))
	api.GET("/sft-datasets/:id/versions", common.list(d.SFTDatasetVersionKind, "id"))
	api.GET("/sft-dataset-versions/:id", common.get(d.SFTDatasetVersionKind))
	api.POST("/sft-datasets", func(c *gin.Context) {
		var in d.SFTInput
		if !strictGovernanceBody(c, &in) {
			return
		}
		r, e := h.service.CreateSFTDataset(c.Request.Context(), gp(c), in, c.GetHeader("Idempotency-Key"))
		recordOK(c, r, e)
	})
	api.GET("/sft-datasets/:id/rows", func(c *gin.Context) {
		n, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
		r, e := h.service.SFTDatasetRows(c.Request.Context(), gp(c), c.Param("id"), c.Query("cursor"), n)
		if e != nil {
			governanceErr(c, e)
			return
		}
		OK(c, r)
	})
	api.POST("/sft-datasets/:id/review", func(c *gin.Context) {
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			Decision        string `json:"decision"`
			Comment         string `json:"comment"`
		}
		if !strictGovernanceBody(c, &in) {
			return
		}
		r, e := h.service.ReviewSFTDataset(c.Request.Context(), gp(c), c.Param("id"), in.Decision, in.Comment, in.ExpectedVersion)
		recordOK(c, r, e)
	})
	api.POST("/sft-datasets/:id/publish", func(c *gin.Context) {
		var in struct {
			ExpectedVersion int `json:"expected_version"`
		}
		if !strictGovernanceBody(c, &in) {
			return
		}
		r, e := h.service.PublishSFTDataset(c.Request.Context(), gp(c), c.Param("id"), in.ExpectedVersion, c.GetHeader("Idempotency-Key"))
		recordOK(c, r, e)
	})
	api.GET("/sft-dataset-versions/:id/export", func(c *gin.Context) {
		raw, e := h.service.ExportSFTDataset(c.Request.Context(), gp(c), c.Param("id"))
		if e != nil {
			governanceErr(c, e)
			return
		}
		c.Header("Content-Disposition", `attachment; filename="sft-dataset.jsonl"`)
		c.Data(http.StatusOK, "application/x-ndjson; charset=utf-8", raw)
	})
}
