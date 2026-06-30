// Package agent provides Agent research service implementations
package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"link/internal/model/agent"
)

// ========================================
// Deep Research Service
// ========================================

// ResearchService implements deep research
type ResearchService struct {
	executor agent.AgentExecutor
	config   *agent.DeepResearchConfig
}

// NewResearchService creates a new research service
func NewResearchService(executor agent.AgentExecutor, config *agent.DeepResearchConfig) *ResearchService {
	if config == nil {
		config = agent.DefaultDeepResearchConfig()
	}
	return &ResearchService{
		executor: executor,
		config:   config,
	}
}

// Execute executes a deep research request
func (s *ResearchService) Execute(ctx context.Context, req *DeepResearchRequest) (*DeepResearchResponse, error) {
	startTime := time.Now()

	if s.executor == nil {
		return &DeepResearchResponse{
			Success: false,
			Error:   "Agent执行器未初始化",
		}, nil
	}

	// 使用固定的 agent ID（实际应用中可以从请求或配置中获取）
	agentID := "deep_research"

	// 调用 Domain 层接口
	answer, err := s.executor.Execute(ctx, agentID, req.Query)
	if err != nil {
		return &DeepResearchResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	// 构建响应
	return &DeepResearchResponse{
		Query:            req.Query,
		ExecutiveSummary: extractSummary(answer),
		DetailedAnalysis:  answer,
		KeyFindings:      extractFindings(answer),
		Sources:          []ReportSourceDTO{}, // 简化版本
		Metadata: ReportMetadataDTO{
			SubQuestionCount: s.config.MaxSubQuestions,
			TotalSources:     0,
			ExecutionTimeMs:  time.Since(startTime).Milliseconds(),
			Timestamp:        time.Now().Unix(),
		},
		Success: true,
	}, nil
}

// ExecuteStreamWithProgress executes a deep research request with progress streaming
func (s *ResearchService) ExecuteStreamWithProgress(ctx context.Context, req *DeepResearchRequest) (<-chan *ResearchProgressDTO, error) {
	progressChan := make(chan *ResearchProgressDTO, 50)

	go func() {
		defer close(progressChan)

		startTime := time.Now()

		// Send start event
		sendProgress(progressChan, &ResearchProgressDTO{
			Stage:       "decomposing",
			Progress:    0.0,
			Detail:      fmt.Sprintf("开始研究: %s", truncate(req.Query, 50)),
			ElapsedTime: time.Since(startTime).Milliseconds(),
			Timestamp:   time.Now().Unix(),
		})

		// Execute research
		_, err := s.Execute(ctx, req)

		if err != nil {
			sendProgress(progressChan, &ResearchProgressDTO{
				Stage:       "error",
				Progress:    0.0,
				Detail:      err.Error(),
				Error:       err.Error(),
				ElapsedTime: time.Since(startTime).Milliseconds(),
				Timestamp:   time.Now().Unix(),
			})
			return
		}

		// Send analysis event
		sendProgress(progressChan, &ResearchProgressDTO{
			Stage:       "analyzing",
			Progress:    0.5,
			Detail:      "分析研究结果",
			ElapsedTime: time.Since(startTime).Milliseconds(),
			Timestamp:   time.Now().Unix(),
		})

		// Send complete event
		sendProgress(progressChan, &ResearchProgressDTO{
			Stage:         "complete",
			Progress:      1.0,
			Detail:        "研究完成",
			ElapsedTime:   time.Since(startTime).Milliseconds(),
			Timestamp:     time.Now().Unix(),
			ResultsCount:  1,
		})
	}()

	return progressChan, nil
}

// ========================================
// Helper functions
// ========================================

func sendProgress(ch chan<- *ResearchProgressDTO, progress *ResearchProgressDTO) {
	select {
	case ch <- progress:
	default:
		// Channel full, drop event
	}
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

func extractSummary(answer string) string {
	runes := []rune(answer)
	if len(runes) <= 200 {
		return answer
	}
	return string(runes[:200]) + "..."
}

func extractFindings(answer string) []string {
	findings := []string{}
	lines := strings.Split(answer, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) > 0 && (strings.HasPrefix(line, "-") || strings.HasPrefix(line, "*") || strings.HasPrefix(line, "•")) {
			findings = append(findings, strings.TrimLeft(line, "-* •"))
		}
		if len(findings) >= 5 {
			break
		}
	}
	return findings
}
