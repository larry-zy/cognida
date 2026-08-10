package milvus

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/milvus-io/milvus/client/v2/milvusclient"

	"cognida/internal/config"
)

var (
	// MilvusClient Milvus 客户端单例
	MilvusClient *milvusclient.Client
)

// InitMilvus 初始化 Milvus 连接
func InitMilvus(cfg *config.MilvusConfig) error {
	if cfg.Host == "" || cfg.Token == "" {
		return fmt.Errorf("Milvus配置不完整: host或token为空")
	}

	// 创建新 SDK v2 客户端
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := milvusclient.New(ctx, &milvusclient.ClientConfig{
		Address: cfg.Host,
		APIKey:  cfg.Token,
	})
	if err != nil {
		return fmt.Errorf("创建Milvus客户端失败: %w", err)
	}

	// 测试连接 - 通过列出collections来验证
	_, err = c.ListCollections(ctx, milvusclient.NewListCollectionOption())
	if err != nil {
		return fmt.Errorf("Milvus连接测试失败: %w", err)
	}

	MilvusClient = c
	log.Printf("✅ Milvus连接成功: %s\n", cfg.Host)

	return nil
}

// CloseMilvus 关闭 Milvus 连接
func CloseMilvus() error {
	if MilvusClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return MilvusClient.Close(ctx)
	}
	return nil
}

// GetClient 获取 Milvus 客户端
func GetClient() *milvusclient.Client {
	return MilvusClient
}
