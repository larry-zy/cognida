package orchestration

import (
	"context"
	"testing"

	_ "link/internal/service/agent/framework" // For framework.Agent and framework.Response types used by mockAgentForOrchestration

	"github.com/stretchr/testify/assert"
)

// TestSequential_TwoAgents tests Sequential with two agents.
func TestSequential_TwoAgents(t *testing.T) {
	firstAgent := &mockAgentForOrchestration{name: "first", response: "first response"}
	secondAgent := &mockAgentForOrchestration{name: "second", response: "second response"}

	seqAgent := Sequential(firstAgent, secondAgent)

	ctx := context.Background()
	resp, err := seqAgent.Chat(ctx, "test")

	assert.NoError(t, err)
	// Sequential passes output of one agent to the next
	// So the final response is just "second response"
	assert.Equal(t, "second response", resp.Content)
}

// TestSequential_ThreeAgents tests Sequential with three agents.
func TestSequential_ThreeAgents(t *testing.T) {
	firstAgent := &mockAgentForOrchestration{name: "first", response: "first"}
	secondAgent := &mockAgentForOrchestration{name: "second", response: "second"}
	thirdAgent := &mockAgentForOrchestration{name: "third", response: "third"}

	seqAgent := Sequential(firstAgent, secondAgent, thirdAgent)

	ctx := context.Background()
	resp, err := seqAgent.Chat(ctx, "test")

	assert.NoError(t, err)
	// Sequential passes output through chain, so final is "third"
	assert.Equal(t, "third", resp.Content)
}

// TestSequential_Empty tests Sequential with no agents.
func TestSequential_Empty(t *testing.T) {
	seqAgent := Sequential()

	ctx := context.Background()
	_, err := seqAgent.Chat(ctx, "test")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no agents provided")
}

// TestSequential_SingleAgent tests Sequential with one agent.
func TestSequential_SingleAgent(t *testing.T) {
	singleAgent := &mockAgentForOrchestration{name: "single", response: "single response"}

	seqAgent := Sequential(singleAgent)

	ctx := context.Background()
	resp, err := seqAgent.Chat(ctx, "test")

	assert.NoError(t, err)
	assert.Equal(t, "single response", resp.Content)
}
