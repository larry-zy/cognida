// Package executor 提供评测执行器实现
package executor

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	domeval "link/internal/model/evaluation"
	"link/internal/model/rag"
	prompts "link/internal/prompt"
)

// RAGExecutor RAG 评测执行器
type RAGExecutor struct {
	retriever  rag.Retriever
	llmChat    rag.LLMChat
	retrieveTimeout time.Duration
	generateTimeout time.Duration
}

// NewRAGExecutor 创建 RAG 执行器
func NewRAGExecutor(retriever rag.Retriever, llmChat rag.LLMChat) *RAGExecutor {
	return &RAGExecutor{
		retriever:       retriever,
		llmChat:         llmChat,
		retrieveTimeout: 10 * time.Second,
		generateTimeout: 30 * time.Second,
	}
}

// Type 返回执行器类型
func (e *RAGExecutor) Type() domeval.EvaluationType {
	return domeval.EvaluationTypeRAG
}

// Execute 执行 RAG 评测
func (e *RAGExecutor) Execute(ctx context.Context, task *domeval.EvaluationTaskConfig, dataset []*domeval.QAPair) ([]*domeval.QAResult, error) {
	// 验证配置
	if task.KnowledgeBaseID == "" {
		return nil, fmt.Errorf("kb_id is required for RAG evaluation")
	}

	// 获取 top_k 配置
	topK := e.getTopK(task.Config)

	// 创建结果切片
	results := make([]*domeval.QAResult, len(dataset))

	// 执行评测
	for i, qa := range dataset {
		results[i] = &domeval.QAResult{
			Question:        qa.Question,
			ReferenceAnswer: qa.ReferenceAnswer,
			RelevantPIDs:    qa.RelevantPIDs,
		}

		// 1. 检索阶段
		retrieveCtx, retrieveCancel := context.WithTimeout(ctx, e.retrieveTimeout)
		retrievedDocs, err := e.retrieve(retrieveCtx, task, qa.Question, topK)
		retrieveCancel()

		if err != nil {
			results[i].Success = false
			results[i].Error = fmt.Sprintf("retrieval failed: %v", err)
			continue
		}

		// 保存检索到的文档：正文用于生成，ChunkID 用于检索指标
		retrievedPIDs := make([]string, len(retrievedDocs))
		retrievedChunks := make([]string, len(retrievedDocs))
		for j, doc := range retrievedDocs {
			retrievedPIDs[j] = doc.ChunkID
			retrievedChunks[j] = doc.Content
		}
		results[i].RetrievedChunks = retrievedChunks
		results[i].RetrievedPIDs = retrievedPIDs

		// 2. 生成阶段
		generateCtx, generateCancel := context.WithTimeout(ctx, e.generateTimeout)
		answer, err := e.generate(generateCtx, qa.Question, retrievedChunks)
		generateCancel()

		if err != nil {
			results[i].Success = false
			results[i].Error = fmt.Sprintf("generation failed: %v", err)
			continue
		}

		results[i].GeneratedAnswer = answer
		results[i].Success = true
	}

	return results, nil
}

// retrieve 执行检索
func (e *RAGExecutor) retrieve(ctx context.Context, task *domeval.EvaluationTaskConfig, query string, topK int) ([]*rag.Document, error) {
	opts := &rag.RetrieveOptions{
		TopK: topK,
	}

	// 使用混合检索
	resp, err := e.retriever.HybridRetrieve(ctx, "", task.KnowledgeBaseID, query, opts)
	if err != nil {
		return nil, fmt.Errorf("hybrid retrieve failed: %w", err)
	}

	return resp.Results, nil
}

// generate 执行生成
func (e *RAGExecutor) generate(ctx context.Context, question string, contexts []string) (string, error) {
	log.Printf("[RAGExecutor] Calling LLM for question: %s, context count: %d", question, len(contexts))

	// 构建提示词
	systemPrompt := e.buildSystemPrompt(contexts)
	userMessage := question

	messages := []rag.LLMMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}

	opts := &rag.ChatOptions{
		Temperature: 0.1,
		MaxTokens:   2000,
	}

	resp, err := e.llmChat.Chat(ctx, messages, opts)
	if err != nil {
		log.Printf("[RAGExecutor] LLM chat failed: %v", err)
		return "", fmt.Errorf("LLM chat failed: %w", err)
	}

	log.Printf("[RAGExecutor] LLM response: %s", resp.Content)
	return resp.Content, nil
}

// buildSystemPrompt 构建系统提示词
func (e *RAGExecutor) buildSystemPrompt(contexts []string) string {
	if len(contexts) == 0 {
		return prompts.MustGet("evaluation", "rag_system_no_context")
	}

	contextText := strings.Join(contexts, "\n\n")

	return fmt.Sprintf(prompts.MustGet("evaluation", "rag_system_with_context"), contextText)
}

// getTopK 获取 top_k 配置
func (e *RAGExecutor) getTopK(config map[string]interface{}) int {
	if config == nil {
		return 5 // 默认 top_k
	}

	if topK, ok := config["top_k"].(float64); ok {
		return int(topK)
	}

	return 5
}

// SetRetrieveTimeout 设置检索超时
func (e *RAGExecutor) SetRetrieveTimeout(timeout time.Duration) {
	e.retrieveTimeout = timeout
}

// SetGenerateTimeout 设置生成超时
func (e *RAGExecutor) SetGenerateTimeout(timeout time.Duration) {
	e.generateTimeout = timeout
}
