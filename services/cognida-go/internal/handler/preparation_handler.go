package handler

import (
	d "cognida/internal/model/lifecycle"
	"github.com/gin-gonic/gin"
)

func (h *LifecycleHandler) Prepare(c *gin.Context) {
	p, ok := lifecyclePrincipal(c)
	if !ok {
		return
	}
	var in struct {
		Version int `json:"version"`
		d.PreparationOptions
	}
	if !readTaskJSON(c, &in) {
		return
	}
	stage, err := h.service.PrepareAsset(c.Request.Context(), p, d.RefID{AssetID: c.Param("id"), Version: in.Version}, in.PreparationOptions, c.GetHeader("Idempotency-Key"), lifecycleRequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	Created(c, stage)
}
