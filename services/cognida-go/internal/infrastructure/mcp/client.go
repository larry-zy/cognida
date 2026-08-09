// Package mcp 提供 MCP 协议客户端实现（备用）
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	agentctx "cognida/internal/model/agent"
	"cognida/internal/model/agent/tools"
)

// HTTPRequestIDHeader 出站 MCP 请求携带 request_id 的 HTTP 头名。
// 与 Python 侧 core.request_context.HTTP_HEADER 保持一致，实现跨进程链路追踪。
const HTTPRequestIDHeader = "X-Request-ID"

// MCPClient MCP 协议的 Skill 客户端实现
type MCPClient struct {
	endpoint string        // Python MCP Server 端点
	timeout  time.Duration // 默认超时
	logger   *slog.Logger
	client   *MCPHTTPClient // HTTP 传输层客户端

	mu              sync.RWMutex
	skillsCache     []tools.SkillInfo // Skill 信息缓存
	cacheExpiry     time.Time         // 缓存过期时间
	cacheTTL        time.Duration     // 缓存有效期
	connected       bool              // 连接状态
	reconnectConfig *ReconnectConfig  // 重连配置
}

// ReconnectConfig 重连配置
type ReconnectConfig struct {
	MaxRetries        int           // 最大重试次数
	InitialDelay      time.Duration // 初始延迟
	MaxDelay          time.Duration // 最大延迟
	BackoffMultiplier float64       // 退避倍数
}

// DefaultReconnectConfig 默认重连配置
func DefaultReconnectConfig() *ReconnectConfig {
	return &ReconnectConfig{
		MaxRetries:        3,
		InitialDelay:      100 * time.Millisecond,
		MaxDelay:          5 * time.Second,
		BackoffMultiplier: 2.0,
	}
}

// MCPHTTPClient HTTP MCP 客户端
type MCPHTTPClient struct {
	baseURL    string
	httpClient HTTPClient
}

// HTTPClient 简化的 HTTP 客户端接口
type HTTPClient interface {
	Post(url string, body []byte) ([]byte, error)
}

// HeaderPoster 可选能力：在 POST 时附带自定义请求头（用于透传 X-Request-ID 等链路头）。
// 传输层实现该接口即可承载 request_id；未实现时 MCPClient 回退到无头 Post，保持向后兼容。
type HeaderPoster interface {
	PostWithHeaders(url string, body []byte, headers map[string]string) ([]byte, error)
}

// DefaultHTTPClient 默认 HTTP 客户端实现
type DefaultHTTPClient struct {
	client *http.Client
}

// NewDefaultHTTPClient 创建默认 HTTP 客户端
func NewDefaultHTTPClient(timeout time.Duration) *DefaultHTTPClient {
	return &DefaultHTTPClient{
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Post 发送 HTTP POST 请求
func (c *DefaultHTTPClient) Post(url string, body []byte) ([]byte, error) {
	resp, err := c.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("http post failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// PostWithHeaders 发送带自定义请求头的 HTTP POST 请求（实现 HeaderPoster）。
func (c *DefaultHTTPClient) PostWithHeaders(url string, body []byte, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to build http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http post failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// Config MCP 客户端配置
type Config struct {
	Endpoint   string        // MCP Server 端点
	Timeout    time.Duration // 请求超时
	CacheTTL   time.Duration // 缓存有效期
	Logger     *slog.Logger
	HTTPClient HTTPClient // 可选的 HTTP 客户端
}

// NewMCPClient 创建新的 MCP Skill 客户端
func NewMCPClient(cfg *Config) (*MCPClient, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("endpoint is required")
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	cacheTTL := cfg.CacheTTL
	if cacheTTL == 0 {
		cacheTTL = 60 * time.Second
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	client := &MCPClient{
		endpoint:        cfg.Endpoint,
		timeout:         timeout,
		logger:          logger,
		cacheTTL:        cacheTTL,
		reconnectConfig: DefaultReconnectConfig(),
	}

	// 初始化 HTTP 客户端（使用传入的或创建默认的）
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = NewDefaultHTTPClient(timeout)
	}

	client.client = &MCPHTTPClient{
		baseURL:    cfg.Endpoint,
		httpClient: httpClient,
	}

	return client, nil
}

// Invoke 调用 Skill 执行
func (c *MCPClient) Invoke(ctx interface{}, skillName string, params map[string]interface{}) (*tools.SkillInvokeResult, error) {
	startTime := time.Now()

	c.logger.Debug("invoking tool via MCP", "tool_name", skillName, "params", params)

	// 从调用方 context 提取 request_id，随出站 MCP 请求透传给 Python 侧（跨进程链路追踪）。
	requestID := extractRequestID(ctx)

	// 直接使用 skillName 作为 MCP 工具名调用
	// params 直接传递给工具
	resp, err := c.callMCPTool(skillName, params, requestID)
	if err != nil {
		c.logger.Error("MCP tool call failed", "tool_name", skillName, "error", err)
		return &tools.SkillInvokeResult{
			Success:  false,
			Error:    err.Error(),
			Duration: time.Since(startTime).Milliseconds(),
		}, err
	}

	// 解析响应（MCP 标准格式返回 TextContent 列表）
	result := &tools.SkillInvokeResult{
		Duration: time.Since(startTime).Milliseconds(),
	}

	if respBytes, ok := resp.([]byte); ok {
		if err := json.Unmarshal(respBytes, result); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
	} else if respList, ok := resp.([]interface{}); ok {
		// MCP 标准响应：TextContent 列表
		// 格式：[{"type": "text", "text": "{\"success\": true, \"data\": {...}}"}]
		if len(respList) > 0 {
			if firstContent, ok := respList[0].(map[string]interface{}); ok {
				if text, ok := firstContent["text"].(string); ok {
					// 解析 text 字段中的 JSON
					var contentData map[string]interface{}
					if err := json.Unmarshal([]byte(text), &contentData); err == nil {
						if success, ok := contentData["success"].(bool); ok {
							result.Success = success
						}
						if data, ok := contentData["data"].(map[string]interface{}); ok {
							result.Data = data
						}
						if errMsg, ok := contentData["error"].(string); ok {
							result.Error = errMsg
						}
					} else {
						// 如果不是 JSON，直接作为 data 返回
						result.Data = map[string]interface{}{"raw": text}
						result.Success = true
					}
				}
			}
		}
	} else if respMap, ok := resp.(map[string]interface{}); ok {
		// 直接使用 map 构造结果（兼容旧格式）
		if success, ok := respMap["success"].(bool); ok {
			result.Success = success
		}
		if data, ok := respMap["data"].(map[string]interface{}); ok {
			result.Data = data
		}
		if errStr, ok := respMap["error"].(string); ok {
			result.Error = errStr
		}
		if duration, ok := respMap["duration_ms"].(float64); ok {
			result.Duration = int64(duration)
		}
	}

	c.logger.Debug("MCP tool call completed", "tool_name", skillName, "success", result.Success, "duration_ms", result.Duration)
	return result, nil
}

// List 获取 Skill 列表
func (c *MCPClient) List(ctx interface{}) (*tools.SkillListResult, error) {
	c.mu.RLock()
	// 检查缓存是否有效
	if time.Now().Before(c.cacheExpiry) && len(c.skillsCache) > 0 {
		c.mu.RUnlock()
		c.logger.Debug("using cached skill list", "count", len(c.skillsCache))
		return &tools.SkillListResult{
			Skills:  c.skillsCache,
			Count:   len(c.skillsCache),
			Success: true,
		}, nil
	}
	c.mu.RUnlock()

	c.logger.Debug("fetching tool list from server")

	// 调用 MCP tools/list 方法
	resp, err := c.callMCPListTools()
	if err != nil {
		c.logger.Error("tools list failed", "error", err)
		return &tools.SkillListResult{
			Skills:  []tools.SkillInfo{},
			Count:   0,
			Success: false,
		}, err
	}

	result := &tools.SkillListResult{
		Skills:  []tools.SkillInfo{},
		Success: true,
	}

	if respBytes, ok := resp.([]byte); ok {
		if err := json.Unmarshal(respBytes, result); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
	} else if respMap, ok := resp.(map[string]interface{}); ok {
		// 解析 MCP tools/list 响应
		// MCP 返回格式: {"tools": [{"name": "...", "description": "...", "inputSchema": {...}}]}
		if tools, ok := respMap["tools"].([]interface{}); ok {
			for _, t := range tools {
				if toolMap, ok := t.(map[string]interface{}); ok {
					skill := c.parseMCPTool(toolMap)
					result.Skills = append(result.Skills, skill)
				}
			}
		}
	}

	result.Count = len(result.Skills)

	// 更新缓存
	c.mu.Lock()
	c.skillsCache = result.Skills
	c.cacheExpiry = time.Now().Add(c.cacheTTL)
	c.mu.Unlock()

	c.logger.Debug("skill list fetched", "count", result.Count)
	return result, nil
}

// Get 获取单个 Skill 详情
func (c *MCPClient) Get(ctx interface{}, skillName string) (*tools.SkillInfo, error) {
	result, err := c.List(ctx)
	if err != nil {
		return nil, err
	}

	for _, skill := range result.Skills {
		if skill.Name == skillName {
			return &skill, nil
		}
	}

	return nil, fmt.Errorf("skill not found: %s", skillName)
}

// HealthCheck 健康检查
func (c *MCPClient) HealthCheck(ctx interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 尝试调用 tools/list 验证连接
	_, err := c.callMCPListTools()
	if err != nil {
		c.connected = false
		return fmt.Errorf("MCP client health check failed: %w", err)
	}

	c.connected = true
	c.logger.Debug("MCP client health check passed")
	return nil
}

// extractRequestID 从调用方传入的 ctx（interface{}）中提取全链路 request_id。
// Invoke 的 ctx 形参为 interface{}，此处做类型断言到 context.Context 后复用既有 agentctx 助手；
// 无 context 或未注入 request_id 时返回空串（不透传）。
func extractRequestID(ctx interface{}) string {
	if c, ok := ctx.(context.Context); ok && c != nil {
		if rid, ok := agentctx.GetRequestID(c); ok {
			return rid
		}
	}
	return ""
}

// postMCP 发送 MCP HTTP 请求；有 request_id 且传输层支持带头 POST 时附带 X-Request-ID，
// 否则回退到无头 Post，保持对既有 HTTPClient 实现的向后兼容。
func (c *MCPClient) postMCP(url string, body []byte, requestID string) ([]byte, error) {
	if requestID != "" {
		if hp, ok := c.client.httpClient.(HeaderPoster); ok {
			return hp.PostWithHeaders(url, body, map[string]string{HTTPRequestIDHeader: requestID})
		}
	}
	return c.client.httpClient.Post(url, body)
}

// callMCPTool 调用 MCP 工具（核心方法）
func (c *MCPClient) callMCPTool(toolName string, arguments map[string]interface{}, requestID string) (interface{}, error) {
	// 构造 JSON-RPC 请求
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      fmt.Sprintf("skill-%d", time.Now().UnixNano()),
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      toolName,
			"arguments": arguments,
		},
	}

	// 序列化请求
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// 构造 MCP URL
	url := c.buildMCPURL()

	// 使用重试机制发送请求
	var lastErr error
	for attempt := 0; attempt <= c.reconnectConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := c.calculateBackoff(attempt)
			c.logger.Debug("retrying mcp call", "attempt", attempt, "delay", delay)
			time.Sleep(delay)
		}

		// 检查 HTTP 客户端是否可用
		if c.client == nil || c.client.httpClient == nil {
			lastErr = fmt.Errorf("HTTP client not initialized")
			continue
		}

		// 发送 HTTP 请求（透传 request_id 到 Python 侧）
		respBytes, err := c.postMCP(url, reqBytes, requestID)
		if err != nil {
			lastErr = fmt.Errorf("HTTP request failed: %w", err)
			c.logger.Warn("mcp call attempt failed", "attempt", attempt, "error", err)
			continue
		}

		// 解析 JSON-RPC 响应
		var resp map[string]interface{}
		if err := json.Unmarshal(respBytes, &resp); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}

		// 检查 JSON-RPC 错误
		if jsonErr, ok := resp["error"].(map[string]interface{}); ok {
			errMsg := "unknown error"
			if msg, ok := jsonErr["message"].(string); ok {
				errMsg = msg
			}
			return nil, fmt.Errorf("MCP error: %s", errMsg)
		}

		// 返回 result 字段
		if result, ok := resp["result"]; ok {
			return result, nil
		}

		return respBytes, nil
	}

	return nil, fmt.Errorf("mcp call failed after %d attempts: %w", c.reconnectConfig.MaxRetries+1, lastErr)
}

// callMCPListTools 调用 MCP tools/list 方法
func (c *MCPClient) callMCPListTools() (interface{}, error) {
	// 构造 JSON-RPC 请求
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      fmt.Sprintf("list-%d", time.Now().UnixNano()),
		"method":  "tools/list",
		"params":  map[string]interface{}{},
	}

	// 序列化请求
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// 构造 MCP URL
	url := c.buildMCPURL()

	// 使用重试机制发送请求
	var lastErr error
	for attempt := 0; attempt <= c.reconnectConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := c.calculateBackoff(attempt)
			c.logger.Debug("retrying mcp list", "attempt", attempt, "delay", delay)
			time.Sleep(delay)
		}

		// 检查 HTTP 客户端是否可用
		if c.client == nil || c.client.httpClient == nil {
			lastErr = fmt.Errorf("HTTP client not initialized")
			continue
		}

		// 发送 HTTP 请求
		respBytes, err := c.client.httpClient.Post(url, reqBytes)
		if err != nil {
			lastErr = fmt.Errorf("HTTP request failed: %w", err)
			c.logger.Warn("mcp list attempt failed", "attempt", attempt, "error", err)
			continue
		}

		// 解析 JSON-RPC 响应
		var resp map[string]interface{}
		if err := json.Unmarshal(respBytes, &resp); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}

		// 检查 JSON-RPC 错误
		if jsonErr, ok := resp["error"].(map[string]interface{}); ok {
			errMsg := "unknown error"
			if msg, ok := jsonErr["message"].(string); ok {
				errMsg = msg
			}
			return nil, fmt.Errorf("MCP error: %s", errMsg)
		}

		// 返回 result 字段
		if result, ok := resp["result"]; ok {
			return result, nil
		}

		return respBytes, nil
	}

	return nil, fmt.Errorf("mcp list failed after %d attempts: %w", c.reconnectConfig.MaxRetries+1, lastErr)
}

// parseMCPTool 解析 MCP 工具信息为 SkillInfo
func (c *MCPClient) parseMCPTool(m map[string]interface{}) tools.SkillInfo {
	info := tools.SkillInfo{}

	if name, ok := m["name"].(string); ok {
		info.Name = name
	}
	if desc, ok := m["description"].(string); ok {
		info.Description = desc
	}
	if inputSchema, ok := m["inputSchema"].(map[string]interface{}); ok {
		info.InputSchema = inputSchema
	}

	return info
}

// parseSkillInfo 解析 Skill 信息
func (c *MCPClient) parseSkillInfo(m map[string]interface{}) tools.SkillInfo {
	info := tools.SkillInfo{}

	if name, ok := m["name"].(string); ok {
		info.Name = name
	}
	if desc, ok := m["description"].(string); ok {
		info.Description = desc
	}
	if content, ok := m["content"].(string); ok {
		info.Content = content
	}
	if when, ok := m["when_to_use"].(string); ok {
		info.WhenToUse = when
	}
	if version, ok := m["version"].(string); ok {
		info.Version = version
	}
	if model, ok := m["model"].(string); ok {
		info.Model = model
	}
	if agent, ok := m["agent"].(string); ok {
		info.Agent = agent
	}
	if hint, ok := m["argument_hint"].(string); ok {
		info.ArgumentHint = hint
	}
	if category, ok := m["category"].(string); ok {
		info.Category = category
	}
	if author, ok := m["author"].(string); ok {
		info.Author = author
	}
	if source, ok := m["source"].(string); ok {
		info.Source = tools.SkillSource(source)
	} else {
		info.Source = tools.SkillSourceUnknown
	}
	if mode, ok := m["context_mode"].(string); ok {
		info.ContextMode = tools.ContextMode(mode)
	} else {
		info.ContextMode = tools.ContextModeInline
	}
	if deprecated, ok := m["deprecated"].(bool); ok {
		info.Deprecated = deprecated
	}
	if experimental, ok := m["experimental"].(bool); ok {
		info.Experimental = experimental
	}
	if timeout, ok := m["timeout"].(float64); ok {
		info.Timeout = int(timeout)
	} else if timeout, ok := m["timeout"].(int); ok {
		info.Timeout = timeout
	}
	if inputSchema, ok := m["input_schema"].(map[string]interface{}); ok {
		info.InputSchema = inputSchema
	}
	if outputSchema, ok := m["output_schema"].(map[string]interface{}); ok {
		info.OutputSchema = outputSchema
	}
	if raw, ok := m["raw_frontmatter"].(map[string]interface{}); ok {
		info.RawFrontmatter = raw
	}

	// 解析列表字段
	if tools, ok := m["allowed_tools"].([]interface{}); ok {
		for _, t := range tools {
			if s, ok := t.(string); ok {
				info.AllowedTools = append(info.AllowedTools, s)
			}
		}
	}
	if tools, ok := m["disallowed_tools"].([]interface{}); ok {
		for _, t := range tools {
			if s, ok := t.(string); ok {
				info.DisallowedTools = append(info.DisallowedTools, s)
			}
		}
	}
	if tags, ok := m["tags"].([]interface{}); ok {
		for _, t := range tags {
			if s, ok := t.(string); ok {
				info.Tags = append(info.Tags, s)
			}
		}
	}
	if args, ok := m["arg_names"].([]interface{}); ok {
		for _, a := range args {
			if s, ok := a.(string); ok {
				info.ArgNames = append(info.ArgNames, s)
			}
		}
	}

	return info
}

// calculateBackoff 计算退避延迟
func (c *MCPClient) calculateBackoff(attempt int) time.Duration {
	cfg := c.reconnectConfig
	// 防止负数或零导致错误的位移结果
	if attempt < 1 {
		attempt = 1
	}
	backoffFactor := 1 << uint(attempt-1)
	delay := time.Duration(float64(cfg.InitialDelay) * float64(backoffFactor) * cfg.BackoffMultiplier)
	if delay > cfg.MaxDelay {
		return cfg.MaxDelay
	}
	return delay
}

// buildMCPURL 构造 MCP 端点 URL
func (c *MCPClient) buildMCPURL() string {
	url := c.endpoint
	// 移除末尾斜杠
	url = strings.TrimSuffix(url, "/")
	// 如果已经包含 /mcp 路径，直接返回
	if strings.HasSuffix(url, "/mcp") {
		return url
	}
	// 添加 /mcp 路径
	return url + "/mcp"
}

// InvalidateCache 清空缓存
func (c *MCPClient) InvalidateCache() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.skillsCache = nil
	c.cacheExpiry = time.Time{}
	c.logger.Debug("skill cache invalidated")
}

// IsConnected 检查连接状态
func (c *MCPClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}
