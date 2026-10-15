package handler

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"strconv"
)

func (h *GovernanceHandler) previewFeature(c *gin.Context) {
	var in struct {
		Feature d.Feature `json:"feature"`
		Limit   int       `json:"limit"`
	}
	if !readTaskJSON(c, &in) {
		return
	}
	result, err := h.service.PreviewFeature(c.Request.Context(), gp(c), in.Feature, in.Limit)
	if err != nil {
		governanceErr(c, err)
		return
	}
	OK(c, result)
}
func (h *GovernanceHandler) getFeatureVersion(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	version, err := strconv.Atoi(c.Param("version"))
	if err != nil || version < 1 {
		governanceErr(c, d.ErrInvalid)
		return
	}
	result, err := h.service.GetFeatureVersion(c.Request.Context(), gp(c), c.Param("id"), version)
	if err != nil {
		governanceErr(c, err)
		return
	}
	OK(c, result)
}
func (h *GovernanceHandler) materializeFeature(c *gin.Context) {
	var in struct {
		Version int `json:"version"`
	}
	if !readTaskJSON(c, &in) {
		return
	}
	result, err := h.service.MaterializeFeature(c.Request.Context(), gp(c), c.Param("id"), in.Version, c.GetHeader("Idempotency-Key"))
	if err != nil {
		governanceErr(c, err)
		return
	}
	OK(c, result)
}
func (h *GovernanceHandler) getGovernedExecution(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	row, err := h.service.Get(c.Request.Context(), gp(c), d.GovernedKind, c.Param("id"))
	if err != nil {
		governanceErr(c, err)
		return
	}
	var state d.GovernedExecution
	if json.Unmarshal(row.Payload, &state) != nil {
		governanceErr(c, d.ErrInvalid)
		return
	}
	if h.service.Assets == nil {
		governanceErr(c, d.ErrForbidden)
		return
	}
	if _, _, err := h.service.Assets.Resolve(c.Request.Context(), row.TenantID, row.UserID, state.Invocation.InputRef); err != nil {
		governanceErr(c, l.ErrForbidden)
		return
	}
	visible := []l.RefID{}
	for _, ref := range state.OutputRefs {
		if _, _, err := h.service.Assets.Resolve(c.Request.Context(), row.TenantID, row.UserID, ref); err == nil {
			visible = append(visible, ref)
		}
	}
	OK(c, gin.H{"id": row.ID, "status": row.Status, "domain_id": state.DomainID, "domain_kind": state.DomainKind, "feature_id": state.Invocation.FeatureID, "feature_version": state.Invocation.FeatureVersion, "output_refs": visible})
}
