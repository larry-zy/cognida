<!--
  A2UINode —— 生成式 UI（A2UI 子集）的递归节点渲染器。

  一次渲染一个组件节点：容器（Column/Row）向下递归其 children；叶子组件
  （Text/MetricCard/Callout/Table/LineChart）用 props 渲染。所有数据一律通过
  {path} 绑定从 dataModel 解析（RFC6901 JSON Pointer），组件本身不含任何数字，
  数字全部来自后端装配的真实 sql_execute / data_analysis 结果。

  组件映射与 dataModel 由 A2UIRenderer 通过 provide 注入。
-->
<template>
  <!-- 布局容器 -->
  <div v-if="node.type === 'Column'" class="a2ui-col">
    <A2UINode v-for="cid in node.children || []" :key="cid" :id="cid" />
  </div>
  <div v-else-if="node.type === 'Row'" class="a2ui-row">
    <A2UINode v-for="cid in node.children || []" :key="cid" :id="cid" />
  </div>

  <!-- 文本 -->
  <div v-else-if="node.type === 'Text'" :class="['a2ui-text', `a2ui-text--${prop('variant') || 'body'}`]">
    {{ prop('text') }}
  </div>

  <!-- 指标卡 -->
  <UiCard v-else-if="node.type === 'MetricCard'" class="a2ui-metric" variant="bordered" padding="none">
    <div class="a2ui-metric__label">{{ prop('label') }}</div>
    <div class="a2ui-metric__value">
      {{ formatValue(prop('value')) }}<span v-if="prop('unit')" class="a2ui-metric__unit">{{ prop('unit') }}</span>
    </div>
  </UiCard>

  <!-- 结论条 -->
  <UiAlert
    v-else-if="node.type === 'Callout'"
    class="a2ui-callout"
    :type="toneToType(prop('tone'))"
    :title="String(prop('title') || '')"
  >
    {{ prop('text') }}
  </UiAlert>

  <!-- 数据表：融合真实 sql_execute 行集 -->
  <div v-else-if="node.type === 'Table'" class="a2ui-table">
    <div v-if="prop('title')" class="a2ui-block-title">{{ prop('title') }}</div>
    <UiTable :columns="tableColumns" :data="tableRows" size="sm" border stripe />
  </div>

  <!-- 折线图：自研 SVG（无外部依赖），actual 实线 + forecast 虚线 -->
  <div v-else-if="node.type === 'LineChart'" class="a2ui-chart">
    <div v-if="prop('title')" class="a2ui-block-title">{{ prop('title') }}</div>
    <svg v-if="chart" :viewBox="`0 0 ${chart.w} ${chart.h}`" class="a2ui-chart__svg" preserveAspectRatio="none">
      <!-- 网格基线 -->
      <line :x1="chart.padX" :y1="chart.baseY" :x2="chart.w - chart.padX" :y2="chart.baseY" class="a2ui-chart__axis" />
      <!-- 实测折线 -->
      <polyline :points="chart.actualPoints" class="a2ui-chart__line" />
      <!-- 预测折线（虚线） -->
      <polyline v-if="chart.forecastPoints" :points="chart.forecastPoints" class="a2ui-chart__line a2ui-chart__line--forecast" />
      <!-- 数据点 -->
      <circle v-for="(p, i) in chart.actualDots" :key="'a' + i" :cx="p.x" :cy="p.y" r="2.5" class="a2ui-chart__dot" />
      <circle v-for="(p, i) in chart.forecastDots" :key="'f' + i" :cx="p.x" :cy="p.y" r="2.5" class="a2ui-chart__dot a2ui-chart__dot--forecast" />
    </svg>
    <div v-if="chart" class="a2ui-chart__legend">
      <span class="a2ui-chart__legend-item"><i class="a2ui-chart__swatch"></i>{{ chart.name }}（实测）</span>
      <span v-if="chart.forecastPoints" class="a2ui-chart__legend-item"><i class="a2ui-chart__swatch a2ui-chart__swatch--forecast"></i>预测</span>
    </div>
  </div>

  <!-- 未知类型：静默跳过（catalog 已在后端白名单校验） -->
</template>

<script setup lang="ts">
import { computed, inject } from 'vue'
import UiCard from '@/components/ui/UiCard.vue'
import UiAlert from '@/components/ui/UiAlert.vue'
import UiTable from '@/components/ui/UiTable.vue'
import { A2UI_CTX, type A2UIContext } from './a2ui-context'

const props = defineProps<{ id: string }>()

const ctx = inject<A2UIContext>(A2UI_CTX)!
const node = computed(() => ctx.nodes.value[props.id] || { id: props.id, type: 'Unknown' })

// prop(name)：取组件属性；若为 {path} 绑定则从 dataModel 解析。
function prop(name: string): any {
  const raw = node.value.props?.[name]
  return resolve(raw)
}

function resolve(v: any): any {
  if (v && typeof v === 'object' && typeof v.path === 'string') {
    return resolvePointer(ctx.dataModel.value, v.path)
  }
  return v
}

// RFC6901 JSON Pointer 解析
function resolvePointer(root: any, pointer: string): any {
  if (!pointer) return root
  if (!pointer.startsWith('/')) return undefined
  let cur = root
  for (const rawTok of pointer.slice(1).split('/')) {
    const tok = rawTok.replace(/~1/g, '/').replace(/~0/g, '~')
    if (cur == null) return undefined
    if (Array.isArray(cur)) {
      const idx = Number(tok)
      if (!Number.isInteger(idx) || idx < 0 || idx >= cur.length) return undefined
      cur = cur[idx]
    } else if (typeof cur === 'object') {
      cur = cur[tok]
    } else {
      return undefined
    }
  }
  return cur
}

function formatValue(v: any): string {
  if (v == null) return '—'
  if (typeof v === 'number') {
    // 小数保留至多 4 位，去掉多余的 0
    return Number.isInteger(v) ? String(v) : parseFloat(v.toFixed(4)).toString()
  }
  return String(v)
}

function toneToType(tone: any): 'info' | 'success' | 'warning' | 'error' {
  if (tone === 'success' || tone === 'warning' || tone === 'error') return tone
  return 'info'
}

// ---- Table 绑定 → UiTable columns/data ----
const tableData = computed(() => prop('data') || {})
const tableColumns = computed(() => {
  const cols: string[] = tableData.value.columns || []
  return cols.map((c) => ({ key: c, title: c }))
})
const tableRows = computed(() => tableData.value.rows || [])

// ---- LineChart：把 series 绑定映射成 SVG 折线坐标 ----
const chart = computed(() => {
  const s = prop('series')
  if (!s || !Array.isArray(s.actual) || s.actual.length < 2) return null

  const actual: number[] = s.actual
  const forecast: number[] = Array.isArray(s.forecast) ? s.forecast : []
  const all = [...actual, ...forecast]
  const min = Math.min(...all)
  const max = Math.max(...all)
  const span = max - min || 1

  const w = 320
  const h = 120
  const padX = 12
  const padY = 12
  const baseY = h - padY
  const n = actual.length + forecast.length
  const stepX = (w - padX * 2) / Math.max(n - 1, 1)
  const toY = (val: number) => padY + (1 - (val - min) / span) * (h - padY * 2)
  const xAt = (i: number) => padX + i * stepX

  const actualDots = actual.map((v, i) => ({ x: xAt(i), y: toY(v) }))
  const actualPoints = actualDots.map((p) => `${p.x},${p.y}`).join(' ')

  let forecastPoints = ''
  let forecastDots: { x: number; y: number }[] = []
  if (forecast.length) {
    // 预测段从最后一个实测点起画，保证连续
    const startIdx = actual.length - 1
    forecastDots = forecast.map((v, i) => ({ x: xAt(startIdx + 1 + i), y: toY(v) }))
    const joined = [actualDots[actualDots.length - 1], ...forecastDots]
    forecastPoints = joined.map((p) => `${p.x},${p.y}`).join(' ')
  }

  return { w, h, padX, baseY, name: s.name || '数值', actualPoints, forecastPoints, actualDots, forecastDots }
})
</script>

<style scoped>
.a2ui-col { display: flex; flex-direction: column; gap: 12px; }
.a2ui-row { display: flex; flex-direction: row; flex-wrap: wrap; gap: 12px; }
.a2ui-row > * { flex: 1 1 120px; }

.a2ui-text--title { font-size: 15px; font-weight: 600; color: var(--color-text-primary, #1f2329); }
.a2ui-text--subtitle { font-size: 13px; font-weight: 500; color: var(--color-text-secondary, #646a73); }
.a2ui-text--body { font-size: 13px; color: var(--color-text-primary, #1f2329); }
.a2ui-text--caption { font-size: 12px; color: var(--color-text-tertiary, #8f959e); }

.a2ui-block-title { font-size: 13px; font-weight: 600; margin-bottom: 6px; color: var(--color-text-secondary, #646a73); }

.a2ui-metric { padding: 12px 14px; }
.a2ui-metric__label { font-size: 12px; color: var(--color-text-tertiary, #8f959e); margin-bottom: 4px; }
.a2ui-metric__value { font-size: 20px; font-weight: 700; color: var(--color-primary, #3370ff); }
.a2ui-metric__unit { font-size: 12px; font-weight: 500; margin-left: 2px; color: var(--color-text-tertiary, #8f959e); }

.a2ui-chart__svg { width: 100%; height: 140px; }
.a2ui-chart__axis { stroke: var(--color-border, #dee0e3); stroke-width: 1; }
.a2ui-chart__line { fill: none; stroke: var(--color-primary, #3370ff); stroke-width: 2; }
.a2ui-chart__line--forecast { stroke: var(--color-warning, #ff8800); stroke-dasharray: 4 3; }
.a2ui-chart__dot { fill: var(--color-primary, #3370ff); }
.a2ui-chart__dot--forecast { fill: var(--color-warning, #ff8800); }
.a2ui-chart__legend { display: flex; gap: 16px; margin-top: 4px; font-size: 12px; color: var(--color-text-tertiary, #8f959e); }
.a2ui-chart__legend-item { display: inline-flex; align-items: center; gap: 4px; }
.a2ui-chart__swatch { width: 10px; height: 3px; border-radius: 2px; background: var(--color-primary, #3370ff); }
.a2ui-chart__swatch--forecast { background: var(--color-warning, #ff8800); }
</style>
