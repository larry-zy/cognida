// Package llm provides LLM infrastructure implementations.
package llm

import (
	"link/internal/model/llm"
	domainmemory "link/internal/model/memory"
)

// ========================================
// TokenCounter - Token 计数器实现
// ========================================

// tokenCounter 实现 Token 计数器
type tokenCounter struct{}

// NewTokenCounter 创建一个新的 Token 计数器
func NewTokenCounter() llm.TokenCounter {
	return &tokenCounter{}
}

// NewMemoryTokenCounter 创建 Memory 专用的 Token 计数器
func NewMemoryTokenCounter() domainmemory.TokenCounter {
	return &tokenCounter{}
}

// CountTokens 计算 token 数量
func (c *tokenCounter) CountTokens(text string) int {
	return c.Count(text)
}

// CountMessageTokens 计算消息 token 数量
func (c *tokenCounter) CountMessageTokens(messages []*llm.Message) int {
	total := 0
	for _, msg := range messages {
		// role 通常占少量 token
		total += c.CountTokens(string(msg.Role)) + c.CountTokens(msg.Content) + 4
	}
	return total
}

// Estimate 估算 token 数量（快速但不精确）
func (c *tokenCounter) Estimate(text string) int {
	return c.Count(text)
}

// Count 计算 token 数量（memory.TokenCounter 接口）
func (c *tokenCounter) Count(text string) int {
	if text == "" {
		return 0
	}

	// 简单估算：计算字符数
	runes := []rune(text)
	chineseCount := 0
	otherCount := 0

	for _, r := range runes {
		if isChinese(r) {
			chineseCount++
		} else {
			otherCount++
		}
	}

	// 估算：中文约 0.7 token/字符，其他约 0.25 token/字符
	tokens := int(float64(chineseCount)*0.7) + int(float64(otherCount)*0.25)

	// 至少返回 1
	if tokens == 0 && len(text) > 0 {
		return 1
	}

	return tokens
}

// CountMessages 计算消息列表的 token 数量
func (c *tokenCounter) CountMessages(messages []*domainmemory.Message) int {
	total := 0
	for _, msg := range messages {
		total += c.Count(msg.Content) + 4 // 4 for structure overhead
	}
	return total
}

// CountApprox 粗略估算（更快，不精确）
func (c *tokenCounter) CountApprox(text string) int {
	// 更快的估算：直接按字符数
	return len(text) / 3
}

// isChinese 判断是否为中文字符
func isChinese(r rune) bool {
	return r >= 0x4e00 && r <= 0x9fff
}

// EstimateMessageTokens 估算消息的 token 数量
func EstimateMessageTokens(role, content string) int {
	counter := &tokenCounter{}
	return counter.CountTokens(role) + counter.CountTokens(content) + 4
}

// EstimateMessagesTokens 批量估算消息的 token 数量
func EstimateMessagesTokens(messages []struct{ Role, Content string }) int {
	total := 0
	for _, msg := range messages {
		total += EstimateMessageTokens(msg.Role, msg.Content)
	}
	return total
}
