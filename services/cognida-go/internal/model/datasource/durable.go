package datasource

import "context"

type DurableQueryExecutor func(context.Context, int64, int64, string, QueryRequest, string) (*QueryResult, error)
type DurableQueryProvider interface {
	ExecuteDurableQuery(context.Context, int64, int64, string, QueryRequest, string) (*QueryResult, error)
}
type queryIdentityKey struct{}
type queryIdentity struct {
	UserID int64
	Key    string
}

func WithQueryIdentity(ctx context.Context, userID int64, key string) context.Context {
	return context.WithValue(ctx, queryIdentityKey{}, queryIdentity{userID, key})
}
func QueryIdentity(ctx context.Context) (int64, string) {
	v, _ := ctx.Value(queryIdentityKey{}).(queryIdentity)
	return v.UserID, v.Key
}
