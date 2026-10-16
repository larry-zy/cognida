<template>
  <fieldset class="wb-panel" style="margin-top: 16px">
    <legend>评测映射（与问答一起审核）</legend>
    <label class="wb-label">使用方式<select :value="modelValue?.kind || 'qa'" @change="changeKind(($event.target as HTMLSelectElement).value)"><option value="qa">QA 问答</option><option value="rag">RAG 检索问答</option><option value="agent">Agent 工具与步骤</option><option value="sql">SQL 只读查询</option></select></label>
    <template v-if="modelValue?.kind === 'rag'">
      <label class="wb-label">授权来源资产引用（每行一个）<textarea :value="lines(modelValue.source_refs)" @input="setList('source_refs', $event)" /></label>
      <label class="wb-label">相关分块 ID（每行一个）<textarea :value="lines(modelValue.relevant_chunk_ids)" @input="setList('relevant_chunk_ids', $event)" /></label>
    </template>
    <template v-if="modelValue?.kind === 'agent'">
      <label class="wb-label">预期工具（按顺序，每行一个）<textarea :value="lines(modelValue.expected_tools)" @input="setList('expected_tools', $event)" /></label>
      <label class="wb-label">预期步骤（按顺序，每行一个）<textarea :value="lines(modelValue.expected_steps)" @input="setList('expected_steps', $event)" /></label>
    </template>
    <template v-if="modelValue?.kind === 'sql'">
      <label class="wb-label">数据源 ID<input :value="modelValue.datasource_id" @input="setText('datasource_id', $event)" /></label>
      <label class="wb-label">SQL 方言<select :value="modelValue.dialect || ''" @change="setText('dialect', $event)"><option value="">选择方言</option><option value="mysql">MySQL</option><option value="postgresql">PostgreSQL</option><option value="sqlite">SQLite</option></select></label>
      <label class="wb-label">金标准 SQL<textarea :value="modelValue.gold_sql" @input="setText('gold_sql', $event)" /></label>
    </template>
    <p class="wb-footnote">更改映射后需保存并重新审核。发布时核验来源和目标权限；不会从分类、实体或关系标签编造问答。</p>
  </fieldset>
</template>
<script setup lang="ts">
import type { EvaluationProfile } from '@/api/evaluation/versions'
const props = defineProps<{ modelValue?: EvaluationProfile }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: EvaluationProfile): void }>()
const lines = (value?: string[]) => (value || []).join('\n')
function changeKind(kind: string) { emit('update:modelValue', { kind: kind as EvaluationProfile['kind'] }) }
function setText(key: keyof EvaluationProfile, event: Event) { emit('update:modelValue', { ...(props.modelValue || { kind: 'qa' }), [key]: (event.target as HTMLInputElement).value }) }
function setList(key: keyof EvaluationProfile, event: Event) { emit('update:modelValue', { ...(props.modelValue || { kind: 'qa' }), [key]: (event.target as HTMLTextAreaElement).value.split('\n').map(s => s.trim()).filter(Boolean) }) }
</script>
