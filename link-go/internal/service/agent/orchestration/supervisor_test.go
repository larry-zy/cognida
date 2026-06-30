package orchestration

import (
	"context"
	"testing"

	_ "link/internal/service/agent/framework" // For framework.Agent and framework.Response types used by mockAgentForOrchestration

	"github.com/stretchr/testify/assert"
)

// TestSupervisor_RoutesToWorker tests Supervisor routing to a worker.
func TestSupervisor_RoutesToWorker(t *testing.T) {
	// Create a coordinator that returns "0" to route to first worker
	coordinator := &mockAgentForOrchestration{
		name:     "coordinator",
		response: "0", // Select first worker
	}

	worker1 := &mockAgentForOrchestration{name: "worker1", response: "worker1 response"}
	worker2 := &mockAgentForOrchestration{name: "worker2", response: "worker2 response"}

	supervisorAgent := Supervisor(coordinator, worker1, worker2)

	ctx := context.Background()
	resp, err := supervisorAgent.Chat(ctx, "test message")

	assert.NoError(t, err)
	// Should route to worker1
	assert.Equal(t, "worker1 response", resp.Content)
}

// TestSupervisor_RoutesToSecondWorker tests Supervisor routing to second worker.
func TestSupervisor_RoutesToSecondWorker(t *testing.T) {
	// Create a coordinator that returns "1" to route to second worker
	coordinator := &mockAgentForOrchestration{
		name:     "coordinator",
		response: "1", // Select second worker
	}

	worker1 := &mockAgentForOrchestration{name: "worker1", response: "worker1 response"}
	worker2 := &mockAgentForOrchestration{name: "worker2", response: "worker2 response"}

	supervisorAgent := Supervisor(coordinator, worker1, worker2)

	ctx := context.Background()
	resp, err := supervisorAgent.Chat(ctx, "test message")

	assert.NoError(t, err)
	// Should route to worker2
	assert.Equal(t, "worker2 response", resp.Content)
}

// TestSupervisor_InvalidIndex tests Supervisor with invalid worker index.
func TestSupervisor_InvalidIndex(t *testing.T) {
	// Create a coordinator that returns "9" (out of range)
	coordinator := &mockAgentForOrchestration{
		name:     "coordinator",
		response: "9",
	}

	worker1 := &mockAgentForOrchestration{name: "worker1", response: "worker1 response"}
	worker2 := &mockAgentForOrchestration{name: "worker2", response: "worker2 response"}

	supervisorAgent := Supervisor(coordinator, worker1, worker2)

	ctx := context.Background()
	resp, err := supervisorAgent.Chat(ctx, "test message")

	assert.NoError(t, err)
	// Should fall back to coordinator response
	assert.Equal(t, "9", resp.Content)
}

// TestSupervisor_NoWorkers tests Supervisor with no workers.
func TestSupervisor_NoWorkers(t *testing.T) {
	coordinator := &mockAgentForOrchestration{
		name:     "coordinator",
		response: "coordinator response",
	}

	supervisorAgent := Supervisor(coordinator)

	ctx := context.Background()
	resp, err := supervisorAgent.Chat(ctx, "test message")

	assert.NoError(t, err)
	// Should return coordinator response directly
	assert.Equal(t, "coordinator response", resp.Content)
}

// TestNamedSupervisor_AddWorker tests NamedSupervisor with named workers.
func TestNamedSupervisor_AddWorker(t *testing.T) {
	coordinator := &mockAgentForOrchestration{
		name:     "coordinator",
		response: "search", // Select the "search" worker
	}

	defaultAgent := &mockAgentForOrchestration{
		name:     "default",
		response: "default response",
	}

	searchWorker := &mockAgentForOrchestration{
		name:     "search",
		response: "search results",
	}

	sup := NewNamedSupervisor(coordinator, defaultAgent)
	sup.AddWorker("search", searchWorker)

	ctx := context.Background()
	resp, err := sup.Chat(ctx, "test message")

	assert.NoError(t, err)
	assert.Equal(t, "search results", resp.Content)
}

// TestNamedSupervisor_NoMatch tests NamedSupervisor with no matching worker.
func TestNamedSupervisor_NoMatch(t *testing.T) {
	coordinator := &mockAgentForOrchestration{
		name:     "coordinator",
		response: "unknown-worker",
	}

	defaultAgent := &mockAgentForOrchestration{
		name:     "default",
		response: "default response",
	}

	searchWorker := &mockAgentForOrchestration{
		name:     "search",
		response: "search results",
	}

	sup := NewNamedSupervisor(coordinator, defaultAgent)
	sup.AddWorker("search", searchWorker)

	ctx := context.Background()
	resp, err := sup.Chat(ctx, "test message")

	assert.NoError(t, err)
	// Should fall back to default
	assert.Equal(t, "default response", resp.Content)
}

// TestNamedSupervisor_Name tests NamedSupervisor Name method.
func TestNamedSupervisor_Name(t *testing.T) {
	coordinator := &mockAgentForOrchestration{name: "coord"}
	defaultAgent := &mockAgentForOrchestration{name: "default"}

	sup := NewNamedSupervisor(coordinator, defaultAgent)
	sup.AddWorker("worker1", &mockAgentForOrchestration{name: "w1"})
	sup.AddWorker("worker2", &mockAgentForOrchestration{name: "w2"})

	name := sup.Name()
	assert.Contains(t, name, "NamedSupervisor")
	assert.Contains(t, name, "2")
}
