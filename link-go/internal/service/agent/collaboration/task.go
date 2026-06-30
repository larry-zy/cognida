// Package collaboration provides Multi-Agent collaboration capabilities.
// It enables task decomposition, intelligent dispatching, and result aggregation.
package collaboration

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	infraagent "link/internal/service/agent/framework"
)

// ========================================
// Task Decomposition (任务分解)
// ========================================

// SubTask represents a decomposed sub-task.
type SubTask struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Query       string                 `json:"query"`       // The actual query for the agent
	RequiredSkills []string            `json:"required_skills"` // Required agent capabilities
	Dependencies []string              `json:"dependencies"` // IDs of tasks this depends on
	Priority    int                    `json:"priority"`
	Status      TaskStatus             `json:"status"`
	Result      *SubTaskResult         `json:"result,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
}

// TaskStatus represents the status of a sub-task.
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusAssigned  TaskStatus = "assigned"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusSkipped   TaskStatus = "skipped"
)

// SubTaskResult represents the result of a sub-task.
type SubTaskResult struct {
	Content   string                 `json:"content"`
	ToolCalls []*infraagent.ToolCall      `json:"tool_calls,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Duration  time.Duration          `json:"duration"`
	CompletedAt time.Time            `json:"completed_at"`
}

// TaskPlan represents a decomposed task plan.
type TaskPlan struct {
	ID          string                 `json:"id"`
	OriginalQuery string               `json:"original_query"`
	SubTasks    []*SubTask             `json:"sub_tasks"`
	Complexity  int                    `json:"complexity"`     // 1-10
	EstimatedDuration time.Duration    `json:"estimated_duration"`
	Metadata    map[string]interface{} `json:"metadata"`
	CreatedAt   time.Time              `json:"created_at"`
}

// TaskDecomposer decomposes complex queries into sub-tasks.
type TaskDecomposer struct {
	llm    model.ChatModel
	prompt string
}

// NewTaskDecomposer creates a new task decomposer.
func NewTaskDecomposer(llm model.ChatModel) *TaskDecomposer {
	return &TaskDecomposer{
		llm:    llm,
		prompt: `You are a task planning expert. Your job is to break down complex user queries into sub-tasks.

Analyze the user query and decompose it into sub-tasks that can be executed independently or with minimal dependencies.

Return a JSON array of sub-tasks with the following structure:
[
  {
    "id": "unique_id",
    "name": "task_name",
    "description": "what this task does",
    "query": "the specific query for this task",
    "required_skills": ["skill1", "skill2"],
    "dependencies": ["id_of_dependent_task"],
    "priority": 1
  }
]

Available skills:
- "rag_search": Search knowledge base for information
- "web_search": Search the web for real-time information
- "graph_query": Query knowledge graph for relationships
- "data_analysis": Analyze data and generate insights
- "calculation": Perform calculations

Guidelines:
- Break complex queries into 2-5 sub-tasks
- Minimize dependencies between tasks
- Set priority (1=highest, 5=lowest)
- Use clear, specific queries for each task`,
	}
}

// Decompose breaks down a complex query into sub-tasks.
func (d *TaskDecomposer) Decompose(ctx context.Context, query string) (*TaskPlan, error) {
	messages := []*schema.Message{
		schema.SystemMessage(d.prompt),
		schema.UserMessage(fmt.Sprintf("Break down this query: %s", query)),
	}

	resp, err := d.llm.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to decompose task: %w", err)
	}

	// Parse the LLM response
	plan, err := d.parsePlan(resp.Content, query)
	if err != nil {
		// Fallback: create a simple single-task plan
		return d.createFallbackPlan(query), nil
	}

	return plan, nil
}

// parsePlan parses the LLM response into a TaskPlan.
func (d *TaskDecomposer) parsePlan(content, originalQuery string) (*TaskPlan, error) {
	// Try to extract JSON from the response
	// This is a simplified implementation
	// In production, use robust JSON extraction

	// For now, return a simple plan if parsing fails
	// TODO: Implement proper JSON parsing with error recovery

	return &TaskPlan{
		ID:           fmt.Sprintf("plan_%d", time.Now().Unix()),
		OriginalQuery: originalQuery,
		SubTasks: []*SubTask{
			{
				ID:          "task_1",
				Name:        "Handle Query",
				Description: "Process the user's query",
				Query:       originalQuery,
				RequiredSkills: []string{"rag_search"},
				Priority:    1,
				Status:      TaskStatusPending,
			},
		},
		Complexity:  1,
		CreatedAt:   time.Now(),
	}, nil
}

// createFallbackPlan creates a simple fallback plan.
func (d *TaskDecomposer) createFallbackPlan(query string) *TaskPlan {
	return &TaskPlan{
		ID:           fmt.Sprintf("plan_fallback_%d", time.Now().Unix()),
		OriginalQuery: query,
		SubTasks: []*SubTask{
			{
				ID:          "task_1",
				Name:        "Process Query",
				Description: "Handle the user's request",
				Query:       query,
				RequiredSkills: []string{"rag_search"},
				Priority:    1,
				Status:      TaskStatusPending,
			},
		},
		Complexity:  1,
		CreatedAt:   time.Now(),
	}
}

// ========================================
// Agent Registry (Agent 注册表)
// ========================================

// AgentCapability represents an agent's capability.
type AgentCapability struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Skills      []string `json:"skills"`
}

// AgentInfo represents information about an infraagent.
type AgentInfo struct {
	Name         string             `json:"name"`
	Description  string             `json:"description"`
	Capabilities []AgentCapability  `json:"capabilities"`
}

// AgentEntry represents a registered infraagent.
type AgentEntry struct {
	Agent       infraagent.Agent
	Capabilities []AgentCapability
	Description string
}

// AgentRegistry manages available agents and their capabilities.
type AgentRegistry struct {
	mu     sync.RWMutex
	agents map[string]*AgentEntry
}

// NewAgentRegistry creates a new agent registry.
func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		agents: make(map[string]*AgentEntry),
	}
}

// Register registers an agent with its capabilities.
func (r *AgentRegistry) Register(id string, agent infraagent.Agent, capabilities []AgentCapability, description string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.agents[id] = &AgentEntry{
		Agent:        agent,
		Capabilities: capabilities,
		Description:  description,
	}
}

// FindAgents finds agents that match the required skills.
func (r *AgentRegistry) FindAgents(requiredSkills []string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	matches := make([]string, 0)
	for id, entry := range r.agents {
		if r.matchesSkills(entry, requiredSkills) {
			matches = append(matches, id)
		}
	}
	return matches
}

// Get retrieves an agent by ID.
func (r *AgentRegistry) Get(id string) (infraagent.Agent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.agents[id]
	if !ok {
		return nil, false
	}
	return entry.Agent, true
}

// matchesSkills checks if an agent has the required skills.
func (r *AgentRegistry) matchesSkills(entry *AgentEntry, requiredSkills []string) bool {
	if len(requiredSkills) == 0 {
		return true
	}

	agentSkills := make(map[string]bool)
	for _, cap := range entry.Capabilities {
		agentSkills[cap.Name] = true
	}

	for _, skill := range requiredSkills {
		if !agentSkills[skill] {
			return false
		}
	}
	return true
}

// List returns all registered agent IDs.
func (r *AgentRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.agents))
	for id := range r.agents {
		ids = append(ids, id)
	}
	return ids
}

// GetByName retrieves an agent by name with error return.
// This is the preferred method for tool-based collaboration.
func (r *AgentRegistry) GetByName(name string) (infraagent.Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.agents[name]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", name)
	}
	return entry.Agent, nil
}

// GetDescription retrieves the description of an infraagent.
func (r *AgentRegistry) GetDescription(name string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.agents[name]
	if !ok {
		return "", fmt.Errorf("agent not found: %s", name)
	}
	return entry.Description, nil
}

// GetCapabilities retrieves the capabilities of an infraagent.
func (r *AgentRegistry) GetCapabilities(name string) ([]AgentCapability, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.agents[name]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", name)
	}
	return entry.Capabilities, nil
}

// ListWithDescriptions returns all agents with their descriptions.
func (r *AgentRegistry) ListWithDescriptions() []AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]AgentInfo, 0, len(r.agents))
	for name, entry := range r.agents {
		infos = append(infos, AgentInfo{
			Name:         name,
			Description:  entry.Description,
			Capabilities: entry.Capabilities,
		})
	}
	return infos
}

// ========================================
// Task Dispatcher (任务分发器)
// ========================================

// TaskDispatcher assigns and executes sub-tasks to agents.
type TaskDispatcher struct {
	registry  *AgentRegistry
	parallel  int // Maximum parallel tasks
}

// NewTaskDispatcher creates a new task dispatcher.
func NewTaskDispatcher(registry *AgentRegistry, parallel int) *TaskDispatcher {
	if parallel <= 0 {
		parallel = 3
	}
	return &TaskDispatcher{
		registry: registry,
		parallel: parallel,
	}
}

// Dispatch assigns tasks to agents and executes them.
func (d *TaskDispatcher) Dispatch(ctx context.Context, plan *TaskPlan) (*ExecutionResult, error) {
	result := &ExecutionResult{
		PlanID:    plan.ID,
		SubTasks:  make(map[string]*SubTaskResult),
		StartedAt: time.Now(),
	}

	// Build dependency graph and determine execution order
	tasksByOrder := d.orderTasksByDependencies(plan.SubTasks)

	// Execute tasks in order
	for _, batch := range tasksByOrder {
		// Execute tasks in this batch in parallel
		batchResults := d.executeBatch(ctx, batch, result)

		// Check if any critical tasks failed
		if d.hasCriticalFailures(batchResults) {
			result.Status = "failed"
			result.FinishedAt = time.Now()
			return result, fmt.Errorf("critical tasks failed")
		}
	}

	result.Status = "completed"
	result.FinishedAt = time.Now()
	return result, nil
}

// orderTasksByDependencies orders tasks by their dependencies (topological sort).
func (d *TaskDispatcher) orderTasksByDependencies(tasks []*SubTask) [][]*SubTask {
	// Simple implementation: separate into dependency levels
	// Level 0: No dependencies
	// Level 1: Depends on Level 0 tasks
	// etc.

	taskMap := make(map[string]*SubTask)
	for _, t := range tasks {
		taskMap[t.ID] = t
	}

	levels := make(map[int][]*SubTask)
	visited := make(map[string]bool)

	var assignLevel func(task *SubTask) int
	assignLevel = func(task *SubTask) int {
		if visited[task.ID] {
			return 0 // Already processed
		}
		visited[task.ID] = true

		if len(task.Dependencies) == 0 {
			levels[0] = append(levels[0], task)
			return 0
		}

		maxDepLevel := 0
		for _, depID := range task.Dependencies {
			if depTask, ok := taskMap[depID]; ok {
				depLevel := assignLevel(depTask)
				if depLevel > maxDepLevel {
					maxDepLevel = depLevel
				}
			}
		}

		taskLevel := maxDepLevel + 1
		levels[taskLevel] = append(levels[taskLevel], task)
		return taskLevel
	}

	for _, task := range tasks {
		if !visited[task.ID] {
			assignLevel(task)
		}
	}

	// Convert to ordered slices
	maxLevel := 0
	for level := range levels {
		if level > maxLevel {
			maxLevel = level
		}
	}

	ordered := make([][]*SubTask, maxLevel+1)
	for level := 0; level <= maxLevel; level++ {
		ordered[level] = levels[level]
	}

	return ordered
}

// executeBatch executes a batch of tasks in parallel.
func (d *TaskDispatcher) executeBatch(ctx context.Context, batch []*SubTask, result *ExecutionResult) map[string]*SubTaskResult {
	results := make(map[string]*SubTaskResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Limit parallelism
	sem := make(chan struct{}, d.parallel)

	for _, task := range batch {
		wg.Add(1)
		go func(t *SubTask) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				mu.Lock()
				results[t.ID] = &SubTaskResult{
					Error: "context cancelled",
				}
				mu.Unlock()
				return
			}

			// Find suitable agent
			agentIDs := d.registry.FindAgents(t.RequiredSkills)
			if len(agentIDs) == 0 {
				mu.Lock()
				results[t.ID] = &SubTaskResult{
					Error: fmt.Sprintf("no agent found for skills: %v", t.RequiredSkills),
				}
				mu.Unlock()
				return
			}

			// Use first matching agent
			selectedAgent, ok := d.registry.Get(agentIDs[0])
			if !ok {
				mu.Lock()
				results[t.ID] = &SubTaskResult{
					Error: "agent not found",
				}
				mu.Unlock()
				return
			}

			// Execute task
			start := time.Now()
			resp, err := selectedAgent.Chat(ctx, t.Query)
			duration := time.Since(start)

			mu.Lock()
			if err != nil {
				results[t.ID] = &SubTaskResult{
					Error:    err.Error(),
					Duration: duration,
				}
			} else {
				results[t.ID] = &SubTaskResult{
					Content:   resp.Content,
					ToolCalls: resp.ToolCalls,
					Metadata:  resp.Metadata,
					Duration:  duration,
				}
			}
			mu.Unlock()

			// Update task status
			t.Status = TaskStatusCompleted
			t.Result = results[t.ID]

		}(task)
	}

	wg.Wait()

	// Store results in execution result
	mu.Lock()
	for id, r := range results {
		result.SubTasks[id] = r
		result.TotalTasks++
		if r.Error != "" {
			result.FailedTasks++
		}
	}
	mu.Unlock()

	return results
}

// hasCriticalFailures checks if any critical tasks failed.
func (d *TaskDispatcher) hasCriticalFailures(results map[string]*SubTaskResult) bool {
	for _, r := range results {
		if r.Error != "" {
			// For now, treat all failures as critical
			// Could be refined based on task priority
			return true
		}
	}
	return false
}

// ========================================
// Execution Result (执行结果)
// ========================================

// ExecutionResult represents the result of a collaborative execution.
type ExecutionResult struct {
	PlanID      string                 `json:"plan_id"`
	Status      string                 `json:"status"` // completed, failed, partial
	SubTasks    map[string]*SubTaskResult `json:"sub_tasks"`
	TotalTasks  int                    `json:"total_tasks"`
	FailedTasks int                    `json:"failed_tasks"`
	StartedAt   time.Time              `json:"started_at"`
	FinishedAt  time.Time              `json:"finished_at"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// GetSucceededResults returns all succeeded task results.
func (r *ExecutionResult) GetSucceededResults() []*SubTaskResult {
	results := make([]*SubTaskResult, 0)
	for _, result := range r.SubTasks {
		if result.Error == "" {
			results = append(results, result)
		}
	}
	return results
}

// GetFailedResults returns all failed task results.
func (r *ExecutionResult) GetFailedResults() []*SubTaskResult {
	results := make([]*SubTaskResult, 0)
	for _, result := range r.SubTasks {
		if result.Error != "" {
			results = append(results, result)
		}
	}
	return results
}
