// Package agent provides Agent infrastructure implementations.
// This package contains concrete implementations of domain interfaces using
// the Cloudwego Eino framework.
package framework

import (
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
)

// ========================================
// BaseAgent - Eino Agent 基础实现
// ========================================

// BaseAgent provides a basic Agent implementation using Eino framework components.
// It wraps an Eino ChatModel and a set of tools that the model can call.
type BaseAgent struct {
	name        string
	description string
	chatModel   model.ToolCallingChatModel
	tools       []tool.BaseTool
}

// NewBaseAgent creates a new BaseAgent instance.
func NewBaseAgent(name, description string, chatModel model.ToolCallingChatModel, tools []tool.BaseTool) *BaseAgent {
	return &BaseAgent{
		name:        name,
		description: description,
		chatModel:   chatModel,
		tools:       tools,
	}
}

// Name returns the agent's name.
func (a *BaseAgent) Name() string {
	return a.name
}

// Description returns the agent's description.
func (a *BaseAgent) Description() string {
	return a.description
}

// GetTools returns the list of tools available to this agent.
func (a *BaseAgent) GetTools() []tool.BaseTool {
	return a.tools
}

// GetChatModel returns the Eino chat model used by this agent.
func (a *BaseAgent) GetChatModel() model.ToolCallingChatModel {
	return a.chatModel
}

// ========================================
// 辅助函数
// ========================================

// ShouldUseTool checks if tools are available for the agent to use.
// This is a legacy function kept for compatibility.
func ShouldUseTool(content string, tools []tool.BaseTool) bool {
	// This function is now just an example; actual tool selection is done by the LLM
	return len(tools) > 0
}
