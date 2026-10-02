//go:build integration

package mysql

import (
	storage "cognida/internal/infrastructure/objectstore"
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	job "cognida/internal/infrastructure/job"
	collection "cognida/internal/model/collection"
	collectionsvc "cognida/internal/service/collection"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestCollectionScheduleClaimPersistsRecoverableRun(t *testing.T) {
	db := newIntegrationDB(t)
	repo := NewCollectionRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	id := "it_schedule_" + uuid.NewString()
	now := time.Now().UTC()
	due := now.Add(-time.Minute)
	t.Cleanup(func() { require.NoError(t, db.Where("id=?", id).Delete(&CollectionTaskModel{}).Error) })
	task := &collection.Task{ID: id, TenantID: 7003, UserID: 1, KnowledgeBaseID: "kb", IdempotencyKey: id, Status: collection.TaskStatusDraft, Limits: collection.Limits{MaxSources: 1, MaxResponseBytes: 1024}, Schedule: collection.Schedule{Enabled: true, IntervalSeconds: 60, NextRunAt: &due}, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateTask(ctx, task, []*collection.Source{{ID: id + "_src", TaskID: id, URL: "https://example.com", CreatedAt: now}}))
	var results [2][]*collection.Task
	var errs [2]error
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = repo.claimDueTasks(ctx, now, 100, id) }(i)
	}
	wg.Wait()
	count := 0
	for i := range results {
		require.NoError(t, errs[i])
		for _, claimed := range results[i] {
			if claimed.ID == id {
				count++
				require.Equal(t, collection.TaskStatusRunning, claimed.Status)
			}
		}
	}
	require.Equal(t, 1, count)
	run, err := repo.FindActiveRun(ctx, 7003, id)
	require.NoError(t, err)
	// The scheduler can crash at this point: a durable run already exists.
	pending, err := (collectionsvc.RunRecoverySource{Runs: repo}).PendingJobs(ctx, 1000)
	require.NoError(t, err)
	var recovered *job.Job
	for i := range pending {
		if pending[i].ID == run.ID {
			recovered = &pending[i]
		}
	}
	require.NotNil(t, recovered)
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", Password: os.Getenv("REDIS_PASSWORD")})
	prefix := "it_collection_recovery:" + uuid.NewString()
	queue := job.NewRedisQueueWithPrefix(client, prefix)
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, queue.Purge(cleanup))
		require.NoError(t, client.Close())
	})
	require.NoError(t, client.Ping(ctx).Err())
	scoped := &scopedCollectionProbe{CollectionRepository: repo, id: id, tenant: 7003}
	svc := collectionsvc.NewService(scoped, scheduleFetch{}, scheduleIngest{}, collectionsvc.TextNormalizer{})
	local, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	svc.SetArtifactStore(local)
	submitter := &failOnceCollectionSubmitter{delegate: collectionsvc.JobSubmitter{Queue: queue}}
	svc.SetRunSubmitter(submitter)
	require.Error(t, svc.Start(ctx, 7003, id))
	// No process restart: the next normal scheduler tick recovers the failed submission.
	dispatcher := collectionsvc.NewScheduleDispatcher(scoped, svc, scheduleProbeClock{now}, 10)
	require.NoError(t, dispatcher.Tick(ctx))
	require.Equal(t, 2, submitter.calls)
	require.NoError(t, queue.Enqueue(ctx, *recovered))
	size, err := queue.ReadySize(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, size)
	delivery, err := queue.TryClaim(ctx)
	require.NoError(t, err)
	require.NotNil(t, delivery)
	payload, err := collectionsvc.DecodeRunJob(*delivery)
	require.NoError(t, err)
	require.Equal(t, run.ID, payload.RunID)
	stored, err := repo.FindTask(ctx, 7003, id)
	require.NoError(t, err)
	require.WithinDuration(t, now.Add(time.Minute), *stored.Schedule.NextRunAt, time.Millisecond)
	// Running tasks are not scheduled again; finishing makes the next occurrence eligible.
	claims, err := repo.claimDueTasks(ctx, now.Add(2*time.Minute), 100, id)
	require.NoError(t, err)
	for _, claim := range claims {
		require.NotEqual(t, id, claim.ID)
	}
	require.NoError(t, svc.ExecuteRun(ctx, payload))
	require.NoError(t, queue.Ack(ctx, *delivery))
	completed, err := repo.FindTask(ctx, 7003, id)
	require.NoError(t, err)
	require.Equal(t, collection.TaskStatusCompleted, completed.Status)
	items, err := repo.ListItems(ctx, 7003, run.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, collection.ItemStatusAccepted, items[0].Status)
	require.NotEmpty(t, items[0].RawObjectKey)
	require.NotEmpty(t, items[0].NormalizedObjectKey)
	claims, err = repo.claimDueTasks(ctx, now.Add(2*time.Minute), 100, id)
	require.NoError(t, err)
	found := false
	for _, claim := range claims {
		found = found || claim.ID == id
	}
	require.True(t, found)
}

type scopedCollectionProbe struct {
	*CollectionRepository
	id     string
	tenant int64
}

func (s *scopedCollectionProbe) ClaimDueTasks(ctx context.Context, now time.Time, n int) ([]*collection.Task, error) {
	return s.claimDueTasks(ctx, now, n, s.id)
}
func (s *scopedCollectionProbe) ListRunningRuns(ctx context.Context, n int) ([]*collection.Run, error) {
	run, err := s.FindActiveRun(ctx, s.tenant, s.id)
	if errors.Is(err, collection.ErrRunNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return []*collection.Run{run}, nil
}

type scheduleProbeClock struct{ now time.Time }

func (c scheduleProbeClock) Now() time.Time { return c.now }

type failOnceCollectionSubmitter struct {
	delegate collectionsvc.JobSubmitter
	calls    int
}

func (s *failOnceCollectionSubmitter) Submit(ctx context.Context, run *collection.Run) error {
	s.calls++
	if s.calls == 1 {
		return errors.New("simulated Redis outage")
	}
	return s.delegate.Submit(ctx, run)
}

type scheduleFetch struct{}

func (scheduleFetch) Fetch(context.Context, collection.FetchRequest) (*collection.FetchResponse, error) {
	return &collection.FetchResponse{CanonicalURL: "https://example.com/", ContentType: "text/plain", StatusCode: 200, Body: []byte("retained collection content")}, nil
}

type scheduleIngest struct{}

func (scheduleIngest) Ingest(context.Context, collection.IngestRequest) (*collection.IngestResult, error) {
	return &collection.IngestResult{KnowledgeID: "integration-doc"}, nil
}
