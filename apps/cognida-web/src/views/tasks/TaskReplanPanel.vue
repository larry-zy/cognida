<template>
  <section v-if="canEdit || draft" class="wb-panel replan-panel" aria-label="调整后续步骤">
    <header><div><span class="wb-kicker">PLAN REVISION / 显式重规划</span><h2>调整后续步骤</h2></div><span class="wb-badge">{{ frozen.size }} 个节点已锁定</span></header>
    <p class="wb-muted">保留已经预留、投递或完成的节点及其真实回执。新版本只影响尚未预留的步骤；累计预算、输入范围与原执行期限不会重置。</p>
    <button v-if="!draft" :disabled="busy || !consistent" @click="begin">编辑未执行步骤</button>
    <template v-else>
      <p v-if="stale" class="wb-error" role="alert">任务已有其他计划版本。请放弃本草稿并基于当前版本重新编辑。</p>
      <p v-if="!canEdit" class="wb-error" role="alert">任务已进入不可重规划的状态，不能提交本草稿。</p>
      <p v-if="error" class="wb-error" role="alert">{{ error }}</p>
      <p v-if="uncertain" class="wb-error" role="alert">提交结果尚未确认；不会自动重发。请核对最新计划版本和节点回执。</p>
      <ol class="replan-nodes" aria-label="待审阅的重规划节点">
        <li v-for="node in draft.nodes" :key="node.id">
          <header><strong>{{ node.id }} · {{ capabilityLabel(node.capability) }}</strong><span class="wb-badge">{{ frozen.has(node.id) ? '已预留 · 保持原样' : '尚未预留' }}</span></header>
          <p class="wb-muted small">前置步骤：{{ node.depends_on?.join('、') || '无' }} · {{ node.inputs.length }} 个固定输入绑定</p>
          <fieldset v-if="!frozen.has(node.id)" :disabled="pending || stale || uncertain || !canEdit">
            <label class="wb-label">处理能力<select :value="node.capability" :aria-label="`${node.id} 处理能力`" @change="changeCapability(node, ($event.target as HTMLSelectElement).value)"><option :value="node.capability">{{ capabilityLabel(node.capability) }}</option><option v-for="cap in alternatives(node)" :key="cap.id" :value="cap.id">{{ capabilityLabel(cap.id) }}</option></select></label>
            <div class="wb-form"><label class="wb-label">单次时限（秒）<input v-model.number="node.timeout_seconds" :aria-label="`${node.id} 单次时限`" type="number" min="1" :max="draft.budget.timeout_seconds" /></label><label class="wb-label">最多尝试<input v-model.number="node.max_attempts" :aria-label="`${node.id} 最多尝试`" type="number" min="1" max="5" /></label></div>
            <button type="button" :disabled="!removable(node.id)" @click="remove(node.id)">移除 {{ node.id }}</button>
          </fieldset>
        </li>
      </ol>
      <p v-if="lockedChanged" class="wb-error" role="alert">编辑期间有节点被预留。它必须恢复原样后才能保存。</p>
      <div class="wb-actions"><button :disabled="pending || stale || uncertain || !canEdit || draft.nodes.length >= draft.budget.max_nodes" @click="add">添加后续步骤</button><button v-if="lockedChanged" :disabled="pending" @click="restoreLocked">恢复已锁定节点</button></div>
      <details class="advanced-replan"><summary>高级编辑：未来节点参数与依赖</summary><p class="wb-muted small">仅修改尚未预留的节点。服务端会重新验证能力、来源、依赖、预算及当前权限；填写参数不会授予执行或审批权限。</p><textarea v-model="advanced" aria-label="计划 JSON" rows="12" :disabled="pending || uncertain" /><button :disabled="pending || uncertain" @click="applyAdvanced">应用到待审阅计划</button></details>
      <label class="wb-label">调整原因<textarea v-model="reason" aria-label="调整原因" rows="2" maxlength="2000" :disabled="pending || uncertain" /></label>
      <details class="replan-preview"><summary>审阅新计划（{{ draft.nodes.length }} 个节点）</summary><TaskPlanPreview :plan="draft" :capabilities="capabilities" /></details>
      <label class="confirm-replan"><input v-model="confirmed" type="checkbox" :disabled="pending || uncertain" />我已审阅后续步骤，确认保存为新的执行计划版本</label>
      <div class="wb-actions"><button class="primary" :disabled="!submittable" @click="save">{{ pending ? '正在保存新版本…' : '确认重规划' }}</button><button :disabled="pending" @click="discard">放弃本次调整</button><button :disabled="pending" @click="$emit('refresh')">核对最新状态</button></div>
    </template>
  </section>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { taskControlApi, type Capability, type GovernedTask, type PlanNode, type PlanSpec, type SavedPlan, type TaskSummary } from '@/api/taskcontrol'
import TaskPlanPreview from './TaskPlanPreview.vue'
import { capabilityLabel } from './labels'
import { isRenderablePlan } from './planShape'
const props = defineProps<{ task: GovernedTask; savedPlan: SavedPlan; summary?: TaskSummary; capabilities: Capability[]; scope: string; busy: boolean }>()
const emit = defineEmits<{ dirty: [boolean]; busy: [boolean]; refresh: []; committed: []; inaccessible: [] }>()
const draft = ref<PlanSpec>(), reason = ref(''), advanced = ref(''), confirmed = ref(false), error = ref(''), pending = ref(false), uncertain = ref(false), baseVersion = ref(0)
let epoch = 0, request: AbortController | undefined
const copy = <T,>(value: T): T => JSON.parse(JSON.stringify(value))
const frozen = computed(() => new Set((props.summary?.children || []).map(child => child.node_id)))
const canEdit = computed(() => Boolean(props.task.execution_plan_version && ['running', 'reconcile_required'].includes(props.task.status)))
const consistent = computed(() => props.summary?.task_id === props.task.id && props.summary.task_version === props.task.version && props.summary.current_plan_version === props.savedPlan.version && props.savedPlan.version === props.task.current_plan_version)
const stale = computed(() => Boolean(draft.value && baseVersion.value !== props.savedPlan.version))
const lockedChanged = computed(() => Boolean(draft.value && [...frozen.value].some(id => JSON.stringify(draft.value?.nodes.find(node => node.id === id)) !== JSON.stringify(props.savedPlan.snapshot.nodes.find(node => node.id === id)))))
const submittable = computed(() => Boolean(draft.value && canEdit.value && consistent.value && !stale.value && !lockedChanged.value && !pending.value && !props.busy && !uncertain.value && confirmed.value && reason.value.trim() && JSON.stringify(draft.value) !== JSON.stringify(props.savedPlan.snapshot)))
function begin() { if (!canEdit.value || !consistent.value || props.busy) return; draft.value = copy(props.savedPlan.snapshot); baseVersion.value = props.savedPlan.version; advanced.value = JSON.stringify(draft.value, null, 2); reason.value = ''; confirmed.value = false; error.value = ''; emit('dirty', true) }
function discard() { if (pending.value) return; draft.value = undefined; uncertain.value = false; error.value = ''; confirmed.value = false; emit('dirty', false) }
function alternatives(node: PlanNode) { return props.capabilities.filter(cap => cap.execution && !cap.requires_approval && !cap.parameter_kind && cap.id !== node.capability && node.inputs.every(input => cap.input_kinds.includes(input.kind)) && (!cap.required_any_input_kinds?.length || node.inputs.some(input => cap.required_any_input_kinds?.includes(input.kind)))) }
function changeCapability(node: PlanNode, id: string) { const cap = alternatives(node).find(cap => cap.id === id); if (!cap?.execution) return; node.capability = cap.id; node.capability_version = cap.version; const passthrough = cap.first_input_passthrough_kinds || []; const first = node.inputs.find(input => passthrough.includes(input.kind))?.kind; node.output_kinds = cap.output_kinds.filter(kind => !passthrough.includes(kind) || kind === first); node.tools = [...cap.tools]; node.permissions = [...cap.permissions]; delete node.parameters; node.max_calls_per_attempt = cap.execution.base_calls + cap.execution.calls_per_input * node.inputs.length; node.timeout_seconds = Math.min(node.timeout_seconds, cap.execution.max_timeout_seconds); confirmed.value = false; syncAdvanced() }
function removable(id: string) { return draft.value && draft.value.nodes.length > 1 && !frozen.value.has(id) && !draft.value.nodes.some(node => node.depends_on?.includes(id) || node.inputs.some(input => input.from_node === id)) }
function remove(id: string) { if (removable(id)) { draft.value!.nodes = draft.value!.nodes.filter(node => node.id !== id); syncAdvanced() } }
function add() { const plan = draft.value; if (!plan || plan.nodes.length >= plan.budget.max_nodes) return; const original = plan.nodes.find(node => node.inputs.every(input => input.asset)); if (!original) { error.value = '此计划没有可复用的固定根输入步骤，可在高级编辑中明确绑定已有节点产物。'; return }; const node = copy(original); let index = 1; while (plan.nodes.some(item => item.id === `next_${index}`)) index++; node.id = `next_${index}`; node.depends_on = [plan.nodes.at(-1)!.id]; plan.nodes.push(node); syncAdvanced() }
function restoreLocked() { if (!draft.value) return; for (const id of frozen.value) { const source = props.savedPlan.snapshot.nodes.find(node => node.id === id); if (!source) continue; const index = draft.value.nodes.findIndex(node => node.id === id); if (index < 0) draft.value.nodes.unshift(copy(source)); else draft.value.nodes[index] = copy(source) }; syncAdvanced() }
function syncAdvanced() { advanced.value = JSON.stringify(draft.value, null, 2); confirmed.value = false }
function applyAdvanced() { try { const value: unknown = JSON.parse(advanced.value); if (!isRenderablePlan(value)) throw new Error('计划结构不完整'); if (JSON.stringify(value.budget) !== JSON.stringify(props.savedPlan.snapshot.budget)) throw new Error('运行中累计预算必须保持原值'); draft.value = value; confirmed.value = false; error.value = '' } catch (cause) { error.value = cause instanceof Error ? cause.message : '计划 JSON 不合法' } }
async function save() {
  if (!submittable.value || !draft.value) return
  const generation = epoch, id = props.task.id, scope = props.scope, version = props.task.version, expectedPlan = props.savedPlan.version, submitted = copy(draft.value)
  const abort = new AbortController(); request = abort; pending.value = true; emit('busy', true); error.value = ''
  const live = () => epoch === generation && props.task.id === id && props.scope === scope && !abort.signal.aborted
  try {
    const response = await taskControlApi.replan(id, version, submitted, reason.value.trim(), abort.signal)
    if (!live()) return
    if (response.data?.plan.task_id !== id || response.data.plan.version !== expectedPlan + 1) throw new Error('返回的计划版本不匹配，请核对最新状态')
    draft.value = undefined; emit('dirty', false); emit('committed')
  } catch (cause) {
    if (!live()) return
    const response = (cause as { response?: { status?: number; data?: { message?: string; issues?: { message: string }[] } } })?.response
    if ([401, 403, 404].includes(response?.status || 0)) { draft.value = undefined; emit('dirty', false); emit('inaccessible'); return }
    error.value = response?.data?.issues?.map(issue => issue.message).join('；') || response?.data?.message || (cause instanceof Error ? cause.message : '保存失败')
    uncertain.value = !response?.status || response.status >= 500
    confirmed.value = false
  } finally { if (live()) { pending.value = false; emit('busy', false); request = undefined; emit('refresh') } }
}
watch([() => props.scope, () => props.task.id], () => { epoch++; request?.abort(); request = undefined; pending.value = false; draft.value = undefined; uncertain.value = false; error.value = ''; emit('dirty', false); emit('busy', false) })
watch(draft, () => { confirmed.value = false }, { deep: true })
onBeforeUnmount(() => { epoch++; request?.abort(); emit('dirty', false); emit('busy', false) })
</script>
<style scoped>
.replan-panel header{display:flex;align-items:center;justify-content:space-between;gap:16px;flex-wrap:wrap}.replan-panel h2{margin:8px 0}.replan-panel p,.replan-panel summary{font-size:12px;line-height:1.8}.replan-nodes{list-style:none;padding:0;margin:22px 0}.replan-nodes>li{border-top:1px solid var(--border-default);padding:18px 0}.replan-nodes fieldset{border:0;padding:0;display:grid;gap:12px}.replan-nodes strong{font-size:13px}.advanced-replan,.replan-preview{padding:16px 0;border-top:1px solid var(--border-default)}summary{cursor:pointer}.advanced-replan textarea{width:100%;box-sizing:border-box;font-family:monospace;font-size:12px;margin:12px 0}.confirm-replan{display:flex;align-items:flex-start;gap:10px;margin:18px 0;font-size:12px;line-height:1.7}.small{font-size:11px}
</style>
