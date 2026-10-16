package strategy

import (
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/strategy"
	"context"
	"encoding/json"
	"errors"
	"strings"
)

type TrialBinder func(context.Context, l.Principal, string) (planDigest string, maximum int, err error)

func (s *Service) WithOrchestration(binding TrialBinder, read RegressionReader) *Service {
	s.TrialBinding = binding
	s.OrchestrationRegression = read
	return s
}
func DefaultOrchestrationStrategy(context.Context, l.Principal) (json.RawMessage, error) {
	return json.RawMessage(`{"inherit_defaults":true}`), nil
}
func OrchestrationRunKey(taskID, planDigest string) string {
	return d.Digest([]string{"taskcontrol", taskID, planDigest})
}

// PrepareOrchestrationTrial only stores an isolated candidate pin. The task remains
// a draft and still requires the ordinary explicit Start command and all its gates.
func (s *Service) PrepareOrchestrationTrial(ctx context.Context, p l.Principal, id, taskID, key string) (*d.Snapshot, error) {
	if e := s.auth(ctx, p, "read"); e != nil {
		return nil, e
	}
	if !d.ValidKey(key) {
		return nil, d.ErrInvalid
	}
	if s.TrialBinding == nil {
		return nil, d.ErrUnavailable
	}
	v, e := s.Read(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if v.Kind != "orchestration" {
		return nil, d.ErrInvalid
	}
	digest, maximum, e := s.TrialBinding(ctx, p, taskID)
	if e != nil {
		return nil, e
	}
	if digest == "" || maximum < 1 || maximum > 8 {
		return nil, d.ErrInvalid
	}
	// Reserve the caller's complete intent before the resumable pin write. A
	// crash between these transactions can be retried only with that same intent;
	// this request receipt is not a claim that a trial or task has executed.
	hash := d.Digest([]string{id, taskID, digest})
	if e = s.Repo.InSlot(ctx, p, v.Slot, "", func(tx d.Tx) error {
		old, err := replay(ctx, tx, "trial_request", key, hash)
		if err != nil || old != nil {
			return err
		}
		_, err = event(ctx, tx, "trial_request", key, hash, id, map[string]string{"task_id": taskID, "plan_digest": digest})
		return err
	}); e != nil {
		return nil, e
	}
	effective, _ := json.Marshal(d.OrchestrationPolicy{MaxParallel: maximum})
	return s.PinEvaluation(ctx, p, id, OrchestrationRunKey(taskID, digest), effective)
}
func (s *Service) ResolveOrchestration(ctx context.Context, p l.Principal, taskID, planDigest string, planned int) (int, *d.Snapshot, error) {
	if taskID == "" || planDigest == "" || planned < 1 || planned > 8 {
		return 0, nil, d.ErrInvalid
	}
	effective, _ := json.Marshal(d.OrchestrationPolicy{MaxParallel: planned})
	snap, e := s.pin(ctx, p, "orchestration.default", "", OrchestrationRunKey(taskID, planDigest), false, true, effective)
	if errors.Is(e, d.ErrNotFound) {
		return planned, nil, nil
	}
	if e != nil {
		return 0, nil, e
	}
	if e = s.ValidateSnapshot(ctx, p, snap); e != nil {
		return 0, nil, e
	}
	var resolved d.OrchestrationPolicy
	if json.Unmarshal(snap.EffectiveBody, &resolved) != nil || resolved.MaxParallel < 1 || resolved.MaxParallel > 8 {
		return 0, nil, d.ErrInvalid
	}
	return min(planned, resolved.MaxParallel), snap, nil
}

// ReadOrchestrationPin never manufactures consumption evidence for a finished task.
func (s *Service) ReadOrchestrationPin(ctx context.Context, p l.Principal, taskID, planDigest string) (*d.Snapshot, error) {
	if e := s.auth(ctx, p, "read"); e != nil {
		return nil, e
	}
	event, e := s.Repo.FindEvent(ctx, p, "orchestration.default", "pin", OrchestrationRunKey(taskID, planDigest))
	if e != nil {
		return nil, e
	}
	snap, e := decode[*d.Snapshot](event)
	if e != nil || snap == nil {
		return nil, d.ErrConflict
	}
	snap.PinID = event.ID
	if e = s.ValidateSnapshot(ctx, p, snap); e != nil {
		return nil, e
	}
	return snap, nil
}
func (s *Service) regressionReader(kind string) RegressionReader {
	if kind == "orchestration" {
		return s.OrchestrationRegression
	}
	return s.ReadRegression
}

// captureDefault records the trusted existing behavior on first consumption.
// It grants no candidate approval and prevents a later initial registration from
// changing an already running request that previously had no policy head.
func (s *Service) captureDefault(ctx context.Context, p l.Principal, slot string) error {
	kind := strings.TrimSuffix(slot, ".default")
	consumer, ok := s.consumers[kind]
	if !ok || slot != kind+".default" || consumer.Baseline == nil {
		return d.ErrNotFound
	}
	body, e := consumer.Baseline(ctx, p)
	if e != nil {
		return e
	}
	if d.ValidateBody(kind, body) != nil {
		return d.ErrInvalid
	}
	return s.Repo.InSlot(ctx, p, slot, kind, func(tx d.Tx) error {
		h := tx.Head()
		if h.BaselineID != "" {
			return nil
		}
		version := &d.Version{Kind: kind, Body: body, EvidenceRefs: []string{}, SourceDigests: map[string]string{}, Gates: defaultGates}
		if e := tx.AppendVersion(ctx, version); e != nil {
			return e
		}
		h.BaselineID = version.ID
		h.ActiveID = version.ID
		h.Revision++
		if e := tx.SetHead(ctx); e != nil {
			return e
		}
		_, e := event(ctx, tx, "baseline", "first-consumption", version.Digest, version.ID, d.Activation{VersionID: version.ID, Digest: version.Digest, Revision: h.Revision, Reason: "captured_existing_default_before_first_consumption", Consumer: consumer.Name})
		return e
	})
}
