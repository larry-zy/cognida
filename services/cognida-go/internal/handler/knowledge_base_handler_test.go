package handler

import (
	"context"
	"errors"
	"testing"

	domain_knowledge "cognida/internal/model/knowledge"
)

type recordingParseStatusUpdater struct {
	called       bool
	knowledgeID  string
	parseStatus  string
	errorMessage string
}

func (u *recordingParseStatusUpdater) UpdateParseStatus(
	ctx context.Context,
	id, parseStatus, errorMessage string,
) error {
	u.called = true
	u.knowledgeID = id
	u.parseStatus = parseStatus
	u.errorMessage = errorMessage
	return ctx.Err()
}

func TestValidateKnowledgeFileSize(t *testing.T) {
	if err := validateKnowledgeFileSize(maxKnowledgeFileSize); err != nil {
		t.Fatalf("expected 10 MiB file to be accepted, got %v", err)
	}

	if err := validateKnowledgeFileSize(maxKnowledgeFileSize + 1); err == nil {
		t.Fatal("expected file larger than 10 MiB to be rejected")
	}
}

func TestUpdateDocumentFailureStatusPreservesCause(t *testing.T) {
	updater := &recordingParseStatusUpdater{}
	processingErr := errors.New("rpc error: code = Unavailable")

	if err := updateDocumentFailureStatus(context.Background(), updater, "knowledge-1", processingErr); err != nil {
		t.Fatalf("updateDocumentFailureStatus returned error: %v", err)
	}
	if !updater.called {
		t.Fatal("expected parse status update")
	}
	if updater.parseStatus != domain_knowledge.ParseStatusFailed {
		t.Fatalf("parse status = %q, want %q", updater.parseStatus, domain_knowledge.ParseStatusFailed)
	}
	if updater.errorMessage != processingErr.Error() {
		t.Fatalf("error message = %q, want %q", updater.errorMessage, processingErr)
	}
}
