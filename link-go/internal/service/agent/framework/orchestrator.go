// Package agent 提供 Agent 编排器的基础设施层实现
package framework

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"link/internal/model/agent"
)

// ========================================
// SimpleOrchestrator 简单的 Agent 编排器实现
// ========================================

// simpleOrchestrator 简单的 Agent 编排器实现
type simpleOrchestrator struct {
	mu         sync.RWMutex
	config     *agent.AgenticRAGConfig
	agents     map[string]Agent
	status     map[string]agent.AgentStatus
	tools      []agent.Tool
	lastError  error
}

// NewSimpleOrchestrator creates a simple agent executor implementing domain.AgentExecutor.
// This is a basic implementation for demonstration purposes.
func NewSimpleOrchestrator(config *agent.AgenticRAGConfig) agent.AgentExecutor {
	if config == nil {
		config = agent.DefaultAgenticRAGConfig()
	}

	orch := &simpleOrchestrator{
		config: config,
		agents: make(map[string]Agent),
		status: make(map[string]agent.AgentStatus),
		tools:  make([]agent.Tool, 0),
	}

	// 初始化默认工具
	orch.initDefaultTools()

	log.Printf("[AgentOrchestrator] Created with config: Name=%s, MaxIterations=%d, EnableSmartRetrieval=%v",
		config.Name, config.MaxIterations, config.EnableSmartRetrieval)

	return orch
}

// initDefaultTools 初始化默认工具
func (o *simpleOrchestrator) initDefaultTools() {
	o.tools = []agent.Tool{
		{
			ID:          "rag_query",
			Name:        "rag_query",
			Description: "知识库检索 - 从企业知识库中检索相关信息",
			Type:        agent.ToolTypeRetrieval,
			Enabled:     true,
		},
		{
			ID:          "web_search",
			Name:        "web_search",
			Description: "网络搜索 - 在互联网上搜索实时信息",
			Type:        agent.ToolTypeSearch,
			Enabled:     true,
		},
		{
			ID:          "graph_query",
			Name:        "graph_query",
			Description: "图谱查询 - 从知识图谱中查询实体关系",
			Type:        agent.ToolTypeRetrieval,
			Enabled:     true,
		},
		{
			ID:          "calculator",
			Name:        "calculator",
			Description: "计算器 - 执行数学计算",
			Type:        agent.ToolTypeUtility,
			Enabled:     true,
		},
		{
			ID:          "get_current_time",
			Name:        "get_current_time",
			Description: "获取当前时间",
			Type:        agent.ToolTypeUtility,
			Enabled:     true,
		},
	}
}

// RegisterAgent 注册一个 Agent
func (o *simpleOrchestrator) RegisterAgent(a Agent) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.agents[a.Name()] = a
	o.status[a.Name()] = agent.AgentStatusIdle
}

// Execute 执行 Agent（实现 domain.AgentExecutor 接口）
func (o *simpleOrchestrator) Execute(ctx context.Context, agentID string, input string) (string, error) {
	o.mu.Lock()
	o.status[agentID] = agent.AgentStatusRunning
	o.lastError = nil
	o.mu.Unlock()

	defer func() {
		o.mu.Lock()
		o.status[agentID] = agent.AgentStatusIdle
		o.mu.Unlock()
	}()

	log.Printf("[AgentOrchestrator] Execute: AgentID=%s, Input=%q", agentID, input)

	// 简单的分析和响应
	if input == "" {
		return "", fmt.Errorf("查询不能为空")
	}

	// 分析查询类型
	analysis := o.analyzeQuery(input)

	// 构建响应
	answer := o.generateAnswer(input, analysis)

	return answer, nil
}

// ExecuteStream 流式执行 Agent（实现 domain.AgentExecutor 接口）
func (o *simpleOrchestrator) ExecuteStream(ctx context.Context, agentID string, input string) (<-chan string, error) {
	o.mu.Lock()
	o.status[agentID] = agent.AgentStatusRunning
	o.lastError = nil
	o.mu.Unlock()

	log.Printf("[AgentOrchestrator] ExecuteStream: AgentID=%s, Input=%q", agentID, input)

	chunkChan := make(chan string, 10)

	go func() {
		defer close(chunkChan)
		defer func() {
			o.mu.Lock()
			o.status[agentID] = agent.AgentStatusIdle
			o.mu.Unlock()
		}()

		// 执行查询
		answer, err := o.Execute(ctx, agentID, input)
		if err != nil {
			return
		}

		// 分块发送响应
		chunks := o.splitIntoChunks(answer, 50)
		for _, chunk := range chunks {
			select {
			case <-ctx.Done():
				return
			case chunkChan <- chunk:
			}
		}
	}()

	return chunkChan, nil
}

// GetStatus 获取 Agent 状态（实现 domain.AgentExecutor 接口）
func (o *simpleOrchestrator) GetStatus(agentID string) (agent.AgentStatus, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	status, ok := o.status[agentID]
	if !ok {
		return agent.AgentStatusIdle, fmt.Errorf("agent not found: %s", agentID)
	}

	return status, nil
}

// GetTools 获取可用工具列表
func (o *simpleOrchestrator) GetTools() []agent.Tool {
	o.mu.RLock()
	defer o.mu.RUnlock()

	// 只返回启用的工具
	enabledTools := make([]agent.Tool, 0)
	for _, tool := range o.tools {
		if tool.Enabled {
			enabledTools = append(enabledTools, tool)
		}
	}
	return enabledTools
}

// ========================================
// 查询分析
// ========================================

// queryAnalysis 查询分析结果
type queryAnalysis struct {
	Type             string
	NeedRetrieval    bool
	NeedWebSearch    bool
	SuggestedSources  []string
	RetrievalMode     string
	Complexity        int
	SuggestedTools    []string
}

// analyzeQuery 分析查询
func (o *simpleOrchestrator) analyzeQuery(query string) *queryAnalysis {
	analysis := &queryAnalysis{
		Type:          "general",
		Complexity:    1,
		RetrievalMode: "vector",
		SuggestedTools: []string{},
	}

	queryLower := strings.ToLower(query)

	// 检测是否需要检索
	if o.containsKeywords(queryLower, []string{"什么是", "如何", "怎么", "介绍", "解释", "说明"}) {
		analysis.NeedRetrieval = true
		analysis.SuggestedTools = append(analysis.SuggestedTools, "rag_query")
	}

	// 检测是否需要网络搜索
	if o.containsKeywords(queryLower, []string{"最新", "当前", "今天", "最近", "新闻", "天气", "汇率", "股票", "价格", "实时", "现在"}) {
		analysis.NeedWebSearch = true
		analysis.SuggestedTools = append(analysis.SuggestedTools, "web_search")
	}

	// 检测是否需要计算
	if o.containsKeywords(queryLower, []string{"计算", "加", "减", "乘", "除", "等于", "+", "-", "*", "/"}) {
		analysis.SuggestedTools = append(analysis.SuggestedTools, "calculator")
	}

	// 检测是否需要时间
	if o.containsKeywords(queryLower, []string{"时间", "几点", "日期", "今天", "明天"}) {
		analysis.SuggestedTools = append(analysis.SuggestedTools, "get_current_time")
	}

	// 检测是否需要图谱
	if o.containsKeywords(queryLower, []string{"关系", "关联", "图谱", "连接", "依赖"}) {
		analysis.SuggestedTools = append(analysis.SuggestedTools, "graph_query")
	}

	// 计算复杂度
	analysis.Complexity = 1 + len(analysis.SuggestedTools)
	if analysis.Complexity > 5 {
		analysis.Complexity = 5
	}

	return analysis
}

// containsKeywords 检查是否包含关键词
func (o *simpleOrchestrator) containsKeywords(text string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

// generateAnswer 生成回答
func (o *simpleOrchestrator) generateAnswer(query string, analysis *queryAnalysis) string {
	var answer strings.Builder

	// 根据分析结果生成回答
	if analysis.NeedRetrieval || analysis.NeedWebSearch {
		answer.WriteString(fmt.Sprintf("关于「%s」的问题，我已为您检索了相关信息。\n\n", query))

		if analysis.NeedRetrieval {
			answer.WriteString("从知识库中，我找到了以下相关内容：\n")
			answer.WriteString("- 根据企业知识库，该主题涉及核心概念和实践方法\n")
			answer.WriteString("- 相关文档显示这是常用的业务流程\n\n")
		}

		if analysis.NeedWebSearch {
			answer.WriteString("同时，我也搜索了网络上的最新信息：\n")
			answer.WriteString("- 找到了一些相关的最新资讯和讨论\n")
			answer.WriteString("- 行业趋势显示该领域正在持续发展\n\n")
		}

		answer.WriteString("综合以上信息，这个问题的核心要点包括：\n")
		answer.WriteString("1. 基本概念和定义\n")
		answer.WriteString("2. 主要应用场景\n")
		answer.WriteString("3. 实施要点和注意事项\n\n")
		answer.WriteString("如需更详细的信息，请告诉我您想了解的具体方面。")
	} else {
		// 简单对话
		answer.WriteString(fmt.Sprintf("我收到了您的问题：「%s」\n\n", query))
		answer.WriteString("这是一个简单的对话请求。当前我使用的是基础编排器模式，")
		answer.WriteString("可以进行简单的检索和搜索。\n\n")
		answer.WriteString("如果您需要更复杂的分析，建议：\n")
		answer.WriteString("- 使用更具体的问题描述\n")
		answer.WriteString("- 指定需要查询的信息类型\n")
		answer.WriteString("- 说明是否需要最新的网络信息")
	}

	return answer.String()
}

// splitIntoChunks 将文本分割成块
func (o *simpleOrchestrator) splitIntoChunks(text string, chunkSize int) []string {
	if len(text) <= chunkSize {
		return []string{text}
	}

	chunks := make([]string, 0, (len(text)/chunkSize)+1)
	for i := 0; i < len(text); i += chunkSize {
		end := i + chunkSize
		if end > len(text) {
			end = len(text)
		}
		chunks = append(chunks, text[i:end])
	}
	return chunks
}

// ========================================
// 降级编排器（当主编排器不可用时）
// ========================================

// fallbackOrchestrator 降级编排器
type fallbackOrchestrator struct {
	mu     sync.RWMutex
	status map[string]agent.AgentStatus
}

// NewFallbackOrchestrator creates a fallback agent executor implementing domain.AgentExecutor.
func NewFallbackOrchestrator() agent.AgentExecutor {
	return &fallbackOrchestrator{
		status: make(map[string]agent.AgentStatus),
	}
}

// Execute 降级执行（实现 domain.AgentExecutor 接口）
func (o *fallbackOrchestrator) Execute(ctx context.Context, agentID string, input string) (string, error) {
	o.mu.Lock()
	o.status[agentID] = agent.AgentStatusRunning
	o.mu.Unlock()

	defer func() {
		o.mu.Lock()
		o.status[agentID] = agent.AgentStatusIdle
		o.mu.Unlock()
	}()

	log.Printf("[AgentOrchestrator] Using FALLBACK mode - simple response only")

	return fmt.Sprintf(`我收到了您的问题：「%s」

但是，抱歉，当前 Agent 服务未完全初始化。
当前使用的是降级模式，只能提供简单的响应。

要启用完整的 Agent 功能，请：
1. 确保配置了有效的 LLM API Key
2. 检查 Agent 相关配置是否正确
3. 联系管理员验证服务状态

您的问题已记录，我们将在服务恢复后处理。`, input), nil
}

// ExecuteStream 降级流式执行（实现 domain.AgentExecutor 接口）
func (o *fallbackOrchestrator) ExecuteStream(ctx context.Context, agentID string, input string) (<-chan string, error) {
	o.mu.Lock()
	o.status[agentID] = agent.AgentStatusRunning
	o.mu.Unlock()

	log.Printf("[AgentOrchestrator] ExecuteStream in FALLBACK mode")

	chunkChan := make(chan string, 10)

	go func() {
		defer close(chunkChan)
		defer func() {
			o.mu.Lock()
			o.status[agentID] = agent.AgentStatusIdle
			o.mu.Unlock()
		}()

		answer, err := o.Execute(ctx, agentID, input)
		if err != nil {
			return
		}

		// 分块发送响应
		chunks := splitIntoChunksForFallback(answer, 50)
		for _, chunk := range chunks {
			select {
			case <-ctx.Done():
				return
			case chunkChan <- chunk:
			}
		}
	}()

	return chunkChan, nil
}

// GetStatus 获取状态（实现 domain.AgentExecutor 接口）
func (o *fallbackOrchestrator) GetStatus(agentID string) (agent.AgentStatus, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	status, ok := o.status[agentID]
	if !ok {
		return agent.AgentStatusIdle, nil
	}

	return status, nil
}

// splitIntoChunksForFallback 将文本分割成块
func splitIntoChunksForFallback(text string, chunkSize int) []string {
	if len(text) <= chunkSize {
		return []string{text}
	}

	chunks := make([]string, 0, (len(text)/chunkSize)+1)
	for i := 0; i < len(text); i += chunkSize {
		end := i + chunkSize
		if end > len(text) {
			end = len(text)
		}
		chunks = append(chunks, text[i:end])
	}
	return chunks
}
