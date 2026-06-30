// Package tools 提供工具依赖上下文管理
package tools

import (
	"sync"

	"link/internal/model/knowledge"
)

// ========================================
// 服务接口定义
// ========================================

// RAGQueryService RAG 查询服务接口
type RAGQueryService interface {
	Query(ctx interface{}, req *RAGQueryRequest) (*RAGQueryResult, error)
}

// GraphQueryService 图谱查询服务接口
type GraphQueryService interface {
	GraphQuery(ctx interface{}, req *GraphQueryRequest) (*GraphQueryResult, error)
}

// DataQueryService 数据查询服务接口
type DataQueryService interface {
	NaturalLanguageToSQL(ctx interface{}, query string, schema interface{}) (string, error)
	ExecuteQuery(ctx interface{}, sql string, params map[string]interface{}) (*SQLExecuteResult, error)
	GetDatabaseSchema(ctx interface{}, dbID string) (interface{}, error)
	ValidateQuery(sql string) error
}

// ========================================
// ToolContext 工具依赖上下文
// ========================================

// ToolContext 工具依赖上下文
// 管理所有工具运行时需要的服务依赖，支持线程安全访问
type ToolContext struct {
	mu sync.RWMutex

	// 服务依赖
	RAGService   RAGQueryService
	GraphService GraphQueryService
	KBRepo       knowledge.KnowledgeBaseRepository
	DataService  DataQueryService
}

// NewToolContext 创建工具上下文
func NewToolContext() *ToolContext {
	return &ToolContext{}
}

// ========================================
// 链式设置方法 (Fluent API)
// ========================================

// WithRAGService 设置 RAG 服务
func (c *ToolContext) WithRAGService(svc RAGQueryService) *ToolContext {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.RAGService = svc
	return c
}

// WithGraphService 设置图谱服务
func (c *ToolContext) WithGraphService(svc GraphQueryService) *ToolContext {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.GraphService = svc
	return c
}

// WithKBRepo 设置知识库仓库
func (c *ToolContext) WithKBRepo(repo knowledge.KnowledgeBaseRepository) *ToolContext {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.KBRepo = repo
	return c
}

// WithDataService 设置数据查询服务
func (c *ToolContext) WithDataService(svc DataQueryService) *ToolContext {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.DataService = svc
	return c
}

// ========================================
// 线程安全的获取方法
// ========================================

// GetRAGService 获取 RAG 服务
func (c *ToolContext) GetRAGService() RAGQueryService {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.RAGService
}

// GetGraphService 获取图谱服务
func (c *ToolContext) GetGraphService() GraphQueryService {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.GraphService
}

// GetKBRepo 获取知识库仓库
func (c *ToolContext) GetKBRepo() knowledge.KnowledgeBaseRepository {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.KBRepo
}

// GetDataService 获取数据查询服务
func (c *ToolContext) GetDataService() DataQueryService {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.DataService
}

// ========================================
// 全局上下文（向后兼容）
// ========================================

var (
	globalContext *ToolContext
	contextOnce   sync.Once
)

// GetGlobalContext 获取全局工具上下文（单例）
func GetGlobalContext() *ToolContext {
	contextOnce.Do(func() {
		globalContext = NewToolContext()
	})
	return globalContext
}

// SetGlobalContext 设置全局上下文（测试用）
func SetGlobalContext(ctx *ToolContext) {
	globalContext = ctx
	contextOnce = sync.Once{}
}
