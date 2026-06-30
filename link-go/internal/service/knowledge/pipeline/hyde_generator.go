// Package rag 提供 HyDE (Hypothetical Document Embeddings) 实现
package rag

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	domainrag "link/internal/model/rag"
)

// ========================================
// HyDE Generator 实现
// ========================================

// HyDEGeneratorImpl HyDE 生成器实现
type HyDEGeneratorImpl struct {
	llm model.ChatModel
}

// NewHyDEGenerator 创建 HyDE 生成器
func NewHyDEGenerator(llm model.ChatModel) domainrag.HyDEGenerator {
	return &HyDEGeneratorImpl{
		llm: llm,
	}
}

// GenerateHypotheticalDoc 生成假设文档
func (h *HyDEGeneratorImpl) GenerateHypotheticalDoc(ctx context.Context, query string, opts *domainrag.HyDEOptions) (string, error) {
	if opts == nil {
		opts = domainrag.DefaultHyDEOptions()
	}

	// 构建生成 prompt
	prompt := h.buildHyDEPrompt(query, opts)

	// 构建消息
	messages := []*schema.Message{
		schema.SystemMessage(h.getSystemPrompt(opts)),
		schema.UserMessage(prompt),
	}

	// 调用 LLM 生成
	resp, err := h.llm.Generate(ctx, messages, model.WithTemperature(float32(opts.Temperature)), model.WithMaxTokens(opts.MaxTokens))
	if err != nil {
		return "", fmt.Errorf("LLM generation failed: %w", err)
	}

	// 提取假设文档
	hypotheticalDoc := h.extractHypotheticalDoc(resp.Content)
	return hypotheticalDoc, nil
}

// GenerateMultiple 生成多个假设文档
func (h *HyDEGeneratorImpl) GenerateMultiple(ctx context.Context, query string, count int, opts *domainrag.HyDEOptions) ([]string, error) {
	if count <= 0 {
		count = 3
	}

	docs := make([]string, 0, count)

	// 生成多个假设文档，使用不同的温度参数
	for i := 0; i < count; i++ {
		docOpts := *opts
		docOpts.Temperature = 0.5 + float64(i)*0.2 // 0.5, 0.7, 0.9

		doc, err := h.GenerateHypotheticalDoc(ctx, query, &docOpts)
		if err != nil {
			continue
		}
		docs = append(docs, doc)
	}

	return docs, nil
}

// ========================================
// Prompt 构建
// ========================================

// getSystemPrompt 获取系统 prompt
func (h *HyDEGeneratorImpl) getSystemPrompt(opts *domainrag.HyDEOptions) string {
	basePrompt := `你是一个专业的文档生成助手。你的任务是根据用户查询，生成一个假设性的、理想的答案文档。

生成的文档应该：
1. 直接回答用户的问题
2. 包含准确、具体的信息
3. 使用清晰的结构和格式
4. 保持在 200-500 字之间
5. 只输出文档内容，不要包含其他解释`

	if opts.Domain != "" {
		basePrompt += fmt.Sprintf(`

6. 专注于 %s 领域的专业内容`, opts.Domain)
	}

	return basePrompt
}

// buildHyDEPrompt 构建 HyDE prompt
func (h *HyDEGeneratorImpl) buildHyDEPrompt(query string, opts *domainrag.HyDEOptions) string {
	var builder strings.Builder

	builder.WriteString("请根据以下用户查询，生成一个假设性的理想答案文档：\n\n")
	builder.WriteString(fmt.Sprintf("用户查询：%s\n\n", query))

	if opts.UseConversationHistory && len(opts.ConversationHistory) > 0 {
		builder.WriteString("对话上下文：\n")
		for i, msg := range opts.ConversationHistory {
			builder.WriteString(fmt.Sprintf("%d. %s\n", i+1, msg))
		}
		builder.WriteString("\n")
	}

	builder.WriteString(`请生成一个直接、准确、结构清晰的假设性答案文档。
文档应该包含用户查询所需的所有关键信息。
只输出文档内容，不要添加任何前缀或后缀说明。`)

	return builder.String()
}

// extractHypotheticalDoc 提取假设文档内容
func (h *HyDEGeneratorImpl) extractHypotheticalDoc(content string) string {
	// 清理内容
	content = strings.TrimSpace(content)

	// 移除可能的前缀
	prefixes := []string{
		"假设性答案：",
		"假设文档：",
		"理想答案：",
		"文档：",
		"答案：",
	}

	for _, prefix := range prefixes {
		if strings.HasPrefix(content, prefix) {
			content = strings.TrimPrefix(content, prefix)
			content = strings.TrimSpace(content)
			break
		}
	}

	// 移除可能的 markdown 代码块标记
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) > 1 {
			content = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	return content
}

// ========================================
// Query Rewriter 实现
// ========================================

// QueryRewriterImpl 查询重写器实现
type QueryRewriterImpl struct {
	llm model.ChatModel
}

// NewQueryRewriter 创建查询重写器
func NewQueryRewriter(llm model.ChatModel) domainrag.QueryRewriter {
	return &QueryRewriterImpl{
		llm: llm,
	}
}

// RewriteQuery 重写查询
func (q *QueryRewriterImpl) RewriteQuery(ctx context.Context, query string, opts *domainrag.RewriteOptions) (*domainrag.RewrittenQuery, error) {
	if opts == nil {
		opts = domainrag.DefaultRewriteOptions()
	}

	// 构建重写 prompt
	prompt := q.buildRewritePrompt(query, opts)

	messages := []*schema.Message{
		schema.SystemMessage(q.getRewriteSystemPrompt()),
		schema.UserMessage(prompt),
	}

	resp, err := q.llm.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("query rewrite failed: %w", err)
	}

	result := &domainrag.RewrittenQuery{
		Original:  query,
		Rewritten: q.extractRewrittenQuery(resp.Content),
		Metadata:  make(map[string]interface{}),
	}

	// 提取关键词
	result.Keywords = q.extractKeywords(query, resp.Content)

	return result, nil
}

// ExpandQuery 扩展查询
func (q *QueryRewriterImpl) ExpandQuery(ctx context.Context, query string, count int, opts *domainrag.RewriteOptions) ([]string, error) {
	if count <= 0 {
		count = 3
	}

	prompt := fmt.Sprintf(`请为以下查询生成 %d 个不同的变体，用于提高检索覆盖率。

原始查询：%s

要求：
1. 每个变体应该从不同角度表达相同的信息需求
2. 使用不同的关键词和句式
3. 保持原意不变
4. 每行一个变体，不要编号`, count, query)

	messages := []*schema.Message{
		schema.UserMessage(prompt),
	}

	resp, err := q.llm.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("query expansion failed: %w", err)
	}

	// 解析扩展查询
	queries := q.parseExpandedQueries(resp.Content, count)

	return queries, nil
}

// DecomposeQuery 分解复杂查询
func (q *QueryRewriterImpl) DecomposeQuery(ctx context.Context, query string, opts *domainrag.RewriteOptions) ([]*domainrag.SubQuery, error) {
	prompt := fmt.Sprintf(`请分析以下复杂查询，将其分解为 2-4 个可独立检索的子查询。

查询：%s

对于每个子查询，请提供：
1. 子查询内容
2. 优先级（1=最高，3=最低）
3. 是否依赖其他子查询的结果

请以 JSON 格式输出：
{
  "sub_queries": [
    {"query": "子查询1", "priority": 1, "dependencies": []},
    {"query": "子查询2", "priority": 2, "dependencies": [0]}
  ]
}`, query)

	messages := []*schema.Message{
		schema.UserMessage(prompt),
	}

	resp, err := q.llm.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("query decomposition failed: %w", err)
	}

	// 解析子查询
	subQueries := q.parseSubQueries(resp.Content)

	return subQueries, nil
}

// ========================================
// Query Rewriter 辅助方法
// ========================================

// getRewriteSystemPrompt 获取重写系统 prompt
func (q *QueryRewriterImpl) getRewriteSystemPrompt() string {
	return `你是一个专业的查询优化助手。你的任务是重写用户查询，使其更适合信息检索。

重写原则：
1. 保持原意不变
2. 添加必要的上下文信息
3. 使用更精确的关键词
4. 移除冗余词汇
5. 输出重写后的查询，不要添加解释`
}

// buildRewritePrompt 构建重写 prompt
func (q *QueryRewriterImpl) buildRewritePrompt(query string, opts *domainrag.RewriteOptions) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("请重写以下查询，使其更适合信息检索：\n\n原始查询：%s\n\n", query))

	if len(opts.ConversationHistory) > 0 {
		builder.WriteString("对话历史（用于上下文理解）：\n")
		for _, msg := range opts.ConversationHistory {
			builder.WriteString(fmt.Sprintf("%s\n", msg))
		}
		builder.WriteString("\n")
	}

	if opts.Domain != "" {
		builder.WriteString(fmt.Sprintf("知识域：%s\n\n", opts.Domain))
	}

	builder.WriteString("请输出重写后的查询，不要添加任何解释。")

	return builder.String()
}

// extractRewrittenQuery 提取重写后的查询
func (q *QueryRewriterImpl) extractRewrittenQuery(content string) string {
	content = strings.TrimSpace(content)

	// 移除可能的前缀
	prefixes := []string{
		"重写后的查询：",
		"重写：",
		"优化后的查询：",
		"查询：",
	}

	for _, prefix := range prefixes {
		if strings.HasPrefix(content, prefix) {
			content = strings.TrimPrefix(content, prefix)
			content = strings.TrimSpace(content)
			break
		}
	}

	return content
}

// extractKeywords 提取关键词
func (q *QueryRewriterImpl) extractKeywords(original, rewritten string) []string {
	// 简单实现：从重写后的查询中提取名词短语
	// 实际应用中可以使用更复杂的 NLP 方法

	words := strings.Fields(rewritten)
	keywords := make([]string, 0)

	stopWords := map[string]bool{
		"的": true, "了": true, "是": true, "在": true,
		"和": true, "与": true, "或": true, "等": true,
		"请": true, "如何": true, "什么": true, "哪些": true,
	}

	for _, word := range words {
		if len(word) > 1 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}

	return keywords
}

// parseExpandedQueries 解析扩展查询
func (q *QueryRewriterImpl) parseExpandedQueries(content string, count int) []string {
	lines := strings.Split(content, "\n")
	queries := make([]string, 0, count)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 移除可能的编号
		if len(line) > 2 && (line[0] >= '1' && line[0] <= '9') && line[1] == '.' {
			line = strings.TrimPrefix(line, line[:2])
			line = strings.TrimSpace(line)
		}

		// 移除可能的 markdown 列表标记
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			line = strings.TrimPrefix(line, line[:2])
			line = strings.TrimSpace(line)
		}

		if line != "" {
			queries = append(queries, line)
		}

		if len(queries) >= count {
			break
		}
	}

	return queries
}

// parseSubQueries 解析子查询
func (q *QueryRewriterImpl) parseSubQueries(content string) []*domainrag.SubQuery {
	// 简化实现：返回单个子查询
	// 实际应用中应该解析 JSON 响应

	return []*domainrag.SubQuery{
		{
			Query:         strings.TrimSpace(content),
			Priority:      1,
			Dependencies:  []int{},
		},
	}
}
