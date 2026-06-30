// Package llm provides message conversion utilities between LLM and persistence layers
package chat

import (
	"encoding/json"

	"link/internal/model/conversation"
	"link/internal/model/llm"
)

// ========================================
// Message Type Conversion
// ========================================

// ToLLMMessages converts conversation.Messages to llm.Messages for LLM calling
func ToLLMMessages(convMsgs []*conversation.Message) []*llm.Message {
	result := make([]*llm.Message, len(convMsgs))
	for i, m := range convMsgs {
		result[i] = &llm.Message{
			Role:    m.Role,
			Content: m.Content,
			ToolID:  m.ID,
		}

		// Parse tool calls if present
		if m.ToolCalls != "" {
			var toolCalls []*llm.ToolCall
			if err := json.Unmarshal([]byte(m.ToolCalls), &toolCalls); err == nil {
				result[i].ToolCalls = toolCalls
			}
		}

		// Parse metadata from KnowledgeReferences and AgentSteps
		if m.KnowledgeReferences != "" || m.AgentSteps != "" {
			metadata := make(map[string]interface{})
			if m.KnowledgeReferences != "" {
				var knowledgeRefs []interface{}
				if err := json.Unmarshal([]byte(m.KnowledgeReferences), &knowledgeRefs); err == nil {
					metadata["knowledge_references"] = knowledgeRefs
				}
			}
			if m.AgentSteps != "" {
				var agentSteps []interface{}
				if err := json.Unmarshal([]byte(m.AgentSteps), &agentSteps); err == nil {
					metadata["agent_steps"] = agentSteps
				}
			}
			result[i].Metadata = metadata
		}
	}
	return result
}

// ToConversationMessage converts llm.Message to conversation.Message for persistence
func ToConversationMessage(llmMsg *llm.Message, sessionID, requestID string) *conversation.Message {
	convMsg := &conversation.Message{
		SessionID:   sessionID,
		RequestID:   requestID,
		Role:        llmMsg.Role,
		Content:     llmMsg.Content,
		IsCompleted: true,
		TokenCount:  0, // Will be set from Usage if available
	}

	// Serialize tool calls if present
	if len(llmMsg.ToolCalls) > 0 {
		toolCallsJSON, err := json.Marshal(llmMsg.ToolCalls)
		if err == nil {
			convMsg.ToolCalls = string(toolCallsJSON)
		}
	}

	// Extract metadata to KnowledgeReferences and AgentSteps
	if llmMsg.Metadata != nil {
		if refs, ok := llmMsg.Metadata["knowledge_references"]; ok {
			refsJSON, err := json.Marshal(refs)
			if err == nil {
				convMsg.KnowledgeReferences = string(refsJSON)
			}
		}
		if steps, ok := llmMsg.Metadata["agent_steps"]; ok {
			stepsJSON, err := json.Marshal(steps)
			if err == nil {
				convMsg.AgentSteps = string(stepsJSON)
			}
		}
	}

	return convMsg
}

// ToConversationMessages converts llm.Messages to conversation.Messages for persistence
func ToConversationMessages(llmMsgs []*llm.Message, sessionID, requestID string) []*conversation.Message {
	result := make([]*conversation.Message, len(llmMsgs))
	for i, msg := range llmMsgs {
		result[i] = ToConversationMessage(msg, sessionID, requestID)
	}
	return result
}

// StringifyToolCalls converts llm.ToolCall slice to JSON string
func StringifyToolCalls(toolCalls []*llm.ToolCall) string {
	if len(toolCalls) == 0 {
		return ""
	}
	data, err := json.Marshal(toolCalls)
	if err != nil {
		return ""
	}
	return string(data)
}

// ParseToolCalls parses JSON string to llm.ToolCall slice
func ParseToolCalls(toolCallsStr string) []*llm.ToolCall {
	if toolCallsStr == "" {
		return nil
	}
	var toolCalls []*llm.ToolCall
	if err := json.Unmarshal([]byte(toolCallsStr), &toolCalls); err != nil {
		return nil
	}
	return toolCalls
}
