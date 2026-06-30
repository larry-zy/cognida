// Package tools Skill 工具单元测试
package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockInvokableTool 模拟一个可调用工具，用于验证 skill_invoke 的路由逻辑
type mockInvokableTool struct {
	name   string
	desc   string
	result string
	err    error
}

func (m *mockInvokableTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: m.name, Desc: m.desc}, nil
}

func (m *mockInvokableTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.result, nil
}

// ========================================
// 工具构造与 Info
// ========================================

func TestNewSkillInvokeTool(t *testing.T) {
	tl, err := NewSkillInvokeTool()
	assert.NoError(t, err)
	assert.NotNil(t, tl)
}

func TestNewSkillListTool(t *testing.T) {
	tl, err := NewSkillListTool()
	assert.NoError(t, err)
	assert.NotNil(t, tl)
}

func TestSkillInvokeTool_Info(t *testing.T) {
	tl, err := NewSkillInvokeTool()
	require.NoError(t, err)

	info, err := tl.Info(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "skill_invoke", info.Name)
	assert.NotEmpty(t, info.Desc)
	assert.NotNil(t, info.ParamsOneOf)
}

func TestSkillListTool_Info(t *testing.T) {
	tl, err := NewSkillListTool()
	require.NoError(t, err)

	info, err := tl.Info(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "skill_list", info.Name)
	assert.NotEmpty(t, info.Desc)
}

// ========================================
// skill_invoke 调用逻辑
// ========================================

func TestSkillInvokeTool_InvokableRun(t *testing.T) {
	t.Run("缺少 task 参数返回错误", func(t *testing.T) {
		tl, err := NewSkillInvokeTool()
		require.NoError(t, err)

		_, err = tl.InvokableRun(context.Background(), `{}`)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "task")
	})

	t.Run("指定的工具不存在返回错误", func(t *testing.T) {
		tl, err := NewSkillInvokeTool()
		require.NoError(t, err)

		req := `{"task":"do something","tool_name":"nonexistent_tool"}`
		_, err = tl.InvokableRun(context.Background(), req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tool not found")
	})

	t.Run("路由到已注册的工具并返回其输出", func(t *testing.T) {
		mock := &mockInvokableTool{name: "mock_skill_tool", result: "mock output"}
		require.NoError(t, GlobalRegistry.Register("test", mock))
		defer GlobalRegistry.Unregister("mock_skill_tool")

		tl, err := NewSkillInvokeTool()
		require.NoError(t, err)

		req := `{"task":"do something","tool_name":"mock_skill_tool","parameters":"{\"k\":\"v\"}"}`
		result, err := tl.InvokableRun(context.Background(), req)
		assert.NoError(t, err)
		assert.Equal(t, "mock output", result)
	})
}

// ========================================
// skill_list 列表逻辑
// ========================================

func TestSkillListTool_InvokableRun(t *testing.T) {
	mock := &mockInvokableTool{name: "mock_list_tool", desc: "a mock tool"}
	require.NoError(t, GlobalRegistry.Register("test", mock))
	defer GlobalRegistry.Unregister("mock_list_tool")

	tl, err := NewSkillListTool()
	require.NoError(t, err)

	result, err := tl.InvokableRun(context.Background(), `{}`)
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result), &parsed))

	count, ok := parsed["count"].(float64)
	assert.True(t, ok)
	assert.GreaterOrEqual(t, int(count), 1)

	tools, ok := parsed["tools"].([]interface{})
	assert.True(t, ok)
	assert.NotEmpty(t, tools)
}

// ========================================
// 类型 JSON 序列化（types 仍在使用，保留覆盖）
// ========================================

func TestSkillInfo_JSONSerialization(t *testing.T) {
	tests := []struct {
		name  string
		skill SkillInfo
	}{
		{
			name: "完整 Skill 信息",
			skill: SkillInfo{
				Name:         "test_skill",
				Description:  "Test skill for serialization",
				Content:      "# Test Skill\n\nThis is a test skill.",
				Version:      "1.0.0",
				Category:     "test",
				Source:       SkillSourceBundled,
				ContextMode:  ContextModeInline,
				AllowedTools: []string{"tool1", "tool2"},
				Tags:         []string{"test", "mock"},
				Deprecated:   false,
				Experimental: false,
				Timeout:      300,
			},
		},
		{
			name: "fork 模式 Skill",
			skill: SkillInfo{
				Name:         "fork_skill",
				Description:  "Fork mode skill",
				Version:      "2.0.0",
				ContextMode:  ContextModeFork,
				Agent:        "sub_agent",
				Source:       SkillSourcePlugin,
				Experimental: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.skill)
			assert.NoError(t, err)
			assert.NotEmpty(t, data)

			var decoded SkillInfo
			err = json.Unmarshal(data, &decoded)
			assert.NoError(t, err)

			assert.Equal(t, tt.skill.Name, decoded.Name)
			assert.Equal(t, tt.skill.Description, decoded.Description)
			assert.Equal(t, tt.skill.Version, decoded.Version)
			assert.Equal(t, tt.skill.ContextMode, decoded.ContextMode)
			assert.Equal(t, tt.skill.Source, decoded.Source)
			assert.Equal(t, tt.skill.AllowedTools, decoded.AllowedTools)
			assert.Equal(t, tt.skill.Tags, decoded.Tags)
		})
	}
}

func TestSkillInvokeResult_JSONSerialization(t *testing.T) {
	tests := []struct {
		name   string
		result SkillInvokeResult
	}{
		{
			name: "成功结果",
			result: SkillInvokeResult{
				Success: true,
				Data: map[string]interface{}{
					"output": "test output",
					"count":  10,
				},
				Duration: 150,
			},
		},
		{
			name: "失败结果",
			result: SkillInvokeResult{
				Success:  false,
				Error:    "Execution failed",
				Duration: 50,
			},
		},
		{
			name: "带元数据的结果",
			result: SkillInvokeResult{
				Success: true,
				Data: map[string]interface{}{
					"result": "done",
				},
				Metadata: map[string]interface{}{
					"cached":  true,
					"version": "1.0.0",
				},
				Duration: 100,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.result)
			assert.NoError(t, err)
			assert.NotEmpty(t, data)

			var decoded SkillInvokeResult
			err = json.Unmarshal(data, &decoded)
			assert.NoError(t, err)

			assert.Equal(t, tt.result.Success, decoded.Success)
			assert.Equal(t, tt.result.Duration, decoded.Duration)
			assert.Equal(t, tt.result.Error, decoded.Error)

			if tt.result.Data != nil {
				for k, v := range tt.result.Data {
					decodedVal := decoded.Data[k]
					if intVal, ok := v.(int); ok {
						if floatVal, ok := decodedVal.(float64); ok {
							assert.Equal(t, intVal, int(floatVal))
						} else {
							assert.Equal(t, v, decodedVal)
						}
					} else {
						assert.Equal(t, v, decodedVal)
					}
				}
			}
		})
	}
}
