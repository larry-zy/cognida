// 复杂任务下沉 skill（design D8 / 任务组 7）：把「多维归因」「经营报告」这类多步任务
// 封装成带 handler 的可执行 skill。handler 内经注入 ctx 的 collabRegistry inline 编排
// SQLAuthor/Analysis/Insight/Report/Operation 子代理群（复用 framework 委派内核的紧凑
// 回传 + IsCyclic/MaxDepth 护栏 + 每次委派最小 scope 授予），主循环只见一次 skill 调用、
// 只收结论摘要，内部逐轮往返不回灌。
//
// 修复据此天然分两层：子代理 execLoop 内做战术修复（改 SQL 重试/瞬时退避），子代理回报
// 搞不定时 handler 返回错误，交由主 agent/编排层做战略重规划（D5 护栏在主层计数）。
//
// 向后兼容：这些是「新增」的可执行 skill，缺省不影响任何既有纯指导 skill 与简单单步任务
// （WhenToUse 明确限定复杂场景，简单查询/单步分析不下沉，主 agent 直连）。
package dataagent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	infraagent "cognida/internal/service/agent/framework"
	"cognida/internal/service/agent/skills"
)

const (
	// attributionSkillName 多维归因技能名（ASCII kebab，IsValidSkillName 约束）。
	attributionSkillName = "multi-dim-attribution"
	// businessReportSkillName 经营报告技能名。
	businessReportSkillName = "business-report"
	// attributionMaxRows 归因取数的默认行数上限（防子代理拉全表；仍以 result_id 句柄回传）。
	attributionMaxRows = 5000
)

// skillTaskFromInput 从 skill_invoke 透传的 JSON 入参中提取任务目标（task/question），
// 取不到时退化为整体入参文本。
func skillTaskFromInput(input string) string {
	var m map[string]interface{}
	if json.Unmarshal([]byte(input), &m) == nil {
		for _, key := range []string{"task", "question", "goal"} {
			if v, ok := m[key].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	return strings.TrimSpace(input)
}

// persistIntentKeywords 触发「写类下沉到 Operation」的意图关键词（仅显式要求落库/导出时才写）。
var persistIntentKeywords = []string{"导出", "落库", "入库", "持久化", "保存", "写入", "存表", "export"}

// wantsPersist 判断报告任务是否显式要求持久化/导出（否则纯只读产出，不触发写子代理）。
func wantsPersist(task string) bool {
	lower := strings.ToLower(task)
	for _, kw := range persistIntentKeywords {
		if strings.Contains(task, kw) || strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// attributionHandler 多维归因 handler：串行 inline 编排 SQLAuthor（取数）→ Analysis（归因）。
// 只回传最终归因结论；两阶段各自的内部工具往返都留在子代理上下文，不回灌主循环。
func attributionHandler(ctx context.Context, input string) (string, error) {
	task := skillTaskFromInput(input)
	if task == "" {
		return "", fmt.Errorf("多维归因技能需要明确的分析目标")
	}

	// 战术层第一步：委派 SQLAuthor 按归因维度取数（只读，回传 result_id）。
	fetch, err := infraagent.InlineDelegate(ctx, infraagent.DelegationEnvelope{
		AgentName: "SQLAuthor",
		Goal: fmt.Sprintf("为多维归因分析取数：%s。请按可能的归因维度（时间/地区/品类/渠道等）"+
			"分组取出目标指标，回传 result_id。", task),
		Inputs:      infraagent.DelegationInputs{Question: task},
		Constraints: infraagent.DelegationConstraints{Scope: infraagent.ScopeRead, MaxRows: attributionMaxRows},
		Return:      "result_id + 数据概览摘要",
	})
	if err != nil {
		return "", fmt.Errorf("多维归因取数阶段失败: %w", err)
	}

	// 战术层第二步：委派 Analysis 对上一步结果做多维对比与归因（result_id 经问题文本携入）。
	insight, err := infraagent.InlineDelegate(ctx, infraagent.DelegationEnvelope{
		AgentName: "Analysis",
		Goal: fmt.Sprintf("对上一步取数结果做多维对比与归因，定位对「%s」贡献最大的维度及其贡献度，"+
			"给出可执行结论。", task),
		Inputs:      infraagent.DelegationInputs{Question: fetch},
		Constraints: infraagent.DelegationConstraints{Scope: infraagent.ScopeRead},
		Return:      "归因结论（主因维度 + 贡献度 + result_id 引用）",
	})
	if err != nil {
		return "", fmt.Errorf("多维归因分析阶段失败: %w", err)
	}
	return insight, nil
}

// businessReportHandler 经营报告 handler：inline 编排 Report 编排者产出多主题洞察汇总（只读）；
// 仅当任务显式要求导出/落库时，才把结论下沉给 Operation 做写类操作（危险分级/确认协议全程生效）。
func businessReportHandler(ctx context.Context, input string) (string, error) {
	task := skillTaskFromInput(input)
	if task == "" {
		return "", fmt.Errorf("经营报告技能需要明确的报告主题")
	}

	// 只读产出：委派 Report 编排者拆主题、逐节委派 Insight，汇总成完整报告。
	report, err := infraagent.InlineDelegate(ctx, infraagent.DelegationEnvelope{
		AgentName: "Report",
		Goal: fmt.Sprintf("生成结构化经营报告：%s。拆成若干经营主题，逐一产出洞察并汇总成"+
			"含结论与 result_id 引用的完整报告。", task),
		Inputs:      infraagent.DelegationInputs{Question: task},
		Constraints: infraagent.DelegationConstraints{Scope: infraagent.ScopeRead},
		Return:      "结构化经营报告（各节结论 + 依据 result_id）",
	})
	if err != nil {
		return "", fmt.Errorf("经营报告生成阶段失败: %w", err)
	}

	if !wantsPersist(task) {
		return report, nil
	}

	// 写类下沉：经 Operation 子代理落库/导出（唯一持写子代理，写错误细节留在其上下文）。
	// 报告本身已产出——写阶段失败不吞报告，附注写状态回传，交战略层决策。
	handle, perr := infraagent.InlineDelegate(ctx, infraagent.DelegationEnvelope{
		AgentName: "Operation",
		Goal: fmt.Sprintf("将上述经营报告结论落库/导出：%s。写类操作遵守危险分级与人工确认协议，"+
			"超阈值产出 pending_action_id。", task),
		Inputs:      infraagent.DelegationInputs{Question: report},
		Constraints: infraagent.DelegationConstraints{Scope: infraagent.ScopeETL},
		Return:      "落库/导出句柄或 pending_action_id",
	})
	if perr != nil {
		return fmt.Sprintf("%s\n\n[导出/落库阶段未完成：%v]", report, perr), nil
	}
	return fmt.Sprintf("%s\n\n[导出/落库：%s]", report, handle), nil
}

// complexSkills 复杂任务下沉 skill 定义（可执行）。WhenToUse 明确限定复杂多步场景，
// 使简单单步任务不误命中（主 agent 直连不多跳）。
var complexSkills = []skills.Skill{
	{
		Name:        attributionSkillName,
		Description: "多维归因分析：对某指标的变化按多个维度拆解，定位主因维度与贡献度",
		WhenToUse: "仅当用户要求对某指标（如 GMV、转化率、成本）的变化做跨多个维度的归因/下钻/贡献度拆解时使用；" +
			"单一指标查询、单步统计或已知维度的简单对比请勿使用，直接作答。",
		Category:  "data-analysis",
		Tags:      []string{"归因", "多维", "下钻", "贡献度", "attribution"},
		CanInvoke: true,
		Handler:   attributionHandler,
		Content: "# 多维归因分析方法论\n" +
			"1. 明确目标指标与观察到的变化（同比/环比/异常）。\n" +
			"2. 按候选维度（时间/地区/品类/渠道/客群等）分组取数，回传 result_id。\n" +
			"3. 对各维度做贡献度分解，定位主因维度，量化其对总变化的贡献。\n" +
			"4. 输出结论：主因维度 + 贡献度 + 依据 result_id 引用。",
	},
	{
		Name:        businessReportSkillName,
		Description: "经营报告：拆解经营主题、逐节产出洞察并汇总成结构化报告，可选落库/导出",
		WhenToUse: "仅当用户要求产出跨多个主题的完整经营/分析报告时使用；" +
			"单一问题、单张图表或单步查询请勿使用，直接作答。仅当明确要求导出/落库时才触发写操作。",
		Category:  "data-analysis",
		Tags:      []string{"经营报告", "报告", "综合分析", "report"},
		CanInvoke: true,
		Handler:   businessReportHandler,
		Content: "# 经营报告方法论\n" +
			"1. 将报告拆成若干经营主题（增长/效率/风险等）。\n" +
			"2. 逐一（独立主题可并行）产出各主题洞察，携 result_id 依据。\n" +
			"3. 汇总成结构清晰的报告：结论先行 + 依据引用。\n" +
			"4. 仅当用户显式要求时，才把结论落库/导出（经受控写操作）。",
	},
}

// RegisterDataComplexSkills 幂等注册复杂任务下沉 skill 到全局技能管理器。
// 已注册则跳过（buildDataAgent 每次构建都会调用，需幂等）；注册失败仅告警不阻断构建
// （复杂 skill 缺失时退化为主 agent 直接编排，不影响简单任务）。
func RegisterDataComplexSkills() {
	manager := skills.GetGlobalManager()
	reg := manager.GetRegistry()
	for i := range complexSkills {
		s := &complexSkills[i]
		if _, exists := manager.GetSkill(s.Name); exists {
			continue
		}
		if err := reg.Register(s); err != nil {
			log.Printf("[Agent] 注册复杂任务 skill %s 失败: %v", s.Name, err)
		}
	}
}
