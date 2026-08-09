//go:build integration
// +build integration

// 外部测试包（tools_test）：本文件 import 被测包本身，放在 package tools 内会构成自导入循环。
package tools_test

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"cognida/internal/service/agent/tools"
)

// newIntegrationRegistry 经显式构造装配注册表（替代旧的包级全局单例）。
// 仅需满足必需依赖 SQLDB 即可完成全量注册；这里用 sqlmock 提供一个不落库的连接。
func newIntegrationRegistry(t *testing.T) *tools.ToolRegistry {
	t.Helper()
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      db,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)

	registry, err := tools.NewToolRegistry(tools.ToolDeps{SQLDB: gormDB})
	require.NoError(t, err)
	return registry
}

// TestRealLLM_ToolRegistry 测试工具注册表。
func TestRealLLM_ToolRegistry(t *testing.T) {
	registry := newIntegrationRegistry(t)
	toolNames := registry.ListTools()

	t.Logf("注册表中的工具数量: %d", len(toolNames))
	require.NotEmpty(t, toolNames)

	for _, name := range toolNames {
		tl, ok := registry.Get(name)
		require.True(t, ok, "tool %s should be retrievable", name)
		info, err := tl.Info(context.Background())
		require.NoError(t, err)
		t.Logf("工具: %s - %.60s", info.Name, info.Desc)
	}
}

// TestRealLLM_ToolList 测试列出所有工具分组。
func TestRealLLM_ToolList(t *testing.T) {
	registry := newIntegrationRegistry(t)
	groups := registry.ListGroups()

	t.Logf("工具分组: %v，共 %d 个工具", groups, registry.Size())
	require.Contains(t, groups, "operation", "操作工具组应已注册")

	for _, name := range []string{"sql_mutate", "etl_run", "data_export"} {
		_, ok := registry.Get(name)
		require.True(t, ok, "%s should be registered", name)
	}
}
