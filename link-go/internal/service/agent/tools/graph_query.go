// Package tools 提供图谱查询工具
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	agentctx "link/internal/model/agent"
)

// ========================================
// 图谱查询工具
// ========================================

// 全局图谱服务实例
var graphService GraphQueryService

// InitGraphQueryTool 初始化图谱查询工具
func InitGraphQueryTool(service GraphQueryService) {
	graphService = service
}

// SetGraphService 设置图谱服务（用于测试）
func SetGraphService(service GraphQueryService) {
	graphService = service
}

// GraphQueryRequest 图谱查询请求
type GraphQueryRequest struct {
	// Query 查询内容
	Query string `json:"query" jsonschema:"required,description=用户的问题或查询内容"`

	// TopK 返回结果数量，默认5
	TopK int `json:"top_k" jsonschema:"description=返回结果数量，默认5，范围1-20"`

	// Depth 查询深度（跳数），默认2
	Depth int `json:"depth" jsonschema:"description=查询深度（跳数），默认2，范围1-3"`

	// IncludePath 是否包含关系路径
	IncludePath bool `json:"include_path" jsonschema:"description=是否返回完整的关系路径，默认false"`
}

// GraphQueryResult 图谱查询结果
type GraphQueryResult struct {
	// Answer 基于图谱信息生成的答案
	Answer string `json:"answer"`

	// Entities 查询到的实体
	Entities []GraphEntity `json:"entities"`

	// Relations 查询到的关系
	Relations []GraphRelation `json:"relations"`

	// Paths 实体间的关系路径（如果启用）
	Paths []GraphPath `json:"paths,omitempty"`

	// Count 结果数量
	Count int `json:"count"`

	// Query 原始查询
	Query string `json:"query"`

	// KnowledgeBaseID 使用的知识库ID
	KnowledgeBaseID string `json:"kb_id"`

	// Latency 查询耗时（毫秒）
	Latency int64 `json:"latency_ms"`

	// HasAnswer 是否有答案
	HasAnswer bool `json:"has_answer"`
}

// GraphEntity 图谱实体
type GraphEntity struct {
	// ID 实体ID
	ID string `json:"id"`

	// Name 实体名称
	Name string `json:"name"`

	// Type 实体类型
	Type string `json:"type"`

	// Description 实体描述
	Description string `json:"description,omitempty"`

	// Properties 实体属性
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// GraphRelation 图谱关系
type GraphRelation struct {
	// Source 源实体
	Source string `json:"source"`

	// Target 目标实体
	Target string `json:"target"`

	// Type 关系类型
	Type string `json:"type"`

	// Description 关系描述
	Description string `json:"description,omitempty"`

	// Strength 关系强度（0-1）
	Strength float64 `json:"strength"`
}

// GraphPath 关系路径
type GraphPath struct {
	// Source 起始实体
	Source string `json:"source"`

	// Target 目标实体
	Target string `json:"target"`

	// Hops 跳数
	Hops int `json:"hops"`

	// Path 路径上的实体和关系
	Path []PathStep `json:"path"`
}

// PathStep 路径步骤
type PathStep struct {
	// Entity 实体名称
	Entity string `json:"entity"`

	// Relation 关系类型（如果有）
	Relation string `json:"relation,omitempty"`

	// Direction 方向：forward/backward
	Direction string `json:"direction"`
}

// NewGraphQueryTool 创建图谱查询工具
func NewGraphQueryTool() *TypedBaseTool[GraphQueryRequest, GraphQueryResult] {
	return NewTypedBaseTool("graph_query",
		`使用知识图谱进行关系查询，查找实体之间的关联关系。

这是专门处理关系型问题的工具，能够：
1. 查找实体之间的直接关系
2. 发现多跳关联路径
3. 分析实体的影响力范围
4. 返回关系强度和路径信息

适用场景：
- 关系查询："张三负责哪些项目？"
- 关联分析："李四和王五有什么共同参与的项目？"
- 影响分析："服务A的下游有哪些服务？"
- 组织架构："技术部有多少人？汇报关系是怎样的？"
- 依赖查询："这个功能依赖哪些模块？"
- 溯源分析："这个需求是哪个客户提出的？"

不适用的场景：
- 概念解释："什么是微服务？"（请使用 rag_query）
- 操作指南："如何配置Nginx？"（请使用 rag_query）
- 文档摘要："总结文档内容"（请使用 rag_query）

前置条件：仅当用户在本次会话开启了「图谱增强」时可用；未开启时本工具会返回提示而不检索。
检索范围（哪些知识库）由用户在会话入口选定，或在结合/智能模式下由你经 kb_route 聚焦；
系统始终在允许范围内强制，无需也无法在本工具参数中指定 kb_id。

参数说明：
- query: 查询内容（必需）
- top_k: 返回结果数量（可选，默认5）
- depth: 查询深度/跳数（可选，默认2，最大3）
- include_path: 是否返回完整路径（可选，默认false）`,
		graphQuery,
	)
}

// graphQuery 执行图谱查询
func graphQuery(ctx context.Context, req *GraphQueryRequest) (*GraphQueryResult, error) {
	startTime := time.Now()

	// 1. 参数验证
	if req.Query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}

	// 门控：图谱增强未开启时，直接返回提示而不检索（非错误）。
	if !agentctx.IsGraphEnabled(ctx) {
		return &GraphQueryResult{
			Query:     req.Query,
			Entities:  []GraphEntity{},
			Relations: []GraphRelation{},
			Count:     0,
			HasAnswer: false,
			Answer:    "图谱增强未开启，本次未执行图谱检索。如需关系/关联类分析，请在会话中开启「图谱增强」后重试，或改用 rag_query 查询文档内容。",
			Latency:   time.Since(startTime).Milliseconds(),
		}, nil
	}

	// 设置默认值
	if req.TopK <= 0 {
		req.TopK = 5
	}
	if req.TopK > 20 {
		req.TopK = 20
	}
	if req.Depth <= 0 {
		req.Depth = 2
	}
	if req.Depth > 3 {
		req.Depth = 3
	}

	// 2. 检查图谱服务是否已初始化
	if graphService == nil {
		return nil, fmt.Errorf("graph service not initialized. Please configure graph service before using graph_query tool")
	}

	// 3. 调用真实的图谱服务
	result, err := graphService.GraphQuery(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("graph query failed: %w", err)
	}

	// 4. 更新耗时
	result.Latency = time.Since(startTime).Milliseconds()
	result.Query = req.Query

	return result, nil
}

// ========================================
// 工具工厂
// ========================================

// GraphToolFactory 图谱工具工厂
type GraphToolFactory struct {
	service GraphQueryService
}

// NewGraphToolFactory 创建图谱工具工厂
func NewGraphToolFactory(service GraphQueryService) *GraphToolFactory {
	return &GraphToolFactory{
		service: service,
	}
}

// CreateTool 创建工具
func (f *GraphToolFactory) CreateTool() *TypedBaseTool[GraphQueryRequest, GraphQueryResult] {
	InitGraphQueryTool(f.service)
	return NewGraphQueryTool()
}

// ========================================
// JSON 转换辅助函数
// ========================================

// GraphQueryRequestFromJSON 从 JSON 解析请求
func GraphQueryRequestFromJSON(jsonStr string) (*GraphQueryRequest, error) {
	var req GraphQueryRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}
	return &req, nil
}

// GraphQueryResultToJSON 将结果转为 JSON
func GraphQueryResultToJSON(result *GraphQueryResult) (string, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}
	return string(data), nil
}
