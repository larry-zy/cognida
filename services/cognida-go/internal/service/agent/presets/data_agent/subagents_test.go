// Phase 7 任务 8.7：数据域子代理最小工具集 / 唯一持写 / 治理目录注册的单元测试。
package dataagent

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	domainagent "cognida/internal/model/agent"
	infraagent "cognida/internal/service/agent/framework"
	toolregistry "cognida/internal/service/agent/tools"
)

// writeToolNames 数据域写类工具全集（唯一持写断言基准）。
var writeToolNames = map[string]bool{
	"sql_mutate":  true,
	"etl_run":     true,
	"data_export": true,
}

// TestSubAgentSpecs_MinimalToolsets 叶子子代理各自只声明最小工具集，
// 写类工具仅 Operation 持有（任务 8.2），编排者不持任何直接数据工具（任务 8.1a）。
func TestSubAgentSpecs_MinimalToolsets(t *testing.T) {
	expected := map[string][]string{
		"SchemaExplorer": {"get_schema"},
		"SQLAuthor":      {"get_schema", "sql_execute"},
		"Analysis":       {"data_analysis"},
		"Operation":      {"sql_mutate", "etl_run", "data_export"},
		"Viz":            {"render_ui"},
	}
	for i := range leafSubAgentSpecs {
		spec := &leafSubAgentSpecs[i]
		want, ok := expected[spec.id]
		if !ok {
			t.Fatalf("未预期的叶子子代理: %s", spec.id)
		}
		if len(spec.toolNames) != len(want) {
			t.Fatalf("%s 工具集非最小: got=%v want=%v", spec.id, spec.toolNames, want)
		}
		for j, name := range want {
			if spec.toolNames[j] != name {
				t.Fatalf("%s 工具集不符: got=%v want=%v", spec.id, spec.toolNames, want)
			}
		}
		// 唯一持写：非 Operation 叶子不得声明写类工具，风险级必须 read
		if spec.id != "Operation" {
			for _, name := range spec.toolNames {
				if writeToolNames[name] {
					t.Fatalf("%s 不应持写类工具 %s（唯一持写=Operation）", spec.id, name)
				}
			}
			if spec.riskClass != infraagent.ScopeRead {
				t.Fatalf("%s 风险级应为 read: got=%s", spec.id, spec.riskClass)
			}
		} else if spec.riskClass != infraagent.ScopeWrite {
			t.Fatalf("Operation 风险级应为 write: got=%s", spec.riskClass)
		}
		if spec.delegates {
			t.Fatalf("叶子子代理 %s 不应是编排者", spec.id)
		}
	}

	// 上下文防火墙默认：SchemaExplorer/SQLAuthor 隔离，其余摘要
	isolated := map[string]bool{"SchemaExplorer": true, "SQLAuthor": true}
	for i := range leafSubAgentSpecs {
		spec := &leafSubAgentSpecs[i]
		wantMode := domainagent.ContextModeSummary
		if isolated[spec.id] {
			wantMode = domainagent.ContextModeIsolated
		}
		if spec.mode != wantMode {
			t.Fatalf("%s 默认上下文模式应为 %s: got=%s", spec.id, wantMode, spec.mode)
		}
	}

	// 分层编排者：只挂委派工具，不持直接数据工具
	for i := range orchestratorSubAgentSpecs {
		spec := &orchestratorSubAgentSpecs[i]
		if !spec.delegates {
			t.Fatalf("%s 应为委派型编排者", spec.id)
		}
		for _, name := range spec.toolNames {
			if name != "delegate_to_agent" && name != "delegate_parallel" {
				t.Fatalf("%s 不应持直接工具 %s", spec.id, name)
			}
		}
		if spec.riskClass != infraagent.ScopeRead {
			t.Fatalf("%s 风险级应为 read: got=%s", spec.id, spec.riskClass)
		}
	}
}

// ========================================
// RegisterDataSubAgents 集成注册（fake 工具 + fake 模型）
// ========================================

// stubToolModel 满足 ToolCallingChatModel 的空实现。
type stubToolModel struct{}

func (m *stubToolModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return &schema.Message{Role: schema.Assistant, Content: "final"}, nil
}

func (m *stubToolModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	reader, writer := schema.Pipe[*schema.Message](1)
	writer.Close()
	return reader, nil
}

func (m *stubToolModel) WithTools(_ []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

// newTestRegistry 构造一个仅注入 sqlmock 库连接的工具注册表：
// 子代理最小工具集（get_schema/sql_execute/data_analysis/sql_mutate/etl_run/
// data_export/render_ui）均无需可选依赖即注册，足以覆盖按名解析与治理目录登记。
func newTestRegistry(t *testing.T) *toolregistry.ToolRegistry {
	t.Helper()
	sqlDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	gdb, err := gorm.Open(gormmysql.New(gormmysql.Config{
		Conn: sqlDB, SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	reg, err := toolregistry.NewToolRegistry(toolregistry.ToolDeps{SQLDB: gdb})
	if err != nil {
		t.Fatalf("构造工具注册表: %v", err)
	}
	return reg
}

// TestRegisterDataSubAgents_GovernanceCatalog 全量注册后治理目录与上下文模式正确落表。
func TestRegisterDataSubAgents_GovernanceCatalog(t *testing.T) {
	reg := newTestRegistry(t)

	collab := infraagent.NewCollaborationRegistry()
	if err := RegisterDataSubAgents(context.Background(), collab, &stubToolModel{}, reg); err != nil {
		t.Fatalf("注册数据域子代理失败: %v", err)
	}

	wantAgents := []string{"SchemaExplorer", "SQLAuthor", "Analysis", "Operation", "Viz", "Insight", "Report"}
	for _, name := range wantAgents {
		if _, err := collab.GetByName(name); err != nil {
			t.Fatalf("子代理 %s 未注册: %v", name, err)
		}
		gov := collab.GetGovernance(name)
		if gov == nil || gov.Purpose == "" || gov.DataScope == "" || gov.RiskClass == "" {
			t.Fatalf("子代理 %s 治理目录不完整: %+v", name, gov)
		}
	}
	if len(collab.List()) != len(wantAgents) {
		t.Fatalf("注册数量不符: got=%v", collab.List())
	}

	if mode := collab.GetContextMode("SchemaExplorer"); mode != domainagent.ContextModeIsolated {
		t.Fatalf("SchemaExplorer 默认模式应 isolated: got=%s", mode)
	}
	if mode := collab.GetContextMode("Operation"); mode != domainagent.ContextModeSummary {
		t.Fatalf("Operation 默认模式应 summary: got=%s", mode)
	}
	if gov := collab.GetGovernance("Operation"); gov.RiskClass != infraagent.ScopeWrite {
		t.Fatalf("Operation 治理风险级应 write: got=%s", gov.RiskClass)
	}
	if gov := collab.GetGovernance("Insight"); gov.RiskClass != infraagent.ScopeRead {
		t.Fatalf("Insight 治理风险级应 read: got=%s", gov.RiskClass)
	}
}

// TestRegisterDataSubAgents_RequiresDeps 缺依赖直接报错（宁拒不猜）。
func TestRegisterDataSubAgents_RequiresDeps(t *testing.T) {
	reg := newTestRegistry(t)
	if err := RegisterDataSubAgents(context.Background(), nil, &stubToolModel{}, reg); err == nil {
		t.Fatalf("缺协作注册表应报错")
	}
	if err := RegisterDataSubAgents(context.Background(), infraagent.NewCollaborationRegistry(), nil, reg); err == nil {
		t.Fatalf("缺 ToolCallingChatModel 应报错")
	}
	if err := RegisterDataSubAgents(context.Background(), infraagent.NewCollaborationRegistry(), &stubToolModel{}, nil); err == nil {
		t.Fatalf("缺工具注册表应报错")
	}
}
