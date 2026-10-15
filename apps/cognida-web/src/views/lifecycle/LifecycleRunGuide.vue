<template>
  <div class="execution-guide" :data-state="current.status">
    <div class="execution-guide__heading"><strong>{{ headline }}</strong><button v-if="report" class="primary" type="button" @click="$emit('open', report)">查看分析报告</button></div>
    <p aria-live="polite">{{ nextStep }}</p>
    <div v-if="stages.length" class="execution-progress"><progress :value="completed" :max="stages.length" aria-label="已完成阶段" /><span>{{ completed }} / {{ stages.length }} 个阶段完成</span></div>
    <p v-if="failure" class="failure-reason" role="alert">{{ failure }}</p>
  </div>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import type { Approval, Invocation, RefID, Stage } from '@/api/lifecycle'
const props = defineProps<{ current: Invocation; stages: Stage[]; approvals: Approval[]; retryable: boolean; failureMessage?: string }>()
defineEmits<{ open: [RefID] }>()
const completed = computed(() => props.stages.filter(stage => stage.status === 'succeeded').length)
const awaitingApproval = computed(() => ['queued', 'running', 'waiting', 'awaiting_approval'].includes(props.current.status) && props.approvals.some(approval => ['pending', 'awaiting_approval'].includes(approval.status)))
const failure = computed(() => props.failureMessage ?? (props.current.error || props.stages.find(stage => ['failed', 'quarantined'].includes(stage.status))?.error))
const report = computed(() => [...(props.current.output?.refs || []), ...props.stages.flatMap(stage => stage.output?.refs || [])].find(ref => ref.kind === 'insight'))
const headline = computed(() => awaitingApproval.value ? '下一步：审核后继续执行' : ({ queued: '任务已排队', waiting: '正在等待前序阶段', running: '任务正在执行', awaiting_approval: '等待审批结果', succeeded: '处理完成，可以查看产出', failed: '执行失败，需要处理', quarantined: '未通过门禁，产物已隔离', cancelled: '任务已取消' } as Record<string, string>)[props.current.status] || props.current.status)
const nextStep = computed(() => awaitingApproval.value ? '先查看审批原因和阶段产出，再决定批准或拒绝。未批准前不会发布。' : ({ queued: '进度会自动更新。离开页面后任务继续运行，可从执行历史返回。', waiting: '前序阶段完成后自动继续；可展开阶段查看等待原因。', running: '进度会自动更新。离开页面后任务继续运行，可从执行历史返回。', awaiting_approval: '审批完成后会继续更新，无需重复提交。', succeeded: report.value ? '分析报告已生成，下方保留每个阶段的产出与来源。' : '查看阶段产出，确认质量与来源后继续后续工作。', failed: props.retryable ? '查看失败原因与对应阶段。排除问题后可重试失败任务。' : '查看失败原因。核对输入与参数后重新发起任务。', quarantined: '查看质量门禁与产出。确认数据或处理参数后重新发起任务。', cancelled: '取消不会删除已有产出。可查看阶段结果，确认参数后重新发起任务。' } as Record<string, string>)[props.current.status] || '')
</script>
<style scoped>
.execution-guide { padding: 18px; border: 1px solid var(--border-default); border-left: 3px solid var(--primary); border-radius: 6px; background: var(--bg-elevated); margin: 16px 0; }
.execution-guide[data-state="failed"], .execution-guide[data-state="quarantined"] { border-left-color: var(--danger); }
.execution-guide__heading { display: flex; justify-content: space-between; align-items: center; gap: 12px; }.execution-guide__heading strong { font-size: 15px; }
.execution-guide p { color: var(--text-muted); font-size: 13px; line-height: 1.7; margin: 8px 0 0; }.execution-guide .failure-reason { color: var(--danger); overflow-wrap: anywhere; }
.execution-progress { display: flex; gap: 14px; align-items: center; margin-top: 16px; font-size: 12px; }.execution-progress progress { flex: 1; min-width: 0; height: 8px; accent-color: var(--primary); }.execution-progress span { white-space: nowrap; }
@media(max-width: 600px) { .execution-guide__heading { align-items: flex-start; flex-direction: column; } }
</style>
