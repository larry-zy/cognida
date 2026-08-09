package orchestration

import (
	"context"
	"fmt"
	"strings"

	infraagent "cognida/internal/service/agent/framework"
)

// Conditional creates an agent that selects between two agents based on a predicate.
// The predicate is evaluated on each Chat call.
func Conditional(predicate func(message string) bool, trueAgent, falseAgent infraagent.Agent) infraagent.Agent {
	if trueAgent == nil {
		return falseAgent
	}
	if falseAgent == nil {
		return trueAgent
	}
	return &conditionalAgent{
		predicate:   predicate,
		trueAgent:   trueAgent,
		falseAgent:  falseAgent,
	}
}

type conditionalAgent struct {
	predicate  func(message string) bool
	trueAgent  infraagent.Agent
	falseAgent infraagent.Agent
}

func (c *conditionalAgent) Chat(ctx context.Context, message string) (*infraagent.Response, error) {
	if c.predicate(message) {
		return c.trueAgent.Chat(ctx, message)
	}
	return c.falseAgent.Chat(ctx, message)
}

func (c *conditionalAgent) Stream(ctx context.Context, message string) (<-chan *infraagent.Chunk, error) {
	if c.predicate(message) {
		return c.trueAgent.Stream(ctx, message)
	}
	return c.falseAgent.Stream(ctx, message)
}

func (c *conditionalAgent) Name() string {
	return fmt.Sprintf("Conditional(%s, %s)", c.trueAgent.Name(), c.falseAgent.Name())
}

// Branch creates an agent with multiple branches.
// Each branch has a predicate and an associated infraagent.
// The first matching predicate is executed; if none match, the default agent is used.
func Branch(defaultAgent infraagent.Agent, branches ...BranchEntry) infraagent.Agent {
	return &branchAgent{
		defaultAgent: defaultAgent,
		branches:     branches,
	}
}

// BranchEntry represents a predicate-agent pair for branching.
type BranchEntry struct {
	Predicate func(message string) bool
	Agent     infraagent.Agent
	Name      string
}

type branchAgent struct {
	defaultAgent infraagent.Agent
	branches     []BranchEntry
}

func (b *branchAgent) Chat(ctx context.Context, message string) (*infraagent.Response, error) {
	for _, entry := range b.branches {
		if entry.Predicate(message) {
			return entry.Agent.Chat(ctx, message)
		}
	}
	return b.defaultAgent.Chat(ctx, message)
}

func (b *branchAgent) Stream(ctx context.Context, message string) (<-chan *infraagent.Chunk, error) {
	for _, entry := range b.branches {
		if entry.Predicate(message) {
			return entry.Agent.Stream(ctx, message)
		}
	}
	return b.defaultAgent.Stream(ctx, message)
}

func (b *branchAgent) Name() string {
	names := make([]string, len(b.branches))
	for i, entry := range b.branches {
		if entry.Name != "" {
			names[i] = entry.Name
		} else {
			names[i] = entry.Agent.Name()
		}
	}
	return fmt.Sprintf("Branch(default: %s, branches: %v)", b.defaultAgent.Name(), names)
}

// Switch creates an agent that routes based on message content matching.
func Switch(defaultAgent infraagent.Agent, cases map[string]infraagent.Agent) infraagent.Agent {
	return &switchAgent{
		defaultAgent: defaultAgent,
		cases:        cases,
	}
}

type switchAgent struct {
	defaultAgent infraagent.Agent
	cases        map[string]infraagent.Agent
}

func (s *switchAgent) Chat(ctx context.Context, message string) (*infraagent.Response, error) {
	for key, agent := range s.cases {
		if strings.Contains(message, key) {
			return agent.Chat(ctx, message)
		}
	}
	return s.defaultAgent.Chat(ctx, message)
}

func (s *switchAgent) Stream(ctx context.Context, message string) (<-chan *infraagent.Chunk, error) {
	for key, agent := range s.cases {
		if strings.Contains(message, key) {
			return agent.Stream(ctx, message)
		}
	}
	return s.defaultAgent.Stream(ctx, message)
}

func (s *switchAgent) Name() string {
	keys := make([]string, 0, len(s.cases))
	for key := range s.cases {
		keys = append(keys, key)
	}
	return fmt.Sprintf("Switch(keys: %v, default: %s)", keys, s.defaultAgent.Name())
}

// Route creates an agent that routes based on a categorization function.
// The categorizer returns a string key, which is used to select the infraagent.
func Route(categorizer func(message string) string, agents map[string]infraagent.Agent, defaultAgent infraagent.Agent) infraagent.Agent {
	return &routeAgent{
		categorizer:  categorizer,
		agents:       agents,
		defaultAgent: defaultAgent,
	}
}

type routeAgent struct {
	categorizer  func(message string) string
	agents       map[string]infraagent.Agent
	defaultAgent infraagent.Agent
}

func (r *routeAgent) Chat(ctx context.Context, message string) (*infraagent.Response, error) {
	key := r.categorizer(message)
	if agent, ok := r.agents[key]; ok {
		return agent.Chat(ctx, message)
	}
	return r.defaultAgent.Chat(ctx, message)
}

func (r *routeAgent) Stream(ctx context.Context, message string) (<-chan *infraagent.Chunk, error) {
	key := r.categorizer(message)
	if agent, ok := r.agents[key]; ok {
		return agent.Stream(ctx, message)
	}
	return r.defaultAgent.Stream(ctx, message)
}

func (r *routeAgent) Name() string {
	return fmt.Sprintf("Route(agents: %d)", len(r.agents))
}
