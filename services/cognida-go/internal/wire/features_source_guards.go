package wire

import (
	"context"
	"errors"
	"strings"

	agent "cognida/internal/model/agent"
	c "cognida/internal/model/catalog"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/repository/mysql"
	"cognida/internal/service/lifecycle"
	"gorm.io/gorm"
)

type evaluationSourceAuthorizer func(context.Context, l.Principal, string) error

// Assigned by ProvideEvaluationService after construction, before any worker or
// HTTP listener starts. A missing binding denies access rather than bypassing
// fixed-dataset/offline source authorization.
func (f *FeatureServices) authorizeEvaluationSource(ctx context.Context, p l.Principal, id string) error {
	if f.evaluationSourceRead == nil || p.Validate() != nil {
		return l.ErrForbidden
	}
	ctx = agent.WithUserID(agent.WithTenantID(ctx, p.TenantID), p.UserID)
	return f.evaluationSourceRead(ctx, p, id)
}

func availableFeatureDatasource(status string) bool {
	return status == "active" || status == "error"
}

func authorizeCatalogSource(ctx context.Context, db *gorm.DB, p l.Principal, asset *c.AssetTable) (string, error) {
	if asset == nil || asset.TenantID != p.TenantID || asset.Deprecated {
		return "", l.ErrForbidden
	}
	sensitivity := asset.Classification
	for _, column := range asset.Columns {
		// Historical snapshots can contain now-deprecated columns. Their current
		// sensitive classification must still protect those retained bytes.
		if column.Classification == "secret" || (column.Classification == "confidential" && sensitivity != "secret") {
			sensitivity = column.Classification
		}
	}
	if sensitivity == "secret" || sensitivity == "confidential" {
		if err := featureAuthorizer(db)(ctx, p, "read_sensitive"); err != nil {
			return "", err
		}
	}
	return sensitivity, nil
}

func validateDatasourceCatalogOrigin(ctx context.Context, db *gorm.DB, p l.Principal, req lifecycle.SourceRequest, sourceID string) error {
	repo := mysql.NewCatalogRepository(db)
	asset, err := repo.Get(ctx, p.TenantID, sourceID)
	if errors.Is(err, c.ErrNotFound) {
		// Unregistered sources store the exact schema resolved at acquisition.
		// A formerly registered/deleted catalog ID must never become ungoverned.
		prefix, suffix := req.DatasourceID+"/", "/"+req.Table
		if !strings.HasPrefix(sourceID, prefix) || !strings.HasSuffix(sourceID, suffix) || len(sourceID) <= len(prefix)+len(suffix) {
			return l.ErrForbidden
		}
		schema := sourceID[len(prefix) : len(sourceID)-len(suffix)]
		id, lookupErr := repo.ResolveCoordinate(ctx, p.TenantID, req.DatasourceID, schema, req.Table, "")
		if errors.Is(lookupErr, c.ErrNotFound) {
			return nil
		}
		if lookupErr != nil {
			return lookupErr
		}
		asset, err = repo.Get(ctx, p.TenantID, id)
	}
	if err != nil {
		return err
	}
	if asset == nil || asset.DatasourceID != req.DatasourceID || asset.TableName != req.Table || (req.Schema != "" && asset.Schema != req.Schema) {
		return l.ErrForbidden
	}
	_, err = authorizeCatalogSource(ctx, db, p, asset)
	return err
}
