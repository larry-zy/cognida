// Package agent provides Agent service implementations
package agent

import (
	"context"
	"fmt"

	"cognida/internal/model/agent"
	domainservices "cognida/internal/model/services"
	"cognida/internal/pkg/safego"
)

// ========================================
// Agent Execute Service
// ========================================

// ExecuteService implements agent execution
type ExecuteService struct {
	executor agent.AgentExecutor
}

// NewExecuteService creates a new execute service
func NewExecuteService(executor agent.AgentExecutor) *ExecuteService {
	return &ExecuteService{
		executor: executor,
	}
}

// SetExecutor sets the agent executor
func (s *ExecuteService) SetExecutor(executor agent.AgentExecutor) {
	s.executor = executor
}

// Execute executes an agent request
func (s *ExecuteService) Execute(ctx context.Context, req *AgenticRAGRequest) (*AgenticRAGResponse, error) {
	if s.executor == nil {
		return &AgenticRAGResponse{
			Success: false,
			Error:   "Agent执行器未初始化",
		}, nil
	}

	// 使用请求中的 agent ID，默认为 "default"
	agentID := req.AgentID
	if agentID == "" {
		agentID = "default"
	}

	// 调用 Domain 层接口（简化版本）
	answer, err := s.executor.Execute(ctx, agentID, req.Query)
	if err != nil {
		return &AgenticRAGResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &AgenticRAGResponse{
		Answer:  answer,
		Success: true,
	}, nil
}

// ExecuteStream executes an agent request with streaming response
func (s *ExecuteService) ExecuteStream(ctx context.Context, req *AgenticRAGRequest) (<-chan *ChatChunkDTO, error) {
	if s.executor == nil {
		return nil, fmt.Errorf("Agent执行器未初始化")
	}

	// 使用请求中的 agent ID，默认为 "default"
	agentID := req.AgentID
	if agentID == "" {
		agentID = "default"
	}

	// 调用 Domain 层流式接口
	domainChan, err := s.executor.ExecuteStream(ctx, agentID, req.Query)
	if err != nil {
		return nil, err
	}

	// 转换为 DTO channel。
	// content 事件仅携带文本；工具调用/结果/错误等结构化事件写入 Metadata，
	// 供 handler 按 SSE step 契约下发前端渲染时间线。
	resultChan := make(chan *ChatChunkDTO, 10)
	go func() {
		defer safego.Recover("agent-run")
		defer close(resultChan)
		// send 遵守 ctx 取消，避免下游提前退出时 goroutine 阻塞泄漏
		send := func(dto *ChatChunkDTO) bool {
			select {
			case <-ctx.Done():
				return false
			case resultChan <- dto:
				return true
			}
		}
		for ev := range domainChan {
			if ev == nil {
				continue
			}
			if ev.Type == agent.StreamEventContent {
				if !send(&ChatChunkDTO{Content: ev.Content}) {
					return
				}
				continue
			}
			// 结构化事件（tool_call / tool_result / error）
			if !send(&ChatChunkDTO{
				Metadata: map[string]interface{}{
					"type":        string(ev.Type),
					"step":        ev.Step,
					"tool_id":     ev.ToolID,
					"tool_name":   ev.ToolName,
					"tool_input":  ev.ToolInput,
					"tool_output": ev.Output,
					"status":      ev.Status,
					"error":       ev.Error,
				},
			}) {
				return
			}
		}
		// 发送完成标记
		send(&ChatChunkDTO{Done: true})
	}()

	return resultChan, nil
}

// GetTools returns available tools
func (s *ExecuteService) GetTools(ctx context.Context) ([]agent.Tool, error) {
	// Note: AgentExecutor doesn't expose tools directly
	// This would need to be added to the interface or use a different approach
	return nil, fmt.Errorf("getting tools from executor not yet implemented")
}

// GetStatus returns the current agent status
func (s *ExecuteService) GetStatus(ctx context.Context) agent.AgentStatus {
	if s.executor == nil {
		return agent.AgentStatusIdle
	}

	status, _ := s.executor.GetStatus("default")
	return status
}

// ========================================
// Config Service
// ========================================

// ConfigService manages agent configuration
type ConfigService struct {
	agenticRAGConfig   *agent.AgenticRAGConfig
	deepResearchConfig *agent.DeepResearchConfig
}

// NewConfigService creates a new config service
func NewConfigService() *ConfigService {
	return &ConfigService{
		agenticRAGConfig:   agent.DefaultAgenticRAGConfig(),
		deepResearchConfig: agent.DefaultDeepResearchConfig(),
	}
}

// GetAgenticRAGConfig returns the AgenticRAG configuration
func (s *ConfigService) GetAgenticRAGConfig() *agent.AgenticRAGConfig {
	return s.agenticRAGConfig
}

// GetDeepResearchConfig returns the DeepResearch configuration
func (s *ConfigService) GetDeepResearchConfig() *agent.DeepResearchConfig {
	return s.deepResearchConfig
}

// UpdateAgenticRAGConfig updates the AgenticRAG configuration
func (s *ConfigService) UpdateAgenticRAGConfig(config *agent.AgenticRAGConfig) {
	if config != nil {
		s.agenticRAGConfig = config
	}
}

// UpdateDeepResearchConfig updates the DeepResearch configuration
func (s *ConfigService) UpdateDeepResearchConfig(config *agent.DeepResearchConfig) {
	if config != nil {
		s.deepResearchConfig = config
	}
}

// ========================================
// Progress Service
// ========================================

// ProgressService manages progress tracking
type ProgressService struct {
	// Progress store (in production, use Redis)
	progressStore map[string]*SessionProgress
}

// NewProgressService creates a new progress service
func NewProgressService() *ProgressService {
	return &ProgressService{
		progressStore: make(map[string]*SessionProgress),
	}
}

// GetProgress retrieves progress for a session
func (s *ProgressService) GetProgress(ctx context.Context, sessionID string) (*SessionProgress, error) {
	if progress, ok := s.progressStore[sessionID]; ok {
		return progress, nil
	}
	// Return default progress
	return &SessionProgress{
		SessionID:   sessionID,
		Stage:       "idle",
		Progress:    0.0,
		Detail:      "会话未开始或已过期",
		IsCompleted: false,
	}, nil
}

// SetProgress sets progress for a session (internal use)
func (s *ProgressService) SetProgress(sessionID string, progress *SessionProgress) {
	s.progressStore[sessionID] = progress
}

// ConvertToSSEEvent converts progress to SSE event format
func (s *ProgressService) ConvertToSSEEvent(progress *ResearchProgressDTO) map[string]interface{} {
	return map[string]interface{}{
		"type":          getEventType(progress.Stage),
		"stage":         progress.Stage,
		"progress":      progress.Progress,
		"detail":        progress.Detail,
		"sub_stage":     progress.SubStage,
		"total_steps":   progress.TotalSteps,
		"current_step":  progress.CurrentStep,
		"elapsed_ms":    progress.ElapsedTime,
		"results_cnt":   progress.ResultsCount,
		"sub_questions": progress.SubQuestions,
		"sources":       progress.Sources,
		"error":         progress.Error,
	}
}

// CalculateSimilarity calculates similarity between two texts
func (s *ProgressService) CalculateSimilarity(ctx context.Context, req *SimilarityRequest) (*SimilarityResponse, error) {
	method := req.Method
	if method == "" {
		method = "jaccard"
	}

	// Use domain service for similarity calculation
	similarity := domainservices.CalculateSimilarity(req.Text1, req.Text2, domainservices.SimilarityMethod(method))

	return &SimilarityResponse{
		Similarity: similarity,
		Method:     method,
	}, nil
}

// getEventType returns the event type for a given stage
func getEventType(stage string) string {
	switch stage {
	case "decomposing":
		return "decompose"
	case "searching":
		return "search"
	case "deduplicating":
		return "deduplicate"
	case "expanding":
		return "expand"
	case "synthesizing":
		return "synthesize"
	case "complete":
		return "done"
	case "error":
		return "error"
	default:
		return "progress"
	}
}
