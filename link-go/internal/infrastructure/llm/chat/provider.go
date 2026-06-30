package chat

import (
	"fmt"
	"strings"
)

// ========================================
// Provider Constants
// ========================================

const (
	ProviderOpenAI   = "openai"
	ProviderAliyun   = "aliyun"
	ProviderDeepSeek = "deepseek"
	ProviderLKEAP    = "lkeap"
	ProviderQwen     = "qwen"
	ProviderGeneric  = "generic"
)

// ProviderName provider名称类型
type ProviderName string

// Provider provider枚举
var Provider = struct {
	OpenAI   ProviderName
	Aliyun   ProviderName
	DeepSeek ProviderName
	LKEAP    ProviderName
	Qwen     ProviderName
	Generic  ProviderName
}{
	OpenAI:   ProviderOpenAI,
	Aliyun:   ProviderAliyun,
	DeepSeek: ProviderDeepSeek,
	LKEAP:    ProviderLKEAP,
	Qwen:     ProviderQwen,
	Generic:  ProviderGeneric,
}

// ========================================
// Provider Detection
// ========================================

// DetectProvider 根据BaseURL检测provider
func DetectProvider(baseURL string) string {
	lowerURL := strings.ToLower(baseURL)

	switch {
	case strings.Contains(lowerURL, "aliyun"):
		return ProviderAliyun
	case strings.Contains(lowerURL, "deepseek"):
		return ProviderDeepSeek
	case strings.Contains(lowerURL, "lkeap"):
		return ProviderLKEAP
	case strings.Contains(lowerURL, "qwen"):
		return ProviderQwen
	default:
		return ProviderGeneric
	}
}

// ========================================
// Provider Helper Functions
// ========================================

// IsQwen3Model 检查是否为Qwen3模型
func IsQwen3Model(modelName string) bool {
	return strings.HasPrefix(strings.ToLower(modelName), "qwen3")
}

// GetProviderByName 根据名称获取provider
func GetProviderByName(name string) ProviderName {
	switch strings.ToLower(name) {
	case ProviderOpenAI:
		return Provider.OpenAI
	case ProviderAliyun:
		return Provider.Aliyun
	case ProviderDeepSeek:
		return Provider.DeepSeek
	case ProviderLKEAP:
		return Provider.LKEAP
	case ProviderQwen:
		return Provider.Qwen
	default:
		return Provider.Generic
	}
}

// ValidateProvider 验证provider
func ValidateProvider(provider string) error {
	validProviders := []string{
		ProviderOpenAI,
		ProviderAliyun,
		ProviderDeepSeek,
		ProviderLKEAP,
		ProviderQwen,
		ProviderGeneric,
	}

	for _, p := range validProviders {
		if strings.EqualFold(provider, p) {
			return nil
		}
	}

	return fmt.Errorf("invalid provider: %s, must be one of: %v", provider, validProviders)
}
