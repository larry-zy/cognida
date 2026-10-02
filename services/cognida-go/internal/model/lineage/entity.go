package lineage

import "context"

type Node struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	AssetID      string `json:"asset_id"`
	DatasourceID string `json:"datasource_id,omitempty"`
	Schema       string `json:"schema,omitempty"`
	Table        string `json:"table,omitempty"`
	Column       string `json:"column,omitempty"`
	Unresolved   bool   `json:"unresolved"`
}

// Edges point from a source to its dependent, so downstream is impact analysis.
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}
type Graph struct {
	Nodes     []Node `json:"nodes"`
	Edges     []Edge `json:"edges"`
	Truncated bool   `json:"truncated"`
}
type Repository interface {
	Upsert(context.Context, int64, string, Graph) error
	Query(context.Context, int64, string, string, int) (*Graph, error)
}
