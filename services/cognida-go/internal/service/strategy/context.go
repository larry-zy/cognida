package strategy

import (
	d "cognida/internal/model/strategy"
	"context"
)

type snapshotKey struct{}

// WithSnapshot carries only validated immutable data; it never carries authorization.
func WithSnapshot(ctx context.Context, s *d.Snapshot) context.Context {
	return context.WithValue(ctx, snapshotKey{}, s)
}
func FromContext(ctx context.Context) *d.Snapshot {
	s, _ := ctx.Value(snapshotKey{}).(*d.Snapshot)
	return s
}
