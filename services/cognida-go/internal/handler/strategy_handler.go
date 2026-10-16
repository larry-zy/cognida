package handler

import (
	d "cognida/internal/model/strategy"
	svc "cognida/internal/service/strategy"
	"errors"
	"github.com/gin-gonic/gin"
	"strconv"
)

type StrategyHandler struct{ service *svc.Service }

func NewStrategyHandler(s *svc.Service) *StrategyHandler { return &StrategyHandler{s} }
func strategyError(c *gin.Context, e error) {
	switch {
	case errors.Is(e, d.ErrInvalid):
		BadRequest(c, "策略参数无效")
	case errors.Is(e, d.ErrForbidden):
		Forbidden(c, "策略或来源当前不可访问")
	case errors.Is(e, d.ErrNotFound):
		NotFound(c, "策略记录不存在")
	case errors.Is(e, d.ErrConflict), errors.Is(e, d.ErrGate):
		c.JSON(409, gin.H{"code": 409, "message": e.Error()})
	case errors.Is(e, d.ErrUnavailable):
		c.JSON(503, gin.H{"code": 503, "message": "该策略没有接通真实消费者或回归读取器"})
	default:
		InternalError(c, "策略操作失败")
	}
}
func (h *StrategyHandler) Register(api *gin.RouterGroup) {
	g := api.Group("/strategies")
	g.Use(func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() })
	g.GET("/consumers", func(c *gin.Context) {
		if _, e := h.service.Head(c.Request.Context(), gp(c), "readiness"); errors.Is(e, d.ErrForbidden) {
			strategyError(c, e)
			return
		}
		OK(c, h.service.ConsumerInfo())
	})
	g.GET("/:slot", func(c *gin.Context) {
		v, e := h.service.Head(c.Request.Context(), gp(c), c.Param("slot"))
		if e != nil {
			strategyError(c, e)
			return
		}
		OK(c, v)
	})
	g.GET("/:slot/versions", func(c *gin.Context) {
		limit, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
		v, e := h.service.List(c.Request.Context(), gp(c), c.Param("slot"), c.Query("cursor"), limit)
		if e != nil {
			strategyError(c, e)
			return
		}
		OK(c, v)
	})
	g.GET("/:slot/events", func(c *gin.Context) {
		limit, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
		v, e := h.service.Events(c.Request.Context(), gp(c), c.Param("slot"), c.Query("kind"), c.Query("cursor"), limit)
		if e != nil {
			strategyError(c, e)
			return
		}
		OK(c, v)
	})
	g.POST("/:slot/baseline", func(c *gin.Context) {
		var in struct {
			Kind string `json:"kind"`
		}
		if !readTaskJSON(c, &in) {
			return
		}
		v, e := h.service.RegisterBaseline(c.Request.Context(), gp(c), c.Param("slot"), in.Kind, c.GetHeader("Idempotency-Key"))
		if e != nil {
			strategyError(c, e)
			return
		}
		OK(c, v)
	})
	g.POST("/:slot/versions", func(c *gin.Context) {
		var in svc.CandidateInput
		if !readTaskJSON(c, &in) {
			return
		}
		v, e := h.service.CreateCandidate(c.Request.Context(), gp(c), c.Param("slot"), in, c.GetHeader("Idempotency-Key"))
		if e != nil {
			strategyError(c, e)
			return
		}
		OK(c, v)
	})
	g.GET("/:slot/versions/:id", func(c *gin.Context) {
		v, e := h.service.Read(c.Request.Context(), gp(c), c.Param("id"))
		if e == nil && v.Slot != c.Param("slot") {
			e = d.ErrNotFound
		}
		if e != nil {
			strategyError(c, e)
			return
		}
		OK(c, v)
	})
	g.POST("/:slot/versions/:id/trials", func(c *gin.Context) {
		var in struct {
			TaskID string `json:"task_id"`
		}
		if !readTaskJSON(c, &in) {
			return
		}
		if c.Param("slot") != "orchestration.default" || !d.ValidKey(c.GetHeader("Idempotency-Key")) {
			strategyError(c, d.ErrInvalid)
			return
		}
		v, e := h.service.PrepareOrchestrationTrial(c.Request.Context(), gp(c), c.Param("id"), in.TaskID, c.GetHeader("Idempotency-Key"))
		if e != nil {
			strategyError(c, e)
			return
		}
		OK(c, gin.H{"snapshot": v, "saved": true, "execution_started": false})
	})
	g.POST("/:slot/versions/:id/regressions", func(c *gin.Context) {
		var in svc.RegressionInput
		if !readTaskJSON(c, &in) {
			return
		}
		v, e := h.service.Regress(c.Request.Context(), gp(c), c.Param("slot"), c.Param("id"), in, c.GetHeader("Idempotency-Key"))
		if e != nil {
			strategyError(c, e)
			return
		}
		OK(c, v)
	})
	g.POST("/:slot/versions/:id/approvals", func(c *gin.Context) {
		var in svc.ApprovalInput
		if !readTaskJSON(c, &in) {
			return
		}
		v, e := h.service.Approve(c.Request.Context(), gp(c), c.Param("slot"), c.Param("id"), in, c.GetHeader("Idempotency-Key"))
		if e != nil {
			strategyError(c, e)
			return
		}
		OK(c, v)
	})
	g.POST("/:slot/versions/:id/activate", func(c *gin.Context) {
		var in svc.ActivateInput
		if !readTaskJSON(c, &in) {
			return
		}
		v, e := h.service.Activate(c.Request.Context(), gp(c), c.Param("slot"), c.Param("id"), in, c.GetHeader("Idempotency-Key"))
		if e != nil {
			strategyError(c, e)
			return
		}
		OK(c, v)
	})
	g.POST("/:slot/versions/:id/disable", func(c *gin.Context) {
		var in struct {
			Reason string `json:"reason"`
		}
		if !readTaskJSON(c, &in) {
			return
		}
		v, e := h.service.Disable(c.Request.Context(), gp(c), c.Param("slot"), c.Param("id"), in.Reason, c.GetHeader("Idempotency-Key"))
		if e != nil {
			strategyError(c, e)
			return
		}
		OK(c, v)
	})
	g.POST("/:slot/versions/:id/observations", func(c *gin.Context) {
		var in struct {
			TaskID string `json:"task_id"`
		}
		if !readTaskJSON(c, &in) {
			return
		}
		v, e := h.service.Observe(c.Request.Context(), gp(c), c.Param("slot"), c.Param("id"), in.TaskID, c.GetHeader("Idempotency-Key"))
		if e != nil {
			strategyError(c, e)
			return
		}
		OK(c, v)
	})
}
