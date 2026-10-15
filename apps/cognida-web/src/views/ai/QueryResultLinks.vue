<template>
  <div v-if="sessionId && resultIds.length" class="query-result-links" aria-label="查询结果导入">
    <RouterLink v-for="(resultId, index) in resultIds" :key="resultId" :to="{ path: '/pipelines', query: { source: 'result', session_id: sessionId, result_id: resultId, name: `查询结果 ${index + 1}` } }">转入流水线 · 查询结果 {{ index + 1 }}</RouterLink>
    <small>结果有时效，过期后请重新查询。</small>
  </div>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import type { AgentStep } from '@/types'
const props = defineProps<{ sessionId: string; steps?: AgentStep[] }>()
// Only durable tool observations expose result handles; assistant prose is not a data source.
const resultIds = computed(() => {
  const ids = new Set<string>()
  for (const step of props.steps || []) {
    if (step.type !== 'tool_result' || step.status !== 'success' || step.error || !step.tool_output) continue
    try {
      const output = JSON.parse(step.tool_output)
      if (typeof output?.result_id === 'string' && output.result_id.trim()) ids.add(output.result_id.trim())
    } catch { /* An incomplete or redacted observation cannot be imported. */ }
  }
  return [...ids]
})
</script>
<style scoped>
.query-result-links { display: flex; flex-wrap: wrap; gap: 8px 12px; margin-top: 12px; font-size: 12px; }
.query-result-links small { flex-basis: 100%; color: var(--text-muted); }
</style>
