package lifecycle

import (
	"context"
	"strings"
	"time"

	d "cognida/internal/model/lifecycle"
)

func governedOrigin(ref d.DataAssetRef) bool {
	for _, prefix := range []string{"governance-governed:", "feature-materialization:"} {
		if strings.HasPrefix(ref.ProducerID, prefix) {
			return true
		}
	}
	return false
}

// Check origin authority on reads as well as dispatch. Descendants must not
// turn a revoked domain report into a readable copy. Historical external roots
// outside these registered producers still require their own migration.
func (s *Service) validateOriginChain(ctx context.Context, p d.Principal, root d.DataAssetRef) error {
	visited, active := map[string]bool{}, map[string]bool{}
	var visit func(d.DataAssetRef, int) error
	visit = func(ref d.DataAssetRef, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		key := ref.RefID.String()
		if depth > 16 || active[key] || len(ref.Parents) > 16 {
			return d.ErrInvalid
		}
		if visited[key] {
			return nil
		}
		if len(visited)+len(active) >= 128 {
			return d.ErrInvalid
		}
		if ref.TenantID != p.TenantID || ref.Status != "available" || (ref.ExpiresAt != nil && !ref.ExpiresAt.After(time.Now())) {
			return d.ErrForbidden
		}
		if ref.Policy.Sensitivity == "restricted" || ref.Policy.Sensitivity == "confidential" || ref.Policy.Sensitivity == "secret" {
			if err := s.authorize(ctx, p, "sensitive"); err != nil {
				return d.ErrForbidden
			}
		}
		active[key] = true
		defer delete(active, key)
		if governedOrigin(ref) {
			if s.config.ValidateGovernedOrigin == nil {
				return d.ErrForbidden
			}
			if err := s.config.ValidateGovernedOrigin(ctx, p, ref); err != nil {
				return err
			}
		}
		for _, id := range ref.Parents {
			if id.Validate() != nil {
				return d.ErrInvalid
			}
			parent, err := s.repo.FindAsset(ctx, p.TenantID, id)
			if err != nil {
				return governedSourceError(err)
			}
			if err = visit(parent, depth+1); err != nil {
				return err
			}
		}
		visited[key] = true
		return nil
	}
	return visit(root, 0)
}
