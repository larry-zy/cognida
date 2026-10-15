package handler

import (
	svc "cognida/internal/service/taskcontrol"
	"github.com/gin-gonic/gin"
)

// SetSupervisor is construction-time wiring, before routes begin serving.
func (h *TaskControlHandler) SetSupervisor(supervisor *svc.Supervisor) {
	h.supervisor = supervisor
}
func (h *TaskControlHandler) requireSupervisor(c *gin.Context) bool {
	if h.supervisor == nil {
		taskControlError(c, svc.ErrPlannerUnavailable)
		return false
	}
	return true
}
func (h *TaskControlHandler) AssessTask(c *gin.Context) {
	if !h.requireSupervisor(c) {
		return
	}
	var in svc.AssessmentInput
	if !readTaskJSON(c, &in) {
		return
	}
	result, err := h.supervisor.Assess(c.Request.Context(), taskControlScope(c), in)
	if err != nil {
		taskControlError(c, err)
		return
	}
	OK(c, result)
}
func (h *TaskControlHandler) ProposeTask(c *gin.Context) {
	if !h.requireSupervisor(c) {
		return
	}
	var in svc.ProposeInput
	if !readTaskJSON(c, &in) {
		return
	}
	result, err := h.supervisor.Propose(c.Request.Context(), taskControlScope(c), c.Param("id"), in)
	if err != nil {
		taskControlError(c, err)
		return
	}
	OK(c, result)
}
func (h *TaskControlHandler) SummarizeTask(c *gin.Context) {
	if !h.requireSupervisor(c) {
		return
	}
	result, err := h.supervisor.Summarize(c.Request.Context(), taskControlScope(c), c.Param("id"))
	if err != nil {
		taskControlError(c, err)
		return
	}
	OK(c, result)
}
