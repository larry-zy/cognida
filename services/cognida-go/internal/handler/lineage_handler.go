package handler

import (
	svc "cognida/internal/service/lineage"
	"github.com/gin-gonic/gin"
	"strconv"
)

type LineageHandler struct{ service *svc.Service }

func NewLineageHandler(s *svc.Service) *LineageHandler { return &LineageHandler{s} }
func (h *LineageHandler) Register(api *gin.RouterGroup) {
	g := api.Group("/lineage")
	g.GET("/:asset", h.Query)
	g.GET("/:asset/impact", h.Impact)
	g.POST("/rebuild", h.Rebuild)
}
func (h *LineageHandler) Query(c *gin.Context)  { h.query(c, c.DefaultQuery("direction", "both")) }
func (h *LineageHandler) Impact(c *gin.Context) { h.query(c, "downstream") }
func (h *LineageHandler) query(c *gin.Context, direction string) {
	depth, e := strconv.Atoi(c.DefaultQuery("depth", "3"))
	if e != nil {
		BadRequest(c, "depth 必须是 1 到 8 的整数")
		return
	}
	v, e := h.service.Query(c.Request.Context(), GetTenantID(c), c.Param("asset"), direction, depth)
	if e != nil {
		catalogErr(c, e)
		return
	}
	OK(c, v)
}
func (h *LineageHandler) Rebuild(c *gin.Context) {
	var req struct {
		ModelID string `json:"model_id"`
	}
	if !BindJSON(c, &req) {
		return
	}
	v, e := h.service.Rebuild(c.Request.Context(), GetTenantID(c), req.ModelID)
	if e != nil {
		catalogErr(c, e)
		return
	}
	OK(c, v)
}
