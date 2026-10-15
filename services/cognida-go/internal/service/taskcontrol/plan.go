// Package taskcontrol validates and persists reviewable cross-domain plans.
// It does not dispatch domain jobs; successful preflight is not execution.
package taskcontrol

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"

	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/taskcontrol"
)

// Budget is a reservation ceiling. Runtime adapters must independently meter
// actual usage before dispatch; a proposed estimate never grants more budget.
type Budget struct {
	MaxNodes       int   `json:"max_nodes"`
	MaxConcurrency int   `json:"max_concurrency"`
	MaxAttempts    int   `json:"max_attempts"`
	MaxCalls       int   `json:"max_calls"`
	MaxCostMicros  int64 `json:"max_cost_micros"`
	TimeoutSeconds int   `json:"timeout_seconds"`
}

type InputBinding struct {
	Asset    *l.RefID `json:"asset,omitempty"`
	FromNode string   `json:"from_node,omitempty"`
	Kind     l.Kind   `json:"kind"`
}

type Node struct {
	ID                      string         `json:"id"`
	Capability              string         `json:"capability"`
	CapabilityVersion       int            `json:"capability_version"`
	DependsOn               []string       `json:"depends_on"`
	Inputs                  []InputBinding `json:"inputs"`
	OutputKinds             []l.Kind       `json:"output_kinds"`
	Tools                   []string       `json:"tools"`
	Permissions             []string       `json:"permissions"`
	MaxAttempts             int            `json:"max_attempts"`
	MaxCallsPerAttempt      int            `json:"max_calls_per_attempt"`
	MaxCostMicrosPerAttempt int64          `json:"max_cost_micros_per_attempt"`
	TimeoutSeconds          int            `json:"timeout_seconds"`
}

type PlanSpec struct {
	SchemaVersion int    `json:"schema_version"`
	Budget        Budget `json:"budget"`
	Nodes         []Node `json:"nodes"`
}

// Capability comes from trusted server configuration, never from a submitted plan.
type Capability struct {
	ID               string   `json:"id"`
	Version          int      `json:"version"`
	Domain           string   `json:"domain"`
	InputKinds       []l.Kind `json:"input_kinds"`
	OutputKinds      []l.Kind `json:"output_kinds"`
	Tools            []string `json:"tools"`
	Permissions      []string `json:"permissions"`
	RequiresApproval bool     `json:"requires_approval"`
	// At least one of these kinds must be present in the ordered input list.
	RequiredAnyInputKinds []l.Kind `json:"required_any_input_kinds,omitempty"`
	// Outputs in this set can only pass through the first matching input.
	// This models adapters which select one data input rather than convert it.
	FirstInputPassthroughKinds []l.Kind `json:"first_input_passthrough_kinds,omitempty"`
}

type Issue struct {
	NodeID  string `json:"node_id,omitempty"`
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ValidationError struct {
	Issues []Issue `json:"issues"`
}

func (e *ValidationError) Error() string { return "task plan validation failed" }
func (e *ValidationError) Unwrap() error { return d.ErrInvalid }

type Validation struct {
	Valid              bool      `json:"valid"`
	ExecutionEnabled   bool      `json:"execution_enabled"`
	AuthorizationScope string    `json:"authorization_scope"`
	OriginRevalidated  bool      `json:"origin_revalidated"`
	TopologicalOrder   []string  `json:"topological_order"`
	ApprovalRequired   []string  `json:"approval_required"`
	InputRefs          []l.RefID `json:"input_refs"`
	ReservedAttempts   int       `json:"reserved_attempts"`
	ReservedCalls      int       `json:"reserved_calls"`
	ReservedCostMicros int64     `json:"reserved_cost_micros"`
	ReservedSeconds    int       `json:"reserved_seconds"`
}

type Authorizer func(context.Context, d.Scope, string) error
type Resolver func(context.Context, d.Scope, l.RefID) (l.Kind, error)
type Validator struct {
	capabilities map[string]Capability
	authorize    Authorizer
	resolve      Resolver
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

func NewValidator(capabilities []Capability, authorize Authorizer, resolve Resolver) (*Validator, error) {
	if len(capabilities) == 0 || authorize == nil || resolve == nil {
		return nil, fmt.Errorf("%w: trusted capabilities and authorization are required", d.ErrInvalid)
	}
	v := &Validator{capabilities: map[string]Capability{}, authorize: authorize, resolve: resolve}
	for _, c := range capabilities {
		if !identifier.MatchString(c.ID) || c.Version < 1 || !identifier.MatchString(c.Domain) || len(c.InputKinds) == 0 || len(c.OutputKinds) == 0 || len(c.Tools) == 0 || len(c.Permissions) == 0 {
			return nil, d.ErrInvalid
		}
		if _, ok := v.capabilities[c.ID]; ok {
			return nil, d.ErrConflict
		}
		if !uniqueIdentifiers(c.InputKinds) || !uniqueIdentifiers(c.OutputKinds) || !uniqueIdentifiers(c.Tools) || !uniqueIdentifiers(c.Permissions) || !uniqueIdentifiers(c.RequiredAnyInputKinds) || !uniqueIdentifiers(c.FirstInputPassthroughKinds) {
			return nil, d.ErrInvalid
		}
		for _, kind := range c.RequiredAnyInputKinds {
			if !contains(c.InputKinds, kind) {
				return nil, d.ErrInvalid
			}
		}
		for _, kind := range c.FirstInputPassthroughKinds {
			if !contains(c.InputKinds, kind) || !contains(c.OutputKinds, kind) {
				return nil, d.ErrInvalid
			}
		}
		if (contains(c.Permissions, "write") || contains(c.Permissions, "etl")) && !c.RequiresApproval {
			return nil, d.ErrInvalid
		}
		// Copy slices so the registry cannot change through a caller's backing array.
		c.InputKinds = append([]l.Kind(nil), c.InputKinds...)
		c.OutputKinds = append([]l.Kind(nil), c.OutputKinds...)
		c.Tools = append([]string(nil), c.Tools...)
		c.Permissions = append([]string(nil), c.Permissions...)
		c.RequiredAnyInputKinds = append([]l.Kind(nil), c.RequiredAnyInputKinds...)
		c.FirstInputPassthroughKinds = append([]l.Kind(nil), c.FirstInputPassthroughKinds...)
		v.capabilities[c.ID] = c
	}
	return v, nil
}

func (v *Validator) Capabilities() []Capability {
	items := make([]Capability, 0, len(v.capabilities))
	for _, c := range v.capabilities {
		c.InputKinds = append([]l.Kind(nil), c.InputKinds...)
		c.OutputKinds = append([]l.Kind(nil), c.OutputKinds...)
		c.Tools = append([]string(nil), c.Tools...)
		c.Permissions = append([]string(nil), c.Permissions...)
		c.RequiredAnyInputKinds = append([]l.Kind(nil), c.RequiredAnyInputKinds...)
		c.FirstInputPassthroughKinds = append([]l.Kind(nil), c.FirstInputPassthroughKinds...)
		items = append(items, c)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}

func validBudget(b Budget) bool {
	return b.MaxNodes >= 1 && b.MaxNodes <= 32 && b.MaxConcurrency >= 1 && b.MaxConcurrency <= 8 && b.MaxConcurrency <= b.MaxNodes &&
		b.MaxAttempts >= 1 && b.MaxAttempts <= 160 && b.MaxCalls >= 1 && b.MaxCalls <= 4096 &&
		b.MaxCostMicros >= 0 && b.MaxCostMicros <= 1_000_000_000 && b.TimeoutSeconds >= 1 && b.TimeoutSeconds <= 86400
}

func (v *Validator) Validate(ctx context.Context, p d.Scope, plan PlanSpec) (*Validation, error) {
	if e := p.Validate(); e != nil {
		return nil, e
	}
	if e := v.authorize(ctx, p, "read"); e != nil {
		return nil, d.ErrForbidden
	}
	issues := []Issue{}
	add := func(node, field, code, message string) { issues = append(issues, Issue{node, field, code, message}) }
	if plan.SchemaVersion != 1 {
		add("", "schema_version", "unsupported_schema", "schema_version must be 1")
	}
	if !validBudget(plan.Budget) {
		add("", "budget", "invalid_budget", "budget is missing, negative or exceeds server limits")
	}
	if len(plan.Nodes) == 0 || len(plan.Nodes) > 32 || len(plan.Nodes) > plan.Budget.MaxNodes {
		add("", "nodes", "node_limit", "plan must contain 1-32 nodes within its budget")
	}
	if len(issues) > 0 {
		return nil, &ValidationError{issues}
	}
	result := &Validation{TopologicalOrder: []string{}, ApprovalRequired: []string{}, InputRefs: []l.RefID{}}
	nodes := map[string]Node{}
	for _, n := range plan.Nodes {
		if !identifier.MatchString(n.ID) {
			add(n.ID, "id", "invalid_id", "node id must be a bounded identifier")
		}
		if _, ok := nodes[n.ID]; ok {
			add(n.ID, "id", "duplicate_node", "node id must be unique")
		}
		nodes[n.ID] = n
		c, ok := v.capabilities[n.Capability]
		if !ok || n.CapabilityVersion != c.Version {
			add(n.ID, "capability", "unknown_capability", "capability and version must match the trusted registry")
			continue
		}
		if !equalSet(n.Tools, c.Tools) {
			add(n.ID, "tools", "tool_mismatch", "declare exactly the capability tool set")
		}
		if !equalSet(n.Permissions, c.Permissions) {
			add(n.ID, "permissions", "scope_mismatch", "declare exactly the capability permission set")
		}
		if c.RequiresApproval {
			result.ApprovalRequired = append(result.ApprovalRequired, n.ID)
		}
		if len(n.Inputs) == 0 || len(n.Inputs) > 16 {
			add(n.ID, "inputs", "invalid_inputs", "each node requires 1-16 input references")
		}
		if len(n.OutputKinds) == 0 || len(n.OutputKinds) > 16 {
			add(n.ID, "output_kinds", "invalid_outputs", "output kinds must be explicitly declared")
		}
		seenKinds := map[l.Kind]bool{}
		var firstPassthrough l.Kind
		requiredInputPresent := len(c.RequiredAnyInputKinds) == 0
		for _, input := range n.Inputs {
			if contains(c.RequiredAnyInputKinds, input.Kind) {
				requiredInputPresent = true
			}
			if firstPassthrough == "" && contains(c.FirstInputPassthroughKinds, input.Kind) {
				firstPassthrough = input.Kind
			}
		}
		if !requiredInputPresent {
			add(n.ID, "inputs", "required_input_missing", "capability requires a data input, not only supporting reports")
		}
		for _, kind := range n.OutputKinds {
			if seenKinds[kind] || !contains(c.OutputKinds, kind) {
				add(n.ID, "output_kinds", "output_kind_mismatch", "output kind is duplicated or unsupported")
			}
			if contains(c.FirstInputPassthroughKinds, kind) && kind != firstPassthrough {
				add(n.ID, "output_kinds", "passthrough_kind_mismatch", "output must match the first data input; this capability does not convert asset kinds")
			}
			seenKinds[kind] = true
		}
		if n.MaxAttempts < 1 || n.MaxAttempts > 5 || n.MaxCallsPerAttempt < 1 || n.MaxCallsPerAttempt > 128 || n.MaxCostMicrosPerAttempt < 0 || n.MaxCostMicrosPerAttempt > 1_000_000_000 || n.TimeoutSeconds < 1 || n.TimeoutSeconds > 3600 {
			add(n.ID, "budget", "invalid_node_budget", "node attempts, calls, cost or timeout exceed allowed limits")
		} else {
			result.ReservedAttempts += n.MaxAttempts
			result.ReservedCalls += n.MaxAttempts * n.MaxCallsPerAttempt
			result.ReservedCostMicros += int64(n.MaxAttempts) * n.MaxCostMicrosPerAttempt
			result.ReservedSeconds += n.MaxAttempts * n.TimeoutSeconds
		}
	}
	if result.ReservedAttempts > plan.Budget.MaxAttempts || result.ReservedCalls > plan.Budget.MaxCalls || result.ReservedCostMicros > plan.Budget.MaxCostMicros || result.ReservedSeconds > plan.Budget.TimeoutSeconds {
		add("", "budget", "budget_exceeded", "worst-case reservations including retries exceed the plan budget")
	}
	// Check all structure before resolving any asset or making domain calls.
	indegree := map[string]int{}
	edges := map[string][]string{}
	for _, n := range plan.Nodes {
		seen := map[string]bool{}
		for _, dependency := range n.DependsOn {
			if _, ok := nodes[dependency]; !ok || dependency == n.ID || seen[dependency] {
				add(n.ID, "depends_on", "invalid_dependency", "dependency is missing, repeated or self-referential")
				continue
			}
			seen[dependency] = true
			indegree[n.ID]++
			edges[dependency] = append(edges[dependency], n.ID)
		}
		c, ok := v.capabilities[n.Capability]
		if !ok {
			continue
		}
		for _, input := range n.Inputs {
			if (input.Asset == nil) == (input.FromNode == "") {
				add(n.ID, "inputs", "ambiguous_input", "provide exactly one asset reference or predecessor output")
				continue
			}
			if !contains(c.InputKinds, input.Kind) {
				add(n.ID, "inputs", "input_kind_mismatch", "input kind is unsupported by the capability")
			}
			if input.Asset != nil {
				if e := input.Asset.Validate(); e != nil {
					add(n.ID, "inputs", "invalid_reference", "asset reference requires a UUID and positive version")
				}
				continue
			}
			producer, exists := nodes[input.FromNode]
			if !exists || !seen[input.FromNode] || !contains(producer.OutputKinds, input.Kind) {
				add(n.ID, "inputs", "unbound_output", "input must bind a declared output of a direct dependency")
			}
		}
	}
	if len(issues) > 0 {
		return nil, &ValidationError{issues}
	}
	queue := []string{}
	for _, n := range plan.Nodes {
		if indegree[n.ID] == 0 {
			queue = append(queue, n.ID)
		}
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		result.TopologicalOrder = append(result.TopologicalOrder, id)
		for _, next := range edges[id] {
			indegree[next]--
			if indegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if len(result.TopologicalOrder) != len(nodes) {
		return nil, &ValidationError{[]Issue{{Field: "depends_on", Code: "cycle", Message: "plan dependencies must be acyclic"}}}
	}
	permissions := map[string]bool{"read": true}
	resolved := map[string]l.Kind{}
	for _, n := range plan.Nodes {
		for _, permission := range v.capabilities[n.Capability].Permissions {
			if !permissions[permission] {
				if e := v.authorize(ctx, p, permission); e != nil {
					return nil, d.ErrForbidden
				}
				permissions[permission] = true
			}
		}
		for _, input := range n.Inputs {
			if input.Asset == nil {
				continue
			}
			key := input.Asset.String()
			kind, ok := resolved[key]
			if !ok {
				if len(resolved) >= 64 {
					return nil, &ValidationError{[]Issue{{Field: "inputs", Code: "reference_limit", Message: "plan exceeds 64 distinct source references"}}}
				}
				var e error
				kind, e = v.resolve(ctx, p, *input.Asset)
				if e != nil {
					return nil, d.ErrForbidden
				}
				resolved[key] = kind
				result.InputRefs = append(result.InputRefs, *input.Asset)
			}
			if kind != input.Kind {
				return nil, &ValidationError{[]Issue{{NodeID: n.ID, Field: "inputs", Code: "source_kind_mismatch", Message: "declared kind does not match the authorized asset"}}}
			}
		}
	}
	result.Valid = true
	// This release exposes draft/preflight only. Do not imply that an executor,
	// approval or runtime budget enforcement has already run.
	result.ExecutionEnabled = false
	// The current resolver checks lifecycle asset access and integrity. Source
	// invalidation propagation must be implemented before executable plans ship.
	result.AuthorizationScope = "lifecycle_asset"
	result.OriginRevalidated = false
	return result, nil
}

func uniqueIdentifiers[T ~string](items []T) bool {
	seen := map[T]bool{}
	for _, item := range items {
		if !identifier.MatchString(string(item)) || seen[item] {
			return false
		}
		seen[item] = true
	}
	return true
}

func equalSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]bool{}
	for _, s := range a {
		if seen[s] {
			return false
		}
		seen[s] = true
	}
	for _, s := range b {
		if !seen[s] {
			return false
		}
	}
	return true
}
func contains[T comparable](items []T, value T) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func decodeStored[T any](raw json.RawMessage) (T, error) {
	var out T
	e := json.Unmarshal(raw, &out)
	return out, e
}
