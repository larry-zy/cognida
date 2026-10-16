package tools

import (
	agent "cognida/internal/model/agent"
	ds "cognida/internal/model/datasource"
	source "cognida/internal/service/datasource"
	"context"
	"fmt"
	"strings"
)

type restMetadata interface {
	Get(context.Context, int64, string) (*source.Sanitized, error)
	ListTables(context.Context, int64, string) ([]source.TableInfo, error)
	DescribeTable(context.Context, int64, string, string) (*source.TableSchema, error)
}

func remoteSchema(ctx context.Context, p ds.ConnectionProvider, id, table, keywords string, full bool) (*GetSchemaResult, bool, error) {
	if selected := agent.MustGetDatasourceID(ctx); selected != "" {
		id = selected
	}
	if id == "" {
		return nil, false, nil
	}
	metadata, ok := p.(restMetadata)
	if !ok {
		return nil, false, nil
	}
	src, e := metadata.Get(ctx, agent.MustGetTenantID(ctx), id)
	if e != nil {
		return nil, true, e
	}
	if src.Type != string(ds.TypeBigQuery) {
		return nil, false, nil
	}
	if src.Status == "disabled" || src.Status == "inactive" || src.Status == "need_credentials" {
		return nil, true, fmt.Errorf("source currently unavailable")
	}
	names, e := metadata.ListTables(ctx, agent.MustGetTenantID(ctx), id)
	if e != nil {
		return nil, true, e
	}
	out := &GetSchemaResult{Database: src.DatabaseName, Tables: []TableSchema{}, Note: "当前授权的 BigQuery REST 元数据；未运行采样或付费查询"}
	found := false
	for _, name := range names {
		if validatePrivateDataAccess(name.Name) != nil {
			continue
		}
		if table != "" && name.Name != table {
			continue
		}
		if keywords != "" && !strings.Contains(strings.ToLower(name.Name+" "+name.Comment), strings.ToLower(keywords)) {
			continue
		}
		found = true
		v := TableSchema{TableName: name.Name, Description: name.Comment, Columns: []ColumnSchema{}}
		if full || table != "" || keywords != "" {
			if len(out.Tables) >= maxRelevantTables {
				out.Note = "元数据已截断到15张表；请指定table_name"
				break
			}
			schema, e := metadata.DescribeTable(ctx, agent.MustGetTenantID(ctx), id, name.Name)
			if e != nil {
				return nil, true, e
			}
			for _, column := range schema.Columns {
				v.Columns = append(v.Columns, ColumnSchema{Name: column.Name, Type: column.Type, Nullable: column.Nullable, Description: column.Comment})
			}
		}
		if len(out.Tables) >= maxCatalogTables {
			out.Note = "目录已截断；请指定table_name"
			break
		}
		out.Tables = append(out.Tables, v)
	}
	if !found && table != "" {
		return nil, true, fmt.Errorf("requested table is absent or not authorized")
	}
	return out, true, nil
}
