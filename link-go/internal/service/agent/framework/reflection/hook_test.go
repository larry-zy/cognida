// Package reflection provides tests for the Reflection Hook.
package reflection

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"link/internal/model/agent/reflection"
)

// mockChatModel 是一个模拟的 ChatModel
type mockChatModel struct {
	responses []string
	index     int
}

func (m *mockChatModel) Chat(ctx context.Context, messages []Message, opts interface{}) (interface{}, error) {
	resp := m.responses[m.index%len(m.responses)]
	m.index++
	return struct {
		Content string
	}{Content: resp}, nil
}

// mockCritic 是一个模拟的 Critic
type mockCritic struct {
	shouldRefine   bool
	score          float64
	callCount      int    // 跟踪调用次数
	stopAfterCount int    // 在第 N 次调用后停止 refine
}

func (m *mockCritic) Evaluate(ctx context.Context, task, output string) (*reflection.CritiqueResult, error) {
	m.callCount++
	shouldRefine := m.shouldRefine
	// 如果设置了停止次数且达到该次数，则不再 refine
	if m.stopAfterCount > 0 && m.callCount >= m.stopAfterCount {
		shouldRefine = false
	}
	return &reflection.CritiqueResult{
		OverallScore: m.score,
		Dimensions: map[string]reflection.DimensionScore{
			"test": {Score: m.score, Feedback: "test feedback"},
		},
		ShouldRefine: shouldRefine,
	}, nil
}

func (m *mockCritic) ShouldRefine(result *reflection.CritiqueResult) bool {
	return result.ShouldRefine
}

// TestReflectionHook_Refine 测试反思改进功能
func TestReflectionHook_Refine(t *testing.T) {
	tests := []struct {
		name         string
		shouldRefine bool
		score        float64
		wantIter     int
		wantSuccess  bool
	}{
		{
			name:         "no refinement needed",
			shouldRefine: false,
			score:        0.8,
			wantIter:     1,
			wantSuccess:  true,
		},
		{
			name:         "one refinement needed",
			shouldRefine: true,
			score:        0.9, // 第二次不 refine
			wantIter:     2,
			wantSuccess:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			model := &mockChatModel{
				responses: []string{"initial response", "improved response", "final response"},
			}
			critic := &mockCritic{
				shouldRefine:   tt.shouldRefine,
				score:          tt.score,
				stopAfterCount: tt.wantIter, // 在达到期望迭代次数后停止
			}

			hook := &ReflectionHook{
				actor:  model,
				critic: critic,
				config: &reflection.ReflectionConfig{
					MaxIterations: 3,
				},
			}

			result := hook.Refine(ctx, "test task", "test content")

			assert.Equal(t, tt.wantIter, result.Iterations)
			assert.Equal(t, tt.wantSuccess, result.Success)
			assert.NotEmpty(t, result.FinalContent)
		})
	}
}

// TestReflectionHook_MaxIterations 测试最大迭代次数限制
func TestReflectionHook_MaxIterations(t *testing.T) {
	ctx := context.Background()
	model := &mockChatModel{
		responses: []string{"response 1", "response 2", "response 3", "response 4"},
	}
	critic := &mockCritic{
		shouldRefine: true, // 始终需要改进
		score:        0.5,  // 始终低分
	}

	hook := &ReflectionHook{
		actor:  model,
		critic: critic,
		config: &reflection.ReflectionConfig{
			MaxIterations: 3,
		},
		agentID: "test-agent",
	}

	result := hook.Refine(ctx, "test task", "test content")

	// 应该达到最大迭代次数
	assert.Equal(t, 3, result.Iterations)
	// 最终不成功（因为仍然需要改进）
	assert.False(t, result.Success)
}

// TestReflectionHook_IsEnabled 测试启用检查
func TestReflectionHook_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   *reflection.ReflectionConfig
		wantBool bool
	}{
		{
			name:     "nil config",
			config:   nil,
			wantBool: false,
		},
		{
			name: "disabled",
			config: &reflection.ReflectionConfig{
				Enabled: false,
			},
			wantBool: false,
		},
		{
			name: "enabled",
			config: &reflection.ReflectionConfig{
				Enabled: true,
			},
			wantBool: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := &ReflectionHook{config: tt.config}
			assert.Equal(t, tt.wantBool, hook.IsEnabled())
		})
	}
}

// TestReflectionHook_Duration 测试处理耗时记录
func TestReflectionHook_Duration(t *testing.T) {
	ctx := context.Background()
	model := &mockChatModel{
		responses: []string{"response"},
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

	result := hook.Refine(ctx, "test task", "test content")

	assert.GreaterOrEqual(t, result.Duration, time.Duration(0))
	assert.Less(t, result.Duration, time.Second) // 应该很快
}

// BenchmarkReflectionHook_Refine 性能测试
func BenchmarkReflectionHook_Refine(b *testing.B) {
	ctx := context.Background()
	model := &mockChatModel{
		responses: []string{"response"},
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hook.Refine(ctx, "test task", "test content")
	}
}
