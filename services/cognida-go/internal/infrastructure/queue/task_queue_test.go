package queue

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"cognida/internal/model/task"
)

func testTaskQueue(t *testing.T) (*miniredis.Miniredis, *RedisTaskQueue) {
	t.Helper()
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return s, NewRedisTaskQueue(client).(*RedisTaskQueue)
}

func testTask(typ string) *task.Task {
	return &task.Task{
		ID:       "task-1",
		TenantID: 1,
		Type:     typ,
		Status:   task.TaskStatusPending,
		Payload:  map[string]interface{}{"key": "value"},
	}
}

func TestRedisTaskQueueDeliversLegacyTaskThroughGenericJob(t *testing.T) {
	_, queue := testTaskQueue(t)
	ctx := context.Background()
	want := testTask(task.TaskTypeEvaluation)
	require.NoError(t, queue.Enqueue(ctx, want))
	size, err := queue.GetQueueSize(ctx, task.TaskTypeEvaluation)
	require.NoError(t, err)
	require.EqualValues(t, 1, size)
	got, err := queue.Dequeue(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, want.ID, got.ID)
	require.Equal(t, want.Payload, got.Payload)
	size, err = queue.GetQueueSize(ctx, task.TaskTypeEvaluation)
	require.NoError(t, err)
	require.Zero(t, size)
}

func TestRedisTaskQueuePreservesPriorityAndDuplicateDelivery(t *testing.T) {
	_, queue := testTaskQueue(t)
	ctx := context.Background()
	low := testTask(task.TaskTypeDocumentParse)
	high := testTask(task.TaskTypeEvaluation)
	high.ID = "high"
	require.NoError(t, queue.Enqueue(ctx, low))
	require.NoError(t, queue.Enqueue(ctx, high))
	got, err := queue.Dequeue(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, task.TaskTypeEvaluation, got.Type)

	duplicate := testTask(task.TaskTypeEvaluation)
	require.NoError(t, queue.Enqueue(ctx, duplicate))
	require.NoError(t, queue.Enqueue(ctx, duplicate))
	size, err := queue.GetQueueSize(ctx, task.TaskTypeEvaluation)
	require.NoError(t, err)
	require.EqualValues(t, 2, size)
}

func TestRedisTaskQueuePromotesDelayedJobs(t *testing.T) {
	_, queue := testTaskQueue(t)
	ctx := context.Background()
	want := testTask(task.TaskTypeKBIndex)
	require.NoError(t, queue.EnqueueDelay(ctx, want, 0))
	tasks, err := queue.GetDelayedTasks(ctx, 10)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, want.ID, tasks[0].ID)
}

func TestRedisTaskQueueClearAndValidation(t *testing.T) {
	_, queue := testTaskQueue(t)
	ctx := context.Background()
	require.Error(t, queue.Enqueue(ctx, nil))
	require.NoError(t, queue.Enqueue(ctx, testTask(task.TaskTypeEvaluation)))
	require.NoError(t, queue.ClearQueue(ctx, task.TaskTypeEvaluation))
	size, err := queue.GetQueueSize(ctx, task.TaskTypeEvaluation)
	require.NoError(t, err)
	require.Zero(t, size)
	require.NoError(t, queue.Complete(ctx, "task-1"))
	require.NoError(t, queue.Fail(ctx, "task-1", "failure"))
}
