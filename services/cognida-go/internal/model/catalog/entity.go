// Package catalog defines tenant-scoped physical metadata and governance contracts.
package catalog

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("catalog asset not found")
var ErrInvalid = errors.New("invalid catalog input")

type Governance struct {
	Owner          string   `json:"owner"`
	Tags           []string `json:"tags"`
	Classification string   `json:"classification"`
}
type AssetTable struct {
	ID           string    `json:"id"`
	TenantID     int64     `json:"tenant_id"`
	DatasourceID string    `json:"datasource_id"`
	Schema       string    `json:"schema"`
	TableName    string    `json:"table_name"`
	Type         string    `json:"type"`
	Comment      string    `json:"comment"`
	RowCount     int64     `json:"row_count"`
	LastSyncedAt time.Time `json:"last_synced_at"`
	Deprecated   bool      `json:"deprecated"`
	Governance
	Columns     []AssetColumn `json:"columns,omitempty"`
	MatchSource string        `json:"match_source,omitempty"`
}
type AssetColumn struct {
	ID            string   `json:"id"`
	TenantID      int64    `json:"tenant_id"`
	TableID       string   `json:"table_id"`
	Name          string   `json:"name"`
	DataType      string   `json:"data_type"`
	Comment       string   `json:"comment"`
	Nullable      bool     `json:"nullable"`
	IsPK          bool     `json:"is_pk"`
	IsFK          bool     `json:"is_fk"`
	Deprecated    bool     `json:"deprecated"`
	Profiled      bool     `json:"profiled"`
	NullRate      *float64 `json:"null_rate"`
	DistinctCount *int64   `json:"distinct_count"`
	SampleValues  []string `json:"sample_values"`
	SampleSize    int      `json:"sample_size"`
	Governance
}
type Glossary struct {
	ID          string   `json:"id"`
	TenantID    int64    `json:"tenant_id"`
	Term        string   `json:"term"`
	Description string   `json:"description"`
	Synonyms    []string `json:"synonyms"`
	AssetIDs    []string `json:"asset_ids"`
}
type Filter struct {
	DatasourceID, Query, Tag, Cursor string
	Limit                            int
	IncludeDeprecated                bool
}
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor"`
	HasMore    bool   `json:"has_more"`
}
type SyncResult struct {
	Tables     int      `json:"tables"`
	Columns    int      `json:"columns"`
	Created    int      `json:"created"`
	Updated    int      `json:"updated"`
	Deprecated int      `json:"deprecated"`
	Warnings   []string `json:"warnings"`
}
type Repository interface {
	Snapshot(context.Context, int64, string, []AssetTable) (*SyncResult, error)
	List(context.Context, int64, Filter) (*Page[AssetTable], error)
	Get(context.Context, int64, string) (*AssetTable, error)
	ActiveColumnNames(context.Context, int64, []string) (map[string][]string, error)
	UpdateGovernance(context.Context, int64, string, Governance) error
	Glossaries(context.Context, int64, Filter) (*Page[Glossary], error)
	SaveGlossary(context.Context, int64, Glossary) (*Glossary, error)
	DeleteGlossary(context.Context, int64, string) error
	ResolveCoordinate(context.Context, int64, string, string, string, string) (string, error)
}
