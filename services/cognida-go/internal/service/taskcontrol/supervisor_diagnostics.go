package taskcontrol

import (
	"errors"
	"unicode/utf8"

	"cognida/internal/model/llm"
)

// PlannerDiagnosticDTO contains only program-defined codes and numeric facts.
// It never retains model text, reasoning, URLs, prompts, tool arguments or keys.
type PlannerDiagnosticDTO struct {
	Code                string                `json:"code"`
	FinishReason        string                `json:"finish_reason,omitempty"`
	MaxOutputTokens     int                   `json:"max_output_tokens"`
	OutputCharacters    int                   `json:"output_characters"`
	ReasoningCharacters int                   `json:"reasoning_characters"`
	Usage               *PlannerTokenUsageDTO `json:"usage,omitempty"`
}

type PlannerTokenUsageDTO struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type plannerOutputError struct{ diagnostic PlannerDiagnosticDTO }

func (e *plannerOutputError) Error() string {
	return ErrPlannerOutput.Error() + ": " + e.diagnostic.Code
}
func (e *plannerOutputError) Unwrap() error { return ErrPlannerOutput }

func PlannerDiagnostic(err error) *PlannerDiagnosticDTO {
	var failure *plannerOutputError
	if !errors.As(err, &failure) {
		return nil
	}
	copy := failure.diagnostic
	if copy.Usage != nil {
		usage := *copy.Usage
		copy.Usage = &usage
	}
	return &copy
}

func plannerOutputFailure(code string, response *llm.ChatResponse) error {
	diagnostic := PlannerDiagnosticDTO{Code: code, MaxOutputTokens: plannerMaxTokens}
	if response != nil {
		content := response.Content
		if content == "" && response.Message != nil {
			content = response.Message.Content
		}
		diagnostic.OutputCharacters = utf8.RuneCountInString(content)
		diagnostic.ReasoningCharacters = utf8.RuneCountInString(response.Thinking)
		switch response.FinishReason {
		case "", "stop", "end_turn", "length", "content_filter", "tool_calls":
			diagnostic.FinishReason = response.FinishReason
		default:
			diagnostic.FinishReason = "unknown"
		}
		if usage := response.Usage; usage != nil && usage.PromptTokens >= 0 && usage.CompletionTokens >= 0 && usage.TotalTokens >= 0 {
			diagnostic.Usage = &PlannerTokenUsageDTO{PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens, TotalTokens: usage.TotalTokens}
		}
	}
	return &plannerOutputError{diagnostic: diagnostic}
}
