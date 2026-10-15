package lifecycle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	d "cognida/internal/model/lifecycle"
)

// governedOption is reserved for trusted in-process coordination. Ordinary
// invocations cannot supply it, including through pipeline stage_options.
const governedOption = "_governed_execution"

func governedTimeoutError() error {
	return fmt.Errorf("governed execution deadline exceeded: %w", context.DeadlineExceeded)
}

// GovernedTimedOut distinguishes a persisted controller deadline from an
// arbitrary upstream error string. It does not change lifecycle's status enum.
func GovernedTimedOut(st *d.StageRun) bool {
	bound, err := governedFromStage(st)
	return err == nil && bound != nil && st.Status == d.StatusFailed && st.Error == governedTimeoutError().Error()
}

type GovernedPreset struct {
	Profile           string
	BaseCalls         int
	MaxTimeoutSeconds int
}

func GovernedPresetFor(agent string) (GovernedPreset, bool) {
	profiles := map[string]GovernedPreset{
		"data-quality":    {"quality-default-v1; min_score=70", 1, 300},
		"data-cleaning":   {"clean-trim-dedup-v1; keep=first; min_score=70", 2, 300},
		"data-enrichment": {"enrich-rules-v1; keywords,summary,sensitivity", 1, 300},
		"data-evaluation": {"artifact-integrity-and-provenance-v1; no model accuracy claims", 1, 300},
	}
	v, ok := profiles[agent]
	return v, ok
}

// GovernedInvocation is an immutable domain submission, not an HTTP DTO.
// A parent attempt maps to one domain attempt; retries belong to the parent.
type GovernedInvocation struct {
	OperationKey   string    `json:"operation_key"`
	PayloadHash    string    `json:"payload_hash"`
	AgentID        string    `json:"agent_id"`
	Profile        string    `json:"profile"`
	InputRefs      []d.RefID `json:"input_refs"`
	TimeoutSeconds int       `json:"timeout_seconds"`
	MaxCalls       int       `json:"max_calls"`
	CallsReserved  int       `json:"calls_reserved"`
	Deadline       time.Time `json:"deadline"`
}

func governedDigest(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
func (in GovernedInvocation) validate() error {
	preset, ok := GovernedPresetFor(in.AgentID)
	if !ok || !governedDigest(in.OperationKey) || !governedDigest(in.PayloadHash) || in.Profile != preset.Profile || len(in.InputRefs) < 1 || len(in.InputRefs) > 16 || in.TimeoutSeconds < 1 || in.TimeoutSeconds > preset.MaxTimeoutSeconds || in.MaxCalls < 1 || in.MaxCalls > 128 || in.CallsReserved != len(in.InputRefs)+preset.BaseCalls || in.CallsReserved > in.MaxCalls || in.Deadline.IsZero() {
		return d.ErrInvalid
	}
	seen := map[string]bool{}
	for _, ref := range in.InputRefs {
		if ref.Validate() != nil || seen[ref.String()] {
			return d.ErrInvalid
		}
		seen[ref.String()] = true
	}
	return nil
}
func governedKey(p d.Principal, operation string) string {
	raw := fmt.Sprintf("%d/%d/%s", p.TenantID, p.UserID, operation)
	sum := sha256.Sum256([]byte(raw))
	return "governed:" + hex.EncodeToString(sum[:])
}
func governedOptions(in GovernedInvocation) map[string]any {
	options := map[string]any{governedOption: in}
	switch in.AgentID {
	case "data-quality":
		options["min_score"] = float64(70)
	case "data-cleaning":
		options["min_score"] = float64(70)
		options["operations"] = []string{"trim", "dedup"}
		options["keep"] = "first"
	case "data-enrichment":
		options["method"] = "rules"
		options["features"] = []string{"keywords", "summary", "sensitivity"}
		options["max_keywords"] = 10
		options["max_summary_chars"] = 300
	}
	return options
}

// Stored JSON objects are maps with lexicographic keys; normalize nested
// structs before comparing so persistence does not alter the binding.
func governedSameJSON(a, b any) bool {
	normalize := func(value any) ([]byte, error) {
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		var decoded any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err = decoder.Decode(&decoded); err != nil {
			return nil, err
		}
		return json.Marshal(decoded)
	}
	x, ex := normalize(a)
	y, ey := normalize(b)
	return ex == nil && ey == nil && bytes.Equal(x, y)
}

func governedFromStage(st *d.StageRun) (*GovernedInvocation, error) {
	raw, exists := st.Options[governedOption]
	if !exists {
		return nil, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, d.ErrInvalid
	}
	var in GovernedInvocation
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil || in.validate() != nil || st.MaxAttempts != 1 || st.RunID != "" || st.AgentID != in.AgentID || st.Approved || st.ApprovalRequired || !sameJSON(st.InputRefs, in.InputRefs) || !governedSameJSON(st.Options, governedOptions(in)) || st.IdempotencyKey != governedKey(d.Principal{TenantID: st.TenantID, UserID: st.UserID}, in.OperationKey) {
		return nil, d.ErrInvalid
	}
	return &in, nil
}

func governedSourceError(err error) error {
	if errors.Is(err, d.ErrConflict) || errors.Is(err, d.ErrNotFound) || errors.Is(err, d.ErrExpired) {
		return d.ErrForbidden
	}
	return err
}

func (s *Service) authorizeGoverned(ctx context.Context, p d.Principal) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || s.config.Authorize == nil || p.Validate() != nil {
		return d.ErrForbidden
	}
	return s.authorize(ctx, p, "read")
}

// ValidateGovernedInputs permits only integrity-checked, available snapshots
// whose complete parent chain ends at a manual upload. External and legacy
// unknown roots remain disabled until original-source revocation is supported.
func (s *Service) ValidateGovernedInputs(ctx context.Context, p d.Principal, refs []d.RefID) error {
	if err := s.authorizeGoverned(ctx, p); err != nil {
		return err
	}
	if len(refs) < 1 || len(refs) > 16 {
		return d.ErrInvalid
	}
	visited, active := map[string]bool{}, map[string]bool{}
	var visit func(d.RefID, int) error
	visit = func(id d.RefID, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth > 16 || id.Validate() != nil || active[id.String()] {
			return d.ErrInvalid
		}
		if visited[id.String()] {
			return nil
		}
		if len(visited)+len(active) >= 128 {
			return d.ErrInvalid
		}
		active[id.String()] = true
		defer delete(active, id.String())
		ref, body, err := s.Resolve(ctx, p.TenantID, p.UserID, id)
		if err != nil {
			// Revoked, removed and expired lineage is an authorization failure,
			// not an idempotency conflict that the parent should keep polling.
			return governedSourceError(err)
		}
		if ref.TenantID != p.TenantID || ref.Provenance.SourceType != "lifecycle" || ref.Provenance.SourceID != ref.ProducerID {
			return d.ErrForbidden
		}
		if len(ref.Parents) == 0 {
			if ref.Kind != d.RawAsset || !strings.HasPrefix(ref.ProducerID, "upload:") || len(ref.ProducerID) <= len("upload:") {
				return d.ErrForbidden
			}
			var input d.Input
			if json.Unmarshal(body, &input) != nil || input.Validate() != nil {
				return d.ErrInvalid
			}
		} else {
			if len(ref.Parents) > 16 {
				return d.ErrInvalid
			}
			for _, parent := range ref.Parents {
				if err := visit(parent, depth+1); err != nil {
					return err
				}
			}
		}
		visited[id.String()] = true
		return nil
	}
	for _, ref := range refs {
		if err := visit(ref, 0); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) governedLookup(ctx context.Context, p d.Principal, in GovernedInvocation, validateInputs bool) (*d.StageRun, error) {
	if err := s.authorizeGoverned(ctx, p); err != nil {
		return nil, err
	}
	if err := in.validate(); err != nil {
		return nil, err
	}
	if validateInputs {
		if err := s.ValidateGovernedInputs(ctx, p, in.InputRefs); err != nil {
			return nil, err
		}
	}
	st, err := s.repo.FindStageByKey(ctx, p.TenantID, governedKey(p, in.OperationKey))
	if errors.Is(err, d.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if st.TenantID != p.TenantID || st.UserID != p.UserID {
		return nil, d.ErrForbidden
	}
	bound, err := governedFromStage(st)
	if err != nil || bound == nil || !sameJSON(bound, in) {
		return nil, d.ErrConflict
	}
	return st, nil
}

func (s *Service) LookupGoverned(ctx context.Context, p d.Principal, in GovernedInvocation) (*d.StageRun, error) {
	// A receipt remains discoverable after source revocation for cancellation.
	return s.governedLookup(ctx, p, in, false)
}

func (s *Service) InvokeGoverned(ctx context.Context, p d.Principal, in GovernedInvocation, requestID string) (*d.StageRun, error) {
	if err := s.ValidateGovernedInputs(ctx, p, in.InputRefs); err != nil {
		return nil, err
	}
	existing, err := s.LookupGoverned(ctx, p, in)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	if !in.Deadline.After(time.Now()) {
		return nil, context.DeadlineExceeded
	}
	if err = s.validateInputs(ctx, p, in.AgentID, in.InputRefs); err != nil {
		return nil, err
	}
	// Round-trip to detach caller-owned slices/maps before persistence.
	raw, _ := json.Marshal(governedOptions(in))
	var options map[string]any
	if err = json.Unmarshal(raw, &options); err != nil {
		return nil, d.ErrInvalid
	}
	stage := newStage(p, "", in.AgentID, in.AgentID, append([]d.RefID(nil), in.InputRefs...), options, governedKey(p, in.OperationKey), requestID, 1)
	if err = s.repo.CreateStage(ctx, &stage); err != nil {
		// The database unique key is authoritative under concurrent submission.
		existing, lookupErr := s.LookupGoverned(ctx, p, in)
		if lookupErr != nil {
			return nil, lookupErr
		}
		if existing != nil {
			return existing, nil
		}
		return nil, err
	}
	if err = s.enqueue(ctx, stage); err != nil {
		return &stage, err
	}
	return &stage, nil
}

func (s *Service) InspectGoverned(ctx context.Context, p d.Principal, in GovernedInvocation, id string) (*d.StageRun, error) {
	st, err := s.governedLookup(ctx, p, in, true)
	if err != nil {
		return nil, err
	}
	if st == nil || st.ID != id {
		return nil, d.ErrNotFound
	}
	if st.Output != nil {
		for _, output := range st.Output.Refs {
			if _, _, err := s.Resolve(ctx, p.TenantID, p.UserID, output.RefID); err != nil {
				return nil, governedSourceError(err)
			}
		}
	}
	return st, nil
}

// CancelGovernedSubmission uses the same unique key as Submit. A cancelled
// tombstone fences a late Submit; if Submit won, cancel its actual record.
func (s *Service) CancelGovernedSubmission(ctx context.Context, p d.Principal, in GovernedInvocation, requestID string) (*d.StageRun, error) {
	st, err := s.LookupGoverned(ctx, p, in)
	if err != nil {
		return nil, err
	}
	if st == nil {
		raw, _ := json.Marshal(governedOptions(in))
		var options map[string]any
		if json.Unmarshal(raw, &options) != nil {
			return nil, d.ErrInvalid
		}
		tombstone := newStage(p, "", in.AgentID, in.AgentID, append([]d.RefID(nil), in.InputRefs...), options, governedKey(p, in.OperationKey), requestID, 1)
		tombstone.Status = d.StatusCancelled
		if err = s.repo.CreateStage(ctx, &tombstone); err == nil {
			return &tombstone, nil
		}
		st, err = s.LookupGoverned(ctx, p, in)
		if err != nil {
			return nil, err
		}
		if st == nil {
			return nil, d.ErrConflict
		}
	}
	return s.CancelGoverned(ctx, p, in, st.ID)
}

// Cancellation remains possible after input revocation. The owner and immutable
// binding still have to match, and a completed stage remains completed. Existing
// outputs are exposed only if the complete source chain is currently readable.
func (s *Service) CancelGoverned(ctx context.Context, p d.Principal, in GovernedInvocation, id string) (*d.StageRun, error) {
	st, err := s.governedLookup(ctx, p, in, false)
	if err != nil {
		return nil, err
	}
	if st == nil || st.ID != id {
		return nil, d.ErrNotFound
	}
	if err = s.CancelStage(ctx, p, id); err != nil {
		return nil, err
	}
	st, err = s.governedLookup(ctx, p, in, false)
	if err == nil && st != nil && st.Output != nil {
		visible := s.ValidateGovernedInputs(ctx, p, in.InputRefs) == nil
		if visible {
			for _, output := range st.Output.Refs {
				if _, _, resolveErr := s.Resolve(ctx, p.TenantID, p.UserID, output.RefID); resolveErr != nil {
					visible = false
					break
				}
			}
		}
		if !visible {
			copy := *st
			copy.Output = nil
			st = &copy
		}
	}
	return st, err
}

// governedStageContext rehydrates persisted limits on every delivery. It never
// derives authority from an arbitrary caller's context or allows custom code.
func (s *Service) governedStageContext(ctx context.Context, st *d.StageRun) (context.Context, context.CancelFunc, bool, error) {
	in, err := governedFromStage(st)
	if err != nil {
		return nil, nil, false, err
	}
	limit := time.Now().Add(s.config.StageTimeout)
	if in != nil {
		if candidate := time.Now().Add(time.Duration(in.TimeoutSeconds) * time.Second); candidate.Before(limit) {
			limit = candidate
		}
		if in.Deadline.Before(limit) {
			limit = in.Deadline
		}
	}
	work, cancel := context.WithDeadline(ctx, limit)
	return work, cancel, in != nil, nil
}
