// Package llmsummary 提供 ctxeng.Summarizer 的「LLM 语义摘要」实现，替代能力层内置的
// 确定性抽取式摘要（ExtractiveSummarizer 只做逐轮截断拼接，语义大量丢失）。
//
// 分层定位：本包是「上下文工程能力层」的叶子扩展——只依赖 ctxeng（Turn/Summarizer 契约）与
// eino 生成接口，不依赖任何基础设施层，可被 framework（③ 折叠兜底）与 convcontext（开场装配）
// 复用。业务侧只需注入一个生成句柄（BaseChatModel / ToolCallingChatModel 均满足）。
//
// 健壮性契约（关键）：摘要发生在对话主循环内/开场装配处，绝不能因 LLM 慢/不可用而破坏流程。
// 因此本摘要器对超时、报错、空产出一律**降级到确定性抽取式摘要**——调用方永远拿得到结果，
// 且被引用的 result_id 由调用方的 EnsureResultIDsPresent 兜底保真（本器也会在 prompt 里强约束）。
package llmsummary

import (
	"context"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	ctxeng "link/internal/service/agent/context"
)

// LLM 是语义摘要所需的最小生成接口；eino 的 model.BaseChatModel / ToolCallingChatModel 均满足。
type LLM interface {
	Generate(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.Message, error)
}

// 默认边界：喂给摘要器的转写文本上限（字符），与单次摘要的超时。避免把超大历史整坨塞进
// 摘要调用（成本/延迟），也避免摘要调用拖垮主循环。
const (
	defaultMaxInputChars = 12000
	defaultTimeout       = 20 * time.Second
)

// Summarizer 用一次独立 LLM 调用把早期对话压成简洁事实摘要，实现 ctxeng.Summarizer。
// llm 为 nil 或调用失败时降级到 fallback（确定性抽取式摘要）。
type Summarizer struct {
	llm           LLM
	fallback      ctxeng.Summarizer
	maxInputChars int
	timeout       time.Duration
}

// New 创建 LLM 语义摘要器。llm 为 nil 时退化为纯抽取式（等价 ExtractiveSummarizer），零回归。
func New(llm LLM) *Summarizer {
	return &Summarizer{
		llm:           llm,
		fallback:      ctxeng.ExtractiveSummarizer{},
		maxInputChars: defaultMaxInputChars,
		timeout:       defaultTimeout,
	}
}

// Summarize 实现 ctxeng.Summarizer：把 older 压成保留关键决策/结论/数字/实体、且原样保住
// preserved 里全部 result_id 的紧凑摘要。任何失败路径都降级到抽取式摘要，绝不返回错误破坏主流程。
func (s *Summarizer) Summarize(ctx context.Context, older []ctxeng.Turn, preserved []string) (string, error) {
	if s == nil || s.llm == nil || len(older) == 0 {
		return s.fallbackSummarize(ctx, older, preserved)
	}

	transcript := s.buildTranscript(older)
	if strings.TrimSpace(transcript) == "" {
		return s.fallbackSummarize(ctx, older, preserved)
	}

	callCtx := ctx
	if s.timeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}

	resp, err := s.llm.Generate(callCtx, []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage(s.buildUserPrompt(transcript, preserved)),
	})
	if err != nil || resp == nil || strings.TrimSpace(resp.Content) == "" {
		return s.fallbackSummarize(ctx, older, preserved) // 降级，绝不把错误抛给主循环
	}

	// result_id 保真由调用方 EnsureResultIDsPresent 兜底；此处直接返回 LLM 摘要正文。
	return strings.TrimSpace(resp.Content), nil
}

func (s *Summarizer) fallbackSummarize(ctx context.Context, older []ctxeng.Turn, preserved []string) (string, error) {
	fb := s.fallback
	if fb == nil {
		fb = ctxeng.ExtractiveSummarizer{}
	}
	return fb.Summarize(ctx, older, preserved)
}

// buildTranscript 把早期轮拼成「role: content」转写，并按 maxInputChars 从**尾部**保留
// （越靠近当前对话的早期轮越重要，超限时先丢最老的）。
func (s *Summarizer) buildTranscript(older []ctxeng.Turn) string {
	lines := make([]string, 0, len(older))
	for _, t := range older {
		content := strings.TrimSpace(t.Content)
		if content == "" {
			continue
		}
		role := t.Role
		if role == "" {
			role = "turn"
		}
		lines = append(lines, role+": "+content)
	}
	joined := strings.Join(lines, "\n")
	limit := s.maxInputChars
	if limit <= 0 {
		limit = defaultMaxInputChars
	}
	r := []rune(joined)
	if len(r) > limit {
		joined = "…（更早内容已略）\n" + string(r[len(r)-limit:])
	}
	return joined
}

func (s *Summarizer) buildUserPrompt(transcript string, preserved []string) string {
	var b strings.Builder
	b.WriteString("把下面这段早期对话压缩成简洁的事实摘要，供后续对话作为记忆使用：\n\n")
	b.WriteString(transcript)
	b.WriteString("\n\n要求：\n")
	b.WriteString("- 保留关键决策、结论、数字、实体、口径；丢弃寒暄与冗余过程。\n")
	b.WriteString("- 用要点式中文，尽量简短，不要复述原文。\n")
	if len(preserved) > 0 {
		b.WriteString("- 必须原样保留以下 result_id（供按引用取回完整数据），不得改写或省略：")
		b.WriteString(strings.Join(preserved, ", "))
		b.WriteString("\n")
	}
	b.WriteString("- 只输出摘要正文，不要任何解释或前后缀。")
	return b.String()
}

const systemPrompt = "你是对话上下文压缩器。你的唯一任务是把冗长的早期对话无损地压成紧凑摘要，" +
	"保住事实、结论与被引用的数据句柄（result_id），供模型在后续对话里继续使用。不要发挥、不要编造。"
