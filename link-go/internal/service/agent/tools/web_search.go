// Package tool 提供增强的网络搜索工具
// 基于 Agentic RAG 最佳实践实现
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"link/internal/infrastructure/config"
	agenttools "link/internal/model/agent/tools"
)

// ========================================
// Metaso 搜索客户端
// ========================================

// MetasoClient Metaso 搜索客户端
type MetasoClient struct {
	apiKey      string
	apiEndpoint string
	httpClient  *http.Client
}

// NewMetasoClient 创建 Metaso 客户端
func NewMetasoClient(cfg *config.SearchConfig) *MetasoClient {
	return &MetasoClient{
		apiKey:      cfg.MetasoAPIKey,
		apiEndpoint: cfg.APIEndpoint,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// MetasoSearchRequest Metaso 搜索请求
type MetasoSearchRequest struct {
	Q                 string `json:"q"`                 // 查询内容
	Scope             string `json:"scope"`             // 搜索范围: webpage
	IncludeSummary    bool   `json:"includeSummary"`    // 是否包含摘要
	Size              int    `json:"size"`              // 返回结果数量
	IncludeRawContent bool   `json:"includeRawContent"` // 是否包含原始内容
	ConciseSnippet    bool   `json:"conciseSnippet"`    // 是否简洁摘要
}

// MetasoSearchResponse Metaso 搜索响应
type MetasoSearchResponse struct {
	Credits          int                 `json:"credits"`
	SearchParameters MetasoSearchRequest `json:"searchParameters"`
	Webpages         []MetasoResultItem  `json:"webpages"`
	Total            int                 `json:"total"`
}

// MetasoResultItem Metaso 搜索结果项
type MetasoResultItem struct {
	Title    string   `json:"title"`
	Link     string   `json:"link"`
	Score    string   `json:"score"`
	Snippet  string   `json:"snippet"`
	Position int      `json:"position"`
	Date     string   `json:"date,omitempty"`
	Authors  []string `json:"authors,omitempty"`
}

// Search 执行搜索
func (c *MetasoClient) Search(ctx context.Context, query string, size int) (*MetasoSearchResponse, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("METASO_API_KEY is not configured")
	}

	// 构建请求体
	reqBody := MetasoSearchRequest{
		Q:                 query,
		Scope:             "webpage",
		IncludeSummary:    false,
		Size:              size,
		IncludeRawContent: false,
		ConciseSnippet:    false,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// 创建 HTTP 请求
	req, err := http.NewRequestWithContext(ctx, "POST", c.apiEndpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置请求头
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned error status %d: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var result MetasoSearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w, body: %s", err, string(body))
	}

	return &result, nil
}

// ========================================
// 工具 1: web_search - 增强的网络搜索工具
// ========================================

// NewWebSearchTool 创建网络搜索工具；client 搜索客户端（可为 nil，nil 时降级返回模拟数据）经参数注入。
func NewWebSearchTool(client *MetasoClient) *TypedBaseTool[agenttools.WebSearchRequest, agenttools.WebSearchResult] {
	handler := func(ctx context.Context, req *agenttools.WebSearchRequest) (*agenttools.WebSearchResult, error) {
		return webSearch(ctx, req, client)
	}
	return NewTypedBaseTool(
		"web_search",
		`在互联网上搜索实时信息。这是获取最新资讯和公开资料的主要工具。

**核心能力**:
1. 实时搜索 - 获取最新新闻、技术资讯、市场动态
2. 多领域覆盖 - 科技、财经、时事、百科等
3. 结果评分 - 按相关性排序返回最相关的结果

**适用场景**:
- 时事查询："最近 AI 领域有什么新进展"
- 最新数据："当前的汇率是多少"、"某公司股价"
- 技术资料："Go 语言最新特性"、"Docker 最佳实践"
- 补充信息：当知识库信息不足时

**与知识库检索的配合**:
- 知识库检索：企业内部文档、历史资料
- 网络搜索：最新资讯、公开资料
- 建议流程：先用 rag_query，结果不足时用 web_search

**参数说明**:
- query: 搜索关键词（必需）
- limit: 返回数量（可选，默认5，最大10）
- time_range: 时间范围（可选，day/week/month/year/all）
- domain: 限制域名（可选，如 github.com）
- search_depth: 搜索深度（可选，basic/advanced）`,
	handler,
	)
}

// webSearch 执行网络搜索；metasoClient 为 nil 时降级返回模拟数据。
func webSearch(ctx context.Context, req *agenttools.WebSearchRequest, metasoClient *MetasoClient) (*agenttools.WebSearchResult, error) {
	startTime := time.Now()

	// 1. 参数验证
	if req.Query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}

	// 设置默认值
	if req.Limit <= 0 {
		req.Limit = 5
	}
	if req.Limit > 10 {
		req.Limit = 10
	}
	if req.TimeRange == "" {
		req.TimeRange = "all"
	}
	if req.SearchDepth == "" {
		req.SearchDepth = "basic"
	}

	// 2. 查询优化（基于最佳实践）
	optimizedQuery := optimizeSearchQuery(req.Query, req.TimeRange)

	// 3. 执行搜索
	var items []agenttools.SearchItem
	var isFallback bool

	if metasoClient == nil {
		// 降级：返回模拟数据
		isFallback = true
		items = generateMockSearchResults(optimizedQuery, req.Limit)
	} else {
		// 调用真实 API
		result, err := metasoClient.Search(ctx, optimizedQuery, req.Limit)
		if err != nil {
			// API 失败时使用模拟数据
			isFallback = true
			items = generateMockSearchResults(optimizedQuery, req.Limit)
		} else {
			items = convertMetasoResults(result.Webpages)
		}
	}

	// 4. 结果后处理
	items = filterByDomain(items, req.Domain)
	items = deduplicateResults(items)
	items = scoreResults(items, req.Query)

	// 5. 生成摘要
	summary := generateSearchSummary(req.Query, items, isFallback)

	return &agenttools.WebSearchResult{
		Query:     req.Query,
		Items:     items,
		Count:     len(items),
		LatencyMs: time.Since(startTime).Milliseconds(),
		HasAnswer: len(items) > 0,
		Summary:   summary,
		IsFallback: isFallback,
	}, nil
}

// ========================================
// 工具 2: fetch_url - 网页内容提取工具
// ========================================

// FetchURLRequest 网页内容提取请求
type FetchURLRequest struct {
	// URL 要提取内容的网页地址
	URL string `json:"url" jsonschema:"required,description=要提取内容的网页URL"`

	// MaxLength 最大内容长度，默认10000
	MaxLength int `json:"max_length" jsonschema:"description=最大内容长度，默认10000"`

	// ExtractLinks 是否提取链接
	ExtractLinks bool `json:"extract_links" jsonschema:"description=是否提取页面中的链接，默认false"`
}

// FetchURLResult 网页内容提取结果
type FetchURLResult struct {
	// URL 原始URL
	URL string `json:"url"`

	// Title 页面标题
	Title string `json:"title"`

	// Content 页面内容
	Content string `json:"content"`

	// ContentType 内容类型
	ContentType string `json:"content_type"`

	// Links 提取的链接（如果启用）
	Links []string `json:"links,omitempty"`

	// WordCount 字数统计
	WordCount int `json:"word_count"`

	// LatencyMs 提取耗时
	LatencyMs int64 `json:"latency_ms"`
}

// NewFetchURLTool 创建网页内容提取工具
func NewFetchURLTool() *TypedBaseTool[FetchURLRequest, FetchURLResult] {
	return NewTypedBaseTool(
		"fetch_url",
		`获取指定URL的完整网页内容。

**适用场景**:
- 需要获取文章的完整内容（而不仅仅是搜索摘要）
- 验证搜索结果中的信息
- 深入了解某个网页的详细内容

**使用流程**:
1. 先使用 web_search 获取相关网页
2. 选择感兴趣的 URL
3. 使用 fetch_url 获取完整内容

**参数说明**:
- url: 网页URL（必需）
- max_length: 最大内容长度（可选，默认10000）
- extract_links: 是否提取链接（可选，默认false）

**注意事项**:
- 请遵守目标网站的 robots.txt
- 仅用于合法的信息获取
- 部分网站可能有反爬虫机制`,
		fetchURL,
	)
}

// fetchURL 获取网页内容
func fetchURL(ctx context.Context, req *FetchURLRequest) (*FetchURLResult, error) {
	startTime := time.Now()

	// 1. 参数验证
	if req.URL == "" {
		return nil, fmt.Errorf("url cannot be empty")
	}

	// 验证 URL 格式
	parsedURL, err := url.Parse(req.URL)
	if err != nil || parsedURL.Scheme == "" {
		return nil, fmt.Errorf("invalid url format: %s", req.URL)
	}

	// 设置默认值
	if req.MaxLength <= 0 {
		req.MaxLength = 10000
	}

	// 2. 发送 HTTP 请求
	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	httpReq, err := http.NewRequestWithContext(ctx, "GET", req.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置 User-Agent
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AgenticRAG/1.0)")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	// 3. 读取内容
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// 4. 处理内容
	contentType := resp.Header.Get("Content-Type")
	content := string(body)

	// 简单的 HTML 清理
	content = cleanHTML(content)

	// 截断内容
	if len(content) > req.MaxLength {
		content = content[:req.MaxLength] + "..."
	}

	// 提取标题
	title := extractTitle(req.URL, content)

	// 5. 提取链接（如果需要）
	var links []string
	if req.ExtractLinks {
		links = extractLinks(content)
	}

	return &FetchURLResult{
		URL:         req.URL,
		Title:       title,
		Content:     content,
		ContentType: contentType,
		Links:       links,
		WordCount:   len(strings.Fields(content)),
		LatencyMs:   time.Since(startTime).Milliseconds(),
	}, nil
}

// ========================================
// 工具 3: search_multi - 多源搜索聚合工具
// ========================================

// SearchMultiRequest 多源搜索请求
type SearchMultiRequest struct {
	// Queries 搜索查询列表（多个变体）
	Queries []string `json:"queries" jsonschema:"required,description=搜索查询列表"`

	// Limit 每个查询返回的数量
	Limit int `json:"limit" jsonschema:"description=每个查询返回数量，默认3"`

	// MergeStrategy 合并策略：deduplicate/rank/all
	MergeStrategy string `json:"merge_strategy" jsonschema:"description=合并策略:deduplicate/rank/all，默认rank"`
}

// SearchMultiResult 多源搜索结果
type SearchMultiResult struct {
	// Queries 执行的查询列表
	Queries []string `json:"queries"`

	// TotalResults 总结果数
	TotalResults int `json:"total_results"`

	// MergedItems 合并后的结果
	MergedItems []agenttools.SearchItem `json:"merged_items"`

	// PerQueryResults 每个查询的结果
	PerQueryResults []WebQueryResult `json:"per_query_results"`

	// LatencyMs 总耗时
	LatencyMs int64 `json:"latency_ms"`
}

// WebQueryResult 单个查询的结果
type WebQueryResult struct {
	Query string             `json:"query"`
	Items []agenttools.SearchItem `json:"items"`
	Count int                `json:"count"`
}

// NewSearchMultiTool 创建多源搜索工具；client 搜索客户端（可为 nil）经参数注入。
func NewSearchMultiTool(client *MetasoClient) *TypedBaseTool[SearchMultiRequest, SearchMultiResult] {
	handler := func(ctx context.Context, req *SearchMultiRequest) (*SearchMultiResult, error) {
		return searchMulti(ctx, req, client)
	}
	return NewTypedBaseTool(
		"search_multi",
		`使用多个查询变体进行搜索，并合并结果。

**适用场景**:
- 需要从多个角度搜索同一主题
- 提高搜索覆盖率
- 获取更全面的信息

**工作流程**:
1. 接受多个查询变体（如 "AI 最新进展"、"人工智能突破"）
2. 并发执行所有搜索
3. 根据策略合并结果
4. 返回去重、排序后的结果

**参数说明**:
- queries: 查询列表（必需）
- limit: 每个查询的结果数（可选，默认3）
- merge_strategy: 合并策略（可选，deduplicate/rank/all）

**合并策略**:
- deduplicate: 去重后合并
- rank: 按相关性排序
- all: 保留所有结果`,
		handler,
	)
}

// searchMulti 执行多源搜索；client 透传给每个并发 webSearch。
func searchMulti(ctx context.Context, req *SearchMultiRequest, client *MetasoClient) (*SearchMultiResult, error) {
	startTime := time.Now()

	// 1. 参数验证
	if len(req.Queries) == 0 {
		return nil, fmt.Errorf("queries cannot be empty")
	}
	if len(req.Queries) > 5 {
		return nil, fmt.Errorf("too many queries, max 5")
	}

	// 设置默认值
	if req.Limit <= 0 {
		req.Limit = 3
	}
	if req.MergeStrategy == "" {
		req.MergeStrategy = "rank"
	}

	// 2. 并发执行搜索
	type resultPair struct {
		query string
		items []agenttools.SearchItem
		err   error
	}

	resultChan := make(chan resultPair, len(req.Queries))

	for _, query := range req.Queries {
		go func(q string) {
			req := &agenttools.WebSearchRequest{
				Query: q,
				Limit: req.Limit,
			}
			result, err := webSearch(ctx, req, client)
			if err != nil || result == nil {
				// 出错时 result 为 nil，解引用 result.Items 会 panic 并击穿整个进程。
				resultChan <- resultPair{query: q, err: err}
				return
			}
			resultChan <- resultPair{
				query: q,
				items: result.Items,
				err:   err,
			}
		}(query)
	}

	// 3. 收集结果
	perQueryResults := make([]WebQueryResult, 0, len(req.Queries))
	allItems := make([]agenttools.SearchItem, 0)

	for i := 0; i < len(req.Queries); i++ {
		r := <-resultChan
		if r.err != nil {
			continue
		}
		perQueryResults = append(perQueryResults, WebQueryResult{
			Query: r.query,
			Items: r.items,
			Count: len(r.items),
		})
		allItems = append(allItems, r.items...)
	}

	// 4. 合并结果
	mergedItems := mergeSearchResults(allItems, req.MergeStrategy)

	return &SearchMultiResult{
		Queries:         req.Queries,
		TotalResults:    len(allItems),
		MergedItems:     mergedItems,
		PerQueryResults: perQueryResults,
		LatencyMs:       time.Since(startTime).Milliseconds(),
	}, nil
}

// ========================================
// 辅助函数
// ========================================

// optimizeSearchQuery 优化搜索查询
func optimizeSearchQuery(query string, timeRange string) string {
	optimized := query

	// 添加时间范围相关的关键词
	switch timeRange {
	case "day":
		optimized += " 今天 最新"
	case "week":
		optimized += " 本周 最新"
	case "month":
		optimized += " 本月 最新"
	case "year":
		optimized += " 今年"
	}

	return strings.TrimSpace(optimized)
}

// convertMetasoResults 转换 Metaso 结果
func convertMetasoResults(items []MetasoResultItem) []agenttools.SearchItem {
	results := make([]agenttools.SearchItem, 0, len(items))
	for _, item := range items {
		snippet := item.Snippet
		if snippet == "" {
			snippet = "无摘要"
		}

		// 提取来源域名
		source := extractSource(item.Link)

		results = append(results, agenttools.SearchItem{
			Title:         item.Title,
			URL:           item.Link,
			Snippet:       snippet,
			Score:         0.5, // 初始分数，后续会重新计算
			PublishedDate: item.Date,
			Source:        source,
		})
	}
	return results
}

// extractSource 从 URL 提取来源
func extractSource(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "unknown"
	}
	return u.Hostname()
}

// filterByDomain 按域名过滤
func filterByDomain(items []agenttools.SearchItem, domain string) []agenttools.SearchItem {
	if domain == "" {
		return items
	}

	filtered := make([]agenttools.SearchItem, 0)
	for _, item := range items {
		if strings.Contains(item.URL, domain) || strings.Contains(item.Source, domain) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// deduplicateResults 去重结果
func deduplicateResults(items []agenttools.SearchItem) []agenttools.SearchItem {
	seen := make(map[string]bool)
	result := make([]agenttools.SearchItem, 0, len(items))

	for _, item := range items {
		// 使用 URL 作为去重标识
		key := normalizeURL(item.URL)
		if !seen[key] {
			seen[key] = true
			result = append(result, item)
		}
	}

	return result
}

// normalizeURL 标准化 URL
func normalizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	// 移除跟踪参数
	query := u.Query()
	query.Del("utm_source")
	query.Del("utm_medium")
	query.Del("utm_campaign")
	query.Del("fbclid")
	query.Del("gclid")
	u.RawQuery = query.Encode()
	return u.String()
}

// scoreResults 对结果进行评分
func scoreResults(items []agenttools.SearchItem, query string) []agenttools.SearchItem {
	queryLower := strings.ToLower(query)
	queryWords := strings.Fields(queryLower)

	for i := range items {
		score := 0.5 // 基础分数

		titleLower := strings.ToLower(items[i].Title)
		snippetLower := strings.ToLower(items[i].Snippet)

		// 标题匹配加分
		for _, word := range queryWords {
			if strings.Contains(titleLower, word) {
				score += 0.15
			}
			if strings.Contains(snippetLower, word) {
				score += 0.05
			}
		}

		// 完整短语匹配加分
		if strings.Contains(titleLower, queryLower) {
			score += 0.2
		}

		if score > 1.0 {
			score = 1.0
		}

		items[i].Score = score
	}

	// 按分数排序
	for i := 0; i < len(items)-1; i++ {
		for j := i + 1; j < len(items); j++ {
			if items[i].Score < items[j].Score {
				items[i], items[j] = items[j], items[i]
			}
		}
	}

	return items
}

// generateSearchSummary 生成搜索摘要
func generateSearchSummary(query string, items []agenttools.SearchItem, isFallback bool) string {
	if len(items) == 0 {
		if isFallback {
			return fmt.Sprintf("⚠️ 网络搜索服务未配置，无法查找关于「%s」的信息。请配置 METASO_API_KEY 以启用网络搜索功能。", query)
		}
		return fmt.Sprintf("未找到关于「%s」的搜索结果", query)
	}

	var sb strings.Builder
	if isFallback {
		sb.WriteString("⚠️ 当前使用降级模式（网络搜索服务未配置）\n")
		sb.WriteString(fmt.Sprintf("关于「%s」找到 %d 个模拟结果\n\n", query, len(items)))
	} else {
		sb.WriteString(fmt.Sprintf("关于「%s」找到 %d 个结果\n\n", query, len(items)))
	}

	for i, item := range items {
		if i >= 3 {
			break
		}
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, item.Title))
		sb.WriteString(fmt.Sprintf("   %s\n", item.Snippet))
		if item.Source != "" {
			sb.WriteString(fmt.Sprintf("   来源: %s\n", item.Source))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// generateMockSearchResults 生成模拟搜索结果
func generateMockSearchResults(query string, count int) []agenttools.SearchItem {
	items := make([]agenttools.SearchItem, 0, count)

	for i := 0; i < count && i < 5; i++ {
		items = append(items, agenttools.SearchItem{
			Title:   fmt.Sprintf("关于「%s」的搜索结果 #%d", query, i+1),
			URL:     fmt.Sprintf("https://example.com/result%d", i+1),
			Snippet: fmt.Sprintf("这是关于「%s」的第 %d 个搜索结果摘要。实际使用中，这里会包含从网络搜索 API 获取的真实摘要内容。", query, i+1),
			Score:   0.9 - float64(i)*0.1,
			Source:  "example.com",
		})
	}

	return items
}

// cleanHTML 简单的 HTML 清理
func cleanHTML(content string) string {
	// 移除 script 和 style 标签
	content = removeHTMLTag(content, "script")
	content = removeHTMLTag(content, "style")

	// 移除所有 HTML 标签
	content = removeTags(content)

	// 清理多余空白
	content = collapseWhitespace(content)

	return content
}

// removeHTMLTag 移除指定标签及其内容
func removeHTMLTag(content, tag string) string {
	openTag := "<" + tag
	closeTag := "</" + tag + ">"

	var result strings.Builder
	inTag := false

	for i := 0; i < len(content); i++ {
		if i+len(openTag) <= len(content) && content[i:i+len(openTag)] == openTag {
			inTag = true
			i += len(openTag) - 1
			continue
		}

		if inTag && i+len(closeTag) <= len(content) && content[i:i+len(closeTag)] == closeTag {
			inTag = false
			i += len(closeTag) - 1
			continue
		}

		if !inTag {
			result.WriteByte(content[i])
		}
	}

	return result.String()
}

// removeTags 移除所有 HTML 标签
func removeTags(content string) string {
	var result strings.Builder
	inTag := false

	for _, c := range content {
		if c == '<' {
			inTag = true
		} else if c == '>' {
			inTag = false
		} else if !inTag {
			result.WriteRune(c)
		}
	}

	return result.String()
}

// collapseWhitespace 合并多余空白
func collapseWhitespace(content string) string {
	words := strings.Fields(content)
	return strings.Join(words, " ")
}

// extractTitle 从内容中提取标题
func extractTitle(urlStr, content string) string {
	// 尝试从 HTML 中提取 title 标签
	if idx := strings.Index(content, "<title>"); idx != -1 {
		endIdx := strings.Index(content[idx:], "</title>")
		if endIdx != -1 {
			title := content[idx+7 : idx+endIdx]
			title = strings.TrimSpace(title)
			if title != "" {
				return title
			}
		}
	}

	// 使用 URL 的路径部分
	u, err := url.Parse(urlStr)
	if err == nil {
		path := strings.Trim(u.Path, "/")
		if path != "" {
			return path
		}
	}

	// 使用域名
	if u, err := url.Parse(urlStr); err == nil {
		return u.Hostname()
	}

	return "Untitled"
}

// extractLinks 从内容中提取链接
func extractLinks(content string) []string {
	links := make([]string, 0)
	const maxLinks = 20

	idx := 0
	for len(links) < maxLinks {
		// 查找 http:// 或 https://
		httpIdx := strings.Index(content[idx:], "http://")
		httpsIdx := strings.Index(content[idx:], "https://")

		var linkStart int
		if httpIdx == -1 && httpsIdx == -1 {
			break
		} else if httpIdx == -1 {
			linkStart = httpsIdx
		} else if httpsIdx == -1 {
			linkStart = httpIdx
		} else if httpIdx < httpsIdx {
			linkStart = httpIdx
		} else {
			linkStart = httpsIdx
		}

		linkStart += idx

		// 查找链接结束
		linkEnd := linkStart
		for linkEnd < len(content) {
			c := content[linkEnd]
			if c == ' ' || c == '\n' || c == '\t' || c == '"' || c == '\'' || c == '>' || c == ')' {
				break
			}
			linkEnd++
		}

		if linkEnd > linkStart {
			link := content[linkStart:linkEnd]
			links = append(links, link)
		}

		idx = linkEnd
	}

	return links
}

// mergeSearchResults 合并搜索结果
func mergeSearchResults(items []agenttools.SearchItem, strategy string) []agenttools.SearchItem {
	switch strategy {
	case "deduplicate":
		return deduplicateResults(items)
	case "rank":
		return scoreResults(items, "")
	case "all":
		return items
	default:
		return scoreResults(items, "")
	}
}
