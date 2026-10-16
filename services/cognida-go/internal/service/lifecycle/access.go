package lifecycle

import (
	ds "cognida/internal/model/datasource"
	d "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
)

type SourceRequest struct {
	Kind           string `json:"kind"`
	URL            string `json:"url,omitempty"`
	KnowledgeID    string `json:"knowledge_id,omitempty"`
	DatasourceID   string `json:"datasource_id,omitempty"`
	Schema         string `json:"schema,omitempty"`
	Table          string `json:"table,omitempty"`
	RunID          string `json:"run_id,omitempty"`
	ItemID         string `json:"item_id,omitempty"`
	ResultID       string `json:"result_id,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
	CatalogAssetID string `json:"catalog_asset_id,omitempty"`
}
type AccessFunc func(context.Context, d.Principal, SourceRequest) (d.Input, d.Provenance, d.PolicySnapshot, error)

func (s *Service) acquire(ctx context.Context, p d.Principal, source SourceRequest, name, key string) (d.DataAssetRef, error) {
	if e := s.authorize(ctx, p, "read"); e != nil {
		return d.DataAssetRef{}, e
	}
	if e := validateKey(key); e != nil {
		return d.DataAssetRef{}, e
	}
	switch source.Kind {
	case "web", "knowledge", "datasource", "collection", "result", "evaluation":
	default:
		return d.DataAssetRef{}, d.ErrInvalid
	}
	if s.config.Access == nil {
		return d.DataAssetRef{}, fmt.Errorf("external asset resolver unavailable")
	}
	input, origin, policy, e := s.config.Access(ds.WithQueryIdentity(ctx, p.UserID, "lifecycle-source:"+key), p, source)
	if e != nil {
		return d.DataAssetRef{}, e
	}
	if e = input.Validate(); e != nil {
		return d.DataAssetRef{}, e
	}
	if origin.SourceType == "" || origin.SourceID == "" {
		return d.DataAssetRef{}, fmt.Errorf("access adapter returned no provenance")
	}
	body, e := json.Marshal(input)
	if e != nil {
		return d.DataAssetRef{}, e
	}
	producer := uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("access/%d/%s", p.TenantID, key))).String()
	origin.AccessRequest, e = json.Marshal(source)
	if e != nil {
		return d.DataAssetRef{}, e
	}
	origin.SourceOwnerID = p.UserID
	return s.importPayload(ctx, p, d.RawAsset, "access:"+producer, nil, body, policy, name, origin)
}

// ImportWithKey gives HTTP ingestion stable replay semantics while Import
// remains convenient for internal callers creating distinct snapshots.
func (s *Service) ImportWithKey(ctx context.Context, p d.Principal, input d.Input, name, key string) (d.DataAssetRef, error) {
	if e := validateKey(key); e != nil {
		return d.DataAssetRef{}, e
	}
	if e := s.authorize(ctx, p, "read"); e != nil {
		return d.DataAssetRef{}, e
	}
	if e := input.Validate(); e != nil {
		return d.DataAssetRef{}, e
	}
	body, e := json.Marshal(input)
	if e != nil {
		return d.DataAssetRef{}, e
	}
	return s.importPayload(ctx, p, d.RawAsset, "upload:"+key, nil, body, d.PolicySnapshot{Sensitivity: "unknown", Scopes: []string{"read"}, Version: "1"}, name)
}
