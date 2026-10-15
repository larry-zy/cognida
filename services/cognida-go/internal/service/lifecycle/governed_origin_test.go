package lifecycle

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	d "cognida/internal/model/lifecycle"
	"github.com/stretchr/testify/require"
)

func TestRegisteredOriginAndDescendantsRecheckAuthorityOnEveryRead(t *testing.T) {
	s, repo, _, p, input := governedFixture(t)
	ctx := context.Background()
	output, err := s.ImportDerived(ctx, p, d.Result, "feature-materialization:receipt", []d.RefID{input.RefID}, json.RawMessage(`{"count":1}`), input.Policy)
	require.NoError(t, err)
	_, _, err = s.Resolve(ctx, p.TenantID, p.UserID, output.RefID)
	require.ErrorIs(t, err, d.ErrForbidden, "a producer string alone grants no authority")
	allowed := true
	s.config.ValidateGovernedOrigin = func(_ context.Context, owner d.Principal, ref d.DataAssetRef) error {
		if !allowed || owner != p || ref.RefID != output.RefID {
			return d.ErrForbidden
		}
		return nil
	}
	copy, err := s.ImportDerived(ctx, p, d.Result, "copied-report", []d.RefID{output.RefID}, json.RawMessage(`{"copy":true}`), input.Policy)
	require.NoError(t, err)
	require.NoError(t, s.ValidateGovernedInputs(ctx, p, []d.RefID{copy.RefID}))
	allowed = false
	for _, id := range []d.RefID{output.RefID, copy.RefID} {
		_, _, err = s.Resolve(ctx, p.TenantID, p.UserID, id)
		require.ErrorIs(t, err, d.ErrForbidden)
	}
	allowed = true
	for _, change := range []func(*d.DataAssetRef){
		func(ref *d.DataAssetRef) { ref.Status = "revoked" },
		func(ref *d.DataAssetRef) { expired := time.Now().Add(-time.Minute); ref.ExpiresAt = &expired },
		func(ref *d.DataAssetRef) { ref.TenantID++ },
	} {
		revoked := input
		change(&revoked)
		repo.assets[input.RefID.String()] = revoked
		_, _, err = s.Resolve(ctx, p.TenantID, p.UserID, copy.RefID)
		require.Error(t, err, "ordinary reads must not bypass ancestor revocation")
	}
	repo.assets[input.RefID.String()] = input
	_, _, err = s.Resolve(ctx, p.TenantID, p.UserID, copy.RefID)
	require.NoError(t, err)
}

func TestOriginMetadataTraversalRejectsCyclesWithoutReadingBodies(t *testing.T) {
	s, repo, _, p, input := governedFixture(t)
	input.Parents = []d.RefID{input.RefID}
	repo.assets[input.RefID.String()] = input
	_, _, err := s.Resolve(context.Background(), p.TenantID, p.UserID, input.RefID)
	require.ErrorIs(t, err, d.ErrInvalid)
}
