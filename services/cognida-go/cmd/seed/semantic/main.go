// 工具：为电商演示库（ecommerce_demo）冷启动写入受治理的语义模型，
// 让 semantic_query 走「治理口径」主路径而非每次回退词法 NL2SQL。
//
// 用法：cd cognida-go && set -a && source .env && set +a && go run ./cmd/seed/semantic
// 连接的是 link 应用库（元数据库，非业务库 ecommerce_demo），经 GORM +
// SemanticRepository.UpsertBundle 全量幂等写入；固定 ID，可反复执行。
//
// 落库对象：租户 tenant_id=1（dev 用户）、status=active 的两套语义模型：
//   - 电商销售（grain=order，orders ⋈ customers）
//   - 商品销售（grain=order_item，order_items ⋈ products ⋈ categories）
//
// 两套均为「扇出安全」：所有 JOIN 都是从事实表出发的 to-one 关系，聚合不虚增。
//
// 前置：ecommerce_demo 已 seed（cmd/seed/ecommerce），且 link 库已 migrate-db。
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"cognida/internal/model/semantic"
	mysqlrepo "cognida/internal/repository/mysql"
	semanticseed "cognida/internal/seed/semantic"
	"cognida/internal/service/agent/metricsql"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// seedTenant 目标租户：dev 用户 tenant_id=1（与 users.tenant_id 修复后一致）。
const seedTenant = int64(1)

// ecommerceDatasourceID 电商演示库（ecommerce_demo）数据源 ID：写入逻辑表的
// DatabaseID 绑定，使 semantic_query 返回的治理 SQL 显式打向该外部数据源，
// 而非依赖「会话隐式选库」。默认值为 dev 环境登记的 ID，可经 env 覆盖以适配他机。
// 查询来源：SELECT id FROM data_sources WHERE name='电商演示库' AND tenant_id=1。
var ecommerceDatasourceID = env("ECOMMERCE_DATASOURCE_ID", "20260705030628ad7827ced5604d2c")

func main() {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		env("DB_USER", "root"),
		env("DB_PASSWORD", ""),
		env("DB_HOST", "127.0.0.1"),
		env("DB_PORT", "3306"),
		env("DB_NAME", "cognida"),
	)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("connect failed: %v", err)
	}

	// 前置校验：语义表须已由版本化迁移（cmd/migrate-db up，〔INF-4〕）建好。
	// 本工具只写种子数据，不再私自建表——schema 唯一真源是 migrations/，避免绕过迁移造成漂移。
	if !db.Migrator().HasTable(&mysqlrepo.SemanticModelModel{}) {
		log.Fatalf("语义表缺失：请先执行 `go run ./cmd/migrate-db up` 应用迁移，再运行本 seed 工具")
	}

	repo := mysqlrepo.NewSemanticRepository(db)
	ctx := context.Background()

	bundles := []*semantic.ModelBundle{semanticseed.SalesBundle(seedTenant, ecommerceDatasourceID, ""), semanticseed.ProductBundle(seedTenant, ecommerceDatasourceID, "")}
	for _, b := range bundles {
		// 建模期治理体检（D1/D2/D3）：非致命，仅提示。修复后的模型应全清；若报出
		// GRAIN_IDENTITY/FANOUT_RISK/FACT_NO_PK 说明有实体粒度/扇出/主键缺失风险，需先治理。
		if warns := metricsql.ValidateBundle(b); len(warns) > 0 {
			fmt.Printf("WARN: 语义治理校验发现 %d 项风险 @ %q：\n", len(warns), b.Model.Name)
			for _, w := range warns {
				fmt.Printf("  - %s\n", w.String())
			}
		}
		if err := repo.UpsertBundle(ctx, b); err != nil {
			log.Fatalf("upsert bundle %q failed: %v", b.Model.Name, err)
		}
		fmt.Printf("OK: seeded semantic model %q (tenant=%d, %d tables / %d dims / %d measures / %d metrics)\n",
			b.Model.Name, b.Model.TenantID,
			len(b.LogicalTables), len(b.Dimensions), len(b.Measures), len(b.Metrics))
	}
	fmt.Printf("DONE: %d semantic models active for tenant %d\n", len(bundles), seedTenant)
}
