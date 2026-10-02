package collection

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	domain "cognida/internal/model/collection"
	knowledgeModel "cognida/internal/model/knowledge"
	"cognida/internal/service/collection/fetcher"
	knowledge "cognida/internal/service/knowledge"
)

// SafeFetcherAdapter makes the safe fetcher conform to the collection-domain
// port while applying the task's response budget per request.
type SafeFetcherAdapter struct{ Gate fetcher.RequestGate }

func (a SafeFetcherAdapter) Fetch(ctx context.Context, request domain.FetchRequest) (*domain.FetchResponse, error) {
	client := fetcher.New(fetcher.Options{MaxBytes: request.MaxResponseBytes, Gate: a.Gate})
	result, err := client.Fetch(ctx, request.URL)
	if err != nil {
		return nil, err
	}
	return &domain.FetchResponse{CanonicalURL: result.URL, StatusCode: result.StatusCode, ContentType: result.ContentType, Body: result.Body}, nil
}

// KnowledgeIngestorAdapter is the intentionally narrow boundary into the
// established document processor. It attaches URL provenance without exposing
// collection state to the knowledge bounded context.
type KnowledgeIngestorAdapter struct {
	processor knowledge.DocumentProcessorService
}

func NewKnowledgeIngestorAdapter(processor knowledge.DocumentProcessorService) *KnowledgeIngestorAdapter {
	return &KnowledgeIngestorAdapter{processor: processor}
}

func (a *KnowledgeIngestorAdapter) Ingest(ctx context.Context, request domain.IngestRequest) (*domain.IngestResult, error) {
	if len(request.ContentHash) != 64 {
		return nil, fmt.Errorf("invalid content hash")
	}
	u, err := url.Parse(request.CanonicalURL)
	if err != nil {
		return nil, fmt.Errorf("parse canonical URL: %w", err)
	}
	title := u.Hostname()
	if p := strings.Trim(u.Path, "/"); p != "" {
		title += " / " + p
	}
	response, err := a.processor.ProcessDocument(ctx, request.TenantID, request.UserID, &knowledge.ProcessDocumentRequest{
		KnowledgeBaseID: request.KnowledgeBaseID,
		IdempotencyKey:  request.ContentHash,
		Title:           title,
		FileName:        "collected-" + request.ContentHash[:12] + ".txt",
		FileType:        "txt",
		URL:             request.CanonicalURL,
		Content:         []byte(request.Content),
	})
	if err != nil {
		return nil, err
	}
	if response == nil || response.ParseStatus != knowledgeModel.ParseStatusCompleted || response.KnowledgeID == "" {
		return nil, fmt.Errorf("document ingestion did not complete successfully")
	}
	return &domain.IngestResult{KnowledgeID: response.KnowledgeID}, nil
}

var _ domain.Fetcher = SafeFetcherAdapter{}
var _ domain.DocumentIngestor = (*KnowledgeIngestorAdapter)(nil)
