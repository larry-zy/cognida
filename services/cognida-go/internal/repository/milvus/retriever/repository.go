package retriever

import (
	"fmt"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/milvus-io/milvus/client/v2/milvusclient"

	"cognida/internal/repository/milvus"
)

// VectorRetriever 向量检索器
type VectorRetriever struct {
	client   *milvusclient.Client
	embedder embedding.Embedder
}

// NewVectorRetriever 创建向量检索器
func NewVectorRetriever(embedder embedding.Embedder) (*VectorRetriever, error) {
	if embedder == nil {
		return nil, fmt.Errorf("embedder is required")
	}

	cli := milvus.GetClient()
	if cli == nil {
		return nil, fmt.Errorf("milvus client not initialized")
	}

	return &VectorRetriever{
		client:   cli,
		embedder: embedder,
	}, nil
}
