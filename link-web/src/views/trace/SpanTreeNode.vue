<template>
  <div class="span-node">
    <div
      class="span-row"
      :class="{ 'span-row--error': isError }"
      :style="{ paddingLeft: `${depth * 18 + 8}px` }"
      @click="toggle"
    >
      <!-- 展开/收起箭头 -->
      <span class="span-caret" :class="{ 'span-caret--leaf': !hasChildren }">
        <svg v-if="hasChildren" viewBox="0 0 24 24" width="12" height="12" :class="{ open: expanded }">
          <path d="M9 6l6 6-6 6" fill="none" stroke="currentColor" stroke-width="2" />
        </svg>
      </span>

      <!-- span 名称 + kind -->
      <span class="span-name" :title="node.span.name">{{ node.span.name }}</span>
      <UiTag v-if="node.span.kind" size="sm" variant="info" class="span-kind">{{ node.span.kind }}</UiTag>

      <!-- 甘特条：相对整条 trace 的时间轴 -->
      <div class="span-gantt">
        <div
          class="span-bar"
          :class="{ 'span-bar--error': isError }"
          :style="barStyle"
        />
      </div>

      <!-- 耗时 -->
      <span class="span-dur mono">{{ formatDuration(node.span.duration_ms) }}</span>

      <!-- 状态 -->
      <UiTag size="sm" :variant="statusVariant" class="span-status">
        {{ statusLabel }}
      </UiTag>

      <!-- agent 名 -->
      <span v-if="node.span.agent_name" class="span-agent" :title="node.span.agent_name">
        {{ node.span.agent_name }}
      </span>
    </div>

    <!-- 详情：attributes / events / 状态信息 -->
    <div v-if="expanded && showDetail" class="span-detail" :style="{ marginLeft: `${depth * 18 + 26}px` }">
      <div v-if="node.span.status_msg" class="detail-block">
        <h5>状态信息</h5>
        <pre class="detail-pre" :class="{ 'detail-pre--error': isError }">{{ node.span.status_msg }}</pre>
      </div>
      <div class="detail-block">
        <h5>Span 元信息</h5>
        <pre class="detail-pre">{{ spanMeta }}</pre>
      </div>
      <div class="detail-block">
        <h5>Attributes</h5>
        <pre class="detail-pre">{{ prettyAttributes }}</pre>
      </div>
      <div class="detail-block">
        <h5>Events</h5>
        <pre class="detail-pre">{{ prettyEvents }}</pre>
      </div>
    </div>

    <!-- 递归渲染子节点 -->
    <template v-if="expanded && hasChildren">
      <SpanTreeNode
        v-for="child in node.children"
        :key="child.span.span_id"
        :node="child"
        :depth="depth + 1"
        :trace-start="traceStart"
        :trace-duration="traceDuration"
      />
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { UiTag } from '@/components'
import type { SpanNode } from '@/api/trace'

const props = defineProps<{
  node: SpanNode
  depth: number
  // 整条 trace 的起点（ms 时间戳）与总时长（ms），用于计算甘特条位置。
  traceStart: number
  traceDuration: number
}>()

// 首两层默认展开，深层默认收起，避免大 trace 一次性铺满。
const expanded = ref(props.depth < 2)
const showDetail = ref(false)

const hasChildren = computed(() => props.node.children && props.node.children.length > 0)

const isError = computed(() => {
  const code = (props.node.span.status_code || '').toUpperCase()
  return code === 'ERROR' || code === 'STATUS_CODE_ERROR'
})

const statusVariant = computed(() => {
  if (isError.value) return 'danger' as const
  const code = (props.node.span.status_code || '').toUpperCase()
  if (code === 'OK' || code === 'STATUS_CODE_OK') return 'success' as const
  return 'default' as const
})

const statusLabel = computed(() => {
  const code = (props.node.span.status_code || '').toUpperCase()
  if (isError.value) return 'ERROR'
  if (code === 'OK' || code === 'STATUS_CODE_OK') return 'OK'
  return code || 'UNSET'
})

// 点击行：若无子节点仅切换详情；有子节点则同时展开子树 + 详情。
function toggle() {
  expanded.value = !expanded.value
  showDetail.value = expanded.value
}

function toMs(iso: string): number {
  const t = new Date(iso).getTime()
  return Number.isNaN(t) ? 0 : t
}

// 甘特条：left = 相对偏移比例，width = 时长占比。
const barStyle = computed(() => {
  const total = props.traceDuration > 0 ? props.traceDuration : 1
  const start = toMs(props.node.span.start_time)
  const offset = Math.max(0, start - props.traceStart)
  const dur = props.node.span.duration_ms > 0 ? props.node.span.duration_ms : 0
  const left = Math.min(100, (offset / total) * 100)
  let width = (dur / total) * 100
  if (width < 0.8) width = 0.8 // 极短 span 仍可见
  if (left + width > 100) width = Math.max(0.8, 100 - left)
  return { left: `${left}%`, width: `${width}%` }
})

function formatDuration(ms: number): string {
  if (ms == null) return '-'
  if (ms < 1) return '<1 ms'
  if (ms < 1000) return `${Math.round(ms)} ms`
  return `${(ms / 1000).toFixed(2)} s`
}

function prettyJson(raw: string): string {
  if (!raw) return '-'
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

const prettyAttributes = computed(() => prettyJson(props.node.span.attributes))
const prettyEvents = computed(() => prettyJson(props.node.span.events))

const spanMeta = computed(() =>
  JSON.stringify(
    {
      span_id: props.node.span.span_id,
      parent_span_id: props.node.span.parent_span_id || null,
      kind: props.node.span.kind || null,
      agent_name: props.node.span.agent_name || null,
      start_time: props.node.span.start_time,
      end_time: props.node.span.end_time,
      duration_ms: props.node.span.duration_ms,
      status_code: props.node.span.status_code || null,
    },
    null,
    2,
  ),
)
</script>

<style scoped>
.span-node {
  width: 100%;
}

.span-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-bottom: 1px solid var(--border-subtle);
  cursor: pointer;
  font-size: 13px;
  transition: background 0.15s;
}

.span-row:hover {
  background: var(--bg-secondary);
}

.span-row--error {
  background: color-mix(in srgb, var(--danger) 6%, transparent);
}

.span-caret {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  flex-shrink: 0;
  color: var(--text-muted);
}

.span-caret svg {
  transition: transform 0.15s;
}

.span-caret svg.open {
  transform: rotate(90deg);
}

.span-name {
  flex: 0 0 auto;
  max-width: 260px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-primary);
  font-weight: 500;
}

.span-kind {
  flex-shrink: 0;
}

.span-gantt {
  position: relative;
  flex: 1 1 auto;
  min-width: 80px;
  height: 12px;
  background: var(--bg-tertiary, var(--bg-secondary));
  border-radius: 3px;
  overflow: hidden;
}

.span-bar {
  position: absolute;
  top: 0;
  height: 100%;
  background: var(--primary, #4f8cff);
  border-radius: 3px;
  min-width: 2px;
}

.span-bar--error {
  background: var(--danger);
}

.span-dur {
  flex: 0 0 auto;
  width: 64px;
  text-align: right;
  color: var(--text-secondary);
}

.span-status {
  flex-shrink: 0;
}

.span-agent {
  flex: 0 0 auto;
  max-width: 140px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-muted);
  font-size: 12px;
}

.span-detail {
  padding: 8px 12px 12px;
}

.detail-block {
  margin-top: 8px;
}

.detail-block h5 {
  margin: 0 0 4px;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-muted);
}

.detail-pre {
  margin: 0;
  padding: 10px;
  background: var(--bg-tertiary, var(--bg-secondary));
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  font-family: var(--font-mono, ui-monospace, monospace);
  font-size: 12px;
  line-height: 1.6;
  color: var(--text-secondary);
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 320px;
  overflow: auto;
}

.detail-pre--error {
  color: var(--danger);
}

.mono {
  font-family: var(--font-mono, ui-monospace, monospace);
  font-size: 12px;
}
</style>
