<template>
  <div class="gv-page">

    <!-- ===== 顶部工具栏 ===== -->
    <div class="gv-topbar">
      <span class="gv-topbar__title">知识图谱</span>
      <nav class="gv-breadcrumb">
        <span>知识库</span>
        <span class="gv-breadcrumb__sep">/</span>
        <span>{{ kbId }}</span>
        <span class="gv-breadcrumb__sep">/</span>
        <span class="gv-breadcrumb__cur">图谱</span>
      </nav>

      <div class="gv-topbar__right">
        <!-- 搜索实体 -->
        <div class="gv-search" @click="focusSearch">
          <svg class="gv-search__icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="11" cy="11" r="7"/><path d="M21 21l-4.3-4.3"/>
          </svg>
          <input
            ref="searchInputRef"
            v-model="searchText"
            class="gv-search__input"
            placeholder="搜索实体…"
            @keyup.enter="handleSearch"
          />
        </div>

        <div class="gv-topbar__divider"></div>

        <!-- 添加节点 -->
        <button class="gv-btn-sec" @click="showAddNodeDialog">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="4"/><path d="M12 2v4M12 18v4M2 12h4M18 12h4"/>
          </svg>
          添加节点
        </button>

        <!-- 添加关系 -->
        <button class="gv-btn-sec" @click="showAddRelationDialog">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M5 12h14M14 7l5 5-5 5"/>
          </svg>
          添加关系
        </button>

        <div class="gv-topbar__divider"></div>

        <!-- 导出 -->
        <button class="gv-icon-btn" title="导出" @click="handleExport">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
            <polyline points="7 10 12 15 17 10"/>
            <line x1="12" y1="15" x2="12" y2="3"/>
          </svg>
        </button>

        <!-- 全屏 -->
        <button class="gv-icon-btn" title="全屏" @click="toggleFullscreen">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="M8 3H5a2 2 0 0 0-2 2v3M21 8V5a2 2 0 0 0-2-2h-3M3 16v3a2 2 0 0 0 2 2h3M16 21h3a2 2 0 0 0 2-2v-3"/>
          </svg>
        </button>
      </div>
    </div>

    <!-- ===== 主体：画布 + 详情抽屉 ===== -->
    <div class="gv-body">

      <!-- 图谱画布区 -->
      <div class="gv-canvas-wrap">
        <div ref="graphContainer" class="gv-canvas">
          <div v-if="loading" class="gv-loader">
            <UiLoader size="lg" />
          </div>
        </div>

        <!-- 图例浮层（左下角） -->
        <div class="gv-legend">
          <div class="gv-legend__title">ENTITY TYPE</div>
          <div
            v-for="item in legendItems"
            :key="item.type"
            class="gv-legend__row"
          >
            <span class="gv-legend__dot" :style="{ background: item.color }"></span>
            <span>{{ item.label }}</span>
          </div>
        </div>

        <!-- 缩放控件（右下角） -->
        <div class="gv-zoom">
          <button class="gv-icon-btn" title="放大" @click="zoomIn">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="11" cy="11" r="7"/><path d="M21 21l-4.3-4.3"/>
              <path d="M8 11h6M11 8v6"/>
            </svg>
          </button>
          <button class="gv-icon-btn" title="缩小" @click="zoomOut">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="11" cy="11" r="7"/><path d="M21 21l-4.3-4.3"/>
              <path d="M8 11h6"/>
            </svg>
          </button>
          <button class="gv-icon-btn" title="适应屏幕" @click="fitGraph">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M8 3H5a2 2 0 0 0-2 2v3M21 8V5a2 2 0 0 0-2-2h-3M3 16v3a2 2 0 0 0 2 2h3M16 21h3a2 2 0 0 0 2-2v-3"/>
            </svg>
          </button>
        </div>
      </div>

      <!-- 节点详情抽屉（右侧，宽 300px，选中节点时显示） -->
      <aside v-if="detailDrawerVisible" class="gv-drawer">
        <div class="gv-drawer__head">
          <span class="gv-drawer__title">节点详情</span>
          <button class="gv-icon-btn" title="关闭" @click="detailDrawerVisible = false">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M18 6L6 18M6 6l12 12"/>
            </svg>
          </button>
        </div>

        <div v-if="selectedNode" class="gv-drawer__body">

          <!-- 基本信息 -->
          <div class="gv-info-section">
            <div class="gv-info-row">
              <span class="gv-info-label">名称</span>
              <span class="gv-info-value gv-info-value--accent">{{ selectedNode.label }}</span>
            </div>
            <div class="gv-info-row">
              <span class="gv-info-label">类型</span>
              <span class="gv-info-value">{{ selectedNodeType }}</span>
            </div>
            <div class="gv-info-row">
              <span class="gv-info-label">属性</span>
              <span class="gv-info-value gv-info-value--mono gv-info-value--muted">
                {{ selectedNodeAttributes.length > 0 ? selectedNodeAttributes.join(', ') : '-' }}
              </span>
            </div>
            <div class="gv-info-row">
              <span class="gv-info-label">关联分块数</span>
              <span class="gv-info-value gv-info-value--mono">{{ selectedNodeChunks.length }}</span>
            </div>
          </div>

          <!-- 关联关系 -->
          <div class="gv-section-head">
            关联关系
            <span class="gv-section-head__count">{{ nodeRelations.length }}</span>
          </div>
          <div class="gv-rel-list">
            <div
              v-for="rel in nodeRelations"
              :key="rel.id"
              class="gv-rel-row"
            >
              <span class="gv-rel-type">{{ rel.label || rel.type || '-' }}</span>
              <span class="gv-rel-arrow">→</span>
              <span class="gv-rel-target">{{ rel.target || rel.to || '-' }}</span>
              <div class="gv-rel-actions">
                <button class="gv-rel-btn" @click="showEditRelationDialog(rel)">编辑</button>
                <button class="gv-rel-btn gv-rel-btn--danger" @click="handleDeleteRelation(rel)">删除</button>
              </div>
            </div>
            <div v-if="nodeRelations.length === 0" class="gv-rel-empty">
              暂无关联关系
            </div>
          </div>

        </div>

        <!-- 底部操作 -->
        <div class="gv-drawer__footer">
          <button class="gv-btn-primary" @click="showEditNodeDialog">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="13" height="13">
              <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/>
              <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/>
            </svg>
            编辑节点
          </button>
          <button class="gv-btn-danger" @click="handleDeleteNode" :disabled="deleting">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="13" height="13">
              <polyline points="3 6 5 6 21 6"/>
              <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
            </svg>
            删除节点
          </button>
        </div>
      </aside>
    </div>

    <!-- ===== 底部状态栏 ===== -->
    <div class="gv-statusbar">
      <div class="gv-statusbar__left">
        <span class="gv-stat-item">节点 <span class="gv-stat-val">{{ nodeCount }}</span></span>
        <span class="gv-stat-item">关系 <span class="gv-stat-val">{{ edgeCount }}</span></span>
        <span class="gv-stat-item">实体类型 <span class="gv-stat-val">{{ entityTypeCount }}</span></span>
        <span class="gv-stat-item">布局 <span class="gv-stat-val">force</span></span>
        <span class="gv-stat-item">渲染 <span class="gv-stat-val">60fps</span></span>
      </div>
      <div class="gv-statusbar__right">
        <span class="gv-status-dot"><i></i></span>
        最后同步 <span class="gv-stat-val">{{ lastSyncTime }}</span>
      </div>
    </div>

    <!-- ===== 添加节点 Modal ===== -->
    <UiModal v-model="addNodeDialogVisible" title="添加节点" size="sm">
      <div class="gv-form-group">
        <UiInput v-model="addNodeForm.name" label="节点名称" placeholder="请输入节点名称" required />
      </div>
      <div class="gv-form-group">
        <UiSelect v-model="addNodeForm.entity_type" label="实体类型" :options="entityTypeOptions" placeholder="选择类型" />
      </div>
      <div class="gv-form-group">
        <UiInput v-model="addNodeForm.attributesStr" label="属性" type="text" placeholder="多个属性用逗号分隔" />
      </div>
      <template #footer>
        <UiButton variant="secondary" @click="addNodeDialogVisible = false">取消</UiButton>
        <UiButton variant="primary" @click="handleAddNode" :loading="adding">确定</UiButton>
      </template>
    </UiModal>

    <!-- ===== 添加关系 Modal ===== -->
    <UiModal v-model="addRelationDialogVisible" title="添加关系" size="sm">
      <div class="gv-form-group">
        <UiSelect v-model="addRelationForm.source" label="源节点" :options="nodeOptions" placeholder="选择源节点" filterable required />
      </div>
      <div class="gv-form-group">
        <UiSelect v-model="addRelationForm.target" label="目标节点" :options="nodeOptions" placeholder="选择目标节点" filterable required />
      </div>
      <div class="gv-form-group">
        <UiSelect v-model="addRelationForm.type" label="关系类型" :options="relationTypeOptions" placeholder="选择关系类型" required />
      </div>
      <div class="gv-form-group">
        <UiSlider v-model="addRelationForm.strength" label="强度" :min="1" :max="10" :marks="strengthMarks" :format-value="(v: number) => `当前值: ${v}`" />
      </div>
      <template #footer>
        <UiButton variant="secondary" @click="addRelationDialogVisible = false">取消</UiButton>
        <UiButton variant="primary" @click="handleAddRelation" :loading="adding">确定</UiButton>
      </template>
    </UiModal>

    <!-- ===== 编辑节点 Modal ===== -->
    <UiModal v-model="editNodeDialogVisible" title="编辑节点" size="sm">
      <div class="gv-form-group">
        <UiInput v-model="editNodeForm.name" label="节点名称" placeholder="请输入节点名称" required />
      </div>
      <div class="gv-form-group">
        <UiSelect v-model="editNodeForm.entity_type" label="实体类型" :options="entityTypeOptions" placeholder="选择类型" />
      </div>
      <div class="gv-form-group">
        <UiInput v-model="editNodeForm.attributesStr" label="属性" type="text" placeholder="多个属性用逗号分隔" />
      </div>
      <template #footer>
        <UiButton variant="secondary" @click="editNodeDialogVisible = false">取消</UiButton>
        <UiButton variant="primary" @click="handleEditNode" :loading="editing">确定</UiButton>
      </template>
    </UiModal>

    <!-- ===== 编辑关系 Modal ===== -->
    <UiModal v-model="editRelationDialogVisible" title="编辑关系" size="sm">
      <div class="gv-form-group">
        <UiSelect v-model="editRelationForm.type" label="关系类型" :options="relationTypeOptions" placeholder="选择关系类型" required />
      </div>
      <div class="gv-form-group">
        <UiSlider v-model="editRelationForm.strength" label="强度" :min="1" :max="10" :marks="strengthMarks" :format-value="(v: number) => `当前值: ${v}`" />
      </div>
      <template #footer>
        <UiButton variant="secondary" @click="editRelationDialogVisible = false">取消</UiButton>
        <UiButton variant="primary" @click="handleEditRelation" :loading="editing">确定</UiButton>
      </template>
    </UiModal>

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

import { toastSuccess, toastError, toastWarning } from '@/utils/toast'

// UI Components
import UiButton from '@/components/ui/UiButton.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import UiSlider from '@/components/ui/UiSlider.vue'
import UiModal from '@/components/ui/UiModal.vue'
import UiConfirm from '@/components/ui/UiConfirm.vue'
import UiLoader from '@/components/ui/UiLoader.vue'

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

const graphContainer = ref<HTMLElement>()
const searchInputRef = ref<HTMLInputElement>()

const graphData = ref<VisGraphData>({ nodes: [], edges: [] })
const loading = ref(false)
const searchText = ref('')
const selectedNode = ref<VisNode | null>(null)
const nodeRelations = ref<RelationDisplay[]>([])

const detailDrawerVisible = ref(false)

const addNodeDialogVisible = ref(false)
const addNodeForm = ref({ name: '', entity_type: 'concept', attributesStr: '' })

const addRelationDialogVisible = ref(false)
const addRelationForm = ref({ source: '', target: '', type: 'relates', strength: 5 })

const adding = ref(false)
const deleting = ref(false)

const relationTypeOptions = ref<RelationTypeOption[]>([])

const editNodeDialogVisible = ref(false)
const editNodeForm = ref({ id: '', name: '', entity_type: 'concept', attributesStr: '' })

const editRelationDialogVisible = ref(false)
const editRelationForm = ref({ id: '', type: 'relates', strength: 5 })

const editing = ref(false)

const confirmVisible = ref(false)
const confirmTitle = ref('')
const confirmMessage = ref('')
const confirmCallback = ref<(() => void) | null>(null)

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

// ========================================
// 月夜台账配色 — 钢青灰分族（取自设计稿）
// Canvas/D3 内部取不到 CSS 变量，直接写字面色值
// ========================================

/**
 * 实体类型 → 钢青灰主色族（按顺序循环）
 * #62758a 服务
 * #56687c 技术组件
 * #4a5b6e 存储
 * #3d4a5c 文档/其他
 */
const TYPE_PALETTE: string[] = [
  '#62758a',
  '#56687c',
  '#4a5b6e',
  '#3d4a5c'
]

// 已知实体类型固定映射
const typeColorMap: Record<string, string> = {
  // 服务类
  Department:   '#62758a',
  person:       '#62758a',
  organization: '#62758a',
  // 技术组件类
  Module:       '#56687c',
  Technology:   '#56687c',
  concept:      '#56687c',
  // 存储类
  Product:      '#4a5b6e',
  location:     '#4a5b6e',
  // 文档/其他
  Other:        '#3d4a5c',
  other:        '#3d4a5c'
}

// 动态分配：未在上表中的类型按出现顺序取 palette
const _dynamicTypeMap: Record<string, string> = {}
let _paletteIdx = 0

function getTypeColor(entityType: string): string {
  if (typeColorMap[entityType]) return typeColorMap[entityType]
  if (_dynamicTypeMap[entityType]) return _dynamicTypeMap[entityType]
  const color = TYPE_PALETTE[_paletteIdx % TYPE_PALETTE.length]
  _paletteIdx++
  _dynamicTypeMap[entityType] = color
  return color
}

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
  if (result.length === 0) {
    // 占位：设计稿的四族
    return [
      { type: 'service',    label: '服务',     color: '#62758a' },
      { type: 'component',  label: '技术组件', color: '#56687c' },
      { type: 'storage',    label: '存储',     color: '#4a5b6e' },
      { type: 'doc',        label: '文档',     color: '#3d4a5c' }
    ]
  }
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
    const nodesDataSet = new DataSet(graphData.value.nodes)
    const edgesDataSet = new DataSet(graphData.value.edges)

    network = new Network(container, { nodes: nodesDataSet, edges: edgesDataSet }, options)

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
      color: {
        background: baseColor,
        border: baseColor,
        highlight: {
          background: '#9cb4cd',
          border: '#9cb4cd'
        },
        hover: {
          background: baseColor,
          border: '#7f92a8'
        }
      } as any,
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
      target:      rel.target
    }
  })

  return { nodes, edges }
}

async function loadNodeRelations(nodeId: string) {
  try {
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
      target:      rel.target || getNodeName(rel.to)
    }))
  } catch (error: any) {
    toastError('加载节点关系失败')
  }
}

// ========================================
// User Actions
// ========================================

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

function focusSearch() {
  searchInputRef.value?.focus()
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
      await loadGraph()
    }
  } catch (error: any) {
    toastError(error.message || '添加节点失败')
  } finally {
    adding.value = false
  }
}

function showAddRelationDialog() {
  addRelationForm.value = { source: '', target: '', type: 'relates', strength: 5 }
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
      await loadGraph()
    }
  } catch (error: any) {
    toastError(error.message || '更新关系失败')
  } finally {
    editing.value = false
  }
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
  if (confirmCallback.value) { confirmCallback.value(); confirmCallback.value = null }
}

function toggleFullscreen() {
  if (!document.fullscreenElement) {
    graphContainer.value?.requestFullscreen()
  } else {
    document.exitFullscreen()
  }
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

onMounted(async () => {
  await nextTick()
  if (!graphContainer.value) return
  await loadGraph()
  await loadRelationTypes()
})

onUnmounted(() => {
  if (network) { network.destroy(); network = null }
})
</script>

<style scoped>
/* ===================================================
   知识图谱页 — 月夜台账视觉规范
   只用 global.css 中定义的 CSS 变量；
   图渲染引擎内部（vis-network canvas）直接写字面色值
   红线：无 linear-gradient/backdrop-filter/发光 box-shadow/emoji
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

/* ===== 顶部工具栏 ===== */
.gv-topbar {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 0 20px;
  height: 54px;
  flex: 0 0 54px;
  /* 顶栏分隔线与 mockup 一致：发丝线 05% */
  border-bottom: 1px solid var(--color-border-subtle);
}

.gv-topbar__title {
  font-family: var(--font-display);
  font-size: 16px;
  font-weight: 700;
  color: var(--color-text-primary);
  white-space: nowrap;
}

.gv-breadcrumb {
  display: flex;
  align-items: center;
  gap: 5px;
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--color-text-muted);
}

.gv-breadcrumb__sep {
  color: var(--color-border-default);
}

.gv-breadcrumb__cur {
  color: var(--color-text-secondary);
}

.gv-topbar__right {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 8px;
}

/* 搜索框 */
.gv-search {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 6px 11px;
  border-radius: var(--radius-sm);
  background: var(--color-bg-primary);
  border: 1px solid var(--color-border-subtle);
  width: 180px;
  cursor: text;
  transition: border-color var(--duration-fast) var(--ease-default);
}

.gv-search:focus-within {
  border-color: var(--color-border-default);
}

.gv-search__icon {
  width: 13px;
  height: 13px;
  flex: 0 0 13px;
  color: var(--color-text-muted);
}

.gv-search__input {
  flex: 1;
  background: transparent;
  border: none;
  outline: none;
  font-size: 12px;
  font-family: var(--font-mono);
  color: var(--color-text-secondary);
  min-width: 0;
}

.gv-search__input::placeholder {
  color: var(--color-text-muted);
}

/* 顶栏分隔线 */
.gv-topbar__divider {
  width: 1px;
  height: 18px;
  background: var(--color-border-default);
  flex: 0 0 1px;
}

/* hairline 次级按钮（添加节点/关系）：border 对应 mockup var(--hair) = --border-default */
.gv-btn-sec {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 13px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--color-border-default);
  background: transparent;
  color: var(--color-text-secondary);
  font-size: 12px;
  cursor: pointer;
  white-space: nowrap;
  transition:
    color var(--duration-fast) var(--ease-default),
    border-color var(--duration-fast) var(--ease-default),
    background var(--duration-fast) var(--ease-default);
}

.gv-btn-sec svg {
  width: 13px;
  height: 13px;
}

.gv-btn-sec:hover {
  color: var(--color-text-primary);
  background: var(--color-bg-elevated);
  border-color: var(--color-border-default);
}

/* 图标按钮（30×30） */
.gv-icon-btn {
  width: 30px;
  height: 30px;
  border-radius: var(--radius-sm);
  border: none;
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition:
    color var(--duration-fast) var(--ease-default),
    background var(--duration-fast) var(--ease-default);
}

.gv-icon-btn svg {
  width: 15px;
  height: 15px;
}

.gv-icon-btn:hover {
  color: var(--color-text-primary);
  background: var(--color-bg-elevated);
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

/* 图例浮层（左下角）：mockup 无 shadow，纯 --ink-2 底色 */
.gv-legend {
  position: absolute;
  left: 16px;
  bottom: 16px;
  background: var(--color-bg-tertiary);
  border-radius: var(--radius-sm);
  padding: 10px 13px;
  min-width: 130px;
}

.gv-legend__title {
  font-family: var(--font-mono);
  font-size: 9.5px;
  letter-spacing: 1.2px;
  color: var(--color-text-muted);
  margin-bottom: 8px;
}

.gv-legend__row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 0;
  font-size: 11.5px;
  color: var(--color-text-secondary);
}

.gv-legend__row + .gv-legend__row {
  border-top: 1px solid var(--color-border-subtle);
}

.gv-legend__dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
  flex: 0 0 9px;
}

/* 缩放控件（右下角）：mockup 无 shadow，纯 --ink-2 底色 */
.gv-zoom {
  position: absolute;
  right: 16px;
  bottom: 16px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  background: var(--color-bg-tertiary);
  border-radius: var(--radius-sm);
  padding: 4px;
}

.gv-zoom .gv-icon-btn {
  width: 28px;
  height: 28px;
}

.gv-zoom .gv-icon-btn svg {
  width: 13px;
  height: 13px;
}

/* ===== 节点详情抽屉（右侧固定，300px） ===== */
.gv-drawer {
  width: 300px;
  flex: 0 0 300px;
  border-left: 1px solid var(--color-border-default);
  background: var(--color-bg-tertiary);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.gv-drawer__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px 12px;
  border-bottom: 1px solid var(--color-border-default);
  flex: 0 0 auto;
}

.gv-drawer__title {
  font-family: var(--font-display);
  font-size: 14px;
  font-weight: 700;
  color: var(--color-text-primary);
}

.gv-drawer__body {
  flex: 1;
  overflow-y: auto;
  padding: 0;
}

/* 基本信息 */
.gv-info-section {
  padding: 12px 16px;
  border-bottom: 1px solid var(--color-border-subtle);
}

.gv-info-row {
  display: flex;
  align-items: baseline;
  gap: 10px;
  padding: 6px 0;
  font-size: 12.5px;
}

.gv-info-row + .gv-info-row {
  border-top: 1px solid var(--color-border-subtle);
}

.gv-info-label {
  color: var(--color-text-muted);
  font-size: 11px;
  font-family: var(--font-mono);
  flex: 0 0 70px;
}

.gv-info-value {
  color: var(--color-text-secondary);
  flex: 1;
}

.gv-info-value--accent {
  color: var(--color-primary);
  font-weight: 600;
}

.gv-info-value--mono {
  font-family: var(--font-mono);
  font-size: 11.5px;
}

/* 属性行：比正常值更暗（对应 mockup style="font-size:11px;color:var(--t3)"） */
.gv-info-value--muted {
  color: var(--color-text-muted);
  font-size: 11px;
}

/* 小节标题 */
.gv-section-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 10px 16px 8px;
  font-size: 11.5px;
  font-weight: 600;
  color: var(--color-text-secondary);
  border-bottom: 1px solid var(--color-border-subtle);
}

.gv-section-head__count {
  font-family: var(--font-mono);
  font-size: 10px;
  color: var(--color-text-muted);
  font-weight: 400;
}

/* 关系列表 */
.gv-rel-list {
  padding: 4px 0;
}

.gv-rel-row {
  display: flex;
  align-items: center;
  gap: 0;
  padding: 7px 16px;
  font-size: 12px;
  cursor: pointer;
  transition: background var(--duration-fast) var(--ease-default);
}

.gv-rel-row:hover {
  background: var(--color-bg-elevated);
}

.gv-rel-type {
  font-family: var(--font-mono);
  font-size: 10.5px;
  color: var(--color-text-muted);
  margin-right: 6px;
  min-width: 68px;
}

.gv-rel-arrow {
  color: var(--color-text-muted);
  margin-right: 6px;
  font-family: var(--font-mono);
  font-size: 10px;
}

.gv-rel-target {
  color: var(--color-text-secondary);
  font-size: 12px;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.gv-rel-actions {
  display: flex;
  gap: 4px;
  opacity: 0;
  transition: opacity var(--duration-fast) var(--ease-default);
}

.gv-rel-row:hover .gv-rel-actions {
  opacity: 1;
}

.gv-rel-btn {
  padding: 2px 6px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--color-border-default);
  background: transparent;
  color: var(--color-text-muted);
  font-size: 10px;
  cursor: pointer;
  transition: color var(--duration-fast), background var(--duration-fast);
}

.gv-rel-btn:hover {
  color: var(--color-text-primary);
  background: var(--color-bg-elevated);
}

.gv-rel-btn--danger {
  color: var(--color-danger);
  border-color: var(--color-border-subtle);
}

.gv-rel-btn--danger:hover {
  background: rgba(192, 138, 138, 0.08);
  border-color: var(--color-danger);
}

.gv-rel-empty {
  padding: 24px 16px;
  text-align: center;
  color: var(--color-text-muted);
  font-size: 12px;
}

/* 抽屉底部按钮区 */
.gv-drawer__footer {
  display: flex;
  gap: 8px;
  padding: 12px 16px;
  border-top: 1px solid var(--color-border-default);
  flex: 0 0 auto;
}

/* 银青实心按钮 */
.gv-btn-primary {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 8px;
  border: none;
  border-radius: var(--radius-sm);
  cursor: pointer;
  background: var(--color-primary);
  color: var(--on-primary);
  font-size: 12.5px;
  font-weight: 600;
  transition: background var(--duration-fast) var(--ease-default);
}

.gv-btn-primary:hover {
  background: var(--color-primary-light);
}

/* hairline 危险按钮 */
.gv-btn-danger {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 8px;
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-sm);
  cursor: pointer;
  background: transparent;
  color: var(--color-danger);
  font-size: 12.5px;
  transition:
    border-color var(--duration-fast) var(--ease-default),
    background var(--duration-fast) var(--ease-default);
}

.gv-btn-danger:hover {
  border-color: var(--color-danger);
  background: rgba(192, 138, 138, 0.08);
}

.gv-btn-danger:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* ===== 底部状态栏 ===== */
.gv-statusbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
  height: 32px;
  flex: 0 0 32px;
  /* mockup 使用 var(--hair) = --border-default (9% opacity) */
  border-top: 1px solid var(--color-border-default);
}

.gv-statusbar__left {
  display: flex;
  align-items: center;
  gap: 18px;
  font-family: var(--font-mono);
  font-size: 10.5px;
  color: var(--color-text-muted);
}

.gv-stat-item {
  display: flex;
  align-items: center;
  gap: 5px;
}

.gv-stat-val {
  color: var(--color-text-secondary);
  font-family: var(--font-mono);
}

.gv-statusbar__right {
  display: flex;
  align-items: center;
  gap: 6px;
  font-family: var(--font-mono);
  font-size: 10.5px;
  color: var(--color-text-muted);
}

/* 呼吸状态点 */
.gv-status-dot {
  display: flex;
  align-items: center;
}

.gv-status-dot i {
  display: block;
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--color-success);
  animation: pulse 3s ease-in-out infinite;
}

/* ===== 表单 ===== */
.gv-form-group {
  margin-bottom: var(--space-4);
}

.gv-form-group:last-child {
  margin-bottom: 0;
}

/* ===== 滚动条：与 mockup 一致（4px 细轨道） ===== */
.gv-drawer__body::-webkit-scrollbar {
  width: 4px;
}

.gv-drawer__body::-webkit-scrollbar-track {
  background: transparent;
}

.gv-drawer__body::-webkit-scrollbar-thumb {
  background: rgba(232, 230, 223, 0.14);
  border-radius: 2px;
}

/* 呼吸脉冲动画（本地定义，确保 scoped 作用域可用） */
@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}
</style>
