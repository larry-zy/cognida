//go:build integration
// +build integration

// Package llm provides real integration tests for ChatService
package chat

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	infraagent "link/internal/service/agent/framework"
	"link/internal/model/common"
	"link/internal/infrastructure/llm/chat"
)

// ========================================
// Test Configuration Loader
// ========================================

// TestConfig 测试配置
type TestConfig struct {
	APIKey    string
	BaseURL   string
	ModelName string
	Provider  string
}

// loadTestConfig 从环境变量或 .env 文件加载测试配置
// 支持多种来源：环境变量、.env 文件、命令行参数
func loadTestConfig(t *testing.T) *TestConfig {
	cfg := &TestConfig{
		// 默认值 - DeepSeek
		APIKey:    os.Getenv("DEEPSEEK_API_KEY"),
		BaseURL:   os.Getenv("DEEPSEEK_BASE_URL"),
		ModelName: "deepseek-chat",
		Provider:  "deepseek",
	}

	// 如果 DeepSeek 配置存在，直接使用
	if cfg.APIKey != "" {
		t.Logf("使用 DeepSeek 配置")
		return cfg
	}

	// 尝试使用 OpenAI 配置
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		cfg.APIKey = apiKey
		cfg.BaseURL = os.Getenv("OPENAI_BASE_URL")
		cfg.ModelName = os.Getenv("OPENAI_MODEL_NAME")
		if cfg.ModelName == "" {
			cfg.ModelName = "gpt-3.5-turbo"
		}
		cfg.Provider = "openai"
		t.Logf("使用 OpenAI 配置")
		return cfg
	}

	// 尝试从 .env 文件加载
	if envConfig := loadEnvFile(); envConfig != nil {
		// 优先使用 DeepSeek 配置
		if apiKey := envConfig["DEEPSEEK_API_KEY"]; apiKey != "" {
			cfg.APIKey = apiKey
			cfg.BaseURL = envConfig["DEEPSEEK_BASE_URL"]
			if cfg.BaseURL == "" {
				cfg.BaseURL = "https://api.deepseek.com"
			}
			cfg.ModelName = "deepseek-chat"
			cfg.Provider = "deepseek"
			t.Logf("从 .env 加载 DeepSeek 配置")
			return cfg
		}

		// 其次使用 CHAT 配置
		if apiKey := envConfig["CHAT_API_KEY"]; apiKey != "" {
			cfg.APIKey = apiKey
			cfg.BaseURL = envConfig["CHAT_BASE_URL"]
			cfg.ModelName = envConfig["CHAT_MODEL_NAME"]
			if cfg.ModelName == "" {
				cfg.ModelName = "gpt-3.5-turbo"
			}
			// 从 BaseURL 检测 Provider
			cfg.Provider = detectProviderFromURL(cfg.BaseURL)
			t.Logf("从 .env 加载 CHAT 配置 (provider: %s)", cfg.Provider)
			return cfg
		}
	}

	// 都没有配置，跳过测试
	t.Skip("未配置 LLM API Key，跳过集成测试\n" +
		"请设置以下环境变量之一：\n" +
		"  - DEEPSEEK_API_KEY (推荐，DeepSeek)\n" +
		"  - OPENAI_API_KEY (OpenAI)\n" +
		"  - 或在 .env 文件中配置 CHAT_API_KEY")

	return nil
}

// loadEnvFile 加载 .env 文件
func loadEnvFile() map[string]string {
	// 首先检查环境变量是否已设置
	if apiKey := os.Getenv("CHAT_API_KEY"); apiKey != "" {
		return map[string]string{
			"CHAT_API_KEY":     apiKey,
			"CHAT_BASE_URL":    os.Getenv("CHAT_BASE_URL"),
			"CHAT_MODEL_NAME":  os.Getenv("CHAT_MODEL_NAME"),
			"CHAT_PROVIDER":    os.Getenv("CHAT_PROVIDER"),
			"DEEPSEEK_API_KEY": os.Getenv("DEEPSEEK_API_KEY"),
		}
	}

	// 尝试从项目根目录加载 .env
	pwd, _ := os.Getwd()

	// 从当前目录向上查找 .env
	baseDir := pwd
	for i := 0; i < 6; i++ { // 最多向上查找6级
		envPath := baseDir + "/.env"
		if _, err := os.Stat(envPath); err == nil {
			// 找到了 .env 文件
			data, err := os.ReadFile(envPath)
			if err != nil {
				continue
			}

			config := make(map[string]string)
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}

				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])
					// 移除引号
					value = strings.Trim(value, `"`)
					config[key] = value
				}
			}

			if len(config) > 0 {
				return config
			}
		}

		// 向上一级
		parentDir := baseDir[:strings.LastIndex(baseDir, "/")]
		if parentDir == baseDir {
			break
		}
		baseDir = parentDir
	}

	return nil
}

// detectProviderFromURL 从 URL 检测 Provider
func detectProviderFromURL(baseURL string) string {
	if baseURL == "" {
		return "openai"
	}

	lowerURL := strings.ToLower(baseURL)
	switch {
	case strings.Contains(lowerURL, "deepseek"):
		return "deepseek"
	case strings.Contains(lowerURL, "aliyun"):
		return "aliyun"
	case strings.Contains(lowerURL, "qwen"):
		return "qwen"
	case strings.Contains(lowerURL, "gpts.vin"):
		// 这个是中转服务
		return "generic"
	default:
		return "openai"
	}
}

// ========================================
// Test Setup Functions
// ========================================

// createToolModel 创建测试用的 LLM 模型
func createToolModel(t *testing.T) model.ToolCallingChatModel {
	cfg := loadTestConfig(t)
	if cfg == nil {
		return nil
	}

	config := &chat.ChatConfig{
		Source:    common.ModelSourceRemote,
		APIKey:    cfg.APIKey,
		BaseURL:   cfg.BaseURL,
		ModelName: cfg.ModelName,
		Provider:  cfg.Provider,
	}

	toolModel, err := chat.NewToolCallingChatModel(context.Background(), config)
	require.NoError(t, err, "创建模型失败")

	t.Logf("✅ 模型创建成功: provider=%s, model=%s", cfg.Provider, cfg.ModelName)
	return toolModel
}

// createRealAgent 创建使用真实 LLM 的 Agent
func createRealAgent(t *testing.T, name string, prompt string) infraagent.Agent {
	model := createToolModel(t)
	if model == nil {
		return nil
	}

	agent, err := infraagent.New(nil).
		Name(name).
		Prompt(prompt).
		WithToolModel(model).
		Build(context.Background())

	require.NoError(t, err)
	require.NotNil(t, agent)

	t.Logf("✅ Agent 创建成功: %s", name)
	return agent
}

// ========================================
// Real Integration Tests
// ========================================

// TestIntegration_RealLLMChat_Basic 基础真实 LLM 聊天测试
func TestIntegration_RealLLMChat_Basic(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试（使用 -short 标志）")
	}

	// Arrange - 创建真实 Agent
	agent := createRealAgent(t, "basic-test", "你是一个简洁的AI助手。请用一句话回答。")
	if agent == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Act - 调用真实 LLM
	resp, err := agent.Chat(ctx, "你好，请用一句话说明你是谁")

	// Assert
	require.NoError(t, err, "Chat 调用失败")
	assert.NotEmpty(t, resp.Content, "响应内容不应为空")

	t.Logf("✅ 真实 LLM 响应: %s", resp.Content)
}

// TestIntegration_RealLLMChat_Streaming 流式聊天测试
func TestIntegration_RealLLMChat_Streaming(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试")
	}

	agent := createRealAgent(t, "stream-test", "你是一个简洁的AI助手。")
	if agent == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	streamChan, err := agent.Stream(ctx, "数到5")

	require.NoError(t, err, "Stream 调用失败")

	var fullContent strings.Builder
	chunkCount := 0

	for chunk := range streamChan {
		chunkCount++
		if chunk.Content != "" {
			fullContent.WriteString(chunk.Content)
		}
		if chunk.Done {
			break
		}
	}

	assert.Greater(t, chunkCount, 0, "应该收到至少一个 chunk")
	assert.NotEmpty(t, fullContent.String(), "完整内容不应为空")

	t.Logf("✅ 收到 %d 个 chunks, 完整内容: %s", chunkCount, fullContent.String())
}

// TestIntegration_RealLLMChat_Codes 解释代码测试
func TestIntegration_RealLLMChat_Codes(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试")
	}

	agent := createRealAgent(t, "code-test", "你是一个编程助手。简洁解释代码。")
	if agent == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	code := `func add(a, b int) int { return a + b }`
	resp, err := agent.Chat(ctx, fmt.Sprintf("解释这行代码: %s", code))

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Content)

	t.Logf("✅ 代码解释: %s", resp.Content)
}

// TestIntegration_RealLLMChat_Formatting 格式化测试
func TestIntegration_RealLLMChat_Formatting(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试")
	}

	agent := createRealAgent(t, "format-test", "你是一个AI助手。按要求格式化输出。")
	if agent == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := agent.Chat(ctx, "用 JSON 格式返回：{\"name\": \"测试\", \"value\": 123}")

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Content)

	// 检查是否包含 JSON 结构
	hasJSON := strings.Contains(resp.Content, "{") && strings.Contains(resp.Content, "}")
	if hasJSON {
		t.Logf("✅ 响应包含 JSON: %s", resp.Content)
	} else {
		t.Logf("⚠️  响应可能不是标准 JSON: %s", resp.Content)
	}
}

// TestIntegration_Performance_Latency 性能测试
func TestIntegration_Performance_Latency(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试")
	}

	agent := createRealAgent(t, "perf-test", "你好。")
	if agent == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 测试简单请求的延迟
	start := time.Now()
	resp, err := agent.Chat(ctx, "说：OK")
	elapsed := time.Since(start)

	require.NoError(t, err)
	t.Logf("✅ 请求耗时: %v", elapsed)
	t.Logf("响应: %s", resp.Content)

	// 性能断言
	if elapsed > 10*time.Second {
		t.Logf("⚠️  响应较慢: %v", elapsed)
	}
}

// TestIntegration_Configuration_Display 配置显示
func TestIntegration_Configuration_Display(t *testing.T) {
	cfg := loadTestConfig(t)
	if cfg == nil {
		return
	}

	t.Logf("=== 当前测试配置 ===")
	t.Logf("Provider:  %s", cfg.Provider)
	t.Logf("ModelName: %s", cfg.ModelName)
	t.Logf("BaseURL:   %s", cfg.BaseURL)
	t.Logf("APIKey:    %s***", maskAPIKey(cfg.APIKey))
}

// maskAPIKey 遮蔽 API Key 显示
func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "***" + key[len(key)-4:]
}
