package resultstore

import (
	"context"
	"time"
)

type guardedStore struct {
	store Store
	check func(context.Context, *Result) error
}

func Guard(store Store, check func(context.Context, *Result) error) Store {
	return &guardedStore{store, check}
}
func (s *guardedStore) Put(ctx context.Context, r *Result, ttl time.Duration) (string, error) {
	if err := s.check(ctx, r); err != nil {
		return "", err
	}
	return s.store.Put(ctx, r, ttl)
}
func (s *guardedStore) Get(ctx context.Context, owner, id string) (*Result, error) {
	return s.get(ctx, owner, id, false)
}
func (s *guardedStore) GetExact(ctx context.Context, owner, id string) (*Result, error) {
	return s.get(ctx, owner, id, true)
}
func (s *guardedStore) get(ctx context.Context, owner, id string, exact bool) (*Result, error) {
	var r *Result
	var err error
	if v, ok := s.store.(interface {
		GetExact(context.Context, string, string) (*Result, error)
	}); exact && ok {
		r, err = v.GetExact(ctx, owner, id)
	} else {
		r, err = s.store.Get(ctx, owner, id)
	}
	if err != nil {
		return nil, err
	}
	if err = s.check(ctx, r); err != nil {
		return nil, err
	}
	return r, nil
}
