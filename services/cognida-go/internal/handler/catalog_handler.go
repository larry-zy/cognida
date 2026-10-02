package handler

import (
	catalog "cognida/internal/model/catalog"
	svc "cognida/internal/service/catalog"
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"strconv"
)

type CatalogHandler struct {
	service   *svc.Service
	Authorize func(context.Context, int64, int64, string) error
}

func NewCatalogHandler(s *svc.Service) *CatalogHandler { return &CatalogHandler{service: s} }
func (h *CatalogHandler) Register(api *gin.RouterGroup) {
	g := api.Group("/catalog")
	g.GET("/tables", h.List)
	g.GET("/tables/:id", h.Get)
	g.GET("/tables/:id/profile-card", h.ProfileCard)
	g.PUT("/tables/:id", h.Update)
	g.PUT("/columns/:id", h.Update)
	g.POST("/sync", h.Sync)
	g.GET("/glossary", h.Glossaries)
	g.POST("/glossary", h.SaveGlossary)
	g.PUT("/glossary/:id", h.SaveGlossary)
	g.DELETE("/glossary/:id", h.DeleteGlossary)
}
func catalogFilter(c *gin.Context) catalog.Filter {
	n, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	return catalog.Filter{DatasourceID: c.Query("datasource_id"), Query: c.Query("q"), Tag: c.Query("tag"), Cursor: c.Query("cursor"), Limit: n, IncludeDeprecated: c.Query("include_deprecated") == "true"}
}
func catalogErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, catalog.ErrNotFound):
		NotFound(c, "资产不存在")
	case errors.Is(err, catalog.ErrInvalid):
		BadRequest(c, err.Error())
	default:
		InternalError(c, "资产操作失败，请检查数据源或配置")
	}
}
func (h *CatalogHandler) List(c *gin.Context) {
	v, e := h.service.List(c.Request.Context(), GetTenantID(c), catalogFilter(c))
	if e != nil {
		catalogErr(c, e)
		return
	}
	for i := range v.Items {
		redactCatalogSamples(&v.Items[i])
	}
	OK(c, v)
}
func redactCatalogSamples(t *catalog.AssetTable) {
	sensitive := t.Classification == "secret" || t.Classification == "confidential"
	for i := range t.Columns {
		col := &t.Columns[i]
		if sensitive || col.Classification == "secret" || col.Classification == "confidential" {
			col.SampleValues = []string{}
		}
	}
}
func (h *CatalogHandler) Get(c *gin.Context) {
	v, e := h.service.Get(c.Request.Context(), GetTenantID(c), c.Param("id"))
	if e != nil {
		catalogErr(c, e)
		return
	}
	redactCatalogSamples(v)
	OK(c, v)
}
func (h *CatalogHandler) canWrite(c *gin.Context) bool {
	if h.Authorize != nil {
		if err := h.Authorize(c.Request.Context(), GetTenantID(c), GetUserID(c), "catalog.write"); err != nil {
			Forbidden(c, "需要管理员权限修改资产目录")
			return false
		}
	}
	return true
}
func (h *CatalogHandler) Update(c *gin.Context) {
	if !h.canWrite(c) {
		return
	}
	var g catalog.Governance
	if !BindJSON(c, &g) {
		return
	}
	if e := h.service.Update(c.Request.Context(), GetTenantID(c), GetUserID(c), c.Param("id"), g); e != nil {
		catalogErr(c, e)
		return
	}
	OK(c, g)
}
func (h *CatalogHandler) Sync(c *gin.Context) {
	if h.Authorize != nil {
		if err := h.Authorize(c.Request.Context(), GetTenantID(c), GetUserID(c), "catalog.sync"); err != nil {
			Forbidden(c, "需要管理员权限同步资产")
			return
		}
	}
	var req struct {
		DatasourceID string `json:"datasource_id"`
		Profile      bool   `json:"profile"`
	}
	if !BindJSON(c, &req) {
		return
	}
	v, e := h.service.Sync(c.Request.Context(), GetTenantID(c), GetUserID(c), req.DatasourceID, req.Profile)
	if e != nil {
		catalogErr(c, e)
		return
	}
	OK(c, v)
}
func (h *CatalogHandler) Glossaries(c *gin.Context) {
	v, e := h.service.Repo.Glossaries(c.Request.Context(), GetTenantID(c), catalogFilter(c))
	if e != nil {
		catalogErr(c, e)
		return
	}
	OK(c, v)
}
func (h *CatalogHandler) SaveGlossary(c *gin.Context) {
	if !h.canWrite(c) {
		return
	}
	var g catalog.Glossary
	if !BindJSON(c, &g) {
		return
	}
	g.ID = c.Param("id")
	v, e := h.service.SaveGlossary(c.Request.Context(), GetTenantID(c), GetUserID(c), g)
	if e != nil {
		catalogErr(c, e)
		return
	}
	OK(c, v)
}
func (h *CatalogHandler) DeleteGlossary(c *gin.Context) {
	if !h.canWrite(c) {
		return
	}
	if e := h.service.Repo.DeleteGlossary(c.Request.Context(), GetTenantID(c), c.Param("id")); e != nil {
		catalogErr(c, e)
		return
	}
	OK(c, gin.H{"deleted": true})
}

func (h *CatalogHandler) ProfileCard(c *gin.Context) {
	v, e := h.service.ProfileCard(c.Request.Context(), GetTenantID(c), c.Param("id"))
	if e != nil {
		catalogErr(c, e)
		return
	}
	OK(c, v)
}
