package governance

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
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

type sourceVisibilityRepo struct {
	d.Repository
	page d.Page[d.Record]
}

func (r *sourceVisibilityRepo) List(context.Context, int64, d.Filter) (*d.Page[d.Record], error) {
	out := r.page
	return &out, nil
}

type sourceVisibilityAssets struct {
	*unitAssets
	err error
}

func (a sourceVisibilityAssets) Resolve(ctx context.Context, tenant, user int64, id l.RefID) (l.DataAssetRef, json.RawMessage, error) {
	if a.err != nil {
		return l.DataAssetRef{}, nil, a.err
	}
	return a.unitAssets.Resolve(ctx, tenant, user, id)
}
func TestGovernanceListHidesRevokedSourcesPreservingCursor(t *testing.T) {
	p := l.Principal{TenantID: 1, UserID: 2}
	good := testAsset("good", time.Now())
	bad := testAsset("revoked", time.Now())
	assets := &unitAssets{refs: map[string]l.DataAssetRef{good.AssetRefID: good}}
	for _, kind := range []string{"annotation_task", "tagging_task", "feature", "feature_version"} {
		t.Run(kind, func(t *testing.T) {
			payload := func(ref string) json.RawMessage {
				raw, _ := json.Marshal(map[string]string{"source_ref_id": ref})
				return raw
			}
			r := &sourceVisibilityRepo{page: d.Page[d.Record]{Items: []d.Record{{Kind: kind, Meta: d.Meta{ID: "revoked"}, Payload: payload(bad.AssetRefID)}, {Kind: kind, Meta: d.Meta{ID: "visible"}, Payload: payload(good.AssetRefID)}}, NextCursor: "scanned", HasMore: true}}
			s := NewService(r, sourceVisibilityAssets{unitAssets: assets}, nil, nil, func(context.Context, l.Principal, string) error { return nil })
			page, err := s.List(context.Background(), p, d.Filter{Kind: kind})
			require.NoError(t, err)
			require.Len(t, page.Items, 1)
			require.Equal(t, "visible", page.Items[0].ID)
			require.Equal(t, "scanned", page.NextCursor)
			require.True(t, page.HasMore)
			s.Assets = sourceVisibilityAssets{unitAssets: assets, err: l.ErrExpired}
			page, err = s.List(context.Background(), p, d.Filter{Kind: kind})
			require.NoError(t, err)
			require.Empty(t, page.Items)
			require.Equal(t, "scanned", page.NextCursor)
			s.Assets = sourceVisibilityAssets{unitAssets: assets}
			r.page.Items = r.page.Items[:1]
			page, err = s.List(context.Background(), p, d.Filter{Kind: kind})
			require.NoError(t, err)
			require.Empty(t, page.Items)
			require.Equal(t, "scanned", page.NextCursor)
			outage := errors.New("source unavailable")
			s.Assets = sourceVisibilityAssets{unitAssets: assets, err: outage}
			_, err = s.List(context.Background(), p, d.Filter{Kind: kind})
			require.ErrorIs(t, err, outage)
		})
	}
}
