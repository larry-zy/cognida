package knowledge

import (
	d "cognida/internal/model/feedback"
	g "cognida/internal/model/governance"
	k "cognida/internal/model/knowledge"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/service/governance"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"sort"
	"strings"
)

// CandidateDocumentProcessor is intentionally absent from HTTP request bodies.
// Its immutable ID must already be registered in the feedback publication ledger.
type CandidateDocumentProcessor interface {
	ProcessCandidate(context.Context, l.Principal, string, *ProcessDocumentRequest) (*ProcessDocumentResponse, error)
}

func (s *documentProcessorService) ProcessCandidate(ctx context.Context, p l.Principal, id string, req *ProcessDocumentRequest) (*ProcessDocumentResponse, error) {
	if p.Validate() != nil || req == nil || len(req.Content) == 0 || len(req.Content) > 16000 || s.embedder == nil || s.vectorRepo == nil {
		return nil, d.ErrInvalid
	}
	if _, e := uuid.Parse(id); e != nil {
		return nil, d.ErrInvalid
	}
	if _, e := s.requireIngestionKnowledgeBase(ctx, req.KnowledgeBaseID, p.TenantID); e != nil {
		return nil, e
	}
	locker, ok := s.knowledgeRepo.(DocumentIngestionLocker)
	if !ok {
		return nil, d.ErrConflict
	}
	var result *ProcessDocumentResponse
	err := locker.WithDocumentLock(ctx, id, func() error {
		old, e := s.knowledgeRepo.FindByID(ctx, id)
		if e != nil && !errors.Is(e, k.ErrKnowledgeNotFound) {
			return e
		}
		if old != nil {
			return fmt.Errorf("%w: existing candidate must be reconciled, never reprocessed", d.ErrConflict)
		}
		copy := *req
		copy.IdempotencyKey = ""
		copy.DocumentID = ""
		copy.creationID = id
		copy.contentHash = d.Digest(string(copy.Content))
		copy.candidateOnly = true
		copy.GraphEnabled = false
		copy.ChunkSize = 512
		copy.ChunkOverlap = 0
		copy.ChunkOverlapSet = true
		copy.ChunkStrategy = "fixed_size"
		result, e = s.processDocument(ctx, p.TenantID, p.UserID, &copy)
		return e
	})
	return result, err
}

type FeedbackProjectionReader interface {
	FeedbackProjection(context.Context, int64, string, string) (map[string]string, error)
}
type FeedbackProjection struct {
	KBs         k.KnowledgeBaseRepository
	Documents   k.KnowledgeRepository
	Chunks      k.ChunkRepository
	Processor   CandidateDocumentProcessor
	Indices     FeedbackProjectionReader
	deleteBound func(context.Context, l.Principal, string, string) error
	Governance  *governance.Service
}

func NewFeedbackProjection(kbs k.KnowledgeBaseRepository, docs k.KnowledgeRepository, chunks k.ChunkRepository, processor DocumentProcessorService, indices FeedbackProjectionReader, kb KnowledgeBaseService, gov *governance.Service) *FeedbackProjection {
	candidate, _ := processor.(CandidateDocumentProcessor)
	out := &FeedbackProjection{KBs: kbs, Documents: docs, Chunks: chunks, Processor: candidate, Indices: indices, Governance: gov}
	if cleanup, ok := kb.(interface {
		deleteFeedbackProjection(context.Context, l.Principal, string, string) error
	}); ok {
		out.deleteBound = cleanup.deleteFeedbackProjection
	}
	return out
}
func (p *FeedbackProjection) Visible(ctx context.Context, who l.Principal, kb, id string, allowDisabled bool) (bool, error) {
	if e := p.target(ctx, who, kb); e != nil {
		return false, e
	}
	doc, e := p.Documents.FindByID(ctx, id)
	if errors.Is(e, k.ErrKnowledgeNotFound) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	return doc != nil && doc.DeletedAt == nil && doc.TenantID == who.TenantID && doc.KnowledgeBaseID == kb && doc.ParseStatus == "completed" && (allowDisabled || doc.IsEnabled()), nil
}
func (p *FeedbackProjection) target(ctx context.Context, who l.Principal, kbID string) error {
	if who.Validate() != nil || p.KBs == nil {
		return d.ErrForbidden
	}
	kb, e := p.KBs.FindByIDForTenant(ctx, kbID, who.TenantID)
	if e != nil {
		if errors.Is(e, k.ErrKnowledgeBaseNotFound) {
			return d.ErrForbidden
		}
		return e
	}
	if kb == nil || kb.DeletedAt != nil || !kb.IsActive() || !kb.IsPublic && kb.UserID != who.UserID {
		return d.ErrForbidden
	}
	return nil
}
func (p *FeedbackProjection) text(ctx context.Context, who l.Principal, kb, id string, enabled bool) (*k.Knowledge, string, error) {
	if e := p.target(ctx, who, kb); e != nil {
		return nil, "", e
	}
	doc, e := p.Documents.FindByID(ctx, id)
	if e != nil {
		if errors.Is(e, k.ErrKnowledgeNotFound) {
			return nil, "", d.ErrForbidden
		}
		return nil, "", e
	}
	if doc == nil || doc.DeletedAt != nil || doc.TenantID != who.TenantID || doc.KnowledgeBaseID != kb || enabled && !doc.IsEnabled() {
		return nil, "", d.ErrForbidden
	}
	chunks, e := p.Chunks.FindByKnowledgeID(ctx, id, false)
	if e != nil {
		return nil, "", e
	}
	sort.Slice(chunks, func(i, j int) bool { return chunks[i].ChunkIndex < chunks[j].ChunkIndex })
	var text strings.Builder
	for _, chunk := range chunks {
		if chunk.DeletedAt != nil || chunk.TenantID != who.TenantID || chunk.KnowledgeBaseID != kb || !chunk.IsEnabled {
			return nil, "", d.ErrForbidden
		}
		text.WriteString(chunk.Content)
		text.WriteByte('\n')
		if text.Len() > 800000 {
			return nil, "", d.ErrInvalid
		}
	}
	if len(chunks) == 0 {
		return nil, "", d.ErrForbidden
	}
	return doc, text.String(), nil
}
func (p *FeedbackProjection) Baseline(ctx context.Context, who l.Principal, ref l.DataAssetRef, kb string) (string, string, error) {
	if ref.Provenance.SourceType != "knowledge" || ref.Provenance.SourceID == "" {
		return "", "", fmt.Errorf("%w: baseline must be an imported knowledge document", d.ErrInvalid)
	}
	_, content, e := p.text(ctx, who, kb, ref.Provenance.SourceID, true)
	if e != nil {
		return "", "", e
	}
	return ref.Provenance.SourceID, d.Digest(content), nil
}
func (p *FeedbackProjection) Validate(ctx context.Context, who l.Principal, kb, id, digest string) error {
	_, content, e := p.text(ctx, who, kb, id, true)
	if e != nil {
		return e
	}
	if d.Digest(content) != digest {
		return d.ErrForbidden
	}
	return nil
}
func (p *FeedbackProjection) Prepare(ctx context.Context, who l.Principal, c *d.Case, v *d.Publication, content string) (json.RawMessage, error) {
	if p.Processor == nil || p.Indices == nil {
		return nil, d.ErrConflict
	}
	if e := p.target(ctx, who, c.KnowledgeBaseID); e != nil {
		return nil, e
	}
	result, e := p.Processor.ProcessCandidate(ctx, who, v.KnowledgeID, &ProcessDocumentRequest{KnowledgeBaseID: c.KnowledgeBaseID, Title: c.Title, FileName: "feedback-" + v.ID + ".txt", FileType: "txt", Content: []byte(content)})
	if e != nil {
		return nil, e
	}
	if result == nil || result.KnowledgeID != v.KnowledgeID || result.ParseStatus != "completed" || !result.Vectorized {
		return nil, d.ErrConflict
	}
	return p.Lookup(ctx, who, c, v)
}
func (p *FeedbackProjection) Lookup(ctx context.Context, who l.Principal, c *d.Case, v *d.Publication) (json.RawMessage, error) {
	if e := p.target(ctx, who, c.KnowledgeBaseID); e != nil {
		return nil, e
	}
	doc, e := p.Documents.FindByID(ctx, v.KnowledgeID)
	if errors.Is(e, k.ErrKnowledgeNotFound) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if doc == nil {
		return nil, nil
	}
	if doc.TenantID != who.TenantID || doc.KnowledgeBaseID != c.KnowledgeBaseID || doc.UserID != who.UserID || doc.DeletedAt != nil || doc.FileHash != v.ContentDigest {
		return nil, d.ErrForbidden
	}
	if doc.ParseStatus != "completed" {
		return nil, nil
	}
	if p.Indices == nil {
		return nil, d.ErrConflict
	}
	chunks, e := p.Chunks.FindByKnowledgeID(ctx, doc.ID, false)
	if e != nil {
		return nil, e
	}
	if len(chunks) == 0 || len(chunks) > 64 {
		return nil, d.ErrConflict
	}
	actual, e := p.Indices.FeedbackProjection(ctx, who.TenantID, c.KnowledgeBaseID, doc.ID)
	if e != nil {
		return nil, e
	}
	if len(actual) != len(chunks) {
		return nil, nil
	}
	ids := []string{}
	for _, chunk := range chunks {
		if chunk.TenantID != who.TenantID || chunk.DeletedAt != nil || actual[chunk.ID] != chunk.Content {
			return nil, nil
		}
		ids = append(ids, chunk.ID)
	}
	sort.Strings(ids)
	return json.Marshal(map[string]any{"knowledge_id": doc.ID, "chunk_ids": ids, "content_digest": v.ContentDigest, "index": "milvus", "state": "ready", "graph": "not_requested", "projection_digest": d.Digest(actual)})
}
func (p *FeedbackProjection) Remove(ctx context.Context, who l.Principal, kb, id string) error {
	if p.deleteBound == nil {
		return d.ErrConflict
	}
	return p.deleteBound(ctx, who, kb, id)
}

// Only the feedback publication ledger supplies these immutable bindings. This
// package-private capability is not part of KnowledgeBaseService or any HTTP
// route: deleting a KB must not strand its already-owned candidate projections.
func (s *knowledgeBaseService) deleteFeedbackProjection(ctx context.Context, who l.Principal, kb, id string) error {
	if who.Validate() != nil || kb == "" || id == "" || s.knowledgeRepo == nil {
		return d.ErrForbidden
	}
	doc, e := s.knowledgeRepo.FindByID(ctx, id)
	if errors.Is(e, k.ErrKnowledgeNotFound) {
		return nil
	}
	if e != nil {
		return e
	}
	if doc == nil {
		return nil
	}
	if doc.TenantID != who.TenantID || doc.UserID != who.UserID || doc.KnowledgeBaseID != kb {
		return d.ErrForbidden
	}
	if s.statsQuerier == nil || s.vectorRepo == nil {
		return d.ErrConflict
	}
	return s.deleteKnowledgeProjection(ctx, kb, id, who.TenantID)
}
func (p *FeedbackProjection) Labels(ctx context.Context, who l.Principal, id string, version int, source l.RefID, taskID string) ([]string, error) {
	if p.Governance == nil {
		return nil, d.ErrConflict
	}
	r, e := p.Governance.Get(ctx, who, "tagging_result", id)
	if e != nil {
		return nil, e
	}
	if r.Version != version || r.Status != "approved" || r.ParentID != taskID {
		return nil, d.ErrConflict
	}
	var result g.TaggingResult
	if json.Unmarshal(r.Payload, &result) != nil || result.ReviewStatus != "approved" || result.ReviewedBy < 1 {
		return nil, d.ErrConflict
	}
	task, e := p.Governance.Get(ctx, who, "tagging_task", r.ParentID)
	if e != nil {
		return nil, e
	}
	var input g.TaggingTask
	if json.Unmarshal(task.Payload, &input) != nil || input.SourceRefID != source.String() {
		return nil, d.ErrForbidden
	}
	labels := append([]string{}, result.Tags...)
	if result.Category != "" {
		labels = append(labels, result.Category)
	}
	sort.Strings(labels)
	return labels, nil
}

func (p *FeedbackProjection) CurrentLabels(ctx context.Context, who l.Principal, id string) ([]string, error) {
	if p.Governance == nil {
		return nil, d.ErrConflict
	}
	labels, e := p.Governance.KnowledgeLabels(ctx, who, id)
	if labels == nil {
		labels = []string{}
	}
	sort.Strings(labels)
	return labels, e
}
