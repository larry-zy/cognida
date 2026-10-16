package datasource

import (
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
		source, err := s.Get(ctx, tenant, r.DatasourceID)
		if err != nil || source == nil || source.Status == "disabled" || source.Status == "inactive" || source.Status == "need_credentials" || source.UpdatedAt.UTC().Format(time.RFC3339Nano) != r.SourceVersion {
			return resultstore.ErrUnauthorized
		}
		return nil
	})
}
