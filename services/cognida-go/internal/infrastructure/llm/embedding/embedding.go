package embedding

import (
	"context"
	"fmt"

	"cognida/internal/config"
	"github.com/cloudwego/eino/components/embedding"
)

// NewDashScopeEmbedderWrapper 创建 DashScope Embedder（实现 Eino 接口）
func NewDashScopeEmbedderWrapper(cfg *config.EmbeddingConfig) (embedding.Embedder, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("EMBEDDING_API_KEY is required")
	}

	impl := NewDashScopeEmbedder(cfg)

	return &DashScopeEmbedderWrapper{
		embedder: impl,
	}, nil
}

// DashScopeEmbedderWrapper 包装器，实现 Eino Embedder 接口
type DashScopeEmbedderWrapper struct {
	embedder *DashScopeEmbedder
}

// EmbedStrings 批量向量化文本（实现 Eino Embedder 接口）
func (w *DashScopeEmbedderWrapper) EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
	// TODO: 如果需要支持 Eino 的选项（如 WithModel），可以在这里处理
	// 目前暂时忽略选项
	_ = opts

	return w.embedder.EmbedStrings(ctx, texts)
}

// ========================================
// 通用工厂函数
// ========================================

// NewEmbedder 根据配置创建 Embedder
func NewEmbedder(cfg *config.EmbeddingConfig) (embedding.Embedder, error) {
	switch cfg.Provider {
	case "dashscope":
		return NewDashScopeEmbedderWrapper(cfg)
	case "openai":
		return NewOpenAIEmbedderWrapper(cfg)
	case "ollama":
		return NewOllamaEmbedderWrapper(cfg)
	default:
		return NewDashScopeEmbedderWrapper(cfg)
	}
}
