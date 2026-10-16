package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	d "cognida/internal/model/lifecycle"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type publicationRepository struct {
	*governedMemoryRepo
	approval d.Approval
}

func (r *publicationRepository) FindApproval(_ context.Context, tenant int64, id string, _ bool) (*d.Approval, error) {
	if tenant != r.approval.TenantID || id != r.approval.ID {
		return nil, d.ErrNotFound
	}
	copy := r.approval
	return &copy, nil
}
func TestKnowledgePublicationRequiresApprovedBoundReceiptAndExactAncestry(t *testing.T) {
	for _, scenario := range []string{"valid", "indirect_parent", "missing_source", "changed_source", "wrong_owner", "wrong_kind", "not_approved", "failed_stage", "wrong_agent", "missing_output", "wrong_output_digest", "wrong_input", "approval_pending", "approval_actor_missing", "approval_time_missing", "approval_stage", "receipt_mismatch", "wrong_kb", "revoked", "expired", "cycle", "invalid_ref", "policy_unapproved", "missing_chunk_digest", "missing_content_digest", "target_revoked"} {
		t.Run(scenario, func(t *testing.T) {
			s, r, _, p, source := governedFixture(t)
			ctx := context.Background()
			_, sourceRaw, err := s.Resolve(ctx, p.TenantID, p.UserID, source.RefID)
			require.NoError(t, err)
			var input d.Input
			require.NoError(t, json.Unmarshal(sourceRaw, &input))
			receipt := map[string]any{"knowledge_id": "document", "knowledge_base_id": "kb", "publication_status": "published", "content_digest": fmt.Sprintf("%x", sha256.Sum256([]byte(input.Text))), "chunk_digest": strings.Repeat("a", 64)}
			parent := source
			if scenario == "indirect_parent" {
				parent, err = s.ImportDerived(ctx, p, d.CleanedAsset, uuid.NewString(), []d.RefID{source.RefID}, sourceRaw, source.Policy)
				require.NoError(t, err)
			}
			raw, _ := json.Marshal(map[string]any{"kind": "text", "text": input.Text, "publication": receipt})
			stage := d.StageRun{ID: uuid.NewString(), TenantID: p.TenantID, UserID: p.UserID, AgentID: "data-asset", Status: d.StatusSucceeded, Approved: true, InputRefs: []d.RefID{parent.RefID}, Options: map[string]any{"knowledge_base_id": "kb"}}
			output, err := s.ImportDerived(ctx, p, d.PublishedAsset, stage.ID, []d.RefID{parent.RefID}, raw, d.PolicySnapshot{Approved: true})
			require.NoError(t, err)
			out := passedOutput("Published", output)
			out.Metadata = receipt
			stage.Output = &out
			now := time.Now().UTC()
			approval := d.Approval{ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte("approval:"+stage.ID)).String(), TenantID: p.TenantID, StageID: stage.ID, Status: "approved", ResolvedBy: p.UserID, ResolvedAt: &now}
			publishedID, sourceID := output.RefID, source.RefID
			switch scenario {
			case "missing_source":
				sourceID = d.RefID{AssetID: uuid.NewString(), Version: 1}
			case "changed_source":
				input.Text += "changed"
			case "wrong_owner":
				p.UserID++
			case "wrong_kind":
				output.Kind = d.CleanedAsset
			case "not_approved":
				stage.Approved = false
			case "failed_stage":
				stage.Status = d.StatusFailed
			case "wrong_agent":
				stage.AgentID = "data-cleaning"
			case "missing_output":
				stage.Output.Refs = nil
			case "wrong_output_digest":
				stage.Output.Refs[0].Provenance.Digest = "changed"
			case "wrong_input":
				stage.InputRefs = nil
			case "approval_pending":
				approval.Status = "pending"
			case "approval_actor_missing":
				approval.ResolvedBy = 0
			case "approval_time_missing":
				approval.ResolvedAt = nil
			case "approval_stage":
				approval.StageID = uuid.NewString()
			case "receipt_mismatch":
				stage.Output.Metadata = map[string]any{"knowledge_id": "unrelated"}
			case "wrong_kb":
				stage.Options["knowledge_base_id"] = "another"
			case "revoked":
				s.config.Authorize = func(context.Context, d.Principal, string) error { return d.ErrForbidden }
			case "expired":
				past := time.Now().Add(-time.Hour)
				source.ExpiresAt = &past
				r.assets[source.RefID.String()] = source
			case "cycle":
				source.Parents = []d.RefID{publishedID}
				r.assets[source.RefID.String()] = source
			case "policy_unapproved":
				output.Policy.Approved = false
			case "missing_chunk_digest":
				delete(receipt, "chunk_digest")
				raw, _ = json.Marshal(map[string]any{"kind": "text", "text": input.Text, "publication": receipt})
				_, err = approvedKnowledgePublication(p, output, raw, &stage, &approval, input.Text)
				require.ErrorIs(t, err, d.ErrForbidden)
				return
			case "missing_content_digest":
				delete(receipt, "content_digest")
				raw, _ = json.Marshal(map[string]any{"kind": "text", "text": input.Text, "publication": receipt})
				_, err = approvedKnowledgePublication(p, output, raw, &stage, &approval, input.Text)
				require.ErrorIs(t, err, d.ErrForbidden)
				return
			case "target_revoked":
				s.config.ValidatePublication = func(context.Context, d.Principal, d.Input, map[string]any) error { return d.ErrForbidden }
			case "invalid_ref":
				publishedID = d.RefID{}
			}
			r.assets[output.RefID.String()] = output
			r.stages[stage.ID] = stage
			s.repo = &publicationRepository{r, approval}
			id, err := s.ResolveKnowledgePublication(ctx, p, publishedID, sourceID, input.Text)
			if scenario == "valid" || scenario == "indirect_parent" {
				require.NoError(t, err)
				require.Equal(t, "document", id.KnowledgeID)
			} else {
				require.Error(t, err)
				require.Empty(t, id)
			}
		})
	}
}
