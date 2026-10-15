package governance

import (
	"cognida/internal/infrastructure/job"
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	lifecycle "cognida/internal/service/lifecycle"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
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
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 128 || utf8.RuneCountInString(in.Input.Prompt) > 4000 || in.Channel != "in_app" {
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
	in.ScheduleError = ""
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
	record, err := s.createReport(ctx, sub, at, key)
	if err != nil {
		return nil, err
	}
	return record, s.enqueueReport(ctx, record)
}
func (s *Service) createReport(ctx context.Context, sub *d.Record, at time.Time, key string) (*d.Record, error) {
	var snapshot d.Subscription
	if err := json.Unmarshal(sub.Payload, &snapshot); err != nil {
		return nil, err
	}
	snapshot.Meta = sub.Meta
	run := reportRun{SubscriptionRun: d.SubscriptionRun{SubscriptionID: sub.ID, Status: "queued", ScheduledAt: at}, Snapshot: snapshot}
	return s.create(ctx, l.Principal{TenantID: sub.TenantID, UserID: sub.UserID}, "subscription_run", sub.ID, "queued", sub.ID+":"+key, run, nil)
}
func (s *Service) enqueueReport(ctx context.Context, record *d.Record) error {
	if record.Status == "queued" || record.Status == "running" {
		j, e := job.New(record.ID, "governance", jobScope{TenantID: record.TenantID, UserID: record.UserID, Kind: record.Kind, ID: record.ID})
		if e != nil {
			return e
		}
		if e = s.Queue.Enqueue(ctx, j); e != nil {
			return e
		}
	}
	return nil
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
		var sub d.Subscription
		if e := json.Unmarshal(record.Payload, &sub); e != nil {
			failures = append(failures, s.suspendSubscription(ctx, &record, "invalid_schedule_configuration"))
			continue
		}
		next, e := NextRun(sub.Schedule, time.Now())
		if e != nil {
			failures = append(failures, s.suspendSubscription(ctx, &record, "invalid_schedule_configuration"))
			continue
		}
		var run *d.Record
		// A run and advancement of its schedule form one durable claim. A second
		// scheduler's stale CAS rolls back its run, and Redis failure cannot keep
		// the oldest schedules blocking all subsequent subscriptions.
		e = s.Repo.Transaction(ctx, func(repo d.Repository) error {
			tx := *s
			tx.Repo = repo
			var err error
			run, err = tx.createReport(ctx, &record, *record.DueAt, record.DueAt.UTC().Format(time.RFC3339Nano))
			if err != nil {
				return err
			}
			record.DueAt = &next
			sub.NextRunAt = &next
			sub.ScheduleError = ""
			return tx.update(ctx, &record, sub, "enabled", record.Version)
		})
		if errors.Is(e, d.ErrConflict) {
			continue
		}
		if errors.Is(e, d.ErrForbidden) {
			failures = append(failures, s.suspendSubscription(ctx, &record, "owner_access_unavailable"))
			continue
		}
		if e == nil {
			e = s.enqueueReport(ctx, run)
		}
		if e != nil {
			failures = append(failures, e)
		}
	}
	return errors.Join(failures...)
}

// Suspension is a scheduler-owned status transition, not an impersonated user
// mutation. An administrator must explicitly save a valid enabled subscription
// after fixing configuration or restoring the owner's access.
func (s *Service) suspendSubscription(ctx context.Context, record *d.Record, code string) error {
	var payload map[string]json.RawMessage
	if json.Unmarshal(record.Payload, &payload) != nil || payload == nil {
		payload = map[string]json.RawMessage{}
	}
	payload["enabled"] = json.RawMessage("false")
	payload["next_run_at"] = json.RawMessage("null")
	payload["schedule_error"], _ = json.Marshal(code)
	record.DueAt = nil
	if err := s.update(ctx, record, payload, "disabled", record.Version); err != nil {
		if errors.Is(err, d.ErrConflict) {
			return nil
		}
		return err
	}
	return fmt.Errorf("subscription %s suspended: %s", record.ID, code)
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
	var watchEvidence any
	alert := false
	if run.Snapshot.Kind == "metric_watch" {
		evidence, trigger, e := WatchMetric(input, run.Snapshot.Input, run.Snapshot.Watch)
		if e != nil {
			return e
		}
		watchEvidence = evidence
		alert = trigger
	}
	result, _, err := s.Assets.Resolve(ctx, p.TenantID, p.UserID, lifecycle.DerivedRefID(p.TenantID, l.Insight, record.ID, []l.RefID{ref.RefID}))
	if err != nil && !errors.Is(err, l.ErrNotFound) {
		return err
	}
	if errors.Is(err, l.ErrNotFound) {
		analysis, err := lifecycle.AnalyzeGoal(ctx, s.Compute, input, run.Snapshot.Input.Prompt, ref.AssetRefID, "subscription:"+record.ID)
		if err != nil {
			return err
		}
		analysis["prompt"] = run.Snapshot.Input.Prompt
		analysis["source_ref_id"] = ref.AssetRefID
		if watchEvidence != nil {
			analysis["metric_watch"] = watchEvidence
		}
		analysis["decision"] = map[string]any{"evidence_refs": []string{ref.AssetRefID}, "method": "deterministic_statistics", "assumptions": []string{"Source snapshot represents the intended observation period"}, "risks": []string{"Sampling and source quality may limit conclusions"}, "verification_status": "computed", "confidence": nil}
		if strings.TrimSpace(run.Snapshot.Input.Prompt) != "" {
			analysis["decision"].(map[string]any)["method"] = analysis["method"]
			analysis["decision"].(map[string]any)["verification_status"] = "citations_checked_interpretation_unverified"
		}
		raw, err := json.Marshal(analysis)
		if err != nil {
			return err
		}
		result, err = s.Assets.ImportDerived(ctx, p, l.Insight, record.ID, []l.RefID{ref.RefID}, raw, ref.Policy)
		if err != nil {
			return err
		}

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
		if errors.Is(err, d.ErrConflict) {
			current, reloadErr := s.Get(ctx, p, "notification", id)
			if reloadErr != nil {
				return nil, reloadErr
			}
			if current.Status == "read" {
				return current, nil
			}
		}
		return nil, err
	}
	return record, nil
}
