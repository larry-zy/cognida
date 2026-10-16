package lifecycle

import (
	"context"
	"encoding/json"
	"testing"

	d "cognida/internal/model/lifecycle"
	"github.com/stretchr/testify/require"
)

func TestStandardizePreservesPrecisionAndExplicitProjection(t *testing.T) {
	in := d.Input{Kind: "table", Columns: []string{"id", "body", "gold"}, Rows: [][]any{{json.Number("9007199254740993"), "a\r\nb\rc", "answer"}}}
	opts := d.PreparationOptions{NormalizeNewlines: true, Fields: []d.PreparationField{{Source: "id", Name: "record_id", Type: "number", Required: true}, {Source: "body", Name: "text", Type: "text", Required: true}}}
	out, err := StandardizeInput(in, opts)
	require.NoError(t, err)
	require.Equal(t, []string{"record_id", "text"}, out.Columns)
	require.Equal(t, [][]any{{json.Number("9007199254740993"), "a\nb\nc"}}, out.Rows)
	require.Equal(t, "a\r\nb\rc", in.Rows[0][1], "input must remain unchanged")
}

func TestStandardizeRejectsInvalidFieldsAndRows(t *testing.T) {
	in := d.Input{Kind: "table", Columns: []string{"id", "json"}, Rows: [][]any{{"001", "[]"}, {"002", "{} trailing"}}}
	for _, fields := range [][]d.PreparationField{
		{{Source: "id", Name: "id", Type: "number"}},
		{{Source: "json", Name: "messages", Type: "json"}},
		{{Source: "missing", Name: "missing", Type: "any"}},
		{{Source: "id", Name: "same", Type: "any"}, {Source: "json", Name: "same", Type: "any"}},
	} {
		_, err := StandardizeInput(in, d.PreparationOptions{Fields: fields})
		require.ErrorIs(t, err, d.ErrInvalid)
	}
	in.Rows[1][0] = " "
	_, err := StandardizeInput(in, d.PreparationOptions{Fields: []d.PreparationField{{Source: "id", Name: "id", Type: "text", Required: true}}})
	require.ErrorIs(t, err, d.ErrInvalid)
}

func TestPreparationRequiresCleaningAndBindsExactOutput(t *testing.T) {
	s, r, q, p, raw := governedFixture(t)
	ctx := context.Background()
	_, err := s.PrepareAsset(ctx, p, raw.RefID, d.PreparationOptions{}, "raw-rejected", "")
	require.ErrorIs(t, err, d.ErrInvalid)
	input := d.Input{Kind: "table", Columns: []string{"text", "id"}, Rows: [][]any{{"a\r\nb", json.Number("9007199254740993")}}}
	body, err := json.Marshal(input)
	require.NoError(t, err)
	cleaned, err := s.ImportDerived(ctx, p, d.CleanedAsset, "clean-test", []d.RefID{raw.RefID}, body, raw.Policy)
	require.NoError(t, err)
	opts := d.PreparationOptions{NormalizeNewlines: true}
	stage, err := s.PrepareAsset(ctx, p, cleaned.RefID, opts, "prepared", "request")
	require.NoError(t, err)
	same, err := s.PrepareAsset(ctx, p, cleaned.RefID, opts, "prepared", "retry")
	require.NoError(t, err)
	require.Equal(t, stage.ID, same.ID)
	_, err = s.PrepareAsset(ctx, p, cleaned.RefID, d.PreparationOptions{}, "prepared", "")
	require.ErrorIs(t, err, d.ErrConflict)
	require.NoError(t, s.Handle(ctx, q.jobs[0]))
	stored := r.stages[stage.ID]
	require.Equal(t, d.StatusSucceeded, stored.Status)
	require.NotNil(t, stored.Output)
	require.Len(t, stored.Output.Refs, 1)
	ref, payload, err := s.Resolve(ctx, p.TenantID, p.UserID, stored.Output.Refs[0].RefID)
	require.NoError(t, err)
	require.True(t, d.IsPreparedAsset(ref))
	require.Equal(t, cleaned.Provenance.Digest, ref.Provenance.Preparation.SourceDigest)
	require.Contains(t, string(payload), "9007199254740993")
	require.Equal(t, []d.RefID{cleaned.RefID}, ref.Parents)
	ref.Provenance.Preparation.OutputDigest = "forged"
	require.False(t, d.IsPreparedAsset(ref))
}
