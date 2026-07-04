// Package handler 提供 Graph 相关的 HTTP 处理器
package handler

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	domainGraph "link/internal/model/knowledge"
	knowledgeapp "link/internal/service/knowledge"
)

// ========================================
// Graph Handler
// ========================================

// GraphHandler 图谱处理器
type GraphHandler struct {
	graphService *knowledgeapp.GraphService
}

// NewGraphHandler 创建图谱处理器
func NewGraphHandler(graphService *knowledgeapp.GraphService) *GraphHandler {
	return &GraphHandler{
		graphService: graphService,
	}
}

// CheckHealth 检查 Neo4j 连接健康状态
func (h *GraphHandler) CheckHealth(c *gin.Context) {
	err := h.graphService.CheckHealth(c.Request.Context())
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, gin.H{"message": "Neo4j 连接正常"})
}

// GetStats 获取图谱统计信息
func (h *GraphHandler) GetStats(c *gin.Context) {
	knowledgeBaseID := c.Query("kb_id")
	if knowledgeBaseID == "" {
		BadRequest(c, "知识库ID不能为空")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		tenantID = "1"
	}

	namespace := domainGraph.NameSpace{
		TenantID:        tenantID,
		KnowledgeBaseID: knowledgeBaseID,
	}

	stats, err := h.graphService.GetGraphStats(c.Request.Context(), namespace)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, stats)
}

// GetRelationStats 获取关系统计
func (h *GraphHandler) GetRelationStats(c *gin.Context) {
	knowledgeBaseID := c.Query("kb_id")
	if knowledgeBaseID == "" {
		BadRequest(c, "知识库ID不能为空")
		return
	}

	// NameSpace 参数由 knowledgeBaseID 和 tenantID 组成，需要传递给图谱服务
	log.Printf("[GetRelationStats] KB ID: %s", knowledgeBaseID)
	OK(c, gin.H{"kb_id": knowledgeBaseID, "note": "关系统计功能待实现"})
}

// AddNode 添加节点
func (h *GraphHandler) AddNode(c *gin.Context) {
	var req struct {
		Name       string   `json:"name" binding:"required"`
		EntityType string   `json:"entity_type" binding:"required"`
		Attributes []string `json:"attributes"`
	}

	if !BindJSON(c, &req) {
		return
	}

	// 获取租户ID和 KnowledgeBase ID 作为命名空间
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		tenantID = "1"
	}
	knowledgeBaseID := c.Param("id")
	if knowledgeBaseID == "" {
		BadRequest(c, "知识库ID不能为空")
		return
	}

	// 生成节点 ID
	nodeID := uuid.New().String()

	node := &domainGraph.GraphNode{
		ID:         nodeID,
		Name:       req.Name,
		EntityType: req.EntityType,
		Attributes: req.Attributes,
	}

	namespace := domainGraph.NameSpace{
		TenantID:        tenantID,
		KnowledgeBaseID: knowledgeBaseID,
	}

	err := h.graphService.AddNode(c.Request.Context(), namespace, node)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, gin.H{"message": "添加成功"})
}

// AddRelation 添加关系
func (h *GraphHandler) AddRelation(c *gin.Context) {
	var req struct {
		Source   string   `json:"source" binding:"required"`
		Target   string   `json:"target" binding:"required"`
		Type     string   `json:"type" binding:"required"`
		Weight   float64  `json:"weight"`
		Strength float64  `json:"strength"`
		ChunkIDs []string `json:"chunk_ids"`
	}

	if !BindJSON(c, &req) {
		return
	}

	// 获取租户ID和 KnowledgeBase ID 作为命名空间
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		tenantID = "1"
	}
	knowledgeBaseID := c.Param("id")
	if knowledgeBaseID == "" {
		BadRequest(c, "知识库ID不能为空")
		return
	}

	// 生成关系 ID
	relationID := uuid.New().String()

	relation := &domainGraph.GraphRelation{
		ID:       relationID,
		Source:   req.Source,
		Target:   req.Target,
		Type:     req.Type,
		Weight:   req.Weight,
		Strength: req.Strength,
		ChunkIDs: req.ChunkIDs,
	}

	namespace := domainGraph.NameSpace{
		TenantID:        tenantID,
		KnowledgeBaseID: knowledgeBaseID,
	}

	result, err := h.graphService.AddRelation(c.Request.Context(), namespace, relation)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, result)
}

// ========================================
// KnowledgeBase Graph Routes (缺失的接口)
// ========================================

// GetGraph 获取知识库图谱
// GET /api/v1/knowledge-bases/:id/graph
func (h *GraphHandler) GetGraph(c *gin.Context) {
	kbID := c.Param("id")

	// 获取租户ID
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		tenantID = "1"
	}

	// 构建命名空间
	namespace := domainGraph.NameSpace{
		TenantID:        tenantID,
		KnowledgeBaseID: kbID,
	}

	// 获取图谱数据
	graphData, err := h.graphService.GetGraph(c.Request.Context(), namespace)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, graphData)
}

// SearchNode 搜索节点
// POST /api/v1/knowledge-bases/:id/graph/search
func (h *GraphHandler) SearchNode(c *gin.Context) {
	kbID := c.Param("id")

	// 获取租户ID
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		tenantID = "1"
	}

	// 解析请求体
	var req struct {
		Nodes []string `json:"nodes" binding:"required"`
	}
	if !BindJSON(c, &req) {
		return
	}

	if len(req.Nodes) == 0 {
		BadRequest(c, "nodes 不能为空")
		return
	}

	// 构建命名空间
	namespace := domainGraph.NameSpace{
		TenantID:        tenantID,
		KnowledgeBaseID: kbID,
	}

	// 搜索节点
	graphData, err := h.graphService.SearchNode(c.Request.Context(), namespace, req.Nodes)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, graphData)
}

// GetNodeDetail 获取节点详情
// GET /api/v1/knowledge-bases/:id/graph/nodes/:nodeId
func (h *GraphHandler) GetNodeDetail(c *gin.Context) {
	kbID := c.Param("id")
	nodeTitle := c.Param("nodeId")

	// 获取租户ID
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		tenantID = "1"
	}

	// 构建命名空间
	namespace := domainGraph.NameSpace{
		TenantID:        tenantID,
		KnowledgeBaseID: kbID,
	}

	// 轻量查询：以节点名为中心取 1-hop 双向邻居及关系（不加载全图）
	result, err := h.graphService.GetNodeNeighbors(c.Request.Context(), namespace, nodeTitle)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	if result == nil || result.Node == nil {
		NotFound(c, "节点未找到")
		return
	}

	OK(c, gin.H{
		"node":      result.Node,
		"neighbors": result.Neighbors,
		"relations": result.Relations,
		"degree":    result.Degree,
	})
}

// UpdateNode 更新节点属性
// PUT /api/v1/knowledge-bases/:id/graph/nodes/:nodeId
func (h *GraphHandler) UpdateNode(c *gin.Context) {
	kbID := c.Param("id")
	nodeID := c.Param("nodeId")

	// 获取租户ID
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		tenantID = "1"
	}

	// 解析请求体
	var req struct {
		Name       string   `json:"name" binding:"required"`
		EntityType string   `json:"entity_type"`
		Attributes []string `json:"attributes"`
	}
	if !BindJSON(c, &req) {
		return
	}

	// 构建节点对象
	node := &domainGraph.GraphNode{
		ID:         nodeID,
		Name:       req.Name,
		EntityType: req.EntityType,
		Attributes: req.Attributes,
	}

	// 构建命名空间
	namespace := domainGraph.NameSpace{
		TenantID:        tenantID,
		KnowledgeBaseID: kbID,
	}

	// 更新节点
	if err := h.graphService.UpdateNode(c.Request.Context(), namespace, node); err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, node)
}

// UpdateRelation 更新关系属性
// PUT /api/v1/knowledge-bases/:id/graph/relations/:relationId
func (h *GraphHandler) UpdateRelation(c *gin.Context) {
	kbID := c.Param("id")
	relationID := c.Param("relationId")

	// 获取租户ID
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		tenantID = "1"
	}

	// 解析请求体
	var req struct {
		Type        string  `json:"type" binding:"required"`
		Description string  `json:"description"`
		Strength    float64 `json:"strength"`
	}
	if !BindJSON(c, &req) {
		return
	}

	// 构建关系对象
	relation := &domainGraph.GraphRelation{
		ID:          relationID,
		Type:        req.Type,
		Description: req.Description,
		Strength:    req.Strength,
		Weight:      req.Strength,
	}

	// 构建命名空间
	namespace := domainGraph.NameSpace{
		TenantID:        tenantID,
		KnowledgeBaseID: kbID,
	}

	// 更新关系
	updatedRelation, err := h.graphService.UpdateRelation(c.Request.Context(), namespace, relation)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	if updatedRelation == nil {
		NotFound(c, "关系未找到")
		return
	}

	OK(c, updatedRelation)
}

// DeleteNode 删除单个节点
// DELETE /api/v1/knowledge-bases/:id/graph/nodes/:nodeId
func (h *GraphHandler) DeleteNode(c *gin.Context) {
	kbID := c.Param("id")
	nodeID := c.Param("nodeId")

	// 获取租户ID
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		tenantID = "1"
	}

	// 构建命名空间
	namespace := domainGraph.NameSpace{
		TenantID:        tenantID,
		KnowledgeBaseID: kbID,
	}

	// 删除节点
	if err := h.graphService.DeleteNode(c.Request.Context(), namespace, nodeID); err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, gin.H{"message": "删除成功"})
}

// DeleteRelation 删除单个关系
// DELETE /api/v1/knowledge-bases/:id/graph/relations/:relationId
func (h *GraphHandler) DeleteRelation(c *gin.Context) {
	kbID := c.Param("id")
	relationID := c.Param("relationId")

	// 获取租户ID
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		tenantID = "1"
	}

	// 构建命名空间
	namespace := domainGraph.NameSpace{
		TenantID:        tenantID,
		KnowledgeBaseID: kbID,
	}

	// 删除关系
	if err := h.graphService.DeleteRelation(c.Request.Context(), namespace, relationID); err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, gin.H{"message": "删除成功"})
}

// DeleteGraph 删除图谱
// DELETE /api/v1/knowledge-bases/:id/graph
func (h *GraphHandler) DeleteGraph(c *gin.Context) {
	kbID := c.Param("id")

	// 获取租户ID
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		tenantID = "1"
	}

	// 构建命名空间
	namespace := domainGraph.NameSpace{
		TenantID:        tenantID,
		KnowledgeBaseID: kbID,
	}

	// 删除图谱
	if err := h.graphService.DeleteGraph(c.Request.Context(), []domainGraph.NameSpace{namespace}); err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, gin.H{"message": "删除成功"})
}

// GetRelationTypes 获取关系类型选项
// GET /api/v1/knowledge-bases/:id/graph/relation-types
func (h *GraphHandler) GetRelationTypes(c *gin.Context) {
	// 返回预设的关系类型选项
	relationTypes := []gin.H{
		{"value": "contains", "label": "包含"},
		{"value": "relates", "label": "关联"},
		{"value": "depends", "label": "依赖"},
		{"value": "belongs", "label": "属于"},
		{"value": "owns", "label": "拥有"},
		{"value": "author", "label": "作者"},
		{"value": "alias", "label": "别名"},
		{"value": "other", "label": "其他"},
	}

	OK(c, relationTypes)
}
