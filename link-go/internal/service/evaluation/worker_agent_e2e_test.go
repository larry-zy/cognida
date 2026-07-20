//go:build integration

// Package evaluation Agent 评测跨进程 e2e 集成测试。
//
// 与 worker_metrics_test.go 的 httptest 桩不同，本用例打真实运行中的 Python 评测
// 服务（fastapi_app），完整跑通 Go 端 computeMetrics → 组装 ComputeItem →
// ensureAgentGraders 注入 Agent 家族评分器 → HTTP → Python compute_agent_metrics
// 聚合 → fillMetrics → augmentAgentRuntimeMetrics 全链路，验证前端目录里新登记的
// 5 个 Agent 评测器（answer_accuracy/tool_selection/tool_order/trajectory_match/
// step_efficiency）真正产出任务级聚合指标 + Go 采集的运行时基础指标。
//
// 运行：需先起 Python 评测服务，并通过环境变量指定端点（未设置则 Skip）：
//
//	LINK_EVAL_E2E_ENDPOINT=http://127.0.0.1:18899 \
//	  go test -tags=integration -run TestE2E_AgentComputeMetrics_Live \
//	  ./internal/service/evaluation/ -v
package evaluation

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	domeval "link/internal/model/evaluation"
)

// TestE2E_AgentComputeMetrics_Live 打真实 Python 服务，断言 Agent 家族聚合指标 + 运行时指标齐全。
func TestE2E_AgentComputeMetrics_Live(t *testing.T) {
	endpoint := os.Getenv("LINK_EVAL_E2E_ENDPOINT")
	if endpoint == "" {
		t.Skip("LINK_EVAL_E2E_ENDPOINT 未设置，跳过跨进程 e2e（需运行中的 Python 评测服务）")
	}

	worker := &EvaluationWorker{pythonClient: NewPythonEvaluationClient(endpoint)}

	// 模拟 AgentExecutor 跑完 sample_agent 数据集后的逐条结果：
	//   item0 完美命中；item1 多调一个工具且多绕一步；item2 选错工具。
	// 覆盖 precision/recall/order/traj/step 的非平凡取值，避免全 1 掩盖计算错误。
	qaResults := []*QAResult{
		{
			Question:        "今天北京的天气怎么样？",
			ReferenceAnswer: "今天北京晴天，温度25度",
			GeneratedAnswer: "今天北京晴天，温度25度",
			ExpectedTools:   []string{"search"},
			ExpectedSteps:   []string{"思考用户问题", "调用搜索工具查询天气", "返回结果"},
			ToolsUsed:       []string{"search"},
			Trajectory:      []string{"思考用户问题", "调用搜索工具查询天气", "返回结果"},
			TotalSteps:      3,
			Success:         true,
			LatencyMs:       820,
			TokensUsed:      312,
			LLMCalls:        2,
		},
		{
			Question:        "计算123 + 456等于多少？",
			ReferenceAnswer: "579",
			GeneratedAnswer: "123 加 456 等于 579",
			ExpectedTools:   []string{"calculator"},
			ExpectedSteps:   []string{"识别计算需求", "调用计算器", "返回结果"},
			ToolsUsed:       []string{"search", "calculator"}, // 多调一个 search
			Trajectory:      []string{"识别计算需求", "误调搜索", "调用计算器", "返回结果"},
			TotalSteps:      4, // 比最优多一步
			Success:         true,
			LatencyMs:       1450,
			TokensUsed:      688,
			LLMCalls:        3,
		},
		{
			Question:        "查询用户张三的订单",
			ReferenceAnswer: "张三有3笔订单",
			GeneratedAnswer: "抱歉，没有查询到",
			ExpectedTools:   []string{"database"},
			ExpectedSteps:   []string{"解析用户名", "查询订单库", "汇总返回"},
			ToolsUsed:       []string{"search"}, // 选错工具
			Trajectory:      []string{"解析用户名", "调用搜索"},
			TotalSteps:      2,
			Success:         true,
			LatencyMs:       990,
			TokensUsed:      401,
			LLMCalls:        2,
		},
	}

	config := &DomainEvaluationTaskConfig{DatasetID: "sample_agent", Type: domeval.EvaluationTypeAgent}

	result, err := worker.computeMetrics(context.Background(), config, qaResults)
	if err != nil {
		t.Fatalf("computeMetrics 打真实服务失败: %v", err)
	}
	if result.Scores == nil {
		t.Fatal("evalResult.Scores 为空，聚合指标未回填")
	}

	// 1) Agent 家族聚合指标必须齐全（compute_agent_metrics 扁平化后的键）
	agentKeys := []string{
		"answer_accuracy",
		"tool_precision", "tool_recall", "tool_f1", "tool_order",
		"traj_exact_match", "traj_similarity",
		"step_optimal_ratio",
	}
	for _, k := range agentKeys {
		v, ok := result.Scores[k]
		if !ok {
			t.Errorf("缺少 Agent 聚合指标 %q（Python compute_agent_metrics 未产出或未回填）", k)
			continue
		}
		if v < 0 || v > 1.0001 {
			t.Errorf("Agent 指标 %q=%v 超出 [0,1] 合理区间", k, v)
		}
	}

	// 2) 运行时基础指标（Go 侧采集）：均值 + 成功率
	//    latency 均值 = (820+1450+990)/3 = 1086.67；success_rate = 3/3 = 1
	if got := result.Scores["latency_ms"]; got < 1000 || got > 1200 {
		t.Errorf("运行时 latency_ms 均值 = %v, 期望 ~1086.67", got)
	}
	if got := result.Scores["success_rate"]; got != 1.0 {
		t.Errorf("success_rate = %v, want 1.0", got)
	}
	if _, ok := result.Scores["tokens_used"]; !ok {
		t.Error("缺少运行时指标 tokens_used")
	}
	if _, ok := result.Scores["llm_calls"]; !ok {
		t.Error("缺少运行时指标 llm_calls")
	}

	// 3) 非平凡性：item1/item2 有工具/轨迹偏差，tool_f1 不应恒为 1
	if result.Scores["tool_f1"] >= 0.9999 {
		t.Errorf("tool_f1=%v 恒为满分，与构造的工具偏差不符，聚合可能未真正区分", result.Scores["tool_f1"])
	}

	// 落盘：把完整聚合写入 test-output/，供人工核对（路径由环境变量给出，缺省则跳过写盘）
	if out := os.Getenv("LINK_EVAL_E2E_OUTPUT"); out != "" {
		payload, _ := json.MarshalIndent(map[string]any{
			"endpoint":  endpoint,
			"dataset":   config.DatasetID,
			"aggregate": result.Scores,
			"per_item_scores": func() []map[string]float64 {
				s := make([]map[string]float64, 0, len(result.QAResults))
				for _, qa := range result.QAResults {
					s = append(s, qa.Scores)
				}
				return s
			}(),
		}, "", "  ")
		if err := os.WriteFile(out, payload, 0o644); err != nil {
			t.Logf("写盘失败（不影响断言）: %v", err)
		} else {
			t.Logf("聚合指标已写入 %s", out)
		}
	}

	t.Logf("Agent 聚合指标: answer_accuracy=%.3f tool_f1=%.3f tool_order=%.3f traj_similarity=%.3f step_optimal_ratio=%.3f",
		result.Scores["answer_accuracy"], result.Scores["tool_f1"], result.Scores["tool_order"],
		result.Scores["traj_similarity"], result.Scores["step_optimal_ratio"])
}
