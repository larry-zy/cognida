import { computed, onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import { taskControlApi, type ExecutionReadiness, type GovernedTask, type Proposal, type SavedPlan, type TaskSummary } from '@/api/taskcontrol'

type RuntimeContext = {
  task: Ref<GovernedTask | undefined>
  summary: Ref<TaskSummary | undefined>
  savedPlan: Ref<SavedPlan | undefined>
  proposal: Ref<Proposal | undefined>
  scope: Ref<string>
  planningBusy: Ref<boolean>
  onRefreshed: () => void
  onInaccessible: (id: string, message: string) => void
}
const activeStates = new Set(['running', 'awaiting_approval', 'partially_completed', 'cancel_requested', 'reconcile_required'])
const statusCode = (cause: unknown) => (cause as { response?: { status?: number } })?.response?.status
function errorText(cause: unknown) {
  if (statusCode(cause) === 403) return '任务当前不可访问，请核对工作空间及权限。'
  if (statusCode(cause) === 404) return '任务不存在或当前账号无权查看。'
  if (statusCode(cause) === 409) return '任务版本或状态已变化，请重新核对最新状态。'
  return (cause as { response?: { data?: { message?: string } } })?.response?.data?.message || (cause instanceof Error ? cause.message : '状态查询失败，请重试。')
}
function data<T>(response: { data?: T }): T { if (response.data == null) throw new Error('服务未返回有效状态，请重试。'); return response.data }
export function useTaskRuntime(context: RuntimeContext) {
  const readiness = ref<ExecutionReadiness>(), loading = ref(false), action = ref(''), error = ref(''), readinessError = ref(''), actionError = ref(''), uncertain = ref(false), updatedAt = ref('')
  let generation = 0, disposed = false, timer: ReturnType<typeof setTimeout> | undefined
  const requests = new Set<AbortController>()
  const busy = computed(() => Boolean(action.value))
  const consistent = computed(() => context.task.value && context.summary.value?.task_id === context.task.value.id && context.summary.value?.task_version === context.task.value.version && context.summary.value?.current_plan_version === context.task.value.current_plan_version)
  const canStart = computed(() => {
    const task = context.task.value, ready = readiness.value
    return Boolean(task && ready && !loading.value && !busy.value && !context.planningBusy.value && !error.value && !readinessError.value && !uncertain.value && !context.proposal.value && consistent.value && ready.allowed === true && ready.execution_enabled === true && ready.task_id === task.id && ready.task_version === task.version && ready.plan_version === task.current_plan_version && ready.plan_version > 0 && context.savedPlan.value?.version === ready.plan_version && context.savedPlan.value.task_id === task.id)
  })
  const canCancel = computed(() => Boolean(consistent.value && context.summary.value?.can_cancel === true && context.summary.value?.terminal === false && context.task.value?.status !== 'cancel_requested' && !loading.value && !busy.value && !context.planningBusy.value && !error.value && !uncertain.value))
  const shouldPoll = () => Boolean(context.task.value && (uncertain.value || (context.summary.value?.terminal !== true && activeStates.has(context.task.value.status))))
  const needsReadiness = (task: GovernedTask) => task.current_plan_version > 0 && !task.execution_plan_version && !task.started_at && ['draft', 'ready'].includes(task.status)
  function stopTimer() { if (timer) clearTimeout(timer); timer = undefined }
  function schedule() { stopTimer(); if (!disposed && !document.hidden && shouldPoll()) timer = setTimeout(() => { void refresh() }, 4000) }
  function reset() { generation++; stopTimer(); requests.forEach(request => request.abort()); requests.clear(); readiness.value = undefined; loading.value = false; action.value = ''; error.value = ''; readinessError.value = ''; actionError.value = ''; uncertain.value = false; updatedAt.value = '' }
  function requestContext() {
    const id = context.task.value?.id, scope = context.scope.value, epoch = generation, abort = new AbortController()
    requests.add(abort)
    return { id, abort, live: () => !disposed && generation === epoch && context.scope.value === scope && context.task.value?.id === id }
  }
  function acceptReadiness(result: ExecutionReadiness, task: GovernedTask) {
    if (result.task_id !== task.id || result.task_version !== task.version || result.plan_version !== task.current_plan_version) throw new Error('执行准入对应的版本已变化，请重新核对最新状态。')
    readiness.value = result; readinessError.value = ''
  }
  // Initial details already contain a consistent task/summary. Do not repeat
  // those reads, and never clear the user's candidate while refreshing runtime.
  async function initialize() {
    if (!context.task.value || loading.value || context.planningBusy.value) return
    if (!needsReadiness(context.task.value)) { schedule(); return }
    const task = context.task.value, request = requestContext()
    loading.value = true; readiness.value = undefined; readinessError.value = ''
    try { const result = data(await taskControlApi.readiness(task.id, request.abort.signal)); if (request.live()) acceptReadiness(result, task) }
    catch (cause) { if (request.live()) readinessError.value = errorText(cause) }
    finally { requests.delete(request.abort); if (request.live()) { loading.value = false; schedule() } }
  }
  async function refresh() {
    if (!context.task.value || loading.value || busy.value || context.planningBusy.value || disposed) { schedule(); return }
    const request = requestContext(), id = request.id!
    stopTimer(); loading.value = true; readiness.value = undefined; error.value = ''; readinessError.value = ''
    try {
      const task = data(await taskControlApi.get(id, request.abort.signal))
      if (!request.live()) return
      if (task.id !== id) throw new Error('任务身份不匹配，请重新打开任务。')
      if (task.version < context.task.value!.version) throw new Error('收到过期的任务版本；显示的仍是上次状态，请重试。')
      const status = data(await taskControlApi.summary(id, request.abort.signal))
      if (!request.live()) return
      if (status.task_id !== id || status.task_version !== task.version || status.current_plan_version !== task.current_plan_version) throw new Error('任务在查询期间发生变化；显示的仍是上次状态，请重试。')
      let plan = context.savedPlan.value
      if (task.current_plan_version && (plan?.version !== task.current_plan_version || plan.task_id !== id)) {
        plan = data(await taskControlApi.plan(id, task.current_plan_version, request.abort.signal))
        if (!request.live()) return
        if (plan.task_id !== id || plan.version !== task.current_plan_version) throw new Error('已保存计划版本不匹配，请重试。')
      }
      context.task.value = task; context.summary.value = status; context.savedPlan.value = task.current_plan_version ? plan : undefined
      updatedAt.value = new Date().toLocaleTimeString('zh-CN'); uncertain.value = false; actionError.value = ''; context.onRefreshed()
      if (needsReadiness(task) && status.terminal !== true) {
        try { const ready = data(await taskControlApi.readiness(id, request.abort.signal)); if (request.live()) acceptReadiness(ready, task) }
        catch (cause) { if (request.live()) readinessError.value = errorText(cause) }
      }
    } catch (cause) {
      if (!request.live()) return
      if ([401, 403, 404].includes(statusCode(cause) || 0)) context.onInaccessible(id, errorText(cause))
      else error.value = errorText(cause)
    } finally { requests.delete(request.abort); if (request.live()) { loading.value = false; schedule() } }
  }
  async function mutate(kind: 'start' | 'cancel') {
    if ((kind === 'start' && !canStart.value) || (kind === 'cancel' && !canCancel.value)) return
    const task = context.task.value!, request = requestContext(), planVersion = readiness.value?.plan_version
    stopTimer(); action.value = kind; actionError.value = ''; readiness.value = undefined
    try {
      const result = data(await (kind === 'start' ? taskControlApi.start(task.id, task.version, planVersion!, request.abort.signal) : taskControlApi.cancel(task.id, task.version, request.abort.signal)))
      if (!request.live()) return
      if (result.id !== task.id || result.version < task.version) throw new Error('操作返回的任务状态不匹配，请核对最新状态。')
      context.task.value = result; context.summary.value = undefined
    } catch (cause) {
      if (!request.live()) return
      if ([401, 403, 404].includes(statusCode(cause) || 0)) { context.onInaccessible(task.id, errorText(cause)); return }
      actionError.value = errorText(cause); uncertain.value = true
    } finally {
      requests.delete(request.abort)
      if (request.live()) { action.value = ''; if (!uncertain.value) await refresh(); else schedule() }
    }
  }
  function visibilityChanged() { if (document.hidden) stopTimer(); else if (context.task.value) void refresh() }
  watch(context.scope, reset)
  watch(context.planningBusy, busy => { if (busy) stopTimer(); else schedule() })
  onMounted(() => document.addEventListener('visibilitychange', visibilityChanged))
  onBeforeUnmount(() => { reset(); disposed = true; document.removeEventListener('visibilitychange', visibilityChanged) })
  return { readiness, loading, busy, action, error, readinessError, actionError, uncertain, updatedAt, canStart, canCancel, initialize, refresh, reset, start: () => mutate('start'), cancel: () => mutate('cancel') }
}
