<template>
  <div class="gv-page" :class="{ 'gv-page--embedded': embedded }">

    <!-- ===== 顶部工具栏 ===== -->
    <GraphToolbar
      v-model:search-text="searchText"
      :embedded="embedded"
      :kb-id="kbId"
      @search="handleSearch"
      @add-node="showAddNodeDialog"
      @add-relation="showAddRelationDialog"
      @export="handleExport"
      @toggle-fullscreen="toggleFullscreen"
      @clear-graph="handleClearGraph"
    />

    <!-- ===== 主体：画布 + 详情抽屉 ===== -->
    <div class="gv-body">

      <!-- 图谱画布区 -->
      <!-- 全屏目标为外层 wrap（含图例/缩放/背景点阵），而非内层裸 canvas〔FE-2〕 -->
      <div ref="canvasWrap" class="gv-canvas-wrap">
        <div ref="graphContainer" class="gv-canvas">
          <div v-if="loading" class="gv-loader">
            <UiLoader size="lg" />
          </div>
        </div>

        <!-- 图例浮层（左下角） -->
        <GraphLegend :items="legendItems" />

        <!-- 缩放控件（右下角） -->
        <GraphZoomControls @zoom-in="zoomIn" @zoom-out="zoomOut" @fit="fitGraph" />
      </div>

      <!-- 节点详情抽屉（右侧，宽 300px，选中节点时显示） -->
      <GraphNodeDetailPanel
        v-if="detailDrawerVisible"
        :selected-node="selectedNode"
        :selected-node-type="selectedNodeType"
        :selected-node-attributes="selectedNodeAttributes"
        :selected-node-chunks="selectedNodeChunks"
        :node-relations="nodeRelations"
        :deleting="deleting"
        @close="detailDrawerVisible = false"
        @open-chunk-viewer="openChunkViewer"
        @edit-relation="showEditRelationDialog"
        @delete-relation="handleDeleteRelation"
        @edit-node="showEditNodeDialog"
        @delete-node="handleDeleteNode"
      />
    </div>

    <!-- ===== 底部状态栏 ===== -->
    <GraphStatusBar
      v-if="!embedded"
      :node-count="nodeCount"
      :edge-count="edgeCount"
      :entity-type-count="entityTypeCount"
      :stats="stats"
      :last-sync-time="lastSyncTime"
    />

    <!-- ===== 增删改弹窗集合（添加/编辑 节点/关系） ===== -->
    <GraphDialogs
      v-model:add-node-visible="addNodeDialogVisible"
      v-model:add-relation-visible="addRelationDialogVisible"
      v-model:edit-node-visible="editNodeDialogVisible"
      v-model:edit-relation-visible="editRelationDialogVisible"
      v-model:add-node-form="addNodeForm"
      v-model:add-relation-form="addRelationForm"
      v-model:edit-node-form="editNodeForm"
      v-model:edit-relation-form="editRelationForm"
      :entity-type-options="entityTypeOptions"
      :relation-type-options="relationTypeOptions"
      :node-options="nodeOptions"
      :strength-marks="strengthMarks"
      :adding="adding"
      :editing="editing"
      @confirm-add-node="handleAddNode"
      @confirm-add-relation="handleAddRelation"
      @confirm-edit-node="handleEditNode"
      @confirm-edit-relation="handleEditRelation"
    />

    <!-- ===== 来源分块查看器 Modal ===== -->
    <GraphChunkViewerModal
      v-model="chunkViewerVisible"
      :title="chunkViewerTitle"
      :items="chunkViewerItems"
    />

    <!-- ===== 确认对话框 ===== -->
    <UiConfirm
      v-model:show="confirmVisible"
      :title="confirmTitle"
      :message="confirmMessage"
      type="warning"
      @confirm="handleConfirm"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, nextTick, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Network } from 'vis-network'
import { DataSet } from 'vis-data'
import 'vis-network/styles/vis-network.css'
import { graphApi } from '@/api/graph'
import { knowledgeApi } from '@/api/knowledge'
import type {
  GraphData,
  AddNodeRequest,
  AddRelationRequest,
  UpdateNodeRequest,
  UpdateRelationRequest,
  RelationTypeOption,
  NodeDetailResponse,
  GraphStatsResponse
} from '@/types'

import { toastSuccess, toastError, toastWarning } from '@/utils/toast'

// UI Components（容器自用）
import UiConfirm from '@/components/ui/UiConfirm.vue'
import UiLoader from '@/components/ui/UiLoader.vue'

// 拆分子组件〔M14〕
import GraphToolbar from './graph/GraphToolbar.vue'
import GraphLegend from './graph/GraphLegend.vue'
import GraphZoomControls from './graph/GraphZoomControls.vue'
import GraphNodeDetailPanel from './graph/GraphNodeDetailPanel.vue'
import GraphStatusBar from './graph/GraphStatusBar.vue'
import GraphDialogs from './graph/GraphDialogs.vue'
import GraphChunkViewerModal from './graph/GraphChunkViewerModal.vue'

// 图谱配色 composable〔M14〕
import { useGraphColors } from './graph/composables/useGraphColors'

// 共享视图类型〔M14〕
import type {
  VisNode,
  VisEdge,
  VisGraphData,
  RelationDisplay,
  ChunkViewerItem,
  AddNodeForm,
  AddRelationForm,
  EditNodeForm,
  EditRelationForm
} from './graph/types'

// ========================================
// State
// ========================================

// props：既可作为独立路由页（读 route.params.kbId），也可作为知识库详情页内的图谱 Tab 嵌入
// - kbId：外部直接指定知识库 ID（嵌入模式）；不传则回退到路由参数
// - embedded：嵌入模式，去掉整页 full-bleed 负边距/底部状态栏，适配 tab 容器
// - active：嵌入在 v-show tab 中时用于控制何时首次加载（避免 display:none 下容器 0×0）
const props = withDefaults(defineProps<{
  kbId?: string
  embedded?: boolean
  active?: boolean
}>(), {
  embedded: false,
  active: true
})

const route = useRoute()
const kbId = ref<string>(props.kbId || (route.params.kbId as string))

const graphContainer = ref<HTMLElement>()
// 全屏容器：外层 wrap（含图例/缩放/背景点阵），全屏时一并保留控件〔FE-2〕
const canvasWrap = ref<HTMLElement>()

const graphData = ref<VisGraphData>({ nodes: [], edges: [] })
const loading = ref(false)
const searchText = ref('')
const selectedNode = ref<VisNode | null>(null)
const nodeRelations = ref<RelationDisplay[]>([])

const detailDrawerVisible = ref(false)

// 图谱统计（真实值，来自后端 /graph/stats）
const stats = ref<GraphStatsResponse | null>(null)

const addNodeDialogVisible = ref(false)
const addNodeForm = ref<AddNodeForm>({ name: '', entity_type: 'concept', attributesStr: '' })

const addRelationDialogVisible = ref(false)
const addRelationForm = ref<AddRelationForm>({ source: '', target: '', type: 'relates', strength: 5, weight: 5 })

// 分块查看器（实体/关系 → 来源分块）
const chunkViewerVisible = ref(false)
const chunkViewerTitle = ref('')
const chunkViewerItems = ref<ChunkViewerItem[]>([])

const adding = ref(false)
const deleting = ref(false)

const relationTypeOptions = ref<RelationTypeOption[]>([])

const editNodeDialogVisible = ref(false)
const editNodeForm = ref<EditNodeForm>({ id: '', name: '', entity_type: 'concept', attributesStr: '' })

const editRelationDialogVisible = ref(false)
const editRelationForm = ref<EditRelationForm>({ id: '', type: 'relates', strength: 5 })

const editing = ref(false)

const confirmVisible = ref(false)
const confirmTitle = ref('')
const confirmMessage = ref('')
const confirmCallback = ref<(() => void) | null>(null)

let network: any = null
// 提升到模块作用域，供 mergeNeighbors 在下钻时向画布增量注入新节点/边
let nodesDataSet: any = null
let edgesDataSet: any = null

// 配色（实体类型 → 钢青灰主色族）〔M14〕
const { getTypeColor, buildVisNodeColor } = useGraphColors()

// ========================================
// Computed
// ========================================

const nodeCount = computed(() => graphData.value.nodes?.length || 0)
const edgeCount = computed(() => graphData.value.edges?.length || 0)
const entityTypeCount = computed(() => {
  const types = new Set(graphData.value.nodes?.map(n => n.entity_type) || [])
  return types.size
})

const lastSyncTime = ref('--:--')
function updateSyncTime() {
  const now = new Date()
  const h = String(now.getHours()).padStart(2, '0')
  const m = String(now.getMinutes()).padStart(2, '0')
  lastSyncTime.value = `${h}:${m}`
}

const selectedNodeType = computed(() => selectedNode.value?.entity_type || '-')
const selectedNodeAttributes = computed(() => selectedNode.value?.attributes || [])
const selectedNodeChunks = computed(() => selectedNode.value?.chunks || [])

const nodeOptions = computed(() =>
  graphData.value.nodes.map(n => ({ label: n.label, value: n.id }))
)

const entityTypeOptions = [
  { label: '人物', value: 'person' },
  { label: '组织', value: 'organization' },
  { label: '地点', value: 'location' },
  { label: '概念', value: 'concept' },
  { label: '其他', value: 'other' }
]

const strengthMarks = [
  { value: 1, label: '1' },
  { value: 5, label: '5' },
  { value: 10, label: '10' }
]

// 图例数据（右侧面板）
const legendItems = computed(() => {
  const seen = new Set<string>()
  const result: { type: string; label: string; color: string }[] = []
  for (const node of graphData.value.nodes) {
    const t = node.entity_type || 'other'
    if (!seen.has(t)) {
      seen.add(t)
      result.push({ type: t, label: t, color: getTypeColor(t) })
    }
  }
  // 空图不再渲染 service/component/storage/doc 假占位项，仅在有对应类型节点时才显示图例〔轻微〕
  return result
})

// ========================================
// vis-network 初始化（月夜台账配色）
// ========================================

function initGraph() {
  if (!graphContainer.value) {
    toastError('图谱容器未找到')
    return
  }

  const container = graphContainer.value as HTMLElement

  const options = {
    nodes: {
      shape: 'dot',
      size: 16,
      font: {
        size: 12,
        color: '#a8a69e',           // --text-secondary 暖灰
        face: 'IBM Plex Mono, JetBrains Mono, monospace'
      },
      borderWidth: 0,
      shadow: false
    },
    edges: {
      width: 1,
      color: {
        color:     'rgba(232,230,223,0.14)',  // 普通边：极淡
        highlight: 'rgba(156,180,205,0.5)',  // 选中边：银青半透
        hover:     'rgba(156,180,205,0.35)'
      },
      arrows: {
        to: {
          enabled: true,
          scaleFactor: 0.6,
          type: 'arrow'
        }
      },
      smooth: {
        enabled: true,
        type: 'continuous',
        roundness: 0.4
      },
      font: {
        size: 9,
        color: '#6d6c66',            // --text-muted 边标签
        face: 'IBM Plex Mono, monospace',
        background: 'transparent',
        strokeWidth: 0,
        align: 'middle'
      },
      labelHighlightBold: false
    },
    physics: {
      forceAtlas2Based: {
        gravitationalConstant: -50,
        centralGravity: 0.01,
        springLength: 100,
        springConstant: 0.08
      },
      maxVelocity: 50,
      solver: 'forceAtlas2Based',
      timestep: 0.35,
      stabilization: { iterations: 150 }
    },
    interaction: {
      hover: true,
      tooltipDelay: 200,
      zoomView: true,
      dragView: true
    }
  }

  try {
    nodesDataSet = new DataSet(graphData.value.nodes)
    edgesDataSet = new DataSet(graphData.value.edges)

    network = new Network(container, { nodes: nodesDataSet, edges: edgesDataSet }, options)

    // 稳定完成后关闭 physics：forceAtlas2 稳定后若继续模拟会持续占用主线程，
    // 表现为进入图谱页后整个标签页卡顿、无法切换其他页面。稳定后停掉即可。
    network.once('stabilizationIterationsDone', () => {
      if (network) network.setOptions({ physics: false })
    })

    network.on('click', (params: any) => {
      if (params.nodes.length > 0) {
        const nodeId = params.nodes[0]
        const node = graphData.value.nodes.find(n => n.id === nodeId)
        if (node) {
          selectedNode.value = node
          detailDrawerVisible.value = true
          loadNodeRelations(node)
        }
      }
    })

    network.on('hoverNode', () => {
      network.canvas.body.container.style.cursor = 'pointer'
    })

    network.on('blurNode', () => {
      network.canvas.body.container.style.cursor = 'default'
    })
  } catch (error) {
    toastError(`图谱初始化失败: ${error}`)
  }
}

// ========================================
// Data Loading
// ========================================

async function loadGraph() {
  loading.value = true
  try {
    const res = await graphApi.getGraph(kbId.value)
    if (res.data) {
      graphData.value = convertToVisData(res.data)
      if (network) { network.destroy(); network = null }
      initGraph()
      updateSyncTime()
    }
  } catch (error: any) {
    toastError(`加载图谱失败: ${error?.message || error}`)
  } finally {
    loading.value = false
  }
  // 统计信息非关键路径，失败不阻塞图谱渲染
  loadStats()
}

// 加载图谱统计（平均度/最大度/孤立节点/连通分量）
async function loadStats() {
  try {
    const res = await graphApi.getStats(kbId.value)
    stats.value = res.data || null
  } catch {
    stats.value = null
  }
}

function convertToVisData(data: GraphData): VisGraphData {
  const rawNodes = data.Node || data.nodes || []
  const rawEdges = data.Relation || data.relations || []

  const nodes: VisNode[] = rawNodes.map(node => {
    const entityType = node.entity_type || 'Other'
    const baseColor = getTypeColor(entityType)
    return {
      id: node.id,
      label: node.name,
      // vis-network color object：正常 fill、选中时银青
      color: buildVisNodeColor(baseColor) as any,
      size: 16,
      attributes: node.attributes || [],
      chunks: node.chunks || [],
      entity_type: entityType
    }
  })

  const edges: VisEdge[] = rawEdges.map(rel => {
    const fromNode = nodes.find(n => n.id === rel.source || n.label === rel.source)
    const toNode   = nodes.find(n => n.id === rel.target || n.label === rel.target)
    return {
      id:          rel.id,
      from:        fromNode ? fromNode.id : rel.source,
      to:          toNode   ? toNode.id   : rel.target,
      label:       rel.type,
      // 边颜色由 options.edges.color 统一控制；此处不覆盖
      width:       1,
      description: rel.description || '',
      strength:    rel.strength || 0,
      type:        rel.type,
      source:      rel.source,
      target:      rel.target,
      chunk_ids:   rel.chunk_ids || []
    }
  })

  return { nodes, edges }
}

// 用已加载的本地边兜底展示关联关系（双向）
function applyLocalRelations(nodeId: string) {
  const relations = graphData.value.edges.filter(
    (edge: VisEdge) => edge.from === nodeId || edge.to === nodeId
  )
  const getNodeName = (id: string) => {
    const n = graphData.value.nodes.find(node => node.id === id)
    return n?.label || id
  }
  nodeRelations.value = relations.map((rel: VisEdge) => ({
    id:          rel.id,
    label:       rel.label || rel.type,
    type:        rel.type,
    description: rel.description || '-',
    strength:    rel.strength || 0,
    from:        getNodeName(rel.from),
    to:          getNodeName(rel.to),
    source:      rel.source || getNodeName(rel.from),
    target:      rel.target || getNodeName(rel.to),
    chunk_ids:   rel.chunk_ids || []
  }))
}

// 下钻：把服务端返回的邻居/关系里画布上还没有的部分增量注入
// physics 稳定后已永久关闭，注入无坐标的新节点会全部堆在 (0,0)。
// 取被点节点当前坐标，让新邻居围绕它做小半径散开，避免"点一次就炸"〔FE-1〕
function mergeNeighbors(detail: NodeDetailResponse, centerId: string) {
  if (!nodesDataSet || !edgesDataSet) return

  const nodeById = new Map(graphData.value.nodes.map(n => [n.id, n]))
  const nameToId = new Map(graphData.value.nodes.map(n => [n.label, n.id]))

  const newNodes: VisNode[] = []
  for (const nb of detail.neighbors || []) {
    if (!nb || !nb.id) continue
    if (nodeById.has(nb.id) || nameToId.has(nb.name)) continue
    const et = nb.entity_type || 'Other'
    const baseColor = getTypeColor(et)
    const vn: VisNode = {
      id: nb.id,
      label: nb.name,
      color: buildVisNodeColor(baseColor) as any,
      size: 16,
      attributes: nb.attributes || [],
      chunks: nb.chunks || [],
      entity_type: et
    }
    newNodes.push(vn)
    nodeById.set(nb.id, vn)
    nameToId.set(nb.name, nb.id)
  }
  if (newNodes.length) {
    graphData.value.nodes.push(...newNodes)
    // 被点节点当前坐标作为散布中心（physics 已关，需手动给初始坐标）
    let cx = 0
    let cy = 0
    if (network) {
      const pos = network.getPositions([centerId])
      const p = pos && pos[centerId]
      if (p) { cx = p.x; cy = p.y }
    }
    // 围绕中心节点小半径环形散开注入画布，x/y 仅用于初始定位，仍可拖拽
    const radius = 130
    const positioned = newNodes.map((vn, i) => {
      const angle = (2 * Math.PI * i) / newNodes.length
      return { ...vn, x: cx + radius * Math.cos(angle), y: cy + radius * Math.sin(angle) }
    })
    nodesDataSet.add(positioned)
  }

  const edgeIds = new Set(graphData.value.edges.map(e => e.id))
  const newEdges: VisEdge[] = []
  for (const rel of detail.relations || []) {
    if (!rel || !rel.id || edgeIds.has(rel.id)) continue
    // 端点必须落在画布已有节点上，否则 vis.js 会按未知 id 凭空生成一个"幽灵节点"。
    // 服务端 relations 可能引用本次未随 neighbors 返回的节点——此类边直接跳过〔#9〕。
    const fromId = nameToId.get(rel.source)
    const toId = nameToId.get(rel.target)
    if (!fromId || !toId) continue
    const ve: VisEdge = {
      id: rel.id,
      from: fromId,
      to: toId,
      label: rel.type,
      width: 1,
      description: rel.description || '',
      strength: rel.strength || 0,
      type: rel.type,
      source: rel.source,
      target: rel.target,
      chunk_ids: rel.chunk_ids || []
    }
    newEdges.push(ve)
    edgeIds.add(rel.id)
  }
  if (newEdges.length) {
    graphData.value.edges.push(...newEdges)
    edgesDataSet.add(newEdges)
  }
}

// 点击节点：先本地兜底，再用服务端 1-hop 双向邻居作为权威结果，并把新邻居并入画布
async function loadNodeRelations(node: VisNode) {
  applyLocalRelations(node.id)
  try {
    // 后端 GetNodeDetail 以节点 name 为中心匹配（Cypher MATCH (n:Entity {name: $name})），
    // 路由参数虽名为 nodeId 实为名称，故此处仍传 node.label；散布中心用 node.id 定位画布坐标〔FE-1〕
    const res = await graphApi.getNodeDetail(kbId.value, node.label)
    const detail = res.data
    if (!detail || !detail.node) return
    mergeNeighbors(detail, node.id)
    const rels = detail.relations || []
    nodeRelations.value = rels.map(rel => ({
      id:          rel.id,
      label:       rel.type,
      type:        rel.type,
      description: rel.description || '-',
      strength:    rel.strength || 0,
      from:        rel.source,
      to:          rel.target,
      source:      rel.source,
      target:      rel.target,
      chunk_ids:   rel.chunk_ids || []
    }))
  } catch {
    // 服务端失败时保留本地兜底结果
  }
}

// ========================================
// User Actions
// ========================================

// 增删改后按当前视图状态刷新：搜索态回搜索子图，否则刷全量图，
// 避免在搜索结果上做增删改后被静默刷回全量图（搜索框有字、结果却丢失）〔FE-5〕
function refreshView() {
  if (searchText.value.trim()) return handleSearch()
  return loadGraph()
}

async function handleSearch() {
  if (!searchText.value.trim()) { loadGraph(); return }
  loading.value = true
  try {
    const res = await graphApi.searchNode(kbId.value, { nodes: [searchText.value] })
    if (res.data) {
      graphData.value = convertToVisData(res.data)
      if (network) { network.destroy(); network = null }
      initGraph()
    }
  } catch (error: any) {
    toastError(error.message || '搜索失败')
  } finally {
    loading.value = false
  }
}

function showAddNodeDialog() {
  addNodeForm.value = { name: '', entity_type: 'concept', attributesStr: '' }
  addNodeDialogVisible.value = true
}

async function handleAddNode() {
  if (!addNodeForm.value.name) { toastWarning('请输入节点名称'); return }
  adding.value = true
  try {
    const attributes = addNodeForm.value.attributesStr
      ? addNodeForm.value.attributesStr.split(',').map(s => s.trim())
      : []
    const data: AddNodeRequest = {
      name: addNodeForm.value.name,
      entity_type: addNodeForm.value.entity_type,
      attributes
    }
    const res = await graphApi.addNode(kbId.value, data)
    if (res.data) {
      toastSuccess('节点添加成功')
      addNodeDialogVisible.value = false
      await refreshView()
    }
  } catch (error: any) {
    toastError(error.message || '添加节点失败')
  } finally {
    adding.value = false
  }
}

function showAddRelationDialog() {
  addRelationForm.value = { source: '', target: '', type: 'relates', strength: 5, weight: 5 }
  addRelationDialogVisible.value = true
}

async function handleAddRelation() {
  if (!addRelationForm.value.source || !addRelationForm.value.target) {
    toastWarning('请选择源节点和目标节点'); return
  }
  adding.value = true
  try {
    const data: AddRelationRequest = {
      source:   addRelationForm.value.source,
      target:   addRelationForm.value.target,
      type:     addRelationForm.value.type,
      strength: addRelationForm.value.strength,
      weight:   addRelationForm.value.weight
    }
    const res = await graphApi.addRelation(kbId.value, data)
    if (res.data) {
      toastSuccess('关系添加成功')
      addRelationDialogVisible.value = false
      await refreshView()
    }
  } catch (error: any) {
    toastError(error.message || '添加关系失败')
  } finally {
    adding.value = false
  }
}

function showEditNodeDialog() {
  if (!selectedNode.value) return
  editNodeForm.value = {
    id:           selectedNode.value.id,
    name:         selectedNode.value.label,
    entity_type:  selectedNode.value.entity_type || 'concept',
    attributesStr: selectedNode.value.attributes?.join(', ') || ''
  }
  editNodeDialogVisible.value = true
}

async function handleEditNode() {
  if (!editNodeForm.value.name) { toastWarning('请输入节点名称'); return }
  editing.value = true
  try {
    const attributes = editNodeForm.value.attributesStr
      ? editNodeForm.value.attributesStr.split(',').map(s => s.trim())
      : []
    const data: UpdateNodeRequest = {
      name:        editNodeForm.value.name,
      title:       editNodeForm.value.name,
      entity_type: editNodeForm.value.entity_type,
      attributes
    }
    const res = await graphApi.updateNode(kbId.value, editNodeForm.value.id, data)
    if (res.data) {
      toastSuccess('节点更新成功')
      editNodeDialogVisible.value = false
      detailDrawerVisible.value = false
      await refreshView()
    }
  } catch (error: any) {
    toastError(error.message || '更新节点失败')
  } finally {
    editing.value = false
  }
}

function showEditRelationDialog(rel: RelationDisplay) {
  editRelationForm.value = {
    id:       rel.id,
    type:     rel.type || rel.label || 'relates',
    strength: rel.strength || 5
  }
  editRelationDialogVisible.value = true
}

async function handleEditRelation() {
  if (!editRelationForm.value.type) { toastWarning('请选择关系类型'); return }
  editing.value = true
  try {
    const originalRelation = nodeRelations.value.find(r => r.id === editRelationForm.value.id)
    const originalDescription = originalRelation?.description && originalRelation.description !== '-'
      ? originalRelation.description : ''
    const data: UpdateRelationRequest = {
      type:        editRelationForm.value.type,
      description: originalDescription,
      strength:    editRelationForm.value.strength
    }
    const res = await graphApi.updateRelation(kbId.value, editRelationForm.value.id, data)
    if (res.data) {
      toastSuccess('关系更新成功')
      editRelationDialogVisible.value = false
      detailDrawerVisible.value = false
      await refreshView()
    }
  } catch (error: any) {
    toastError(error.message || '更新关系失败')
  } finally {
    editing.value = false
  }
}

// #3 清空整个图谱
function handleClearGraph() {
  if (!graphData.value.nodes?.length) { toastWarning('图谱已为空'); return }
  confirmTitle.value = '清空图谱'
  confirmMessage.value = '确定要清空整个图谱吗？将删除全部节点与关系，且不可恢复。'
  confirmCallback.value = async () => {
    try {
      await graphApi.deleteGraph(kbId.value)
      toastSuccess('图谱已清空')
      detailDrawerVisible.value = false
      selectedNode.value = null
      await loadGraph()
    } catch (error: any) {
      toastError(error.message || '清空图谱失败')
    }
  }
  confirmVisible.value = true
}

// #5 查看实体/关系的来源分块内容
async function openChunkViewer(title: string, ids: string[]) {
  const list = (ids || []).filter(Boolean)
  if (list.length === 0) { toastWarning('暂无关联分块'); return }
  chunkViewerTitle.value = title
  chunkViewerItems.value = list.map(id => ({ id, loading: true, content: '', error: '' }))
  chunkViewerVisible.value = true
  await Promise.all(chunkViewerItems.value.map(async (item) => {
    try {
      const res = await knowledgeApi.getChunk(kbId.value, item.id)
      item.content = res.data?.content || ''
      if (!item.content) item.error = '（空内容）'
    } catch (e: any) {
      item.error = e?.message || '加载失败'
    } finally {
      item.loading = false
    }
  }))
}

function handleExport() {
  if (!graphData.value.nodes?.length) { toastWarning('没有可导出的数据'); return }
  const dataStr = JSON.stringify(graphData.value, null, 2)
  const blob = new Blob([dataStr], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `graph-${kbId.value}-${Date.now()}.json`
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
  toastSuccess('导出成功')
}

async function loadRelationTypes() {
  try {
    const res = await graphApi.getRelationTypes(kbId.value)
    if (res.data) { relationTypeOptions.value = res.data }
  } catch {
    relationTypeOptions.value = [
      { value: 'contains', label: '包含' },
      { value: 'relates',  label: '关联' },
      { value: 'depends',  label: '依赖' },
      { value: 'belongs',  label: '属于' },
      { value: 'owns',     label: '拥有' },
      { value: 'author',   label: '作者' },
      { value: 'alias',    label: '别名' },
      { value: 'other',    label: '其他' }
    ]
  }
}

function handleDeleteNode() {
  if (!selectedNode.value) return
  confirmTitle.value = '删除节点'
  confirmMessage.value = `确定要删除节点"${selectedNode.value.label}"吗？删除节点将同时删除所有相关关系。`
  confirmCallback.value = async () => {
    deleting.value = true
    try {
      await graphApi.deleteNode(kbId.value, selectedNode.value!.id)
      toastSuccess('节点删除成功')
      detailDrawerVisible.value = false
      await refreshView()
    } catch (error: any) {
      toastError(error.message || '删除节点失败')
    } finally {
      deleting.value = false
    }
  }
  confirmVisible.value = true
}

function handleDeleteRelation(rel: RelationDisplay) {
  confirmTitle.value = '删除关系'
  confirmMessage.value = '确定要删除关系吗？'
  confirmCallback.value = async () => {
    deleting.value = true
    try {
      await graphApi.deleteRelation(kbId.value, rel.id)
      toastSuccess('关系删除成功')
      detailDrawerVisible.value = false
      await refreshView()
    } catch (error: any) {
      toastError(error.message || '删除关系失败')
    } finally {
      deleting.value = false
    }
  }
  confirmVisible.value = true
}

function handleConfirm() {
  if (confirmCallback.value) { confirmCallback.value(); confirmCallback.value = null }
}

function toggleFullscreen() {
  if (!document.fullscreenElement) {
    // 对外层 wrap 请求全屏，保留图例/缩放/背景点阵；只全屏内层 canvas 会丢失所有控件〔FE-2〕
    canvasWrap.value?.requestFullscreen()
  } else {
    document.exitFullscreen()
  }
}

// 全屏进入/退出后 wrap 尺寸变化，vis-network 需重算画布尺寸并适配视图〔FE-2〕
function handleFullscreenChange() {
  nextTick(() => {
    if (!network) return
    network.setSize('100%', '100%')
    network.redraw()
    network.fit({ animation: { duration: 200, easingFunction: 'easeInOutQuad' } })
  })
}

function zoomIn() {
  if (network) network.moveTo({ scale: network.getScale() * 1.25 })
}

function zoomOut() {
  if (network) network.moveTo({ scale: network.getScale() * 0.8 })
}

function fitGraph() {
  if (network) network.fit({ animation: { duration: 300, easingFunction: 'easeInOutQuad' } })
}

// ========================================
// Lifecycle
// ========================================

let inited = false

async function ensureInited() {
  if (inited) return
  await nextTick()
  // 容器不可见（tab 未激活，display:none）时宽高为 0，vis-network 无法正确布局，
  // 推迟到激活后再初始化。
  if (!graphContainer.value || graphContainer.value.offsetParent === null) return
  inited = true
  await loadGraph()
  await loadRelationTypes()
}

onMounted(async () => {
  document.addEventListener('fullscreenchange', handleFullscreenChange)
  if (props.active) await ensureInited()
})

// 嵌入式 tab：从隐藏切到激活时首次加载
watch(() => props.active, async (v) => {
  if (v) await ensureInited()
})

// 外部切换知识库（嵌入模式）时重新加载
watch(() => props.kbId, async (v) => {
  if (!v) return
  kbId.value = v
  inited = false
  detailDrawerVisible.value = false
  selectedNode.value = null
  if (props.active) await ensureInited()
})

onUnmounted(() => {
  document.removeEventListener('fullscreenchange', handleFullscreenChange)
  if (network) { network.destroy(); network = null }
})
</script>

<style scoped>
/* ===================================================
   知识图谱页 — 月夜台账视觉规范
   只用 global.css 中定义的 CSS 变量；
   图渲染引擎内部（vis-network canvas）直接写字面色值
   红线：无 linear-gradient/backdrop-filter/发光 box-shadow/emoji

   〔M14〕god 组件拆分：顶栏/图例/缩放/详情抽屉/状态栏/弹窗/分块查看器
   已抽为 ./graph/ 下子组件，各自携带 scoped 样式；此处仅保留整页骨架布局。
   =================================================== */

/* 整页：用负 margin 抵消父层 .content-wrapper 的 24px padding，撑满可用区域 */
.gv-page {
  display: flex;
  flex-direction: column;
  /* 抵消 platform/index.vue 的 .content-wrapper { padding: 24px } */
  margin: -24px;
  /* 精确撑高：fill parent height + 抵消两端 padding。
     底色由壳层 .main-content 提供，不再叠一层半透明底 */
  height: calc(100% + 48px);
  overflow: hidden;
}

/* 嵌入知识库详情 tab：去掉整页 full-bleed 负边距，改为固定高度容器，
   顶栏/画布正常排布，底部状态栏隐藏 */
.gv-page--embedded {
  margin: 0;
  height: 640px;
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
}

/* ===== 主体区域（画布 + 抽屉） ===== */
.gv-body {
  flex: 1;
  min-height: 0;
  display: flex;
}

/* 画布包裹 */
.gv-canvas-wrap {
  flex: 1;
  min-width: 0;
  position: relative;
  background-color: var(--color-bg-primary);
  /* 细网格点阵：用 radial-gradient 仅为背景图案（非色彩渐变，符合规范） */
  background-image: radial-gradient(circle, rgba(232,230,223,0.06) 1px, transparent 1px);
  background-size: 24px 24px;
  overflow: hidden;
}

/* vis-network 画布容器 */
.gv-canvas {
  width: 100%;
  height: 100%;
  position: absolute;
  inset: 0;
}

/* loading 遮罩 */
.gv-loader {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(12, 15, 21, 0.7);
  z-index: 10;
}
</style>
