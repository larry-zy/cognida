package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	agentctx "cognida/internal/model/agent"
	d "cognida/internal/model/evaluation"
	job "cognida/internal/model/job"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/service/lifecycle"
)

const OfflineJobType = "evaluation.offline"
const offlineLease = 30 * time.Second

var errOfflineScoring = errors.New("offline scoring service unavailable or returned invalid scores")

// OfflineScorer only scores answers already present in a bounded immutable snapshot.
// It must not generate answers or invoke a model. Production uses NewOfflineScorer.
type OfflineScorer interface {
	Score(context.Context, []d.OfflineItem) ([]d.OfflineScore, error)
}
type pythonOfflineScorer struct{ client *PythonEvaluationClient }

func NewOfflineScorer(client *PythonEvaluationClient) OfflineScorer {
	return &pythonOfflineScorer{client}
}
func (s *pythonOfflineScorer) Score(ctx context.Context, items []d.OfflineItem) ([]d.OfflineScore, error) {
	if s.client == nil || len(items) < 1 || len(items) > d.OfflineBatchSize {
		return nil, errOfflineScoring
	}
	req := &ComputeMetricsRequest{Graders: []string{"exact_match", "contains_match"}}
	for _, item := range items {
		if item.Validate() != nil {
			return nil, d.ErrOfflineInvalid
		}
		req.Items = append(req.Items, &ComputeItem{Question: item.Question, ReferenceAnswer: item.ReferenceAnswer, GeneratedAnswer: item.GeneratedAnswer})
	}
	result, e := s.client.ComputeMetricsWithRetry(ctx, req, 1, 0)
	if e != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errOfflineScoring
	}
	if result == nil || len(result.Items) != len(items) || len(result.Unsupported) != 0 {
		return nil, errOfflineScoring
	}
	out := make([]d.OfflineScore, len(items))
	seen := map[int]bool{}
	for _, item := range result.Items {
		if item == nil || item.Index < 0 || item.Index >= len(items) || seen[item.Index] || len(item.Scores) != 2 {
			return nil, errOfflineScoring
		}
		exact, ok := item.Scores["exact_match"]
		contains, ok2 := item.Scores["contains_match"]
		score := d.OfflineScore{Index: item.Index, ExactMatch: exact, ContainsMatch: contains}
		if !ok || !ok2 || score.Validate() != nil {
			return nil, errOfflineScoring
		}
		seen[item.Index] = true
		out[item.Index] = score
	}
	return out, nil
}

type OfflineService struct {
	repo   d.OfflineRepository
	assets *lifecycle.Service
	queue  job.Queue
	scorer OfflineScorer
}

func NewOfflineService(repo d.OfflineRepository, assets *lifecycle.Service, queue job.Queue, scorer OfflineScorer) (*OfflineService, error) {
	if repo == nil || assets == nil || queue == nil || scorer == nil {
		return nil, d.ErrOfflineInvalid
	}
	return &OfflineService{repo, assets, queue, scorer}, nil
}
func offlinePrincipal(p d.OfflineScope) l.Principal {
	return l.Principal{TenantID: p.TenantID, UserID: p.UserID}
}
func (s *OfflineService) ValidateInputs(ctx context.Context, p d.OfflineScope, refs []l.RefID) error {
	if e := p.Validate(); e != nil {
		return e
	}
	if len(refs) != 1 {
		return d.ErrOfflineInvalid
	}
	return s.assets.ValidateGovernedInputs(ctx, offlinePrincipal(p), refs)
}
func (s *OfflineService) snapshot(ctx context.Context, p d.OfflineScope, req d.OfflineRequest) ([]d.OfflineItem, l.DataAssetRef, error) {
	if e := req.Validate(); e != nil {
		return nil, l.DataAssetRef{}, e
	}
	if e := s.ValidateInputs(ctx, p, []l.RefID{req.InputRef}); e != nil {
		return nil, l.DataAssetRef{}, e
	}
	ref, body, e := s.assets.Resolve(ctx, p.TenantID, p.UserID, req.InputRef)
	if e != nil {
		return nil, ref, e
	}
	if ref.Kind != l.RawAsset && ref.Kind != l.CleanedAsset && ref.Kind != l.EnrichedAsset {
		return nil, ref, d.ErrOfflineInvalid
	}
	items, e := offlineItems(body, req.Columns)
	return items, ref, e
}
func offlineItems(body json.RawMessage, columns d.OfflineColumns) ([]d.OfflineItem, error) {
	if columns.Validate() != nil || len(body) > 8<<20 {
		return nil, d.ErrOfflineInvalid
	}
	var input l.Input
	if json.Unmarshal(body, &input) != nil || input.Kind != "table" || input.Validate() != nil || len(input.Rows) > d.OfflineMaxRows {
		return nil, d.ErrOfflineInvalid
	}
	indexes := map[string]int{}
	for i, column := range input.Columns {
		if _, exists := indexes[column]; exists {
			return nil, d.ErrOfflineInvalid
		}
		indexes[column] = i
	}
	names := []string{columns.QuestionColumn, columns.ReferenceAnswerColumn, columns.GeneratedAnswerColumn}
	for _, name := range names {
		if _, exists := indexes[name]; !exists {
			return nil, d.ErrOfflineInvalid
		}
	}
	items := make([]d.OfflineItem, 0, len(input.Rows))
	for _, row := range input.Rows {
		if len(row) != len(input.Columns) {
			return nil, d.ErrOfflineInvalid
		}
		values := make([]string, 3)
		for i, name := range names {
			v, ok := row[indexes[name]].(string)
			if !ok {
				return nil, d.ErrOfflineInvalid
			}
			values[i] = v
		}
		items = append(items, d.OfflineItem{Question: values[0], ReferenceAnswer: values[1], GeneratedAnswer: values[2]})
	}
	if e := d.ValidateOfflineSnapshot(items, d.OfflineSnapshotHash(items)); e != nil {
		return nil, e
	}
	return items, nil
}
func (s *OfflineService) Submit(ctx context.Context, p d.OfflineScope, req d.OfflineRequest) (*d.OfflineRun, error) {
	items, _, e := s.snapshot(ctx, p, req)
	if e != nil {
		return nil, e
	}
	run, e := s.repo.Submit(ctx, p, req, items, d.OfflineSnapshotHash(items))
	if e != nil {
		return nil, e
	}
	if !run.Terminal() {
		j, _ := offlineJob(run)
		if e = s.queue.Enqueue(ctx, j); e != nil {
			return run, errors.New("offline evaluation queued in database; delivery recovery required")
		}
	}
	return run, nil
}
func (s *OfflineService) Lookup(ctx context.Context, p d.OfflineScope, req d.OfflineRequest) (*d.OfflineRun, error) {
	if p.Validate() != nil || req.Validate() != nil {
		return nil, d.ErrOfflineInvalid
	}
	run, e := s.repo.Lookup(ctx, p, req.OperationKey)
	if errors.Is(e, d.ErrTaskNotFound) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if !offlineRequestEqual(run.Request, req) {
		return nil, d.ErrOfflineConflict
	}
	return run, nil
}
func offlineRequestEqual(a, b d.OfflineRequest) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
func (s *OfflineService) CancelSubmission(ctx context.Context, p d.OfflineScope, req d.OfflineRequest) (*d.OfflineRun, error) {
	return s.repo.CancelSubmission(ctx, p, req)
}
func (s *OfflineService) Inspect(ctx context.Context, p d.OfflineScope, req d.OfflineRequest, id string) (*d.OfflineRun, error) {
	run, e := s.Lookup(ctx, p, req)
	if e != nil {
		return nil, e
	}
	if run == nil || run.ID != id {
		return nil, d.ErrOfflineConflict
	}
	if e = s.ValidateInputs(ctx, p, []l.RefID{req.InputRef}); e != nil {
		return nil, e
	}
	if run.OutputRef != nil {
		if _, _, e = s.assets.Resolve(ctx, p.TenantID, p.UserID, *run.OutputRef); e != nil {
			return nil, e
		}
	}
	return run, nil
}
func (s *OfflineService) Cancel(ctx context.Context, p d.OfflineScope, req d.OfflineRequest, id string) (*d.OfflineRun, error) {
	run, e := s.Lookup(ctx, p, req)
	if e != nil {
		return nil, e
	}
	if run == nil || run.ID != id {
		return nil, d.ErrOfflineConflict
	}
	run, e = s.repo.CancelSubmission(ctx, p, req)
	if e != nil {
		return nil, e
	}
	copy := *run
	if copy.OutputRef != nil {
		if s.ValidateInputs(ctx, p, []l.RefID{req.InputRef}) != nil {
			copy.OutputRef = nil
		} else if _, _, e = s.assets.Resolve(ctx, p.TenantID, p.UserID, *copy.OutputRef); e != nil {
			copy.OutputRef = nil
		}
	}
	return &copy, nil
}

// ValidateGovernedOrigin validates only the origin binding. Lifecycle validates
// parents separately, so resolving this output again here would recurse.
func (s *OfflineService) ValidateGovernedOrigin(ctx context.Context, p l.Principal, ref l.DataAssetRef) error {
	if !strings.HasPrefix(ref.ProducerID, "evaluation-governed:") || ref.Kind != l.Result || ref.TenantID != p.TenantID {
		return l.ErrForbidden
	}
	id := strings.TrimPrefix(ref.ProducerID, "evaluation-governed:")
	run, e := s.repo.Get(ctx, d.OfflineScope{TenantID: p.TenantID, UserID: p.UserID}, id)
	if e != nil {
		return l.ErrForbidden
	}
	if run.Status != d.OfflineSucceeded || run.OutputRef == nil || *run.OutputRef != ref.RefID || len(ref.Parents) != 1 || ref.Parents[0] != run.Request.InputRef {
		return l.ErrForbidden
	}
	return nil
}

// AuthorizeRead is shared by the existing evaluation HTTP service. It checks
// typed identity and current full input lineage before returning answer data.
func (s *OfflineService) AuthorizeRead(ctx context.Context, id string) error {
	tenant, tok := agentctx.GetTenantID(ctx)
	user, uok := agentctx.GetUserID(ctx)
	if !tok || !uok {
		return d.ErrOfflineForbidden
	}
	p := d.OfflineScope{TenantID: tenant, UserID: user}
	run, e := s.repo.Get(ctx, p, id)
	if e != nil {
		return d.ErrOfflineForbidden
	}
	if err := s.ValidateInputs(ctx, p, []l.RefID{run.Request.InputRef}); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return d.ErrOfflineForbidden
	}
	return nil
}

type offlinePayload struct {
	Scope d.OfflineScope `json:"scope"`
	ID    string         `json:"id"`
}

func offlineJob(run *d.OfflineRun) (job.Job, error) {
	return job.New("evaluation-offline:"+run.ID, OfflineJobType, offlinePayload{Scope: run.Scope, ID: run.ID})
}
func decodeOffline(j job.Job) (offlinePayload, error) {
	var p offlinePayload
	if j.Type != OfflineJobType || json.Unmarshal(j.Payload, &p) != nil || p.Scope.Validate() != nil || p.ID == "" || j.ID != "evaluation-offline:"+p.ID {
		return p, d.ErrOfflineInvalid
	}
	return p, nil
}
func (s *OfflineService) PendingJobs(ctx context.Context, limit int) ([]job.Job, error) {
	if limit <= 0 {
		limit = 100
	}
	runs, e := s.repo.Pending(ctx, limit)
	if e != nil {
		return nil, e
	}
	out := make([]job.Job, 0, len(runs))
	for i := range runs {
		j, e := offlineJob(&runs[i])
		if e != nil {
			return nil, e
		}
		out = append(out, j)
	}
	return out, nil
}
func (s *OfflineService) Handle(ctx context.Context, j job.Job) error {
	payload, e := decodeOffline(j)
	if e != nil {
		return e
	}
	run, e := s.repo.Claim(ctx, payload.Scope, payload.ID, offlineLease)
	if errors.Is(e, d.ErrOfflineConflict) {
		return nil
	}
	if e != nil {
		return e
	}
	if run.Terminal() {
		return nil
	}
	base := agentctx.WithUserID(agentctx.WithTenantID(ctx, run.Scope.TenantID), run.Scope.UserID)
	base = agentctx.WithRequestID(base, run.Request.RequestID)
	work, cancel := context.WithDeadline(base, run.ExecutionDeadline)
	defer cancel()
	// A domain lease, independent of Redis delivery, fences stale workers. Polling
	// also interrupts an in-flight HTTP scorer promptly after explicit cancellation.
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		poll := time.NewTicker(250 * time.Millisecond)
		defer poll.Stop()
		renew := time.NewTicker(5 * time.Second)
		defer renew.Stop()
		for {
			select {
			case <-work.Done():
				return
			case <-poll.C:
				current, err := s.repo.Get(work, run.Scope, run.ID)
				if err != nil || current.Terminal() || current.Epoch != run.Epoch {
					cancel()
					return
				}
			case <-renew.C:
				if s.repo.Renew(work, run.Scope, run.ID, run.Epoch, offlineLease) != nil {
					cancel()
					return
				}
			}
		}
	}()
	e = s.executeOffline(work, run)
	cancel()
	<-stopped
	persist, done := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer done()
	if e == nil {
		return nil
	}
	current, readErr := s.repo.Get(persist, run.Scope, run.ID)
	if readErr != nil {
		return readErr
	}
	if current.Terminal() || current.Epoch != run.Epoch {
		return nil
	}
	if !time.Now().Before(run.ExecutionDeadline) {
		_, e = s.repo.Finish(persist, run.Scope, run.ID, run.Epoch, d.OfflineTimedOut, "offline evaluation deadline exceeded", nil)
		return e
	}
	if errors.Is(e, context.Canceled) || ctx.Err() != nil {
		_ = s.repo.Release(persist, run.Scope, run.ID, run.Epoch)
		return context.Canceled
	}
	message := "offline scoring failed"
	if errors.Is(e, l.ErrForbidden) || errors.Is(e, l.ErrNotFound) || errors.Is(e, l.ErrExpired) || errors.Is(e, d.ErrOfflineForbidden) {
		message = "offline evaluation source access revoked"
	}
	if errors.Is(e, d.ErrOfflineInvalid) {
		message = "invalid immutable offline evaluation input"
	}
	_, e = s.repo.Finish(persist, run.Scope, run.ID, run.Epoch, d.OfflineFailed, message, nil)
	return e
}
func (s *OfflineService) executeOffline(ctx context.Context, run *d.OfflineRun) error {
	items, input, e := s.snapshot(ctx, run.Scope, run.Request)
	if e != nil {
		return e
	}
	if d.OfflineSnapshotHash(items) != run.SnapshotHash || d.ValidateOfflineSnapshot(run.Items, run.SnapshotHash) != nil {
		return d.ErrOfflineConflict
	}
	current := run
	for current.NextIndex < len(current.Items) {
		if e = s.ValidateInputs(ctx, current.Scope, []l.RefID{current.Request.InputRef}); e != nil {
			return e
		}
		end := current.NextIndex + d.OfflineBatchSize
		if end > len(current.Items) {
			end = len(current.Items)
		}
		scores, err := s.scorer.Score(ctx, current.Items[current.NextIndex:end])
		if err != nil {
			return err
		}
		if len(scores) != end-current.NextIndex {
			return errOfflineScoring
		}
		for i := range scores {
			if scores[i].Index != i || scores[i].Validate() != nil {
				return errOfflineScoring
			}
			scores[i].Index += current.NextIndex
		}
		current, e = s.repo.Checkpoint(ctx, current.Scope, current.ID, current.Epoch, current.NextIndex, scores)
		if e != nil {
			return e
		}
	}
	if e = s.ValidateInputs(ctx, current.Scope, []l.RefID{current.Request.InputRef}); e != nil {
		return e
	}
	body, _ := json.Marshal(map[string]any{"schema_version": 1, "mode": "offline_existing_answers", "profile": d.OfflineProfile, "evaluation_task_id": current.ID, "input_ref": current.Request.InputRef, "columns": current.Request.Columns, "snapshot_sha256": current.SnapshotHash, "total_count": len(current.Items), "scored_count": len(current.Scores), "items": current.Items, "scores": current.Scores, "aggregate": d.OfflineAverages(current.Scores), "model_calls": 0})
	ref, e := s.assets.ImportDerived(ctx, offlinePrincipal(current.Scope), l.Result, "evaluation-governed:"+current.ID, []l.RefID{current.Request.InputRef}, body, input.Policy)
	if e != nil {
		return e
	}
	_, e = s.repo.Finish(ctx, current.Scope, current.ID, current.Epoch, d.OfflineSucceeded, "", &ref.RefID)
	return e
}
func (s *OfflineService) Exhausted(ctx context.Context, j job.Job, _ error) error {
	p, e := decodeOffline(j)
	if e != nil {
		return e
	}
	run, e := s.repo.Claim(ctx, p.Scope, p.ID, offlineLease)
	if errors.Is(e, d.ErrOfflineConflict) {
		return nil
	}
	if e != nil || run.Terminal() {
		return e
	}
	_, e = s.repo.Finish(ctx, p.Scope, p.ID, run.Epoch, d.OfflineFailed, "offline evaluation delivery attempts exhausted", nil)
	return e
}
