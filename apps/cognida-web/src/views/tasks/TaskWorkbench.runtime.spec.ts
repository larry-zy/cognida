import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent, reactive } from 'vue'
import { createMemoryHistory, createRouter, RouterView, useRoute } from 'vue-router'
import type { GovernedTask, PlanSpec, Proposal, TaskSummary } from '@/api/taskcontrol'

const api = vi.hoisted(() => ({ capabilities: vi.fn(), list: vi.fn(), get: vi.fn(), assess: vi.fn(), create: vi.fn(), propose: vi.fn(), appendPlan: vi.fn(), plan: vi.fn(), summary: vi.fn(), readiness: vi.fn(), start: vi.fn(), cancel: vi.fn(), events: vi.fn() }))
vi.mock('@/api/taskcontrol', () => ({ taskControlApi: api }))
vi.mock('@/api/lifecycle', () => ({ lifecycleApi: { assets: vi.fn().mockResolvedValue({ data: { items: [], next_cursor: '' } }) } }))
const auth = reactive({ authEpoch: 1, user: { id: 1 }, currentTenant: { id: 1 } })
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
import TaskWorkbench from './TaskWorkbench.vue'

const input = { asset_id: 'asset-1', version: 1 }
const budget = { max_nodes: 8, max_concurrency: 2, max_attempts: 16, max_calls: 64, max_cost_micros: 1000000, timeout_seconds: 600 }
const task = (status = 'ready', version = 2): GovernedTask => ({ id: 'task-1', goal: '检查并清洗数据', input_refs: [input], budget, status, version, current_plan_version: 1, execution_plan_version: status === 'ready' ? 0 : 1, result_refs: [], created_at: '', updated_at: '' })
const plan: PlanSpec = { schema_version: 1, budget, nodes: [{ id: 'clean', capability: 'lifecycle.data-cleaning', capability_version: 1, depends_on: [], inputs: [{ asset: input, kind: 'raw_asset' }], output_kinds: ['cleaned_asset'], tools: ['clean'], permissions: ['read'], max_attempts: 1, max_calls_per_attempt: 1, max_cost_micros_per_attempt: 0, timeout_seconds: 30 }] }
const candidate: Proposal = { task_id: 'task-1', task_version: 2, decision: { kind: 'proposal', explanation: '保留中的候选', plan }, validation: { valid: true, execution_enabled: false, authorization_scope: 'lifecycle_asset', origin_revalidated: false, topological_order: ['clean'], approval_required: [], input_refs: [input], reserved_attempts: 1, reserved_calls: 1, reserved_cost_micros: 0, reserved_seconds: 30 }, execution_enabled: false, planning_calls: 1, saved: false }
let state: GovernedTask
let stateSummary: TaskSummary
function summary(): TaskSummary { return { task_id: state.id, task_version: state.version, status: state.status, current_plan_version: 1, planned_nodes: 1, children: [], result_refs: [], warnings: [], execution_enabled: true, terminal: ['completed', 'cancelled', 'failed'].includes(state.status), can_cancel: state.status === 'running', nodes: [{ node_id: 'clean', capability: 'lifecycle.data-cleaning', status: state.status === 'ready' ? 'pending' : state.status === 'completed' ? 'succeeded' : state.status, attempt: state.status === 'ready' ? 0 : 1, output_refs: [], domain_kind: 'invocation', domain_id: 'real-invocation' }], usage: { reserved_attempts: 1, reserved_calls: 2, reserved_cost_micros: 0 } } }
const ready = () => ({ task_id: state.id, task_version: state.version, plan_version: state.current_plan_version, execution_enabled: true, allowed: state.status === 'ready', blockers: [] })
const deferred = <T,>() => { let resolve!: (data: T) => void; const promise = new Promise<T>(done => { resolve = done }); return { promise, resolve } }
const wrappers: VueWrapper[] = []
const button = (wrapper: VueWrapper, text: string) => wrapper.findAll('button').find(item => item.text() === text)
async function setup() {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/tasks', component: TaskWorkbench }, { path: '/:pathMatch(.*)*', component: { template: '<p>Domain detail</p>' } }] })
  await router.push('/tasks?task_id=task-1'); await router.isReady()
  const host = defineComponent({ components: { RouterView }, setup: () => ({ route: useRoute() }), template: '<RouterView :key="route.fullPath" />' })
  const wrapper = mount(host, { global: { plugins: [router] } }); wrappers.push(wrapper); await flushPromises(); return { wrapper, router }
}
async function tick() { await vi.advanceTimersByTimeAsync(4000); await flushPromises() }
describe('governed task execution controls', () => {
  beforeEach(() => {
    vi.clearAllMocks(); api.events.mockImplementation(async function* () {}); vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false); vi.spyOn(window, 'confirm').mockReturnValue(true)
    auth.authEpoch++; auth.currentTenant.id = 1; state = task(); stateSummary = summary()
    api.capabilities.mockResolvedValue({ data: { capabilities: [{ id: 'lifecycle.data-cleaning', version: 1, execution: { profile: 'bounded-clean', base_calls: 1, calls_per_input: 1, max_timeout_seconds: 30 } }], execution_enabled: true } })
    api.list.mockResolvedValue({ data: { items: [], next_cursor: '' } })
    api.get.mockImplementation(() => Promise.resolve({ data: { ...state } }))
    api.summary.mockImplementation(() => Promise.resolve({ data: structuredClone(stateSummary) }))
    api.plan.mockResolvedValue({ data: { task_id: 'task-1', version: 1, snapshot: plan, content_hash: 'saved-hash', created_at: '' } })
    api.readiness.mockImplementation(() => Promise.resolve({ data: ready() }))
    api.propose.mockResolvedValue({ data: candidate })
    api.start.mockImplementation(() => { state = task('running', 3); stateSummary = summary(); return Promise.resolve({ data: { ...state } }) })
    api.cancel.mockImplementation(() => { state = task('cancel_requested', 4); stateSummary = summary(); return Promise.resolve({ data: { ...state } }) })
  })
  afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()); vi.useRealTimers(); vi.restoreAllMocks() })
  it('requires explicit current readiness rather than saved plans or a globally enabled runtime', async () => {
    api.readiness.mockResolvedValue({ data: { ...ready(), allowed: false, blockers: [{ node_id: 'clean', code: 'unsupported_capability', message: '此能力尚未接通执行。' }] } })
    const { wrapper } = await setup()
    expect(wrapper.text()).toContain('已保存计划 v1'); expect(wrapper.text()).toContain('此能力尚未接通执行。')
    expect(button(wrapper, '启动任务')).toBeUndefined(); expect(api.start).not.toHaveBeenCalled()
    api.readiness.mockResolvedValue({ data: { ...ready(), task_version: 99 } })
    await button(wrapper, '重新核对执行状态')!.trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('执行准入对应的版本已变化'); expect(button(wrapper, '启动任务')).toBeUndefined()
  })
  it('starts once with the saved plan version, polls real node results, links authoritative handles, and stops at terminal state', async () => {
    const pending = deferred<{ data: GovernedTask }>(); api.start.mockImplementationOnce(() => pending.promise)
    const { wrapper, router } = await setup(); const startButton = button(wrapper, '启动任务')!
    await startButton.trigger('click'); await startButton.trigger('click')
    expect(api.start).toHaveBeenCalledTimes(1); expect(api.start).toHaveBeenCalledWith('task-1', 2, 1, expect.any(AbortSignal))
    state = task('running', 3); stateSummary = summary(); pending.resolve({ data: { ...state } }); await flushPromises()
    expect(wrapper.text()).toContain('已成功 0 / 1 个节点'); expect(wrapper.text()).not.toContain('已成功 1 / 1')
    state = task('completed', 4); stateSummary = summary(); stateSummary.nodes![0]!.output_refs = [{ asset_id: 'result-asset', version: 3, kind: 'cleaned_asset' }]
    await tick()
    expect(wrapper.text()).toContain('已成功 1 / 1 个节点'); expect(wrapper.text()).toContain('已完成')
    expect(wrapper.get('a[href="/analysis?asset=result-asset:3"]').text()).toContain('查看清洗版本')
    const calls = api.get.mock.calls.length; await tick(); expect(api.get).toHaveBeenCalledTimes(calls)
    expect(api.readiness).toHaveBeenCalledTimes(1) // Running/terminal reads never repeat start preflight.
    await wrapper.get('a[href="/experts?invocation_id=real-invocation"]').trigger('click'); await flushPromises()
    expect(router.currentRoute.value.query.invocation_id).toBe('real-invocation'); expect(api.propose).not.toHaveBeenCalled()
  })
  it('keeps cancellation pending until the backend reports a real terminal status', async () => {
    state = task('running', 3); stateSummary = summary()
    const { wrapper } = await setup()
    await button(wrapper, '申请取消')!.trigger('click'); await flushPromises()
    expect(api.cancel).toHaveBeenCalledWith('task-1', 3, expect.any(AbortSignal))
    expect(wrapper.text()).toContain('已申请取消，正在等待领域任务确认'); expect(wrapper.text()).not.toContain('已取消')
    expect(button(wrapper, '申请取消')).toBeUndefined()
    state = task('cancelled', 5); stateSummary = summary(); await tick()
    expect(wrapper.text()).toContain('已取消'); expect(wrapper.text()).not.toContain('正在等待领域任务确认')
    const calls = api.get.mock.calls.length; await tick(); expect(api.get).toHaveBeenCalledTimes(calls)
  })
  it('preserves an unsaved candidate during refresh and blocks it after an externally advanced task version', async () => {
    const { wrapper } = await setup()
    await button(wrapper, '生成候选计划')!.trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('保留中的候选'); expect(button(wrapper, '启动任务')).toBeUndefined()
    state = task('ready', 3); stateSummary = summary()
    await button(wrapper, '刷新任务状态')!.trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('保留中的候选'); expect(wrapper.text()).toContain('此候选已过期')
    expect(button(wrapper, '保存计划')!.attributes('disabled')).toBeDefined(); expect(api.appendPlan).not.toHaveBeenCalled()
  })
  it('reconciles uncertain writes through reads without automatically replaying start', async () => {
    api.start.mockRejectedValueOnce(new Error('connection lost'))
    const { wrapper } = await setup(); await button(wrapper, '启动任务')!.trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('操作结果尚未确认'); expect(button(wrapper, '启动任务')).toBeUndefined()
    state = task('running', 3); stateSummary = summary(); await tick()
    expect(wrapper.text()).not.toContain('操作结果尚未确认'); expect(wrapper.text()).toContain('当前执行计划 v1')
    expect(api.start).toHaveBeenCalledTimes(1); expect(api.cancel).not.toHaveBeenCalled()
  })
  it('pauses automatic reads while hidden and resumes immediately when the document returns', async () => {
    state = task('running', 3); stateSummary = summary()
    const { wrapper } = await setup(); const before = api.get.mock.calls.length
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(true); document.dispatchEvent(new Event('visibilitychange'))
    await tick(); expect(api.get).toHaveBeenCalledTimes(before)
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false); document.dispatchEvent(new Event('visibilitychange')); await flushPromises()
    expect(api.get).toHaveBeenCalledTimes(before + 1); expect(wrapper.text()).toContain('最近核对')
  })
  it('discards an old tenant runtime response and aborts its request on identity change', async () => {
    state = task('running', 3); stateSummary = summary()
    const { wrapper } = await setup(); const old = deferred<{ data: GovernedTask }>()
    api.get.mockImplementationOnce(() => old.promise).mockRejectedValue({ response: { status: 403 } })
    await tick(); const signal = api.get.mock.calls.at(-1)![1] as AbortSignal
    auth.authEpoch++; auth.currentTenant.id = 2; await flushPromises(); expect(signal.aborted).toBe(true)
    old.resolve({ data: { ...task('running', 4), goal: 'OLD_PRIVATE_GOAL' } }); await flushPromises()
    expect(wrapper.text()).not.toContain('OLD_PRIVATE_GOAL'); expect(wrapper.text()).toContain('不可访问')
    expect(button(wrapper, '申请取消')).toBeUndefined()
  })
  it('hides private task state and stops recovery polling after access is revoked', async () => {
    state = task('running', 3); stateSummary = summary()
    const { wrapper } = await setup(); api.summary.mockRejectedValue({ response: { status: 403 } })
    await tick(); expect(wrapper.text()).toContain('不可访问'); expect(wrapper.text()).not.toContain('检查并清洗数据')
    const calls = api.get.mock.calls.length; await tick(); expect(api.get).toHaveBeenCalledTimes(calls)
  })
  it.each(['summary changes concurrently', 'task revision goes backwards'])('keeps the last coherent snapshot when %s', async scenario => {
    state = task('running', 3); stateSummary = summary()
    const { wrapper } = await setup()
    if (scenario === 'task revision goes backwards') { state = task('ready', 2); stateSummary = summary() }
    else state = task('completed', 4)
    await tick()
    expect(wrapper.text()).toContain('显示的仍是上次状态'); expect(wrapper.text()).toContain('任务版本 3'); expect(wrapper.get('.execution-panel > .execution-heading .wb-badge').text()).toBe('进行中')
    state = task('completed', 4); stateSummary = summary(); await tick(); expect(wrapper.text()).toContain('任务版本 4'); expect(wrapper.text()).toContain('已完成')
  })
  it('drops an in-flight runtime read when switching task deep links with the real router', async () => {
    state = task('running', 3); stateSummary = summary()
    const { wrapper, router } = await setup(); const old = deferred<{ data: GovernedTask }>()
    api.get.mockImplementationOnce(() => old.promise)
    await tick(); const signal = api.get.mock.calls.at(-1)![1] as AbortSignal
    const nextTask = { ...task('draft', 1), id: 'task-2', goal: '另一任务', current_plan_version: 0, execution_plan_version: 0 }
    api.get.mockResolvedValue({ data: nextTask }); api.summary.mockResolvedValue({ data: { ...summary(), task_id: 'task-2', task_version: 1, status: 'draft', current_plan_version: 0, nodes: [], children: [] } })
    await router.push('/tasks?task_id=task-2'); await flushPromises()
    expect(signal.aborted).toBe(true)
    old.resolve({ data: { ...task('completed', 4), goal: 'OLD_PRIVATE_GOAL' } }); await flushPromises()
    expect(wrapper.get('.task-goal').text()).toBe('另一任务'); expect(wrapper.text()).not.toContain('OLD_PRIVATE_GOAL')
    expect(api.start).not.toHaveBeenCalled(); expect(api.cancel).not.toHaveBeenCalled()
  })
  it('uses supported domain kinds for deep links and renders unknown errors only as text', async () => {
    state = task('running', 3); stateSummary = summary()
    stateSummary.nodes = [{ node_id: 'clean', capability: 'lifecycle.data-cleaning', status: 'failed', attempt: 1, error: '<img src=x onerror=alert(1)>', output_refs: [], domain_kind: 'run', domain_id: 'run/one' }, { node_id: 'unknown', capability: 'external.task', status: 'unknown', attempt: 0, output_refs: [], domain_kind: 'https://untrusted.invalid', domain_id: 'external-handle' }]
    const { wrapper, router } = await setup()
    expect(wrapper.find('.runtime-nodes img').exists()).toBe(false); expect(wrapper.text()).toContain('<img src=x onerror=alert(1)>')
    expect(wrapper.findAll('.runtime-nodes a')).toHaveLength(1)
    await wrapper.get('.runtime-nodes a').trigger('click'); await flushPromises()
    expect(router.currentRoute.value.path).toBe('/pipelines'); expect(router.currentRoute.value.query.run_id).toBe('run/one')
  })
  it('uses read-only stream changes and falls back to polling without replaying commands', async () => {
    state = task('running', 3); stateSummary = summary(); let notify!: () => void
    api.events.mockImplementationOnce(async function* () { await new Promise<void>(resolve => { notify = resolve }); yield structuredClone(stateSummary); throw new Error('stream disconnected') })
    const { wrapper } = await setup(); expect(api.events).toHaveBeenCalledWith('task-1', expect.any(AbortSignal)); state = task('completed', 4); stateSummary = summary(); notify(); await flushPromises()
    expect(wrapper.text()).toContain('已成功 1 / 1 个节点'); expect(api.start).not.toHaveBeenCalled(); expect(api.cancel).not.toHaveBeenCalled(); expect(api.get.mock.calls.length).toBeGreaterThan(1)
  })

})
