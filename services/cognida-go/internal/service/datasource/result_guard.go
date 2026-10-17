package datasource

import (
	model "cognida/internal/model/datasource"
	"context"
	"strconv"
	"strings"
	"time"

	"cognida/internal/service/agent/resultstore"
)

// GuardResults makes current source availability authoritative even for cached
// query results. A changed connection is a new source version, not access to an
// old snapshot with potentially different ownership or credentials.
func (s *Service) GuardResults(base resultstore.Store) resultstore.Store {
	return resultstore.Guard(base, func(ctx context.Context, r *resultstore.Result) error {
		if r.DatasourceID == "" {
			return nil
		}
		owner, _, ok := strings.Cut(r.Owner, ":")
		if !ok {
			return resultstore.ErrUnauthorized
		}
		tenant, err := strconv.ParseInt(owner, 10, 64)
		if err != nil || tenant <= 0 {
			return resultstore.ErrUnauthorized
		}
		// The connection digest includes the encrypted credential. Never compute it
		// from the sanitized HTTP view, which intentionally omits that field.
		source, err := s.repo.Get(ctx, r.DatasourceID, tenant)
		if err != nil || source == nil || source.TenantID != tenant || source.ID != r.DatasourceID || source.Status == "disabled" || source.Status == "inactive" || source.Status == "need_credentials" {
			return resultstore.ErrUnauthorized
		}
		// Synchronous/older results used a timestamp; retain their original strict
		// binding while new async results use immutable connection identity.
		if !model.MatchesQueryConnectionVersion(source, r.SourceVersion) && source.UpdatedAt.UTC().Format(time.RFC3339Nano) != r.SourceVersion {
			return resultstore.ErrUnauthorized
		}
		return nil
	})
}
