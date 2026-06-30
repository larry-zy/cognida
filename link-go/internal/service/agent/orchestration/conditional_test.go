package orchestration

import (
	"context"
	"testing"

	"link/internal/service/agent/framework"

	"github.com/stretchr/testify/assert"
)

// mockAgentForOrchestration is a simple mock agent for orchestration tests.
type mockAgentForOrchestration struct {
	name     string
	response string
}

func (m *mockAgentForOrchestration) Chat(ctx context.Context, message string) (*framework.Response, error) {
	return &framework.Response{
		Content: m.response,
	}, nil
}

func (m *mockAgentForOrchestration) Stream(ctx context.Context, message string) (<-chan *framework.Chunk, error) {
	ch := make(chan *framework.Chunk, 1)
	go func() {
		defer close(ch)
		ch <- &framework.Chunk{
			Content: m.response,
			Done:    true,
		}
	}()
	return ch, nil
}

func (m *mockAgentForOrchestration) Name() string {
	return m.name
}

// TestConditional_TrueBranch tests the true branch of Conditional.
func TestConditional_TrueBranch(t *testing.T) {
	trueAgent := &mockAgentForOrchestration{name: "true", response: "true response"}
	falseAgent := &mockAgentForOrchestration{name: "false", response: "false response"}

	condAgent := Conditional(func(message string) bool {
		return message == "yes"
	}, trueAgent, falseAgent)

	ctx := context.Background()
	resp, err := condAgent.Chat(ctx, "yes")

	assert.NoError(t, err)
	assert.Equal(t, "true response", resp.Content)
}

// TestConditional_FalseBranch tests the false branch of Conditional.
func TestConditional_FalseBranch(t *testing.T) {
	trueAgent := &mockAgentForOrchestration{name: "true", response: "true response"}
	falseAgent := &mockAgentForOrchestration{name: "false", response: "false response"}

	condAgent := Conditional(func(message string) bool {
		return message == "yes"
	}, trueAgent, falseAgent)

	ctx := context.Background()
	resp, err := condAgent.Chat(ctx, "no")

	assert.NoError(t, err)
	assert.Equal(t, "false response", resp.Content)
}

// TestConditional_NilTrueAgent tests Conditional with nil true agent.
func TestConditional_NilTrueAgent(t *testing.T) {
	falseAgent := &mockAgentForOrchestration{name: "false", response: "response"}

	condAgent := Conditional(func(message string) bool {
		return true
	}, nil, falseAgent)

	ctx := context.Background()
	resp, err := condAgent.Chat(ctx, "test")

	assert.NoError(t, err)
	assert.Equal(t, "response", resp.Content)
}

// TestConditional_NilFalseAgent tests Conditional with nil false agent.
func TestConditional_NilFalseAgent(t *testing.T) {
	trueAgent := &mockAgentForOrchestration{name: "true", response: "response"}

	condAgent := Conditional(func(message string) bool {
		return false
	}, trueAgent, nil)

	ctx := context.Background()
	resp, err := condAgent.Chat(ctx, "test")

	assert.NoError(t, err)
	assert.Equal(t, "response", resp.Content)
}

// TestBranch_Match tests Branch with a matching predicate.
func TestBranch_Match(t *testing.T) {
	defaultAgent := &mockAgentForOrchestration{name: "default", response: "default"}
	branch1Agent := &mockAgentForOrchestration{name: "branch1", response: "branch1"}

	branchAgent := Branch(defaultAgent,
		BranchEntry{
			Predicate: func(message string) bool { return message == "test" },
			Agent:     branch1Agent,
			Name:      "test-branch",
		},
	)

	ctx := context.Background()
	resp, err := branchAgent.Chat(ctx, "test")

	assert.NoError(t, err)
	assert.Equal(t, "branch1", resp.Content)
}

// TestBranch_NoMatch tests Branch with no matching predicate.
func TestBranch_NoMatch(t *testing.T) {
	defaultAgent := &mockAgentForOrchestration{name: "default", response: "default"}
	branch1Agent := &mockAgentForOrchestration{name: "branch1", response: "branch1"}

	branchAgent := Branch(defaultAgent,
		BranchEntry{
			Predicate: func(message string) bool { return message == "test" },
			Agent:     branch1Agent,
		},
	)

	ctx := context.Background()
	resp, err := branchAgent.Chat(ctx, "other")

	assert.NoError(t, err)
	assert.Equal(t, "default", resp.Content)
}

// TestSwitch_Match tests Switch with a matching key.
func TestSwitch_Match(t *testing.T) {
	defaultAgent := &mockAgentForOrchestration{name: "default", response: "default"}
	caseAgent := &mockAgentForOrchestration{name: "case", response: "case"}

	switchAgent := Switch(defaultAgent, map[string]framework.Agent{
		"hello": caseAgent,
	})

	ctx := context.Background()
	resp, err := switchAgent.Chat(ctx, "hello world")

	assert.NoError(t, err)
	assert.Equal(t, "case", resp.Content)
}

// TestSwitch_NoMatch tests Switch with no matching key.
func TestSwitch_NoMatch(t *testing.T) {
	defaultAgent := &mockAgentForOrchestration{name: "default", response: "default"}
	caseAgent := &mockAgentForOrchestration{name: "case", response: "case"}

	switchAgent := Switch(defaultAgent, map[string]framework.Agent{
		"hello": caseAgent,
	})

	ctx := context.Background()
	resp, err := switchAgent.Chat(ctx, "goodbye world")

	assert.NoError(t, err)
	assert.Equal(t, "default", resp.Content)
}

// TestRoute_Match tests Route with a matching category.
func TestRoute_Match(t *testing.T) {
	defaultAgent := &mockAgentForOrchestration{name: "default", response: "default"}
	categoryAgent := &mockAgentForOrchestration{name: "category", response: "category"}

	routeAgent := Route(
		func(message string) string {
			if message == "test" {
				return "test-category"
			}
			return "other"
		},
		map[string]framework.Agent{
			"test-category": categoryAgent,
		},
		defaultAgent,
	)

	ctx := context.Background()
	resp, err := routeAgent.Chat(ctx, "test")

	assert.NoError(t, err)
	assert.Equal(t, "category", resp.Content)
}

// TestRoute_NoMatch tests Route with no matching category.
func TestRoute_NoMatch(t *testing.T) {
	defaultAgent := &mockAgentForOrchestration{name: "default", response: "default"}
	categoryAgent := &mockAgentForOrchestration{name: "category", response: "category"}

	routeAgent := Route(
		func(message string) string {
			return "unknown"
		},
		map[string]framework.Agent{
			"test-category": categoryAgent,
		},
		defaultAgent,
	)

	ctx := context.Background()
	resp, err := routeAgent.Chat(ctx, "test")

	assert.NoError(t, err)
	assert.Equal(t, "default", resp.Content)
}
