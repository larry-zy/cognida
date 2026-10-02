package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	c "cognida/internal/model/catalog"
	ds "cognida/internal/model/datasource"
	datasource "cognida/internal/service/datasource"
)

type Sampler interface {
	SampleRecords(context.Context, int64, string, string, int) ([]string, [][]any, error)
}
type Audit interface {
	Record(context.Context, int64, int64, string, string, any)
}
type Service struct {
	Repo        c.Repository
	Connections ds.ConnectionProvider
	Sampler     Sampler
	Audit       Audit
}

func NewService(repo c.Repository, connections ds.ConnectionProvider, sampler Sampler, audit Audit) *Service {
	return &Service{repo, connections, sampler, audit}
}
func (s *Service) Sync(ctx context.Context, tenant, user int64, id string, profile bool) (*c.SyncResult, error) {
	if tenant <= 0 || user <= 0 || id == "" {
		return nil, c.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	db, source, err := s.Connections.Acquire(ctx, tenant, id)
	if err != nil {
		return nil, err
	}
	drv, err := datasource.DriverFor(source.Type)
	if err != nil {
		return nil, err
	}
	schema := source.DatabaseName
	if source.Type == ds.TypePostgres {
		if err := db.QueryRowContext(ctx, "SELECT current_schema()").Scan(&schema); err != nil {
			return nil, err
		}
	}
	tables, err := drv.ListTables(ctx, db, source.DatabaseName)
	if err != nil {
		return nil, err
	}
	if len(tables) > 2000 {
		return nil, fmt.Errorf("%w: source has over 2000 tables", c.ErrInvalid)
	}
	snapshot := make([]c.AssetTable, 0, len(tables))
	warnings := []string{}
	now := time.Now().UTC()
	for _, table := range tables {
		structure, err := drv.DescribeTable(ctx, db, source.DatabaseName, table.Name)
		if err != nil {
			return nil, err
		}
		t := c.AssetTable{DatasourceID: id, Schema: schema, TableName: table.Name, Type: table.Type, Comment: table.Comment, RowCount: table.Rows, LastSyncedAt: now, Governance: c.Governance{Tags: []string{}, Classification: "internal"}, Columns: []c.AssetColumn{}}
		for _, col := range structure.Columns {
			t.Columns = append(t.Columns, c.AssetColumn{Name: col.Name, DataType: col.Type, Comment: col.Comment, Nullable: col.Nullable, IsPK: col.Key == "PRI", IsFK: col.IsFK, Governance: c.Governance{Tags: []string{}, Classification: "internal"}})
		}
		if profile {
			cols, rows, err := s.Sampler.SampleRecords(ctx, tenant, id, table.Name, 1000)
			if err != nil {
				warnings = append(warnings, "profiling unavailable: "+table.Name)
			} else {
				applyProfile(t.Columns, cols, rows)
			}
		}
		snapshot = append(snapshot, t)
	}
	out, err := s.Repo.Snapshot(ctx, tenant, id, snapshot)
	if err != nil {
		return nil, err
	}
	out.Warnings = warnings
	if s.Audit != nil {
		s.Audit.Record(ctx, tenant, user, "catalog.sync", id, out)
	}
	return out, nil
}
func applyProfile(columns []c.AssetColumn, names []string, rows [][]any) {
	index := map[string]int{}
	for i, name := range names {
		index[name] = i
	}
	for i := range columns {
		col := &columns[i]
		pos, ok := index[col.Name]
		if !ok {
			continue
		}
		nulls := 0
		distinct := map[string]bool{}
		samples := []string{}
		for _, row := range rows {
			if pos >= len(row) || row[pos] == nil {
				nulls++
				continue
			}
			v := fmt.Sprint(row[pos])
			if !distinct[v] && len(samples) < 5 {
				r := []rune(v)
				if len(r) > 128 {
					v = string(r[:128]) + "…"
				}
				samples = append(samples, v)
			}
			distinct[fmt.Sprint(row[pos])] = true
		}
		rate := 0.0
		if len(rows) > 0 {
			rate = float64(nulls) / float64(len(rows))
		}
		count := int64(len(distinct))
		col.NullRate = &rate
		col.DistinctCount = &count
		col.SampleSize = len(rows)
		col.SampleValues = samples
		col.Profiled = true
	}
}
func (s *Service) Get(ctx context.Context, tenant int64, id string) (*c.AssetTable, error) {
	if tenant <= 0 {
		return nil, c.ErrInvalid
	}
	return s.Repo.Get(ctx, tenant, id)
}
func (s *Service) List(ctx context.Context, tenant int64, f c.Filter) (*c.Page[c.AssetTable], error) {
	if tenant <= 0 {
		return nil, c.ErrInvalid
	}
	return s.Repo.List(ctx, tenant, f)
}
func (s *Service) Update(ctx context.Context, tenant, user int64, id string, g c.Governance) error {
	if tenant <= 0 || user <= 0 || len(g.Owner) > 128 || len(g.Tags) > 32 {
		return c.ErrInvalid
	}
	switch g.Classification {
	case "public", "internal", "confidential", "secret":
	default:
		return c.ErrInvalid
	}
	if g.Tags == nil {
		g.Tags = []string{}
	}
	for _, tag := range g.Tags {
		if strings.TrimSpace(tag) == "" || len(tag) > 64 {
			return c.ErrInvalid
		}
	}
	if err := s.Repo.UpdateGovernance(ctx, tenant, id, g); err != nil {
		return err
	}
	if s.Audit != nil {
		s.Audit.Record(ctx, tenant, user, "catalog.governance", id, g)
	}
	return nil
}
func (s *Service) SaveGlossary(ctx context.Context, tenant, user int64, g c.Glossary) (*c.Glossary, error) {
	g.Term = strings.TrimSpace(g.Term)
	if tenant <= 0 || user <= 0 || g.Term == "" || len(g.Term) > 128 || len(g.Description) > 4096 || len(g.Synonyms) > 50 || len(g.AssetIDs) > 100 {
		return nil, c.ErrInvalid
	}
	if g.Synonyms == nil {
		g.Synonyms = []string{}
	}
	if g.AssetIDs == nil {
		g.AssetIDs = []string{}
	}
	for _, v := range g.Synonyms {
		if len(v) > 128 || strings.TrimSpace(v) == "" {
			return nil, c.ErrInvalid
		}
	}
	out, err := s.Repo.SaveGlossary(ctx, tenant, g)
	if err == nil && s.Audit != nil {
		s.Audit.Record(ctx, tenant, user, "catalog.glossary", out.ID, out)
	}
	return out, err
}

// Context returns bounded catalog metadata for semantic tools; no sample values
// are included in model context.
func (s *Service) Context(ctx context.Context, tenant int64, query string) (string, error) {
	page, err := s.List(ctx, tenant, c.Filter{Query: query, Limit: 10})
	if err != nil {
		return "", err
	}
	type hint struct{ ID, Datasource, Schema, Table, Description, Classification string }
	items := []hint{}
	for _, t := range page.Items {
		items = append(items, hint{t.ID, t.DatasourceID, t.Schema, t.TableName, t.Comment, t.Classification})
	}
	b, err := json.Marshal(items)
	return string(b), err
}
