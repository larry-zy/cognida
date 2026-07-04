// 并行委派工具（Phase 7 任务 8.5）：独立子任务并行 fan-out，受并发上限护栏
// （与 IsCyclic / MaxDepth 并列）。每个子委派独立成败——单个失败不牵连并行兄弟，
// 失败项可由指挥官按信封单独重试（delegate_to_agent 重发该项即可）。
package framework

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/cloudwego/eino/schema"

	domainagent "link/internal/model/agent"
)

// defaultDelegateConcurrency 并行委派默认并发上限（资源护栏，防子代理树暴涨）。
const defaultDelegateConcurrency = 3

// ParallelDelegateTool implements tool.InvokableTool for fan-out delegation.
type ParallelDelegateTool struct {
	registry       *CollaborationRegistry
	maxConcurrency int
}

// NewParallelDelegateTool creates a new parallel delegate tool.
// maxConcurrency <=0 时使用默认并发上限。
func NewParallelDelegateTool(registry *CollaborationRegistry, maxConcurrency int) *ParallelDelegateTool {
	if maxConcurrency <= 0 {
		maxConcurrency = defaultDelegateConcurrency
	}
	return &ParallelDelegateTool{
		registry:       registry,
		maxConcurrency: maxConcurrency,
	}
}

// Info returns the tool description for LLM.
func (t *ParallelDelegateTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	agentList := formatAgentInfos(t.registry.ListWithDescriptions())

	return &schema.ToolInfo{
		Name: "delegate_parallel",
		Desc: fmt.Sprintf("并行委派多个**相互独立**的子任务（如多数据源/多指标同时探查）。依赖链任务请改用 delegate_to_agent 串行委派。每项须携完整委派信封；单项失败不影响其他项，失败项可单独重发。并发受上限约束（%d）。\n\nAvailable agents:\n%s",
			t.maxConcurrency, agentList),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"delegations": {
				Type:     schema.Array,
				Desc:     "委派信封列表，每项对应一个独立子任务",
				Required: true,
				ElemInfo: &schema.ParameterInfo{
					Type:      schema.Object,
					SubParams: delegationEnvelopeParams(),
				},
			},
		}),
	}, nil
}

// parallelDelegationResult 单个子委派的结果项（成败独立）。
type parallelDelegationResult struct {
	AgentName string `json:"agent_name"`
	Goal      string `json:"goal"`
	Status    string `json:"status"` // ok / failed
	Output    string `json:"output,omitempty"`
	Error     string `json:"error,omitempty"`
}

// InvokableRun executes fan-out delegation with a concurrency cap.
func (t *ParallelDelegateTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...any) (string, error) {
	var args struct {
		Delegations []DelegationEnvelope `json:"delegations"`
	}
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}
	if len(args.Delegations) == 0 {
		return "", fmt.Errorf("delegations 不能为空")
	}

	parentCollab, hasCtx := domainagent.GetCollaborationContext(ctx)

	results := make([]parallelDelegationResult, len(args.Delegations))
	sem := make(chan struct{}, t.maxConcurrency)
	var wg sync.WaitGroup

	for i := range args.Delegations {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{} // 并发上限护栏
			defer func() { <-sem }()

			env := &args.Delegations[idx]
			// 每个子委派持父协作上下文的独立克隆：链路/结果互不干扰（并行安全），
			// 真实结果在全部完成后串行回写父上下文。
			childCtx := ctx
			if hasCtx {
				childCtx = domainagent.WithCollaborationContext(ctx, parentCollab.Clone())
			}

			output, err := executeDelegation(childCtx, t.registry, env)
			results[idx] = parallelDelegationResult{
				AgentName: env.AgentName,
				Goal:      env.Goal,
			}
			if err != nil {
				results[idx].Status = "failed"
				results[idx].Error = err.Error()
				return
			}
			results[idx].Status = "ok"
			results[idx].Output = output
		}(i)
	}
	wg.Wait()

	// 串行回写父协作上下文（避免并发写共享 map）。注意不回写 DelegateChain：
	// 循环检测按委派路径判定，父链追加并行兄弟会误伤后续对同一子代理的再委派。
	succeeded := 0
	for i := range results {
		if results[i].Status == "ok" {
			succeeded++
			if hasCtx {
				parentCollab.StoreResult(results[i].AgentName, &domainagent.TaskResult{
					AgentName: results[i].AgentName,
					Content:   results[i].Output,
				})
			}
		}
	}

	summary := map[string]interface{}{
		"total":     len(results),
		"succeeded": succeeded,
		"failed":    len(results) - succeeded,
		"results":   results,
	}
	if succeeded < len(results) {
		summary["hint"] = "失败项可用 delegate_to_agent 携原信封单独重试，无需重跑已成功项"
	}
	out, err := json.Marshal(summary)
	if err != nil {
		return "", fmt.Errorf("序列化并行委派结果失败: %w", err)
	}
	return string(out), nil
}
