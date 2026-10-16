<template>
 <section v-if="sources.length" class="knowledge-sources" aria-label="回答检索来源">
  <h3>检索来源 · {{ sources.length }} 个片段</h3>
  <p class="source-boundary">来自实际检索回执；有来源不代表回答中的每项结论都已核验。</p>
  <details v-for="(source,index) in sources" :key="`${source.knowledge_base_id}:${source.knowledge_id}:${source.chunk_id}`" class="source-card">
   <summary><span>{{ source.source || `来源片段 ${index+1}` }}</span><small>检索分数 {{ source.score.toFixed(3) }}</small></summary>
   <p class="source-content">{{ source.content }}</p>
   <p class="source-id">片段 {{ source.chunk_id }}<template v-if="source.knowledge_id"> · 文档 {{ source.knowledge_id }}</template></p>
   <RouterLink v-if="source.knowledge_base_id && source.knowledge_id" :to="{path:`/knowledge/${encodeURIComponent(source.knowledge_base_id)}`}">打开来源知识库</RouterLink>
  </details>
 </section>
</template>
<script setup lang="ts">
import {computed} from 'vue'
import type {AgentStep,RAGContext} from '@/types'
import {knowledgeSources} from './knowledgeSources'
const props=defineProps<{steps?:AgentStep[];legacy?:RAGContext}>()
const sources=computed(()=>knowledgeSources(props.steps,props.legacy))
</script>
<style scoped>
.knowledge-sources{margin-top:16px;border:1px solid var(--border-subtle);border-radius:var(--radius-lg);background:var(--bg-secondary);padding:14px;color:var(--text-primary);min-width:0}
h3{font-size:13px;margin:0 0 8px}.source-boundary,.source-id{font-size:12px;line-height:1.7;color:var(--text-muted);overflow-wrap:anywhere}.source-card{padding:10px 0;border-top:1px solid var(--border-subtle)}summary{cursor:pointer;display:flex;justify-content:space-between;gap:12px;flex-wrap:wrap;font-size:13px}summary span{overflow-wrap:anywhere}small{color:var(--text-muted)}.source-content{white-space:pre-wrap;overflow-wrap:anywhere;font-size:13px;line-height:1.8}a{font-size:12px;color:var(--color-primary)}
</style>
