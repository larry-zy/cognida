// Package reflection provides the Reflection Hook implementation.
package reflection

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"link/internal/model/agent/reflection"
)

// ========================================
// ReflectionHook 反思 Hook
// ========================================

// ChatModel 聊天模型接口（简化版）
type ChatModel interface {
	Chat(ctx context.Context, messages []Message, opts interface{}) (interface{}, error)
}

// Message 聊天消息
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatResponse 聊天响应
type ChatResponse struct {
	Content string
}

// ReflectionHook 反思 Hook 实现
type ReflectionHook struct {
	actor    ChatModel            // Actor LLM
	critic   reflection.Critic     // Critic 评估器
	memory   reflection.ReflectionMemory
	config   *reflection.ReflectionConfig
	agentID  string
}

// NewReflectionHook 创建反思 Hook
func NewReflectionHook(
	actor ChatModel,
	critic reflection.Critic,
	memory reflection.ReflectionMemory,
	config *reflection.ReflectionConfig,
	agentID string,
) *ReflectionHook {
	return &ReflectionHook{
		actor:   actor,
		critic:  critic,
		memory:  memory,
		config:  config,
		agentID: agentID,
	}
}

// IsEnabled 检查是否启用
func (h *ReflectionHook) IsEnabled() bool {
	return h.config != nil && h.config.Enabled
}

// Refine 执行反思并改进输出
func (h *ReflectionHook) Refine(ctx context.Context, task string, initialContent string) *RefinementResult {
	startTime := time.Now()

	// 记录调用
	RecordInvocation(h.agentID, h.config.CriticType)

	result := &RefinementResult{
		OriginalContent: initialContent,
		Iterations:      0,
	}

	currentContent := initialContent
	var initialCritique *reflection.CritiqueResult

	// 迭代改进
	for i := 0; i < h.config.MaxIterations; i++ {
		result.Iterations = i + 1

		// 1. 获取历史经验（第一次迭代时）
		var lessons []*reflection.ReflectionRecord
		if i == 0 && h.memory != nil {
			var err error
			lessons, err = h.memory.Retrieve(ctx, h.agentID, task, 3)
			if err == nil && len(lessons) > 0 {
				result.UsedMemory = true
				RecordMemoryHit()
			} else {
				RecordMemoryMiss()
			}
		}

		// 2. Critic 评估
		critique, err := h.critic.Evaluate(ctx, task, currentContent)
		if err != nil {
			// Critic 失败，降级返回原始结果
			RecordError(h.agentID, "critic_evaluation")
			result.Error = fmt.Errorf("critic evaluation failed: %w", err)
			result.FinalContent = currentContent
			result.Duration = time.Since(startTime)
			RecordDuration(h.agentID, false, result.Duration.Seconds())
			return result
		}

		// 保存初始评估结果
		if i == 0 {
			initialCritique = critique
			result.InitialScore = critique.OverallScore
			// 记录初始质量分数
			for dim, score := range critique.Dimensions {
				RecordQualityBefore(h.agentID, dim, score.Score)
			}
		}

		// 3. 判断是否需要改进
		if !h.critic.ShouldRefine(critique) {
			// 评估通过，记录成功经验
			result.FinalContent = currentContent
			result.FinalScore = critique.OverallScore
			result.Success = true

			// 记录最终质量分数
			for dim, score := range critique.Dimensions {
				RecordQualityAfter(h.agentID, dim, score.Score)
			}

			// 异步存储成功经验
			if h.memory != nil {
				go h.storeSuccess(context.Background(), task, currentContent, critique, i+1)
			}
			break
		}

		// 4. 需要改进，构建 refine prompt
		refinePrompt := h.buildRefinePrompt(task, currentContent, critique, lessons)

		// 5. Actor 重新生成
		resp, err := h.actor.Chat(ctx, []Message{
			{Role: "user", Content: refinePrompt},
		}, nil)
		if err != nil {
			// Actor 失败，返回当前结果
			RecordError(h.agentID, "actor_refinement")
			result.FinalContent = currentContent
			result.Error = fmt.Errorf("actor refinement failed: %w", err)
			result.Duration = time.Since(startTime)
			RecordDuration(h.agentID, false, result.Duration.Seconds())
			return result
		}

		// 提取响应内容
		if r, ok := resp.(ChatResponse); ok {
			currentContent = r.Content
		}
	}

	// 如果达到最大迭代次数仍未通过，记录失败经验
	if !result.Success && h.memory != nil {
		go h.storeFailure(context.Background(), task, result.FinalContent, initialCritique, result.Iterations)
	}

	result.Duration = time.Since(startTime)

	// 记录迭代次数和耗时
	RecordIterations(h.agentID, float64(result.Iterations))
	RecordDuration(h.agentID, result.Success, result.Duration.Seconds())

	return result
}

// buildRefinePrompt 构建改进提示词
func (h *ReflectionHook) buildRefinePrompt(
	task string,
	currentOutput string,
	critique *reflection.CritiqueResult,
	lessons []*reflection.ReflectionRecord,
) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("任务：%s\n\n", task))

	// 添加历史经验
	if len(lessons) > 0 {
		builder.WriteString("历史经验（请避免重复错误）：\n")
		for _, lesson := range lessons {
			builder.WriteString(fmt.Sprintf("- 避免：%s\n", lesson.Lesson))
		}
		builder.WriteString("\n")
	}

	builder.WriteString(fmt.Sprintf("你的上次回答：\n%s\n\n", currentOutput))

	builder.WriteString("评估反馈：\n")
	builder.WriteString(fmt.Sprintf("- 总体评分：%.2f\n", critique.OverallScore))

	if len(critique.Issues) > 0 {
		builder.WriteString(fmt.Sprintf("- 发现的问题：%s\n", strings.Join(critique.Issues, "；")))
	}

	if len(critique.Suggestions) > 0 {
		builder.WriteString(fmt.Sprintf("- 改进建议：%s\n", strings.Join(critique.Suggestions, "；")))
	}

	builder.WriteString("\n请根据以上反馈，重新回答任务。请特别注意改进建议中提到的问题。")

	return builder.String()
}

// storeSuccess 异步存储成功经验
func (h *ReflectionHook) storeSuccess(ctx context.Context, task, output string, critique *reflection.CritiqueResult, iterations int) {
	record := &reflection.ReflectionRecord{
		AgentID:    h.agentID,
		Task:       task,
		Attempt:    output,
		Critique:   critique,
		Lesson:     h.extractLesson(critique),
		Success:    true,
		Iterations: iterations,
		Metadata:   make(map[string]interface{}),
	}

	_ = h.memory.Store(ctx, record)
}

// storeFailure 异步存储失败经验
func (h *ReflectionHook) storeFailure(ctx context.Context, task, output string, critique *reflection.CritiqueResult, iterations int) {
	record := &reflection.ReflectionRecord{
		AgentID:    h.agentID,
		Task:       task,
		Attempt:    output,
		Critique:   critique,
		Lesson:     h.extractLesson(critique),
		Success:    false,
		Iterations: iterations,
		Metadata: map[string]interface{}{
			"issues": critique.Issues,
		},
	}

	_ = h.memory.Store(ctx, record)
}

// extractLesson 从批评结果中提取教训
func (h *ReflectionHook) extractLesson(critique *reflection.CritiqueResult) string {
	if len(critique.Issues) == 0 {
		return "执行良好"
	}
	return fmt.Sprintf("主要问题：%s", strings.Join(critique.Issues, "；"))
}

// ========================================
// RefinementResult 改进结果
// ========================================

// RefinementResult 反思改进结果
type RefinementResult struct {
	OriginalContent string        // 原始输出
	FinalContent     string        // 最终输出
	Iterations       int           // 迭代次数
	InitialScore     float64       // 初始评分
	FinalScore       float64       // 最终评分
	UsedMemory       bool          // 是否使用了历史记忆
	Success          bool          // 是否评估通过
	Duration         time.Duration // 处理耗时
	Error            error         // 错误信息
}

// ========================================
// Hook 工厂函数
// ========================================

// NewReflectionHookFromConfig 从配置创建 Reflection Hook
func NewReflectionHookFromConfig(
	actor model.ChatModel,
	config *reflection.ReflectionConfig,
	agentID string,
) (*ReflectionHook, error) {
	if !config.Enabled {
		return &ReflectionHook{config: config, agentID: agentID}, nil
	}

	// 适配 actor 为 ChatModel 接口
	chatModel := &einoModelAdapter{model: actor}

	// 创建 Critic
	critic, err := createCritic(config, chatModel)
	if err != nil {
		return nil, err
	}

	// 注意：Memory 需要由调用者单独创建并传入
	// 因为 memory 包依赖于外部 embedder 和 Milvus 客户端
	return NewReflectionHook(chatModel, critic, nil, config, agentID), nil
}

// SetMemory 设置 Memory (可选)
func (h *ReflectionHook) SetMemory(memory reflection.ReflectionMemory) {
	h.memory = memory
}

// einoModelAdapter 将 eino ChatModel 适配为 ChatModel 接口
type einoModelAdapter struct {
	model model.ChatModel
}

func (a *einoModelAdapter) Chat(ctx context.Context, messages []Message, opts interface{}) (interface{}, error) {
	// 转换消息格式
	einoMessages := make([]*schema.Message, len(messages))
	for i, msg := range messages {
		switch msg.Role {
		case "system":
			einoMessages[i] = schema.SystemMessage(msg.Content)
		case "assistant":
			einoMessages[i] = schema.AssistantMessage(msg.Content, nil)
		default: // user
			einoMessages[i] = schema.UserMessage(msg.Content)
		}
	}

	// 调用模型
	return a.model.Generate(ctx, einoMessages)
}

// createCritic 创建 Critic 实例
func createCritic(config *reflection.ReflectionConfig, llm ChatModel) (reflection.Critic, error) {
	// 这里简化处理，实际应该从 application 层导入 factory
	// 由于跨层导入限制，这里简单创建 LLMCritic
	return &llmCriticImpl{llm: llm, config: config}, nil
}

// llmCriticImpl 简单的 LLM Critic 实现
type llmCriticImpl struct {
	llm    ChatModel
	config *reflection.ReflectionConfig
}

func (c *llmCriticImpl) Evaluate(ctx context.Context, task, output string) (*reflection.CritiqueResult, error) {
	prompt := c.buildPrompt(task, output)
	_, err := c.llm.Chat(ctx, []Message{{Role: "user", Content: prompt}}, nil)
	if err != nil {
		return nil, err
	}
	// 简化：返回默认结果
	return &reflection.CritiqueResult{
		OverallScore: 0.8,
		Dimensions:   make(map[string]reflection.DimensionScore),
		ShouldRefine: false,
	}, nil
}

func (c *llmCriticImpl) ShouldRefine(result *reflection.CritiqueResult) bool {
	return result.OverallScore < 0.7 || len(result.Issues) > 0
}

func (c *llmCriticImpl) buildPrompt(task, output string) string {
	return fmt.Sprintf(`评估以下回答的质量：

任务：%s

回答：%s

请返回 JSON 格式评估：
{
    "overall_score": 0-1的评分,
    "issues": ["问题列表"],
    "should_refine": true/false
}`, task, output)
}
