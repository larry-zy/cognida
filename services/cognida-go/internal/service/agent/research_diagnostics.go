package agent

import (
	"context"
	"errors"
	"io"
	"net"
	"net/url"
	"unicode/utf8"

	"cognida/internal/infrastructure/llm/chat"
	domainllm "cognida/internal/model/llm"
	"github.com/cloudwego/eino/schema"
)

// ResearchFailureDTO contains only bounded program-defined diagnostics. Provider
// messages, URLs, prompts and response bodies must never be persisted here.
type ResearchFailureDTO struct {
	Stage      string                       `json:"stage"`
	Code       string                       `json:"code"`
	HTTPStatus int                          `json:"http_status,omitempty"`
	Output     *ResearchOutputDiagnosticDTO `json:"output,omitempty"`
	Structure  *ResearchDraftStructureDTO   `json:"structure,omitempty"`
}

// ResearchOutputDiagnosticDTO stores provider-reported counters, never content.
// Token usage is absent when the provider did not report valid usage; character
// counts are measured locally and must not be confused with token counts.
type ResearchOutputDiagnosticDTO struct {
	MaxOutputTokens     int                    `json:"max_output_tokens"`
	FinishReason        string                 `json:"finish_reason,omitempty"`
	OutputCharacters    int                    `json:"output_characters"`
	ReasoningCharacters int                    `json:"reasoning_characters"`
	Usage               *ResearchTokenUsageDTO `json:"usage,omitempty"`
}

type ResearchTokenUsageDTO struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type researchOutputError struct {
	code   string
	cause  error
	output *ResearchOutputDiagnosticDTO
}

// Codes are assigned at validation sites, never inferred from provider text.
type researchValidationError struct {
	code      string
	structure *ResearchDraftStructureDTO
}

func (e *researchValidationError) Error() string { return "research validation: " + e.code }

func (e *researchOutputError) Error() string { return "research model output: " + e.code }
func (e *researchOutputError) Unwrap() error { return e.cause }

func newResearchOutputError(code string, cause error, response *schema.Message, maxTokens int) error {
	detail := &ResearchOutputDiagnosticDTO{MaxOutputTokens: maxTokens}
	if response != nil {
		detail.OutputCharacters = utf8.RuneCountInString(response.Content)
		detail.ReasoningCharacters = utf8.RuneCountInString(response.ReasoningContent)
		if meta := response.ResponseMeta; meta != nil {
			switch meta.FinishReason {
			case "", "stop", "length", "tool_calls", "content_filter":
				detail.FinishReason = meta.FinishReason
			default:
				detail.FinishReason = "unknown"
			}
			if usage := meta.Usage; usage != nil && usage.PromptTokens >= 0 && usage.CompletionTokens >= 0 && usage.TotalTokens >= 0 {
				detail.Usage = &ResearchTokenUsageDTO{PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens, TotalTokens: usage.TotalTokens}
			}
		}
	}
	return &researchOutputError{code: code, cause: cause, output: detail}
}

func researchVerificationLimit(metadata ReportMetadataDTO) string {
	if !metadata.CitationChecked {
		return "尚未完成引用 ID 与原文摘录核对；仅保留已收集来源，不能据此认定主张获得支持。"
	}
	if !metadata.SemanticReview {
		return "已核对引用 ID 与原文摘录；语义审查尚未完成，未独立验证来源真实性。"
	}
	if metadata.SupplementalReviewIncomplete {
		return "补充审查未完成；历史快照仅覆盖已列明的首轮来源。新增来源尚未完成语义核对，当前支持结论已降为待核实，已知冲突和原始证据保留。模型审查不证明来源真实性。"
	}
	return "引文格式和摘录由程序核对；语义状态是受限模型审查意见，不证明来源真实性。已知同文转载归组，未知转载仍可能存在。"
}

func researchFailure(stage string, err error) *ResearchFailureDTO {
	failure := &ResearchFailureDTO{Stage: stage, Code: "unknown"}
	var output *researchOutputError
	var validation *researchValidationError
	var httpStatus *chat.HTTPStatusError
	var apiStatus *domainllm.APIError
	var network net.Error
	var transport *url.Error
	switch {
	case errors.Is(err, context.Canceled):
		failure.Code = "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		failure.Code = "deadline_exceeded"
	case errors.As(err, &output):
		failure.Code = output.code
		failure.Output = output.output
	case errors.As(err, &validation):
		failure.Code = validation.code
		failure.Structure = validation.structure
	case errors.As(err, &httpStatus) && httpStatus.StatusCode >= 100 && httpStatus.StatusCode <= 599:
		failure.Code, failure.HTTPStatus = "http_error", httpStatus.StatusCode
	case errors.As(err, &apiStatus) && apiStatus.StatusCode >= 100 && apiStatus.StatusCode <= 599:
		failure.Code, failure.HTTPStatus = "http_error", apiStatus.StatusCode
	case errors.As(err, &network) && network.Timeout():
		failure.Code = "network_timeout"
	case errors.As(err, &transport):
		failure.Code = "transport_error"
	case errors.As(err, &network):
		failure.Code = "network_error"
	case errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF):
		failure.Code = "unexpected_eof"
	case errors.Is(err, ErrResearchNoEvidence):
		failure.Code = "no_evidence"
	}
	return failure
}
