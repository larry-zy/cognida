package huggingface

import (
	"context"
	"encoding/json"
	"time"

	agentctx "cognida/internal/model/agent"
	d "cognida/internal/model/huggingface"
	l "cognida/internal/model/lifecycle"
)

func (s *Service) governedRepo() (d.GovernedRepository, error) {
	r, ok := s.repo.(d.GovernedRepository)
	if !ok {
		return nil, d.ErrUnavailable
	}
	return r, nil
}

// ValidateGovernedRequest performs current local authorization only. Remote
// metadata/file verification remains part of the single durable execution.
func (s *Service) ValidateGovernedRequest(ctx context.Context, p l.Principal, in d.Request) error {
	if e := s.auth(ctx, p, "read"); e != nil {
		return e
	}
	if in.Validate() != nil {
		return d.ErrInvalid
	}
	if s.decoder == nil {
		return d.ErrUnavailable
	}
	if _, e := s.governedRepo(); e != nil {
		return e
	}
	_, e := s.credential(ctx, p, in.CredentialID)
	return publicError(e)
}
func (s *Service) ValidateGovernedPrincipal(ctx context.Context, p l.Principal) error {
	return s.auth(ctx, p, "read")
}

// ValidateOutputs authorizes an already committed import without applying the
// zero-input dispatch contract or requiring a currently configured decoder.
// Resolve checks the stored asset and its origin; the explicit origin check
// also binds this API to an owned, successful HF receipt and current credential.
func (s *Service) ValidateOutputs(ctx context.Context, p l.Principal, refs []l.RefID) error {
	if e := s.auth(ctx, p, "read"); e != nil {
		return e
	}
	if len(refs) != 1 || refs[0].Validate() != nil {
		return d.ErrInvalid
	}
	if s.assets == nil || s.repo == nil {
		return d.ErrUnavailable
	}
	ref, _, e := s.assets.Resolve(ctx, p.TenantID, p.UserID, refs[0])
	if e != nil {
		return publicError(e)
	}
	if ref.RefID != refs[0] || ref.TenantID != p.TenantID || ref.Kind != l.RawAsset {
		return d.ErrForbidden
	}
	return publicError(s.ValidateGovernedOrigin(ctx, p, ref))
}

func (s *Service) SubmitGoverned(ctx context.Context, p l.Principal, in d.GovernedInvocation) (*d.Import, error) {
	if in.Validate() != nil {
		return nil, d.ErrInvalid
	}
	if e := s.ValidateGovernedRequest(ctx, p, in.Request); e != nil {
		return nil, e
	}
	r, e := s.governedRepo()
	if e != nil {
		return nil, e
	}
	return r.ReserveGoverned(ctx, p, in, false)
}
func (s *Service) LookupGoverned(ctx context.Context, p l.Principal, in d.GovernedInvocation) (*d.Import, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	if in.Validate() != nil {
		return nil, d.ErrInvalid
	}
	r, e := s.governedRepo()
	if e != nil {
		return nil, e
	}
	out, e := r.LookupGoverned(ctx, p, in.OperationKey, in.PayloadHash)
	if e != nil || out == nil {
		return out, e
	}
	a, _ := json.Marshal(in)
	b, _ := json.Marshal(out.Governed)
	if out.Governed == nil || digest(a) != digest(b) {
		return nil, d.ErrConflict
	}
	return out, nil
}

// Cancellation needs authenticated ownership/binding, but not live permission
// to the revoked source or credentials. A late Submit sees this same tombstone.
func (s *Service) CancelGoverned(ctx context.Context, p l.Principal, in d.GovernedInvocation) (*d.Import, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	if in.Validate() != nil {
		return nil, d.ErrInvalid
	}
	r, e := s.governedRepo()
	if e != nil {
		return nil, e
	}
	return r.ReserveGoverned(ctx, p, in, true)
}
func (s *Service) GovernedOutputRefs(ctx context.Context, p l.Principal, in d.GovernedInvocation, value *d.Import) ([]l.RefID, error) {
	// Reading/cancelling a committed receipt does not require the decoder to
	// remain configured. Only current authorization and the actual origin matter.
	if e := s.auth(ctx, p, "read"); e != nil {
		return nil, e
	}
	if _, e := s.credential(ctx, p, in.Request.CredentialID); e != nil {
		return nil, publicError(e)
	}
	if value == nil || value.Governed == nil {
		return nil, d.ErrConflict
	}
	refs := []l.RefID{}
	if value.Status != "succeeded" {
		return refs, nil
	}
	if len(value.OutputRefs) != 1 {
		return nil, d.ErrConflict
	}
	for _, id := range value.OutputRefs {
		ref, _, e := s.assets.Resolve(ctx, p.TenantID, p.UserID, id)
		if e != nil {
			return nil, publicError(e)
		}
		if ref.Kind != l.RawAsset || ref.ProducerID != "huggingface-import:"+value.ID {
			return nil, d.ErrConflict
		}
		refs = append(refs, id)
	}
	return refs, nil
}

// ProcessGoverned claims once before any external reads. Repeated delivery or
// process restart does not reclaim running work. Its unknown outcome is exposed
// until the stored database deadline, then permanently timed out without replay.
func (s *Service) ProcessGoverned(ctx context.Context, p l.Principal, id string) error {
	r, e := s.governedRepo()
	if e != nil {
		return e
	}
	receipt, claimed, e := r.ClaimGoverned(ctx, p, id)
	if e != nil || !claimed {
		return e
	}
	work, cancel := context.WithDeadline(ctx, receipt.Deadline)
	defer cancel()
	work = agentctx.WithRequestID(work, receipt.Governed.RequestID)
	monitor, stop := context.WithCancel(work)
	done := make(chan struct{})
	monitorError := make(chan error, 1)
	go func() {
		defer close(done)
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-monitor.Done():
				return
			case <-ticker.C:
				check, finish := context.WithTimeout(monitor, 2*time.Second)
				current, err := r.Get(check, p, id)
				finish()
				if err != nil {
					if monitor.Err() == nil {
						monitorError <- d.ErrUnavailable
						cancel()
					}
					return
				}
				if current.Status != "running" {
					cancel()
					return
				}
			}
		}
	}()
	result := d.Preview{}
	var refs []l.RefID
	e = s.ValidateGovernedRequest(work, p, receipt.Request)
	if e == nil {
		e = work.Err()
	}
	if e == nil {
		result, e = s.acquire(work, p, receipt.Request)
	}
	if e == nil {
		var payload []byte
		payload, e = json.Marshal(struct {
			l.Input
			Manifest d.Manifest `json:"huggingface_manifest"`
		}{l.Input{Kind: "table", Columns: result.Columns, ColumnTypes: result.ColumnTypes, Rows: result.Rows}, result.Manifest})
		if e == nil {
			e = work.Err()
		}
		if e == nil {
			e = s.ValidateGovernedRequest(work, p, receipt.Request)
		}
		if e == nil {
			var ref l.DataAssetRef
			ref, e = s.assets.ImportDerived(work, p, l.RawAsset, "huggingface-import:"+receipt.ID, nil, payload, l.PolicySnapshot{Sensitivity: "unknown", Scopes: []string{"read"}, Version: "1"})
			if e == nil {
				refs = []l.RefID{ref.RefID}
			}
		}
	}
	if e == nil {
		e = s.ValidateGovernedRequest(work, p, receipt.Request)
	}
	stop()
	<-done
	select {
	case failure := <-monitorError:
		e = failure
	default:
	}
	if e == nil {
		e = work.Err()
	}
	errorCode := code(e)
	if ctx.Err() != nil {
		errorCode = "execution_interrupted"
	}
	var manifest *d.Manifest
	if result.Manifest.RepoID != "" {
		manifest = &result.Manifest
	}
	finish, closeFinish := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer closeFinish()
	_, e = r.FinishGoverned(finish, p, id, receipt.ClaimToken, manifest, refs, errorCode)
	return e
}

// Tick is a privileged worker entry point, never exposed by HTTP. The scanner
// includes already running rows solely so their real deadlines can expire.
func (s *Service) Tick(ctx context.Context) error {
	r, e := s.governedRepo()
	if e != nil {
		return e
	}
	rows, e := r.PendingGoverned(ctx, 50)
	if e != nil {
		return e
	}
	for _, row := range rows {
		if e = ctx.Err(); e != nil {
			return e
		}
		if e = s.ProcessGoverned(ctx, l.Principal{TenantID: row.TenantID, UserID: row.UserID}, row.ID); e != nil {
			return e
		}
	}
	return nil
}
