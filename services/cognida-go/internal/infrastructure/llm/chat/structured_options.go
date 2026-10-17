package chat

import (
	"cognida/internal/model/llm"
	"strings"

	"github.com/cloudwego/eino/components/model"
)

// WithBoundedJSONOutput requests a concise structured response without an
// additional reasoning stream on explicitly supported providers. Other adapters
// ignore this option. It does not disable the caller's separate evidence review.
func WithBoundedJSONOutput() model.Option {
	return llm.WithBoundedJSONOutput()
}

func supportsBoundedJSON(provider, modelName string) bool {
	if !strings.EqualFold(strings.TrimSpace(provider), "deepseek") {
		return false
	}
	// Only documented models are enabled; model-name substrings and unknown
	// OpenAI-compatible providers do not establish extension support.
	switch strings.ToLower(strings.TrimSpace(modelName)) {
	case "deepseek-v4-flash", "deepseek-v4-pro":
		return true
	default:
		return false
	}
}
