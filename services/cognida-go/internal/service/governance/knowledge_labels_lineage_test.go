package governance

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type labelSourceFixture struct {
	refs   map[string]l.DataAssetRef
	inputs map[string]l.Input
	denied string
	err    error
}

func (f *labelSourceFixture) Resolve(_ context.Context, tenant, user int64, id l.RefID) (l.DataAssetRef, json.RawMessage, error) {
	if f.err != nil {
		return l.DataAssetRef{}, nil, f.err
	}
	if tenant != 7 || user != 8 || id.String() == f.denied {
		return l.DataAssetRef{}, nil, l.ErrForbidden
	}
	ref, ok := f.refs[id.String()]
	if !ok {
		return l.DataAssetRef{}, nil, l.ErrNotFound
	}
	return ref, mustLabelJSON(f.inputs[id.String()]), nil
}
func (*labelSourceFixture) ImportDerived(context.Context, l.Principal, l.Kind, string, []l.RefID, json.RawMessage, l.PolicySnapshot) (l.DataAssetRef, error) {
	return l.DataAssetRef{}, l.ErrInvalid
}
func (f *labelSourceFixture) add(kind, knowledge string, parents ...l.RefID) l.RefID {
	id := l.RefID{AssetID: uuid.NewString(), Version: 1}
	f.refs[id.String()] = l.DataAssetRef{RefID: id, AssetRefID: id.String(), Kind: l.CleanedAsset, Parents: parents, Provenance: l.Provenance{SourceType: kind, SourceID: knowledge}}
	f.inputs[id.String()] = l.Input{Kind: "text", Text: "sales facts\n"}
	return id
}

func TestKnowledgeLabelSourcePreparedLineageAndRejections(t *testing.T) {
	p := l.Principal{TenantID: 7, UserID: 8}
	for _, scenario := range []string{"prepared", "report_parent", "legacy", "changed_snapshot", "changed_original", "ambiguous", "missing", "revoked", "cycle", "too_many_parents", "depth", "nodes", "storage", "cancelled", "other_user"} {
		t.Run(scenario, func(t *testing.T) {
			f := &labelSourceFixture{refs: map[string]l.DataAssetRef{}, inputs: map[string]l.Input{}}
			original := f.add("knowledge", "doc-one")
			clean := f.add("lifecycle", "", original)
			prepared := f.add("lifecycle", "", clean)
			ctx := context.Background()
			principal := p
			want := error(d.ErrForbidden)
			switch scenario {
			case "prepared":
				want = nil
			case "report_parent":
				report := f.add("lifecycle", "", original)
				f.inputs[report.String()] = l.Input{Kind: "report"}
				prepared = f.add("lifecycle", "", clean, report)
				want = nil
			case "legacy":
				prepared, want = original, nil
			case "changed_snapshot":
				f.inputs[prepared.String()] = l.Input{Kind: "text", Text: "changed"}
			case "changed_original":
				f.inputs[original.String()] = l.Input{Kind: "text", Text: "changed"}
			case "ambiguous":
				other := f.add("knowledge", "doc-two")
				prepared = f.add("lifecycle", "", original, other)
			case "missing":
				prepared = f.add("manual", "")
			case "revoked":
				f.denied = original.String()
			case "cycle":
				r := f.refs[clean.String()]
				r.Parents = []l.RefID{prepared}
				f.refs[clean.String()] = r
			case "too_many_parents":
				parents := []l.RefID{}
				for range 17 {
					parents = append(parents, original)
				}
				prepared = f.add("lifecycle", "", parents...)
			case "depth":
				for range 17 {
					prepared = f.add("lifecycle", "", prepared)
				}
			case "nodes":
				branches := []l.RefID{}
				for range 16 {
					leaves := []l.RefID{}
					for range 4 {
						leaves = append(leaves, f.add("knowledge", "doc-one"))
					}
					branches = append(branches, f.add("lifecycle", "", leaves...))
				}
				prepared = f.add("lifecycle", "", branches...)
			case "storage":
				want = errors.New("storage unavailable")
				f.err = want
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
				want = context.Canceled
			case "other_user":
				principal.UserID++
			}
			s := &Service{Assets: f}
			id, err := s.knowledgeLabelSource(ctx, principal, prepared.String(), "sales facts\n")
			if want == nil {
				require.NoError(t, err)
				require.Equal(t, "doc-one", id)
			} else {
				require.ErrorIs(t, err, want)
				require.Empty(t, id)
			}
		})
	}
}
