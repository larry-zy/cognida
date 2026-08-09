// Package tools 提供工具依赖聚合体 ToolDeps：以显式构造注入替代包级全局与 init() 副作用。
package tools

import (
	"gorm.io/gorm"

	"cognida/internal/model/dataprofile"
	model_datasource "cognida/internal/model/datasource"
	domain_knowledge "cognida/internal/model/knowledge"
	"cognida/internal/model/semantic"
	"cognida/internal/service/agent/resultstore"
	"cognida/internal/service/agent/semanticcache"
	"cognida/internal/service/agent/termgrounding"
	"cognida/internal/service/agent/uibinding"
)

// ToolDeps 聚合工具注册表所需的全部依赖，由组合根一次性装配后传入 NewToolRegistry。
//
// 必需依赖缺失 → 构造失败；可选依赖缺失 → 对应工具仍注册，但 Run 时返回既定降级提示
// （宁拒不闯，行为与旧全局注入路径逐字节保持一致）。
type ToolDeps struct {
	// SQLDB 业务库连接（必需）：sql_execute / get_schema 只读查询的默认目标。
	SQLDB *gorm.DB
	// GetSchemaDB get_schema 使用的库连接；nil 时回落 SQLDB。
	GetSchemaDB *gorm.DB
	// ColumnProfileStore 列画像（数据事实）存储（可选）；nil 时 get_schema 不附数据事实、零回归。
	// 落库走应用自身 MySQL（非被探查的数据源），以物理坐标为键 upsert 快照。
	ColumnProfileStore dataprofile.Store

	// RAGService RAG 检索服务（可选）；nil 时 rag_query 返回未初始化提示。
	RAGService RAGQueryService
	// ChunkFetcher 知识块精确拉取服务（可选）；nil 时 kb_fetch_chunks 返回未初始化提示。
	ChunkFetcher ChunkFetcher
	// GraphService 图谱检索服务（可选）；nil 时 graph_query 返回未初始化提示。
	GraphService GraphQueryService
	// ResultStore 结果存储（可选）；由 sql_execute / etl_run / data_analysis /
	// data_export / render_ui 共享，nil 时对应工具降级不落库。
	ResultStore resultstore.Store
	// SessionResults 会话取数注册表（可选）；进程内、owner 隔离，记录每会话最近取数结果与
	// SQL 去重索引，供 sql_execute 复用同句结果（去重）、data_analysis 缺 result_id 时回退
	// 到会话最近结果。nil 时两处降级为原有行为（不去重、缺来源即失败）。
	SessionResults *resultstore.SessionRegistry
	// UIBinding UI surface 回调绑定存储（可选）；render_ui 落绑定、handler 回调路由共享，
	// nil 时 render_ui 降级为无交互回调。经此字段显式注入，取代原 uibinding 包级单例。
	UIBinding uibinding.Store
	// DatasourceProvider 外部数据源连接提供者（可选）；nil 时 database_id 非空显式报错。
	DatasourceProvider model_datasource.ConnectionProvider
	// SemanticRepo 语义模型仓储（可选）；nil 时语义工具报告未启用并回退词法 NL2SQL。
	SemanticRepo semantic.Repository
	// SemanticCache 受信查询缓存（可选）；nil 时不缓存，每次经引擎生成。
	SemanticCache semanticcache.Cache
	// CoverageSink 语义查询覆盖埋点写入端口（可选）；nil 时不埋点，
	// 非 nil 时 semantic_query 每次以 best-effort 记录 covered/cache_hit/fallback。
	CoverageSink semantic.CoverageSink
	// TermGrounding 术语接地图谱端口（可选）；nil 时仅用模型内同义词接地。
	TermGrounding termgrounding.GraphPort
	// DataAnalysisInvoker MCP 调用器（可选）；nil 时 data_analysis 返回非致命错误结果。
	DataAnalysisInvoker MCPInvoker
	// KnowledgeBaseRepo 知识库仓储（可选）；nil 时 kb_list 返回未初始化错误。
	KnowledgeBaseRepo domain_knowledge.KnowledgeBaseRepository
	// MetasoClient 网络搜索客户端（可选）；nil 时 web_search 降级返回模拟数据。
	MetasoClient *MetasoClient
	// Operation 操作工具配置（sql_mutate / etl_run / data_export：写、派生、导出）。
	Operation OperationConfig
}
