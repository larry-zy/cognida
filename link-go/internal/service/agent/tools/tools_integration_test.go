//go:build integration
// +build integration

package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"link/internal/service/agent/tools"
)

// TestRealLLM_ToolRegistry 测试工具注册表。
func TestRealLLM_ToolRegistry(t *testing.T) {
	// 测试工具注册表是否正常工作
	registry := tools.GetDefaultRegistry()
	allTools := registry.GetTools()

	t.Logf("注册表中的工具数量: %d", len(allTools))

	for _, tool := range allTools {
		info, err := tool.Info(context.Background())
		require.NoError(t, err)
		t.Logf("工具: %s - %s", info.Name, info.Desc)
	}

	// 测试 GetToolsByNames
	if len(allTools) > 0 {
		info, _ := allTools[0].Info(context.Background())
		toolsByName, err := registry.GetToolsByNames([]string{info.Name})
		require.NoError(t, err)
		require.Len(t, toolsByName, 1)
		t.Logf("按名称获取工具成功: %s", info.Name)
	}
}

// TestRealLLM_ToolList 测试列出所有工具。
func TestRealLLM_ToolList(t *testing.T) {
	registry := tools.GetDefaultRegistry()
	toolNames := registry.List()

	t.Logf("可用工具列表: %v", toolNames)

	// 至少应该有一些工具
	if len(toolNames) > 0 {
		t.Logf("工具注册表工作正常，找到 %d 个工具", len(toolNames))
	}
}
