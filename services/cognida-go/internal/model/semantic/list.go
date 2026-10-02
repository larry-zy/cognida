package semantic

import "context"

// ModelLister supports the management console without changing active-model discovery.
type ModelLister interface {
	ListModels(context.Context, int64, string, string, int) ([]*SemanticModel, string, error)
}
