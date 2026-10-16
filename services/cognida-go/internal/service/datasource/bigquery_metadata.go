package datasource

import (
	m "cognida/internal/model/datasource"
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
)

// The factory only quotes identifiers. REST jobs never open database/sql.
type bigQueryDriver struct{}

func (bigQueryDriver) Type() m.Type       { return m.TypeBigQuery }
func (bigQueryDriver) DriverName() string { return "" }
func (bigQueryDriver) QuoteIdent(s string) string {
	return "`" + strings.ReplaceAll(strings.ReplaceAll(s, "\\", "\\\\"), "`", "\\`") + "`"
}
func (bigQueryDriver) BuildDSN(*m.DataSource, string) (string, error) {
	return "", fmt.Errorf("BigQuery requires the REST QueryAdapter")
}
func (bigQueryDriver) Ping(context.Context, *sql.DB) error {
	return fmt.Errorf("BigQuery requires the REST QueryAdapter")
}
func (bigQueryDriver) ListTables(context.Context, *sql.DB, string) ([]TableInfo, error) {
	return nil, fmt.Errorf("BigQuery requires REST metadata")
}
func (bigQueryDriver) DescribeTable(context.Context, *sql.DB, string, string) (*TableSchema, error) {
	return nil, fmt.Errorf("BigQuery requires REST metadata")
}
func (a *bigQueryAdapter) datasetPath() string {
	return "/projects/" + url.PathEscape(a.options.Project) + "/datasets/" + url.PathEscape(a.source.DatabaseName)
}
func (a *bigQueryAdapter) ping(ctx context.Context) error {
	var response struct {
		Location string `json:"location"`
	}
	if e := a.call(ctx, "GET", a.datasetPath(), nil, &response); e != nil {
		return e
	}
	if !strings.EqualFold(response.Location, a.options.Location) {
		return fmt.Errorf("BigQuery dataset location mismatch")
	}
	return nil
}
func (a *bigQueryAdapter) tables(ctx context.Context) ([]TableInfo, error) {
	out := []TableInfo{}
	token := ""
	seen := map[string]bool{}
	for {
		var page struct {
			Next   string `json:"nextPageToken"`
			Tables []struct {
				Reference struct {
					ID string `json:"tableId"`
				} `json:"tableReference"`
				Type string `json:"type"`
			} `json:"tables"`
		}
		if e := a.call(ctx, "GET", a.datasetPath()+"/tables?maxResults=1000&pageToken="+url.QueryEscape(token), nil, &page); e != nil {
			return nil, e
		}
		for _, table := range page.Tables {
			if len(out) >= 10000 {
				return nil, fmt.Errorf("BigQuery metadata limit exceeded")
			}
			out = append(out, TableInfo{Name: table.Reference.ID, Type: strings.ToLower(table.Type)})
		}
		if page.Next == "" {
			return out, nil
		}
		if seen[page.Next] {
			return nil, fmt.Errorf("BigQuery metadata cursor repeated")
		}
		seen[page.Next] = true
		token = page.Next
	}
}
func (a *bigQueryAdapter) table(ctx context.Context, name string) (*TableSchema, error) {
	if !bqName.MatchString(name) {
		return nil, fmt.Errorf("BigQuery table name invalid")
	}
	var table struct {
		Description string `json:"description"`
		Schema      struct {
			Fields []bqField `json:"fields"`
		} `json:"schema"`
	}
	if e := a.call(ctx, "GET", a.datasetPath()+"/tables/"+url.PathEscape(name), nil, &table); e != nil {
		return nil, e
	}
	if len(table.Schema.Fields) > 512 {
		return nil, fmt.Errorf("BigQuery column metadata limit exceeded")
	}
	out := &TableSchema{Name: name, Comment: table.Description, Columns: []ColumnInfo{}}
	for _, f := range table.Schema.Fields {
		typ := f.Type
		if f.Mode == "REPEATED" {
			typ = "ARRAY<" + typ + ">"
		}
		out.Columns = append(out.Columns, ColumnInfo{Name: f.Name, Type: typ, Nullable: f.Mode != "REQUIRED", Comment: f.Description})
	}
	return out, nil
}
func (s *Service) bigQuery(ctx context.Context, tenant int64, id string) (*bigQueryAdapter, *m.DataSource, error) {
	source, e := s.repo.Get(ctx, id, tenant)
	if e != nil {
		return nil, nil, e
	}
	if source == nil {
		return nil, nil, ErrNotFound
	}
	if source.Type != m.TypeBigQuery {
		return nil, source, nil
	}
	if source.Status == m.StatusNeedCredentials || source.Status == "disabled" || source.Status == "inactive" {
		return nil, nil, fmt.Errorf("BigQuery source unavailable")
	}
	factory := s.remoteFactory
	if factory == nil {
		factory = newBigQueryAdapter
	}
	a, e := factory(ctx, source)
	return a, source, e
}
