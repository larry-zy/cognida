// Package critic provides Critic implementations for reflection.
package critic

import (
	"fmt"

	"github.com/cloudwego/eino/components/model"

	"cognida/internal/model/agent/reflection"
)

// ========================================
// Critic 工厂
// ========================================

// Factory 创建 Critic 实例
func Factory(criticType string, llm model.ChatModel, dimensions []reflection.DimensionConfig) (reflection.Critic, error) {
	switch criticType {
	case "llm":
		if llm == nil {
			return nil, fmt.Errorf("llm critic requires chat model")
		}
		return NewLLMCritic(llm, dimensions), nil

	case "rule":
		return NewRuleCritic(nil), nil

	default:
		return nil, reflection.ErrInvalidCriticType
	}
}

// NewCritic 创建 Critic（便捷方法）
func NewCritic(config *reflection.ReflectionConfig, llm model.ChatModel) (reflection.Critic, error) {
	return Factory(config.CriticType, llm, config.Dimensions)
}
