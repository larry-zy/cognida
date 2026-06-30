package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRedisForProgress(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	s, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})

	return s, client
}

func TestProgress_SetAndGet(t *testing.T) {
	s, client := setupTestRedisForProgress(t)
	defer s.Close()

	ctx := context.Background()
	cache := NewProgressCache(client)

	progress := &Progress{
		Stage:      StageGeneration,
		Current:    5,
		Total:      10,
		Message:    "Generating answers...",
		RetryCount: 0,
	}

	err := cache.SetProgress(ctx, "task-1", progress)
	require.NoError(t, err)

	// Get progress
	retrieved, err := cache.GetProgress(ctx, "task-1")
	require.NoError(t, err)

	assert.Equal(t, StageGeneration, retrieved.Stage)
	assert.Equal(t, 5, retrieved.Current)
	assert.Equal(t, 10, retrieved.Total)
	assert.Equal(t, "Generating answers...", retrieved.Message)
	assert.Equal(t, 50, retrieved.Percentage) // 5/10 = 50%
}

func TestProgress_UpdateStage(t *testing.T) {
	s, client := setupTestRedisForProgress(t)
	defer s.Close()

	ctx := context.Background()
	cache := NewProgressCache(client)

	// Set initial progress
	progress := &Progress{
		Stage:   StageQueued,
		Current: 0,
		Total:   100,
		Message: "Queued",
	}
	err := cache.SetProgress(ctx, "task-1", progress)
	require.NoError(t, err)

	// Update stage
	err = cache.UpdateStage(ctx, "task-1", StageLoading, "Loading dataset...")
	require.NoError(t, err)

	// Get and verify
	retrieved, err := cache.GetProgress(ctx, "task-1")
	require.NoError(t, err)

	assert.Equal(t, StageLoading, retrieved.Stage)
	assert.Equal(t, "Loading dataset...", retrieved.Message)
}

func TestProgress_UpdateProgress(t *testing.T) {
	s, client := setupTestRedisForProgress(t)
	defer s.Close()

	ctx := context.Background()
	cache := NewProgressCache(client)

	// Set initial progress
	progress := &Progress{
		Stage:   StageGeneration,
		Current: 0,
		Total:   100,
		Message: "Starting...",
	}
	err := cache.SetProgress(ctx, "task-1", progress)
	require.NoError(t, err)

	// Update progress
	err = cache.UpdateProgress(ctx, "task-1", 25, 100, "Processing...")
	require.NoError(t, err)

	// Get and verify
	retrieved, err := cache.GetProgress(ctx, "task-1")
	require.NoError(t, err)

	assert.Equal(t, 25, retrieved.Current)
	assert.Equal(t, 100, retrieved.Total)
	assert.Equal(t, "Processing...", retrieved.Message)
	assert.Equal(t, 25, retrieved.Percentage)
}

func TestProgress_Increment(t *testing.T) {
	s, client := setupTestRedisForProgress(t)
	defer s.Close()

	ctx := context.Background()
	cache := NewProgressCache(client)

	// Set initial progress
	progress := &Progress{
		Stage:   StageGeneration,
		Current: 5,
		Total:   10,
		Message: "Processing 5/10",
	}
	err := cache.SetProgress(ctx, "task-1", progress)
	require.NoError(t, err)

	// Increment
	err = cache.Increment(ctx, "task-1", "Processing 6/10")
	require.NoError(t, err)

	// Get and verify
	retrieved, err := cache.GetProgress(ctx, "task-1")
	require.NoError(t, err)

	assert.Equal(t, 6, retrieved.Current)
	assert.Equal(t, "Processing 6/10", retrieved.Message)
	assert.Equal(t, 60, retrieved.Percentage)
}

func TestProgress_SetError(t *testing.T) {
	s, client := setupTestRedisForProgress(t)
	defer s.Close()

	ctx := context.Background()
	cache := NewProgressCache(client)

	// Set initial progress
	progress := &Progress{
		Stage:   StageGeneration,
		Current: 5,
		Total:   10,
		Message: "Processing",
	}
	err := cache.SetProgress(ctx, "task-1", progress)
	require.NoError(t, err)

	// Set error
	err = cache.SetError(ctx, "task-1", "Python service unavailable", 2)
	require.NoError(t, err)

	// Get and verify
	retrieved, err := cache.GetProgress(ctx, "task-1")
	require.NoError(t, err)

	assert.Equal(t, StageFailed, retrieved.Stage)
	assert.Equal(t, "Python service unavailable", retrieved.Error)
	assert.Equal(t, 2, retrieved.RetryCount)
}

func TestProgress_Delete(t *testing.T) {
	s, client := setupTestRedisForProgress(t)
	defer s.Close()

	ctx := context.Background()
	cache := NewProgressCache(client)

	// Set progress
	progress := &Progress{
		Stage:   StageCompleted,
		Current: 10,
		Total:   10,
		Message: "Completed",
	}
	err := cache.SetProgress(ctx, "task-1", progress)
	require.NoError(t, err)

	// Delete
	err = cache.Delete(ctx, "task-1")
	require.NoError(t, err)

	// Get should return nil
	retrieved, err := cache.GetProgress(ctx, "task-1")
	require.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestProgress_Expiry(t *testing.T) {
	s, client := setupTestRedisForProgress(t)
	defer s.Close()

	ctx := context.Background()
	cache := NewProgressCache(client)

	// Set progress
	progress := &Progress{
		Stage:   StageCompleted,
		Current: 10,
		Total:   10,
		Message: "Completed",
	}
	err := cache.SetProgress(ctx, "task-1", progress)
	require.NoError(t, err)

	// Fast forward time
	s.FastForward(30 * time.Minute)

	// Should still exist
	retrieved, err := cache.GetProgress(ctx, "task-1")
	require.NoError(t, err)
	assert.NotNil(t, retrieved)

	// Fast forward past expiry
	s.FastForward(31 * time.Minute)

	// Should be expired
	retrieved, err = cache.GetProgress(ctx, "task-1")
	require.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestProgress_GetNotFound(t *testing.T) {
	s, client := setupTestRedisForProgress(t)
	defer s.Close()

	ctx := context.Background()
	cache := NewProgressCache(client)

	// Get non-existent progress
	retrieved, err := cache.GetProgress(ctx, "non-existent")
	require.NoError(t, err)
	assert.Nil(t, retrieved)
}
