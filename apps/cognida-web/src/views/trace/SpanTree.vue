<template>
  <div class="span-tree">
    <div v-if="!tree || tree.length === 0" class="span-tree__empty">
      <UiEmpty description="该调用链暂无 span 数据" />
    </div>
    <template v-else>
      <div class="span-tree__head">
        <span class="col-name">名称 / Kind</span>
        <span class="col-gantt">时间轴</span>
        <span class="col-dur">耗时</span>
        <span class="col-status">状态</span>
        <span class="col-agent">Agent</span>
      </div>
      <SpanTreeNode
        v-for="root in tree"
        :key="root.span.span_id"
        :node="root"
        :depth="0"
        :trace-start="traceStart"
        :trace-duration="traceDuration"
      />
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { UiEmpty } from '@/components'
import SpanTreeNode from './SpanTreeNode.vue'
import type { SpanNode } from '@/api/trace'

const props = defineProps<{
  tree: SpanNode[]
}>()

function toMs(iso: string): number {
  const t = new Date(iso).getTime()
  return Number.isNaN(t) ? 0 : t
}

// 遍历整棵树，求最早开始与最晚结束，作为甘特图时间轴的基准。
const bounds = computed(() => {
  let min = Number.POSITIVE_INFINITY
  let max = Number.NEGATIVE_INFINITY
  const walk = (nodes: SpanNode[]) => {
    for (const n of nodes) {
      const s = toMs(n.span.start_time)
      const e = toMs(n.span.end_time)
      if (s > 0) min = Math.min(min, s)
      if (e > 0) max = Math.max(max, e)
      if (n.children?.length) walk(n.children)
    }
  }
  walk(props.tree || [])
  if (!Number.isFinite(min)) min = 0
  if (!Number.isFinite(max)) max = min
  return { min, max }
})

const traceStart = computed(() => bounds.value.min)
const traceDuration = computed(() => Math.max(1, bounds.value.max - bounds.value.min))
</script>

<style scoped>
.span-tree {
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  overflow: hidden;
}

.span-tree__empty {
  padding: 32px;
}

.span-tree__head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 8px;
  background: var(--bg-secondary);
  border-bottom: 1px solid var(--border-subtle);
  font-size: 12px;
  font-weight: 600;
  color: var(--text-muted);
}

.col-name {
  flex: 0 0 auto;
  width: 300px;
}

.col-gantt {
  flex: 1 1 auto;
  min-width: 80px;
}

.col-dur {
  flex: 0 0 auto;
  width: 64px;
  text-align: right;
}

.col-status {
  flex: 0 0 auto;
  width: 60px;
}

.col-agent {
  flex: 0 0 auto;
  width: 140px;
}
</style>
