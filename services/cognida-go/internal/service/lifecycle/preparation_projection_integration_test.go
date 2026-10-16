//go:build integration

package lifecycle

import (
	"context"
	"encoding/json"
	"testing"

	d "cognida/internal/model/lifecycle"
	"github.com/stretchr/testify/require"
)

func TestKnowledgeProjectionMySQLPreservesPreparedProofAndReplay(t *testing.T) {
	s, _, queue, p, _ := integrationService(t)
	ctx := context.Background()
	input := d.Input{Kind: "table", Columns: []string{"id", "contexts", "answer"}, Rows: [][]any{{"support-1", `[{"text":"Configure the documented application environment."}]`, "withheld answer"}}}
	raw, err := s.Import(ctx, p, input, "support source")
	require.NoError(t, err)
	body, err := json.Marshal(input)
	require.NoError(t, err)
	clean, err := s.ImportDerived(ctx, p, d.CleanedAsset, "test-cleaned-source", []d.RefID{raw.RefID}, body, raw.Policy)
	require.NoError(t, err)
	opts := d.PreparationOptions{TextProjection: &d.PreparationTextProjection{Column: "contexts", TextField: "text", IDColumn: "id"}}
	stage, err := s.PrepareAsset(ctx, p, clean.RefID, opts, "project-support", "request")
	require.NoError(t, err)
	drain(t, s, queue)
	replayed, err := s.PrepareAsset(ctx, p, clean.RefID, opts, "project-support", "replay")
	require.NoError(t, err)
	require.Equal(t, stage.ID, replayed.ID)
	require.Equal(t, d.StatusSucceeded, replayed.Status)
	require.Len(t, replayed.Output.Refs, 1)
	ref, payload, err := s.Resolve(ctx, p.TenantID, p.UserID, replayed.Output.Refs[0].RefID)
	require.NoError(t, err)
	require.True(t, d.IsPreparedAsset(ref))
	require.Equal(t, opts.TextProjection, ref.Provenance.Preparation.TextProjection)
	require.Equal(t, []d.RefID{clean.RefID}, ref.Parents)
	require.NotContains(t, string(payload), "withheld answer")
	require.Contains(t, string(payload), "Configure the documented application environment.")
	_, err = s.PrepareAsset(ctx, p, clean.RefID, d.PreparationOptions{}, "project-support", "changed")
	require.ErrorIs(t, err, d.ErrConflict)
}
