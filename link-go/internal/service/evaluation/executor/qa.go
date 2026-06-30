// Package executor 提供评测执行器实现
package executor

import (
	"context"
	"fmt"
	"log"
	"time"

	domeval "link/internal/model/evaluation"
	"link/internal/model/rag"
)

// QAExecutor QA 评测执行器（直接 LLM 生成）
type QAExecutor struct {
	llmChat rag.LLMChat
	timeout time.Duration
}

// NewQAExecutor 创建 QA 执行器
func NewQAExecutor(llmChat rag.LLMChat) *QAExecutor {
	return &QAExecutor{
		llmChat: llmChat,
		timeout: 30 * time.Second,
	}
}

// Type 返回执行器类型
func (e *QAExecutor) Type() domeval.EvaluationType {
	return domeval.EvaluationTypeQA
}

// Execute 执行 QA 评测
func (e *QAExecutor) Execute(ctx context.Context, task *domeval.EvaluationTaskConfig, dataset []*domeval.QAPair) ([]*domeval.QAResult, error) {
	// 创建结果切片
	results := make([]*domeval.QAResult, len(dataset))

	// 为每个 QA 创建超时上下文
	for i, qa := range dataset {
		results[i] = &domeval.QAResult{
			Question:        qa.Question,
			ReferenceAnswer: qa.ReferenceAnswer,
		}

		// 为每个 QA 设置单独的超时
		qaCtx, cancel := context.WithTimeout(ctx, e.timeout)
		answer, err := e.generate(qaCtx, qa.Question)
		cancel()

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

// generate 执行生成
func (e *QAExecutor) generate(ctx context.Context, question string) (string, error) {
	log.Printf("[QAExecutor] Calling LLM for question: %s", question)

	messages := []rag.LLMMessage{
		{Role: "system", Content: "You are a helpful assistant. Answer the user's question accurately and concisely."},
		{Role: "user", Content: question},
	}

	opts := &rag.ChatOptions{
		Temperature: 0.1,
		MaxTokens:   2000,
	}

	resp, err := e.llmChat.Chat(ctx, messages, opts)
	if err != nil {
		log.Printf("[QAExecutor] LLM chat failed: %v", err)
		return "", fmt.Errorf("LLM chat failed: %w", err)
	}

	log.Printf("[QAExecutor] LLM response: %s", resp.Content)
	return resp.Content, nil
}

// SetTimeout 设置超时时间
func (e *QAExecutor) SetTimeout(timeout time.Duration) {
	e.timeout = timeout
}

// GetTimeout 获取超时时间
func (e *QAExecutor) GetTimeout() time.Duration {
	return e.timeout
}
