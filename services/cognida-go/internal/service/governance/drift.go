package governance

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type DriftRemediation func(context.Context, l.Principal, string, string, string) (string, error)

func (s *Service) SetDriftRemediation(fn DriftRemediation) { s.driftRemediation = fn }
func (s *Service) driftRepo() (d.DriftRepository, error) {
	r, ok := s.Repo.(d.DriftRepository)
	if !ok {
		return nil, fmt.Errorf("drift repository unavailable")
	}
	return r, nil
}
func (s *Service) driftInput(ctx context.Context, p l.Principal, id string) (l.DataAssetRef, l.Input, error) {
	ref, in, e := s.input(ctx, p, id, false)
	if e != nil {
		return ref, in, e
	}
	if in.Kind != "table" || in.Validate() != nil || len(ref.Provenance.Digest) != 64 {
		return ref, in, d.ErrInvalid
	}
	return ref, in, nil
}
func (s *Service) CreateDriftBaseline(ctx context.Context, p l.Principal, in d.DriftBaseline, key string) (*d.DriftRecord, error) {
	if s.authorize(ctx, p, "read") != nil {
		return nil, d.ErrForbidden
	}
	if !validFeatureKey(key) || in.Name == "" || len(in.Name) > 128 {
		return nil, d.ErrInvalid
	}
	sampling, e := normalizeDriftSampling(in.Sampling)
	if e != nil {
		return nil, e
	}
	ref, _, e := s.driftInput(ctx, p, in.SourceRefID)
	if e != nil {
		return nil, e
	}
	in = d.DriftBaseline{Name: in.Name, SourceRefID: in.SourceRefID, SourceDigest: ref.Provenance.Digest, Sampling: sampling}
	body, _ := json.Marshal(in)
	r, e := s.driftRepo()
	if e != nil {
		return nil, e
	}
	return r.CreateDrift(ctx, &d.DriftRecord{TenantID: p.TenantID, UserID: p.UserID, Kind: "baseline", Status: "pending_approval", Body: body}, key)
}
func (s *Service) GetDrift(ctx context.Context, p l.Principal, id string) (*d.DriftRecord, error) {
	if s.authorize(ctx, p, "read") != nil {
		return nil, d.ErrForbidden
	}
	r, e := s.driftRepo()
	if e != nil {
		return nil, e
	}
	v, e := r.GetDrift(ctx, p.TenantID, p.UserID, id)
	if e != nil {
		return nil, e
	}
	check := func(id, digest string) error {
		ref, _, err := s.driftInput(ctx, p, id)
		if err != nil {
			return err
		}
		if digest != "" && digest != ref.Provenance.Digest {
			return d.ErrConflict
		}
		return nil
	}
	switch v.Kind {
	case "baseline":
		var b d.DriftBaseline
		if json.Unmarshal(v.Body, &b) != nil {
			return nil, d.ErrInvalid
		}
		e = check(b.SourceRefID, b.SourceDigest)
	case "report":
		var b d.DriftReport
		if json.Unmarshal(v.Body, &b) != nil {
			return nil, d.ErrInvalid
		}
		e = check(b.BaselineRefID, b.BaselineDigest)
		if e == nil {
			e = check(b.CurrentRefID, b.CurrentDigest)
		}
	case "monitor":
		var m d.DriftMonitor
		if json.Unmarshal(v.Body, &m) != nil {
			return nil, d.ErrInvalid
		}
		_, e = s.GetDrift(ctx, p, m.BaselineID)
		if e == nil {
			e = check(m.CurrentRefID, "")
		}
	default:
		return nil, d.ErrInvalid
	}
	if e != nil {
		return nil, e
	}
	return v, nil
}
func (s *Service) ListDrift(ctx context.Context, p l.Principal, kind, cursor string) ([]d.DriftRecord, string, error) {
	if kind != "baseline" && kind != "report" && kind != "monitor" {
		return nil, "", d.ErrInvalid
	}
	if s.authorize(ctx, p, "read") != nil {
		return nil, "", d.ErrForbidden
	}
	r, e := s.driftRepo()
	if e != nil {
		return nil, "", e
	}
	rows, next, e := r.ListDrift(ctx, p.TenantID, p.UserID, kind, cursor)
	if e != nil {
		return nil, "", e
	}
	out := []d.DriftRecord{}
	for _, row := range rows {
		if _, e = s.GetDrift(ctx, p, row.ID); e == nil {
			out = append(out, row)
		}
	}
	return out, next, nil
}
func (s *Service) ApproveDriftBaseline(ctx context.Context, p l.Principal, id string, version int) (*d.DriftRecord, error) {
	if s.authorize(ctx, p, "admin") != nil {
		return nil, d.ErrForbidden
	}
	v, e := s.GetDrift(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if v.Kind != "baseline" || v.Status != "pending_approval" {
		return nil, d.ErrConflict
	}
	var b d.DriftBaseline
	_ = json.Unmarshal(v.Body, &b)
	now := time.Now().UTC()
	b.ApprovedBy = p.UserID
	b.ApprovedAt = &now
	v.Body, _ = json.Marshal(b)
	v.Status = "approved"
	r, _ := s.driftRepo()
	if e = r.UpdateDrift(ctx, v, version, "baseline_approved"); e != nil {
		return nil, e
	}
	return v, nil
}
func (s *Service) CompareDrift(ctx context.Context, p l.Principal, baselineID, currentID string, threshold float64) (*d.DriftRecord, error) {
	baseline, e := s.GetDrift(ctx, p, baselineID)
	if e != nil {
		return nil, e
	}
	if baseline.Kind != "baseline" || baseline.Status != "approved" {
		return nil, d.ErrConflict
	}
	var b d.DriftBaseline
	_ = json.Unmarshal(baseline.Body, &b)
	a, ai, e := s.driftInput(ctx, p, b.SourceRefID)
	if e != nil {
		return nil, e
	}
	c, ci, e := s.driftInput(ctx, p, currentID)
	if e != nil {
		return nil, e
	}
	if threshold == 0 {
		threshold = 0.2
	}
	metrics, sa, sb, changed, e := compareDrift(ai, ci, b.Sampling, threshold)
	if e != nil {
		return nil, e
	}
	report := d.DriftReport{BaselineID: baseline.ID, BaselineVersion: baseline.Version, BaselineRefID: b.SourceRefID, CurrentRefID: currentID, BaselineDigest: a.Provenance.Digest, CurrentDigest: c.Provenance.Digest, Sampling: b.Sampling, BaselineRows: len(ai.Rows), CurrentRows: len(ci.Rows), BaselineSample: sa, CurrentSample: sb, Metrics: metrics, Changed: changed, Limitations: []string{"分布变化不等于质量下降，也不证明模型性能变化。", "效应量是经验分布距离，不是正确概率；未报告显著性 p 值。", "样本由固定种子对真实行号做 SHA-256 排序选取；行顺序属于固定快照。"}}
	body, _ := json.Marshal(report)
	key := featureHash(body)
	status := "stable"
	if changed {
		status = "changed"
	} else {
		for _, m := range metrics {
			if m.Status == "insufficient" {
				status = "insufficient"
				break
			}
		}
	}
	r, _ := s.driftRepo()
	return r.CreateDrift(ctx, &d.DriftRecord{TenantID: p.TenantID, UserID: p.UserID, Kind: "report", Status: status, Body: body}, key)
}
func (s *Service) CreateDriftMonitor(ctx context.Context, p l.Principal, in d.DriftMonitor, key string) (*d.DriftRecord, error) {
	if !validFeatureKey(key) || in.Name == "" || len(in.Name) > 128 || in.IntervalSeconds < 60 || in.IntervalSeconds > 86400 || in.Threshold <= 0 || in.Threshold > 1 {
		return nil, d.ErrInvalid
	}
	b, e := s.GetDrift(ctx, p, in.BaselineID)
	if e != nil {
		return nil, e
	}
	if b.Kind != "baseline" || b.Status != "approved" {
		return nil, d.ErrConflict
	}
	if _, _, e = s.driftInput(ctx, p, in.CurrentRefID); e != nil {
		return nil, e
	}
	if e = s.validateRefresh(ctx, p, d.AnalysisInput{AssetRefID: in.CurrentRefID, Refresh: in.Refresh}); e != nil {
		return nil, e
	}
	in.PendingBaselineID = ""
	in.LastError = ""
	in.LastReportID = ""
	body, _ := json.Marshal(in)
	now := time.Now().UTC()
	r, _ := s.driftRepo()
	return r.CreateDrift(ctx, &d.DriftRecord{TenantID: p.TenantID, UserID: p.UserID, Kind: "monitor", Status: "active", Body: body, NextRunAt: &now}, key)
}
func (s *Service) UpdateDriftMonitor(ctx context.Context, p l.Principal, id, action, baselineID string, version int) (*d.DriftRecord, error) {
	v, e := s.GetDrift(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if v.Kind != "monitor" {
		return nil, d.ErrInvalid
	}
	var m d.DriftMonitor
	_ = json.Unmarshal(v.Body, &m)
	switch action {
	case "pause":
		v.Status = "paused"
		v.NextRunAt = nil
	case "resume":
		v.Status = "active"
		now := time.Now().UTC()
		v.NextRunAt = &now
	case "propose_baseline":
		b, e := s.GetDrift(ctx, p, baselineID)
		if e != nil {
			return nil, e
		}
		if b.Kind != "baseline" || b.Status != "approved" || b.ID == m.BaselineID {
			return nil, d.ErrInvalid
		}
		m.PendingBaselineID = baselineID
	case "approve_baseline":
		if s.authorize(ctx, p, "admin") != nil {
			return nil, d.ErrForbidden
		}
		if m.PendingBaselineID == "" || baselineID != m.PendingBaselineID {
			return nil, d.ErrConflict
		}
		b, e := s.GetDrift(ctx, p, baselineID)
		if e != nil {
			return nil, e
		}
		if b.Kind != "baseline" || b.Status != "approved" {
			return nil, d.ErrConflict
		}
		m.BaselineID = baselineID
		m.PendingBaselineID = ""
	default:
		return nil, d.ErrInvalid
	}
	v.Body, _ = json.Marshal(m)
	r, _ := s.driftRepo()
	if e = r.UpdateDrift(ctx, v, version, action); e != nil {
		return nil, e
	}
	return v, nil
}
func (s *Service) RunDriftMonitor(ctx context.Context, v d.DriftRecord) error {
	p := l.Principal{TenantID: v.TenantID, UserID: v.UserID}
	r, e := s.driftRepo()
	if e != nil {
		return e
	}
	current, e := s.GetDrift(ctx, p, v.ID)
	if e != nil {
		v.Status = "blocked"
		v.NextRunAt = nil
		return r.UpdateDrift(ctx, &v, v.Version, "source_access_blocked")
	}
	if current.Version != v.Version || current.Status != "active" {
		return d.ErrConflict
	}
	var m d.DriftMonitor
	_ = json.Unmarshal(current.Body, &m)
	source, err := s.resolveSubscriptionInput(ctx, p, d.AnalysisInput{AssetRefID: m.CurrentRefID, Refresh: m.Refresh})
	if err == nil {
		var report *d.DriftRecord
		report, err = s.CompareDrift(ctx, p, m.BaselineID, source, m.Threshold)
		if err == nil {
			m.LastReportID = report.ID
		}
	}
	m.LastError = ""
	if err != nil {
		m.LastError = err.Error()
	}
	current.Body, _ = json.Marshal(m)
	next := time.Now().UTC().Add(time.Duration(m.IntervalSeconds) * time.Second)
	current.NextRunAt = &next
	return r.UpdateDrift(ctx, current, v.Version, "comparison_finished")
}
func (s *Service) DriftAudit(ctx context.Context, p l.Principal, id string) ([]d.DriftEvent, error) {
	if _, e := s.GetDrift(ctx, p, id); e != nil {
		return nil, e
	}
	r, _ := s.driftRepo()
	return r.DriftEvents(ctx, p.TenantID, p.UserID, id)
}
func (s *Service) CreateDriftRemediation(ctx context.Context, p l.Principal, id string, version int) (*d.DriftRecord, error) {
	if s.authorize(ctx, p, "admin") != nil {
		return nil, d.ErrForbidden
	}
	v, e := s.GetDrift(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if v.Kind != "report" || v.Status != "changed" || s.driftRemediation == nil {
		return nil, d.ErrConflict
	}
	var report d.DriftReport
	_ = json.Unmarshal(v.Body, &report)
	if report.TaskID != "" {
		return v, nil
	}
	if v.Version != version {
		return nil, d.ErrConflict
	}
	task, e := s.driftRemediation(ctx, p, id, report.CurrentRefID, "drift:"+id)
	if e != nil {
		return nil, e
	}
	if task == "" {
		return nil, d.ErrConflict
	}
	report.TaskID = task
	v.Body, _ = json.Marshal(report)
	r, _ := s.driftRepo()
	if e = r.UpdateDrift(ctx, v, version, "remediation_draft_created"); e != nil {
		return nil, e
	}
	return v, nil
}
