package llm

import (
	"fmt"
	"github.com/cloudwego/eino/components/model"
)

// HTTPStatusError preserves safe classification data across provider adapters.
type HTTPStatusError struct {
	StatusCode int
	RetryAfter string
	Body       string
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("api error (status %d): %s", e.StatusCode, e.Body)
}

// StructuredOptions describes a provider-neutral request. Unsupported providers ignore it.
type StructuredOptions struct{ BoundedJSON bool }

func WithBoundedJSONOutput() model.Option {
	return model.WrapImplSpecificOptFn(func(o *StructuredOptions) { o.BoundedJSON = true })
}
