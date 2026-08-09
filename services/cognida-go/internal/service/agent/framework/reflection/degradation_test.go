// Package reflection provides degradation tests for the Reflection Hook.
package reflection

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"cognida/internal/model/agent/reflection"
)

// failingCritic 模拟失败的 Critic
type failingCritic struct{}

func (f *failingCritic) Evaluate(ctx context.Context, task, output string) (*reflection.CritiqueResult, error) {
	return nil, errors.New("critic failed")
}

func (f *failingCritic) ShouldRefine(result *reflection.CritiqueResult) bool {
	return false
}

// failingActor 模拟失败的 Actor
type failingActor struct{}

func (f *failingActor) Chat(ctx context.Context, messages []Message, opts interface{}) (interface{}, error) {
	return nil, errors.New("actor failed")
}

// failingMemory 模拟失败的 Memory
type failingMemory struct{}

func (f *failingMemory) Store(ctx context.Context, record *reflection.ReflectionRecord) error {
	return errors.New("memory store failed")
}

func (f *failingMemory) Retrieve(ctx context.Context, agentID, task string, limit int) ([]*reflection.ReflectionRecord, error) {
	return nil, errors.New("memory retrieve failed")
}

func (f *failingMemory) UpdateSuccess(ctx context.Context, id string) error {
	return nil
}

func (f *failingMemory) Cleanup(ctx context.Context) error {
	return nil
}

// TestReflectionHook_CriticFailure 测试 Critic 失败时的降级
func TestReflectionHook_CriticFailure(t *testing.T) {
	ctx := context.Background()
	model := &mockChatModel{
		responses: []string{"initial response"},
	}
	critic := &failingCritic{}

	hook := &ReflectionHook{
		actor:  model,
		critic: critic,
		config: &reflection.ReflectionConfig{
			MaxIterations: 3,
		},
	}

	result := hook.Refine(ctx, "test task", "test content")

	// 应该降级返回原始内容
	assert.Equal(t, "test content", result.FinalContent)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "critic evaluation failed")
}

// TestReflectionHook_ActorFailure 测试 Actor 失败时的降级
func TestReflectionHook_ActorFailure(t *testing.T) {
	ctx := context.Background()
	actor := &failingActor{}
	critic := &mockCritic{
		shouldRefine: true, // 需要 refine，触发 Actor 调用
		score:        0.5,
	}

	hook := &ReflectionHook{
		actor:  actor,
		critic: critic,
		config: &reflection.ReflectionConfig{
			MaxIterations: 3,
		},
	}

	result := hook.Refine(ctx, "test task", "test content")

	// 应该返回当前内容（初始内容）
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "actor refinement failed")
}

// TestReflectionHook_MemoryFailure 测试 Memory 失败时的处理
func TestReflectionHook_MemoryFailure(t *testing.T) {
	ctx := context.Background()
	model := &mockChatModel{
		responses: []string{"response"},
	}
	critic := &mockCritic{
		shouldRefine: false,
		score:        0.8,
	}
	memory := &failingMemory{}

	hook := &ReflectionHook{
		actor:  model,
		critic: critic,
		memory: memory,
		config: &reflection.ReflectionConfig{
			MaxIterations: 1,
			Memory: &reflection.MemoryConfig{
				Enabled: true,
			},
		},
		agentID: "test-agent",
	}

	// Memory 失败不应该阻止反思流程
	result := hook.Refine(ctx, "test task", "test content")

	// 应该正常完成，只是没有使用记忆
	assert.True(t, result.Success)
	assert.False(t, result.UsedMemory) // 未使用记忆
	assert.NotEmpty(t, result.FinalContent)
}

// TestReflectionHook_BackwardCompatibility 测试向后兼容性
func TestReflectionHook_BackwardCompatibility(t *testing.T) {
	ctx := context.Background()
	model := &mockChatModel{
		responses: []string{"response"},
	}
	critic := &mockCritic{
		shouldRefine: false,
		score:        0.8,
	}

	tests := []struct {
		name     string
		config   *reflection.ReflectionConfig
		memory   reflection.ReflectionMemory
		wantBool bool
	}{
		{
			name:     "disabled reflection",
			config:   &reflection.ReflectionConfig{Enabled: false},
			memory:   nil,
			wantBool: false,
		},
		{
			name: "enabled but nil memory",
			config: &reflection.ReflectionConfig{
				Enabled:       true,
				MaxIterations: 1,
			},
			memory:   nil,
			wantBool: true, // 应该工作，只是没有记忆功能
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := NewReflectionHook(model, critic, tt.memory, tt.config, "test-agent")

			// IsEnabled 应该正确报告状态
			assert.Equal(t, tt.wantBool, hook.IsEnabled())

			// Refine 应该正常工作（或返回原始结果）
			result := hook.Refine(ctx, "test task", "test content")
			assert.NotNil(t, result)
		})
	}
}

// TestReflectionHook_EmptyOutput 测试空输出处理
func TestReflectionHook_EmptyOutput(t *testing.T) {
	ctx := context.Background()
	model := &mockChatModel{
		responses: []string{""}, // 空响应
	}
	critic := &mockCritic{
		shouldRefine: false,
		score:        0.8,
	}

	hook := &ReflectionHook{
		actor:  model,
		critic: critic,
		config: &reflection.ReflectionConfig{
			MaxIterations: 1,
		},
	}

	result := hook.Refine(ctx, "test task", "")

	// 应该正常处理空输出
	assert.NotNil(t, result)
	assert.True(t, result.Success)
}

// TestReflectionHook_LongTask 测试长任务描述处理
func TestReflectionHook_LongTask(t *testing.T) {
	ctx := context.Background()
	model := &mockChatModel{
		responses: []string{"response"},
	}
	critic := &mockCritic{
		shouldRefine: false,
		score:        0.8,
	}

	longTask := string(make([]byte, 10000)) // 10KB 任务描述
	for i := 0; i < len(longTask); i++ {
		longTask += "a" // 填充内容
		if len(longTask) >= 10000 {
			break
		}
	}

	hook := &ReflectionHook{
		actor:  model,
		critic: critic,
		config: &reflection.ReflectionConfig{
			MaxIterations: 1,
		},
	}

	// 应该正常处理长任务
	result := hook.Refine(ctx, longTask, "test content")
	assert.NotNil(t, result)
	assert.True(t, result.Success)
}
