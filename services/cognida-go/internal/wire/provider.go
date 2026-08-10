// Package wire 提供依赖注入的提供者函数
package wire

import (
	"context"
	"log"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// ========================================
// Config Providers - 配置提供者
// ========================================

// Neo4jConfig Neo4j 配置
type Neo4jConfig struct {
	URI      string
	Username string
	Password string
}

// CreateDriver 创建 Neo4j 驱动
func CreateDriver(ctx context.Context, cfg Neo4jConfig) (neo4j.DriverWithContext, error) {
	driver, err := neo4j.NewDriverWithContext(
		cfg.URI,
		neo4j.BasicAuth(cfg.Username, cfg.Password, ""),
	)
	if err != nil {
		return nil, err
	}

	// 测试连接
	if err := driver.VerifyConnectivity(ctx); err != nil {
		return nil, err
	}

	log.Printf("✅ Neo4j 连接成功: %s\n", cfg.URI)
	return driver, nil
}
