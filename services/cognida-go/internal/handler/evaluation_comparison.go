package handler

import (
	"cognida/internal/service/evaluation"
	"github.com/gin-gonic/gin"
)

func (h *EvaluationHandler) Compare(c *gin.Context) {
	tenant, user, ok := MustGetTenantAndUserID(c)
	if !ok {
		return
	}
	var in evaluation.ComparisonRequest
	if !BindJSON(c, &in) {
		return
	}
	out, err := h.service.Compare(c.Request.Context(), tenant, user, in)
	if err != nil {
		h.handleError(c, err)
		return
	}
	OK(c, out)
}
