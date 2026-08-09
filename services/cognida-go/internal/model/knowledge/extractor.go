// Package knowledge provides the knowledge management domain definitions.
package knowledge

import "context"

// ========================================
// Graph Extractor Interface (Domain Port)
// ========================================

// GraphExtractor 图谱抽取器接口（领域端口）。
//
// 应用层(service/knowledge.GraphService)依赖此抽象完成实体/关系抽取，
// 具体实现(基于 LLM 的抽取器)位于基础设施层并经 Wire 注入，
// 从而消除 service → infrastructure 的直接依赖。
type GraphExtractor interface {
	// ExtractEntities 从文档中抽取实体节点
	ExtractEntities(
		ctx context.Context,
		chunkID, document, query string,
		mode ExtractionMode,
	) ([]*GraphNode, error)

	// ExtractRelations 基于已抽取实体抽取关系
	ExtractRelations(
		ctx context.Context,
		kbID, chunkID, document, query string,
		entities []*GraphNode,
		mode ExtractionMode,
	) ([]*GraphRelation, error)

	// ExtractGraphJoint 联合抽取实体与关系
	ExtractGraphJoint(
		ctx context.Context,
		chunkID, document string,
		existingContext *ExtractionContext,
	) ([]*GraphNode, []*GraphRelation, error)

	// ExtractGraphIncremental 增量抽取实体与关系
	ExtractGraphIncremental(
		ctx context.Context,
		chunkID, document string,
		existingContext *ExtractionContext,
	) ([]*GraphNode, []*GraphRelation, error)
}
