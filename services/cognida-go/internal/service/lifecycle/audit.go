package lifecycle

import (
	"context"
	"encoding/json"
	"errors"

	obs "cognida/internal/infrastructure/observability"
	agentctx "cognida/internal/model/agent"
	"cognida/internal/model/job"
	d "cognida/internal/model/lifecycle"
)

type AuditFunc func(context.Context, d.Principal, string, string, map[string]any)

func (s *Service) audit(ctx context.Context, p d.Principal, action, resource string, details map[string]any, err error) {
	if s.config.Audit == nil {
		return
	}
	details["request_id"] = agentctx.MustGetRequestID(ctx)
	if _, ok := details["outcome"]; !ok {
		details["outcome"] = "success"
	}
	if err != nil {
		details["outcome"] = "failed"
		switch {
		case errors.Is(err, d.ErrForbidden):
			details["error_class"] = "forbidden"
		case errors.Is(err, d.ErrNotFound):
			details["error_class"] = "not_found"
		case errors.Is(err, d.ErrExpired):
			details["error_class"] = "expired"
		case errors.Is(err, d.ErrInvalid):
			details["error_class"] = "invalid"
		default:
			details["error_class"] = "operation_failed"
		}
	}
	s.config.Audit(ctx, p, action, resource, details)
}
func (s *Service) Resolve(ctx context.Context, tenant, user int64, id d.RefID) (d.DataAssetRef, json.RawMessage, error) {
	ref, body, err := s.resolve(ctx, tenant, user, id)
	s.audit(ctx, d.Principal{TenantID: tenant, UserID: user}, "lifecycle.asset.resolve", id.String(), map[string]any{}, err)
	return ref, body, err
}
func (s *Service) Acquire(ctx context.Context, p d.Principal, source SourceRequest, name, key string) (d.DataAssetRef, error) {
	ref, err := s.acquire(ctx, p, source, name, key)
	s.audit(ctx, p, "lifecycle.asset.acquire", ref.AssetRefID, map[string]any{"source_type": source.Kind}, err)
	return ref, err
}
func (s *Service) ResolveApproval(ctx context.Context, p d.Principal, id, decision, reason string) error {
	err := s.resolveApproval(ctx, p, id, decision, reason)
	s.audit(ctx, p, "lifecycle.approval.resolve", id, map[string]any{"decision": decision}, err)
	return err
}
func (s *Service) Handle(ctx context.Context, j job.Job) error {
	var msg delivery
	if err := json.Unmarshal(j.Payload, &msg); err != nil {
		return err
	}
	st, err := s.repo.FindStage(ctx, msg.TenantID, msg.StageID, false)
	if err != nil {
		return err
	}
	ctx = agentctx.WithRequestID(ctx, st.RequestID)
	ctx = agentctx.WithTenantID(ctx, st.TenantID)
	ctx = agentctx.WithUserID(ctx, st.UserID)
	ctx, span := obs.NewTracer().StartOrchestrationSpan(ctx, "lifecycle.stage")
	defer span.End()
	span.SetAttribute("pipeline.run_id", st.RunID)
	span.SetAttribute("pipeline.stage_id", st.ID)
	span.SetAttribute("agent.name", st.AgentID)
	span.SetAttribute("job.id", j.ID)
	span.SetAttribute("request_id", st.RequestID)
	span.SetAttribute("tenant.id", st.TenantID)
	err = s.handle(ctx, j)
	details := map[string]any{"run_id": st.RunID, "stage_id": st.ID, "agent_id": st.AgentID, "job_id": j.ID, "input_refs": st.InputRefs}
	if latest, e := s.repo.FindStage(ctx, st.TenantID, st.ID, false); e == nil {
		details["status"] = latest.Status
		details["outcome"] = latest.Status
		appendAuditOutput(details, latest.Output)
		span.SetAttribute("pipeline.stage_status", latest.Status)
	}
	failed := err != nil || details["status"] == d.StatusFailed
	span.SetAttribute("pipeline.failed", failed)
	if failed {
		span.RecordError(errors.New("lifecycle stage execution failed"))
	}
	action := "lifecycle.stage.execute"
	if st.AgentID == "data-asset" {
		action = "lifecycle.asset.publish"
	}
	s.audit(ctx, d.Principal{TenantID: st.TenantID, UserID: st.UserID}, action, st.ID, details, err)
	return err
}

// Approval, cancellation and failed attempts can legitimately have no output.
// Keep the absence explicit instead of inventing a successful gate.
func appendAuditOutput(details map[string]any, output *d.Output) {
	refs := []string{}
	if output != nil {
		details["gate"] = output.Gate
		for _, ref := range output.Refs {
			refs = append(refs, ref.RefID.String())
		}
	}
	details["output_refs"] = refs
}
