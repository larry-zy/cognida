// Package id provides ID generation utilities
package id

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// ========================================
// ID Generator
// ========================================

// IDGenerator ID 生成器接口
type IDGenerator interface {
	// Generate 生成新的唯一ID
	Generate() string
}

// DefaultIDGenerator 默认 ID 生成器实现
type DefaultIDGenerator struct{}

// NewIDGenerator 创建 ID 生成器
func NewIDGenerator() IDGenerator {
	return &DefaultIDGenerator{}
}

// Generate 生成新的唯一ID
// 格式: 时间戳(14位) + 随机字符串(16位)
// 示例: 20250502153045a3f8b2c1d4e5f6a7b8
func (g *DefaultIDGenerator) Generate() string {
	timestamp := time.Now().Format("20060102150405")
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		// crypto/rand 读取失败属不可恢复的系统级错误（熵源不可用），
		// 降级用零值随机串会产出可预测 ID，安全上不可接受；接口签名无法返回 error，故 panic。
		panic(fmt.Sprintf("id: crypto/rand read failed: %v", err))
	}
	randomStr := hex.EncodeToString(randomBytes)
	return timestamp + randomStr
}

// ========================================
// Global ID Generator Instance
// ========================================

var globalIDGenerator IDGenerator = NewIDGenerator()

// SetIDGenerator 设置全局 ID 生成器（主要用于测试）
func SetIDGenerator(generator IDGenerator) {
	globalIDGenerator = generator
}

// GenerateID 生成新的唯一ID（使用全局生成器）
func GenerateID() string {
	return globalIDGenerator.Generate()
}
