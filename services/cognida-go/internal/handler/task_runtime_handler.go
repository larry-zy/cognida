package handler

import (
	svc "cognida/internal/service/taskcontrol"
	"github.com/gin-gonic/gin"
)

func (h *TaskControlHandler) SetRuntime(runtime *svc.Runtime) { h.runtime = runtime }
func (h *TaskControlHandler) requireRuntime(c *gin.Context) bool {
	if h.runtime == nil {
		c.JSON(503, gin.H{"code": 503, "message": "任务执行服务未接入"})
		return false
	}
	return true
}
func (h *TaskControlHandler) ExecutionReadiness(c *gin.Context) {
	if !h.requireRuntime(c) {
		return
	}
	result, err := h.runtime.Readiness(c.Request.Context(), taskControlScope(c), c.Param("id"))
	if err != nil {
		taskControlError(c, err)
		return
	}
	OK(c, result)
}
func (h *TaskControlHandler) StartTask(c *gin.Context) {
	if !h.requireRuntime(c) {
		return
	}
	var in svc.StartInput
	if !readTaskJSON(c, &in) {
		return
	}
	result, err := h.runtime.Start(c.Request.Context(), taskControlScope(c), c.Param("id"), in, c.GetHeader("Idempotency-Key"))
	if err != nil {
		taskControlError(c, err)
		return
	}
	OK(c, result)
}
func (h *TaskControlHandler) CancelTask(c *gin.Context) {
	if !h.requireRuntime(c) {
		return
	}
	var in svc.CancelInput
	if !readTaskJSON(c, &in) {
		return
	}
	result, err := h.runtime.Cancel(c.Request.Context(), taskControlScope(c), c.Param("id"), in)
	if err != nil {
		taskControlError(c, err)
		return
	}
	OK(c, result)
}
