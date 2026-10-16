package handler

import (
	d "cognida/internal/model/evaluation"
	svc "cognida/internal/service/evaluation"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

type DatasetVersionHandler struct{ service *svc.VersionService }

func NewDatasetVersionHandler(s *svc.VersionService) *DatasetVersionHandler {
	return &DatasetVersionHandler{s}
}
func datasetVersionErr(c *gin.Context, e error) {
	switch {
	case errors.Is(e, d.ErrVersionForbidden), errors.Is(e, d.ErrOfflineForbidden):
		Forbidden(c, "数据集或来源已不可访问")
	case errors.Is(e, d.ErrVersionNotFound), errors.Is(e, d.ErrTaskNotFound):
		NotFound(c, "记录不存在")
	case errors.Is(e, d.ErrVersionConflict), errors.Is(e, d.ErrOfflineConflict):
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "审核版本或固定评测配置已变化"})
	case errors.Is(e, d.ErrVersionInvalid), errors.Is(e, d.ErrDatasetTypeMismatch):
		BadRequest(c, e.Error())
	default:
		InternalError(c, "数据集版本操作失败")
	}
}
func (h *DatasetVersionHandler) Register(api *gin.RouterGroup) {
	api.POST("/evaluation/dataset-versions", func(c *gin.Context) {
		var in d.PublishDatasetVersion
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4<<20)
		if !BindJSON(c, &in) {
			return
		}
		out, e := h.service.Publish(c.Request.Context(), gp(c), in, c.GetHeader("Idempotency-Key"))
		if e != nil {
			datasetVersionErr(c, e)
			return
		}
		OK(c, out)
	})
	api.GET("/evaluation/dataset-versions", func(c *gin.Context) {
		limit, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
		out, e := h.service.List(c.Request.Context(), gp(c), c.Query("dataset_id"), c.Query("cursor"), limit)
		if e != nil {
			datasetVersionErr(c, e)
			return
		}
		OK(c, out)
	})
	api.GET("/evaluation/dataset-versions/:id", func(c *gin.Context) {
		out, e := h.service.Read(c.Request.Context(), gp(c), c.Param("id"))
		if e != nil {
			datasetVersionErr(c, e)
			return
		}
		OK(c, out)
	})
	api.GET("/evaluation/dataset-versions/:id/export", func(c *gin.Context) {
		out, e := h.service.Read(c.Request.Context(), gp(c), c.Param("id"))
		if e != nil {
			datasetVersionErr(c, e)
			return
		}
		c.Header("Content-Disposition", "attachment; filename=dataset-version.json")
		c.JSON(http.StatusOK, out)
	})
}
