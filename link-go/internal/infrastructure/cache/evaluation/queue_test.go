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

func setupTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	s, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})

	return s, client
}

func TestQueue_EnqueueDequeue(t *testing.T) {
	s, client := setupTestRedis(t)
	defer s.Close()

	ctx := context.Background()
	q := NewQueue(client, 3)

	// Enqueue
	err := q.Enqueue(ctx, "task-1")
	require.NoError(t, err)

	err = q.Enqueue(ctx, "task-2")
	require.NoError(t, err)

	// Check size
	size, err := q.GetSize(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), size)

	// Dequeue (FIFO behavior: LPush + BRPop = first in, first out)
	// First enqueued was task-1, so it should be dequeued first
	taskID, err := q.Dequeue(ctx)
	require.NoError(t, err)
	assert.Equal(t, "task-1", taskID)

	// Dequeue again - get task-2
	taskID, err = q.Dequeue(ctx)
	require.NoError(t, err)
	assert.Equal(t, "task-2", taskID)

	// Queue empty - timeout
	done := make(chan string)
	go func() {
		id, _ := q.Dequeue(ctx)
		done <- id
	}()

	select {
	case id := <-done:
		assert.Empty(t, id)
	case <-time.After(35 * time.Second):
		t.Fatal("expected timeout within 35 seconds")
	}
}

func TestQueue_AcquireReleaseSlot(t *testing.T) {
	s, client := setupTestRedis(t)
	defer s.Close()

	ctx := context.Background()
	q := NewQueue(client, 2) // limit = 2

	// Acquire first slot
	acquired, err := q.AcquireSlot(ctx)
	require.NoError(t, err)
	assert.True(t, acquired)

	count, err := q.GetCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Acquire second slot
	acquired, err = q.AcquireSlot(ctx)
	require.NoError(t, err)
	assert.True(t, acquired)

	count, err = q.GetCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	// Third slot should fail
	acquired, err = q.AcquireSlot(ctx)
	require.NoError(t, err)
	assert.False(t, acquired)

	count, err = q.GetCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	// Release one slot
	err = q.ReleaseSlot(ctx)
	require.NoError(t, err)

	count, err = q.GetCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Now should be able to acquire again
	acquired, err = q.AcquireSlot(ctx)
	require.NoError(t, err)
	assert.True(t, acquired)
}

func TestQueue_Clear(t *testing.T) {
	s, client := setupTestRedis(t)
	defer s.Close()

	ctx := context.Background()
	q := NewQueue(client, 3)

	// Enqueue some tasks
	err := q.Enqueue(ctx, "task-1")
	require.NoError(t, err)
	err = q.Enqueue(ctx, "task-2")
	require.NoError(t, err)

	// Clear
	err = q.Clear(ctx)
	require.NoError(t, err)

	// Check size
	size, err := q.GetSize(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), size)
}
