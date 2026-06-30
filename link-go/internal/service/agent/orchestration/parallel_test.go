package orchestration

import (
	"context"
	"testing"

	_ "link/internal/service/agent/framework" // For framework.Agent and framework.Response types used by mockAgentForOrchestration

	"github.com/stretchr/testify/assert"
)

// TestParallel_TwoAgents tests Parallel with two agents.
func TestParallel_TwoAgents(t *testing.T) {
	agent1 := &mockAgentForOrchestration{name: "agent1", response: "response1"}
	agent2 := &mockAgentForOrchestration{name: "agent2", response: "response2"}

	parallelAgent := Parallel(agent1, agent2)

	ctx := context.Background()
	resp, err := parallelAgent.Chat(ctx, "test")

	assert.NoError(t, err)
	// Parallel combines all responses
	assert.Contains(t, resp.Content, "response1")
	assert.Contains(t, resp.Content, "response2")
}

// TestParallel_ThreeAgents tests Parallel with three agents.
func TestParallel_ThreeAgents(t *testing.T) {
	agent1 := &mockAgentForOrchestration{name: "agent1", response: "response1"}
	agent2 := &mockAgentForOrchestration{name: "agent2", response: "response2"}
	agent3 := &mockAgentForOrchestration{name: "agent3", response: "response3"}

	parallelAgent := Parallel(agent1, agent2, agent3)

	ctx := context.Background()
	resp, err := parallelAgent.Chat(ctx, "test")

	assert.NoError(t, err)
	assert.Contains(t, resp.Content, "response1")
	assert.Contains(t, resp.Content, "response2")
	assert.Contains(t, resp.Content, "response3")
}

// TestParallel_Empty tests Parallel with no agents.
func TestParallel_Empty(t *testing.T) {
	parallelAgent := Parallel()

	ctx := context.Background()
	_, err := parallelAgent.Chat(ctx, "test")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no agents provided")
}

// TestParallel_SingleAgent tests Parallel with one agent.
func TestParallel_SingleAgent(t *testing.T) {
	agent1 := &mockAgentForOrchestration{name: "agent1", response: "single response"}

	parallelAgent := Parallel(agent1)

	ctx := context.Background()
	resp, err := parallelAgent.Chat(ctx, "test")

	assert.NoError(t, err)
	assert.Equal(t, "single response", resp.Content)
}
