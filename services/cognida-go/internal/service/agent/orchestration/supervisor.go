package orchestration

import (
	"context"
	"fmt"
	"strings"

	"cognida/internal/pkg/safego"
	infraagent "cognida/internal/service/agent/framework"
)

// Supervisor creates an agent where a coordinator routes messages to worker agents.
// The coordinator decides which worker should handle each request.
func Supervisor(coordinator infraagent.Agent, workers ...infraagent.Agent) infraagent.Agent {
	if len(workers) == 0 {
		return coordinator
	}
	return &supervisorAgent{
		coordinator: coordinator,
		workers:     workers,
	}
}

type supervisorAgent struct {
	coordinator infraagent.Agent
	workers     []infraagent.Agent
}

func (s *supervisorAgent) Chat(ctx context.Context, message string) (*infraagent.Response, error) {
	// First, ask the coordinator which worker to use
	coordResp, err := s.coordinator.Chat(ctx, fmt.Sprintf(
		"Given the user request: '%s', which worker agent should handle this? "+
			"Respond with just the worker index (0-%d), or -1 to handle directly.",
		message, len(s.workers)-1,
	))
	if err != nil {
		return nil, fmt.Errorf("Supervisor: coordinator failed: %w", err)
	}

	// Parse coordinator's decision
	// For simplicity, we'll check if response mentions a worker index
	workerIndex := s.parseWorkerIndex(coordResp.Content)

	if workerIndex < 0 || workerIndex >= len(s.workers) {
		// Coordinator couldn't decide, handle directly
		return coordResp, nil
	}

	// Route to the selected worker
	workerResp, err := s.workers[workerIndex].Chat(ctx, message)
	if err != nil {
		return nil, fmt.Errorf("Supervisor: worker %d failed: %w", workerIndex, err)
	}

	return workerResp, nil
}

func (s *supervisorAgent) Stream(ctx context.Context, message string) (<-chan *infraagent.Chunk, error) {
	out := make(chan *infraagent.Chunk, 1)

	go func() {
		defer safego.Recover("supervisor-stream")
		defer close(out)

		// Get coordinator decision via Chat (simplified)
		coordResp, err := s.coordinator.Chat(ctx, fmt.Sprintf(
			"Given the user request: '%s', which worker agent should handle this? "+
				"Respond with just the worker index (0-%d).",
			message, len(s.workers)-1,
		))
		if err != nil {
			out <- &infraagent.Chunk{
				Content: "",
				Done:    true,
				Metadata: map[string]interface{}{
					"error": fmt.Sprintf("coordinator failed: %v", err),
				},
			}
			return
		}

		workerIndex := s.parseWorkerIndex(coordResp.Content)

		if workerIndex < 0 || workerIndex >= len(s.workers) {
			// Handle directly
			out <- &infraagent.Chunk{
				Content: coordResp.Content,
				Done:    true,
			}
			return
		}

		// Stream from selected worker
		ch, err := s.workers[workerIndex].Stream(ctx, message)
		if err != nil {
			out <- &infraagent.Chunk{
				Content: "",
				Done:    true,
				Metadata: map[string]interface{}{
					"error": fmt.Sprintf("worker %d failed: %v", workerIndex, err),
				},
			}
			return
		}

		for chunk := range ch {
			out <- chunk
		}
	}()

	return out, nil
}

func (s *supervisorAgent) Name() string {
	return fmt.Sprintf("Supervisor(coordinator: %s, workers: %d)", s.coordinator.Name(), len(s.workers))
}

// parseWorkerIndex attempts to extract a worker index from the coordinator's response.
func (s *supervisorAgent) parseWorkerIndex(content string) int {
	// Try to find a number in the response
	// This is a simplified implementation
	for _, c := range content {
		if c >= '0' && c <= '9' {
			return int(c - '0')
		}
	}
	return -1
}

// NamedSupervisor creates a supervisor with named workers that can be selected by name.
type NamedSupervisor struct {
	coordinator  infraagent.Agent
	workerMap    map[string]infraagent.Agent
	defaultAgent infraagent.Agent
}

// NewNamedSupervisor creates a new named supervisor.
func NewNamedSupervisor(coordinator infraagent.Agent, defaultAgent infraagent.Agent) *NamedSupervisor {
	return &NamedSupervisor{
		coordinator:  coordinator,
		workerMap:    make(map[string]infraagent.Agent),
		defaultAgent: defaultAgent,
	}
}

// AddWorker adds a named worker to the supervisor.
func (s *NamedSupervisor) AddWorker(name string, worker infraagent.Agent) {
	s.workerMap[name] = worker
}

// Chat implements infraagent.Agent.
func (s *NamedSupervisor) Chat(ctx context.Context, message string) (*infraagent.Response, error) {
	// Ask coordinator which worker to use
	coordResp, err := s.coordinator.Chat(ctx, fmt.Sprintf(
		"Given the user request: '%s', which worker should handle this? "+
			"Available workers: %v. Respond with just the worker name.",
		message, s.workerNames(),
	))
	if err != nil {
		return nil, fmt.Errorf("NamedSupervisor: coordinator failed: %w", err)
	}

	// Try to find the worker by name
	workerName := s.parseWorkerName(coordResp.Content)
	if worker, ok := s.workerMap[workerName]; ok {
		return worker.Chat(ctx, message)
	}

	// Fall back to default
	if s.defaultAgent != nil {
		return s.defaultAgent.Chat(ctx, message)
	}

	return coordResp, nil
}

// Stream implements infraagent.Agent.
func (s *NamedSupervisor) Stream(ctx context.Context, message string) (<-chan *infraagent.Chunk, error) {
	out := make(chan *infraagent.Chunk, 1)

	go func() {
		defer safego.Recover("supervisor-stream")
		defer close(out)

		coordResp, err := s.coordinator.Chat(ctx, fmt.Sprintf(
			"Given the user request: '%s', which worker should handle this? "+
				"Available workers: %v. Respond with just the worker name.",
			message, s.workerNames(),
		))
		if err != nil {
			out <- &infraagent.Chunk{
				Content: "",
				Done:    true,
				Metadata: map[string]interface{}{
					"error": err.Error(),
				},
			}
			return
		}

		workerName := s.parseWorkerName(coordResp.Content)
		if worker, ok := s.workerMap[workerName]; ok {
			ch, err := worker.Stream(ctx, message)
			if err != nil {
				out <- &infraagent.Chunk{
					Content: "",
					Done:    true,
					Metadata: map[string]interface{}{
						"error": err.Error(),
					},
				}
				return
			}
			for chunk := range ch {
				out <- chunk
			}
			return
		}

		// Fall back to default or coordinator response
		if s.defaultAgent != nil {
			ch, err := s.defaultAgent.Stream(ctx, message)
			if err != nil {
				out <- &infraagent.Chunk{Content: coordResp.Content, Done: true}
				return
			}
			for chunk := range ch {
				out <- chunk
			}
			return
		}

		out <- &infraagent.Chunk{Content: coordResp.Content, Done: true}
	}()

	return out, nil
}

func (s *NamedSupervisor) Name() string {
	return fmt.Sprintf("NamedSupervisor(workers: %d)", len(s.workerMap))
}

func (s *NamedSupervisor) workerNames() []string {
	names := make([]string, 0, len(s.workerMap))
	for name := range s.workerMap {
		names = append(names, name)
	}
	return names
}

func (s *NamedSupervisor) parseWorkerName(content string) string {
	// Try to match worker names in content
	for name := range s.workerMap {
		if strings.Contains(content, name) {
			return name
		}
	}
	return ""
}
