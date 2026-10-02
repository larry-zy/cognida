package governance

import (
	"cognida/internal/infrastructure/job"
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	lifecycle "cognida/internal/service/lifecycle"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type reportRun struct {
	d.SubscriptionRun
	Snapshot      d.Subscription `json:"subscription_snapshot"`
	ResolvedRefID string         `json:"resolved_ref_id,omitempty"`
}

func (s *Service) SaveSubscription(ctx context.Context, p l.Principal, id, key string, in d.Subscription, expected int) (*d.Record, error) {
	if err := s.authorize(ctx, p, "admin"); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 128 || len(in.Input.Prompt) > 4000 || in.Channel != "in_app" {
		return nil, d.ErrInvalid
	}
	if in.Kind != "report" && in.Kind != "metric_watch" {
		return nil, d.ErrInvalid
	}
	if in.Kind == "metric_watch" && (in.Watch == nil || in.Watch.Method != "zscore" || in.Watch.Threshold <= 0 || in.Watch.Threshold > 20 || in.Input.ValueColumn == "") {
		return nil, d.ErrInvalid
	}
	if err := s.validateRefresh(ctx, p, in.Input); err != nil {
		return nil, err
	}
	_, input, err := s.input(ctx, p, in.Input.AssetRefID, true)
	if err != nil {
		return nil, err
	}
	if in.Kind == "metric_watch" {
		if _, _, _, err = watchColumns(input, in.Input, in.Watch); err != nil {
			return nil, err
		}
	}
	next, err := NextRun(in.Schedule, time.Now())
	if err != nil {
		return nil, err
	}
	in.NextRunAt = &next
	status := "enabled"
	if !in.Enabled {
		status = "disabled"
		in.NextRunAt = nil
	}
	in.Meta = d.Meta{}
	if id == "" {
		return s.create(ctx, p, "subscription", "", status, key, in, in.NextRunAt)
	}
	record, err := s.Get(ctx, p, "subscription", id)
	if err != nil {
		return nil, err
	}
	record.DueAt = in.NextRunAt
	if err = s.update(ctx, record, in, status, expected); err != nil {
		return nil, err
	}
	return record, nil
}
func (s *Service) RunSubscription(ctx context.Context, p l.Principal, id, key string) (*d.Record, error) {
	if strings.TrimSpace(key) == "" || len(key) > 55 {
		return nil, d.ErrInvalid
	}
	record, err := s.Get(ctx, p, "subscription", id)
	if err != nil {
		return nil, err
	}
	return s.submitReport(ctx, record, time.Now().UTC(), "manual:"+key)
}
func (s *Service) submitReport(ctx context.Context, sub *d.Record, at time.Time, key string) (*d.Record, error) {
	var snapshot d.Subscription
	if err := json.Unmarshal(sub.Payload, &snapshot); err != nil {
		return nil, err
	}
	snapshot.Meta = sub.Meta
	run := reportRun{SubscriptionRun: d.SubscriptionRun{SubscriptionID: sub.ID, Status: "queued", ScheduledAt: at}, Snapshot: snapshot}
	record, err := s.create(ctx, l.Principal{TenantID: sub.TenantID, UserID: sub.UserID}, "subscription_run", sub.ID, "queued", sub.ID+":"+key, run, nil)
	if err != nil {
		return nil, err
	}
	if record.Status == "queued" || record.Status == "running" {
		j, e := job.New(record.ID, "governance", jobScope{TenantID: record.TenantID, UserID: record.UserID, Kind: record.Kind, ID: record.ID})
		if e != nil {
			return nil, e
		}
		if e = s.Queue.Enqueue(ctx, j); e != nil {
			return nil, e
		}
	}
	return record, nil
}
func (s *Service) Tick(ctx context.Context) error {
	rows, err := s.Repo.Due(ctx, time.Now().UTC(), 100)
	if err != nil {
		return err
	}
	var failures []error
	for _, record := range rows {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if record.DueAt == nil {
			continue
		}
		_, e := s.submitReport(ctx, &record, *record.DueAt, record.DueAt.UTC().Format(time.RFC3339Nano))
		if e != nil {
			failures = append(failures, e)
			continue
		}
		var sub d.Subscription
		if e = json.Unmarshal(record.Payload, &sub); e != nil {
			failures = append(failures, e)
			continue
		}
		next, e := NextRun(sub.Schedule, time.Now())
		if e != nil {
			failures = append(failures, e)
			continue
		}
		record.DueAt = &next
		sub.NextRunAt = &next
		if e = s.update(ctx, &record, sub, "enabled", record.Version); e != nil && !errors.Is(e, d.ErrConflict) {
			failures = append(failures, e)
		}
	}
	return errors.Join(failures...)
}
func (s *Service) runReport(ctx context.Context, record *d.Record) error {
	var run reportRun
	if err := json.Unmarshal(record.Payload, &run); err != nil {
		return err
	}
	p := l.Principal{TenantID: record.TenantID, UserID: record.UserID}
	if run.ResolvedRefID == "" {
		id, err := s.resolveSubscriptionInput(ctx, p, run.Snapshot.Input)
		if err != nil {
			return err
		}
		run.ResolvedRefID = id
		run.Status = "running"
		// Persist the chosen source before any output side effects. Retries never
		// switch to a newer publication after creating an immutable insight.
		if err = s.update(ctx, record, run, "running", record.Version); err != nil {
			return err
		}
	}
	ref, input, err := s.input(ctx, p, run.ResolvedRefID, true)
	if err != nil {
		return err
	}
	analysis, err := lifecycle.AnalyzeInput(input)
	if err != nil {
		return err
	}
	analysis["prompt"] = run.Snapshot.Input.Prompt
	analysis["source_ref_id"] = ref.AssetRefID
	alert := false
	if run.Snapshot.Kind == "metric_watch" {
		evidence, trigger, e := WatchMetric(input, run.Snapshot.Input, run.Snapshot.Watch)
		if e != nil {
			return e
		}
		analysis["metric_watch"] = evidence
		alert = trigger
	}
	analysis["decision"] = map[string]any{"evidence_refs": []string{ref.AssetRefID}, "method": "deterministic_statistics", "assumptions": []string{"Source snapshot represents the intended observation period"}, "risks": []string{"Sampling and source quality may limit conclusions"}, "verification_status": "computed", "confidence": nil}
	raw, err := json.Marshal(analysis)
	if err != nil {
		return err
	}
	result, err := s.Assets.ImportDerived(ctx, p, l.Insight, record.ID, []l.RefID{ref.RefID}, raw, ref.Policy)
	if err != nil {
		return err
	}
	run.ResultRefID = result.AssetRefID
	run.AlertTriggered = alert
	run.Status = "completed"
	run.Error = ""
	// Notification is idempotent and written before completion; a restart can
	// replay this section without losing or duplicating a delivered result.
	if run.Snapshot.Kind == "report" || alert {
		title := "报告已生成：" + run.Snapshot.Name
		if alert {
			title = "指标告警：" + run.Snapshot.Name
		}
		_, err = s.create(ctx, p, "notification", record.ID, "unread", record.ID, d.Notification{Title: title, Body: "打开运行记录查看来源、统计及验证状态。", RunID: record.ID, ResultRefID: result.AssetRefID}, nil)
		if err != nil {
			return err
		}
	}
	return s.update(ctx, record, run, "completed", record.Version)
}
func (s *Service) ReadNotification(ctx context.Context, p l.Principal, id string) (*d.Record, error) {
	record, err := s.Get(ctx, p, "notification", id)
	if err != nil {
		return nil, err
	}
	if record.Status == "read" {
		return record, nil
	}
	var n d.Notification
	if err = json.Unmarshal(record.Payload, &n); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	n.ReadAt = &now
	if err = s.update(ctx, record, n, "read", record.Version); err != nil {
		return nil, err
	}
	return record, nil
}
