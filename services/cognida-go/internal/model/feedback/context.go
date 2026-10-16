package feedback

import (
	l "cognida/internal/model/lifecycle"
	"context"
)

type viewContextKey struct{}
type boundView struct {
	Principal l.Principal
	View      View
}

// WithView is a service-only context value, never an HTTP header or query parameter.
func WithView(ctx context.Context, p l.Principal, v View) context.Context {
	return context.WithValue(ctx, viewContextKey{}, boundView{p, v})
}
func FromContext(ctx context.Context) (l.Principal, View, bool) {
	v, ok := ctx.Value(viewContextKey{}).(boundView)
	return v.Principal, v.View, ok
}

type labelsContextKey struct{}

func WithLabels(ctx context.Context, labels []string) context.Context {
	return context.WithValue(ctx, labelsContextKey{}, append([]string{}, labels...))
}
func RequestedLabels(ctx context.Context) []string {
	labels, _ := ctx.Value(labelsContextKey{}).([]string)
	return labels
}
