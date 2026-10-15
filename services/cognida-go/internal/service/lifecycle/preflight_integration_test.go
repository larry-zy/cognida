//go:build integration

package lifecycle

import (
	d "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPublicationPreflightRejectsBeforePersistAndRechecksAfterApproval(t *testing.T) {
	s, db, q, p, _ := integrationService(t)
	ctx := context.Background()
	valid := true
	calls := 0
	s.config.ValidatePublication = func(_ context.Context, principal d.Principal, in d.Input, opts map[string]any) error {
		calls++
		require.Equal(t, p, principal)
		if in.Kind == "text" && opts["knowledge_base_id"] != "approved-kb" {
			return d.ErrInvalid
		}
		if !valid {
			return d.ErrForbidden
		}
		return nil
	}
	raw, err := s.Import(ctx, p, d.Input{Kind: "text", Text: "publication evidence"}, "source")
	require.NoError(t, err)
	_, err = s.StartRun(ctx, p, "web-knowledge-to-insight", []d.RefID{raw.RefID}, nil, "bad-target", "")
	require.ErrorIs(t, err, d.ErrInvalid)
	var count int64
	require.NoError(t, db.Table("lifecycle_runs").Where("tenant_id = ?", p.TenantID).Count(&count).Error)
	require.Zero(t, count)
	opts := map[string]any{"stage_options": map[string]any{"asset": map[string]any{"knowledge_base_id": "approved-kb"}}}
	// Use the actual publication stage identifier from the registered definition.
	for _, stage := range DefaultTemplate().Stages {
		if stage.AgentID == "data-asset" {
			opts["stage_options"] = map[string]any{stage.ID: map[string]any{"knowledge_base_id": "approved-kb"}}
		}
	}
	run, err := s.StartRun(ctx, p, "web-knowledge-to-insight", []d.RefID{raw.RefID}, opts, "valid-target", "")
	require.NoError(t, err)
	drain(t, s, q)
	approvals, err := s.repo.ListApprovals(ctx, p.TenantID, run.ID)
	require.NoError(t, err)
	require.Len(t, approvals, 1)
	valid = false
	require.NoError(t, s.ResolveApproval(ctx, p, approvals[0].ID, "approved", "reviewed"))
	// Call the adapter with the approved stage to exercise authorization at the write boundary.
	stages, err := s.repo.ListStages(ctx, p.TenantID, run.ID)
	require.NoError(t, err)
	for _, stage := range stages {
		if stage.AgentID == "data-asset" {
			ref, raw, err := s.Resolve(ctx, p.TenantID, p.UserID, stage.InputRefs[0])
			require.NoError(t, err)
			stage.Approved = true
			_, err = s.publish(ctx, d.StageRequest{Principal: p, Stage: stage, Inputs: []d.DataAssetRef{ref}, Payloads: []json.RawMessage{raw}})
			require.ErrorIs(t, err, d.ErrForbidden)
		}
	}
	require.GreaterOrEqual(t, calls, 3)
}
