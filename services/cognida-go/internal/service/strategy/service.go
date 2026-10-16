// Package strategy governs policy data separately from runtime skill catalogs.
package strategy

import (
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/strategy"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type Assets interface {
	Resolve(context.Context, int64, int64, l.RefID) (l.DataAssetRef, json.RawMessage, error)
}
type Authorizer func(context.Context, l.Principal, string) error
type RegressionReader func(context.Context, l.Principal, string, string) (d.RegressionEvidence, error)
type Consumer struct {
	Name     string
	Baseline func(context.Context, l.Principal) (json.RawMessage, error)
}
type Service struct {
	Repo                    d.Repository
	Assets                  Assets
	Authorize               Authorizer
	ReadRegression          RegressionReader
	TrialBinding            TrialBinder
	OrchestrationRegression RegressionReader
	consumers               map[string]Consumer
}

func NewService(repo d.Repository, assets Assets, authorize Authorizer, consumers map[string]Consumer) *Service {
	return &Service{Repo: repo, Assets: assets, Authorize: authorize, consumers: consumers}
}
func (s *Service) WithRegressionReader(r RegressionReader) *Service { s.ReadRegression = r; return s }
func (s *Service) auth(ctx context.Context, p l.Principal, level string) error {
	if p.Validate() != nil || s.Authorize == nil || s.Authorize(ctx, p, level) != nil {
		return d.ErrForbidden
	}
	return nil
}
func (s *Service) sourceDigests(ctx context.Context, p l.Principal, refs []string) (map[string]string, error) {
	out := map[string]string{}
	if len(refs) > 32 {
		return nil, d.ErrInvalid
	}
	for _, raw := range refs {
		if _, ok := out[raw]; ok {
			return nil, d.ErrInvalid
		}
		id, e := l.ParseRefID(raw)
		if e != nil || s.Assets == nil {
			return nil, d.ErrForbidden
		}
		ref, _, e := s.Assets.Resolve(ctx, p.TenantID, p.UserID, id)
		if e != nil {
			return nil, d.ErrForbidden
		}
		out[raw] = ref.Provenance.Digest
	}
	return out, nil
}
func (s *Service) check(ctx context.Context, p l.Principal, v *d.Version, live bool) error {
	if v == nil || v.TenantID != p.TenantID || v.UserID != p.UserID {
		return d.ErrForbidden
	}
	if live && v.DisabledAt != nil {
		return d.ErrConflict
	}
	now, e := s.sourceDigests(ctx, p, v.EvidenceRefs)
	if e != nil {
		return e
	}
	if d.Digest(now) != d.Digest(v.SourceDigests) {
		return d.ErrConflict
	}
	return nil
}
func replay(ctx context.Context, tx d.Tx, kind, key, hash string) (*d.Event, error) {
	v, e := tx.Event(ctx, kind, key)
	if errors.Is(e, d.ErrNotFound) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if v.Hash != hash {
		return nil, d.ErrConflict
	}
	return v, nil
}
func event(ctx context.Context, tx d.Tx, kind, key, hash, version string, value any) (*d.Event, error) {
	raw, e := json.Marshal(value)
	if e != nil {
		return nil, e
	}
	v := &d.Event{Kind: kind, Key: key, Hash: hash, VersionID: version, Payload: raw}
	e = tx.AppendEvent(ctx, v)
	return v, e
}
func decode[T any](v *d.Event) (T, error) {
	var out T
	e := json.Unmarshal(v.Payload, &out)
	return out, e
}

var defaultGates = d.Gates{MinimumQuality: .5, MaximumQualityDrop: 0, MaximumErrorRate: 0, MaximumCostRatio: 1.2}

// RegisterBaseline captures the consumer's trusted actual default. Request data
// cannot select a convenient baseline or relax fixed regression gates.
func (s *Service) RegisterBaseline(ctx context.Context, p l.Principal, slot, kind, key string) (out *d.Version, err error) {
	if err = s.auth(ctx, p, "admin"); err != nil {
		return
	}
	if !d.ValidKey(key) || !d.ValidSlot(slot) || slot != kind+".default" {
		return nil, d.ErrInvalid
	}
	consumer, ok := s.consumers[kind]
	if !ok || consumer.Name == "" || consumer.Baseline == nil {
		return nil, d.ErrUnavailable
	}
	body, e := consumer.Baseline(ctx, p)
	if e != nil {
		return nil, e
	}
	if d.ValidateBody(kind, body) != nil {
		return nil, d.ErrInvalid
	}
	hash := d.Digest(struct{ Slot, Kind string }{slot, kind})
	err = s.Repo.InSlot(ctx, p, slot, kind, func(tx d.Tx) error {
		prev, e := replay(ctx, tx, "baseline", key, hash)
		if e != nil {
			return e
		}
		if prev != nil {
			out, e = tx.Version(ctx, prev.VersionID)
			return e
		}
		h := tx.Head()
		if h.BaselineID != "" {
			return d.ErrConflict
		}
		v := &d.Version{Kind: kind, Body: body, EvidenceRefs: []string{}, SourceDigests: map[string]string{}, Gates: defaultGates}
		if e = tx.AppendVersion(ctx, v); e != nil {
			return e
		}
		h.BaselineID = v.ID
		h.ActiveID = v.ID
		h.Revision++
		if e = tx.SetHead(ctx); e != nil {
			return e
		}
		_, e = event(ctx, tx, "baseline", key, hash, v.ID, d.Activation{VersionID: v.ID, Digest: v.Digest, Revision: h.Revision, Reason: "captured_existing_default", Consumer: consumer.Name})
		out = v
		return e
	})
	return
}

type CandidateInput struct {
	Kind         string          `json:"kind"`
	Body         json.RawMessage `json:"body"`
	BaselineID   string          `json:"baseline_id"`
	EvidenceRefs []string        `json:"evidence_refs"`
}

func (s *Service) CreateCandidate(ctx context.Context, p l.Principal, slot string, in CandidateInput, key string) (out *d.Version, err error) {
	if err = s.auth(ctx, p, "read"); err != nil {
		return
	}
	if !d.ValidKey(key) || d.ValidateBody(in.Kind, in.Body) != nil || len(in.EvidenceRefs) < 1 {
		return nil, d.ErrInvalid
	}
	digests, e := s.sourceDigests(ctx, p, in.EvidenceRefs)
	if e != nil {
		return nil, e
	}
	hash := d.Digest(in)
	err = s.Repo.InSlot(ctx, p, slot, "", func(tx d.Tx) error {
		base, e := tx.Version(ctx, in.BaselineID)
		if e != nil {
			return e
		}
		if e = s.check(ctx, p, base, true); e != nil {
			return e
		}
		if base.Kind != in.Kind {
			return d.ErrConflict
		}
		prev, e := replay(ctx, tx, "candidate", key, hash)
		if e != nil {
			return e
		}
		if prev != nil {
			out, e = tx.Version(ctx, prev.VersionID)
			if e == nil {
				e = s.check(ctx, p, out, false)
			}
			return e
		}
		out = &d.Version{Kind: in.Kind, Body: in.Body, BaselineID: base.ID, EvidenceRefs: in.EvidenceRefs, SourceDigests: digests, Gates: base.Gates}
		if e = tx.AppendVersion(ctx, out); e != nil {
			return e
		}
		_, e = event(ctx, tx, "candidate", key, hash, out.ID, struct {
			BaselineDigest string          `json:"baseline_digest"`
			Before         json.RawMessage `json:"before"`
			After          json.RawMessage `json:"after"`
		}{base.Digest, base.Body, out.Body})
		return e
	})
	return
}

type RegressionInput struct {
	BaselineTaskID  string `json:"baseline_task_id"`
	CandidateTaskID string `json:"candidate_task_id"`
}

func (s *Service) Regress(ctx context.Context, p l.Principal, slot, id string, in RegressionInput, key string) (out *d.Event, err error) {
	if err = s.auth(ctx, p, "read"); err != nil {
		return
	}
	if !d.ValidKey(key) || in.BaselineTaskID == "" || in.CandidateTaskID == "" || in.BaselineTaskID == in.CandidateTaskID {
		return nil, d.ErrInvalid
	}
	version, e := s.Read(ctx, p, id)
	if e != nil {
		return nil, e
	}
	reader := s.regressionReader(version.Kind)
	if reader == nil {
		return nil, d.ErrUnavailable
	}
	evidence, e := reader(ctx, p, in.BaselineTaskID, in.CandidateTaskID)
	if e != nil {
		return nil, e
	}
	hash := d.Digest(struct {
		ID    string
		Input RegressionInput
	}{id, in})
	err = s.Repo.InSlot(ctx, p, slot, "", func(tx d.Tx) error {
		v, e := tx.Version(ctx, id)
		if e != nil {
			return e
		}
		if e = s.check(ctx, p, v, true); e != nil {
			return e
		}
		base, e := tx.Version(ctx, v.BaselineID)
		if e != nil {
			return e
		}
		if e = s.check(ctx, p, base, true); e != nil {
			return e
		}
		if evidence.CandidateVersionID != id || evidence.BaselineVersionID != v.BaselineID || evidence.DatasetVersionID == "" || evidence.DatasetManifest == "" || evidence.ConfigurationDigest == "" || evidence.GraderVersion == "" || len(evidence.Samples) == 0 || !json.Valid(evidence.Samples) {
			return d.ErrConflict
		}
		prev, e := replay(ctx, tx, "regression", key, hash)
		if e != nil {
			return e
		}
		if prev != nil {
			out = prev
			return nil
		}
		pass, reasons := d.Compare(v.Gates, evidence.Baseline, evidence.Candidate)
		out, e = event(ctx, tx, "regression", key, hash, id, d.Regression{Evidence: evidence, Gates: v.Gates, Passed: pass, Reasons: reasons})
		return e
	})
	return
}

type ApprovalInput struct {
	Digest       string `json:"digest"`
	RegressionID string `json:"regression_id"`
	Approved     bool   `json:"approved"`
	Comment      string `json:"comment"`
}

func (s *Service) Approve(ctx context.Context, p l.Principal, slot, id string, in ApprovalInput, key string) (out *d.Event, err error) {
	if err = s.auth(ctx, p, "admin"); err != nil {
		return
	}
	if !d.ValidKey(key) || len(in.Comment) > 4000 {
		return nil, d.ErrInvalid
	}
	hash := d.Digest(struct {
		ID    string
		Input ApprovalInput
	}{id, in})
	err = s.Repo.InSlot(ctx, p, slot, "", func(tx d.Tx) error {
		v, e := tx.Version(ctx, id)
		if e != nil {
			return e
		}
		if e = s.check(ctx, p, v, true); e != nil {
			return e
		}
		if v.Digest != in.Digest {
			return d.ErrConflict
		}
		reg, e := tx.EventByID(ctx, in.RegressionID)
		if e != nil {
			return e
		}
		r, e := decode[d.Regression](reg)
		if e != nil || reg.Kind != "regression" || reg.VersionID != id {
			return d.ErrConflict
		}
		if in.Approved && !r.Passed {
			return d.ErrGate
		}
		prev, e := replay(ctx, tx, "approval", key, hash)
		if e != nil {
			return e
		}
		if prev != nil {
			out = prev
			return nil
		}
		// An immutable version receives one final review decision. A rejected or
		// superseded proposal must create a new version, never reuse old approval.
		if _, err := tx.Event(ctx, "approval_decision", id); err == nil {
			return d.ErrConflict
		} else if !errors.Is(err, d.ErrNotFound) {
			return err
		}
		out, e = event(ctx, tx, "approval", key, hash, id, d.Approval{Digest: in.Digest, RegressionID: reg.ID, ReviewerID: p.UserID, Approved: in.Approved, Comment: in.Comment})
		if e != nil {
			return e
		}
		_, e = event(ctx, tx, "approval_decision", id, hash, id, map[string]string{"approval_id": out.ID})
		return e
	})
	return
}

type ActivateInput struct {
	Digest           string `json:"digest"`
	ApprovalID       string `json:"approval_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	Reason           string `json:"reason"`
	Rollback         bool   `json:"rollback"`
}

func (s *Service) Activate(ctx context.Context, p l.Principal, slot, id string, in ActivateInput, key string) (out *d.Event, err error) {
	if err = s.auth(ctx, p, "admin"); err != nil {
		return
	}
	if !d.ValidKey(key) || strings.TrimSpace(in.Reason) == "" || len(in.Reason) > 4000 {
		return nil, d.ErrInvalid
	}
	hash := d.Digest(struct {
		ID    string
		Input ActivateInput
	}{id, in})
	err = s.Repo.InSlot(ctx, p, slot, "", func(tx d.Tx) error {
		v, e := tx.Version(ctx, id)
		if e != nil {
			return e
		}
		if e = s.check(ctx, p, v, true); e != nil {
			return e
		}
		consumer, ok := s.consumers[v.Kind]
		if !ok || consumer.Name == "" {
			return d.ErrUnavailable
		}
		if v.Digest != in.Digest {
			return d.ErrConflict
		}
		prev, e := replay(ctx, tx, "activation", key, hash)
		if e != nil {
			return e
		}
		if prev != nil {
			out = prev
			return nil
		}
		h := tx.Head()
		if in.Rollback && id != h.BaselineID {
			if _, err := tx.Event(ctx, "activated_version", id); err != nil {
				return d.ErrConflict
			}
		}
		if h.Revision != in.ExpectedRevision {
			return d.ErrConflict
		}
		if id != h.BaselineID {
			ap, e := tx.EventByID(ctx, in.ApprovalID)
			if e != nil {
				return e
			}
			a, e := decode[d.Approval](ap)
			if e != nil || ap.Kind != "approval" || ap.VersionID != id || !a.Approved || a.Digest != v.Digest {
				return d.ErrConflict
			}
			reg, e := tx.EventByID(ctx, a.RegressionID)
			if e != nil {
				return e
			}
			r, e := decode[d.Regression](reg)
			if e != nil || !r.Passed {
				return d.ErrGate
			}
			reader := s.regressionReader(v.Kind)
			if reader == nil {
				return d.ErrUnavailable
			}
			latest, e := reader(ctx, p, r.Evidence.BaselineTaskID, r.Evidence.CandidateTaskID)
			if e != nil {
				return e
			}
			if d.Digest(latest) != d.Digest(r.Evidence) {
				return d.ErrConflict
			}
			if !in.Rollback && h.ActiveID != v.BaselineID {
				return d.ErrConflict
			}
		} else if !in.Rollback {
			return d.ErrConflict
		}
		old := h.ActiveID
		h.ActiveID = id
		h.Revision++
		if e = tx.SetHead(ctx); e != nil {
			return e
		}
		out, e = event(ctx, tx, "activation", key, hash, id, d.Activation{VersionID: id, Digest: v.Digest, PreviousVersionID: old, Revision: h.Revision, ApprovalID: in.ApprovalID, Reason: in.Reason, Consumer: consumer.Name})
		if e != nil {
			return e
		}
		if _, err := tx.Event(ctx, "activated_version", id); errors.Is(err, d.ErrNotFound) {
			_, e = event(ctx, tx, "activated_version", id, v.Digest, id, map[string]string{"first_activation_id": out.ID})
		} else if err != nil {
			return err
		}
		return e
	})
	return
}
func (s *Service) Disable(ctx context.Context, p l.Principal, slot, id, reason, key string) (out *d.Event, err error) {
	if err = s.auth(ctx, p, "admin"); err != nil {
		return
	}
	if !d.ValidKey(key) || strings.TrimSpace(reason) == "" || len(reason) > 4000 {
		return nil, d.ErrInvalid
	}
	hash := d.Digest([]string{id, reason})
	err = s.Repo.InSlot(ctx, p, slot, "", func(tx d.Tx) error {
		v, e := tx.Version(ctx, id)
		if e != nil {
			return e
		}
		prev, e := replay(ctx, tx, "disable", key, hash)
		if e != nil {
			return e
		}
		if prev != nil {
			out = prev
			return nil
		}
		if e = tx.DisableVersion(ctx, id); e != nil {
			return e
		}
		if tx.Head().ActiveID == id {
			tx.Head().ActiveID = ""
			tx.Head().Revision++
			if e = tx.SetHead(ctx); e != nil {
				return e
			}
		}
		out, e = event(ctx, tx, "disable", key, hash, v.ID, map[string]string{"reason": reason})
		return e
	})
	return
}
func (s *Service) Read(ctx context.Context, p l.Principal, id string) (*d.Version, error) {
	if e := s.auth(ctx, p, "read"); e != nil {
		return nil, e
	}
	v, e := s.Repo.Version(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if e = s.check(ctx, p, v, false); e != nil {
		return nil, e
	}
	return v, nil
}
func (s *Service) List(ctx context.Context, p l.Principal, slot, cursor string, limit int) (*d.Page[d.Version], error) {
	if e := s.auth(ctx, p, "read"); e != nil {
		return nil, e
	}
	page, e := s.Repo.Versions(ctx, p, slot, cursor, limit)
	if e != nil {
		return nil, e
	}
	for i := range page.Items {
		if e = s.check(ctx, p, &page.Items[i], false); e != nil {
			return nil, e
		}
	}
	return page, nil
}
func (s *Service) Events(ctx context.Context, p l.Principal, slot, kind, cursor string, limit int) (*d.Page[d.Event], error) {
	if e := s.auth(ctx, p, "read"); e != nil {
		return nil, e
	}
	page, e := s.Repo.Events(ctx, p, slot, kind, cursor, limit)
	if e != nil {
		return nil, e
	}
	for _, v := range page.Items {
		if _, e = s.Read(ctx, p, v.VersionID); e != nil {
			return nil, e
		}
		// Regression payloads contain real evaluated samples. Their dataset
		// authority is separate from the candidate's improvement evidence.
		if e = s.authorizeEvent(ctx, p, &v); e != nil {
			return nil, e
		}
	}
	return page, nil
}
func (s *Service) Head(ctx context.Context, p l.Principal, slot string) (*d.Head, error) {
	if e := s.auth(ctx, p, "read"); e != nil {
		return nil, e
	}
	return s.Repo.Head(ctx, p, slot)
}

// Pin persists the version consumed by a run. A retry never selects today's default.
func (s *Service) Pin(ctx context.Context, p l.Principal, slot, runKey string, effective ...json.RawMessage) (*d.Snapshot, error) {
	return s.pin(ctx, p, slot, "", runKey, false, false, effective...)
}

// PinEvaluation permits unapproved candidates only in the separate evaluation path.
func (s *Service) PinEvaluation(ctx context.Context, p l.Principal, versionID, runKey string, effective ...json.RawMessage) (*d.Snapshot, error) {
	v, e := s.Read(ctx, p, versionID)
	if e != nil {
		return nil, e
	}
	return s.pin(ctx, p, v.Slot, versionID, runKey, true, false, effective...)
}
func (s *Service) pin(ctx context.Context, p l.Principal, slot, versionID, key string, evaluation bool, prepared bool, effective ...json.RawMessage) (out *d.Snapshot, err error) {
	if err = s.auth(ctx, p, "read"); err != nil {
		return
	}
	if !d.ValidKey(key) {
		return nil, d.ErrInvalid
	}
	kind := "pin"
	if evaluation {
		kind = "evaluation_pin"
	}
	hash := d.Digest([]string{slot, versionID})
	err = s.Repo.InSlot(ctx, p, slot, "", func(tx d.Tx) error {
		prev, e := replay(ctx, tx, kind, key, hash)
		if e != nil {
			return e
		}
		if evaluation {
			if _, err := tx.Event(ctx, "pin", key); err == nil {
				return d.ErrConflict
			} else if !errors.Is(err, d.ErrNotFound) {
				return err
			}
		}
		id := versionID
		if prev == nil && prepared {
			trial, err := tx.Event(ctx, "evaluation_pin", key)
			if err == nil {
				id = trial.VersionID
			} else if !errors.Is(err, d.ErrNotFound) {
				return err
			}
		}
		if prev != nil {
			id = prev.VersionID
		} else if id == "" {
			id = tx.Head().ActiveID
		}
		if id == "" {
			return d.ErrConflict
		}
		v, e := tx.Version(ctx, id)
		if e != nil {
			return e
		}
		if e = s.check(ctx, p, v, true); e != nil {
			return e
		}
		if _, ok := s.consumers[v.Kind]; !ok {
			return d.ErrUnavailable
		}
		if prev != nil {
			out, e = decode[*d.Snapshot](prev)
			if e != nil || out == nil {
				return d.ErrConflict
			}
		} else {
			resolved := v.Body
			var inherit struct {
				Inherit bool `json:"inherit_defaults"`
			}
			_ = json.Unmarshal(v.Body, &inherit)
			if inherit.Inherit && len(effective) > 0 {
				resolved = effective[0]
			}
			if len(effective) > 1 || !json.Valid(resolved) || len(resolved) > 64<<10 {
				return d.ErrInvalid
			}
			out = &d.Snapshot{VersionID: v.ID, Slot: slot, Kind: v.Kind, Digest: v.Digest, Body: v.Body, EffectiveBody: resolved, CacheIdentity: d.Digest([]string{fmt.Sprint(p.TenantID), fmt.Sprint(p.UserID), v.ID, v.Digest, d.Digest(resolved)})}
		}
		if prev == nil {
			prev, e = event(ctx, tx, kind, key, hash, id, out)
			if e != nil {
				return e
			}
		}
		out.PinID = prev.ID
		return nil
	})
	if errors.Is(err, d.ErrNotFound) && !evaluation {
		if _, headErr := s.Repo.Head(ctx, p, slot); !errors.Is(headErr, d.ErrNotFound) {
			return out, err
		}
		if e := s.captureDefault(ctx, p, slot); e != nil {
			return nil, e
		}
		return s.pin(ctx, p, slot, versionID, key, evaluation, prepared, effective...)
	}
	return
}

// Active reads the effective policy without creating a pin or changing state.
// It is suitable for previews; execution must use Pin and ValidateSnapshot.
func (s *Service) Active(ctx context.Context, p l.Principal, slot string) (*d.Version, error) {
	if e := s.auth(ctx, p, "read"); e != nil {
		return nil, e
	}
	h, e := s.Repo.Head(ctx, p, slot)
	if e != nil {
		return nil, e
	}
	if h.ActiveID == "" {
		return nil, d.ErrConflict
	}
	v, e := s.Read(ctx, p, h.ActiveID)
	if e != nil {
		return nil, e
	}
	if e = s.check(ctx, p, v, true); e != nil {
		return nil, e
	}
	if _, ok := s.consumers[v.Kind]; !ok {
		return nil, d.ErrUnavailable
	}
	return v, nil
}

// ValidateSnapshotRead keeps historical results readable after disabling a policy,
// but always rechecks its source authority and immutable receipt binding.
func (s *Service) ValidateSnapshotRead(ctx context.Context, p l.Principal, snap *d.Snapshot) error {
	return s.validateSnapshot(ctx, p, snap, false)
}
func (s *Service) ValidateSnapshot(ctx context.Context, p l.Principal, snap *d.Snapshot) error {
	return s.validateSnapshot(ctx, p, snap, true)
}
func (s *Service) validateSnapshot(ctx context.Context, p l.Principal, snap *d.Snapshot, live bool) error {
	if snap == nil {
		return d.ErrInvalid
	}
	v, e := s.Read(ctx, p, snap.VersionID)
	if e != nil {
		return e
	}
	if e = s.check(ctx, p, v, live); e != nil {
		return e
	}
	if v.Digest != snap.Digest || v.Kind != snap.Kind || v.Slot != snap.Slot || d.Digest(v.Body) != d.Digest(snap.Body) {
		return d.ErrConflict
	}
	receipt, e := s.Repo.ReadEvent(ctx, p, v.Slot, snap.PinID)
	if e != nil {
		return e
	}
	var stored d.Snapshot
	if json.Unmarshal(receipt.Payload, &stored) != nil || stored.CacheIdentity != snap.CacheIdentity || d.Digest(stored.EffectiveBody) != d.Digest(snap.EffectiveBody) || stored.Digest != snap.Digest || stored.Kind != snap.Kind || stored.Slot != snap.Slot || d.Digest(stored.Body) != d.Digest(snap.Body) || receipt.VersionID != v.ID || (receipt.Kind != "pin" && receipt.Kind != "evaluation_pin") {
		return d.ErrConflict
	}
	return nil
}

func (s *Service) authorizeEvent(ctx context.Context, p l.Principal, event *d.Event) error {
	v, e := s.Read(ctx, p, event.VersionID)
	if e != nil {
		return e
	}
	reader := s.regressionReader(v.Kind)
	switch event.Kind {
	case "regression":
		if reader == nil {
			return d.ErrUnavailable
		}
		regression, e := decode[d.Regression](event)
		if e != nil {
			return d.ErrConflict
		}
		_, e = reader(ctx, p, regression.Evidence.BaselineTaskID, regression.Evidence.CandidateTaskID)
		return e
	case "observation":
		if reader == nil {
			return d.ErrUnavailable
		}
		observation, e := decode[Observation](event)
		if e != nil {
			return d.ErrConflict
		}
		_, e = reader(ctx, p, observation.TaskID, observation.TaskID)
		return e
	}
	return nil
}
