// Package agent provides collaboration registry for multi-agent runtime management.
package framework

import (
	"fmt"
	"sync"
)

// ========================================
// Collaboration Registry (多 Agent 协作注册表)
// ========================================

// AgentCapability represents an agent's capability.
type AgentCapability struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Skills      []string `json:"skills"`
}

// AgentInfo represents information about an agent.
type AgentInfo struct {
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Capabilities []AgentCapability `json:"capabilities"`
}

// AgentEntry represents a registered agent.
type AgentEntry struct {
	Agent        Agent
	Capabilities []AgentCapability
	Description  string
}

// CollaborationRegistry manages available agents and their capabilities for multi-agent collaboration.
type CollaborationRegistry struct {
	mu     sync.RWMutex
	agents map[string]*AgentEntry
}

// NewCollaborationRegistry creates a new collaboration registry.
func NewCollaborationRegistry() *CollaborationRegistry {
	return &CollaborationRegistry{
		agents: make(map[string]*AgentEntry),
	}
}

// Register registers an agent with its capabilities.
func (r *CollaborationRegistry) Register(id string, agent Agent, capabilities []AgentCapability, description string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.agents[id] = &AgentEntry{
		Agent:        agent,
		Capabilities: capabilities,
		Description:  description,
	}
}

// FindAgents finds agents that match the required skills.
func (r *CollaborationRegistry) FindAgents(requiredSkills []string) []string {
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
func (r *CollaborationRegistry) Get(id string) (Agent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.agents[id]
	if !ok {
		return nil, false
	}
	return entry.Agent, true
}

// matchesSkills checks if an agent has the required skills.
func (r *CollaborationRegistry) matchesSkills(entry *AgentEntry, requiredSkills []string) bool {
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
func (r *CollaborationRegistry) List() []string {
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
func (r *CollaborationRegistry) GetByName(name string) (Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.agents[name]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", name)
	}
	return entry.Agent, nil
}

// GetDescription retrieves the description of an agent.
func (r *CollaborationRegistry) GetDescription(name string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.agents[name]
	if !ok {
		return "", fmt.Errorf("agent not found: %s", name)
	}
	return entry.Description, nil
}

// GetCapabilities retrieves the capabilities of an agent.
func (r *CollaborationRegistry) GetCapabilities(name string) ([]AgentCapability, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.agents[name]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", name)
	}
	return entry.Capabilities, nil
}

// ListWithDescriptions returns all agents with their descriptions.
func (r *CollaborationRegistry) ListWithDescriptions() []AgentInfo {
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
