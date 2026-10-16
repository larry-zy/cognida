package handler

import (
	d "cognida/internal/model/governance"
	svc "cognida/internal/service/governance"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
)

type LabelStandardHandler struct{ service *svc.Service }

func NewLabelStandardHandler(s *svc.Service) *LabelStandardHandler { return &LabelStandardHandler{s} }
func strictGovernanceBody(c *gin.Context, v any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<20)
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(v) != nil {
		BadRequest(c, "请求字段不合法")
		return false
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		BadRequest(c, "请求字段不合法")
		return false
	}
	return true
}
func (h *LabelStandardHandler) Register(api *gin.RouterGroup) {
	common := NewGovernanceHandler(h.service)
	api.GET("/label-standards", common.list(d.LabelStandardKind, ""))
	api.GET("/label-standards/:id", common.get(d.LabelStandardKind))
	api.GET("/label-standards/:id/versions", common.list(d.LabelStandardVersionKind, "id"))
	api.GET("/label-standard-versions/:id", common.get(d.LabelStandardVersionKind))
	api.POST("/label-standards", func(c *gin.Context) {
		var in d.LabelStandardInput
		if !strictGovernanceBody(c, &in) {
			return
		}
		r, e := h.service.CreateLabelStandard(c.Request.Context(), gp(c), in, c.GetHeader("Idempotency-Key"))
		recordOK(c, r, e)
	})
	api.PUT("/label-standards/:id", func(c *gin.Context) {
		var in struct {
			ExpectedVersion int                  `json:"expected_version"`
			Standard        d.LabelStandardInput `json:"standard"`
		}
		if !strictGovernanceBody(c, &in) {
			return
		}
		r, e := h.service.UpdateLabelStandard(c.Request.Context(), gp(c), c.Param("id"), in.Standard, in.ExpectedVersion)
		recordOK(c, r, e)
	})
	api.POST("/label-standards/:id/publish", func(c *gin.Context) {
		var in struct {
			ExpectedVersion int `json:"expected_version"`
		}
		if !strictGovernanceBody(c, &in) {
			return
		}
		r, e := h.service.PublishLabelStandard(c.Request.Context(), gp(c), c.Param("id"), in.ExpectedVersion, c.GetHeader("Idempotency-Key"))
		recordOK(c, r, e)
	})
}
