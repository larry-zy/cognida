<!--
  A2UINode —— 生成式 UI（A2UI 子集）的递归节点渲染器。

  一次渲染一个组件节点：容器（Column/Row）向下递归其 children；叶子组件
  （Text/MetricCard/Callout/Table/LineChart/BarChart/ScatterChart）用 props 渲染。所有数据一律通过
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
  <!-- 网格布局：响应式 N 列栅格（columns 默认 2），与 Column/Row 一样递归 children -->
  <div v-else-if="node.type === 'grid'" class="a2ui-grid" :style="gridStyle">
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

  <!-- 柱状图：自研 SVG，分类对比（labels 为 x 轴类目，actual 为柱高） -->
  <div v-else-if="node.type === 'BarChart'" class="a2ui-chart">
    <div v-if="prop('title')" class="a2ui-block-title">{{ prop('title') }}</div>
    <svg v-if="bars" :viewBox="`0 0 ${bars.w} ${bars.h}`" class="a2ui-chart__svg" preserveAspectRatio="none">
      <line :x1="bars.padX" :y1="bars.baseY" :x2="bars.w - bars.padX" :y2="bars.baseY" class="a2ui-chart__axis" />
      <rect
        v-for="(b, i) in bars.rects"
        :key="'b' + i"
        :x="b.x"
        :y="b.y"
        :width="bars.barW"
        :height="b.h"
        class="a2ui-chart__bar"
      />
    </svg>
    <div v-if="bars" class="a2ui-chart__xlabels">
      <span v-for="(lb, i) in bars.labels" :key="'l' + i" class="a2ui-chart__xlabel">{{ lb }}</span>
    </div>
  </div>

  <!-- 散点图：自研 SVG，两数值列相关性（x 横轴、y 纵轴） -->
  <div v-else-if="node.type === 'ScatterChart'" class="a2ui-chart">
    <div v-if="prop('title')" class="a2ui-block-title">{{ prop('title') }}</div>
    <svg v-if="scatter" :viewBox="`0 0 ${scatter.w} ${scatter.h}`" class="a2ui-chart__svg" preserveAspectRatio="none">
      <line :x1="scatter.padX" :y1="scatter.baseY" :x2="scatter.w - scatter.padX" :y2="scatter.baseY" class="a2ui-chart__axis" />
      <line :x1="scatter.padX" :y1="scatter.padY" :x2="scatter.padX" :y2="scatter.baseY" class="a2ui-chart__axis" />
      <circle v-for="(p, i) in scatter.dots" :key="'s' + i" :cx="p.x" :cy="p.y" r="3" class="a2ui-chart__dot" />
    </svg>
    <div v-if="scatter" class="a2ui-chart__legend">
      <span class="a2ui-chart__legend-item">{{ scatter.xLabel }}（x） · {{ scatter.yLabel }}（y）</span>
    </div>
  </div>

  <!-- 饼图：自研 SVG，分类占比（labels 类目、actual 数值，切片按占比铺开） -->
  <div v-else-if="node.type === 'pie_chart'" class="a2ui-chart">
    <div v-if="prop('title')" class="a2ui-block-title">{{ prop('title') }}</div>
    <div v-if="pie" class="a2ui-pie">
      <svg :viewBox="`0 0 ${pie.w} ${pie.h}`" class="a2ui-pie__svg">
        <path v-for="(s, i) in pie.slices" :key="'ps' + i" :d="s.d" :fill="s.color" class="a2ui-pie__slice" />
      </svg>
      <div class="a2ui-pie__legend">
        <span v-for="(s, i) in pie.slices" :key="'pl' + i" class="a2ui-pie__legend-item">
          <i class="a2ui-pie__swatch" :style="{ background: s.color }"></i>{{ s.label }} {{ s.pct }}%
        </span>
      </div>
    </div>
  </div>

  <!-- 漏斗图：自研 SVG，有序阶段逐级转化（梯形带 + 值 + 步转化率） -->
  <div v-else-if="node.type === 'funnel'" class="a2ui-chart">
    <div v-if="prop('title')" class="a2ui-block-title">{{ prop('title') }}</div>
    <svg v-if="funnel" :viewBox="`0 0 ${funnel.w} ${funnel.h}`" class="a2ui-chart__svg" preserveAspectRatio="none">
      <polygon
        v-for="(st, i) in funnel.stages"
        :key="'fn' + i"
        :points="st.points"
        class="a2ui-chart__bar"
        :style="{ opacity: st.opacity }"
      />
    </svg>
    <div v-if="funnel" class="a2ui-funnel__labels">
      <div v-for="(st, i) in funnel.stages" :key="'fl' + i" class="a2ui-funnel__row">
        <span class="a2ui-funnel__name">{{ st.label }}</span>
        <span class="a2ui-funnel__val">{{ st.value }}</span>
        <span v-if="st.conv !== null" class="a2ui-funnel__conv">↓ {{ st.conv }}%</span>
      </div>
    </div>
  </div>

  <!-- 按钮：触发声明式动作（回抛宿主，Phase 4/5 操作类工具消费） -->
  <div v-else-if="node.type === 'Button'" class="a2ui-button">
    <UiButton :variant="buttonVariant" size="sm" @click="fireAction(prop('action'))">
      {{ prop('label') || '执行' }}
    </UiButton>
  </div>

  <!-- 确认卡：写操作前置确认（pending_action_id 关联后端待执行操作） -->
  <UiCard v-else-if="node.type === 'Confirm'" class="a2ui-confirm" variant="bordered" padding="none">
    <div class="a2ui-confirm__title">{{ prop('title') || '请确认操作' }}</div>
    <div v-if="prop('text')" class="a2ui-confirm__text">{{ prop('text') }}</div>
    <div v-if="confirmDecided" class="a2ui-confirm__done">
      {{ confirmDecided === 'confirm' ? '已确认，正在执行…' : '已取消，该操作将自动过期' }}
    </div>
    <div v-else class="a2ui-confirm__actions">
      <UiButton variant="secondary" size="sm" @click="fireConfirm(false)">
        {{ prop('cancelLabel') || '取消' }}
      </UiButton>
      <UiButton variant="primary" size="sm" @click="fireConfirm(true)">
        {{ prop('confirmLabel') || '确认执行' }}
      </UiButton>
    </div>
  </UiCard>

  <!-- 表单：参数补全后提交动作 -->
  <div v-else-if="node.type === 'Form'" class="a2ui-form">
    <div v-for="f in formFields" :key="f.name" class="a2ui-form__field">
      <label class="a2ui-form__label">{{ f.label || f.name }}</label>
      <UiInput
        v-model="formValues[f.name]"
        :type="f.type === 'number' ? 'number' : 'text'"
        size="sm"
        :placeholder="f.label || f.name"
      />
    </div>
    <UiButton variant="primary" size="sm" @click="fireSubmit()">提交</UiButton>
  </div>

  <!-- 过滤器：选项可 {path} 绑定自数据树 -->
  <div v-else-if="node.type === 'Filter'" class="a2ui-filter">
    <span v-if="prop('field')" class="a2ui-filter__label">{{ prop('field') }}</span>
    <UiSelect
      v-model="filterValue"
      :options="filterOptions"
      size="sm"
      clearable
      placeholder="筛选"
      @change="fireFilter"
    />
  </div>

  <!-- 日期选择器：原生 date input，选中后经同一 onAction 通道回抛动作（同 Filter） -->
  <div v-else-if="node.type === 'date_picker'" class="a2ui-datepicker">
    <span v-if="prop('field')" class="a2ui-filter__label">{{ prop('field') }}</span>
    <template v-if="prop('range')">
      <input v-model="dateStart" type="date" class="a2ui-datepicker__input" @change="fireDate" />
      <span class="a2ui-datepicker__sep">~</span>
      <input v-model="dateEnd" type="date" class="a2ui-datepicker__input" @change="fireDate" />
    </template>
    <input v-else v-model="dateValue" type="date" class="a2ui-datepicker__input" @change="fireDate" />
  </div>

  <!-- 分页：经 surface 回源按 cursor 取页（不重跑查询），由 Renderer 消化 paginate 动作 -->
  <div v-else-if="node.type === 'Pagination'" class="a2ui-pagination">
    <UiPagination
      :page="pageNo"
      :page-size="pageSize"
      :total="pageTotal"
      size="sm"
      show-info
      @change="firePaginate"
    />
  </div>

  <!-- 未知类型：静默跳过（catalog 已在后端白名单校验） -->
</template>

<script setup lang="ts">
import { computed, inject, reactive, ref } from 'vue'
import UiCard from '@/components/ui/UiCard.vue'
import UiAlert from '@/components/ui/UiAlert.vue'
import UiTable from '@/components/ui/UiTable.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import UiPagination from '@/components/ui/UiPagination.vue'
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

// ---- 交互组件：动作统一走 ctx.onAction（Renderer 注入） ----

// Button 变体映射（后端 variant → UiButton variant）
const buttonVariant = computed(() => {
  const v = prop('variant')
  if (v === 'danger' || v === 'secondary' || v === 'ghost') return v
  return 'primary'
})

// fireAction：Button 的声明式动作 {name, params}
function fireAction(action: any) {
  const a = resolve(action)
  if (!a || typeof a.name !== 'string') return
  ctx.onAction({ name: a.name, params: a.params || {} })
}

// fireConfirm：确认卡两个分支都带完整回调三元组（Phase 5 确认续跑消费）：
// pending_action_id + 一次性确认 token + session_id（后端归属键校验必需）。
// pending action 消费即失效，本地锁定防双击重复提交。
const confirmDecided = ref<'' | 'confirm' | 'cancel'>('')
function fireConfirm(ok: boolean) {
  if (confirmDecided.value) return
  confirmDecided.value = ok ? 'confirm' : 'cancel'
  ctx.onAction({
    name: ok ? 'confirm' : 'cancel',
    params: {
      pending_action_id: prop('pending_action_id'),
      token: prop('token'),
      session_id: prop('session_id')
    }
  })
}

// ---- Form：本地收集字段值后随 submitAction 提交 ----
interface FormField { name: string; label?: string; type?: string; default?: any }
const formFields = computed<FormField[]>(() => {
  const fs = prop('fields')
  return Array.isArray(fs) ? fs.filter((f: any) => f && typeof f.name === 'string') : []
})
const formValues = reactive<Record<string, any>>({})
for (const f of formFields.value) {
  if (formValues[f.name] === undefined) formValues[f.name] = f.default ?? ''
}
function fireSubmit() {
  const name = prop('submitAction')
  if (typeof name !== 'string' || !name) return
  ctx.onAction({ name, params: { ...formValues } })
}

// ---- Filter：选项可为字符串数组或 {label, value} 数组（可 {path} 绑定） ----
const filterValue = ref<string | number>('')
const filterOptions = computed(() => {
  const opts = prop('options')
  if (!Array.isArray(opts)) return []
  return opts.map((o: any) =>
    o && typeof o === 'object' ? { label: String(o.label ?? o.value), value: o.value } : { label: String(o), value: o }
  )
})
function fireFilter(v: string | number | (string | number)[]) {
  const name = prop('action')
  ctx.onAction({
    name: typeof name === 'string' && name ? name : 'filter',
    params: { field: prop('field'), value: v }
  })
}

// ---- Pagination：page/pageSize 本地态，total 通常绑定 /meta/row_count ----
const localPage = ref(0)
const pageNo = computed(() => localPage.value || Number(prop('page')) || 1)
const pageSize = computed(() => Number(prop('pageSize')) || 20)
const pageTotal = computed(() => Number(prop('total')) || 0)
function firePaginate(page: number, size: number) {
  localPage.value = page
  ctx.onAction({ name: 'paginate', params: { page, page_size: size } })
}

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

// ---- BarChart：series.actual 映射成分类柱形（labels 为 x 轴类目） ----
const bars = computed(() => {
  const s = prop('series')
  if (!s || !Array.isArray(s.actual) || s.actual.length < 1) return null

  const actual: number[] = s.actual
  const labels: any[] = Array.isArray(s.labels) ? s.labels : actual.map((_, i) => i + 1)
  const max = Math.max(...actual, 0)
  const min = Math.min(...actual, 0)
  const span = max - min || 1

  const w = 320
  const h = 120
  const padX = 12
  const padY = 12
  const baseY = h - padY
  const n = actual.length
  const slot = (w - padX * 2) / n
  const barW = Math.max(slot * 0.6, 2)
  const toH = (val: number) => ((val - min) / span) * (h - padY * 2)

  const rects = actual.map((v, i) => {
    const barH = toH(v)
    return { x: padX + i * slot + (slot - barW) / 2, y: baseY - barH, h: Math.max(barH, 0) }
  })

  return { w, h, padX, padY, baseY, barW, rects, labels: labels.map((l) => String(l)) }
})

// ---- ScatterChart：points.x/y 映射成二维散点（相关性） ----
const scatter = computed(() => {
  const p = prop('points')
  if (!p || !Array.isArray(p.x) || !Array.isArray(p.y) || p.x.length < 2) return null

  const xs: number[] = p.x
  const ys: number[] = p.y
  const n = Math.min(xs.length, ys.length)
  const xMin = Math.min(...xs)
  const xMax = Math.max(...xs)
  const yMin = Math.min(...ys)
  const yMax = Math.max(...ys)
  const xSpan = xMax - xMin || 1
  const ySpan = yMax - yMin || 1

  const w = 320
  const h = 120
  const padX = 12
  const padY = 12
  const baseY = h - padY
  const toX = (val: number) => padX + ((val - xMin) / xSpan) * (w - padX * 2)
  const toY = (val: number) => padY + (1 - (val - yMin) / ySpan) * (h - padY * 2)

  const dots: { x: number; y: number }[] = []
  for (let i = 0; i < n; i++) dots.push({ x: toX(xs[i]), y: toY(ys[i]) })

  return { w, h, padX, padY, baseY, dots, xLabel: p.xLabel || 'x', yLabel: p.yLabel || 'y' }
})

// 分类占比调色板（饼图切片用；与主题主色协调）。
const PIE_PALETTE = ['#3370ff', '#00c8c8', '#ff8800', '#7f4dff', '#00b42a', '#f53f3f', '#ffca28', '#14c9c9']

// ---- Grid：列数由 props.columns 决定（默认 2），CSS grid 响应式布局 ----
const gridStyle = computed(() => {
  const cols = Math.max(1, Math.floor(Number(prop('columns')) || 2))
  return { display: 'grid', gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`, gap: '12px' }
})

// ---- PieChart：series.actual 按占比铺成扇形切片（labels 为类目） ----
const pie = computed(() => {
  const s = prop('series')
  if (!s || !Array.isArray(s.actual) || s.actual.length < 1) return null

  const values: number[] = s.actual.map((v: any) => Math.max(Number(v) || 0, 0))
  const labels: any[] = Array.isArray(s.labels) ? s.labels : values.map((_, i) => i + 1)
  const total = values.reduce((a, b) => a + b, 0)
  if (total <= 0) return null

  const w = 120
  const h = 120
  const cx = 60
  const cy = 60
  const r = 54
  const slices: { d: string; color: string; label: string; pct: number }[] = []
  let acc = 0
  values.forEach((v, i) => {
    const frac = v / total
    const color = PIE_PALETTE[i % PIE_PALETTE.length]
    const pct = parseFloat((frac * 100).toFixed(1))
    let d: string
    if (frac >= 0.9999) {
      // 单一切片：SVG 弧无法闭合整圆，用两段半弧拼成整圆。
      d = `M ${cx} ${cy - r} A ${r} ${r} 0 1 1 ${cx} ${cy + r} A ${r} ${r} 0 1 1 ${cx} ${cy - r} Z`
    } else {
      const a0 = acc * 2 * Math.PI - Math.PI / 2
      const a1 = (acc + frac) * 2 * Math.PI - Math.PI / 2
      const x0 = (cx + r * Math.cos(a0)).toFixed(2)
      const y0 = (cy + r * Math.sin(a0)).toFixed(2)
      const x1 = (cx + r * Math.cos(a1)).toFixed(2)
      const y1 = (cy + r * Math.sin(a1)).toFixed(2)
      const large = frac > 0.5 ? 1 : 0
      d = `M ${cx} ${cy} L ${x0} ${y0} A ${r} ${r} 0 ${large} 1 ${x1} ${y1} Z`
    }
    slices.push({ d, color, label: String(labels[i]), pct })
    acc += frac
  })
  return { w, h, slices }
})

// ---- Funnel：series.actual 逐级铺成梯形带，标注值 + 相对上一阶段的步转化率 ----
const funnel = computed(() => {
  const s = prop('series')
  if (!s || !Array.isArray(s.actual) || s.actual.length < 1) return null

  const values: number[] = s.actual.map((v: any) => Number(v) || 0)
  const labels: any[] = Array.isArray(s.labels) ? s.labels : values.map((_, i) => i + 1)
  const max = Math.max(...values, 0) || 1

  const w = 320
  const h = 120
  const padX = 12
  const padY = 12
  const fullW = w - padX * 2
  const n = values.length
  const rowH = (h - padY * 2) / n
  const cx = w / 2

  const stages = values.map((v, i) => {
    const wTop = (Math.max(v, 0) / max) * fullW
    const nextV = i < n - 1 ? values[i + 1] : v
    const wBot = (Math.max(nextV, 0) / max) * fullW
    const yTop = padY + i * rowH
    const yBot = yTop + rowH
    const points = [
      `${cx - wTop / 2},${yTop}`,
      `${cx + wTop / 2},${yTop}`,
      `${cx + wBot / 2},${yBot}`,
      `${cx - wBot / 2},${yBot}`,
    ].join(' ')
    // 步转化率：相对上一阶段（首阶段无）。
    let conv: number | null = null
    if (i > 0 && values[i - 1] > 0) conv = parseFloat(((v / values[i - 1]) * 100).toFixed(1))
    return { points, label: String(labels[i]), value: formatValue(v), conv, opacity: 0.85 - i * 0.08 }
  })
  return { w, h, stages }
})

// ---- DatePicker：原生 date input，选中后经 onAction 回抛（同 Filter 通道） ----
const dateValue = ref('')
const dateStart = ref('')
const dateEnd = ref('')
function fireDate() {
  const name = prop('action')
  const actionName = typeof name === 'string' && name ? name : 'filter'
  const params = prop('range')
    ? { field: prop('field'), start: dateStart.value, end: dateEnd.value }
    : { field: prop('field'), value: dateValue.value }
  ctx.onAction({ name: actionName, params })
}
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
.a2ui-chart__bar { fill: var(--color-primary, #3370ff); opacity: 0.85; }
.a2ui-chart__xlabels { display: flex; justify-content: space-around; margin-top: 2px; }
.a2ui-chart__xlabel { flex: 1; text-align: center; font-size: 11px; color: var(--color-text-tertiary, #8f959e); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.a2ui-button { display: inline-flex; }

.a2ui-confirm { padding: 12px 14px; border-left: 3px solid var(--color-warning, #ff8800); }
.a2ui-confirm__title { font-size: 13px; font-weight: 600; color: var(--color-text-primary, #1f2329); }
.a2ui-confirm__text { font-size: 12px; margin-top: 4px; color: var(--color-text-secondary, #646a73); }
.a2ui-confirm__actions { display: flex; gap: 8px; justify-content: flex-end; margin-top: 10px; }
.a2ui-confirm__done { font-size: 12px; margin-top: 10px; text-align: right; color: var(--color-text-secondary, #646a73); }

.a2ui-form { display: flex; flex-direction: column; gap: 8px; max-width: 360px; }
.a2ui-form__field { display: flex; flex-direction: column; gap: 4px; }
.a2ui-form__label { font-size: 12px; color: var(--color-text-secondary, #646a73); }

.a2ui-filter { display: flex; align-items: center; gap: 8px; }
.a2ui-filter__label { font-size: 12px; color: var(--color-text-tertiary, #8f959e); white-space: nowrap; }

.a2ui-pagination { display: flex; justify-content: flex-end; }

/* 饼图 */
.a2ui-pie { display: flex; align-items: center; gap: 16px; flex-wrap: wrap; }
.a2ui-pie__svg { width: 132px; height: 132px; flex: 0 0 auto; }
.a2ui-pie__slice { stroke: var(--color-bg-container, #fff); stroke-width: 1; }
.a2ui-pie__legend { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--color-text-secondary, #646a73); }
.a2ui-pie__legend-item { display: inline-flex; align-items: center; gap: 6px; }
.a2ui-pie__swatch { width: 10px; height: 10px; border-radius: 2px; flex: 0 0 auto; }

/* 漏斗图 */
.a2ui-funnel__labels { display: flex; flex-direction: column; gap: 2px; margin-top: 6px; }
.a2ui-funnel__row { display: flex; align-items: center; gap: 8px; font-size: 12px; }
.a2ui-funnel__name { flex: 1 1 auto; color: var(--color-text-secondary, #646a73); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.a2ui-funnel__val { font-weight: 600; color: var(--color-text-primary, #1f2329); }
.a2ui-funnel__conv { color: var(--color-text-tertiary, #8f959e); }

/* 日期选择器 */
.a2ui-datepicker { display: flex; align-items: center; gap: 8px; }
.a2ui-datepicker__input {
  font-size: 13px;
  padding: 4px 8px;
  border: 1px solid var(--color-border, #dee0e3);
  border-radius: 6px;
  background: var(--color-bg-container, #fff);
  color: var(--color-text-primary, #1f2329);
}
.a2ui-datepicker__sep { font-size: 12px; color: var(--color-text-tertiary, #8f959e); }
</style>
