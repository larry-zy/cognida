// Package docreader provides gRPC client tests
package docreader

import (
	"context"
	"testing"
	"time"
)

// TestDocReaderClient 测试 Python 文档服务连接
func TestDocReaderClient(t *testing.T) {
	if testing.Short() {
		t.Skip("需要运行中的 Python docreader gRPC 服务(127.0.0.1:50051), 跳过短测")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := NewClient("127.0.0.1:50051")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// 测试文本分块
	resp, err := client.ChunkDocument(ctx, &ChunkDocumentRequest{
		Text:     "Hello world. This is a test. Another sentence here.",
		Strategy: ChunkStrategyParagraph,
		Options: &ChunkOptions{
			ChunkSize: 100,
		},
	})

	if err != nil {
		t.Fatalf("ChunkDocument failed: %v", err)
	}

	t.Logf("Got %d chunks", resp.TotalCount)
	for i, chunk := range resp.Chunks {
		t.Logf("Chunk %d: %s", i, chunk.Text)
	}
}

// TestParseDocument 测试文档解析
func TestParseDocument(t *testing.T) {
	if testing.Short() {
		t.Skip("需要运行中的 Python docreader gRPC 服务(127.0.0.1:50051), 跳过短测")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := NewClient("127.0.0.1:50051")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// 创建测试文件
	testContent := []byte("# Test Document\n\nThis is a test markdown file.")

	_, err = client.ParseDocument(ctx, &ParseDocumentRequest{
		Content: testContent,
		Format:  "md",
	})

	if err != nil {
		t.Fatalf("ParseDocument failed: %v", err)
	}

	t.Log("ParseDocument succeeded")
}

// TestFetchURL 测试 URL 获取
func TestFetchURL(t *testing.T) {
	if testing.Short() {
		t.Skip("需要运行中的 Python docreader gRPC 服务与外网访问, 跳过短测")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := NewClient("127.0.0.1:50051")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	resp, err := client.FetchURL(ctx, &FetchURLRequest{
		URL: "https://httpbin.org/json",
	})

	if err != nil {
		t.Fatalf("FetchURL failed: %v", err)
	}

	t.Logf("FetchURL succeeded: title=%s, text_len=%d", resp.Title, len(resp.Text))
}
