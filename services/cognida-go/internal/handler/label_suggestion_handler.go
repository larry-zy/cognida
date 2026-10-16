package handler

import (
	svc "cognida/internal/service/governance"
	"errors"
	"github.com/gin-gonic/gin"
)

func (h *GovernanceHandler) RegisterLabelSuggestionRoutes(api *gin.RouterGroup) {
	for _, kind := range []string{"annotation", "tagging"} {
		path := "/" + kind + "-tasks/:id/"
		if kind == "annotation" {
			path += "items/"
		} else {
			path += "results/"
		}
		api.POST(path+":itemId/suggest", func(c *gin.Context) {
			var in svc.LabelSuggestionRequest
			if !readTaskJSON(c, &in) {
				return
			}
			v, e := h.service.SuggestLabel(c.Request.Context(), gp(c), kind, c.Param("id"), c.Param("itemId"), in, c.GetHeader("Idempotency-Key"))
			if e != nil {
				switch {
				case errors.Is(e, svc.ErrLabelModelUnavailable):
					c.JSON(502, gin.H{"code": 502, "message": e.Error()})
				case errors.Is(e, svc.ErrLabelModelOutput):
					c.JSON(422, gin.H{"code": 422, "message": e.Error()})
				case errors.Is(e, svc.ErrLabelModelUnknown):
					c.JSON(409, gin.H{"code": 409, "message": e.Error()})
				default:
					governanceErr(c, e)
				}
				return
			}
			recordOK(c, v, nil)
		})
	}
}
