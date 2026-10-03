package lineage

import (
	c "cognida/internal/model/catalog"
	l "cognida/internal/model/lineage"
	"cognida/internal/model/semantic"
	"cognida/internal/service/agent/metricsql"
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"strings"
	"time"
)

type Service struct {
	Repo     l.Repository
	Catalog  c.Repository
	Semantic semantic.Repository
}

func NewService(repo l.Repository, catalog c.Repository, models semantic.Repository) *Service {
	return &Service{repo, catalog, models}
}
func stable(parts ...string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(strings.Join(parts, "\x00"))).String()
}
func (s *Service) Rebuild(ctx context.Context, tenant int64, modelID string) (*l.Graph, error) {
	if tenant <= 0 {
		return nil, c.ErrInvalid
	}
	b, err := s.Semantic.GetModelBundle(ctx, modelID)
	if errors.Is(err, semantic.ErrModelNotFound) {
		return nil, c.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if b == nil || b.Model == nil || b.Model.TenantID != tenant {
		return nil, c.ErrNotFound
	}
	return s.rebuildBundle(ctx, b)
}
func (s *Service) rebuildBundle(ctx context.Context, b *semantic.ModelBundle) (*l.Graph, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if len(b.Metrics)+len(b.Dimensions) > 1000 {
		return nil, c.ErrInvalid
	}
	g := l.Graph{Nodes: []l.Node{}, Edges: []l.Edge{}}
	nodes := map[string]bool{}
	edges := map[string]bool{}
	add := func(n l.Node) {
		if !nodes[n.ID] {
			nodes[n.ID] = true
			g.Nodes = append(g.Nodes, n)
		}
	}
	edge := func(from, to, kind string) {
		key := from + "\x00" + to
		if !edges[key] {
			edges[key] = true
			g.Edges = append(g.Edges, l.Edge{From: from, To: to, Kind: kind})
		}
	}
	tables := map[string]*semantic.LogicalTable{}
	for _, t := range b.LogicalTables {
		tables[t.ID] = t
	}
	outputs := []metricsql.Query{}
	for _, m := range b.Metrics {
		outputs = append(outputs, metricsql.Query{Metrics: []string{m.Name}})
	}
	for _, d := range b.Dimensions {
		outputs = append(outputs, metricsql.Query{Dimensions: []string{d.Name}})
	}
	assets := map[string][]c.AssetTable{}
	for _, t := range b.LogicalTables {
		if _, ok := assets[t.DatabaseID]; ok {
			continue
		}
		cursor := ""
		for {
			page, err := s.Catalog.List(ctx, b.Model.TenantID, c.Filter{DatasourceID: t.DatabaseID, Cursor: cursor, Limit: 100, IncludeDeprecated: true})
			if err != nil {
				return nil, err
			}
			assets[t.DatabaseID] = append(assets[t.DatabaseID], page.Items...)
			if !page.HasMore {
				break
			}
			cursor = page.NextCursor
		}
	}
	activeColumns := map[string]map[string]string{}
	for _, q := range outputs {
		result, err := metricsql.Build(b, q)
		if err != nil {
			return nil, err
		}
		if !result.Coverage.Covered {
			continue
		}
		for _, dep := range result.Dependencies {
			t := tables[dep.LogicalTableID]
			if t == nil {
				continue
			}
			name := t.PhysicalTable
			schema := ""
			if parts := strings.SplitN(name, ".", 2); len(parts) == 2 {
				schema, name = strings.Trim(parts[0], "`\""), strings.Trim(parts[1], "`\"")
			}
			tableAsset := ""
			for _, a := range assets[t.DatabaseID] {
				if !a.Deprecated && a.TableName == name && (schema == "" || a.Schema == schema) {
					schema = a.Schema
					tableAsset = a.ID
					break
				}
			}
			physical := stable("physical", t.DatabaseID, schema, name)
			logical := stable("logical", b.Model.ID, t.ID)
			kind, assetID := outputIdentity(b, dep.Output, len(q.Metrics) > 0)
			output := stable(kind, b.Model.ID, assetID)
			add(l.Node{ID: physical, Kind: "PhysicalTable", Name: name, AssetID: tableAsset, DatasourceID: t.DatabaseID, Schema: schema, Table: name, Unresolved: tableAsset == ""})
			add(l.Node{ID: logical, Kind: "LogicalTable", Name: t.Name, AssetID: t.ID})
			add(l.Node{ID: output, Kind: kind, Name: dep.Output, AssetID: assetID})
			edge(physical, logical, "maps_to")
			edge(logical, output, "input")
			if dep.Column != "" {
				// Resolve only active columns, loading each table once. Coordinate
				// lookup intentionally includes tombstones for access control and
				// must not be used to claim a retired column is still available.
				if tableAsset != "" {
					if _, loaded := activeColumns[tableAsset]; !loaded {
						detail, err := s.Catalog.Get(ctx, b.Model.TenantID, tableAsset)
						if err != nil {
							return nil, err
						}
						activeColumns[tableAsset] = map[string]string{}
						if !detail.Deprecated {
							for _, col := range detail.Columns {
								if !col.Deprecated {
									activeColumns[tableAsset][col.Name] = col.ID
								}
							}
						}
					}
				}
				assetID := activeColumns[tableAsset][dep.Column]
				id := stable("column", t.DatabaseID, schema, name, dep.Column)
				add(l.Node{ID: id, Kind: "PhysicalColumn", Name: dep.Column, AssetID: assetID, DatasourceID: t.DatabaseID, Schema: schema, Table: name, Column: dep.Column, Unresolved: assetID == ""})
				edge(physical, id, "contains")
				edge(id, output, "input")
			}
		}
	}
	if err := s.Repo.Upsert(ctx, b.Model.TenantID, b.Model.ID, g); err != nil {
		return nil, err
	}
	return &g, nil
}

func outputIdentity(b *semantic.ModelBundle, name string, metric bool) (string, string) {
	if metric {
		for _, m := range b.Metrics {
			if m.Name == name {
				return "Metric", m.ID
			}
		}
		return "Metric", name
	}
	for _, d := range b.Dimensions {
		if d.Name == name {
			return "Dimension", d.ID
		}
	}
	return "Dimension", name
}
func (s *Service) Observer(b *semantic.ModelBundle, _ metricsql.Query, _ *metricsql.Result) {
	if b == nil || b.Model == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := s.rebuildBundle(ctx, b); err != nil {
		fmt.Printf("[Lineage] projection failed model=%s: %v\n", b.Model.ID, err)
	}
}
func (s *Service) Query(ctx context.Context, tenant int64, asset, direction string, depth int) (*l.Graph, error) {
	if tenant <= 0 || asset == "" || depth < 1 || depth > 8 {
		return nil, c.ErrInvalid
	}
	switch direction {
	case "upstream", "downstream", "both":
	default:
		return nil, c.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.Repo.Query(ctx, tenant, asset, direction, depth)
}
