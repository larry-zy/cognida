package reflection

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"link/internal/model/agent/reflection"
)

// fakeEinoModel 模拟一个 eino model.ChatModel：按 prompt 区分「评估调用」与「改写调用」。
// 评估调用返回真实 JSON 评语（首轮低分触发改写、次轮高分收敛），改写调用返回改写后的答案。
// 用于验证 NewReflectionHookFromConfig 接的是真实 LLMCritic（能解析 JSON）且改写内容会被落地。
type fakeEinoModel struct {
	critiqueCalls int
	refinedAnswer string
}

func (f *fakeEinoModel) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	prompt := messages[len(messages)-1].Content
	// LLMCritic 的评估 prompt 模板以「请评估以下回答的质量」开头。
	if strings.Contains(prompt, "请评估以下回答的质量") {
		f.critiqueCalls++
		if f.critiqueCalls == 1 {
			// 首轮：低分 + 明确问题 → ShouldRefine 为真。
			return schema.AssistantMessage(`{"overall_score":0.4,"dimensions":{},"issues":["回答过于简略"],"should_refine":true}`, nil), nil
		}
		// 次轮：高分、无问题 → 收敛。
		return schema.AssistantMessage(`{"overall_score":0.95,"dimensions":{},"issues":[],"should_refine":false}`, nil), nil
	}
	// 非评估 prompt 即为改写调用。
	return schema.AssistantMessage(f.refinedAnswer, nil), nil
}

func (f *fakeEinoModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("stream not used in reflection")
}

func (f *fakeEinoModel) BindTools(tools []*schema.ToolInfo) error { return nil }

// TestNewReflectionHookFromConfig_RealCriticAppliesRefinement 验证从配置装配的反思 Hook：
//   - 接的是真实 LLMCritic（能从模型 JSON 解析出评分，InitialScore=0.4）；
//   - einoModelAdapter 归一化为 ChatResponse，改写内容会真正落地到 FinalContent；
//   - 低分触发一次改写、次轮高分收敛，Success 为真。
func TestNewReflectionHookFromConfig_RealCriticAppliesRefinement(t *testing.T) {
	ctx := context.Background()
	fake := &fakeEinoModel{refinedAnswer: "REFINED ANSWER"}

	cfg := &reflection.ReflectionConfig{
		Enabled:       true,
		MaxIterations: 3,
		CriticType:    "llm",
	}

	hook, err := NewReflectionHookFromConfig(fake, cfg, "test-agent")
	require.NoError(t, err)
	require.True(t, hook.IsEnabled())

	result := hook.Refine(ctx, "统计上月销售额", "太简略的初始回答")

	// 解析出的初始评分来自 JSON（0.4），证明走的是真实 LLMCritic 而非桩。
	assert.InDelta(t, 0.4, result.InitialScore, 1e-9)
	// 改写内容被真正落地（若 einoModelAdapter 未归一化为 ChatResponse，这里会退回初始回答）。
	assert.Equal(t, "REFINED ANSWER", result.FinalContent)
	assert.True(t, result.Success)
	assert.Equal(t, 2, result.Iterations)
}

// TestNewReflectionHookFromConfig_Disabled 验证禁用配置下不装配 Critic/Actor，IsEnabled 为假。
func TestNewReflectionHookFromConfig_Disabled(t *testing.T) {
	hook, err := NewReflectionHookFromConfig(&fakeEinoModel{}, &reflection.ReflectionConfig{Enabled: false}, "test-agent")
	require.NoError(t, err)
	assert.False(t, hook.IsEnabled())
}
