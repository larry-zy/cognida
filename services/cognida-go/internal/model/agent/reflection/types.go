// Package reflection provides the domain types for Agent self-reflection capability.
package reflection

import (
	"time"
)

// ========================================
// 核心类型定义
// ========================================

// CritiqueResult 评估结果
type CritiqueResult struct {
	OverallScore float64                  `json:"overall_score"` // 总体评分 (0-1)
	Dimensions   map[string]DimensionScore `json:"dimensions"`    // 各维度评分
	Issues      []string                  `json:"issues"`        // 发现的问题
	Suggestions  []string                 `json:"suggestions"`   // 改进建议
	ShouldRefine bool                     `json:"should_refine"` // 是否需要改进
}

// DimensionScore 维度评分
type DimensionScore struct {
	Score    float64 `json:"score"`    // 评分 (0-1)
	Feedback string  `json:"feedback"` // 反馈说明
}

// ReflectionRecord 反思记录
type ReflectionRecord struct {
	ID        string                 `json:"id"`         // 记录 ID
	AgentID   string                 `json:"agent_id"`   // Agent ID
	Task      string                 `json:"task"`       // 任务描述
	Attempt   string                 `json:"attempt"`    // 尝试的输出
	Critique  *CritiqueResult        `json:"critique"`   // 批评结果
	Lesson    string                 `json:"lesson"`     // 学到的教训
	CreatedAt time.Time              `json:"created_at"` // 创建时间
	UpdatedAt time.Time              `json:"updated_at"` // 更新时间
	UsedCount  int                   `json:"used_count"` // 被使用次数
	Success    bool                  `json:"success"`    // 是否最终成功
	Iterations int                   `json:"iterations"` // 迭代次数
	Metadata   map[string]interface{} `json:"metadata"`   // 附加元数据
}

// ReflectionConfig 反思配置
type ReflectionConfig struct {
	Enabled       bool                `json:"enabled"`        // 是否启用
	MaxIterations int                 `json:"max_iterations"` // 最大迭代次数
	CriticType    string              `json:"critic_type"`    // Critic 类型: llm, rule
	CriticModel   string              `json:"critic_model"`   // Critic 使用的模型 (可选)
	Dimensions    []DimensionConfig   `json:"dimensions"`     // 评估维度配置
	Memory        *MemoryConfig       `json:"memory"`         // Memory 配置
}

// DimensionConfig 维度配置
type DimensionConfig struct {
	Name          string  `json:"name"`           // 维度名称
	Weight        float64 `json:"weight"`         // 权重
	PassThreshold float64 `json:"pass_threshold"` // 及格阈值
	Description   string  `json:"description"`    // 维度描述（可选）
}

// MemoryConfig Memory 配置
type MemoryConfig struct {
	Enabled    bool          `json:"enabled"`    // 是否启用
	TTL        time.Duration `json:"ttl"`        // 保留时间
	MaxEntries int           `json:"max_entries"` // 最大条目数
	VectorDim  int           `json:"vector_dim"` // 向量维度
}

// DefaultReflectionConfig 返回默认配置
func DefaultReflectionConfig() *ReflectionConfig {
	return &ReflectionConfig{
		Enabled:       false,
		MaxIterations: 3,
		CriticType:    "llm",
		CriticModel:   "",
		Dimensions:    DefaultDimensions(),
		Memory: &MemoryConfig{
			Enabled:    true,
			TTL:        720 * time.Hour, // 30天
			MaxEntries: 10000,
			VectorDim:  1536,
		},
	}
}

// DefaultDimensions 返回默认评估维度
func DefaultDimensions() []DimensionConfig {
	return []DimensionConfig{
		{Name: "factual", Weight: 0.3, PassThreshold: 0.8},
		{Name: "logic", Weight: 0.25, PassThreshold: 0.75},
		{Name: "completeness", Weight: 0.25, PassThreshold: 0.7},
		{Name: "clarity", Weight: 0.2, PassThreshold: 0.7},
	}
}

// Validate 验证配置
func (c *ReflectionConfig) Validate() error {
	if c.MaxIterations < 1 || c.MaxIterations > 10 {
		return ErrInvalidMaxIterations
	}
	if c.CriticType != "llm" && c.CriticType != "rule" {
		return ErrInvalidCriticType
	}
	return nil
}
