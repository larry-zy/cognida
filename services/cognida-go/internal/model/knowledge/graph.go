// Package knowledge provides the knowledge management domain graph entity definitions
package knowledge

import (
	"time"
)

// ========================================
// Graph Data Entities
// ========================================

// GraphNode graph node
type GraphNode struct {
	ID         string            `json:"id"`          // Node ID (UUID)
	Name       string            `json:"name"`        // Node name (entity name)
	EntityType string            `json:"entity_type"` // Entity type
	Attributes []string          `json:"attributes"`  // Attribute list
	Chunks     []string          `json:"chunks"`      // Associated chunk ID list
	Properties map[string]string `json:"properties"`  // Property key-value pairs
}

// GraphRelation graph relation
type GraphRelation struct {
	ID             string            `json:"id"`              // Relation ID (UUID)
	Source         string            `json:"source"`          // Source node name
	Target         string            `json:"target"`          // Target node name
	Type           string            `json:"type"`            // Relation type
	Strength       float64           `json:"strength"`        // Relation strength (1-10)
	Weight         float64           `json:"weight"`          // Relation weight (calculated value)
	ChunkIDs       []string          `json:"chunk_ids"`       // Associated chunk ID list
	Properties     map[string]string `json:"properties"`      // Property key-value pairs
	CombinedDegree int               `json:"combined_degree"` // Combined degree
	PMI            float64           `json:"pmi"`             // Pointwise mutual information
	Description    string            `json:"description"`     // Description
}

// GraphData graph data
type GraphData struct {
	Node     []*GraphNode     `json:"nodes"`     // Node list
	Relation []*GraphRelation `json:"relations"` // Relation list
}

// ========================================
// Graph Relation Types
// ========================================

// RelationType relation type
type RelationType string

const (
	RelationTypeContains    RelationType = "CONTAINS"
	RelationTypeRelatedTo   RelationType = "RELATED_TO"
	RelationTypeDependsOn   RelationType = "DEPENDS_ON"
	RelationTypePartOf      RelationType = "PART_OF"
	RelationTypeSimilarTo   RelationType = "SIMILAR_TO"
	RelationTypeCauses      RelationType = "CAUSES"
	RelationTypeLocatedIn   RelationType = "LOCATED_IN"
	RelationTypeBelongsTo   RelationType = "BELONGS_TO"
	RelationTypeConnectedTo RelationType = "CONNECTED_TO"
	RelationTypePrecedes    RelationType = "PRECEDES"
	RelationTypeFollows     RelationType = "FOLLOWS"
)

// IsValidRelationType checks if relation type is valid
func IsValidRelationType(t string) bool {
	switch RelationType(t) {
	case RelationTypeContains, RelationTypeRelatedTo, RelationTypeDependsOn,
		RelationTypePartOf, RelationTypeSimilarTo, RelationTypeCauses, RelationTypeLocatedIn,
		RelationTypeBelongsTo, RelationTypeConnectedTo, RelationTypePrecedes, RelationTypeFollows:
		return true
	default:
		return false
	}
}

// RelationTypeLabel gets Chinese label for relation type
func RelationTypeLabel(t string) string {
	labels := map[string]string{
		"CONTAINS":     "包含",
		"RELATED_TO":   "相关",
		"DEPENDS_ON":   "依赖",
		"PART_OF":      "组成",
		"SIMILAR_TO":   "相似",
		"CAUSES":       "导致",
		"LOCATED_IN":   "位于",
		"BELONGS_TO":   "属于",
		"CONNECTED_TO": "连接",
		"PRECEDES":     "先于",
		"FOLLOWS":      "后于",
	}
	if label, ok := labels[t]; ok {
		return label
	}
	return t
}

// RelationTypeOption relation type option
type RelationTypeOption struct {
	Value string
	Label string
}

// RelationTypeOptions gets relation type options
func RelationTypeOptions() []RelationTypeOption {
	return []RelationTypeOption{
		{Value: "CONTAINS", Label: "包含"},
		{Value: "RELATED_TO", Label: "相关"},
		{Value: "DEPENDS_ON", Label: "依赖"},
		{Value: "PART_OF", Label: "组成"},
		{Value: "SIMILAR_TO", Label: "相似"},
		{Value: "CAUSES", Label: "导致"},
		{Value: "LOCATED_IN", Label: "位于"},
		{Value: "BELONGS_TO", Label: "属于"},
		{Value: "CONNECTED_TO", Label: "连接"},
		{Value: "PRECEDES", Label: "先于"},
		{Value: "FOLLOWS", Label: "后于"},
	}
}

// GetDefaultRelationType gets default relation type based on entity types
func GetDefaultRelationType(sourceType, targetType string) string {
	// Infer default relation type based on entity types
	if sourceType == "Person" && targetType == "Organization" {
		return string(RelationTypeBelongsTo)
	}
	if sourceType == "Organization" && targetType == "Person" {
		return string(RelationTypeContains)
	}
	return string(RelationTypeRelatedTo)
}

// ========================================
// Graph Extraction Configuration
// ========================================

// ExtractionMode extraction mode
type ExtractionMode string

const (
	ExtractionModeDocument ExtractionMode = "document" // Document ingestion mode: full extraction
	ExtractionModeQuery    ExtractionMode = "query"    // Query mode: extract relevant only
)

// ChunkExtractionInput document chunk extraction input
type ChunkExtractionInput struct {
	KnowledgeBaseID     string         // Knowledge base ID
	ChunkID  string         // Document chunk ID
	Document string         // Document content
	Query    string         // Extraction query
	Mode     ExtractionMode // Extraction mode: document/query
}

// ExtractedGraphData extracted graph data
type ExtractedGraphData struct {
	ChunkID   string             // Document chunk ID
	Nodes     []*GraphNode       // Extracted nodes
	Relations []*GraphRelation   // Extracted relations
	Error     error              // Extraction error
}

// ========================================
// Graph Query Results
// ========================================

// PathQueryResult path query result
type PathQueryResult struct {
	Nodes     []*GraphNode     // Nodes on path
	Relations []*GraphRelation // Relations on path
	Length    int              // Path length
	Weight    float64          // Path weight
}

// NodeQueryResult node query result
type NodeQueryResult struct {
	Node      *GraphNode       // Node information
	Neighbors []*GraphNode     // Neighbor nodes
	Relations []*GraphRelation // Connection relations
	Degree    int              // Degree
}

// ========================================
// Graph Statistics
// ========================================

// DetailedGraphStats detailed graph statistics
type DetailedGraphStats struct {
	NodeCount      int     // Node count
	RelationCount  int     // Relation count
	AvgDegree      float64 // Average degree
	AvgStrength    float64 // Average relation strength
	AvgWeight      float64 // Average relation weight
	MaxDegree      int     // Maximum degree
	IsolatedNodes  int     // Isolated node count
	ComponentCount int     // Connected component count
}

// RelationStatistics relation statistics
type RelationStatistics struct {
	TotalRelations       int             // Total relations
	AverageStrength      float64         // Average strength
	AverageWeight        float64         // Average weight
	AverageChunkCount    float64         // Average chunk count per relation
	StrengthDistribution map[int]int     // Strength distribution (how many at each level 1-10)
	TypeDistribution     map[string]int  // Relation type distribution
}

// ========================================
// Graph Query Options
// ========================================

// NodeQueryOptions node query options
type NodeQueryOptions struct {
	EntityTypes []string  // Filter by entity types
	Limit       int       // Max results
	Offset      int       // Pagination offset
	WithChunks  bool      // Include associated chunks
}

// RelationQueryOptions relation query options
type RelationQueryOptions struct {
	Types      []string  // Filter by relation types
	MinWeight  float64   // Minimum weight threshold
	MinStrength float64  // Minimum strength threshold
	Limit      int       // Max results
	Offset     int       // Pagination offset
}

// PathQueryOptions path query options
type PathQueryOptions struct {
	MaxDepth      int      // Maximum path depth
	RelationTypes []string // Allowed relation types (empty = all)
	MaxResults    int      // Maximum number of paths to return
	MinWeight     float64  // Minimum path weight threshold
}

// ========================================
// Graph Statistics
// ========================================

// DegreeStats degree statistics
type DegreeStats struct {
	AverageDegree   float64 // Average degree across all nodes
	MaxDegree       int     // Maximum degree
	MinDegree       int     // Minimum degree
	IsolatedNodes   int     // Count of nodes with degree 0
	HighDegreeNodes int     // Count of nodes with degree > 2*average
}

// TypeDistribution type distribution statistics
type TypeDistribution struct {
	EntityTypeDistribution map[string]int // Entity type distribution
	RelationTypeDistribution map[string]int // Relation type distribution
	TopEntityTypes          []TypeCount    // Top N entity types
	TopRelationTypes        []TypeCount    // Top N relation types
}

// TypeCount type count pair
type TypeCount struct {
	Type  string
	Count int
}

// CentralitySummary centrality score summary
type CentralitySummary struct {
	Min      float64          // Minimum score
	Max      float64          // Maximum score
	Average  float64          // Average score
	TopNodes []CentralityNode // Top N nodes by score
}

// CentralityNode node with centrality score
type CentralityNode struct {
	NodeID    string
	NodeName  string
	Score     float64
	Rank      int
}

// CommunityStats community detection statistics
type CommunityStats struct {
	CommunityCount   int     // Number of communities
	Modularity       float64 // Network modularity score
	AverageCommunitySize float64 // Average community size
	LargestCommunitySize int   // Size of largest community
}

// DensityMetrics graph density metrics
type DensityMetrics struct {
	Density         float64 // Graph density (0-1)
	ComponentCount  int     // Number of connected components
	LargestComponentSize int // Size of largest component
	ClusteringCoefficient float64 // Average clustering coefficient
}

// ========================================
// Community Detection Entities
// ========================================

// Community graph community node
type Community struct {
	ID          string    `json:"id"`           // Community ID (UUID)
	KnowledgeBaseID        string    `json:"knowledge_base_id"`        // Knowledge base ID
	Name        string    `json:"name"`         // Community name
	Size        int       `json:"size"`         // Number of members
	Modularity  float64   `json:"modularity"`   // Community modularity contribution
	Labels      []string  `json:"labels"`       // Community labels
	CreatedAt   time.Time `json:"created_at"`   // Creation time
	Properties  map[string]string `json:"properties"` // Additional properties
}

// CommunityMember community membership relationship
type CommunityMember struct {
	CommunityID string  `json:"community_id"` // Community ID
	EntityID    string  `json:"entity_id"`    // Entity node ID
	EntityName  string  `json:"entity_name"`  // Entity node name
	MembershipScore float64 `json:"membership_score"` // Membership strength
}

// ========================================
// Extraction Context
// ========================================

// ExtractionContext context for incremental graph extraction
type ExtractionContext struct {
	KnowledgeBaseID                string            // Knowledge base ID
	ExistingEntities    []string          // Existing entity names
	EntityTypes         map[string]int    // Entity type distribution
	RelationTypes       map[string]int    // Relation type distribution
	SampleEntities      []EntitySample    // Sample entities for context
	HighFrequencyPairs  []EntityPair      // High-frequency entity pairs
	Metadata            map[string]string // Additional metadata
}

// EntitySample entity sample for context
type EntitySample struct {
	Name       string
	EntityType string
	Attributes []string
}

// EntityPair entity pair for context
type EntityPair struct {
	Source     string
	Target     string
	Relation   string
	Frequency  int
}

// ========================================
// Graph Context
// ========================================

// GraphContext graph operation context
type GraphContext struct {
	NameSpace     NameSpace          // Namespace
	ExtractionMode ExtractionMode    // Extraction mode
	Query         string             // Query content
	Timestamp     time.Time          // Operation time
	Metadata      map[string]interface{} // Metadata
}

// ========================================
// Helper Methods for GraphNode
// ========================================

// GetChunkIDs gets all associated ChunkIDs
func (n *GraphNode) GetChunkIDs() []string {
	return n.Chunks
}

// AddChunk adds associated Chunk
func (n *GraphNode) AddChunk(chunkID string) {
	for _, id := range n.Chunks {
		if id == chunkID {
			return
		}
	}
	n.Chunks = append(n.Chunks, chunkID)
}

// GetProperty gets property value
func (n *GraphNode) GetProperty(key string) string {
	if n.Properties == nil {
		return ""
	}
	return n.Properties[key]
}

// SetProperty sets property value
func (n *GraphNode) SetProperty(key, value string) {
	if n.Properties == nil {
		n.Properties = make(map[string]string)
	}
	n.Properties[key] = value
}

// ========================================
// Helper Methods for GraphRelation
// ========================================

// GetKey gets relation key (for deduplication)
func (r *GraphRelation) GetKey() string {
	return r.Source + "#" + r.Target
}

// AddChunkID adds associated ChunkID
func (r *GraphRelation) AddChunkID(chunkID string) {
	for _, id := range r.ChunkIDs {
		if id == chunkID {
			return
		}
	}
	r.ChunkIDs = append(r.ChunkIDs, chunkID)
}

// GetProperty gets property value
func (r *GraphRelation) GetProperty(key string) string {
	if r.Properties == nil {
		return ""
	}
	return r.Properties[key]
}

// SetProperty sets property value
func (r *GraphRelation) SetProperty(key, value string) {
	if r.Properties == nil {
		r.Properties = make(map[string]string)
	}
	r.Properties[key] = value
}

// ========================================
// Graph Data Operations
// ========================================

// Merge merges multiple graph data
func (g *GraphData) Merge(other *GraphData) *GraphData {
	result := &GraphData{
		Node:     make([]*GraphNode, 0),
		Relation: make([]*GraphRelation, 0),
	}

	// Add current nodes and relations
	nodeMap := make(map[string]*GraphNode)
	relMap := make(map[string]*GraphRelation)

	for _, node := range g.Node {
		nodeMap[node.Name] = node
		result.Node = append(result.Node, node)
	}
	for _, rel := range g.Relation {
		key := rel.GetKey()
		relMap[key] = rel
		result.Relation = append(result.Relation, rel)
	}

	// Merge with other
	for _, node := range other.Node {
		if existing, ok := nodeMap[node.Name]; ok {
			// Merge chunks
			for _, chunkID := range node.Chunks {
				existing.AddChunk(chunkID)
			}
			// Merge attributes
			existing.Attributes = append(existing.Attributes, node.Attributes...)
		} else {
			nodeMap[node.Name] = node
			result.Node = append(result.Node, node)
		}
	}

	for _, rel := range other.Relation {
		key := rel.GetKey()
		if existing, ok := relMap[key]; ok {
			// Merge chunk_ids
			for _, chunkID := range rel.ChunkIDs {
				existing.AddChunkID(chunkID)
			}
			// Update strength (weighted average)
			if existing.Strength > 0 && rel.Strength > 0 {
				existing.Strength = (existing.Strength + rel.Strength) / 2
			} else if rel.Strength > 0 {
				existing.Strength = rel.Strength
			}
		} else {
			relMap[key] = rel
			result.Relation = append(result.Relation, rel)
		}
	}

	return result
}

// Clone clones graph data
func (g *GraphData) Clone() *GraphData {
	nodes := make([]*GraphNode, len(g.Node))
	for i, node := range g.Node {
		nodes[i] = &GraphNode{
			ID:         node.ID,
			Name:       node.Name,
			EntityType: node.EntityType,
			Attributes: append([]string{}, node.Attributes...),
			Chunks:     append([]string{}, node.Chunks...),
			Properties: copyMap(node.Properties),
		}
	}

	relations := make([]*GraphRelation, len(g.Relation))
	for i, rel := range g.Relation {
		relations[i] = &GraphRelation{
			ID:             rel.ID,
			Source:         rel.Source,
			Target:         rel.Target,
			Type:           rel.Type,
			Strength:       rel.Strength,
			Weight:         rel.Weight,
			ChunkIDs:       append([]string{}, rel.ChunkIDs...),
			Properties:     copyMap(rel.Properties),
			CombinedDegree: rel.CombinedDegree,
			PMI:            rel.PMI,
			Description:    rel.Description,
		}
	}

	return &GraphData{
		Node:     nodes,
		Relation: relations,
	}
}

// Validate validates graph data
func (g *GraphData) Validate() error {
	// 图谱必须至少包含一个节点：空图（nil 或空切片）视为非法，
	// 避免将无意义的空数据写入图存储。
	if len(g.Node) == 0 {
		return &GraphValidationError{Field: "node", Message: "图谱节点不能为空"}
	}

	// Validate nodes
	nodeNames := make(map[string]bool)
	for _, node := range g.Node {
		if node.Name == "" {
			return &GraphValidationError{Field: "node.name", Message: "节点名称不能为空"}
		}
		if node.ID == "" {
			return &GraphValidationError{Field: "node.id", Message: "节点ID不能为空"}
		}
		nodeNames[node.Name] = true
	}

	// Validate relations
	for _, rel := range g.Relation {
		if rel.Source == "" {
			return &GraphValidationError{Field: "relation.source", Message: "关系源节点不能为空"}
		}
		if rel.Target == "" {
			return &GraphValidationError{Field: "relation.target", Message: "关系目标节点不能为空"}
		}
		if rel.ID == "" {
			return &GraphValidationError{Field: "relation.id", Message: "关系ID不能为空"}
		}
		// Check if nodes exist (optional)
		if !nodeNames[rel.Source] {
			return &GraphValidationError{Field: "relation.source", Message: "源节点不存在: " + rel.Source}
		}
		if !nodeNames[rel.Target] {
			return &GraphValidationError{Field: "relation.target", Message: "目标节点不存在: " + rel.Target}
		}
	}

	return nil
}

// GraphValidationError graph validation error
type GraphValidationError struct {
	Field   string
	Message string
}

func (e *GraphValidationError) Error() string {
	return e.Field + ": " + e.Message
}

func copyMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
