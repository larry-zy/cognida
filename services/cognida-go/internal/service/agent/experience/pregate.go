package experience

import (
	"encoding/json"
	"strings"

	domain_conversation "cognida/internal/model/conversation"
)

// negativeFeedbackMarkers 是「用户末轮明确表达不满/否定」的判定词。
// 只在会话最后一条用户消息里匹配，避免会话中途的纠偏被误判为失败——
// 中途被纠正后又解决好的会话仍应沉淀，只有「用户最后仍在否定」才算失败收尾。
var negativeFeedbackMarkers = []string{
	"不对", "不太对", "错了", "答错", "答非所问", "驴唇不对马嘴",
	"重来", "重新做", "再试", "没用", "不管用", "没解决", "还是不行", "依然不行",
	"不是这个", "不是我要的", "你没懂", "理解错", "牛头不对马嘴",
}

// objectiveFailureGate 是送蒸馏前的客观失败前置门（方案 B）：用不依赖 LLM 的廉价信号
// 判定这段会话是否「明显失败」，命中则直接跳过蒸馏，省一次 LLM 调用、也堵住 LLM 的漏判。
//
// 纯函数：只读消息切片、不触网，可单测。messages 须按时间正序（最早在前）。
// 判定保守（宁放过不错杀）——只在高置信失败信号下返回 skip，返回 (true, 原因)；否则 (false, "")。
func objectiveFailureGate(messages []*domain_conversation.Message) (skip bool, reason string) {
	lastUser := lastMessageByRole(messages, domain_conversation.RoleUser)
	lastAssistant := lastMessageByRole(messages, domain_conversation.RoleAssistant)

	// 信号①：全程没有任何助手回复——无成果可沉淀。
	if lastAssistant == nil {
		return true, "无任何助手回复"
	}

	// 信号②：末条助手回复未正常收尾（流式中断/超时/异常退出）。
	if !lastAssistant.IsCompleted {
		return true, "末轮助手回复未正常收尾"
	}

	// 信号③：末轮助手既无正文也无有效产出（工具调用/知识引用），是空回合。
	if strings.TrimSpace(lastAssistant.Content) == "" &&
		!lastAssistant.HasToolCalls() && !lastAssistant.HasKnowledgeReferences() {
		return true, "末轮助手无正文且无产出"
	}

	// 信号④：末轮助手的工具调用全部报错（有调用但无一成功）——本轮取数/操作整体失败。
	if allToolCallsFailed(lastAssistant.ToolCalls) {
		return true, "末轮工具调用全部失败"
	}

	// 信号⑤：会话以用户否定收尾——用户最后一句仍在明确表达不满/未解决。
	if lastUser != nil && isAfter(lastUser, lastAssistant) &&
		containsAny(lastUser.Content, negativeFeedbackMarkers) {
		return true, "用户末轮负反馈"
	}

	return false, ""
}

// lastMessageByRole 返回指定角色的最后一条消息（messages 时间正序），无则 nil。
func lastMessageByRole(messages []*domain_conversation.Message, role string) *domain_conversation.Message {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i] != nil && messages[i].Role == role {
			return messages[i]
		}
	}
	return nil
}

// isAfter 判断 a 是否发生在 b 之后（用于确认「用户否定」发生在最后一次助手回复之后）。
func isAfter(a, b *domain_conversation.Message) bool {
	if a == nil || b == nil {
		return false
	}
	return a.CreatedAt.After(b.CreatedAt)
}

// containsAny 报告 s 是否包含 markers 中任一子串。
func containsAny(s string, markers []string) bool {
	for _, m := range markers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

// allToolCallsFailed 解析落库的 tool_calls JSON：存在工具调用、且每个调用都带非空 error 时返回 true。
// 无工具调用（空/非数组）返回 false（不作失败判定）。宽松解析：只看每个元素的 "error" 字段是否非空，
// 结构异常一律按「不能判定为全失败」处理（保守，宁放过）。
func allToolCallsFailed(toolCallsJSON string) bool {
	s := strings.TrimSpace(toolCallsJSON)
	if s == "" || s == "[]" || s == "null" {
		return false
	}
	var calls []struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(s), &calls); err != nil || len(calls) == 0 {
		return false
	}
	for _, c := range calls {
		if strings.TrimSpace(c.Error) == "" {
			return false // 有任一调用未报错 → 非全失败
		}
	}
	return true
}
