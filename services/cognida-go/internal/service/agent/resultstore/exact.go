package resultstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/redis/go-redis/v9"
)

// GetExact preserves stored decimal/integer tokens for deterministic comparison.
// Ordinary tools retain their existing float-based Get representation.
func (s *RedisStore) GetExact(ctx context.Context, owner, id string) (*Result, error) {
	data, err := s.client.Get(ctx, s.key(id)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var result Result
	if err = decoder.Decode(&result); err != nil {
		return nil, err
	}
	if result.Owner != owner {
		return nil, ErrUnauthorized
	}
	return &result, nil
}
