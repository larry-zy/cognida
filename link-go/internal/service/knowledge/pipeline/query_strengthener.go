// Package rag 提供 RAG 基础设施层的查询增强器实现
package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"link/internal/model/llm"
	domainrag "link/internal/model/rag"
)

// ========================================
// Query Strengthener Implementation
// ========================================

// queryStrengthenerImpl 查询增强器实现
type queryStrengthenerImpl struct {
	llmClient llm.LLMClient
	config    *StrengthenerConfig
}

// StrengthenerConfig 增强器配置
type StrengthenerConfig struct {
	Temperature float64
	MaxTokens   int
}

// NewQueryStrengthener 创建查询增强器
func NewQueryStrengthener(llmClient llm.LLMClient, config *StrengthenerConfig) domainrag.QueryStrengthener {
	if config == nil {
		config = &StrengthenerConfig{
			Temperature: 0.1,
			MaxTokens:   2000,
		}
	}
	return &queryStrengthenerImpl{
		llmClient: llmClient,
		config:    config,
	}
}

// StrengthenQuery 增强查询（主入口）
func (q *queryStrengthenerImpl) StrengthenQuery(ctx context.Context, query string, conversationHistory string, opts *domainrag.StrengthOptions) (*domainrag.StrengthenedQuery, error) {
	startTime := time.Now()

	if opts == nil {
		opts = domainrag.DefaultStrengthOptions()
	}

	result := &domainrag.StrengthenedQuery{
		OriginalQuery: query,
	}

	// 步骤1: 查询重写
	if opts.EnableRewrite && q.shouldRewrite(query) {
		rewritten, err := q.RewriteQuery(ctx, query, conversationHistory, opts)
		if err == nil && rewritten != "" {
			result.RewrittenQuery = rewritten
			result.RewriteApplied = true
		}
	}

	// 步骤2: 查询拆分（使用重写后的查询或原查询）
	queryToSplit := query
	if result.RewrittenQuery != "" {
		queryToSplit = result.RewrittenQuery
	}

	if opts.EnableSplit && q.shouldSplit(queryToSplit) {
		subQueries, err := q.SplitQuery(ctx, queryToSplit, conversationHistory, opts)
		if err == nil && len(subQueries) > 0 {
			result.SubQueries = subQueries
			result.SplitApplied = true
		}
	}

	result.ProcessingTime = time.Since(startTime).Milliseconds()

	return result, nil
}

// RewriteQuery 重写查询
func (q *queryStrengthenerImpl) RewriteQuery(ctx context.Context, query string, conversationHistory string, opts *domainrag.StrengthOptions) (string, error) {
	// 构建重写提示词
	prompt := q.buildRewritePrompt(query, conversationHistory)

	// 调用 LLM
	response, err := q.callLLM(ctx, prompt, opts)
	if err != nil {
		return "", err
	}

	// 提取重写后的查询
	rewrittenQuery := q.extractRewrittenQuery(response)
	if rewrittenQuery == "" {
		return query, nil // 重写失败，返回原查询
	}

	return rewrittenQuery, nil
}

// SplitQuery 拆分查询
func (q *queryStrengthenerImpl) SplitQuery(ctx context.Context, query string, conversationHistory string, opts *domainrag.StrengthOptions) ([]string, error) {
	// 构建拆分提示词
	prompt := q.buildSplitPrompt(query, conversationHistory)

	// 调用 LLM
	response, err := q.callLLM(ctx, prompt, opts)
	if err != nil {
		return nil, err
	}

	// 提取拆分后的子查询
	subQueries := q.extractSubQueries(response)
	if len(subQueries) == 0 {
		return []string{query}, nil // 拆分失败，返回原查询
	}

	return subQueries, nil
}

// ========================================
// 提示词构建
// ========================================

// buildRewritePrompt 构建查询重写提示词
func (q *queryStrengthenerImpl) buildRewritePrompt(query, conversationHistory string) string {
	var prompt strings.Builder

	prompt.WriteString("# 查询重写\n\n")

	if conversationHistory != "" {
		prompt.WriteString("## 历史对话背景\n")
		prompt.WriteString(conversationHistory)
		prompt.WriteString("\n\n")
	}

	prompt.WriteString("## 当前查询\n")
	prompt.WriteString(query)
	prompt.WriteString("\n\n")

	prompt.WriteString("## 任务\n")
	prompt.WriteString("请根据历史对话背景（如有），将当前查询重写为一个完整、清晰、独立的查询。\n\n")
	prompt.WriteString("## 要求\n")
	prompt.WriteString("1. 重写后的查询应该完整、清晰，能够独立理解\n")
	prompt.WriteString("2. 替换代词（他、她、它、这个、那个等）为具体指代的内容\n")
	prompt.WriteString("3. 补充省略的信息\n")
	prompt.WriteString("4. 保持原意不变\n")
	prompt.WriteString("5. 只返回重写后的查询，不要解释\n\n")

	prompt.WriteString("## 输出格式\n")
	prompt.WriteString("请严格使用以下 JSON 格式输出（不要包含 markdown 代码块标记）：\n\n")
	prompt.WriteString(`{
  "rewritten_query": "重写后的查询内容",
  "original_query": "原始查询",
  "changes": "简要说明做了哪些修改"
}`)

	return prompt.String()
}

// buildSplitPrompt 构建查询拆分提示词
func (q *queryStrengthenerImpl) buildSplitPrompt(query, conversationHistory string) string {
	var prompt strings.Builder

	prompt.WriteString("# 查询拆分\n\n")

	if conversationHistory != "" {
		prompt.WriteString("## 历史对话背景\n")
		prompt.WriteString(conversationHistory)
		prompt.WriteString("\n\n")
	}

	prompt.WriteString("## 当前查询\n")
	prompt.WriteString(query)
	prompt.WriteString("\n\n")

	prompt.WriteString("## 任务\n")
	prompt.WriteString("将当前查询拆分为多个简单、独立的子查询。\n\n")

	prompt.WriteString("## 要求\n")
	prompt.WriteString("1. 每个子查询应该是独立的、可单独检索的\n")
	prompt.WriteString("2. 子查询之间尽量不重叠\n")
	prompt.WriteString("3. 按照逻辑顺序排列子查询\n")
	prompt.WriteString("4. 子查询数量不超过5个\n")
	prompt.WriteString("5. 如果查询本身就很简单，不需要拆分，直接返回原查询\n\n")

	prompt.WriteString("## 输出格式\n")
	prompt.WriteString("请严格使用以下 JSON 格式输出（不要包含 markdown 代码块标记）：\n\n")
	prompt.WriteString(`{
  "sub_queries": [
    {"id": "1", "query": "子查询1", "order": 1},
    {"id": "2", "query": "子查询2", "order": 2}
  ],
  "count": 2
}`)

	return prompt.String()
}

// ========================================
// LLM 调用
// ========================================

// callLLM 调用 LLM
func (q *queryStrengthenerImpl) callLLM(ctx context.Context, prompt string, opts *domainrag.StrengthOptions) (string, error) {
	// 构建请求
	messages := []*llm.Message{
		{
			Role:    llm.RoleUser,
			Content: prompt,
		},
	}

	chatOpts := &llm.ChatOptions{
		Temperature: q.config.Temperature,
		MaxTokens:   q.config.MaxTokens,
	}

	// 使用温度设置（如果提供了）
	if opts.Temperature > 0 {
		chatOpts.Temperature = opts.Temperature
	}
	if opts.MaxTokens > 0 {
		chatOpts.MaxTokens = opts.MaxTokens
	}

	req := &llm.ChatRequest{
		Messages: messages,
		Options:  chatOpts,
	}

	response, err := q.llmClient.Chat(ctx, req)
	if err != nil {
		return "", fmt.Errorf("LLM call failed: %w", err)
	}

	return response.Content, nil
}

// ========================================
// 结果提取
// ========================================

// RewriteResponse 重写响应结构
type RewriteResponse struct {
	RewrittenQuery string `json:"rewritten_query"`
	OriginalQuery  string `json:"original_query"`
	Changes        string `json:"changes,omitempty"`
}

// SplitResponse 拆分响应结构
type SplitResponse struct {
	SubQueries []SubQueryItem `json:"sub_queries"`
	Count      int            `json:"count"`
}

// SubQueryItem 子查询项
type SubQueryItem struct {
	ID           string   `json:"id"`
	Query        string   `json:"query"`
	Description  string   `json:"description,omitempty"`
	Order        int      `json:"order"`
	Dependencies []string `json:"dependencies,omitempty"`
}

// extractRewrittenQuery 从 LLM 响应中提取重写后的查询
func (q *queryStrengthenerImpl) extractRewrittenQuery(response string) string {
	response = strings.TrimSpace(response)

	// 策略1: 尝试提取 JSON 代码块
	if jsonStr := extractJSONCodeBlock(response); jsonStr != "" {
		var result RewriteResponse
		if err := json.Unmarshal([]byte(jsonStr), &result); err == nil && result.RewrittenQuery != "" {
			return strings.TrimSpace(result.RewrittenQuery)
		}
		// 尝试不同的 JSON 结构
		var altResult struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal([]byte(jsonStr), &altResult); err == nil && altResult.Query != "" {
			return strings.TrimSpace(altResult.Query)
		}
	}

	// 策略2: 尝试提取裸 JSON 对象（不在代码块中）
	if jsonStr := extractBareJSON(response); jsonStr != "" {
		var result RewriteResponse
		if err := json.Unmarshal([]byte(jsonStr), &result); err == nil && result.RewrittenQuery != "" {
			return strings.TrimSpace(result.RewrittenQuery)
		}
	}

	// 策略3: 尝试提取 "改写后的问题:" 后的内容
	re := regexp.MustCompile(`改写后的问题[:：]\s*\n*(.+?)(?:\n\n|$)`)
	matches := re.FindStringSubmatch(response)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// 策略4: 尝试提取 markdown 代码块
	re = regexp.MustCompile("```(?:text|plain)?\n*(.+?)\n*```")
	matches = re.FindStringSubmatch(response)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// 策略5: 如果响应简短且不是指令，直接使用
	if len(response) > 3 && len(response) < 200 &&
		!strings.Contains(response, "改写") &&
		!strings.Contains(response, "步骤") &&
		!strings.Contains(response, "注意") &&
		!strings.Contains(response, "以下是") {
		return response
	}

	return ""
}

// extractJSONCodeBlock 提取 JSON 代码块
func extractJSONCodeBlock(response string) string {
	// 匹配 ```json ... ``` 或 ``` ... ```
	re := regexp.MustCompile("```(?:json)?\\n*({[\\s\\S]*?})\\n*```")
	matches := re.FindStringSubmatch(response)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// extractBareJSON 提取裸 JSON 对象
func extractBareJSON(response string) string {
	// 查找最外层的 { ... }
	startIdx := strings.Index(response, "{")
	if startIdx == -1 {
		return ""
	}

	// 使用括号匹配找到对应的 }
	depth := 0
	inString := false
	escapeNext := false

	for i := startIdx; i < len(response); i++ {
		c := response[i]

		if escapeNext {
			escapeNext = false
			continue
		}

		if c == '\\' {
			escapeNext = true
			continue
		}

		if c == '"' {
			inString = !inString
			continue
		}

		if !inString {
			if c == '{' {
				depth++
			} else if c == '}' {
				depth--
				if depth == 0 {
					return strings.TrimSpace(response[startIdx : i+1])
				}
			}
		}
	}

	return ""
}

// extractSubQueries 从 LLM 响应中提取子查询
func (q *queryStrengthenerImpl) extractSubQueries(response string) []string {
	response = strings.TrimSpace(response)

	// 策略1: 尝试提取 JSON 代码块
	if jsonStr := extractJSONCodeBlock(response); jsonStr != "" {
		var result SplitResponse
		if err := json.Unmarshal([]byte(jsonStr), &result); err == nil && len(result.SubQueries) > 0 {
			return extractQueriesFromSplitResult(result)
		}
	}

	// 策略2: 尝试提取裸 JSON 对象
	if jsonStr := extractBareJSON(response); jsonStr != "" {
		var result SplitResponse
		if err := json.Unmarshal([]byte(jsonStr), &result); err == nil && len(result.SubQueries) > 0 {
			return extractQueriesFromSplitResult(result)
		}
	}

	// 策略3: 尝试提取简化的 JSON 格式（数组）
	re := regexp.MustCompile(`\[[\s\S]*?\]`)
	arrayMatch := re.FindString(response)
	var simpleArray []string
	if err := json.Unmarshal([]byte(arrayMatch), &simpleArray); err == nil && len(simpleArray) > 0 {
		// 过滤有效查询
		var validQueries []string
		for _, q := range simpleArray {
			if q != "" && len(q) > 2 && !strings.HasPrefix(q, "步骤") {
				validQueries = append(validQueries, q)
			}
		}
		if len(validQueries) > 0 {
			return validQueries
		}
	}

	// 策略4: 尝试提取列表格式
	re = regexp.MustCompile(`(?:\d+\.|[-*])\s*["']?(.+?)["']?(?:\n|$)`)
	matches := re.FindAllStringSubmatch(response, -1)

	var subQueries []string
	for _, match := range matches {
		if len(match) > 1 {
			query := strings.TrimSpace(match[1])
			// 过滤掉非查询内容
			if query != "" &&
				!strings.HasPrefix(query, "步骤") &&
				!strings.HasPrefix(query, "注意") &&
				!strings.HasPrefix(query, "示例") &&
				len(query) > 2 {
				subQueries = append(subQueries, query)
			}
		}
	}

	// 策略5: 按行分割，过滤说明文字
	if len(subQueries) == 0 {
		lines := strings.Split(response, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && len(line) > 2 && len(line) < 100 &&
				!strings.HasPrefix(line, "#") &&
				!strings.HasPrefix(line, "##") &&
				!strings.Contains(line, "拆分结果") &&
				!strings.Contains(line, "子查询") {
				subQueries = append(subQueries, line)
			}
		}
	}

	return subQueries
}

// extractQueriesFromSplitResult 从 SplitResponse 提取查询列表
func extractQueriesFromSplitResult(result SplitResponse) []string {
	// 按 order 排序
	items := result.SubQueries
	// 先按 order 排序
	for i := 0; i < len(items)-1; i++ {
		for j := i + 1; j < len(items); j++ {
			if items[i].Order > items[j].Order {
				items[i], items[j] = items[j], items[i]
			}
		}
	}

	// 提取查询
	var queries []string
	for _, item := range items {
		if item.Query != "" && len(item.Query) > 2 {
			queries = append(queries, strings.TrimSpace(item.Query))
		}
	}

	// 如果没有有效的 order，直接返回所有查询
	if len(queries) == 0 {
		for _, item := range result.SubQueries {
			if item.Query != "" {
				queries = append(queries, strings.TrimSpace(item.Query))
			}
		}
	}

	return queries
}


// ========================================
// 判断逻辑
// ========================================

// shouldRewrite 判断是否需要重写查询
func (q *queryStrengthenerImpl) shouldRewrite(query string) bool {
	// 过于简短的查询不需要重写
	if len(query) < 5 {
		return false
	}

	// 包含代词的查询需要重写
	pronouns := []string{"他", "她", "它", "他们", "它们", "这", "那", "这个", "那个", "这里", "那里"}
	for _, p := range pronouns {
		if strings.Contains(query, p) {
			return true
		}
	}

	// 包含省略号的查询需要重写
	if strings.Contains(query, "...") || strings.Contains(query, "…") {
		return true
	}

	// 问号开头的疑问句
	if strings.HasPrefix(strings.TrimSpace(query), "？") ||
		strings.HasPrefix(strings.TrimSpace(query), "?") {
		return true
	}

	// 过于简短的查询可能需要补充
	if len(query) < 15 && strings.Contains(query, "怎么") ||
		len(query) < 15 && strings.Contains(query, "如何") {
		return true
	}

	return false
}

// shouldSplit 判断是否需要拆分查询
func (q *queryStrengthenerImpl) shouldSplit(query string) bool {
	query = strings.ToLower(query)

	// 包含多个问题的查询
	questionCount := 0
	questionWords := []string{"什么是", "如何", "怎么", "为什么", "哪个", "哪些",
		"what is", "how to", "why", "which", "list", "列出"}

	for _, word := range questionWords {
		if strings.Contains(query, word) {
			questionCount++
		}
	}

	if questionCount >= 2 {
		return true
	}

	// 包含"和"、"与"、"以及"等连接词，且长度较长
	if (strings.Contains(query, "和") || strings.Contains(query, "与") ||
		strings.Contains(query, "以及") || strings.Contains(query, "和")) &&
		len(query) > 20 {
		return true
	}

	// 包含"区别"、"对比"、"比较"等词
	comparisonWords := []string{"区别", "对比", "比较", "差异", "不同",
		"difference", "compare", "vs", "versus"}
	for _, word := range comparisonWords {
		if strings.Contains(query, word) {
			return true
		}
	}

	// 包含"列举"、"所有"等词
	if strings.Contains(query, "列举") || strings.Contains(query, "所有") {
		return true
	}

	// 包含"步骤"、"流程"等词，且查询较长
	if (strings.Contains(query, "步骤") || strings.Contains(query, "流程")) &&
		len(query) > 20 {
		return true
	}

	return false
}
