package collection

import (
	jobinfra "cognida/internal/infrastructure/job"
	domain "cognida/internal/model/collection"
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCollectionRunRecoveryAdaptsOnlyRunningRuns(t *testing.T) {
	source := RunRecoverySource{Runs: runReader{runs: []*domain.Run{{ID: "r1", TaskID: "t1", TenantID: 1, Status: domain.RunStatusRunning}, {ID: "r2", Status: domain.RunStatusCompleted}}}}
	jobs, err := source.PendingJobs(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	payload, err := DecodeRunJob(jobs[0])
	require.NoError(t, err)
	require.Equal(t, "r1", payload.RunID)
}

type runReader struct{ runs []*domain.Run }

func (r runReader) ListRunningRuns(context.Context, int) ([]*domain.Run, error) { return r.runs, nil }

var _ jobinfra.RecoverySource = RunRecoverySource{}
