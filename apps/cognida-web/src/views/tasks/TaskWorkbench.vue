<template>
  <div class="workbench task-workbench">
    <header class="wb-head"><div><span class="wb-kicker">SUPERVISOR / 任务规划</span><h1>先明确目标，再审阅计划</h1><p>选择输入版本，把目标拆成有依赖、有预算、可核对的处理步骤。</p></div><button :disabled="busy" @click="newDraft">＋ 新建任务</button></header>
    <div class="planning-boundary"><span class="wb-badge">受控执行</span><p>保存只追加计划版本。执行需通过当前准入检查，再单独启动；候选与已保存计划均不代表已执行。</p></div>
    <p v-if="!hasIdentity" class="wb-empty" role="status">请登录并选择工作空间后规划任务。</p>
    <template v-else>
      <div v-if="error" class="wb-error" role="alert">{{ error }}<ul v-if="issues.length"><li v-for="(issue, index) in issues" :key="index">{{ issue.node_id ? `${issue.node_id} · ` : '' }}{{ issue.field }}：{{ issue.message }}</li></ul></div>
      <p v-if="notice" class="wb-success" role="status">{{ notice }}</p>
      <p v-if="busy" class="wb-muted" role="status">{{ action === 'assess' ? '正在核对输入与任务范围…' : action === 'save' ? '正在保存计划版本…' : runtime.action ? '正在提交执行控制请求…' : '正在生成候选计划，请稍候…' }}</p>
      <div class="task-layout">
        <aside class="wb-panel task-history" aria-labelledby="tasks-title">
          <div class="panel-heading"><h2 id="tasks-title">我的任务</h2><button :disabled="listLoading || busy" @click="loadTasks(false)">刷新</button></div>
          <p class="wb-muted small">当前账号 · 当前工作空间</p>
          <p v-if="listError" class="wb-error" role="alert">{{ listError }}<button @click="loadTasks(false)">重试任务列表</button></p>
          <p v-if="listLoading" class="wb-muted" role="status">正在读取任务…</p>
          <p v-else-if="!tasks.length && !listError" class="wb-empty">还没有任务。先填写一个明确目标。</p>
          <ul class="task-list" aria-label="已保存任务"><li v-for="item in tasks" :key="item.id"><button :disabled="busy" :class="{ selected: current?.id === item.id }" @click="selectTask(item.id)"><strong>{{ item.goal }}</strong><span>{{ statusLabel(item.status) }} · {{ item.current_plan_version ? `计划 v${item.current_plan_version}` : '尚无已保存计划' }}</span><time>{{ formatTime(item.updated_at) }}</time></button></li></ul>
          <button v-if="nextCursor" :disabled="listLoading || busy" @click="loadTasks(true)">加载更多任务</button>
        </aside>

        <div class="wb-stack task-main">
          <p v-if="detailLoading" class="wb-panel wb-muted" role="status">正在读取任务与已保存计划…</p>
          <div v-if="detailError" class="wb-panel"><p class="wb-error" role="alert">{{ detailError }}</p><button @click="openTask(detailID)">重试任务详情</button></div>
          <form v-if="!current && !detailLoading && !detailError" class="wb-panel wb-stack" aria-label="新建规划任务" @submit.prevent="beginPlanning">
            <div class="section-intro"><span class="step-number">01</span><div><h2>目标与输入</h2><p class="wb-muted small">描述需要的产物与判断标准。输入固定到具体版本，稍后由后端复查访问权限。</p></div></div>
            <fieldset :disabled="busy" class="draft-fields">
              <label class="wb-label">任务目标<textarea v-model="goal" aria-label="任务目标" rows="4" maxlength="8000" placeholder="例如：检查这份销售数据的质量，清理空白和重复记录，生成质量报告。" /></label>
              <p class="wb-muted small">{{ Array.from(goal).length }} / 4000 字符</p>
              <label class="wb-label">可选资产<select v-model="assetChoice" aria-label="可选资产"><option value="">选择输入资产版本</option><option v-for="asset in assets" :key="refKey(asset)" :value="refKey(asset)" :disabled="asset.status !== 'available' || selectedRefs.some(item => refKey(item) === refKey(asset))">{{ asset.name || asset.asset_id }} · {{ assetLabel(asset.kind) }} · v{{ asset.version }}{{ asset.status !== 'available' ? '（不可用）' : '' }}</option></select></label>
              <div class="wb-actions"><button type="button" :disabled="!assetChoice || selectedRefs.length >= 64" @click="addInput">添加输入</button><button v-if="assetOptions.state.hasMore || assetOptions.state.error" type="button" :disabled="assetOptions.state.loading" @click="moreAssets">{{ assetOptions.state.error ? '重试资产列表' : '加载更多资产' }}</button><span v-if="assetOptions.state.loading" role="status" class="wb-muted">正在读取资产…</span></div>
              <p v-if="assetOptions.state.error" class="wb-error" role="alert">{{ assetOptions.state.error }}</p>
              <p v-if="!assets.length && !assetOptions.state.loading && !assetOptions.state.error" class="wb-muted small">暂无可选资产。可先在<router-link to="/pipelines">处理与发布</router-link>创建输入，再返回规划。</p>
              <ul class="selected-inputs" aria-label="已选择输入"><li v-for="input in selectedRefs" :key="refKey(input)"><span>{{ inputName(input) }} <small>v{{ input.version }}</small></span><button type="button" :aria-label="`移除输入 ${inputName(input)}`" @click="selectedRefs = selectedRefs.filter(item => refKey(item) !== refKey(input))">移除</button></li></ul>
              <div class="budget-heading"><h3>显式预算上限</h3><span class="wb-muted small">包含重试；不会自动提高</span></div>
              <div class="wb-form"><label v-for="field in budgetFields" :key="field.key" class="wb-label">{{ field.label }}<input v-model.number="budget[field.key]" :aria-label="field.label" type="number" step="1" :min="field.min" :max="field.max" /></label></div>
              <p class="wb-muted small">成本额度以百万分之一计价单位表示，仅作为计划上限；不是实际费用。最多 32 个节点、8 个并发，每节点最多 5 次尝试。</p>
            </fieldset>
            <div v-if="assessmentQuestions.length" class="clarification-note" role="status"><h3>还需补充</h3><ul><li v-for="question in assessmentQuestions" :key="question.id">{{ question.question }}</li></ul><p>请在上方修改目标、输入或预算，再核对一次。</p></div>
            <button class="primary" :disabled="busy">核对并生成候选计划</button>
          </form>

          <template v-if="current && !detailLoading">
            <section class="wb-panel" aria-labelledby="task-detail-title">
              <div class="panel-heading"><h2 id="task-detail-title">任务详情</h2><span class="wb-badge">{{ statusLabel(current.status) }}</span></div>
              <h3 class="task-goal">{{ current.goal }}</h3><p v-if="current.reason" class="task-reason">{{ current.reason }}</p>
              <p class="wb-muted small">任务版本 {{ current.version }} · {{ current.current_plan_version ? `已保存计划 v${current.current_plan_version}` : '尚无已保存计划' }} · {{ formatTime(current.updated_at) }}</p>
              <ul class="task-input-refs" aria-label="任务固定输入"><li v-for="input in current.input_refs" :key="refKey(input)"><router-link :to="{ path: '/analysis', query: { asset: refKey(input) } }">{{ inputName(input) }} · v{{ input.version }} ↗</router-link></li></ul>
              <details><summary>任务标识与初始预算</summary><p class="wb-muted small">{{ current.id }}</p><dl class="budget-details"><div v-for="field in budgetFields" :key="field.key"><dt>{{ field.label }}</dt><dd>{{ current.budget[field.key] }}</dd></div></dl></details>
              <div class="wb-actions"><button :disabled="busy" @click="refreshTask">刷新任务状态</button><router-link :to="{ path: '/tasks', query: { task_id: current.id } }">打开此任务链接 ↗</router-link></div>
            </section>

            <section v-if="proposal" class="wb-panel proposal-panel" aria-labelledby="proposal-title">
              <div class="panel-heading"><div><span class="wb-kicker">02 / CANDIDATE</span><h2 id="proposal-title">{{ proposal.decision.kind === 'clarify' ? '补充目标信息' : proposal.decision.kind === 'unsupported' ? '当前能力无法形成计划' : '候选计划' }}</h2></div><span class="wb-badge">尚未保存</span></div>
              <p class="proposal-explanation">{{ proposal.decision.explanation }}</p>
              <details v-if="answerHistory.length"><summary>已补充 {{ answerHistory.length }} 项信息</summary><ul class="answer-history"><li v-for="answer in answerHistory" :key="answer.question_id"><strong>{{ answer.question }}</strong><p>{{ answer.answer }}</p></li></ul></details>
              <form v-if="proposal.decision.kind === 'clarify'" class="wb-stack" aria-label="补充规划信息" @submit.prevent="continuePlanning">
                <label v-for="question in proposal.decision.questions || []" :key="question.id" class="wb-label">{{ question.question }}<select v-if="question.options?.length" v-model="answers[question.id]" :disabled="busy" :aria-label="question.question"><option value="">请选择</option><option v-for="option in question.options" :key="option" :value="option">{{ option }}</option></select><textarea v-else v-model="answers[question.id]" :disabled="busy" :aria-label="question.question" rows="3" maxlength="8000" /></label>
                <button class="primary" :disabled="busy">补充并生成计划</button>
              </form>
              <template v-if="proposal.decision.kind === 'proposal' && proposal.decision.plan">
                <TaskPlanPreview :plan="proposal.decision.plan" :validation="proposal.validation" :capabilities="capabilities" />
                <p class="wb-muted small">候选仅在本页预览。保存会追加不可变计划版本，刷新页面前未保存的候选会丢失。</p><p v-if="proposal.task_version !== current.version" class="wb-error" role="alert">任务版本已变化，此候选已过期，不能保存。请核对当前任务后重新生成。</p>
                <button class="primary" :disabled="busy || saveUncertain || runtime.uncertain || !proposal.validation?.valid || proposal.task_version !== current.version" @click="savePlan">保存计划</button>
                <p v-if="saveUncertain" class="wb-error" role="alert">保存结果尚未确认。请先刷新任务状态，核对已保存版本后再生成候选。</p>
              </template>
              <p class="wb-muted small">本次规划调用 {{ proposal.planning_calls }} 次 · 未执行领域任务</p>
            </section>

            <section v-if="canPropose" class="wb-panel planning-actions"><p>{{ proposal ? '需要换一个处理方案？可以重新请求候选，已有保存版本不会被覆盖。' : '输入和预算已固定。生成候选后，请核对步骤、权限及审批要求。' }}</p><button :disabled="busy || saveUncertain" @click="regenerate">{{ proposal ? '重新生成候选' : '生成候选计划' }}</button><p class="wb-muted small">修改原目标、输入或预算，请新建任务。每次生成都会发起一次有界规划调用。</p></section>

            <section v-if="savedPlan" class="wb-panel" aria-labelledby="saved-plan-title"><div class="panel-heading"><div><span class="wb-kicker">03 / SAVED PLAN</span><h2 id="saved-plan-title">已保存计划 v{{ savedPlan.version }}</h2></div><span class="wb-badge">已持久化</span></div><TaskPlanPreview :plan="savedPlan.snapshot" :capabilities="capabilities" /><details><summary>快照摘要</summary><p class="snapshot-hash">{{ savedPlan.content_hash }}</p></details></section>

            <TaskExecutionPanel :task="current" :summary="summary" :saved-plan="savedPlan" :readiness="runtime.readiness" :error="runtime.error || summaryError" :readiness-error="runtime.readinessError" :action-error="runtime.actionError" :loading="runtime.loading" :busy="busy" :action="runtime.action" :uncertain="runtime.uncertain" :candidate-pending="Boolean(proposal)" :can-start="runtime.canStart" :can-cancel="runtime.canCancel" :updated-at="runtime.updatedAt" @start="runtime.start" @cancel="runtime.cancel" @refresh="runtime.refresh" />
          </template>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useCursorOptions } from '@/composables/useCursorOptions'
import { lifecycleApi, type AssetRef, type RefID } from '@/api/lifecycle'
import { taskControlApi, type Answer, type Capability, type Clarification, type DraftInput, type GovernedTask, type Proposal, type SavedPlan, type TaskBudget, type TaskSummary } from '@/api/taskcontrol'
import TaskPlanPreview from './TaskPlanPreview.vue'
import TaskExecutionPanel from './TaskExecutionPanel.vue'
import { useTaskRuntime } from './useTaskRuntime'
import { assetLabel, statusLabel } from './labels'
import '@/components/workbench/workbench.css'

const auth = useAuthStore(), route = useRoute(), router = useRouter()
const hasIdentity = computed(() => Boolean(auth.user?.id && auth.currentTenant?.id))
const goal = ref(''), assetChoice = ref(''), selectedRefs = ref<RefID[]>([]), assets = ref<AssetRef[]>([])
const defaultBudget = (): TaskBudget => ({ max_nodes: 8, max_concurrency: 2, max_attempts: 16, max_calls: 64, max_cost_micros: 1000000, timeout_seconds: 600 })
const budget = reactive(defaultBudget())
const budgetFields: { key: keyof TaskBudget; label: string; min: number; max: number }[] = [
  { key: 'max_nodes', label: '最多节点数', min: 1, max: 32 }, { key: 'max_concurrency', label: '最多并发数', min: 1, max: 8 },
  { key: 'max_attempts', label: '总尝试次数', min: 1, max: 160 }, { key: 'max_calls', label: '总调用次数', min: 1, max: 4096 },
  { key: 'max_cost_micros', label: '成本额度', min: 0, max: 1000000000 }, { key: 'timeout_seconds', label: '总时限（秒）', min: 1, max: 86400 },
]
const tasks = ref<GovernedTask[]>([]), capabilities = ref<Capability[]>([]), current = ref<GovernedTask>(), summary = ref<TaskSummary>(), savedPlan = ref<SavedPlan>(), proposal = ref<Proposal>()
const nextCursor = ref(''), listLoading = ref(false), listError = ref(''), detailLoading = ref(false), detailError = ref(''), detailID = ref(''), summaryError = ref('')
const error = ref(''), notice = ref(''), action = ref(''), saveUncertain = ref(false), assessmentQuestions = ref<Clarification[]>([])
const answers = reactive<Record<string, string>>(Object.create(null)), issues = ref<{ node_id?: string; field: string; message: string }[]>([])
const answerHistory = ref<Answer[]>([])
let proposalRound = 0
const runtime = reactive(useTaskRuntime({
  task: current, summary, savedPlan, proposal,
  scope: computed(() => `${auth.authEpoch}:${auth.user?.id || ''}:${auth.currentTenant?.id || ''}`),
  planningBusy: computed(() => Boolean(action.value)),
  onInaccessible: (id, message) => { clearDetail(); detailID.value = id; detailError.value = message },
  onRefreshed: () => { summaryError.value = '' },
}))
const busy = computed(() => Boolean(action.value) || runtime.busy)
const canPropose = computed(() => current.value && ['draft', 'ready'].includes(current.value.status) && !summary.value?.children?.length && !summaryError.value && !runtime.uncertain && !runtime.error)
const hasUnsaved = computed(() => Boolean(proposal.value || (!current.value && (goal.value.trim() || selectedRefs.value.length))))
let epoch = 0, detailSequence = 0, actionSequence = 0, listSequence = 0, disposed = false
const controllers = new Set<AbortController>()
function controller() { const result = new AbortController(); controllers.add(result); return result }
const live = (generation: number) => !disposed && generation === epoch
const refKey = (item: RefID) => `${item.asset_id}:${item.version}`
const inputName = (item: RefID) => assets.value.find(asset => refKey(asset) === refKey(item))?.name || item.asset_id
const formatTime = (value: string) => { const date = new Date(value); return Number.isNaN(date.getTime()) ? '时间未提供' : date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) }
const assetOptions = useCursorOptions(assets, cursor => hasIdentity.value ? lifecycleApi.assets('', cursor) : Promise.resolve({ data: { items: [], next_cursor: '' } }))
function requireData<T>(response: { data?: T }): T { if (response.data == null) throw new Error('服务未返回有效内容，请重试'); return response.data }
function failure(cause: unknown) {
  const response = (cause as { response?: { status?: number; data?: { message?: string; issues?: typeof issues.value } } })?.response
  issues.value = response?.data?.issues || []
  if (response?.status === 403) return '任务或所选输入当前不可访问，请核对工作空间及资产权限。'
  if (response?.status === 404) return '任务不存在或当前账号无权查看。'
  if (response?.status === 409) return '任务版本或状态已变化，请刷新任务后重新生成候选。'
  return response?.data?.message || (cause instanceof Error ? cause.message : '读取失败，请重试')
}
function clearProposal() { proposal.value = undefined; saveUncertain.value = false; Object.keys(answers).forEach(key => delete answers[key]) }
function clearDetail() { runtime.reset(); detailSequence++; actionSequence++; action.value = ''; current.value = undefined; savedPlan.value = undefined; summary.value = undefined; detailID.value = ''; detailLoading.value = false; detailError.value = ''; summaryError.value = ''; clearProposal(); answerHistory.value = []; proposalRound = 0 }
function resetDraft() { goal.value = ''; assetChoice.value = ''; selectedRefs.value = []; Object.assign(budget, defaultBudget()); assessmentQuestions.value = [] }
function confirmDiscard() { return !hasUnsaved.value || window.confirm('未保存的目标或候选计划将丢失，仍要离开吗？') }
function newDraft() {
  if (busy.value) return
  if (route.query.task_id) { void router.push({ path: '/tasks' }); return }
  if (!confirmDiscard()) return
  clearDetail(); resetDraft(); error.value = ''; notice.value = ''; issues.value = []
}
function addInput() { const asset = assets.value.find(item => refKey(item) === assetChoice.value); if (asset && asset.status === 'available' && selectedRefs.value.length < 64 && !selectedRefs.value.some(item => refKey(item) === refKey(asset))) selectedRefs.value.push({ asset_id: asset.asset_id, version: asset.version }); assetChoice.value = '' }
async function moreAssets() { try { await assetOptions.loadMore() } catch { /* local pagination error remains visible */ } }
async function loadTasks(append: boolean) {
  if (!hasIdentity.value || listLoading.value) return
  const generation = epoch, request = ++listSequence, abort = controller(), cursor = append ? nextCursor.value : ''
  listLoading.value = true; listError.value = ''
  try {
    const page = requireData(await taskControlApi.list(cursor, abort.signal))
    if (!live(generation) || request !== listSequence) return
    if (!Array.isArray(page.items) || (page.next_cursor && page.next_cursor === cursor)) throw new Error('任务分页返回异常，请刷新重试')
    tasks.value = append ? [...tasks.value, ...page.items.filter(item => !tasks.value.some(old => old.id === item.id))] : page.items
    nextCursor.value = page.next_cursor || ''
  } catch (cause) { if (live(generation) && request === listSequence) listError.value = failure(cause) }
  finally { controllers.delete(abort); if (live(generation) && request === listSequence) listLoading.value = false }
}
async function openTask(id: string) {
  if (!id || !hasIdentity.value) return
  clearDetail(); error.value = ''; issues.value = []; detailID.value = id; detailLoading.value = true
  const generation = epoch, request = detailSequence, abort = controller()
  const active = () => live(generation) && request === detailSequence
  try {
    const task = requireData(await taskControlApi.get(id, abort.signal))
    if (!active()) return
    if (task.id !== id) throw new Error('任务身份不匹配，请重新打开任务。')
    current.value = task; resetDraft()
    const [status, plan] = await Promise.allSettled([
      taskControlApi.summary(id, abort.signal),
      task.current_plan_version ? taskControlApi.plan(id, task.current_plan_version, abort.signal) : Promise.resolve(undefined),
    ])
    if (!active()) return
    const denied = [status, plan].find(result => result.status === 'rejected' && [401, 403, 404].includes((result.reason as { response?: { status?: number } })?.response?.status || 0))
    if (denied?.status === 'rejected') {
      const message = failure(denied.reason)
      clearDetail(); detailID.value = id; detailError.value = message
      return
    }
    if (status.status === 'fulfilled') {
      const result = requireData(status.value)
      if (result.task_id === task.id && result.task_version === task.version && result.current_plan_version === task.current_plan_version) summary.value = result
      else summaryError.value = '任务在读取期间发生变化，请刷新任务状态。'
    }
    else summaryError.value = failure(status.reason)
    if (plan.status === 'fulfilled' && plan.value) {
      const snapshot = requireData(plan.value)
      if (snapshot.task_id !== id || snapshot.version !== task.current_plan_version) throw new Error('已保存计划版本不匹配，请重新读取任务。')
      savedPlan.value = snapshot
    }
    else if (plan.status === 'rejected') error.value = `已保存计划读取失败：${failure(plan.reason)}`
  } catch (cause) { if (active()) { current.value = undefined; detailError.value = failure(cause) } }
  finally { controllers.delete(abort); if (active()) { detailLoading.value = false; void runtime.initialize() } }
}
function selectTask(id: string) { if (!busy.value) void router.push({ path: '/tasks', query: { task_id: id } }) }
function refreshTask() {
  if (busy.value || !current.value) return
  if (saveUncertain.value) { if (confirmDiscard()) { notice.value = ''; void openTask(current.value.id) } }
  else void runtime.refresh()
}
function draftInput(): DraftInput {
  if (Array.from(goal.value.trim()).length > 4000 || goal.value.includes('\0')) throw new Error('任务目标最多 4000 字符，且不能包含空字符。')
  for (const field of budgetFields) if (!Number.isSafeInteger(budget[field.key]) || budget[field.key] < field.min || budget[field.key] > field.max) throw new Error(`${field.label}必须是 ${field.min}–${field.max} 之间的整数。`)
  if (budget.max_concurrency > budget.max_nodes) throw new Error('最多并发数不能超过最多节点数。')
  return { goal: goal.value.trim(), input_refs: selectedRefs.value.map(({ asset_id, version }) => ({ asset_id, version })), budget: { ...budget } }
}
async function receiveProposal(task: GovernedTask, submittedAnswers: Answer[], generation: number, sequence: number, signal: AbortSignal) {
  const result = requireData(await taskControlApi.propose(task.id, task.version, submittedAnswers, signal))
  if (!live(generation) || sequence !== actionSequence || current.value?.id !== task.id) return
  if (result.task_id !== task.id || result.task_version !== task.version || result.execution_enabled !== false || result.saved !== false || !['clarify', 'proposal', 'unsupported'].includes(result.decision?.kind)) throw new Error('候选计划的任务身份或状态不匹配，请刷新任务。')
  if (result.decision.kind === 'proposal' && (!result.decision.plan || !result.validation?.valid)) throw new Error('候选计划未通过后端预检，不能保存。')
  clearProposal(); proposal.value = result; answerHistory.value = submittedAnswers; proposalRound++
}
async function beginPlanning() {
  if (busy.value || !hasIdentity.value) return
  const generation = epoch, sequence = ++actionSequence, abort = controller()
  const active = () => live(generation) && sequence === actionSequence
  error.value = ''; notice.value = ''; issues.value = []; assessmentQuestions.value = []; action.value = 'assess'
  try {
    const input = draftInput()
    const assessment = requireData(await taskControlApi.assess(input, abort.signal))
    if (!active()) return
    if (assessment.execution_enabled !== false) throw new Error('规划服务状态不匹配，请刷新后重试。')
    if (!assessment.ready) { assessmentQuestions.value = assessment.questions || []; return }
    const task = requireData(await taskControlApi.create(input, abort.signal))
    if (!active()) return
    current.value = task; detailID.value = task.id; action.value = 'propose'; notice.value = '任务草稿已保存。候选计划生成后仍需单独保存。'
    void loadTasks(false)
    await receiveProposal(task, [], generation, sequence, abort.signal)
    if (!active()) return
    try { const result = requireData(await taskControlApi.summary(task.id, abort.signal)); if (active()) summary.value = result } catch (cause) { if (active()) summaryError.value = failure(cause) }
  } catch (cause) { if (active()) error.value = failure(cause) }
  finally { controllers.delete(abort); if (active()) action.value = '' }
}
async function proposeAgain(submittedAnswers: Answer[]) {
  if (busy.value || !current.value || !hasIdentity.value || !canPropose.value) return
  const task = current.value, generation = epoch, sequence = ++actionSequence, abort = controller()
  action.value = 'propose'; error.value = ''; issues.value = []; notice.value = ''
  try { await receiveProposal(task, submittedAnswers, generation, sequence, abort.signal) }
  catch (cause) { if (live(generation) && sequence === actionSequence) error.value = failure(cause) }
  finally { controllers.delete(abort); if (live(generation) && sequence === actionSequence) action.value = '' }
}
function continuePlanning() {
  const questions = proposal.value?.decision.questions || []
  const used = new Set(answerHistory.value.map(answer => answer.question_id))
  const submitted = questions.map(question => {
    let id = question.id, suffix = 0
    while (used.has(id)) id = `r${proposalRound + 1}.${question.id.slice(0, 48)}.${suffix++}`
    used.add(id)
    return { question_id: id, question: question.question, answer: (answers[question.id] || '').trim() }
  })
  if (!submitted.length || submitted.some(answer => !answer.answer || Array.from(answer.answer).length > 4000 || answer.answer.includes('\0'))) { error.value = '请完整回答澄清问题，每项最多 4000 字符。'; return }
  if (answerHistory.value.length + submitted.length > 12) { error.value = '本次任务最多保留 12 项澄清回答，请把已确认信息整理进目标后新建任务。'; return }
  void proposeAgain([...answerHistory.value, ...submitted])
}
function regenerate() { if (proposal.value && !window.confirm('重新生成会替换本页未保存的候选，并发起一次新的规划调用。继续吗？')) return; void proposeAgain([...answerHistory.value]) }
async function savePlan() {
  const candidate = proposal.value, task = current.value
  if (busy.value || saveUncertain.value || runtime.uncertain || !task || candidate?.decision.kind !== 'proposal' || !candidate.decision.plan || !candidate.validation?.valid || candidate.task_version !== task.version) return
  const generation = epoch, sequence = ++actionSequence, abort = controller()
  action.value = 'save'; error.value = ''; issues.value = []
  try {
    requireData(await taskControlApi.appendPlan(task.id, task.version, candidate.decision.plan, abort.signal))
    if (!live(generation) || sequence !== actionSequence) return
    clearProposal(); notice.value = '计划已保存。通过执行准入后仍需单独启动，尚未执行任务。'
    // The platform remounts pages on query navigation. A new instance reads the
    // saved version; hosts that retain this instance still need a fresh GET.
    if (route.query.task_id !== task.id) {
      action.value = ''; await router.replace({ path: '/tasks', query: { task_id: task.id } }); await nextTick()
    }
    if (live(generation)) { await openTask(task.id); if (live(generation)) void loadTasks(false) }
  } catch (cause) { if (live(generation) && sequence === actionSequence) { error.value = failure(cause); saveUncertain.value = true } }
  finally { controllers.delete(abort); if (live(generation) && sequence === actionSequence) action.value = '' }
}
function warnBeforeUnload(event: BeforeUnloadEvent) { if (hasUnsaved.value || busy.value) { event.preventDefault(); event.returnValue = '' } }
const allowNavigation = () => !(hasUnsaved.value || busy.value) || window.confirm(busy.value ? '请求尚未结束，离开后请从任务列表核对保存结果。仍要离开吗？' : '未保存的目标或候选计划将丢失，仍要离开吗？')
onBeforeRouteLeave(allowNavigation)
onBeforeRouteUpdate(allowNavigation)
watch(() => [auth.authEpoch, auth.user?.id, auth.currentTenant?.id], () => {
  epoch++; controllers.forEach(item => item.abort()); controllers.clear(); clearDetail(); resetDraft(); tasks.value = []; nextCursor.value = ''; listSequence++; listLoading.value = false; listError.value = ''; capabilities.value = []; error.value = ''; notice.value = ''; issues.value = []; assets.value = []
  void assetOptions.reset().catch(() => {})
  if (!hasIdentity.value) return
  const generation = epoch, abort = controller()
  void loadTasks(false)
  void taskControlApi.capabilities(abort.signal).then(result => { if (live(generation)) capabilities.value = requireData(result).capabilities }).catch(cause => { if (live(generation)) error.value = failure(cause) }).finally(() => controllers.delete(abort))
  if (typeof route.query.task_id === 'string' && route.query.task_id.trim()) void openTask(route.query.task_id.trim())
}, { immediate: true })
watch(() => route.query.task_id, value => { if (typeof value === 'string' && value.trim()) { if (value.trim() !== current.value?.id) void openTask(value.trim()) } else { clearDetail(); resetDraft(); error.value = ''; notice.value = '' } })
onMounted(() => window.addEventListener('beforeunload', warnBeforeUnload))
onBeforeUnmount(() => { disposed = true; epoch++; controllers.forEach(item => item.abort()); controllers.clear(); window.removeEventListener('beforeunload', warnBeforeUnload) })
</script>

<style scoped>
.task-workbench { max-width: 1440px; }.task-workbench h1 { font-size: 30px; }.planning-boundary { display: flex; align-items: center; gap: 10px; padding: 14px 18px; margin-bottom: 24px; background: var(--primary-dim); border-left: 3px solid var(--primary); }.planning-boundary p { margin: 0; font-size: 12px; line-height: 1.7; }
.task-layout { display: grid; grid-template-columns: 260px minmax(0, 1fr); gap: 24px; }.task-main { min-width: 0; }.task-history { align-self: start; }.panel-heading { display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-bottom: 16px; }.panel-heading h2 { margin: 0; }.panel-heading .wb-kicker { display: block; margin-bottom: 8px; }.small { font-size: 12px; line-height: 1.7; }.task-list { list-style: none; padding: 0; margin: 16px 0; }.task-list li + li { margin-top: 8px; }.task-list button { width: 100%; text-align: left; padding: 12px; }.task-list button.selected { border-color: var(--primary); background: var(--primary-dim); }.task-list strong { display: block; line-height: 1.6; overflow-wrap: anywhere; font-size: 13px; }.task-list span,.task-list time { display: block; color: var(--text-muted); margin-top: 7px; font-size: 11px; }
.section-intro { display: flex; gap: 14px; }.section-intro h2 { margin-bottom: 6px; }.section-intro p { margin: 0; }.step-number { font-family: var(--font-mono); color: var(--primary); font-size: 13px; padding-top: 3px; }.draft-fields { border: 0; margin: 0; padding: 0; min-width: 0; }.draft-fields > .wb-label { margin-top: 10px; }.selected-inputs { list-style: none; padding: 0; margin: 12px 0 24px; }.selected-inputs li { display: flex; justify-content: space-between; align-items: center; gap: 12px; padding: 10px 0; border-bottom: 1px solid var(--border-subtle); }.selected-inputs span { overflow-wrap: anywhere; font-size: 13px; }.selected-inputs small { color: var(--text-muted); }.budget-heading { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; margin-bottom: 14px; }.budget-heading h3 { margin: 0; }.clarification-note { padding: 16px; border-left: 3px solid var(--primary); background: var(--bg-tertiary); font-size: 13px; line-height: 1.8; }
.task-goal { line-height: 1.8; white-space: pre-wrap; overflow-wrap: anywhere; }.task-reason,.proposal-explanation { line-height: 1.8; white-space: pre-wrap; overflow-wrap: anywhere; font-size: 13px; }.proposal-panel { border-top: 3px solid var(--primary); }.task-input-refs { padding-left: 18px; font-size: 12px; line-height: 1.9; overflow-wrap: anywhere; }.budget-details { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; font-size: 12px; }.budget-details dt { color: var(--text-muted); }.budget-details dd { margin: 4px 0; }.planning-actions p,.child-records p,.summary-warnings { font-size: 12px; line-height: 1.7; }.child-records { list-style: none; padding: 0; }.child-records > li { border-bottom: 1px solid var(--border-default); padding: 12px 0; }.snapshot-hash { font-family: var(--font-mono); font-size: 11px; overflow-wrap: anywhere; }summary { cursor: pointer; font-size: 12px; color: var(--text-secondary); }.wb-actions { margin-top: 14px; flex-wrap: wrap; }.wb-actions a { font-size: 12px; }.summary-warnings { border-left: 2px solid var(--border-default); padding-left: 20px; }
@media(max-width: 1000px) { .task-layout { grid-template-columns: 210px minmax(0, 1fr); gap: 16px; } }
@media(max-width: 760px) { .task-layout { grid-template-columns: 1fr; }.task-history { order: 2; }.task-workbench h1 { font-size: 25px; }.planning-boundary { align-items: flex-start; }.planning-boundary .wb-badge { flex-shrink: 0; }.wb-form { grid-template-columns: 1fr; }.wb-head { align-items: flex-start; }.task-goal { font-size: 15px; } }
</style>
