// Package mcp MCP 客户端单元测试
package mcp

import (
	"encoding/json"
	"testing"
	"time"

	"link/internal/model/agent/tools"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMCPClient(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "成功创建客户端",
			config: &Config{
				Endpoint: "http://localhost:8080/mcp",
				Timeout:  30 * time.Second,
				CacheTTL: 60 * time.Second,
			},
			wantErr: false,
		},
		{
			name:   "配置为空",
			config: nil,
			wantErr: true,
		},
		{
			name: "缺少端点",
			config: &Config{
				Timeout: 30 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "使用默认超时",
			config: &Config{
				Endpoint: "http://localhost:8080/mcp",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewMCPClient(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				assert.Equal(t, tt.config.Endpoint, client.endpoint)
				if tt.config.Timeout > 0 {
					assert.Equal(t, tt.config.Timeout, client.timeout)
				} else {
					assert.Equal(t, 30*time.Second, client.timeout)
				}
			}
		})
	}
}

func TestMCPClient_parseSkillInfo(t *testing.T) {
	tests := []struct {
		name string
		data map[string]interface{}
		want tools.SkillInfo
	}{
		{
			name: "完整 Skill 信息",
			data: map[string]interface{}{
				"name":         "test_skill",
				"description":  "Test description",
				"content":      "# Test",
				"version":      "1.0.0",
				"category":     "test",
				"source":       "bundled",
				"context_mode": "inline",
				"deprecated":   false,
				"experimental": true,
				"timeout":      300,
				"allowed_tools": []interface{}{"tool1", "tool2"},
				"tags":         []interface{}{"tag1", "tag2"},
				"arg_names":    []interface{}{"arg1"},
				"input_schema": map[string]interface{}{
					"type": "object",
				},
			},
			want: tools.SkillInfo{
				Name:         "test_skill",
				Description:  "Test description",
				Content:      "# Test",
				Version:      "1.0.0",
				Category:     "test",
				Source:       tools.SkillSourceBundled,
				ContextMode:  tools.ContextModeInline,
				Deprecated:   false,
				Experimental: true,
				Timeout:      300,
				AllowedTools: []string{"tool1", "tool2"},
				Tags:         []string{"tag1", "tag2"},
				ArgNames:     []string{"arg1"},
				InputSchema:  map[string]interface{}{"type": "object"},
			},
		},
		{
			name: "最小 Skill 信息",
			data: map[string]interface{}{
				"name":        "minimal_skill",
				"description": "Minimal skill",
			},
			want: tools.SkillInfo{
				Name:         "minimal_skill",
				Description:  "Minimal skill",
				Version:      "",
				Category:     "",
				Source:       tools.SkillSourceUnknown,
				ContextMode:  tools.ContextModeInline,
				Deprecated:   false,
				Experimental: false,
				Timeout:      0,
			},
		},
		{
			name: "fork 模式",
			data: map[string]interface{}{
				"name":         "fork_skill",
				"description":  "Fork mode skill",
				"context_mode": "fork",
				"agent":        "sub_agent",
				"model":        "gpt-4",
			},
			want: tools.SkillInfo{
				Name:         "fork_skill",
				Description:  "Fork mode skill",
				ContextMode:  tools.ContextModeFork,
				Agent:        "sub_agent",
				Model:        "gpt-4",
				Source:       tools.SkillSourceUnknown,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &MCPClient{}
			got := client.parseSkillInfo(tt.data)

			assert.Equal(t, tt.want.Name, got.Name)
			assert.Equal(t, tt.want.Description, got.Description)
			assert.Equal(t, tt.want.Content, got.Content)
			assert.Equal(t, tt.want.Version, got.Version)
			assert.Equal(t, tt.want.Category, got.Category)
			assert.Equal(t, tt.want.Source, got.Source)
			assert.Equal(t, tt.want.ContextMode, got.ContextMode)
			assert.Equal(t, tt.want.Agent, got.Agent)
			assert.Equal(t, tt.want.Model, got.Model)
			assert.Equal(t, tt.want.Deprecated, got.Deprecated)
			assert.Equal(t, tt.want.Experimental, got.Experimental)
			assert.Equal(t, tt.want.Timeout, got.Timeout)

			// 验证数组字段
			assert.Equal(t, tt.want.AllowedTools, got.AllowedTools)
			assert.Equal(t, tt.want.Tags, got.Tags)
			assert.Equal(t, tt.want.ArgNames, got.ArgNames)

			// 验证 InputSchema
			if tt.want.InputSchema != nil {
				assert.Equal(t, tt.want.InputSchema["type"], got.InputSchema["type"])
			}
		})
	}
}

func TestMCPClient_calculateBackoff(t *testing.T) {
	config := DefaultReconnectConfig()
	client := &MCPClient{
		reconnectConfig: config,
	}

	tests := []struct {
		name     string
		attempt  int
		wantMin  time.Duration
		wantMax  time.Duration
	}{
		{
			name:    "第一次重试",
			attempt: 1,
			wantMin: 100 * time.Millisecond,
			wantMax: 200 * time.Millisecond,
		},
		{
			name:    "第二次重试",
			attempt: 2,
			wantMin: 200 * time.Millisecond,
			wantMax: 400 * time.Millisecond,
		},
		{
			name:    "第三次重试",
			attempt: 3,
			wantMin: 400 * time.Millisecond,
			wantMax: 800 * time.Millisecond,
		},
		{
			name:    "多次重试后应达到最大值",
			attempt: 10,
			wantMin: 5 * time.Second,
			wantMax: 5 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := client.calculateBackoff(tt.attempt)
			assert.GreaterOrEqual(t, got, tt.wantMin)
			assert.LessOrEqual(t, got, tt.wantMax)
		})
	}
}

func TestMCPClient_InvalidateCache(t *testing.T) {
	client, err := NewMCPClient(&Config{
		Endpoint: "http://localhost:8080/mcp",
	})
	require.NoError(t, err)

	// 设置缓存
	client.mu.Lock()
	client.skillsCache = []tools.SkillInfo{
		{Name: "cached_skill"},
	}
	client.cacheExpiry = time.Now().Add(1 * time.Hour)
	client.mu.Unlock()

	// 验证缓存存在
	client.mu.RLock()
	assert.Len(t, client.skillsCache, 1)
	assert.True(t, time.Now().Before(client.cacheExpiry))
	client.mu.RUnlock()

	// 清空缓存
	client.InvalidateCache()

	// 验证缓存已清空
	client.mu.RLock()
	assert.Nil(t, client.skillsCache)
	assert.True(t, client.cacheExpiry.IsZero())
	client.mu.RUnlock()
}

func TestMCPClient_IsConnected(t *testing.T) {
	client, err := NewMCPClient(&Config{
		Endpoint: "http://localhost:8080/mcp",
	})
	require.NoError(t, err)

	// 初始状态应该是未连接
	assert.False(t, client.IsConnected())

	// 模拟连接成功
	client.mu.Lock()
	client.connected = true
	client.mu.Unlock()

	assert.True(t, client.IsConnected())
}

func TestSkillSource_String(t *testing.T) {
	tests := []struct {
		source tools.SkillSource
		want   string
	}{
		{tools.SkillSourceBundled, "bundled"},
		{tools.SkillSourcePrivate, "private"},
		{tools.SkillSourcePublic, "public"},
		{tools.SkillSourcePlugin, "plugin"},
		{tools.SkillSourceUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := string(tt.source)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestContextMode_String(t *testing.T) {
	tests := []struct {
		mode tools.ContextMode
		want string
	}{
		{tools.ContextModeInline, "inline"},
		{tools.ContextModeFork, "fork"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := string(tt.mode)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReconnectConfig_Defaults(t *testing.T) {
	config := DefaultReconnectConfig()

	assert.Equal(t, 3, config.MaxRetries)
	assert.Equal(t, 100*time.Millisecond, config.InitialDelay)
	assert.Equal(t, 5*time.Second, config.MaxDelay)
	assert.Equal(t, 2.0, config.BackoffMultiplier)
}

// 测试 JSON 序列化
func TestSkillInfo_JSONRoundTrip(t *testing.T) {
	original := tools.SkillInfo{
		Name:         "json_test_skill",
		Description:  "JSON serialization test",
		Version:      "1.5.0",
		Category:     "test",
		Source:       tools.SkillSourceBundled,
		ContextMode:  tools.ContextModeInline,
		AllowedTools: []string{"tool1", "tool2", "tool3"},
		DisallowedTools: []string{"tool4"},
		Tags:          []string{"json", "test", "serialization"},
		ArgNames:      []string{"input", "output"},
		ArgumentHint:  "Provide input and output parameters",
		Deprecated:    false,
		Experimental:  true,
		Timeout:       600,
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"input": map[string]interface{}{
					"type":        "string",
					"description": "Input parameter",
				},
			},
		},
		OutputSchema: map[string]interface{}{
			"type": "object",
		},
		Model:  "gpt-4",
		Agent:  "sub_agent",
	}

	// 序列化
	data, err := json.Marshal(original)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// 反序列化
	var decoded tools.SkillInfo
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// 验证所有字段
	assert.Equal(t, original.Name, decoded.Name)
	assert.Equal(t, original.Description, decoded.Description)
	assert.Equal(t, original.Version, decoded.Version)
	assert.Equal(t, original.Category, decoded.Category)
	assert.Equal(t, original.Source, decoded.Source)
	assert.Equal(t, original.ContextMode, decoded.ContextMode)
	assert.Equal(t, original.AllowedTools, decoded.AllowedTools)
	assert.Equal(t, original.DisallowedTools, decoded.DisallowedTools)
	assert.Equal(t, original.Tags, decoded.Tags)
	assert.Equal(t, original.ArgNames, decoded.ArgNames)
	assert.Equal(t, original.ArgumentHint, decoded.ArgumentHint)
	assert.Equal(t, original.Deprecated, decoded.Deprecated)
	assert.Equal(t, original.Experimental, decoded.Experimental)
	assert.Equal(t, original.Timeout, decoded.Timeout)
	assert.Equal(t, original.Model, decoded.Model)
	assert.Equal(t, original.Agent, decoded.Agent)
	assert.Equal(t, original.InputSchema["type"], decoded.InputSchema["type"])
}
