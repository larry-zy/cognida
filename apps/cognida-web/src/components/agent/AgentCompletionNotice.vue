<template>
  <div v-if="completion?.partial || completion?.persistence_failed" class="completion-notice" role="status">
    <p v-if="completion.partial">
      <strong>本轮未完成</strong>：{{ reason }}。以下保留已生成的部分回答，不能视为完整结论。
    </p>
    <p v-if="completion.persistence_failed">
      <strong>对话保存失败</strong>：本页仍保留回答，刷新或离开后可能无法恢复。请先复制需要保留的内容。
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { AgentCompletion } from '@/types'
const props = defineProps<{ completion?: AgentCompletion }>()
const reasons: Record<string, string> = {
  max_iter: '已达到执行轮次上限',
  token_budget: '已达到令牌预算上限',
  deadline: '已达到执行时限',
  output_truncated: '模型输出被截断',
  empty_finish: '模型未返回最终正文',
  no_response: '模型未返回最终响应',
  repair_exhausted: '修复尝试已用尽',
}
const reason = computed(() => {
  const code = props.completion?.terminated_by
  return code ? reasons[code] || code : '服务端返回部分结果'
})
</script>

<style scoped>
.completion-notice {
  margin: 0 0 10px;
  padding: 10px 14px;
  border-left: 3px solid var(--warning);
  background: var(--bg-tertiary);
  color: var(--text-primary);
  font-size: 13px;
  line-height: 1.7;
  overflow-wrap: anywhere;
}
.completion-notice p { margin: 0; }
.completion-notice p + p { margin-top: 6px; }
</style>
