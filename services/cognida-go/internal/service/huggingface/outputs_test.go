package huggingface

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	d "cognida/internal/model/huggingface"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/service/collection/fetcher"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type outputAssets struct {
	Assets
	ref l.DataAssetRef
	err error
}

func (a *outputAssets) Resolve(context.Context, int64, int64, l.RefID) (l.DataAssetRef, json.RawMessage, error) {
	return a.ref, nil, a.err
}

type outputRepository struct {
	d.Repository
	owner l.Principal
	value *d.Import
}

func (r *outputRepository) Get(_ context.Context, p l.Principal, id string) (*d.Import, error) {
	if p != r.owner || id != r.value.ID {
		return nil, d.ErrNotFound
	}
	return r.value, nil
}

type outputCredential func(context.Context, l.Principal, string) (fetcher.JSONCredential, error)

func (f outputCredential) Resolve(ctx context.Context, p l.Principal, id string) (fetcher.JSONCredential, error) {
	return f(ctx, p, id)
}

func TestValidateOutputsRequiresCurrentOriginAndAuthorization(t *testing.T) {
	p := l.Principal{TenantID: 1, UserID: 2}
	ref := l.RefID{AssetID: uuid.NewString(), Version: 1}
	for _, scenario := range []string{"allowed without decoder", "revoked principal", "revoked credential", "other owner", "other tenant", "asset forbidden", "asset expired", "wrong returned ref", "wrong kind", "unrelated producer", "uncommitted receipt", "missing manifest", "wrong manifest", "not in receipt"} {
		t.Run(scenario, func(t *testing.T) {
			assets := &outputAssets{ref: l.DataAssetRef{RefID: ref, TenantID: p.TenantID, Kind: l.RawAsset, ProducerID: "huggingface-import:receipt"}}
			receipt := &d.Import{ID: "receipt", Status: "succeeded", RequestHash: "request-digest", Request: d.Request{CredentialID: "credential"}, Manifest: &d.Manifest{RequestHash: "request-digest"}, OutputRefs: []l.RefID{ref}}
			repo := &outputRepository{owner: p, value: receipt}
			credentialCalls := 0
			s := &Service{repo: repo, assets: assets, authorize: func(context.Context, l.Principal, string) error {
				if scenario == "revoked principal" {
					return l.ErrForbidden
				}
				return nil
			}, credentials: outputCredential(func(_ context.Context, owner l.Principal, id string) (fetcher.JSONCredential, error) {
				credentialCalls++
				require.Equal(t, p, owner)
				require.Equal(t, "credential", id)
				if scenario == "revoked credential" {
					return fetcher.JSONCredential{}, d.ErrForbidden
				}
				return fetcher.JSONCredential{}, nil
			})}
			owner := p
			switch scenario {
			case "other owner":
				owner.UserID++
			case "other tenant":
				owner.TenantID++
			case "asset forbidden":
				assets.err = l.ErrForbidden
			case "asset expired":
				assets.err = l.ErrExpired
			case "wrong returned ref":
				assets.ref.Version++
			case "wrong kind":
				assets.ref.Kind = l.PublishedAsset
			case "unrelated producer":
				assets.ref.ProducerID = "upload"
			case "uncommitted receipt":
				receipt.Status = "running"
			case "missing manifest":
				receipt.Manifest = nil
			case "wrong manifest":
				receipt.Manifest.RequestHash = "other"
			case "not in receipt":
				receipt.OutputRefs = nil
			}
			err := s.ValidateOutputs(context.Background(), owner, []l.RefID{ref})
			if scenario == "allowed without decoder" {
				require.NoError(t, err)
				require.Equal(t, 1, credentialCalls)
			} else {
				require.ErrorIs(t, err, d.ErrForbidden)
			}
		})
	}
}

func TestValidateOutputsFailClosedBeforeDependencies(t *testing.T) {
	s := &Service{authorize: func(context.Context, l.Principal, string) error { return nil }}
	p := l.Principal{TenantID: 1, UserID: 2}
	ref := l.RefID{AssetID: uuid.NewString(), Version: 1}
	for _, refs := range [][]l.RefID{nil, {{}}, {ref, ref}} {
		require.ErrorIs(t, s.ValidateOutputs(context.Background(), p, refs), d.ErrInvalid)
	}
	require.ErrorIs(t, s.ValidateOutputs(context.Background(), l.Principal{}, []l.RefID{ref}), d.ErrForbidden)
	require.ErrorIs(t, s.ValidateOutputs(context.Background(), p, []l.RefID{ref}), d.ErrUnavailable)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, s.ValidateOutputs(ctx, p, []l.RefID{ref}), context.Canceled)
	s.assets = &outputAssets{err: errors.New("private storage diagnostic")}
	s.repo = &outputRepository{}
	require.Equal(t, d.ErrUpstream, s.ValidateOutputs(context.Background(), p, []l.RefID{ref}))
}
