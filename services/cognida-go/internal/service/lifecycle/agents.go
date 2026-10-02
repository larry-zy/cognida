package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	a "cognida/internal/model/agent"
	d "cognida/internal/model/lifecycle"
	f "cognida/internal/service/agent/framework"
)

// RegisterAgents keeps specialist payloads out of prompts: agents accept only
// reference envelopes and return a durable invocation/run handle. Their sole
// execution path is the controller, including authorization and budgets.
func (s *Service) RegisterAgents(ctx context.Context, registry f.AgentInstanceRegistry, collaboration *f.CollaborationRegistry) error {
	for _, contract := range s.Agents() {
		contract := contract
		spec := f.AgentSpec{ID: "lifecycle-" + contract.ID, Name: contract.Name, Description: contract.Purpose, Type: a.AgentTypeAgenticRAG, ToolNames: contract.ToolAllowlist, Contract: &contract, Build: func(context.Context) (f.Agent, error) { return &typedAgent{service: s, id: contract.ID}, nil }}
		if e := registry.RegisterSpec(ctx, spec); e != nil {
			return e
		}
		if collaboration != nil {
			collaboration.RegisterGoverned(contract.ID, &typedAgent{service: s, id: contract.ID}, contract.Purpose, &f.AgentGovernance{Purpose: contract.Purpose, DataScope: "authorized immutable asset references", Tools: contract.ToolAllowlist, RiskClass: contract.RiskClass, Contract: &contract}, a.ContextModeIsolated)
		}
	}
	tools := []string{"pipeline.templates", "pipeline.propose", "pipeline.start", "pipeline.inspect", "pipeline.cancel", "pipeline.approvals"}
	return registry.RegisterSpec(ctx, f.AgentSpec{ID: "pipeline-orchestrator", Name: "Pipeline Orchestrator", Description: "Select registered templates and manage durable pipeline runs", Type: a.AgentTypeAgenticRAG, ToolNames: tools, Build: func(context.Context) (f.Agent, error) {
		return &typedAgent{service: s, id: "pipeline-orchestrator"}, nil
	}})
}

type typedAgent struct {
	service *Service
	id      string
}

func (t *typedAgent) Name() string { return t.id }
func (t *typedAgent) Chat(ctx context.Context, message string) (*f.Response, error) {
	p := d.Principal{TenantID: a.MustGetTenantID(ctx), UserID: a.MustGetUserID(ctx)}
	if e := t.service.authorize(ctx, p, "read"); e != nil {
		return nil, e
	}
	if len(message) > 128<<10 {
		return nil, d.ErrInvalid
	}
	var req struct {
		Action         string         `json:"action"`
		InputRefs      []d.RefID      `json:"input_refs"`
		Options        map[string]any `json:"options"`
		IdempotencyKey string         `json:"idempotency_key"`
		TemplateID     string         `json:"template_id"`
		RunID          string         `json:"run_id"`
	}
	decoder := json.NewDecoder(strings.NewReader(message))
	decoder.DisallowUnknownFields()
	if e := decoder.Decode(&req); e != nil {
		return nil, fmt.Errorf("%w: expected a typed reference command", d.ErrInvalid)
	}
	var result any
	var err error
	if t.id != "pipeline-orchestrator" {
		result, err = t.service.Invoke(ctx, p, t.id, req.InputRefs, req.Options, req.IdempotencyKey, a.MustGetRequestID(ctx))
	} else {
		switch req.Action {
		case "templates":
			result = t.service.Templates()
		case "propose":
			definition, ok := t.service.templates[req.TemplateID]
			if !ok {
				return nil, d.ErrInvalid
			}
			if err = t.service.validateInputs(ctx, p, "data-access", req.InputRefs); err != nil {
				return nil, err
			}
			if err = validateOptions(req.Options); err != nil {
				return nil, err
			}
			result = map[string]any{"definition": definition, "input_refs": req.InputRefs, "requires_publication_approval": true}
		case "start":
			result, err = t.service.StartRun(ctx, p, req.TemplateID, req.InputRefs, req.Options, req.IdempotencyKey, a.MustGetRequestID(ctx))
		case "inspect":
			result, err = t.service.repo.FindRun(ctx, p.TenantID, req.RunID, false)
		case "cancel":
			err = t.service.CancelRun(ctx, p, req.RunID)
			result = map[string]string{"status": "cancelled"}
		case "approvals":
			if _, err = t.service.repo.FindRun(ctx, p.TenantID, req.RunID, false); err == nil {
				result, err = t.service.repo.ListApprovals(ctx, p.TenantID, req.RunID)
			}
		default:
			return nil, fmt.Errorf("%w: orchestrator command not allowed", d.ErrInvalid)
		}
	}
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return &f.Response{Content: string(body), Metadata: map[string]any{"agent_id": t.id, "request_id": a.MustGetRequestID(ctx), "typed_reference_output": true}}, nil
}
func (t *typedAgent) Stream(ctx context.Context, message string) (<-chan *f.Chunk, error) {
	response, e := t.Chat(ctx, message)
	if e != nil {
		return nil, e
	}
	out := make(chan *f.Chunk, 1)
	out <- &f.Chunk{Content: response.Content, Metadata: response.Metadata, Done: true}
	close(out)
	return out, nil
}
