// Package graph 提供 Graph 应用层 DTO 定义
package knowledge

import (
	"link/internal/model/knowledge"
)

// ========================================
// 图谱提取请求/响应
// ========================================

// ExtractGraphRequest 图谱提取请求
type ExtractGraphRequest struct {
	KnowledgeBaseID     string `json:"kb_id" binding:"required"`
	ChunkID  string `json:"chunk_id" binding:"required"`
	Document string `json:"document" binding:"required"`
	Query    string `json:"query,omitempty"`
	Mode     string `json:"mode,omitempty"` // document/query
}

// ExtractGraphResponse 图谱提取响应
type ExtractGraphResponse struct {
	ChunkID        string             `json:"chunk_id"`
	Nodes          []GraphNodeDTO     `json:"nodes"`
	Relations      []GraphRelationDTO `json:"relations"`
	ProcessingTime int64              `json:"processing_time_ms"`
}

// BatchExtractGraphRequest 批量图谱提取请求
type BatchExtractGraphRequest struct {
	Inputs []ExtractGraphRequest `json:"inputs" binding:"required"`
	Mode   string                `json:"mode,omitempty"` // document/query
}

// BatchExtractGraphResponse 批量图谱提取响应
type BatchExtractGraphResponse struct {
	Nodes          []GraphNodeDTO     `json:"nodes"`
	Relations      []GraphRelationDTO `json:"relations"`
	TotalNodes     int                `json:"total_nodes"`
	TotalRelations int                `json:"total_relations"`
	ProcessingTime int64              `json:"processing_time_ms"`
}

// ========================================
// 图谱查询请求/响应
// ========================================

// GraphQueryRequest 图谱查询请求
type GraphQueryRequest struct {
	KnowledgeBaseID        string   `json:"kb_id" binding:"required"`
	Nodes       []string `json:"nodes"`
	QueryType   string   `json:"query_type,omitempty"` // node/path/subgraph
	MaxDepth    int      `json:"max_depth,omitempty"`
	MinStrength float64  `json:"min_strength,omitempty"`
	Radius      int      `json:"radius,omitempty"` // 用于子图查询
}

// GraphQueryResponse 图谱查询响应
type GraphQueryResponse struct {
	Nodes     []GraphNodeDTO     `json:"nodes"`
	Relations []GraphRelationDTO `json:"relations"`
	Paths     []PathDTO          `json:"paths,omitempty"`
	Stats     *GraphStatsDTO     `json:"stats,omitempty"`
}

// PathDTO 路径 DTO
type PathDTO struct {
	Nodes     []string `json:"nodes"`
	Relations []string `json:"relations"`
	Length    int      `json:"length"`
	Weight    float64  `json:"weight"`
}

// SubGraphQueryResponse 子图查询响应
type SubGraphQueryResponse struct {
	CenterNode string             `json:"center_node"`
	Radius     int                `json:"radius"`
	Nodes      []GraphNodeDTO     `json:"nodes"`
	Relations  []GraphRelationDTO `json:"relations"`
}

// ========================================
// 图谱统计请求/响应
// ========================================

// GraphStatsRequest 图谱统计请求
type GraphStatsRequest struct {
	KnowledgeBaseID string `json:"kb_id" binding:"required"`
}

// GraphStatsDTO 图谱统计 DTO
type GraphStatsDTO struct {
	NodeCount      int     `json:"node_count"`
	RelationCount  int     `json:"relation_count"`
	AvgDegree      float64 `json:"avg_degree"`
	AvgStrength    float64 `json:"avg_strength"`
	AvgWeight      float64 `json:"avg_weight"`
	MaxDegree      int     `json:"max_degree"`
	IsolatedNodes  int     `json:"isolated_nodes"`
	ComponentCount int     `json:"component_count"`
}

// RelationStatsDTO 关系统计 DTO
type RelationStatsDTO struct {
	TotalRelations       int            `json:"total_relations"`
	AverageStrength      float64        `json:"avg_strength"`
	AverageWeight        float64        `json:"avg_weight"`
	AverageChunkCount    float64        `json:"avg_chunk_count"`
	StrengthDistribution map[string]int `json:"strength_distribution"`
	TypeDistribution     map[string]int `json:"type_distribution"`
}

// CentralityMetricsDTO 中心性指标 DTO
type CentralityMetricsDTO struct {
	NodeName         string  `json:"node_name"`
	DegreeCentrality float64 `json:"degree_centrality"`
	Betweenness      float64 `json:"betweenness"`
	Closeness        float64 `json:"closeness"`
	PageRank         float64 `json:"page_rank"`
}

// CommunityDTO 社区 DTO
type CommunityDTO struct {
	ID          string   `json:"id"`
	Nodes       []string `json:"nodes"`
	MemberCount int      `json:"member_count"`
	Modularity  float64  `json:"modularity"`
}

// ========================================
// 图谱节点/关系 DTO
// ========================================

// GraphNodeDTO 图谱节点 DTO
type GraphNodeDTO struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	EntityType string            `json:"entity_type,omitempty"`
	Attributes []string          `json:"attributes,omitempty"`
	Chunks     []string          `json:"chunks,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
	Degree     int               `json:"degree,omitempty"` // 度数
}

// GraphRelationDTO 图谱关系 DTO
type GraphRelationDTO struct {
	ID             string            `json:"id"`
	Source         string            `json:"source"`
	Target         string            `json:"target"`
	Type           string            `json:"type"`
	Strength       float64           `json:"strength,omitempty"`
	Weight         float64           `json:"weight,omitempty"`
	ChunkIDs       []string          `json:"chunk_ids,omitempty"`
	Properties     map[string]string `json:"properties,omitempty"`
	CombinedDegree int               `json:"combined_degree,omitempty"`
	PMI            float64           `json:"pmi,omitempty"`
}

// ========================================
// 图谱操作请求/响应
// ========================================

// AddNodeRequest 添加节点请求
type AddNodeRequest struct {
	KnowledgeBaseID string       `json:"kb_id" binding:"required"`
	Node GraphNodeDTO `json:"node" binding:"required"`
}

// AddNodeResponse 添加节点响应
type AddNodeResponse struct {
	Node      GraphNodeDTO `json:"node"`
	CreatedAt int64        `json:"created_at"` // Unix timestamp
}

// AddRelationRequest 添加关系请求
type AddRelationRequest struct {
	KnowledgeBaseID     string           `json:"kb_id" binding:"required"`
	Relation GraphRelationDTO `json:"relation" binding:"required"`
}

// AddRelationResponse 添加关系响应
type AddRelationResponse struct {
	Relation  GraphRelationDTO `json:"relation"`
	Weight    float64          `json:"weight"` // 计算后的权重
	CreatedAt int64            `json:"created_at"`
}

// UpdateNodeRequest 更新节点请求
type UpdateNodeRequest struct {
	KnowledgeBaseID       string            `json:"kb_id" binding:"required"`
	NodeID     string            `json:"node_id" binding:"required"`
	Properties map[string]string `json:"properties"`
	Attributes []string          `json:"attributes,omitempty"`
}

// UpdateRelationRequest 更新关系请求
type UpdateRelationRequest struct {
	KnowledgeBaseID       string            `json:"kb_id" binding:"required"`
	RelationID string            `json:"relation_id" binding:"required"`
	Strength   *float64          `json:"strength,omitempty"`
	Type       *string           `json:"type,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// ========================================
// 图谱删除请求/响应
// ========================================

// DeleteNodeRequest 删除节点请求
type DeleteNodeRequest struct {
	KnowledgeBaseID   string `json:"kb_id" binding:"required"`
	NodeID string `json:"node_id" binding:"required"`
}

// DeleteRelationRequest 删除关系请求
type DeleteRelationRequest struct {
	KnowledgeBaseID       string `json:"kb_id" binding:"required"`
	RelationID string `json:"relation_id" binding:"required"`
}

// DeleteByChunkRequest 按Chunk删除请求
type DeleteByChunkRequest struct {
	KnowledgeBaseID    string `json:"kb_id" binding:"required"`
	ChunkID string `json:"chunk_id" binding:"required"`
}

// DeleteByKnowledgeRequest 按Knowledge删除请求
type DeleteByKnowledgeRequest struct {
	KnowledgeBaseID        string `json:"kb_id" binding:"required"`
	KnowledgeID string `json:"knowledge_id" binding:"required"`
}

// ========================================
// 图谱分析请求/响应
// ========================================

// FindKeyNodesRequest 发现关键节点请求
type FindKeyNodesRequest struct {
	KnowledgeBaseID string `json:"kb_id" binding:"required"`
	TopK int    `json:"top_k,omitempty"`
}

// FindKeyNodesResponse 发现关键节点响应
type FindKeyNodesResponse struct {
	Nodes []GraphNodeDTO `json:"nodes"`
}

// DetectCommunitiesRequest 社区检测请求
type DetectCommunitiesRequest struct {
	KnowledgeBaseID         string `json:"kb_id" binding:"required"`
	MinCommunity int    `json:"min_community,omitempty"`
}

// DetectCommunitiesResponse 社区检测响应
type DetectCommunitiesResponse struct {
	Communities    []CommunityDTO `json:"communities"`
	CommunityCount int            `json:"community_count"`
	Modularity     float64        `json:"modularity"`
}

// ========================================
// 图谱导出请求/响应
// ========================================

// ExportGraphRequest 导出图谱请求
type ExportGraphRequest struct {
	KnowledgeBaseID   string `json:"kb_id" binding:"required"`
	Format string `json:"format" binding:"required"` // json/gml/graphml/dot
}

// ExportGraphResponse 导出图谱响应
type ExportGraphResponse struct {
	Data      []byte `json:"data"`
	Format    string `json:"format"`
	Size      int    `json:"size"`
	Timestamp int64  `json:"timestamp"`
}

// ========================================
// 图谱导入请求/响应
// ========================================

// ImportGraphRequest 导入图谱请求
type ImportGraphRequest struct {
	KnowledgeBaseID   string `json:"kb_id" binding:"required"`
	Format string `json:"format" binding:"required"` // json/gml/graphml
	Data   []byte `json:"data" binding:"required"`
}

// ImportGraphResponse 导入图谱响应
type ImportGraphResponse struct {
	NodesCount     int   `json:"nodes_count"`
	RelationsCount int   `json:"relations_count"`
	ProcessingTime int64 `json:"processing_time_ms"`
}

// ========================================
// 转换函数
// ========================================

// ToDomainGraphNode 转换为领域节点
func ToDomainGraphNode(dto *GraphNodeDTO) *knowledge.GraphNode {
	if dto == nil {
		return nil
	}
	return &knowledge.GraphNode{
		ID:         dto.ID,
		Name:       dto.Name,
		EntityType: dto.EntityType,
		Attributes: dto.Attributes,
		Chunks:     dto.Chunks,
		Properties: dto.Properties,
	}
}

// FromDomainGraphNode 从领域节点转换
func FromDomainGraphNode(node *knowledge.GraphNode) *GraphNodeDTO {
	if node == nil {
		return nil
	}
	return &GraphNodeDTO{
		ID:         node.ID,
		Name:       node.Name,
		EntityType: node.EntityType,
		Attributes: node.Attributes,
		Chunks:     node.Chunks,
		Properties: node.Properties,
	}
}

// ToDomainGraphRelation 转换为领域关系
func ToDomainGraphRelation(dto *GraphRelationDTO) *knowledge.GraphRelation {
	if dto == nil {
		return nil
	}
	return &knowledge.GraphRelation{
		ID:             dto.ID,
		Source:         dto.Source,
		Target:         dto.Target,
		Type:           dto.Type,
		Strength:       dto.Strength,
		Weight:         dto.Weight,
		ChunkIDs:       dto.ChunkIDs,
		Properties:     dto.Properties,
		CombinedDegree: dto.CombinedDegree,
		PMI:            dto.PMI,
	}
}

// FromDomainGraphRelation 从领域关系转换
func FromDomainGraphRelation(rel *knowledge.GraphRelation) *GraphRelationDTO {
	if rel == nil {
		return nil
	}
	return &GraphRelationDTO{
		ID:             rel.ID,
		Source:         rel.Source,
		Target:         rel.Target,
		Type:           rel.Type,
		Strength:       rel.Strength,
		Weight:         rel.Weight,
		ChunkIDs:       rel.ChunkIDs,
		Properties:     rel.Properties,
		CombinedDegree: rel.CombinedDegree,
		PMI:            rel.PMI,
	}
}

// ToDomainGraphData 转换为领域图谱数据
func ToDomainGraphData(nodes []GraphNodeDTO, relations []GraphRelationDTO) *knowledge.GraphData {
	domainNodes := make([]*knowledge.GraphNode, len(nodes))
	for i, node := range nodes {
		domainNodes[i] = ToDomainGraphNode(&node)
	}

	domainRelations := make([]*knowledge.GraphRelation, len(relations))
	for i, rel := range relations {
		domainRelations[i] = ToDomainGraphRelation(&rel)
	}

	return &knowledge.GraphData{
		Node:     domainNodes,
		Relation: domainRelations,
	}
}

// FromDomainGraphData 从领域图谱数据转换
func FromDomainGraphData(data *knowledge.GraphData) ([]GraphNodeDTO, []GraphRelationDTO) {
	if data == nil {
		return nil, nil
	}

	nodes := make([]GraphNodeDTO, len(data.Node))
	for i, node := range data.Node {
		nodes[i] = *FromDomainGraphNode(node)
	}

	relations := make([]GraphRelationDTO, len(data.Relation))
	for i, rel := range data.Relation {
		relations[i] = *FromDomainGraphRelation(rel)
	}

	return nodes, relations
}

// ToDomainNameSpace 转换为领域命名空间
func ToDomainNameSpace(kbID string, tenantID string) knowledge.NameSpace {
	return knowledge.NameSpace{
		TenantID: tenantID,
	 KnowledgeBaseID:     kbID,
	}
}

// ToDomainExtractionMode 转换为领域提取模式
func ToDomainExtractionMode(mode string) knowledge.ExtractionMode {
	switch mode {
	case "query":
		return knowledge.ExtractionModeQuery
	default:
		return knowledge.ExtractionModeDocument
	}
}

// ToChunkExtractionInput 转换为领域提取输入
func ToChunkExtractionInput(req *ExtractGraphRequest) *knowledge.ChunkExtractionInput {
	return &knowledge.ChunkExtractionInput{
	 KnowledgeBaseID:     req.KnowledgeBaseID,
		ChunkID:  req.ChunkID,
		Document: req.Document,
		Query:    req.Query,
		Mode:     ToDomainExtractionMode(req.Mode),
	}
}
