package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	d "cognida/internal/model/lifecycle"
	"github.com/google/uuid"
)

// ResolveKnowledgePublication is a trusted provenance bridge, not a document
// search. It resolves only an actual human-approved publication and verifies
// every ancestor's current authority. The caller must still atomically check
// current knowledge bytes, KB authority and the approved label revision.
func (s *Service) ResolveKnowledgePublication(ctx context.Context, p d.Principal, publishedID, sourceID d.RefID, content string) (d.KnowledgePublication, error) {
	if publishedID.Validate() != nil || sourceID.Validate() != nil || content == "" {
		return d.KnowledgePublication{}, d.ErrInvalid
	}
	if err := s.authorize(ctx, p, "read"); err != nil {
		return d.KnowledgePublication{}, err
	}
	published, raw, err := s.Resolve(ctx, p.TenantID, p.UserID, publishedID)
	if err != nil {
		return d.KnowledgePublication{}, err
	}
	stage, err := s.repo.FindStage(ctx, p.TenantID, published.ProducerID, false)
	if err != nil {
		return d.KnowledgePublication{}, err
	}
	if stage == nil {
		return d.KnowledgePublication{}, d.ErrForbidden
	}
	approvalID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("approval:"+stage.ID)).String()
	approval, err := s.repo.FindApproval(ctx, p.TenantID, approvalID, false)
	if err != nil {
		return d.KnowledgePublication{}, err
	}
	knowledgeID, err := approvedKnowledgePublication(p, published, raw, stage, approval, content)
	if err != nil {
		return d.KnowledgePublication{}, err
	}
	if s.config.ValidatePublication != nil {
		if err := s.config.ValidatePublication(ctx, p, d.Input{Kind: "text", Text: content}, stage.Options); err != nil {
			return d.KnowledgePublication{}, err
		}
	}
	// A matching source must be on this publication's actual graph. Repeated
	// Resolve calls also verify object digests; no metadata-only ancestry shortcut.
	active, visited := map[string]bool{}, map[string]bool{}
	found := false
	var visit func(d.RefID, int) error
	visit = func(id d.RefID, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		key := id.String()
		if depth > 16 || active[key] || len(visited) >= 64 && !visited[key] {
			return d.ErrForbidden
		}
		if visited[key] {
			return nil
		}
		active[key], visited[key] = true, true
		defer delete(active, key)
		ref, body, err := s.Resolve(ctx, p.TenantID, p.UserID, id)
		if err != nil {
			return err
		}
		if id == sourceID {
			if !publicationTextMatches(body, content) {
				return d.ErrForbidden
			}
			found = true
		}
		if len(ref.Parents) > 16 {
			return d.ErrForbidden
		}
		for _, parent := range ref.Parents {
			if err := visit(parent, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	for _, parent := range published.Parents {
		if err := visit(parent, 0); err != nil {
			return d.KnowledgePublication{}, err
		}
	}
	if !found {
		return d.KnowledgePublication{}, d.ErrForbidden
	}
	return knowledgeID, nil
}

func publicationTextMatches(raw json.RawMessage, content string) bool {
	var input d.Input
	return json.Unmarshal(raw, &input) == nil && input.Kind == "text" && input.Validate() == nil && sha256.Sum256([]byte(input.Text)) == sha256.Sum256([]byte(content))
}

func approvedKnowledgePublication(p d.Principal, ref d.DataAssetRef, raw json.RawMessage, stage *d.StageRun, approval *d.Approval, content string) (d.KnowledgePublication, error) {
	if stage == nil || approval == nil || ref.Kind != d.PublishedAsset || !ref.Policy.Approved || ref.TenantID != p.TenantID || ref.ProducerID != stage.ID || ref.Provenance.SourceType != "lifecycle" || ref.Provenance.SourceID != stage.ID || len(ref.Parents) != 1 || stage.TenantID != p.TenantID || stage.UserID != p.UserID || stage.AgentID != "data-asset" || stage.Status != d.StatusSucceeded || !stage.Approved || stage.Output == nil || stage.Output.Gate.Status != d.GatePassed {
		return d.KnowledgePublication{}, d.ErrForbidden
	}
	if approval.TenantID != p.TenantID || approval.StageID != stage.ID || approval.RunID != stage.RunID || approval.Status != "approved" || approval.ResolvedBy <= 0 || approval.ResolvedAt == nil {
		return d.KnowledgePublication{}, d.ErrForbidden
	}
	parentBound, outputBound := false, false
	for _, id := range stage.InputRefs {
		if id == ref.Parents[0] {
			parentBound = true
		}
	}
	for _, out := range stage.Output.Refs {
		if out.RefID == ref.RefID && out.Kind == d.PublishedAsset && out.TenantID == ref.TenantID && out.ProducerID == stage.ID && out.Provenance.Digest == ref.Provenance.Digest && out.Policy.Approved && sameJSON(out.Parents, ref.Parents) {
			outputBound = true
		}
	}
	if !parentBound || !outputBound || !publicationTextMatches(raw, content) {
		return d.KnowledgePublication{}, d.ErrForbidden
	}
	var payload struct {
		Publication map[string]any `json:"publication"`
	}
	if json.Unmarshal(raw, &payload) != nil || !sameJSON(payload.Publication, stage.Output.Metadata) {
		return d.KnowledgePublication{}, d.ErrForbidden
	}
	id, idOK := payload.Publication["knowledge_id"].(string)
	kb, kbOK := payload.Publication["knowledge_base_id"].(string)
	status, _ := payload.Publication["publication_status"].(string)
	target, _ := stage.Options["knowledge_base_id"].(string)
	if !idOK || id == "" || !kbOK || kb == "" || kb != target || status != "published" {
		return d.KnowledgePublication{}, d.ErrForbidden
	}
	contentDigest, _ := payload.Publication["content_digest"].(string)
	chunkDigest, _ := payload.Publication["chunk_digest"].(string)
	expected := sha256.Sum256([]byte(content))
	if contentDigest != hex.EncodeToString(expected[:]) || !publicationDigest(chunkDigest) {
		return d.KnowledgePublication{}, d.ErrForbidden
	}
	return d.KnowledgePublication{KnowledgeID: id, ContentDigest: contentDigest, ChunkDigest: chunkDigest}, nil
}

func publicationDigest(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
