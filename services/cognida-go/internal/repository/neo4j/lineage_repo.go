package neo4j

import (
	l "cognida/internal/model/lineage"
	"context"
	"encoding/json"
	"fmt"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"sync"
)

type LineageRepository struct {
	driver   neo4j.DriverWithContext
	database string
	mu       sync.Mutex
	ready    bool
}

func NewLineageRepository(driver neo4j.DriverWithContext, database string) *LineageRepository {
	return &LineageRepository{driver: driver, database: database}
}
func (r *LineageRepository) ensure(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ready {
		return nil
	}
	_, err := neo4j.ExecuteQuery(ctx, r.driver, "CREATE CONSTRAINT lineage_identity IF NOT EXISTS FOR (n:lineage_node) REQUIRE (n.tenant_id,n.id) IS UNIQUE", nil, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase(r.database))
	if err == nil {
		r.ready = true
	}
	return err
}

// A model-version projection is replaced atomically. Other model projections
// sharing physical nodes and all knowledge-graph labels are left untouched.
func (r *LineageRepository) Upsert(ctx context.Context, tenant int64, projection string, g l.Graph) error {
	if tenant <= 0 || projection == "" {
		return fmt.Errorf("invalid lineage scope")
	}
	if err := r.ensure(ctx); err != nil {
		return err
	}
	s := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.database})
	defer s.Close(ctx)
	_, err := s.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		if _, e := tx.Run(ctx, "MATCH (:lineage_node {tenant_id:$tenant})-[r:lineage_depends {projection:$projection}]->(:lineage_node {tenant_id:$tenant}) DELETE r", map[string]any{"tenant": tenant, "projection": projection}); e != nil {
			return nil, e
		}
		for _, n := range g.Nodes {
			b, _ := json.Marshal(n)
			if _, e := tx.Run(ctx, `MERGE (n:lineage_node {tenant_id:$tenant,id:$id}) SET n.kind=$kind,n.asset_id=$asset,n.payload=$payload,n.updated_at=datetime()`, map[string]any{"tenant": tenant, "id": n.ID, "kind": n.Kind, "asset": n.AssetID, "payload": string(b)}); e != nil {
				return nil, e
			}
		}
		for _, edge := range g.Edges {
			if _, e := tx.Run(ctx, `MATCH (a:lineage_node {tenant_id:$tenant,id:$from}),(b:lineage_node {tenant_id:$tenant,id:$to}) MERGE (a)-[r:lineage_depends {projection:$projection}]->(b) SET r.kind=$kind`, map[string]any{"tenant": tenant, "from": edge.From, "to": edge.To, "projection": projection, "kind": edge.Kind}); e != nil {
				return nil, e
			}
		}
		return nil, nil
	})
	return err
}
func (r *LineageRepository) Query(ctx context.Context, tenant int64, asset, direction string, depth int) (*l.Graph, error) {
	if tenant <= 0 || depth < 1 || depth > 8 {
		return nil, fmt.Errorf("invalid lineage query")
	}
	if err := r.ensure(ctx); err != nil {
		return nil, err
	}
	arrows := "-[:lineage_depends*1..%d]-"
	switch direction {
	case "upstream":
		arrows = "<-[:lineage_depends*1..%d]-"
	case "downstream":
		arrows = "-[:lineage_depends*1..%d]->"
	case "both":
	default:
		return nil, fmt.Errorf("invalid lineage direction")
	}
	// Depth and direction are validated constants; every traversed node and
	// relationship is constrained to this namespace and tenant before returning.
	pattern := fmt.Sprintf(arrows, depth)
	query := `MATCH (root:lineage_node {tenant_id:$tenant}) WHERE root.id=$asset OR root.asset_id=$asset OPTIONAL MATCH p=(root)` + pattern + `(other:lineage_node) WHERE all(n IN nodes(p) WHERE n:lineage_node AND n.tenant_id=$tenant) AND all(r IN relationships(p) WHERE type(r)='lineage_depends') RETURN root, nodes(p) AS ns, relationships(p) AS rs LIMIT 501`
	result, err := neo4j.ExecuteQuery(ctx, r.driver, query, map[string]any{"tenant": tenant, "asset": asset}, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase(r.database))
	if err != nil {
		return nil, err
	}
	out := &l.Graph{Nodes: []l.Node{}, Edges: []l.Edge{}, Truncated: len(result.Records) > 500}
	nodes := map[string]bool{}
	elementIDs := map[string]string{}
	edges := []neo4j.Relationship{}
	add := func(raw any) error {
		n, ok := raw.(neo4j.Node)
		if !ok {
			return nil
		}
		payload, _ := n.Props["payload"].(string)
		var v l.Node
		if err := json.Unmarshal([]byte(payload), &v); err != nil {
			return err
		}
		elementIDs[n.ElementId] = v.ID
		if !nodes[v.ID] {
			nodes[v.ID] = true
			out.Nodes = append(out.Nodes, v)
		}
		return nil
	}
	for i, record := range result.Records {
		if i == 500 {
			break
		}
		root, _ := record.Get("root")
		if err := add(root); err != nil {
			return nil, err
		}
		ns, _ := record.Get("ns")
		if list, ok := ns.([]any); ok {
			for _, n := range list {
				if err := add(n); err != nil {
					return nil, err
				}
			}
		}
		rs, _ := record.Get("rs")
		if list, ok := rs.([]any); ok {
			for _, raw := range list {
				if edge, ok := raw.(neo4j.Relationship); ok {
					edges = append(edges, edge)
				}
			}
		}
	}
	seen := map[string]bool{}
	for _, e := range edges {
		from, to := elementIDs[e.StartElementId], elementIDs[e.EndElementId]
		key := from + "\x00" + to
		if from != "" && to != "" && !seen[key] {
			kind, _ := e.Props["kind"].(string)
			out.Edges = append(out.Edges, l.Edge{From: from, To: to, Kind: kind})
			seen[key] = true
		}
	}
	return out, nil
}
