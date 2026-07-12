// 工具：从 GORM model 同步业务表结构到数据库（本项目主流程无 SQL 迁移文件，
// 加字段/建表后用它把库对齐到 model，替代手动 DDL）。
// 用法：cd link-go && set -a && source .env && set +a && go run ./cmd/migrate-db
// 读取 DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME 环境变量（密码不设默认，须由环境提供）。
//
// 注意：图谱表（graph_nodes/graph_relations/graph_communities/graph_community_members）
// 由 graphMetaRepository.ensureSchema 用内部（不可导出）model 懒加载建表，本工具不重复迁移，
// 以免与其真实 schema 冲突。
package main

import (
	"fmt"
	"log"
	"os"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	mysqlrepo "link/internal/repository/mysql"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		env("DB_USER", "root"),
		env("DB_PASSWORD", ""),
		env("DB_HOST", "127.0.0.1"),
		env("DB_PORT", "3306"),
		env("DB_NAME", "link"),
	)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("connect failed: %v", err)
	}

	// 所有导出的业务 model（按领域分组，图谱表除外）。
	models := []interface{}{
		// 租户 / 用户 / 鉴权
		&mysqlrepo.TenantModel{},
		&mysqlrepo.TenantUserModel{},
		&mysqlrepo.UserModel{},
		&mysqlrepo.RefreshTokenModel{},
		&mysqlrepo.SessionModel{},
		&mysqlrepo.AuditLogModel{},
		// 知识库 / 检索
		&mysqlrepo.KnowledgeBaseModel{},
		&mysqlrepo.KnowledgeBaseSettingModel{},
		&mysqlrepo.KnowledgeModel{},
		&mysqlrepo.ChunkModel{},
		&mysqlrepo.TagModel{},
		&mysqlrepo.RetrievalSettingModel{},
		// 会话 / 智能体 / 模型
		&mysqlrepo.AgentModel{},
		&mysqlrepo.MessageModel{},
		&mysqlrepo.Model{}, // llm_models
		// 评测
		&mysqlrepo.DatasetModel{},
		&mysqlrepo.DatasetRecordModel{},
		&mysqlrepo.EvaluationTaskModel{},
		&mysqlrepo.EvaluationResultModel{},
		// 指标语义层（NL2Semantics）
		&mysqlrepo.SemanticModelModel{},
		&mysqlrepo.SemanticLogicalTableModel{},
		&mysqlrepo.SemanticDimensionModel{},
		&mysqlrepo.SemanticMeasureModel{},
		&mysqlrepo.SemanticMetricModel{},
		&mysqlrepo.SemanticRelationModel{},
		&mysqlrepo.SemanticCoverageLogModel{},
		// 数据质量管理中心
		&mysqlrepo.QualityCheckRecordModel{},
		// Data Agent 操作审计
		&mysqlrepo.OperationAuditModel{},
		// 外部数据源注册表
		&mysqlrepo.DataSourceModel{},
	}

	if err := db.AutoMigrate(models...); err != nil {
		log.Fatalf("automigrate failed: %v", err)
	}

	fmt.Printf("OK: %d tables synced from models\n", len(models))
}
