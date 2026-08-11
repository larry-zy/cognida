/**
 * 图谱相关类型定义
 */

// 图谱节点
export interface GraphNode {
  id: string
  name: string
  entity_type: string
  attributes: string[]
  chunks: string[]
}

// 图谱关系
export interface GraphRelation {
  id: string
  chunk_ids: string[]
  combined_degree: number
  weight: number
  source: string
  target: string
  type: string
  description: string
  strength: number
}

// 图谱数据
export interface GraphData {
  Node?: GraphNode[]
  Relation?: GraphRelation[]
  nodes?: GraphNode[]
  relations?: GraphRelation[]
}

// 图谱搜索请求
export interface GraphSearchRequest {
  nodes: string[]
}

// 图谱统计信息
export interface GraphStatistics {
  node_count: number
  relation_count: number
  entity_types: Record<string, number>
}

// 添加节点请求
export interface AddNodeRequest {
  name: string
  entity_type?: string
  attributes?: string[]
}

// 更新节点请求
export interface UpdateNodeRequest {
  name: string
  title?: string          // 新增：节点标题/描述
  entity_type?: string
  attributes?: string[]
}

// 添加关系请求
export interface AddRelationRequest {
  source: string
  target: string
  type: string
  strength?: number
  weight?: number
}

// 更新关系请求
export interface UpdateRelationRequest {
  type: string
  description?: string
  strength?: number
}

// 节点详情响应
export interface NodeDetailResponse {
  node: GraphNode
  neighbors?: GraphNode[]
  relations: GraphRelation[]
  degree?: number
}

// 图谱统计响应（对应后端 GraphStatsDTO）
export interface GraphStatsResponse {
  node_count: number
  relation_count: number
  avg_degree: number
  avg_strength: number
  avg_weight: number
  max_degree: number
  isolated_nodes: number
  component_count: number
}

// 布局类型
export type LayoutType = 'force' | 'hierarchical' | 'circular' | 'radial' | 'concentric'

// 图谱配置
export interface GraphConfig {
  layout: LayoutType
  nodeSize: number
  edgeWidth: number
  nodeLabel: boolean
  edgeLabel: boolean
}

// 关系类型选项（后端返回小写字段名）
export interface RelationTypeOption {
  value: string
  label: string
}
