<template>
  <div class="wb-stack">
    <label class="wb-label">候选数据集名称<input v-model="name" maxlength="128" required aria-label="任务 SFT 名称" /></label>
    <StandardVersionSelect v-model="standardID" :task-types="['qa']" @selected="standard = $event" />
    <div class="wb-form">
      <label>数据格式<select v-model="format" aria-label="任务 SFT 格式"><option value="qa">已有问答列</option><option value="messages">messages / tools JSON 列</option></select></label>
      <template v-if="format === 'qa'"><label>问题列<input v-model="columns.question" required aria-label="任务 SFT 问题列" /></label><label>参考答案列<input v-model="columns.reference_answer" required aria-label="任务 SFT 参考答案列" /></label></template>
      <template v-else><label>messages 列<input v-model="columns.messages" required aria-label="任务 SFT messages 列" /></label><label>tools 列（可选）<input v-model="columns.tools" aria-label="任务 SFT tools 列" /></label></template>
      <label>split 列（可选）<input v-model="columns.split" aria-label="任务 SFT split 列" placeholder="不选则 train" /></label>
      <label>分组列（可选）<input v-model="columns.group" aria-label="任务 SFT 分组列" /></label>
    </div>
    <p class="wb-muted">只构建最多 1,000 行的待审核候选，不补充答案。任务完成后打开 SFT 数据集检查样本，再分别审核、发布和导出；不会自动训练。</p>
  </div>
</template>
<script setup lang="ts">
import { reactive, ref } from 'vue'
import StandardVersionSelect from '@/views/scenario/StandardVersionSelect.vue'
import type { StandardVersion } from '@/api/governance/standards'
import type { NodeParameters } from '@/api/taskcontrol'
const name = ref(''), standardID = ref(''), standard = ref<StandardVersion>(), format = ref<'qa' | 'messages'>('qa')
const columns = reactive({ question: 'question', reference_answer: 'reference_answer', messages: 'messages', tools: '', split: '', group: '' })
function parameters(): NonNullable<NodeParameters['sft']> {
  if (!name.value.trim()) throw new Error('请填写候选数据集名称。')
  if (!standard.value || standard.value.id !== standardID.value || standard.value.task_type !== 'qa' || standard.value.status !== 'published') throw new Error('请选择已发布的 QA 标准版本。')
  const mapped: NonNullable<NodeParameters['sft']>['columns'] = format.value === 'qa'
    ? { question: columns.question.trim(), reference_answer: columns.reference_answer.trim() }
    : { messages: columns.messages.trim(), ...(columns.tools.trim() ? { tools: columns.tools.trim() } : {}) }
  if (columns.split.trim()) mapped.split = columns.split.trim()
  if (columns.group.trim()) mapped.group = columns.group.trim()
  const values = Object.values(mapped)
  if (values.some(value => !value || value.length > 128) || new Set(values).size !== values.length) throw new Error('SFT 映射列须非空、互不重复且不超过 128 字符。')
  return { name: name.value.trim(), standard_version_id: standardID.value, format: format.value, columns: mapped }
}
defineExpose({ parameters })
</script>
