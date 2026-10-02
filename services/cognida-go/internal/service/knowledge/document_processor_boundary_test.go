package knowledge

import (
	"context"
	"testing"

	"cognida/internal/model/docreader"
	domain "cognida/internal/model/knowledge"
)

type captureDocumentReader struct {
	docreader.DocumentReader
	request *docreader.ParseDocumentRequest
}

func (r *captureDocumentReader) ParseDocument(_ context.Context, req *docreader.ParseDocumentRequest) (*docreader.ParseDocumentResponse, error) {
	r.request = req
	return &docreader.ParseDocumentResponse{Success: true, Text: string(req.Content)}, nil
}

func TestDocumentParserUsesValidatedContentInsteadOfProvenanceURL(t *testing.T) {
	r := &captureDocumentReader{}
	svc := &documentProcessorService{grpcClient: r}
	text, _, err := svc.parseDocument(context.Background(), &ProcessDocumentRequest{
		Content: []byte("normalized and checked"), URL: "https://source.example/page", FilePath: "/must/not/reopen",
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != "normalized and checked" || r.request.URL != "" || r.request.FilePath != "" {
		t.Fatalf("unsafe re-read: %+v", r.request)
	}
}

func TestDocumentProcessorRejectsCrossTenantKnowledgeBaseBeforeParsing(t *testing.T) {
	repo := newTenantFakeKBRepo()
	repo.kbs["private"] = &domain.KnowledgeBase{ID: "private", TenantID: 2}
	svc := &documentProcessorService{kbRepo: repo}
	_, err := svc.ProcessDocument(context.Background(), 1, 1, &ProcessDocumentRequest{
		KnowledgeBaseID: "private", Title: "untrusted", Content: []byte("text"),
	})
	if err == nil {
		t.Fatal("cross-tenant ingestion allowed")
	}
}

type processorSettingsRepo struct {
	domain.KnowledgeBaseSettingRepository
}

func (*processorSettingsRepo) FindByKnowledgeBaseID(context.Context, string) (*domain.KnowledgeBaseSetting, error) {
	return &domain.KnowledgeBaseSetting{GraphEnabled: true}, nil
}
func TestDocumentProcessorLoadsKnowledgeBaseSettings(t *testing.T) {
	svc := &documentProcessorService{settingRepo: &processorSettingsRepo{}}
	setting, err := svc.getKnowledgeBaseSetting(context.Background(), "kb")
	if err != nil || setting == nil || !setting.GraphEnabled {
		t.Fatalf("settings ignored: %v %v", setting, err)
	}
}
