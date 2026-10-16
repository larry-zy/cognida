package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"

	"cognida/internal/infrastructure/llm/chat"
	domainllm "cognida/internal/model/llm"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestResearchFailureClassifiesTypedCausesWithoutProviderDetails(t *testing.T) {
	const secret = "private-provider-body-and-key"
	for _, tc := range []struct {
		name, code string
		err        error
		status     int
	}{
		{"http", "http_error", &chat.HTTPStatusError{StatusCode: 429, Body: secret}, 429},
		{"domain http", "http_error", &domainllm.APIError{StatusCode: 503, Detail: secret}, 503},
		{"dns timeout", "network_timeout", &net.DNSError{Err: secret, Name: secret, IsTimeout: true}, 0},
		{"dns", "network_error", &net.DNSError{Err: secret, Name: secret}, 0},
		{"transport", "transport_error", &url.Error{Op: "POST", URL: "https://" + secret, Err: errors.New(secret)}, 0},
		{"eof", "unexpected_eof", io.ErrUnexpectedEOF, 0},
		{"cancel", "cancelled", context.Canceled, 0},
		{"deadline", "deadline_exceeded", context.DeadlineExceeded, 0},
		{"no evidence", "no_evidence", ErrResearchNoEvidence, 0},
		{"unknown", "unknown", errors.New("HTTP 503 network timeout " + secret), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failure := researchFailure("synthesizing", fmt.Errorf("private wrapper %s: %w", secret, tc.err))
			require.Equal(t, tc.code, failure.Code)
			require.Equal(t, "synthesizing", failure.Stage)
			require.Equal(t, tc.status, failure.HTTPStatus)
			raw, err := json.Marshal(failure)
			require.NoError(t, err)
			require.NotContains(t, string(raw), secret)
		})
	}
}

func TestResearchFailedSynthesisRetainsEvidenceAndSafeDiagnosticWithoutRetry(t *testing.T) {
	const secret = "secret-model-response-do-not-persist"
	for _, tc := range []struct {
		name, code string
		response   *schema.Message
		err        error
	}{
		{"nil", "empty_response", nil, nil},
		{"empty", "empty_response", schema.AssistantMessage("  ", nil), nil},
		{"invalid json", "invalid_json", schema.AssistantMessage("not JSON: "+secret, nil), nil},
		{"truncated", "truncated_output", &schema.Message{Content: `{"summary":"` + secret, ResponseMeta: &schema.ResponseMeta{FinishReason: "length"}}, nil},
		{"valid json truncated", "truncated_output", &schema.Message{Content: researchGoodDraft(), ReasoningContent: secret, ResponseMeta: &schema.ResponseMeta{FinishReason: "length", Usage: &schema.TokenUsage{PromptTokens: 1500, CompletionTokens: 8192, TotalTokens: 9692}}}, nil},
		{"tools", "unexpected_tool_calls", schema.AssistantMessage(secret, []schema.ToolCall{{ID: secret}}), nil},
		{"large", "output_limit", schema.AssistantMessage(strings.Repeat("x", 100001)+secret, nil), nil},
		{"http", "http_error", nil, &chat.HTTPStatusError{StatusCode: 502, Body: secret}},
		{"unknown", "unknown", nil, errors.New(secret)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newResearchTestService(t, false, researchGoodDraft())
			calls := 0
			svc.model = researchTestModel{run: func(context.Context, []*schema.Message) (*schema.Message, error) {
				calls++
				if calls == 1 {
					return schema.AssistantMessage(`{"questions":[{"question":"specific question","strategy":"hybrid"}]}`, nil), nil
				}
				return tc.response, tc.err
			}}
			report, err := svc.ExecuteGoverned(researchTestContext(), &DeepResearchRequest{Query: "question", KBIDs: []string{"allowed"}})
			require.Error(t, err)
			require.Equal(t, 2, calls, "diagnostics must not retry paid work")
			require.Equal(t, 2, report.Metadata.ModelCallsUsed)
			require.False(t, report.Success)
			require.False(t, report.Metadata.CitationChecked)
			require.False(t, report.Metadata.SemanticReview)
			require.Len(t, report.Sources, 1)
			expected := &ResearchFailureDTO{Stage: "synthesizing", Code: tc.code}
			if tc.name == "http" {
				expected.HTTPStatus = 502
			}
			if tc.err == nil {
				expected.Output = &ResearchOutputDiagnosticDTO{MaxOutputTokens: 8192}
				if tc.response != nil {
					expected.Output.OutputCharacters = utf8.RuneCountInString(tc.response.Content)
					expected.Output.ReasoningCharacters = utf8.RuneCountInString(tc.response.ReasoningContent)
					if meta := tc.response.ResponseMeta; meta != nil {
						expected.Output.FinishReason = meta.FinishReason
						if meta.Usage != nil {
							expected.Output.Usage = &ResearchTokenUsageDTO{PromptTokens: 1500, CompletionTokens: 8192, TotalTokens: 9692}
						}
					}
				}
			}
			require.Equal(t, expected, report.Metadata.Failure)
			require.Contains(t, report.Metadata.VerificationLimit, "尚未完成引用")
			raw, err := json.Marshal(report)
			require.NoError(t, err)
			require.NotContains(t, string(raw), secret)
		})
	}
}

func TestResearchDiagnosticUsesActualPlanningAndReviewStage(t *testing.T) {
	for _, stage := range []string{"decomposing", "reviewing"} {
		t.Run(stage, func(t *testing.T) {
			svc := newResearchTestService(t, false, researchGoodDraft())
			calls := 0
			svc.model = researchTestModel{run: func(context.Context, []*schema.Message) (*schema.Message, error) {
				calls++
				if stage == "decomposing" || calls == 3 {
					return nil, &chat.HTTPStatusError{StatusCode: 503, Body: "untrusted upstream details"}
				}
				if calls == 1 {
					return schema.AssistantMessage(`{"questions":[{"question":"specific question","strategy":"hybrid"}]}`, nil), nil
				}
				return schema.AssistantMessage(researchGoodDraft(), nil), nil
			}}
			report, err := svc.ExecuteGoverned(researchTestContext(), &DeepResearchRequest{Query: "question", KBIDs: []string{"allowed"}})
			require.Error(t, err)
			require.Equal(t, &ResearchFailureDTO{Stage: stage, Code: "http_error", HTTPStatus: 503}, report.Metadata.Failure)
			require.False(t, report.Success)
			require.False(t, report.Metadata.SemanticReview)
			if stage == "reviewing" {
				require.Equal(t, 3, calls)
				require.True(t, report.Metadata.CitationChecked)
				require.Len(t, report.Claims, 1)
				require.Contains(t, report.Metadata.VerificationLimit, "语义审查尚未完成")
			} else {
				require.Equal(t, 1, calls)
				require.Contains(t, report.Metadata.VerificationLimit, "尚未完成引用")
			}
		})
	}
}

func TestResearchOutputDiagnosticSanitizesUnrecognizedProviderMetadata(t *testing.T) {
	const secret = "provider-body-or-secret"
	err := newResearchOutputError("invalid_json", errors.New(secret), &schema.Message{Content: secret, ReasoningContent: secret, ResponseMeta: &schema.ResponseMeta{FinishReason: secret, Usage: &schema.TokenUsage{PromptTokens: -1}}}, 8192)
	failure := researchFailure("synthesizing", err)
	require.Equal(t, "unknown", failure.Output.FinishReason)
	require.Nil(t, failure.Output.Usage)
	raw, marshalErr := json.Marshal(failure)
	require.NoError(t, marshalErr)
	require.NotContains(t, string(raw), secret)
	require.NotContains(t, err.Error(), secret)
}
