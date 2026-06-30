<template>
  <div class="graph-view-page">
    <!-- Page Header -->
    <UiPageHeader
      title="知识图谱"
      :breadcrumb="[
        { label: '知识库', to: '/knowledge' },
        { label: '图谱', to: '' }
      ]"
    >
      <template #actions>
        <UiButton variant="secondary" @click="handleExport">
          <template #icon><component :is="DownloadIcon" /></template>
          导出
        </UiButton>
        <UiButton variant="secondary" @click="toggleFullscreen">
          <template #icon><component :is="FullscreenIcon" /></template>
          全屏
        </UiButton>
      </template>
    </UiPageHeader>

    <!-- Main Content -->
    <div class="graph-view-container">
      <!-- Toolbar -->
      <div class="toolbar">
        <div class="search-box">
          <UiInput
            v-model="searchText"
            placeholder="搜索实体名称..."
            :clearable="true"
            @keyup.enter="handleSearch"
          >
            <template #suffix>
              <component :is="SearchIcon" />
            </template>
          </UiInput>
        </div>

        <div class="action-buttons">
          <UiButton @click="showAddNodeDialog">
            <template #icon><component :is="PlusIcon" /></template>
            添加节点
          </UiButton>
          <UiButton @click="showAddRelationDialog">
            <template #icon><component :is="ConnectionIcon" /></template>
            添加关系
          </UiButton>
        </div>
      </div>

      <!-- Graph Visualization Area -->
      <div ref="graphContainer" class="graph-container">
        <div v-if="loading" class="graph-loader">
          <UiLoader size="lg" />
        </div>
      </div>

      <!-- Status Bar -->
      <div class="status-bar">
        <div class="status-item">
          <span class="status-label">节点</span>
          <span class="status-value">{{ nodeCount }}</span>
        </div>
        <div class="status-divider"></div>
        <div class="status-item">
          <span class="status-label">关系</span>
          <span class="status-value">{{ edgeCount }}</span>
        </div>
        <div class="status-divider"></div>
        <div class="status-item">
          <span class="status-label">实体类型</span>
          <span class="status-value">{{ entityTypeCount }}</span>
        </div>
      </div>
    </div>

    <!-- Node Detail Drawer -->
    <UiDrawer
      v-model="detailDrawerVisible"
      title="节点详情"
      position="right"
      :size="500"
    >
      <div v-if="selectedNode" class="node-detail">
        <div class="detail-section">
          <h4 class="detail-title">基本信息</h4>
          <div class="detail-row">
            <span class="detail-label">名称</span>
            <span class="detail-value">{{ selectedNode.label }}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">类型</span>
            <span class="detail-value">{{ selectedNodeType }}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">属性</span>
            <div class="detail-value detail-tags">
              <span v-if="selectedNodeAttributes.length > 0" class="detail-tag">
                {{ selectedNodeAttributes.join(', ') }}
              </span>
              <span v-else class="detail-empty">-</span>
            </div>
          </div>
          <div class="detail-row">
            <span class="detail-label">关联分块数</span>
            <span class="detail-value">{{ selectedNodeChunks.length }}</span>
          </div>
        </div>

        <div class="detail-actions">
          <UiButton variant="primary" @click="showEditNodeDialog" block>
            <template #icon><component :is="EditIcon" /></template>
            编辑节点
          </UiButton>
          <UiButton variant="danger" @click="handleDeleteNode" :loading="deleting" block>
            <template #icon><component :is="TrashIcon" /></template>
            删除节点
          </UiButton>
        </div>

        <div class="detail-section">
          <h4 class="detail-title">关联关系</h4>
          <div class="relations-list">
            <div
              v-for="rel in nodeRelations"
              :key="rel.id"
              class="relation-item"
            >
              <div class="relation-header">
                <span class="relation-type">{{ rel.label || rel.type || '-' }}</span>
                <span class="relation-strength">强度: {{ rel.strength || 0 }}</span>
              </div>
              <div class="relation-content">
                <span class="relation-source">{{ rel.source || '-' }}</span>
                <component :is="ChevronRightIcon" class="relation-arrow" />
                <span class="relation-target">{{ rel.target || '-' }}</span>
              </div>
              <div class="relation-description">{{ rel.description || '-' }}</div>
              <div class="relation-actions">
                <UiButton variant="ghost" size="sm" @click="showEditRelationDialog(rel)">
                  编辑
                </UiButton>
                <UiButton variant="ghost" size="sm" @click="handleDeleteRelation(rel)">
                  删除
                </UiButton>
              </div>
            </div>

            <div v-if="nodeRelations.length === 0" class="relations-empty">
              暂无关联关系
            </div>
          </div>
        </div>
      </div>
    </UiDrawer>

    <!-- Add Node Modal -->
    <UiModal
      v-model="addNodeDialogVisible"
      title="添加节点"
      size="sm"
    >
      <div class="form-group">
        <UiInput
          v-model="addNodeForm.name"
          label="节点名称"
          placeholder="请输入节点名称"
          required
        />
      </div>
      <div class="form-group">
        <UiSelect
          v-model="addNodeForm.entity_type"
          label="实体类型"
          :options="entityTypeOptions"
          placeholder="选择类型"
        />
      </div>
      <div class="form-group">
        <UiInput
          v-model="addNodeForm.attributesStr"
          label="属性"
          type="text"
          placeholder="多个属性用逗号分隔"
        />
      </div>
      <template #footer>
        <UiButton variant="secondary" @click="addNodeDialogVisible = false">
          取消
        </UiButton>
        <UiButton variant="primary" @click="handleAddNode" :loading="adding">
          确定
        </UiButton>
      </template>
    </UiModal>

    <!-- Add Relation Modal -->
    <UiModal
      v-model="addRelationDialogVisible"
      title="添加关系"
      size="sm"
    >
      <div class="form-group">
        <UiSelect
          v-model="addRelationForm.source"
          label="源节点"
          :options="nodeOptions"
          placeholder="选择源节点"
          filterable
          required
        />
      </div>
      <div class="form-group">
        <UiSelect
          v-model="addRelationForm.target"
          label="目标节点"
          :options="nodeOptions"
          placeholder="选择目标节点"
          filterable
          required
        />
      </div>
      <div class="form-group">
        <UiSelect
          v-model="addRelationForm.type"
          label="关系类型"
          :options="relationTypeOptions"
          placeholder="选择关系类型"
          required
        />
      </div>
      <div class="form-group">
        <UiSlider
          v-model="addRelationForm.strength"
          label="强度"
          :min="1"
          :max="10"
          :marks="strengthMarks"
          :format-value="(v) => `当前值: ${v}`"
        />
      </div>
      <template #footer>
        <UiButton variant="secondary" @click="addRelationDialogVisible = false">
          取消
        </UiButton>
        <UiButton variant="primary" @click="handleAddRelation" :loading="adding">
          确定
        </UiButton>
      </template>
    </UiModal>

    <!-- Edit Node Modal -->
    <UiModal
      v-model="editNodeDialogVisible"
      title="编辑节点"
      size="sm"
    >
      <div class="form-group">
        <UiInput
          v-model="editNodeForm.name"
          label="节点名称"
          placeholder="请输入节点名称"
          required
        />
      </div>
      <div class="form-group">
        <UiSelect
          v-model="editNodeForm.entity_type"
          label="实体类型"
          :options="entityTypeOptions"
          placeholder="选择类型"
        />
      </div>
      <div class="form-group">
        <UiInput
          v-model="editNodeForm.attributesStr"
          label="属性"
          type="text"
          placeholder="多个属性用逗号分隔"
        />
      </div>
      <template #footer>
        <UiButton variant="secondary" @click="editNodeDialogVisible = false">
          取消
        </UiButton>
        <UiButton variant="primary" @click="handleEditNode" :loading="editing">
          确定
        </UiButton>
      </template>
    </UiModal>

    <!-- Edit Relation Modal -->
    <UiModal
      v-model="editRelationDialogVisible"
      title="编辑关系"
      size="sm"
    >
      <div class="form-group">
        <UiSelect
          v-model="editRelationForm.type"
          label="关系类型"
          :options="relationTypeOptions"
          placeholder="选择关系类型"
          required
        />
      </div>
      <div class="form-group">
        <UiSlider
          v-model="editRelationForm.strength"
          label="强度"
          :min="1"
          :max="10"
          :marks="strengthMarks"
          :format-value="(v) => `当前值: ${v}`"
        />
      </div>
      <template #footer>
        <UiButton variant="secondary" @click="editRelationDialogVisible = false">
          取消
        </UiButton>
        <UiButton variant="primary" @click="handleEditRelation" :loading="editing">
          确定
        </UiButton>
      </template>
    </UiModal>

    <!-- Confirm Dialog -->
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
import { ref, onMounted, onUnmounted, computed, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import { Network } from 'vis-network'
import { DataSet } from 'vis-data'
import 'vis-network/styles/vis-network.css'
import { graphApi } from '@/api/graph'
import type {
  GraphData,
  AddNodeRequest,
  AddRelationRequest,
  UpdateNodeRequest,
  UpdateRelationRequest,
  RelationTypeOption
} from '@/types'

import { h } from 'vue'
import { toastSuccess, toastError, toastWarning } from '@/utils/toast'

// UI Components
import UiPageHeader from '@/components/layout/UiPageHeader.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import UiSlider from '@/components/ui/UiSlider.vue'
import UiDrawer from '@/components/ui/UiDrawer.vue'
import UiModal from '@/components/ui/UiModal.vue'
import UiConfirm from '@/components/ui/UiConfirm.vue'
import UiLoader from '@/components/ui/UiLoader.vue'

// Icons
import {
  SearchIcon,
  PlusIcon,
  EditIcon,
  TrashIcon,
  DownloadIcon,
  ChevronRightIcon
} from '@/components/icons/icons'

const FullscreenIcon = {
  name: 'FullscreenIcon',
  render() {
    return h('svg', {
      viewBox: '0 0 24 24',
      fill: 'none',
      stroke: 'currentColor',
      'stroke-width': 2,
      'stroke-linecap': 'round',
      'stroke-linejoin': 'round'
    }, [
      h('path', { d: 'M8 3H5a2 2 0 0 0-2 2v3m18 0V5a2 2 0 0 0-2-2h-3m0 18h3a2 2 0 0 0 2-2v-3M3 16v3a2 2 0 0 0 2 2h3' })
    ])
  }
}

const ConnectionIcon = {
  name: 'ConnectionIcon',
  render() {
    return h('svg', {
      viewBox: '0 0 24 24',
      fill: 'none',
      stroke: 'currentColor',
      'stroke-width': 2,
      'stroke-linecap': 'round',
      'stroke-linejoin': 'round'
    }, [
      h('circle', { cx: '18', cy: '5', r: '3' }),
      h('circle', { cx: '6', cy: '12', r: '3' }),
      h('circle', { cx: '18', cy: '19', r: '3' }),
      h('line', { x1: '8.59', y1: '13.51', x2: '15.42', y2: '17.49' }),
      h('line', { x1: '15.41', y1: '6.51', x2: '8.59', y2: '10.49' })
    ])
  }
}

// ========================================
// Types
// ========================================

interface RelationDisplay {
  id: string
  label?: string
  type?: string
  description?: string
  strength?: number
  from?: string
  to?: string
  source?: string
  target?: string
}

interface VisNode {
  id: string
  label: string
  color?: string
  size?: number
  attributes?: string[]
  chunks?: string[]
  entity_type?: string
}

interface VisEdge {
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
}

interface VisGraphData {
  nodes: VisNode[]
  edges: VisEdge[]
}

// ========================================
// State
// ========================================

const route = useRoute()
const kbId = ref<string>(route.params.kbId as string)

// References
const graphContainer = ref<HTMLElement>()

// Data state
const graphData = ref<VisGraphData>({ nodes: [], edges: [] })
const loading = ref(false)
const searchText = ref('')
const selectedNode = ref<VisNode | null>(null)
const nodeRelations = ref<RelationDisplay[]>([])

// Drawer
const detailDrawerVisible = ref(false)

// Add Node Dialog
const addNodeDialogVisible = ref(false)
const addNodeForm = ref({
  name: '',
  entity_type: 'concept',
  attributesStr: ''
})

// Add Relation Dialog
const addRelationDialogVisible = ref(false)
const addRelationForm = ref({
  source: '',
  target: '',
  type: 'relates',
  strength: 5
})

const adding = ref(false)
const deleting = ref(false)

// Relation type options
const relationTypeOptions = ref<RelationTypeOption[]>([])

// Edit Node Dialog
const editNodeDialogVisible = ref(false)
const editNodeForm = ref({
  id: '',
  name: '',
  entity_type: 'concept',
  attributesStr: ''
})

// Edit Relation Dialog
const editRelationDialogVisible = ref(false)
const editRelationForm = ref({
  id: '',
  type: 'relates',
  strength: 5
})

const editing = ref(false)

// Confirm Dialog
const confirmVisible = ref(false)
const confirmTitle = ref('')
const confirmMessage = ref('')
const confirmCallback = ref<(() => void) | null>(null)

// vis-network instance
let network: any = null

// ========================================
// Computed
// ========================================

const nodeCount = computed(() => graphData.value.nodes?.length || 0)
const edgeCount = computed(() => graphData.value.edges?.length || 0)
const entityTypeCount = computed(() => {
  const types = new Set(graphData.value.nodes?.map(n => n.entity_type) || [])
  return types.size
})

const selectedNodeType = computed(() => selectedNode.value?.entity_type || '-')
const selectedNodeAttributes = computed(() => selectedNode.value?.attributes || [])
const selectedNodeChunks = computed(() => selectedNode.value?.chunks || [])

// Node options for select dropdown
const nodeOptions = computed(() =>
  graphData.value.nodes.map(n => ({ label: n.label, value: n.id }))
)

// Entity type options
const entityTypeOptions = [
  { label: '人物', value: 'person' },
  { label: '组织', value: 'organization' },
  { label: '地点', value: 'location' },
  { label: '概念', value: 'concept' },
  { label: '其他', value: 'other' }
]

// Strength marks for slider
const strengthMarks = [
  { value: 1, label: '1' },
  { value: 5, label: '5' },
  { value: 10, label: '10' }
]

// ========================================
// Color Mapping
// ========================================

const typeColorMap: Record<string, string> = {
  Department: '#5B8FF9',
  Module: '#F4664A',
  Concept: '#722ED1',
  Technology: '#06D177',
  Product: '#FADB14',
  Company: '#E6A23C',
  Other: '#909399',
  person: '#5B8FF9',
  organization: '#F4664A',
  location: '#06D177',
  concept: '#722ED1',
  other: '#909399'
}

// ========================================
// Methods
// ========================================

function initGraph() {
  console.log('[initGraph] 开始初始化 vis-network 图谱')
  if (!graphContainer.value) {
    console.error('[initGraph] graphContainer 为空')
    toastError('图谱容器未找到')
    return
  }

  const container = graphContainer.value as HTMLElement

  const options = {
    nodes: {
      shape: 'dot',
      size: 16,
      font: {
        size: 14,
        color: '#303133'
      },
      borderWidth: 2,
      shadow: true
    },
    edges: {
      width: 2,
      color: {
        color: '#848484',
        highlight: '#22d3ee',
        hover: '#22d3ee'
      },
      arrows: {
        to: {
          enabled: true,
          scaleFactor: 1,
          type: 'arrow'
        }
      },
      smooth: {
        enabled: true,
        type: 'continuous',
        roundness: 0.5
      },
      font: {
        size: 14,
        color: '#22d3ee',
        background: 'rgba(255, 255, 255, 0.9)',
        strokeWidth: 0,
        align: 'middle'
      },
      labelHighlightBold: true
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
      stabilization: {
        iterations: 150
      }
    },
    interaction: {
      hover: true,
      tooltipDelay: 200,
      zoomView: true,
      dragView: true
    }
  }

  try {
    const nodesDataSet = new DataSet(graphData.value.nodes)
    const edgesDataSet = new DataSet(graphData.value.edges)

    network = new Network(container, { nodes: nodesDataSet, edges: edgesDataSet }, options)

    console.log('[initGraph] vis-network 创建成功')

    network.on('click', (params: any) => {
      if (params.nodes.length > 0) {
        const nodeId = params.nodes[0]
        const node = graphData.value.nodes.find(n => n.id === nodeId)
        if (node) {
          selectedNode.value = node
          detailDrawerVisible.value = true
          loadNodeRelations(node.id)
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
    console.error('[initGraph] vis-network 初始化异常:', error)
    toastError(`图谱初始化失败: ${error}`)
  }
}

async function loadGraph() {
  console.log('[loadGraph] 开始加载图谱数据')
  loading.value = true
  try {
    const res = await graphApi.getGraph(kbId.value)
    console.log('[loadGraph] API 响应:', res)

    if (res.data) {
      graphData.value = convertToVisData(res.data)
      console.log('[loadGraph] 转换后的 vis-network 数据:', graphData.value)

      if (network) {
        network.destroy()
        network = null
      }
      initGraph()
    }
  } catch (error: any) {
    console.error('[loadGraph] 异常:', error)
    toastError(`加载图谱失败: ${error?.message || error}`)
  } finally {
    loading.value = false
  }
}

function convertToVisData(data: GraphData): VisGraphData {
  const rawNodes = data.Node || data.nodes || []
  const rawEdges = data.Relation || data.relations || []

  const nodes: VisNode[] = rawNodes.map(node => {
    const entityType = node.entity_type || 'Other'
    return {
      id: node.id,
      label: node.name,
      color: typeColorMap[entityType] || typeColorMap['Other'],
      size: 20,
      attributes: node.attributes || [],
      chunks: node.chunks || [],
      entity_type: entityType
    }
  })

  const nodeNameToId = new Map<string, string>()
  nodes.forEach(node => {
    nodeNameToId.set(node.label, node.id)
  })

  const edges: VisEdge[] = rawEdges.map(rel => {
    let fromId = rel.source
    let toId = rel.target

    const fromNode = nodes.find(n => n.id === rel.source || n.label === rel.source)
    const toNode = nodes.find(n => n.id === rel.target || n.label === rel.target)

    if (fromNode) fromId = fromNode.id
    if (toNode) toId = toNode.id

    return {
      id: rel.id,
      from: fromId,
      to: toId,
      label: rel.type,
      color: getEdgeColor(rel.weight || 5),
      width: Math.max(2, (rel.weight || 5) / 2),
      description: rel.description || '',
      strength: rel.strength || 0,
      type: rel.type,
      source: rel.source,
      target: rel.target
    }
  })

  return { nodes, edges }
}

function getEdgeColor(weight: number): string {
  if (weight >= 8) return '#F4664A'
  if (weight >= 5) return '#E6A23C'
  if (weight >= 3) return '#FADB14'
  return '#91d5ff'
}

async function loadNodeRelations(nodeId: string) {
  try {
    const relations = graphData.value.edges.filter((edge: VisEdge) => {
      return edge.from === nodeId || edge.to === nodeId
    })

    const getNodeName = (nodeId: string) => {
      const node = graphData.value.nodes.find(n => n.id === nodeId)
      return node?.label || nodeId
    }

    nodeRelations.value = relations.map((rel: VisEdge) => ({
      id: rel.id,
      label: rel.label || rel.type,
      type: rel.type,
      description: rel.description || '-',
      strength: rel.strength || 0,
      from: getNodeName(rel.from),
      to: getNodeName(rel.to),
      source: rel.source || getNodeName(rel.from),
      target: rel.target || getNodeName(rel.to)
    }))
  } catch (error: any) {
    console.error('[loadNodeRelations] 加载失败:', error)
    toastError('加载节点关系失败')
  }
}

async function handleSearch() {
  if (!searchText.value.trim()) {
    loadGraph()
    return
  }

  loading.value = true
  try {
    const res = await graphApi.searchNode(kbId.value, { nodes: [searchText.value] })
    if (res.data) {
      graphData.value = convertToVisData(res.data)
      if (network) {
        network.destroy()
        network = null
      }
      initGraph()
    }
  } catch (error: any) {
    toastError(error.message || '搜索失败')
  } finally {
    loading.value = false
  }
}

function showAddNodeDialog() {
  addNodeForm.value = {
    name: '',
    entity_type: 'concept',
    attributesStr: ''
  }
  addNodeDialogVisible.value = true
}

async function handleAddNode() {
  if (!addNodeForm.value.name) {
    toastWarning('请输入节点名称')
    return
  }

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
      await loadGraph()
    }
  } catch (error: any) {
    toastError(error.message || '添加节点失败')
  } finally {
    adding.value = false
  }
}

function showAddRelationDialog() {
  addRelationForm.value = {
    source: '',
    target: '',
    type: 'relates',
    strength: 5
  }
  addRelationDialogVisible.value = true
}

async function handleAddRelation() {
  if (!addRelationForm.value.source || !addRelationForm.value.target) {
    toastWarning('请选择源节点和目标节点')
    return
  }

  adding.value = true
  try {
    const data: AddRelationRequest = {
      source: addRelationForm.value.source,
      target: addRelationForm.value.target,
      type: addRelationForm.value.type,
      strength: addRelationForm.value.strength
    }

    const res = await graphApi.addRelation(kbId.value, data)
    if (res.data) {
      toastSuccess('关系添加成功')
      addRelationDialogVisible.value = false
      await loadGraph()
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
    id: selectedNode.value.id,
    name: selectedNode.value.label,
    entity_type: selectedNode.value.entity_type || 'concept',
    attributesStr: selectedNode.value.attributes?.join(', ') || ''
  }
  editNodeDialogVisible.value = true
}

async function handleEditNode() {
  if (!editNodeForm.value.name) {
    toastWarning('请输入节点名称')
    return
  }

  editing.value = true
  try {
    const attributes = editNodeForm.value.attributesStr
      ? editNodeForm.value.attributesStr.split(',').map(s => s.trim())
      : []

    const data: UpdateNodeRequest = {
      name: editNodeForm.value.name,
      title: editNodeForm.value.name,
      entity_type: editNodeForm.value.entity_type,
      attributes
    }

    const res = await graphApi.updateNode(kbId.value, editNodeForm.value.id, data)
    if (res.data) {
      toastSuccess('节点更新成功')
      editNodeDialogVisible.value = false
      detailDrawerVisible.value = false
      await loadGraph()
    }
  } catch (error: any) {
    toastError(error.message || '更新节点失败')
  } finally {
    editing.value = false
  }
}

function showEditRelationDialog(rel: RelationDisplay) {
  editRelationForm.value = {
    id: rel.id,
    type: rel.type || rel.label || 'relates',
    strength: rel.strength || 5
  }
  editRelationDialogVisible.value = true
}

async function handleEditRelation() {
  if (!editRelationForm.value.type) {
    toastWarning('请选择关系类型')
    return
  }

  editing.value = true
  try {
    const originalRelation = nodeRelations.value.find(r => r.id === editRelationForm.value.id)
    const originalDescription = originalRelation?.description && originalRelation.description !== '-'
      ? originalRelation.description
      : ''

    const data: UpdateRelationRequest = {
      type: editRelationForm.value.type,
      description: originalDescription,
      strength: editRelationForm.value.strength
    }

    const res = await graphApi.updateRelation(kbId.value, editRelationForm.value.id, data)
    if (res.data) {
      toastSuccess('关系更新成功')
      editRelationDialogVisible.value = false
      detailDrawerVisible.value = false
      await loadGraph()
    }
  } catch (error: any) {
    toastError(error.message || '更新关系失败')
  } finally {
    editing.value = false
  }
}

function handleExport() {
  if (!graphData.value.nodes?.length) {
    toastWarning('没有可导出的数据')
    return
  }

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
    if (res.data) {
      relationTypeOptions.value = res.data
    }
  } catch (error: any) {
    console.error('[loadRelationTypes] 加载失败:', error)
    relationTypeOptions.value = [
      { value: 'contains', label: '包含' },
      { value: 'relates', label: '关联' },
      { value: 'depends', label: '依赖' },
      { value: 'belongs', label: '属于' },
      { value: 'owns', label: '拥有' },
      { value: 'author', label: '作者' },
      { value: 'alias', label: '别名' },
      { value: 'other', label: '其他' }
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
      await loadGraph()
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
      await loadGraph()
    } catch (error: any) {
      toastError(error.message || '删除关系失败')
    } finally {
      deleting.value = false
    }
  }
  confirmVisible.value = true
}

function handleConfirm() {
  if (confirmCallback.value) {
    confirmCallback.value()
    confirmCallback.value = null
  }
}

function toggleFullscreen() {
  if (!document.fullscreenElement) {
    graphContainer.value?.requestFullscreen()
  } else {
    document.exitFullscreen()
  }
}

// ========================================
// Lifecycle
// ========================================

onMounted(async () => {
  await nextTick()
  if (!graphContainer.value) {
    console.error('[GraphView] graphContainer 未找到')
    return
  }
  await loadGraph()
  await loadRelationTypes()
})

onUnmounted(() => {
  if (network) {
    network.destroy()
    network = null
  }
})
</script>

<style scoped>
.graph-view-page {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: transparent;
}

.graph-view-container {
  display: flex;
  flex-direction: column;
  flex: 1;
  background: rgba(0, 0, 0, 0.3);
  border-radius: var(--radius-xl);
  padding: var(--space-6);
  overflow: hidden;
  backdrop-filter: blur(10px);
}

.toolbar {
  display: flex;
  align-items: center;
  gap: var(--space-4);
  margin-bottom: var(--space-4);
}

.search-box {
  width: 300px;
}

.action-buttons {
  display: flex;
  gap: var(--space-2);
  margin-left: auto;
}

.graph-container {
  flex: 1;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: var(--radius-lg);
  overflow: hidden;
  background: rgba(0, 0, 0, 0.5);
  min-height: 500px;
  position: relative;
}

.graph-loader {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.5);
}

.status-bar {
  display: flex;
  align-items: center;
  gap: var(--space-4);
  padding: var(--space-4) var(--space-6);
  background: rgba(0, 0, 0, 0.5);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: var(--radius-lg);
  margin-top: var(--space-4);
  backdrop-filter: blur(10px);
}

.status-item {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.status-label {
  font-size: var(--text-sm);
  color: var(--text-muted);
}

.status-value {
  font-size: var(--text-sm);
  font-weight: 600;
  color: var(--text-primary);
}

.status-divider {
  width: 1px;
  height: 16px;
  background: var(--border-subtle);
}

/* Node Detail Drawer */
.node-detail {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
}

.detail-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.detail-title {
  margin: 0;
  font: var(--font-display);
  font-size: var(--text-base);
  font-weight: 600;
  color: var(--text-primary);
}

.detail-row {
  display: flex;
  align-items: flex-start;
  gap: var(--space-4);
}

.detail-label {
  min-width: 80px;
  font-size: var(--text-sm);
  color: var(--text-secondary);
}

.detail-value {
  flex: 1;
  font-size: var(--text-sm);
  color: var(--text-primary);
}

.detail-tags {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.detail-tag {
  padding: var(--space-1) var(--space-3);
  background: rgba(255, 255, 255, 0.1);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: var(--radius-full);
  font-size: var(--text-xs);
  color: var(--text-secondary);
}

.detail-empty {
  color: var(--text-muted);
}

.detail-actions {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

/* Relations List */
.relations-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.relation-item {
  padding: var(--space-4);
  background: rgba(0, 0, 0, 0.3);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: var(--radius-lg);
  backdrop-filter: blur(10px);
}

.relation-header {
  display: flex;
  gap: var(--space-2);
  margin-bottom: var(--space-2);
}

.relation-type {
  padding: var(--space-1) var(--space-2);
  background: rgba(34, 211, 238, 0.2);
  border-radius: var(--radius-sm);
  font-size: var(--text-xs);
  font-weight: 500;
  color: var(--primary);
}

.relation-strength {
  padding: var(--space-1) var(--space-2);
  background: rgba(255, 255, 255, 0.1);
  border-radius: var(--radius-sm);
  font-size: var(--text-xs);
  color: var(--text-muted);
}

.relation-content {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin-bottom: var(--space-2);
  font-weight: 500;
  font-size: var(--text-sm);
  color: var(--text-primary);
}

.relation-source,
.relation-target {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.relation-arrow {
  flex-shrink: 0;
  width: 16px;
  height: 16px;
  color: var(--primary);
}

.relation-description {
  margin-bottom: var(--space-3);
  font-size: var(--text-sm);
  color: var(--text-muted);
  line-height: 1.5;
}

.relation-actions {
  display: flex;
  gap: var(--space-2);
}

.relations-empty {
  padding: var(--space-8);
  text-align: center;
  color: var(--text-muted);
  font-size: var(--text-sm);
}

/* Form */
.form-group {
  margin-bottom: var(--space-4);
}

.form-group:last-child {
  margin-bottom: 0;
}

/* Responsive */
@media (max-width: 768px) {
  .toolbar {
    flex-wrap: wrap;
  }

  .search-box {
    width: 100%;
  }

  .action-buttons {
    width: 100%;
    justify-content: space-between;
  }
}
</style>
