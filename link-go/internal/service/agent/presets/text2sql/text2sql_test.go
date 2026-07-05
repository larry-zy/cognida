// Package text2sql Plan-Execute-Reflect 模式 Text2SQL Agent 测试
package text2sql

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestPrompts 测试 Prompt 定义
func TestPrompts(t *testing.T) {
	// 验证 Prompt 不为空
	assert.NotEmpty(t, planPrompt, "planPrompt 不应为空")
	assert.NotEmpty(t, executePrompt, "executePrompt 不应为空")
	assert.NotEmpty(t, reflectPrompt, "reflectPrompt 不应为空")

	// 验证 Prompt 包含关键内容
	assert.Contains(t, planPrompt, "查询规划", "planPrompt 应该包含角色定义")
	assert.Contains(t, executePrompt, "SQL", "executePrompt 应该包含 SQL")
	assert.Contains(t, reflectPrompt, "审核", "reflectPrompt 应该包含审核任务")

	// 验证 planPrompt 包含输出格式说明
	assert.Contains(t, planPrompt, "query_type", "planPrompt 应该说明输出格式")
	assert.Contains(t, planPrompt, "tables", "planPrompt 应该说明输出格式")

	// 验证 executePrompt 包含执行流程说明
	assert.Contains(t, executePrompt, "get_schema", "executePrompt 应该说明使用 get_schema 工具")
	assert.Contains(t, executePrompt, "sql_execute", "executePrompt 应该说明使用 sql_execute 工具")

	// 验证 reflectPrompt 包含状态判断
	assert.Contains(t, reflectPrompt, "success", "reflectPrompt 应该说明成功状态")
	assert.Contains(t, reflectPrompt, "need_retry", "reflectPrompt 应该说明重试状态")
	assert.Contains(t, reflectPrompt, "检查查询结果", "reflectPrompt 应该说明审核任务")
}

// TestSpec 测试声明式描述的元信息与 Build 工厂。
func TestSpec(t *testing.T) {
	spec := Spec(nil, nil)

	assert.Equal(t, Text2SQLAgentID, spec.ID, "主 ID 应为 agent-text2sql-per")
	assert.Contains(t, spec.Aliases, "agent-text2sql-001", "应保留历史 agentID 别名")
	assert.Contains(t, spec.ToolNames, "get_schema")
	assert.Contains(t, spec.ToolNames, "sql_execute")
	assert.NotNil(t, spec.Build, "Build 工厂不应为 nil")
	assert.Equal(t, "sequential+semantic", spec.Metadata["pattern"])
}

// TestSpecBuild_WithoutTools 测试无工具注册时 Build 应 fail-fast（依赖必需工具）。
func TestSpecBuild_WithoutTools(t *testing.T) {
	spec := Spec(nil, nil)
	inst, err := spec.Build(context.Background())
	assert.Error(t, err, "缺少 get_schema/sql_execute 工具时应报错")
	assert.Nil(t, inst)
}
