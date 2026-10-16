package handler

import (
	d "cognida/internal/model/queryjob"
	svc "cognida/internal/service/queryjob"
	"errors"
	"github.com/gin-gonic/gin"
	"strconv"
)

type QueryJobHandler struct{ service *svc.Service }

func NewQueryJobHandler(s *svc.Service) *QueryJobHandler { return &QueryJobHandler{s} }
func queryJobError(c *gin.Context, e error) {
	switch {
	case errors.Is(e, d.ErrInvalid):
		BadRequest(c, "查询或预算无效")
	case errors.Is(e, d.ErrForbidden):
		Forbidden(c, "数据源当前不可访问或配置已改变")
	case errors.Is(e, d.ErrNotFound):
		NotFound(c, "查询任务不存在")
	case errors.Is(e, d.ErrConflict):
		c.JSON(409, gin.H{"code": 409, "message": "查询状态或幂等请求冲突"})
	default:
		InternalError(c, "查询操作失败")
	}
}
func (h *QueryJobHandler) Register(api *gin.RouterGroup) {
	g := api.Group("/query-jobs")
	g.Use(func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() })
	g.POST("", func(c *gin.Context) {
		var in d.Input
		if !readTaskJSON(c, &in) {
			return
		}
		v, e := h.service.Create(c.Request.Context(), gp(c), in, c.GetHeader("Idempotency-Key"))
		if e != nil {
			queryJobError(c, e)
			return
		}
		OK(c, v)
	})
	g.GET("", func(c *gin.Context) {
		limit, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
		v, e := h.service.List(c.Request.Context(), gp(c), c.Query("cursor"), limit)
		if e != nil {
			queryJobError(c, e)
			return
		}
		OK(c, v)
	})
	g.GET("/:id", func(c *gin.Context) {
		v, e := h.service.Get(c.Request.Context(), gp(c), c.Param("id"))
		if e != nil {
			queryJobError(c, e)
			return
		}
		OK(c, v)
	})
	g.GET("/:id/results", func(c *gin.Context) {
		limit, _ := strconv.Atoi(c.DefaultQuery("page_size", "100"))
		v, e := h.service.Results(c.Request.Context(), gp(c), c.Param("id"), c.Query("cursor"), limit)
		if e != nil {
			queryJobError(c, e)
			return
		}
		OK(c, v)
	})
	g.POST("/:id/cancel", func(c *gin.Context) {
		var in struct{}
		if !readTaskJSON(c, &in) {
			return
		}
		v, e := h.service.Cancel(c.Request.Context(), gp(c), c.Param("id"))
		if e != nil {
			queryJobError(c, e)
			return
		}
		OK(c, v)
	})
}
