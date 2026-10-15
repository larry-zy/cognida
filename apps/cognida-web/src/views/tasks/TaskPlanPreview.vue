<template>
  <div class="plan-preview">
    <dl class="plan-limits" aria-label="计划预算上限">
      <div><dt>节点 / 并发</dt><dd>{{ plan.nodes.length }} / {{ plan.budget.max_concurrency }}</dd></div>
      <div><dt>总尝试 / 总调用</dt><dd>{{ plan.budget.max_attempts }} / {{ plan.budget.max_calls }}</dd></div>
      <div><dt>成本额度</dt><dd>{{ plan.budget.max_cost_micros.toLocaleString() }}</dd></div>
      <div><dt>总时限</dt><dd>{{ plan.budget.timeout_seconds }} 秒</dd></div>
    </dl>
    <p class="wb-muted plan-note">成本按百万分之一计价单位计，以上为计划上限。{{ validation ? `含重试预留 ${validation.reserved_attempts} 次尝试、${validation.reserved_calls} 次调用、${validation.reserved_seconds} 秒。` : '已保存快照不代表当前权限或运行结果已再次核验。' }}</p>
    <ol class="plan-nodes" aria-label="计划步骤">
      <li v-for="(node, index) in orderedNodes" :key="node.id">
        <div class="node-heading"><span class="node-number">{{ String(index + 1).padStart(2, '0') }}</span><h3>{{ capabilityLabel(node.capability) }}</h3><span v-if="requiresApproval(node.id, node.capability, node.capability_version, node.permissions)" class="wb-badge">需要人工审批</span></div>
        <p><strong>依赖</strong> {{ node.depends_on?.length ? node.depends_on.map(dependencyLabel).join(' → ') : '从所选输入开始' }}</p>
        <ul class="node-inputs" aria-label="步骤输入"><li v-for="(input, i) in node.inputs" :key="i">{{ assetLabel(input.kind) }} · <template v-if="input.asset">{{ input.asset.asset_id }} · v{{ input.asset.version }}</template><template v-else>来自 {{ dependencyLabel(input.from_node || '') }}</template></li></ul>
        <dl v-if="node.parameters" class="domain-parameters" aria-label="固定领域参数">
          <template v-if="node.parameters.governance"><dt>标签方案</dt><dd>{{ node.parameters.governance.label_schema?.map(label => `${label.name}：${(label.keywords || label.terms || []).join('、')}`).join('；') || node.parameters.governance.labels?.join('、') || '问答字段映射' }}</dd><template v-if="node.parameters.governance.qa_mapping"><dt>问题／参考答案列</dt><dd>{{ node.parameters.governance.qa_mapping.question_column }} ／ {{ node.parameters.governance.qa_mapping.reference_answer_column }}</dd></template><dt>审核</dt><dd>生成候选，保持待人工审核</dd></template>
          <template v-if="node.parameters.feature"><dt>特征定义</dt><dd>{{ node.parameters.feature.feature_id }} · v{{ node.parameters.feature.version }}</dd><dt>物化</dt><dd>固定来源及定义版本，保存实际值与可用时间</dd></template>
        </dl>
        <p><strong>产出</strong> {{ node.output_kinds.map(assetLabel).join('、') }}</p>
        <p><strong>工具</strong> {{ node.tools.join('、') }}<br /><strong>权限</strong> {{ node.permissions.join('、') }}</p>
        <p class="wb-muted">最多 {{ node.max_attempts }} 次尝试 · 每次 {{ node.max_calls_per_attempt }} 次调用 / {{ node.timeout_seconds }} 秒 · 每次成本额度 {{ node.max_cost_micros_per_attempt }}</p>
        <p v-if="executionFor(node.capability, node.capability_version)" class="preset-note">固定执行预置：{{ executionProfileLabel(executionFor(node.capability, node.capability_version)!.profile) }}<br />每次基础 {{ executionFor(node.capability, node.capability_version)!.base_calls }} 次调用 + 每项输入 {{ executionFor(node.capability, node.capability_version)!.calls_per_input }} 次；单次最多 {{ executionFor(node.capability, node.capability_version)!.max_timeout_seconds }} 秒。具体任务仍需通过执行准入。</p>
        <p v-else-if="capabilities.some(item => item.id === node.capability && item.version === node.capability_version)" class="wb-muted">此能力尚未接入当前任务控制器，计划可保存供审阅，执行准入会说明阻塞原因。</p>
        <p v-else class="wb-muted">执行支持情况需以当前准入检查为准。</p>
        <details><summary>技术标识与版本</summary><p class="wb-muted">{{ node.id }} · {{ node.capability }} v{{ node.capability_version }}</p><p v-if="executionFor(node.capability, node.capability_version)" class="wb-muted">{{ executionFor(node.capability, node.capability_version)!.profile }}</p></details>
      </li>
    </ol>
    <p v-if="validation && !validation.origin_revalidated" class="wb-muted plan-note">预览不代表获准执行；启动及执行过程中会复查已支持的来源链、领域参数和当前授权。</p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Capability, PlanSpec, PlanValidation } from '@/api/taskcontrol'
import { assetLabel, capabilityLabel, executionProfileLabel } from './labels'
const props = defineProps<{ plan: PlanSpec; validation?: PlanValidation; capabilities: Capability[] }>()
const orderedNodes = computed(() => {
  // Saved snapshots do not include validation. Reconstruct a stable dependency
  // order without changing the immutable snapshot or inventing execution state.
  const order = [...(props.validation?.topological_order || [])]
  const remaining = props.plan.nodes.filter(node => !order.includes(node.id))
  while (remaining.length) {
    const index = remaining.findIndex(node => (node.depends_on || []).every(id => order.includes(id)))
    if (index < 0) break
    order.push(remaining.splice(index, 1)[0]!.id)
  }
  const indices = new Map(order.map((id, index) => [id, index]))
  return [...props.plan.nodes].sort((a, b) => (indices.get(a.id) ?? order.length) - (indices.get(b.id) ?? order.length))
})
const dependencyLabel = (id: string) => { const node = props.plan.nodes.find(item => item.id === id); return node ? `${capabilityLabel(node.capability)}（${id}）` : id }
const requiresApproval = (id: string, capability: string, version: number, permissions: string[]) => props.validation?.approval_required?.includes(id) || permissions.some(scope => scope === 'write' || scope === 'etl') || props.capabilities.some(item => item.id === capability && item.version === version && item.requires_approval)
const executionFor = (id: string, version: number) => props.capabilities.find(item => item.id === id && item.version === version)?.execution
</script>

<style scoped>
.domain-parameters { display: grid; grid-template-columns: 100px minmax(0,1fr); gap: 8px 12px; padding: 14px; background: var(--bg-tertiary); font-size: 12px; line-height: 1.8; } .domain-parameters dt { color: var(--text-muted); } .domain-parameters dd { margin: 0; overflow-wrap: anywhere; white-space: pre-wrap; }
.plan-limits { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 16px; margin: 0; padding: 18px 0; border-block: 1px solid var(--border-default); }
.plan-limits dt { font-size: 11px; color: var(--text-muted); }.plan-limits dd { margin: 8px 0 0; font-family: var(--font-mono); font-size: 16px; overflow-wrap: anywhere; }
.plan-note { font-size: 12px; line-height: 1.7; }.plan-nodes { list-style: none; padding: 0; margin: 20px 0; }.plan-nodes > li { border-left: 2px solid var(--border-default); padding: 0 0 24px 20px; margin-left: 12px; }.plan-nodes > li:last-child { padding-bottom: 0; }
.node-heading { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }.node-heading h3 { margin: 0; }.node-number { font-family: var(--font-mono); color: var(--primary); font-size: 12px; }.plan-nodes p,.node-inputs { font-size: 12px; line-height: 1.8; overflow-wrap: anywhere; }.plan-nodes strong { font-weight: 500; color: var(--text-secondary); margin-right: 8px; }.node-inputs { padding-left: 16px; color: var(--text-secondary); }summary { font-size: 11px; cursor: pointer; color: var(--text-muted); }
@media(max-width: 680px) { .domain-parameters { display: grid; grid-template-columns: 100px minmax(0,1fr); gap: 8px 12px; padding: 14px; background: var(--bg-tertiary); font-size: 12px; line-height: 1.8; } .domain-parameters dt { color: var(--text-muted); } .domain-parameters dd { margin: 0; overflow-wrap: anywhere; white-space: pre-wrap; }
.plan-limits { grid-template-columns: repeat(2, minmax(0, 1fr)); }.plan-nodes > li { margin-left: 0; padding-left: 12px; } }
</style>
