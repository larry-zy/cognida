// Package rag provides RAG service implementations
// @Deprecated: Chat functionality moved to link/internal/service/chat/chat_service.go
// Use the unified ChatService with Agent-based architecture instead.
package knowledge

import (
	"context"
	"fmt"
	"time"

	domainrag "link/internal/model/rag"
)

// ========================================
// Pipeline Service Implementation (DEPRECATED)
// ========================================

// Pipeline implements RAG chat pipeline
// @Deprecated: Use ChatService from llm package instead
type Pipeline struct {
	retriever         domainrag.Retriever
	reranker          domainrag.Reranker
	queryStrengthener domainrag.QueryStrengthener
	llmChat           domainrag.LLMChat
}

// NewPipeline creates a new RAG pipeline service
func NewPipeline(
	retriever domainrag.Retriever,
	reranker domainrag.Reranker,
	queryStrengthener domainrag.QueryStrengthener,
	llmChat domainrag.LLMChat,
) *Pipeline {
	return &Pipeline{
		retriever:         retriever,
		reranker:          reranker,
		queryStrengthener: queryStrengthener,
		llmChat:           llmChat,
	}
}

// Chat executes a RAG chat request
func (p *Pipeline) Chat(ctx context.Context, tenantID int64, req *ChatRequest) (*ChatResponse, error) {
	startTime := time.Now()

	// 1. Build retrieve options
	opts := p.buildRetrieveOptions(req.Options)

	// 2. Query enhancement (if enabled)
	var enhancedQuery *domainrag.StrengthenedQuery
	var finalQuery string
	if req.Options != nil && (req.Options.EnableQueryRewrite || req.Options.EnableQuerySplit) {
		strengthOpts := domainrag.DefaultStrengthOptions()
		strengthOpts.EnableRewrite = req.Options.EnableQueryRewrite
		strengthOpts.EnableSplit = req.Options.EnableQuerySplit

		convHistory := buildConversationHistory(req.ConversationHistory)
		var err error
		enhancedQuery, err = p.queryStrengthener.StrengthenQuery(ctx, req.Query, convHistory, strengthOpts)
		if err != nil {
			// Enhancement failed, use original query
			finalQuery = req.Query
		} else {
			// Use enhanced query
			queries := enhancedQuery.GetQueriesForRetrieve()
			if len(queries) > 0 {
				finalQuery = queries[0]
			} else {
				finalQuery = req.Query
			}
		}
	} else {
		finalQuery = req.Query
	}

	// 3. Execute retrieval
	kbID := req.KnowledgeBaseID
	retrieveOpts := &domainrag.RetrieveOptions{
		TopK:                opts.TopK,
		SimilarityThreshold: opts.SimilarityThreshold,
		RerankEnabled:       opts.EnableRerank,
		GraphEnabled:        opts.GraphEnabled,
		RetrievalMode:       opts.RetrievalMode,
	}

	var retrieveResp *domainrag.RetrieveResponse
	var err error

	switch opts.RetrievalMode {
	case "vector":
		retrieveResp, err = p.retriever.VectorRetrieve(ctx, formatTenantID(tenantID), kbID, finalQuery, retrieveOpts)
	case "bm25", "keyword":
		retrieveResp, err = p.retriever.BM25Retrieve(ctx, formatTenantID(tenantID), kbID, finalQuery, retrieveOpts)
	case "graph":
		retrieveResp, err = p.retriever.GraphRetrieve(ctx, formatTenantID(tenantID), kbID, finalQuery, retrieveOpts)
	default: // hybrid
		retrieveResp, err = p.retriever.HybridRetrieve(ctx, formatTenantID(tenantID), kbID, finalQuery, retrieveOpts)
	}

	if err != nil {
		return nil, fmt.Errorf("检索失败: %w", err)
	}

	// 4. Generate answer
	llmMessages := p.buildRAGMessages(req.Query, retrieveResp.Results, req.ConversationHistory)

	llmOpts := &domainrag.ChatOptions{}
	if req.Options != nil {
		llmOpts.Temperature = req.Options.Temperature
		llmOpts.MaxTokens = req.Options.MaxTokens
	}

	llmResp, err := p.llmChat.Chat(ctx, llmMessages, llmOpts)
	if err != nil {
		return nil, fmt.Errorf("生成答案失败: %w", err)
	}

	// 5. Build response
	metadata := &ChatMetadata{
		ProcessingTime:  time.Since(startTime).Milliseconds(),
		RetrievalCount:  len(retrieveResp.Results),
	}

	if retrieveResp.SearchTrace != nil {
		metadata.VectorCount = retrieveResp.SearchTrace.VectorResultCount
		metadata.BM25Count = retrieveResp.SearchTrace.BM25ResultCount
		metadata.GraphCount = retrieveResp.SearchTrace.GraphResultCount
		metadata.RetrievalTrace = convertRetrievalTrace(retrieveResp.SearchTrace)
	}

	if enhancedQuery != nil {
		metadata.QueryRewritten = enhancedQuery.RewriteApplied || enhancedQuery.SplitApplied
		metadata.OriginalQuery = enhancedQuery.OriginalQuery
		metadata.RewrittenQuery = enhancedQuery.RewrittenQuery
		metadata.SubQueries = enhancedQuery.SubQueries
	}

	return &ChatResponse{
		Answer:     llmResp.Content,
		Documents:  convertDocuments(retrieveResp.Results),
		SessionID:  req.SessionID,
		MessageID:  llmResp.MessageID,
		Metadata:   metadata,
	}, nil
}

// ChatStream executes a RAG chat request with streaming
func (p *Pipeline) ChatStream(ctx context.Context, tenantID int64, req *ChatRequest) (<-chan *StreamEvent, error) {
	eventChan := make(chan *StreamEvent, 10)

	go func() {
		defer close(eventChan)

		startTime := time.Now()

		// 1. Build retrieve options
		opts := p.buildRetrieveOptions(req.Options)

		// 2. Query enhancement
		var enhancedQuery *domainrag.StrengthenedQuery
		var finalQuery string
		if req.Options != nil && (req.Options.EnableQueryRewrite || req.Options.EnableQuerySplit) {
			strengthOpts := domainrag.DefaultStrengthOptions()
			strengthOpts.EnableRewrite = req.Options.EnableQueryRewrite
			strengthOpts.EnableSplit = req.Options.EnableQuerySplit

			convHistory := buildConversationHistory(req.ConversationHistory)
			var err error
			enhancedQuery, err = p.queryStrengthener.StrengthenQuery(ctx, req.Query, convHistory, strengthOpts)
			if err == nil {
				queries := enhancedQuery.GetQueriesForRetrieve()
				if len(queries) > 0 {
					finalQuery = queries[0]
				}
			}
		}

		if finalQuery == "" {
			finalQuery = req.Query
		}

		// 3. Send retrieve start event
		if !sendEvent(ctx, eventChan, &StreamEvent{
			Event:   "retrieve_start",
			Content: finalQuery,
			Metadata: map[string]interface{}{
				"retrieval_mode": opts.RetrievalMode,
				"top_k":          opts.TopK,
			},
		}) {
			return
		}

		// 4. Execute retrieval
		kbID := req.KnowledgeBaseID
		retrieveOpts := &domainrag.RetrieveOptions{
			TopK:                opts.TopK,
			SimilarityThreshold: opts.SimilarityThreshold,
			RerankEnabled:       opts.EnableRerank,
			GraphEnabled:        opts.GraphEnabled,
			RetrievalMode:       opts.RetrievalMode,
		}

		var retrieveResp *domainrag.RetrieveResponse
		var err error

		switch opts.RetrievalMode {
		case "vector":
			retrieveResp, err = p.retriever.VectorRetrieve(ctx, formatTenantID(tenantID), kbID, finalQuery, retrieveOpts)
		case "bm25", "keyword":
			retrieveResp, err = p.retriever.BM25Retrieve(ctx, formatTenantID(tenantID), kbID, finalQuery, retrieveOpts)
		case "graph":
			retrieveResp, err = p.retriever.GraphRetrieve(ctx, formatTenantID(tenantID), kbID, finalQuery, retrieveOpts)
		default:
			retrieveResp, err = p.retriever.HybridRetrieve(ctx, formatTenantID(tenantID), kbID, finalQuery, retrieveOpts)
		}

		if err != nil {
			sendEvent(ctx, eventChan, &StreamEvent{
				Event: "error",
				Error: err.Error(),
			})
			return
		}

		// 5. Send retrieve complete event
		if !sendEvent(ctx, eventChan, &StreamEvent{
			Event: "retrieve_complete",
			Metadata: map[string]interface{}{
				"count": len(retrieveResp.Results),
			},
		}) {
			return
		}

		// 6. Send document events
		for _, doc := range retrieveResp.Results {
			if !sendEvent(ctx, eventChan, &StreamEvent{
				Event:    "document",
				Document: convertDocumentDTO(doc),
			}) {
				return
			}
		}

		// 7. Stream generate answer
		llmMessages := p.buildRAGMessages(req.Query, retrieveResp.Results, req.ConversationHistory)
		llmOpts := &domainrag.ChatOptions{}
		if req.Options != nil {
			llmOpts.Temperature = req.Options.Temperature
			llmOpts.MaxTokens = req.Options.MaxTokens
		}

		streamChan, err := p.llmChat.ChatStream(ctx, llmMessages, llmOpts)
		if err != nil {
			sendEvent(ctx, eventChan, &StreamEvent{
				Event: "error",
				Error: err.Error(),
			})
			return
		}

		// 8. Forward stream events
		for streamEvent := range streamChan {
			if streamEvent.Error != nil {
				sendEvent(ctx, eventChan, &StreamEvent{
					Event: "error",
					Error: streamEvent.Error.Error(),
				})
				return
			}

			if !sendEvent(ctx, eventChan, &StreamEvent{
				Event:   "chunk",
				Content: streamEvent.Content,
				Done:    streamEvent.Done,
			}) {
				return
			}

			if streamEvent.Done {
				break
			}
		}

		// 9. Send complete event
		sendEvent(ctx, eventChan, &StreamEvent{
			Event: "done",
			Done:  true,
			Metadata: map[string]interface{}{
				"processing_time_ms": time.Since(startTime).Milliseconds(),
			},
		})
	}()

	return eventChan, nil
}

// buildRetrieveOptions builds retrieve options with defaults
func (p *Pipeline) buildRetrieveOptions(opts *ChatOptions) *ChatOptions {
	if opts == nil {
		return &ChatOptions{
			TopK:                10,
			SimilarityThreshold: 0.5,
			RetrievalMode:       "hybrid",
			EnableRerank:        true,
			GraphEnabled:        false,
		}
	}

	// Set defaults
	result := *opts
	if result.TopK <= 0 {
		result.TopK = 10
	}
	if result.SimilarityThreshold <= 0 {
		result.SimilarityThreshold = 0.5
	}
	if result.RetrievalMode == "" {
		result.RetrievalMode = "hybrid"
	}

	return &result
}

// buildRAGMessages builds LLM messages for RAG
func (p *Pipeline) buildRAGMessages(query string, docs []*domainrag.Document, history []ConversationMessage) []domainrag.LLMMessage {
	messages := make([]domainrag.LLMMessage, 0)

	// Add history messages
	for _, msg := range history {
		messages = append(messages, domainrag.LLMMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Build context content
	context := ""
	for i, doc := range docs {
		if i >= 5 { // Use at most 5 documents
			break
		}
		context += fmt.Sprintf("[%d] %s\n", i+1, doc.Content)
	}

	// Build system prompt
	systemPrompt := `你是一个专业的助手，请基于以下上下文信息回答用户问题。
如果上下文中没有相关信息，请明确告知用户。

上下文信息：
` + context

	// If no history messages, add system prompt
	if len(history) == 0 {
		messages = append(messages, domainrag.LLMMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	// Add user query
	messages = append(messages, domainrag.LLMMessage{
		Role:    "user",
		Content: query,
	})

	return messages
}

// sendEvent 阻塞发送事件：慢消费者产生背压而非静默丢块；
// ctx 取消时返回 false，调用方应停止生产。
func sendEvent(ctx context.Context, ch chan<- *StreamEvent, event *StreamEvent) bool {
	select {
	case ch <- event:
		return true
	case <-ctx.Done():
		return false
	}
}
