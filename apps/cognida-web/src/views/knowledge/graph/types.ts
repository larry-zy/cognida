// 知识图谱视图共享类型 —— 拆分自 GraphView.vue〔M14〕
// 容器与各子组件共用的本地视图类型（vis-network 数据结构 + 关系展示模型）

export interface RelationDisplay {
  id: string
  label?: string
  type?: string
  description?: string
  strength?: number
  from?: string
  to?: string
  source?: string
  target?: string
  chunk_ids?: string[]
}

export interface VisNode {
  id: string
  label: string
  color?: string
  size?: number
  attributes?: string[]
  chunks?: string[]
  entity_type?: string
}

export interface VisEdge {
  id: string
  from: string
  to: string
  label?: string
  color?: string
  width?: number
  description?: string
  strength?: number
  type?: string
  source?: string
  target?: string
  chunk_ids?: string[]
}

export interface VisGraphData {
  nodes: VisNode[]
  edges: VisEdge[]
}

// 来源分块查看器条目
export interface ChunkViewerItem {
  id: string
  loading: boolean
  content: string
  error: string
}

// ===== 图谱增删改弹窗表单 =====
export interface AddNodeForm {
  name: string
  entity_type: string
  attributesStr: string
}

export interface AddRelationForm {
  source: string
  target: string
  type: string
  strength: number
  weight: number
}

export interface EditNodeForm {
  id: string
  name: string
  entity_type: string
  attributesStr: string
}

export interface EditRelationForm {
  id: string
  type: string
  strength: number
}

// 下拉选项（名称 / 值）
export interface LabeledOption {
  label: string
  value: string
}

// 滑块刻度
export interface SliderMark {
  value: number
  label: string
}
