// Package search 提供联网搜索能力
package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// WebSearcher 联网搜索接口
type WebSearcher interface {
	Search(ctx context.Context, query string, opts *SearchOptions) (*SearchResult, error)
}

// SearchOptions 搜索选项
type SearchOptions struct {
	MaxResults  int    `json:"max_results"`
	TimeRange   string `json:"time_range,omitempty"` // "day", "week", "month"
	SearchDepth string `json:"search_depth,omitempty"` // "basic", "advanced"
}

// SearchResult 搜索结果
type SearchResult struct {
	Query   string        `json:"query"`
	Answer  string        `json:"answer"`  // AI 总结的答案
	Sources []*SourceItem `json:"sources"`
	Images  []*ImageItem  `json:"images,omitempty"`
}

// SourceItem 搜索来源
type SourceItem struct {
	Title    string `json:"title"`
	URL      string `json:"url"`
	Snippet  string `json:"snippet"`
	Score    float64 `json:"score,omitempty"`
	PublishedDate string `json:"published_date,omitempty"`
}

// ImageItem 图片结果
type ImageItem struct {
	URL     string `json:"url"`
	Caption string `json:"caption,omitempty"`
}

// ========================================
// Tavily 实现
// ========================================

// TavilySearcher Tavily 联网搜索实现
type TavilySearcher struct {
	apiKey  string
	client  *http.Client
	baseURL string
}

// NewTavilySearcher 创建 Tavily 搜索器
func NewTavilySearcher(apiKey string) *TavilySearcher {
	return &TavilySearcher{
		apiKey: apiKey,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: "https://api.tavily.com/search",
	}
}

// tavilyRequest Tavily API 请求
type tavilyRequest struct {
	APIKey      string   `json:"api_key"`
	Query       string   `json:"query"`
	SearchDepth string   `json:"search_depth,omitempty"`
	MaxResults  int      `json:"max_results,omitempty"`
	IncludeAnswer bool   `json:"include_answer"`
	IncludeRawContent bool `json:"include_raw_content,omitempty"`
	IncludeImages     bool `json:"include_images,omitempty"`
}

// tavilyResponse Tavily API 响应
type tavilyResponse struct {
	Answer   string          `json:"answer"`
	Query    string          `json:"query"`
	Results  []tavilyResult  `json:"results"`
	Images   []tavilyImage   `json:"images,omitempty"`
}

// tavilyResult 搜索结果
type tavilyResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
	Score   float64 `json:"score"`
	PublishedDate string `json:"published_date,omitempty"`
}

// tavilyImage 图片结果
type tavilyImage struct {
	URL     string `json:"url"`
	Description string `json:"description,omitempty"`
}

// Search 执行搜索
func (t *TavilySearcher) Search(ctx context.Context, query string, opts *SearchOptions) (*SearchResult, error) {
	if opts == nil {
		opts = &SearchOptions{MaxResults: 5}
	}
	if opts.MaxResults == 0 {
		opts.MaxResults = 5
	}

	reqBody := &tavilyRequest{
		APIKey:      t.apiKey,
		Query:       query,
		SearchDepth: opts.SearchDepth,
		MaxResults:  opts.MaxResults,
		IncludeAnswer: true,
		IncludeImages: true,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", t.baseURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("api error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var tavilyResp tavilyResponse
	if err := json.NewDecoder(resp.Body).Decode(&tavilyResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// 转换结果
	result := &SearchResult{
		Query:  query,
		Answer: tavilyResp.Answer,
		Sources: make([]*SourceItem, 0, len(tavilyResp.Results)),
		Images:  make([]*ImageItem, 0, len(tavilyResp.Images)),
	}

	for _, r := range tavilyResp.Results {
		result.Sources = append(result.Sources, &SourceItem{
			Title:    r.Title,
			URL:      r.URL,
			Snippet:  r.Content,
			Score:    r.Score,
			PublishedDate: r.PublishedDate,
		})
	}

	for _, img := range tavilyResp.Images {
		result.Images = append(result.Images, &ImageItem{
			URL:     img.URL,
			Caption: img.Description,
		})
	}

	return result, nil
}

// ========================================
// Mock 实现（用于测试）
// ========================================

// MockSearcher Mock 搜索器
type MockSearcher struct{}

// NewMockSearcher 创建 Mock 搜索器
func NewMockSearcher() *MockSearcher {
	return &MockSearcher{}
}

// Search 执行模拟搜索
func (m *MockSearcher) Search(ctx context.Context, query string, opts *SearchOptions) (*SearchResult, error) {
	return &SearchResult{
		Query:  query,
		Answer: fmt.Sprintf("关于 '%s' 的模拟答案：这是一个测试结果。", query),
		Sources: []*SourceItem{
			{
				Title:   "测试来源 1",
				URL:     "https://example.com/1",
				Snippet: "这是第一个测试来源的摘要...",
				Score:   0.95,
			},
			{
				Title:   "测试来源 2",
				URL:     "https://example.com/2",
				Snippet: "这是第二个测试来源的摘要...",
				Score:   0.87,
			},
		},
	}, nil
}
