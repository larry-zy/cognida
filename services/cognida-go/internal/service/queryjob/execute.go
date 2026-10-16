package queryjob

import (
	ds "cognida/internal/model/datasource"
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/queryjob"
	"context"
	"errors"
	"fmt"
	"time"
)

// Execute is the bounded bridge for existing sampling and Agent SQL consumers.
// Its key must come from the authenticated parent operation, never a random retry.
func (s *Service) Execute(ctx context.Context, tenant, user int64, sourceID string, in ds.QueryRequest, key string) (*ds.QueryResult, error) {
	// The durable HTTP contract stores literal read-only SQL. Do not silently
	// discard parameters or accept a remote operation ID chosen by a caller.
	if len(in.Parameters) != 0 || in.OperationID != "" {
		return nil, d.ErrInvalid
	}
	p := l.Principal{TenantID: tenant, UserID: user}
	j, e := s.Create(ctx, p, d.Input{DataSourceID: sourceID, SQL: in.SQL, MaxRows: in.MaxRows, MaxBytes: in.MaxBytes, TimeoutSeconds: int(in.Timeout / time.Second), MaxScanBytes: in.MaxScanBytes}, key)
	if e != nil {
		return nil, e
	}
	for {
		if ctx.Err() != nil {
			end, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_, _ = s.Cancel(end, p, j.ID)
			cancel()
			return nil, ctx.Err()
		}
		r, e := s.repo.Get(ctx, p, j.ID)
		if e != nil {
			return nil, e
		}
		if e = s.authorized(ctx, p, r); e != nil {
			return nil, e
		}
		if r.Job.Terminal() {
			if r.Job.Status != "succeeded" || r.Result == nil {
				return nil, fmt.Errorf("query job %s ended as %s", j.ID, r.Job.Status)
			}
			return r.Result, nil
		}
		if e = s.Tick(ctx, p, j.ID); e != nil && !errors.Is(e, d.ErrConflict) {
			return nil, e
		}
		select {
		case <-ctx.Done():
			continue
		case <-time.After(100 * time.Millisecond):
		}
	}
}
