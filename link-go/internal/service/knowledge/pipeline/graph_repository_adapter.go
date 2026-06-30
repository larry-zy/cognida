// Package rag provides RAG infrastructure layer implementations
package rag

import (
	"context"

	domainrag "link/internal/model/rag"
	domain_knowledge "link/internal/model/knowledge"
)

// GraphRepositoryAdapter adapts the new knowledge GraphRepository to the old rag GraphRepository interface
// This maintains backward compatibility while using the unified graph repository
type GraphRepositoryAdapter struct {
	repo domain_knowledge.GraphRepository
}

// NewGraphRepositoryAdapter creates a new adapter
func NewGraphRepositoryAdapter(repo domain_knowledge.GraphRepository) domainrag.GraphRepository {
	return &GraphRepositoryAdapter{repo: repo}
}

// AddGraph adds graph data
func (a *GraphRepositoryAdapter) AddGraph(ctx context.Context, namespace domainrag.NameSpace, graphs []*domainrag.GraphData) error {
	// Convert rag types to knowledge types
	knowledgeGraphs := make([]*domain_knowledge.GraphData, len(graphs))
	for i, g := range graphs {
		knowledgeGraphs[i] = a.convertGraphDataToKnowledge(g)
	}

	knowledgeNamespace := a.convertNameSpaceToKnowledge(namespace)
	return a.repo.AddGraph(ctx, knowledgeNamespace, knowledgeGraphs)
}

// DeleteGraph deletes graph data
func (a *GraphRepositoryAdapter) DeleteGraph(ctx context.Context, namespaces []domainrag.NameSpace) error {
	knowledgeNamespaces := make([]domain_knowledge.NameSpace, len(namespaces))
	for i, ns := range namespaces {
		knowledgeNamespaces[i] = a.convertNameSpaceToKnowledge(ns)
	}
	return a.repo.DeleteGraph(ctx, knowledgeNamespaces)
}

// GetGraph gets complete graph data
func (a *GraphRepositoryAdapter) GetGraph(ctx context.Context, namespace domainrag.NameSpace) (*domainrag.GraphData, error) {
	knowledgeNamespace := a.convertNameSpaceToKnowledge(namespace)
	knowledgeGraph, err := a.repo.GetGraph(ctx, knowledgeNamespace)
	if err != nil {
		return nil, err
	}
	return a.convertGraphDataToRAG(knowledgeGraph), nil
}

// SearchNode searches nodes by name list
// Implements the old API using the new SearchNodes method
func (a *GraphRepositoryAdapter) SearchNode(ctx context.Context, namespace domainrag.NameSpace, nodes []string) (*domainrag.GraphData, error) {
	knowledgeNamespace := a.convertNameSpaceToKnowledge(namespace)

	// Use SearchNodes to find each node
	var allNodes []*domain_knowledge.GraphNode
	for _, nodeName := range nodes {
		foundNodes, err := a.repo.SearchNodes(ctx, knowledgeNamespace, nodeName, &domain_knowledge.NodeQueryOptions{
			Limit: 100,
		})
		if err != nil {
			continue // Ignore errors, continue finding other nodes
		}
		allNodes = append(allNodes, foundNodes...)
	}

	// Get full graph to include relations
	fullGraph, err := a.repo.GetGraph(ctx, knowledgeNamespace)
	if err != nil {
		return nil, err
	}

	// Create node map for filtering
	nodeMap := make(map[string]*domain_knowledge.GraphNode)
	for _, node := range allNodes {
		nodeMap[node.Name] = node
	}

	// Filter relations
	var filteredRelations []*domain_knowledge.GraphRelation
	for _, rel := range fullGraph.Relation {
		if _, ok := nodeMap[rel.Source]; ok {
			filteredRelations = append(filteredRelations, rel)
		}
	}

	// Convert map to slice
	var resultNodes []*domain_knowledge.GraphNode
	for _, node := range nodeMap {
		resultNodes = append(resultNodes, node)
	}

	resultGraph := &domain_knowledge.GraphData{
		Node:     resultNodes,
		Relation: filteredRelations,
	}

	return a.convertGraphDataToRAG(resultGraph), nil
}

// SearchPath searches for a path between two nodes
// Implements the old API using the new FindShortestPath method
func (a *GraphRepositoryAdapter) SearchPath(ctx context.Context, namespace domainrag.NameSpace, startNode, endNode string, maxDepth int) ([]*domainrag.GraphData, error) {
	knowledgeNamespace := a.convertNameSpaceToKnowledge(namespace)

	result, err := a.repo.FindShortestPath(ctx, knowledgeNamespace, startNode, endNode, &domain_knowledge.PathQueryOptions{
		MaxDepth: maxDepth,
	})
	if err != nil {
		return nil, err
	}

	if result == nil || len(result.Nodes) == 0 {
		return []*domainrag.GraphData{}, nil
	}

	// Convert PathQueryResult to GraphData
	// PathQueryResult has Nodes and Relations fields
	graphData := &domain_knowledge.GraphData{
		Node:     result.Nodes,
		Relation: result.Relations,
	}

	return []*domainrag.GraphData{a.convertGraphDataToRAG(graphData)}, nil
}

// CheckHealth checks graph service health
func (a *GraphRepositoryAdapter) CheckHealth(ctx context.Context) error {
	return a.repo.CheckHealth(ctx)
}

// UpdateNode updates node attributes
func (a *GraphRepositoryAdapter) UpdateNode(ctx context.Context, namespace domainrag.NameSpace, node *domainrag.GraphNode) error {
	knowledgeNamespace := a.convertNameSpaceToKnowledge(namespace)
	knowledgeNode := a.convertNodeToKnowledge(node)
	return a.repo.UpdateNode(ctx, knowledgeNamespace, knowledgeNode)
}

// AddRelation adds a single relation
func (a *GraphRepositoryAdapter) AddRelation(ctx context.Context, namespace domainrag.NameSpace, relation *domainrag.GraphRelation) (*domainrag.GraphRelation, error) {
	knowledgeNamespace := a.convertNameSpaceToKnowledge(namespace)
	knowledgeRelation := a.convertRelationToKnowledge(relation)

	result, err := a.repo.AddRelation(ctx, knowledgeNamespace, knowledgeRelation)
	if err != nil {
		return nil, err
	}

	return a.convertRelationToRAG(result), nil
}

// AddNode adds a single node
func (a *GraphRepositoryAdapter) AddNode(ctx context.Context, namespace domainrag.NameSpace, node *domainrag.GraphNode) error {
	knowledgeNamespace := a.convertNameSpaceToKnowledge(namespace)
	knowledgeNode := a.convertNodeToKnowledge(node)
	return a.repo.AddNode(ctx, knowledgeNamespace, knowledgeNode)
}

// UpdateRelation updates relation attributes
func (a *GraphRepositoryAdapter) UpdateRelation(ctx context.Context, namespace domainrag.NameSpace, relation *domainrag.GraphRelation) (*domainrag.GraphRelation, error) {
	knowledgeNamespace := a.convertNameSpaceToKnowledge(namespace)
	knowledgeRelation := a.convertRelationToKnowledge(relation)

	result, err := a.repo.UpdateRelation(ctx, knowledgeNamespace, knowledgeRelation)
	if err != nil {
		return nil, err
	}

	return a.convertRelationToRAG(result), nil
}

// DeleteNode deletes a single node
func (a *GraphRepositoryAdapter) DeleteNode(ctx context.Context, namespace domainrag.NameSpace, nodeID string) error {
	knowledgeNamespace := a.convertNameSpaceToKnowledge(namespace)
	return a.repo.DeleteNode(ctx, knowledgeNamespace, nodeID)
}

// DeleteRelation deletes a single relation
func (a *GraphRepositoryAdapter) DeleteRelation(ctx context.Context, namespace domainrag.NameSpace, relationID string) error {
	knowledgeNamespace := a.convertNameSpaceToKnowledge(namespace)
	return a.repo.DeleteRelation(ctx, knowledgeNamespace, relationID)
}

// DeleteByChunkID deletes data by chunk ID
func (a *GraphRepositoryAdapter) DeleteByChunkID(ctx context.Context, namespace domainrag.NameSpace, chunkID string) error {
	knowledgeNamespace := a.convertNameSpaceToKnowledge(namespace)
	return a.repo.DeleteByChunkID(ctx, knowledgeNamespace, chunkID)
}

// DeleteByKnowledgeID deletes data by knowledge ID
func (a *GraphRepositoryAdapter) DeleteByKnowledgeID(ctx context.Context, namespace domainrag.NameSpace, knowledgeID string) error {
	knowledgeNamespace := a.convertNameSpaceToKnowledge(namespace)
	return a.repo.DeleteByKnowledgeID(ctx, knowledgeNamespace, knowledgeID)
}

// ========================================
// Conversion helpers
// ========================================

func (a *GraphRepositoryAdapter) convertNameSpaceToKnowledge(ns domainrag.NameSpace) domain_knowledge.NameSpace {
	return domain_knowledge.NameSpace{
		TenantID:  ns.TenantID,
	 KnowledgeBaseID:      ns.KnowledgeBaseID,
		Knowledge: "", // rag.NameSpace doesn't have Knowledge field
	}
}

func (a *GraphRepositoryAdapter) convertGraphDataToKnowledge(g *domainrag.GraphData) *domain_knowledge.GraphData {
	if g == nil {
		return nil
	}

	nodes := make([]*domain_knowledge.GraphNode, len(g.Node))
	for i, n := range g.Node {
		nodes[i] = a.convertNodeToKnowledge(n)
	}

	relations := make([]*domain_knowledge.GraphRelation, len(g.Relation))
	for i, r := range g.Relation {
		relations[i] = a.convertRelationToKnowledge(r)
	}

	return &domain_knowledge.GraphData{
		Node:     nodes,
		Relation: relations,
	}
}

func (a *GraphRepositoryAdapter) convertGraphDataToRAG(g *domain_knowledge.GraphData) *domainrag.GraphData {
	if g == nil {
		return nil
	}

	nodes := make([]*domainrag.GraphNode, len(g.Node))
	for i, n := range g.Node {
		nodes[i] = a.convertNodeToRAG(n)
	}

	relations := make([]*domainrag.GraphRelation, len(g.Relation))
	for i, r := range g.Relation {
		relations[i] = a.convertRelationToRAG(r)
	}

	return &domainrag.GraphData{
		Node:     nodes,
		Relation: relations,
	}
}

func (a *GraphRepositoryAdapter) convertNodeToKnowledge(n *domainrag.GraphNode) *domain_knowledge.GraphNode {
	if n == nil {
		return nil
	}

	// Convert slices
	chunks := make([]string, len(n.Chunks))
	copy(chunks, n.Chunks)

	attributes := make([]string, len(n.Attributes))
	copy(attributes, n.Attributes)

	return &domain_knowledge.GraphNode{
		ID:         n.ID,
		Name:       n.Name,
		EntityType: n.EntityType,
		Attributes: attributes,
		Chunks:     chunks,
	}
}

func (a *GraphRepositoryAdapter) convertNodeToRAG(n *domain_knowledge.GraphNode) *domainrag.GraphNode {
	if n == nil {
		return nil
	}

	chunks := make([]string, len(n.Chunks))
	copy(chunks, n.Chunks)

	attributes := make([]string, len(n.Attributes))
	copy(attributes, n.Attributes)

	return &domainrag.GraphNode{
		ID:         n.ID,
		Name:       n.Name,
		EntityType: n.EntityType,
		Attributes: attributes,
		Chunks:     chunks,
	}
}

func (a *GraphRepositoryAdapter) convertRelationToKnowledge(r *domainrag.GraphRelation) *domain_knowledge.GraphRelation {
	if r == nil {
		return nil
	}

	// rag.GraphRelation uses ChunkIDs
	chunkIDs := make([]string, len(r.ChunkIDs))
	copy(chunkIDs, r.ChunkIDs)

	return &domain_knowledge.GraphRelation{
		ID:          r.ID,
		Source:      r.Source,
		Target:      r.Target,
		Type:        r.Type,
		Description: "", // rag.GraphRelation doesn't have Description
		Strength:    r.Strength,
		Weight:      r.Weight,
		ChunkIDs:    chunkIDs,
	}
}

func (a *GraphRepositoryAdapter) convertRelationToRAG(r *domain_knowledge.GraphRelation) *domainrag.GraphRelation {
	if r == nil {
		return nil
	}

	// knowledge.GraphRelation uses ChunkIDs
	chunkIDs := make([]string, len(r.ChunkIDs))
	copy(chunkIDs, r.ChunkIDs)

	return &domainrag.GraphRelation{
		ID:       r.ID,
		Source:   r.Source,
		Target:   r.Target,
		Type:     r.Type,
		Strength: r.Strength,
		Weight:   r.Weight,
		ChunkIDs: chunkIDs,
	}
}
