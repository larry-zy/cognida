package taskcontrol

import (
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/taskcontrol"
	"context"
)

type RuntimeNodeSummary struct {
	NodeID     string    `json:"node_id"`
	Capability string    `json:"capability"`
	Status     string    `json:"status"`
	Attempt    int       `json:"attempt"`
	Error      string    `json:"error,omitempty"`
	OutputRefs []l.RefID `json:"output_refs"`
	DomainKind string    `json:"domain_kind,omitempty"`
	DomainID   string    `json:"domain_id,omitempty"`
}
type RuntimeUsage struct {
	ReservedAttempts   int   `json:"reserved_attempts"`
	ReservedCalls      int   `json:"reserved_calls"`
	ReservedCostMicros int64 `json:"reserved_cost_micros"`
}

func (r *Runtime) Enabled() bool { return r.enabled }

// DecorateSummary reads the controller's persisted domain observations. HTTP
// reads never poll a domain executor or trigger dispatch/reconciliation.
func (r *Runtime) DecorateSummary(ctx context.Context, p d.Scope, summary *SupervisorSummary) error {
	task, err := r.tasks.Get(ctx, p, summary.TaskID)
	if err != nil {
		return err
	}
	if task.Version != summary.TaskVersion {
		return d.ErrConflict
	}
	children, err := r.repo.ListChildren(ctx, p, task.ID)
	if err != nil {
		return err
	}
	summary.ExecutionEnabled = r.enabled
	summary.Terminal = task.Status.Terminal()
	summary.CanCancel = !summary.Terminal && task.CancelRequestedAt == nil
	summary.Nodes = []RuntimeNodeSummary{}
	summary.Children = []SupervisorChildSummary{}
	summary.Warnings = []string{"领域状态来自最近一次持久化核对；预算显示为已预留额度，不代表实际模型费用。"}
	// The legacy summary only resolves the artifact itself. Execution results
	// also require the complete currently supported source chain.
	summary.ResultRefs = []l.RefID{}
	for _, child := range children {
		kind := ""
		if child.Domain == "lifecycle" && child.DomainID != "" {
			kind = "invocation"
		}
		summary.Children = append(summary.Children, SupervisorChildSummary{ID: child.ID, NodeID: child.NodeID, PlanVersion: child.PlanVersion, Attempt: child.Attempt, Domain: child.Domain, DomainID: child.DomainID, DomainKind: kind, DispatchStatus: child.Status, DomainStatus: child.DomainStatus, ReceiptPresent: len(child.Receipt) > 0 && string(child.Receipt) != "null"})
		summary.Usage.ReservedAttempts++
		summary.Usage.ReservedCalls += child.CallsReserved
		summary.Usage.ReservedCostMicros += child.CostMicrosReserved
	}
	if task.CurrentPlanVersion > 0 {
		plan, err := r.plan(ctx, p, task)
		if err != nil {
			return err
		}
		latest := latestChildren(children, task.CurrentPlanVersion)
		for _, node := range plan.Nodes {
			item := RuntimeNodeSummary{NodeID: node.ID, Capability: node.Capability, Status: "pending", OutputRefs: []l.RefID{}}
			if child, ok := latest[node.ID]; ok {
				item.Attempt, item.Error, item.DomainID = child.Attempt, child.Error, child.DomainID
				item.Status = string(child.Status)
				if child.Status == d.ChildBound {
					item.Status = child.DomainStatus
				}
				if child.Domain == "lifecycle" && child.DomainID != "" {
					item.DomainKind = "invocation"
				}
				if len(child.OutputRefs) > 0 {
					refs, err := decodeStored[[]l.RefID](child.OutputRefs)
					if err != nil || len(refs) > 64 {
						return d.ErrInvalid
					}
					for _, ref := range refs {
						adapter := r.adapters[node.Capability]
						if adapter != nil && adapter.ValidateInputs(ctx, p, []l.RefID{ref}) == nil {
							if _, err := r.tasks.validator.resolve(ctx, p, ref); err != nil {
								continue
							}
							item.OutputRefs = append(item.OutputRefs, ref)
						}
					}
				}
			} else if summary.Terminal || task.CancelRequestedAt != nil {
				item.Status = "skipped"
			}
			summary.Nodes = append(summary.Nodes, item)
		}
		stored, err := decodeStored[[]l.RefID](task.ResultRefs)
		if err != nil || len(stored) > 64 {
			return d.ErrInvalid
		}
		visible := map[string]bool{}
		for _, node := range summary.Nodes {
			for _, ref := range node.OutputRefs {
				visible[ref.String()] = true
			}
		}
		for _, ref := range stored {
			if visible[ref.String()] {
				summary.ResultRefs = append(summary.ResultRefs, ref)
			}
		}
	}
	current, err := r.tasks.Get(ctx, p, task.ID)
	if err != nil {
		return err
	}
	if current.Version != task.Version {
		return d.ErrConflict
	}
	return nil
}
