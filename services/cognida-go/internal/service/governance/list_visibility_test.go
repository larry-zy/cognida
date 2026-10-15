package governance

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

type visibilityRepo struct {
	d.Repository
	filter d.Filter
}

func (r *visibilityRepo) List(_ context.Context, _ int64, f d.Filter) (*d.Page[d.Record], error) {
	r.filter = f
	return &d.Page[d.Record]{Items: []d.Record{}}, nil
}
func TestLabelTaskListBindsPrivateTaskVisibilityToCurrentUser(t *testing.T) {
	r := &visibilityRepo{}
	s := NewService(r, nil, nil, nil, func(context.Context, l.Principal, string) error { return nil })
	for _, kind := range []string{"annotation_task", "tagging_task"} {
		_, err := s.List(context.Background(), l.Principal{TenantID: 1, UserID: 23}, d.Filter{Kind: kind, GovernedOwnerID: 99, Cursor: "cursor", Limit: 1})
		require.NoError(t, err)
		require.EqualValues(t, 23, r.filter.GovernedOwnerID, "a caller cannot request another governed owner's private tasks")
		require.Zero(t, r.filter.UserID, "ordinary shared tasks retain source-policy authorization")
		require.Equal(t, "cursor", r.filter.Cursor)
	}
}
