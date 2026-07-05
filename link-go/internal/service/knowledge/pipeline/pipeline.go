// Package rag 提供 RAG 基础设施层实现 - 管道
package rag

import (
	"context"
	"fmt"
	"time"

	domainrag "link/internal/model/rag"
)

// ========================================
// Pipeline 实现
// ========================================

// PipelineImpl RAG 管道实现
type PipelineImpl struct {
	retriever         domainrag.Retriever
	reranker          domainrag.Reranker
	llmChat           domainrag.LLMChat
	queryStrengthener domainrag.QueryStrengthener
}

// NewPipeline 创建 RAG 管道
func NewPipeline(
	retriever domainrag.Retriever,
	reranker domainrag.Reranker,
	llmChat domainrag.LLMChat,
	queryStrengthener domainrag.QueryStrengthener,
) domainrag.Pipeline {
	return &PipelineImpl{
		retriever:         retriever,
		reranker:          reranker,
		llmChat:           llmChat,
		queryStrengthener: queryStrengthener,
	}
}

// Execute 执行 RAG 管道
func (p *PipelineImpl) Execute(ctx context.Context, req *domainrag.PipelineRequest) (*domainrag.PipelineResponse, error) {
	startTime := time.Now()

	// 1. 查询增强
	var finalQuery string
	var ragContext *domainrag.RAGContext

	if req.Options.EnableQueryRewrite || req.Options.EnableQuerySplit {
		strengthOpts := domainrag.DefaultStrengthOptions()
		strengthOpts.EnableRewrite = req.Options.EnableQueryRewrite
		strengthOpts.EnableSplit = req.Options.EnableQuerySplit

		convHistory := buildHistoryString(req.Context)
		enhanced, err := p.queryStrengthener.StrengthenQuery(ctx, req.Query, convHistory, strengthOpts)
		if err == nil {
			queries := enhanced.GetQueriesForRetrieve()
			if len(queries) > 0 {
				finalQuery = queries[0]
			}
			ragContext = &domainrag.RAGContext{
				Query:          req.Query,
				RewrittenQuery: enhanced.RewrittenQuery,
				SubQueries:     enhanced.SubQueries,
				Timestamp:      time.Now(),
			}
		}
	}

	if finalQuery == "" {
		finalQuery = req.Query
	}
	if ragContext == nil {
		ragContext = &domainrag.RAGContext{
			Query:     req.Query,
			Timestamp: time.Now(),
		}
	}

	// 2. 检索
	retrieveOpts := &domainrag.RetrieveOptions{
		TopK:                req.Options.TopK,
		SimilarityThreshold: req.Options.SimilarityThreshold,
		RerankEnabled:       req.Options.EnableRerank,
		GraphEnabled:        req.Options.GraphEnabled,
		Alpha:               req.Options.Alpha,
		RetrievalMode:       req.Options.RetrievalMode,
	}

	var retrieveResp *domainrag.RetrieveResponse
	var err error

	switch req.Options.RetrievalMode {
	case "vector":
		retrieveResp, err = p.retriever.VectorRetrieve(ctx, req.TenantID, req.KnowledgeBaseID, finalQuery, retrieveOpts)
	case "bm25":
		retrieveResp, err = p.retriever.BM25Retrieve(ctx, req.TenantID, req.KnowledgeBaseID, finalQuery, retrieveOpts)
	case "graph":
		retrieveResp, err = p.retriever.GraphRetrieve(ctx, req.TenantID, req.KnowledgeBaseID, finalQuery, retrieveOpts)
	default:
		retrieveResp, err = p.retriever.HybridRetrieve(ctx, req.TenantID, req.KnowledgeBaseID, finalQuery, retrieveOpts)
	}

	if err != nil {
		return nil, fmt.Errorf("检索失败: %w", err)
	}

	ragContext.Documents = retrieveResp.Results
	ragContext.RetrievalMetadata = retrieveResp.SearchTrace

	// 3. 构建消息
	messages := p.buildMessages(req.Query, retrieveResp.Results, ragContext)

	// 4. 生成答案
	llmOpts := &domainrag.ChatOptions{
		Temperature: 0.7,
		MaxTokens:   2000,
	}

	llmResp, err := p.llmChat.Chat(ctx, messages, llmOpts)
	if err != nil {
		return nil, fmt.Errorf("生成答案失败: %w", err)
	}

	// 5. 构建响应
	return &domainrag.PipelineResponse{
		Answer:         llmResp.Content,
		Documents:      retrieveResp.Results,
		Context:        ragContext,
		Metadata:       map[string]interface{}{
			"message_id": llmResp.MessageID,
		},
		ProcessingTime: time.Since(startTime).Milliseconds(),
	}, nil
}

// ExecuteStream 流式执行 RAG 管道
func (p *PipelineImpl) ExecuteStream(ctx context.Context, req *domainrag.PipelineRequest) (<-chan *domainrag.PipelineEvent, error) {
	eventChan := make(chan *domainrag.PipelineEvent, 10)

	go func() {
		defer close(eventChan)

		startTime := time.Now()

		// 1. 查询增强
		if !sendEvent(ctx, eventChan, &domainrag.PipelineEvent{
			Event: "query_strengthening",
			Metadata: map[string]interface{}{
				"query": req.Query,
			},
		}) {
			return
		}

		var finalQuery string
		var ragContext *domainrag.RAGContext

		if req.Options.EnableQueryRewrite || req.Options.EnableQuerySplit {
			strengthOpts := domainrag.DefaultStrengthOptions()
			strengthOpts.EnableRewrite = req.Options.EnableQueryRewrite
			strengthOpts.EnableSplit = req.Options.EnableQuerySplit

			convHistory := buildHistoryString(req.Context)
			enhanced, err := p.queryStrengthener.StrengthenQuery(ctx, req.Query, convHistory, strengthOpts)
			if err == nil {
				queries := enhanced.GetQueriesForRetrieve()
				if len(queries) > 0 {
					finalQuery = queries[0]
				}
				ragContext = &domainrag.RAGContext{
					Query:          req.Query,
					RewrittenQuery: enhanced.RewrittenQuery,
					SubQueries:     enhanced.SubQueries,
					Timestamp:      time.Now(),
				}
			}
		}

		if finalQuery == "" {
			finalQuery = req.Query
		}
		if ragContext == nil {
			ragContext = &domainrag.RAGContext{
				Query:     req.Query,
				Timestamp: time.Now(),
			}
		}

		// 2. 检索
		if !sendEvent(ctx, eventChan, &domainrag.PipelineEvent{
			Event: "retrieve",
			Metadata: map[string]interface{}{
				"query": finalQuery,
				"mode":  req.Options.RetrievalMode,
			},
		}) {
			return
		}

		retrieveOpts := &domainrag.RetrieveOptions{
			TopK:                req.Options.TopK,
			SimilarityThreshold: req.Options.SimilarityThreshold,
			RerankEnabled:       req.Options.EnableRerank,
			GraphEnabled:        req.Options.GraphEnabled,
			Alpha:               req.Options.Alpha,
			RetrievalMode:       req.Options.RetrievalMode,
		}

		var retrieveResp *domainrag.RetrieveResponse
		var err error

		switch req.Options.RetrievalMode {
		case "vector":
			retrieveResp, err = p.retriever.VectorRetrieve(ctx, req.TenantID, req.KnowledgeBaseID, finalQuery, retrieveOpts)
		case "bm25":
			retrieveResp, err = p.retriever.BM25Retrieve(ctx, req.TenantID, req.KnowledgeBaseID, finalQuery, retrieveOpts)
		case "graph":
			retrieveResp, err = p.retriever.GraphRetrieve(ctx, req.TenantID, req.KnowledgeBaseID, finalQuery, retrieveOpts)
		default:
			retrieveResp, err = p.retriever.HybridRetrieve(ctx, req.TenantID, req.KnowledgeBaseID, finalQuery, retrieveOpts)
		}

		if err != nil {
			sendEvent(ctx, eventChan, &domainrag.PipelineEvent{
				Event: "error",
				Error: err,
			})
			return
		}

		ragContext.Documents = retrieveResp.Results
		ragContext.RetrievalMetadata = retrieveResp.SearchTrace

		// 3. 发送检索完成事件
		if !sendEvent(ctx, eventChan, &domainrag.PipelineEvent{
			Event: "retrieve_complete",
			Metadata: map[string]interface{}{
				"count": len(retrieveResp.Results),
			},
		}) {
			return
		}

		// 4. 发送文档事件
		for _, doc := range retrieveResp.Results {
			if !sendEvent(ctx, eventChan, &domainrag.PipelineEvent{
				Event:    "document",
				Document: doc,
			}) {
				return
			}
		}

		// 5. 流式生成
		if !sendEvent(ctx, eventChan, &domainrag.PipelineEvent{
			Event: "generate",
			Metadata: map[string]interface{}{
				"doc_count": len(retrieveResp.Results),
			},
		}) {
			return
		}

		messages := p.buildMessages(req.Query, retrieveResp.Results, ragContext)
		llmOpts := &domainrag.ChatOptions{
			Temperature: 0.7,
			MaxTokens:   2000,
		}

		streamChan, err := p.llmChat.ChatStream(ctx, messages, llmOpts)
		if err != nil {
			sendEvent(ctx, eventChan, &domainrag.PipelineEvent{
				Event: "error",
				Error: err,
			})
			return
		}

		// 转发流式事件
		for streamEvent := range streamChan {
			if streamEvent.Error != nil {
				sendEvent(ctx, eventChan, &domainrag.PipelineEvent{
					Event: "error",
					Error: streamEvent.Error,
				})
				return
			}

			if !sendEvent(ctx, eventChan, &domainrag.PipelineEvent{
				Event:   "chunk",
				Content: streamEvent.Content,
			}) {
				return
			}

			if streamEvent.Done {
				break
			}
		}

		// 6. 完成
		sendEvent(ctx, eventChan, &domainrag.PipelineEvent{
			Event: "done",
			Metadata: map[string]interface{}{
				"processing_time_ms": time.Since(startTime).Milliseconds(),
			},
		})
	}()

	return eventChan, nil
}

// ========================================
// 辅助方法
// ========================================

func (p *PipelineImpl) buildMessages(query string, docs []*domainrag.Document, context *domainrag.RAGContext) []domainrag.LLMMessage {
	messages := make([]domainrag.LLMMessage, 0)

	// 构建系统提示
	systemPrompt := `你是一个专业的助手，请基于以下上下文信息回答用户问题。
如果上下文中没有相关信息，请明确告知用户。`

	messages = append(messages, domainrag.LLMMessage{
		Role:    "system",
		Content: systemPrompt,
	})

	// 构建上下文内容
	contextStr := ""
	for i, doc := range docs {
		if i >= 5 {
			break
		}
		contextStr += fmt.Sprintf("[%d] %s\n", i+1, doc.Content)
	}

	// 用户消息
	userPrompt := fmt.Sprintf("上下文信息：\n%s\n\n问题：%s", contextStr, query)

	messages = append(messages, domainrag.LLMMessage{
		Role:    "user",
		Content: userPrompt,
	})

	return messages
}

func buildHistoryString(ragContext *domainrag.RAGContext) string {
	if ragContext == nil || len(ragContext.ConversationHistory) == 0 {
		return ""
	}

	history := ""
	for _, msg := range ragContext.ConversationHistory {
		history += fmt.Sprintf("%s: %s\n", msg.Role, msg.Content)
	}
	return history
}

// sendEvent 阻塞发送事件：慢消费者产生背压而非静默丢块；
// ctx 取消时返回 false，调用方应停止生产。
func sendEvent(ctx context.Context, ch chan<- *domainrag.PipelineEvent, event *domainrag.PipelineEvent) bool {
	select {
	case ch <- event:
		return true
	case <-ctx.Done():
		return false
	}
}
