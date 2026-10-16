package handler

import (
	d "cognida/internal/model/lifecycle"
	"github.com/gin-gonic/gin"
	"strconv"
)

func (h *LifecycleHandler) RegisterCleaningReview(g *gin.RouterGroup) {
	g.POST("/cleaning/preview", func(c *gin.Context) {
		p, ok := lifecyclePrincipal(c)
		if !ok {
			return
		}
		var in struct {
			InputRef d.RefID        `json:"input_ref"`
			Options  map[string]any `json:"options"`
			Cursor   string         `json:"cursor"`
		}
		if !readTaskJSON(c, &in) {
			return
		}
		out, e := h.service.PreviewCleaning(c.Request.Context(), p, in.InputRef, in.Options, in.Cursor)
		if e != nil {
			h.writeError(c, e)
			return
		}
		c.Header("Cache-Control", "no-store")
		OK(c, out)
	})
	g.GET("/assets/:id/cleaning-diff", func(c *gin.Context) {
		p, ok := lifecyclePrincipal(c)
		if !ok {
			return
		}
		version, e := strconv.Atoi(c.Query("version"))
		if e != nil || version < 1 {
			h.writeError(c, d.ErrInvalid)
			return
		}
		out, e := h.service.CleaningDifferences(c.Request.Context(), p, d.RefID{AssetID: c.Param("id"), Version: version}, c.Query("cursor"))
		if e != nil {
			h.writeError(c, e)
			return
		}
		c.Header("Cache-Control", "no-store")
		OK(c, out)
	})
}
