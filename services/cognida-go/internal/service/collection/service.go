// Package collection implements the durable web-collection workflow.  It has
// no dependency on Agent tools; the only cross-context port is DocumentIngestor.
package collection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"

	domain "cognida/internal/model/collection"
	"cognida/internal/model/objectstore"
)

// Service is an in-process asynchronous Runner. Durable task/run/item state,
// rather than its goroutines, is the source of truth exposed by the API.
type Service struct {
	repository domain.Repository
	fetcher    domain.Fetcher
	ingestor   domain.DocumentIngestor
	normalize  Normalizer
	artifacts  objectstore.Store
	submitter  interface {
		Submit(context.Context, *domain.Run) error
	}

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

// Normalizer converts an accepted response to deterministic textual content.
type Normalizer interface {
	Normalize(contentType string, body []byte) (string, error)
}

func NewService(repository domain.Repository, fetcher domain.Fetcher, ingestor domain.DocumentIngestor, normalize Normalizer) *Service {
	return &Service{repository: repository, fetcher: fetcher, ingestor: ingestor, normalize: normalize, cancels: make(map[string]context.CancelFunc)}
}

func (s *Service) SetArtifactStore(store objectstore.Store) { s.artifacts = store }

func (s *Service) SetRunSubmitter(submitter interface {
	Submit(context.Context, *domain.Run) error
}) {
	s.submitter = submitter
}

func (s *Service) Create(ctx context.Context, task *domain.Task, sources []*domain.Source) (*domain.Task, error) {
	if existing, err := s.repository.FindTaskByIdempotencyKey(ctx, task.TenantID, task.IdempotencyKey); err == nil {
		return existing, nil
	} else if !errors.Is(err, domain.ErrTaskNotFound) {
		return nil, err
	}
	if err := s.repository.CreateTask(ctx, task, sources); err != nil {
		// Another request may have committed this key after our initial read.
		if existing, lookupErr := s.repository.FindTaskByIdempotencyKey(ctx, task.TenantID, task.IdempotencyKey); lookupErr == nil {
			return existing, nil
		}
		return nil, err
	}
	return task, nil
}

// Start is idempotent while a run is active. It starts an asynchronous worker
// after the run has been persisted, so a restart never loses the audit trail.
func (s *Service) Start(ctx context.Context, tenantID int64, taskID string) error {
	task, err := s.repository.FindTask(ctx, tenantID, taskID)
	if err != nil {
		return err
	}
	if task.Status == domain.TaskStatusRunning {
		active, activeErr := s.repository.FindActiveRun(ctx, tenantID, taskID)
		err = activeErr
		if err == nil {
			return s.dispatchRun(ctx, task, active)
		}
		if !errors.Is(err, domain.ErrRunNotFound) {
			return err
		}
	}
	if err := task.Start(); err != nil {
		return err
	}
	now := time.Now().UTC()
	run := &domain.Run{ID: uuid.NewString(), TaskID: task.ID, TenantID: tenantID, Status: domain.RunStatusRunning, StartedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := s.repository.CreateRun(ctx, run); err != nil {
		if errors.Is(err, domain.ErrActiveRunExists) {
			return nil
		}
		return err
	}
	return s.dispatchRun(ctx, task, run)
}

func (s *Service) dispatchRun(ctx context.Context, task *domain.Task, run *domain.Run) error {
	if s.submitter != nil {
		// Keep the durable run recoverable when Redis is temporarily unavailable.
		return s.submitter.Submit(ctx, run)
	}
	// Compatibility for focused tests and deployments that have not enabled
	// Redis yet. Production composition injects a durable submitter.
	workerCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	if _, exists := s.cancels[run.ID]; exists {
		s.mu.Unlock()
		cancel()
		return nil
	}
	s.cancels[run.ID] = cancel
	s.mu.Unlock()
	go func() {
		if err := s.execute(workerCtx, task, run); err != nil {
			log.Printf("[Collection] execute run %s failed: %v", run.ID, err)
		}
	}()
	return nil
}

// ExecuteRun is the worker entry point. It rehydrates state from durable
// storage instead of relying on the process that accepted Start.
func (s *Service) ExecuteRun(ctx context.Context, payload RunJobPayload) error {
	task, err := s.repository.FindTask(ctx, payload.TenantID, payload.TaskID)
	if err != nil {
		return err
	}
	run, err := s.repository.FindActiveRun(ctx, payload.TenantID, payload.TaskID)
	if errors.Is(err, domain.ErrRunNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if run.ID != payload.RunID {
		return nil // stale redelivery must not execute or fail a newer run
	}
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.mu.Lock()
	s.cancels[run.ID] = cancel
	s.mu.Unlock()
	return s.execute(workerCtx, task, run)
}

func (s *Service) Cancel(ctx context.Context, tenantID int64, taskID string) error {
	task, err := s.repository.FindTask(ctx, tenantID, taskID)
	if err != nil {
		return err
	}
	if err := task.Cancel(); err != nil {
		return err
	}
	if err := s.repository.UpdateTask(ctx, task); err != nil {
		return err
	}
	if run, err := s.repository.FindActiveRun(ctx, tenantID, taskID); err == nil {
		s.mu.Lock()
		cancel := s.cancels[run.ID]
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	} else if !errors.Is(err, domain.ErrRunNotFound) {
		return err
	}
	return nil
}

// execute returns persistence failures to the durable dispatcher, which can
// retry from the already committed per-source outcomes.
func (s *Service) execute(ctx context.Context, task *domain.Task, run *domain.Run) error {
	defer func() { s.mu.Lock(); delete(s.cancels, run.ID); s.mu.Unlock() }()
	sources, err := s.repository.ListSources(ctx, task.TenantID, task.ID)
	if err != nil {
		return err
	}
	previous, err := s.repository.ListItems(ctx, task.TenantID, run.ID)
	if err != nil {
		return err
	}
	bySource := make(map[string]*domain.Item, len(previous))
	for _, item := range previous {
		bySource[item.SourceID] = item
	}
	seen := make(map[string]string, len(sources))
	for _, source := range sources {
		cancelled, err := s.cancelled(ctx, task)
		if err != nil {
			return err
		}
		if cancelled {
			return s.finish(task, run, domain.RunStatusCancelled, "")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		item := bySource[source.ID]
		if item != nil && item.Status != domain.ItemStatusPending {
			if item.Status == domain.ItemStatusAccepted {
				seen[item.ContentHash] = item.KnowledgeID
			}
			continue
		}
		if item == nil {
			now := time.Now().UTC()
			item = &domain.Item{ID: uuid.NewSHA1(uuid.NameSpaceURL, []byte(run.ID+":"+source.ID)).String(), TenantID: task.TenantID, TaskID: task.ID, RunID: run.ID, SourceID: source.ID, SourceURL: source.URL, Status: domain.ItemStatusPending, CreatedAt: now, UpdatedAt: now}
			if err := s.repository.CreateItem(ctx, item); err != nil {
				return err
			}
		}
		response, fetchErr := s.fetchOrRestore(ctx, task, item, source)
		if fetchErr != nil && (item.RawObjectKey != "" || errors.Is(fetchErr, errArtifact)) {
			return fetchErr
		}
		// Cancellation may happen during an outbound request. Never ingest its
		// response after the task has been cancelled, including the final source.
		cancelled, err = s.cancelled(ctx, task)
		if err != nil {
			return err
		}
		if cancelled {
			return s.finish(task, run, domain.RunStatusCancelled, "")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if fetchErr != nil {
			item.Status, item.ErrorReason = domain.ItemStatusFailed, fetchErr.Error()
		} else {
			if response == nil {
				return fmt.Errorf("fetcher returned no response")
			}
			item.CanonicalURL, item.HTTPStatus, item.ContentType = response.CanonicalURL, response.StatusCode, response.ContentType
			if err := s.persistRaw(ctx, item, response); err != nil {
				return err
			}
			content, err := s.normalize.Normalize(response.ContentType, response.Body)
			if err != nil || content == "" {
				item.Status, item.ErrorReason = domain.ItemStatusRejected, "empty or invalid normalized content"
				if err != nil {
					item.ErrorReason = err.Error()
				}
			} else {
				sum := sha256.Sum256([]byte(content))
				item.ContentHash = hex.EncodeToString(sum[:])
				if err := s.persistNormalized(ctx, item, content); err != nil {
					return err
				}
				knowledgeID, duplicate := seen[item.ContentHash]
				if !duplicate {
					accepted, err := s.repository.FindAcceptedContent(ctx, task.TenantID, task.ID, item.ContentHash)
					if err != nil && !errors.Is(err, domain.ErrItemNotFound) {
						return err
					}
					if err == nil {
						knowledgeID, duplicate = accepted.KnowledgeID, true
					}
				}
				if duplicate {
					item.Status, item.KnowledgeID, item.ErrorReason = domain.ItemStatusSkipped, knowledgeID, "duplicate content"
				} else {
					result, err := s.ingestor.Ingest(ctx, domain.IngestRequest{TenantID: task.TenantID, UserID: task.UserID, KnowledgeBaseID: task.KnowledgeBaseID, SourceURL: source.URL, CanonicalURL: response.CanonicalURL, Content: content, ContentHash: item.ContentHash})
					if err != nil {
						item.Status, item.ErrorReason = domain.ItemStatusFailed, err.Error()
					} else if result == nil {
						return fmt.Errorf("ingestor returned no result")
					} else {
						item.Status, item.KnowledgeID = domain.ItemStatusAccepted, result.KnowledgeID
						if result.Duplicate {
							item.Status = domain.ItemStatusSkipped
						}
						seen[item.ContentHash] = result.KnowledgeID
					}
				}
			}
		}
		item.UpdatedAt = time.Now().UTC()
		if err := s.repository.UpdateItem(ctx, item); err != nil {
			return err
		}
	}
	cancelled, err := s.cancelled(ctx, task)
	if err != nil {
		return err
	}
	if cancelled {
		return s.finish(task, run, domain.RunStatusCancelled, "")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.finish(task, run, domain.RunStatusCompleted, "")
}

func (s *Service) cancelled(ctx context.Context, task *domain.Task) (bool, error) {
	// A local cancel interrupts workerCtx; the authoritative task read must
	// survive that cancellation to distinguish it from process shutdown.
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	current, err := s.repository.FindTask(readCtx, task.TenantID, task.ID)
	if err != nil {
		return false, err
	}
	return current.Status == domain.TaskStatusCancelled, nil
}
func (s *Service) finish(task *domain.Task, run *domain.Run, status domain.RunStatus, message string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	now := time.Now().UTC()
	run.Status, run.ErrorMessage, run.CompletedAt, run.UpdatedAt = status, message, &now, now
	return s.repository.FinishRun(ctx, run)
}

// ExhaustRun closes durable running state after the dispatcher exhausts its
// retry policy. A stale delivery cannot close a newer run.
func (s *Service) ExhaustRun(ctx context.Context, payload RunJobPayload, cause error) error {
	run, err := s.repository.FindActiveRun(ctx, payload.TenantID, payload.TaskID)
	if errors.Is(err, domain.ErrRunNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if run.ID != payload.RunID {
		return nil
	}
	task, err := s.repository.FindTask(ctx, payload.TenantID, payload.TaskID)
	if err != nil {
		return err
	}
	return s.finish(task, run, domain.RunStatusFailed, cause.Error())
}

var _ domain.Runner = (*Service)(nil)

// RecoverPendingRuns replays durable runs on every scheduler tick, including
// queue submissions interrupted while Redis was unavailable. Queue IDs fence duplicates.
func (s *Service) RecoverPendingRuns(ctx context.Context, limit int) error {
	runs, err := s.repository.ListRunningRuns(ctx, limit)
	if err != nil {
		return err
	}
	var failures []error
	for _, run := range runs {
		task, err := s.repository.FindTask(ctx, run.TenantID, run.TaskID)
		if err == nil {
			err = s.dispatchRun(ctx, task, run)
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
