package tools

import (
	agent "cognida/internal/model/agent"
	"context"
	"fmt"
)

type CatalogSearchRequest struct {
	Query string `json:"query" jsonschema:"description=业务术语或表名称"`
}
type CatalogSearchResult struct {
	Metadata string `json:"metadata"`
}

func NewCatalogSearchTool(catalog interface {
	Context(context.Context, int64, string) (string, error)
}) *TypedBaseTool[CatalogSearchRequest, CatalogSearchResult] {
	return NewTypedBaseTool("catalog_search", "搜索租户已同步的数据资产目录与业务术语，返回数据源、物理表、业务描述、分级和资产标识。用于取数前确认来源；不返回原始数据。", func(ctx context.Context, req *CatalogSearchRequest) (*CatalogSearchResult, error) {
		tenant, ok := agent.GetTenantID(ctx)
		if !ok || tenant <= 0 {
			return nil, fmt.Errorf("tenant required")
		}
		v, e := catalog.Context(ctx, tenant, req.Query)
		if e != nil {
			return nil, e
		}
		return &CatalogSearchResult{Metadata: v}, nil
	})
}
