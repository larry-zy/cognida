import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { reactive } from 'vue'
import type { GovernedTask, PlanSpec, Proposal, TaskSummary } from '@/api/taskcontrol'

const api = vi.hoisted(() => ({ validate: vi.fn(), capabilities: vi.fn(), list: vi.fn(), get: vi.fn(), assess: vi.fn(), create: vi.fn(), propose: vi.fn(), appendPlan: vi.fn(), plan: vi.fn(), summary: vi.fn(), readiness: vi.fn(), start: vi.fn(), cancel: vi.fn() }))
const lifecycle = vi.hoisted(() => ({ assets: vi.fn() }))
const navigation = vi.hoisted(() => ({ replace: vi.fn(), push: vi.fn(), leave: undefined as undefined | (() => boolean), update: undefined as undefined | (() => boolean) }))
vi.mock('@/api/taskcontrol', () => ({ taskControlApi: api }))
vi.mock('@/api/lifecycle', () => ({ lifecycleApi: lifecycle }))
const auth = reactive({ authEpoch: 1, user: { id: 1 }, currentTenant: { id: 1 } })
const route = reactive({ query: {} as Record<string, unknown> })
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('vue-router', () => ({ useRoute: () => route, useRouter: () => navigation, onBeforeRouteLeave: (fn: () => boolean) => { navigation.leave = fn }, onBeforeRouteUpdate: (fn: () => boolean) => { navigation.update = fn } }))
import TaskWorkbench from './TaskWorkbench.vue'
import DomainTaskForm from './DomainTaskForm.vue'

const input = { asset_id: 'asset-1', version: 2 }
const budget = { max_nodes: 8, max_concurrency: 2, max_attempts: 16, max_calls: 64, max_cost_micros: 1000000, timeout_seconds: 600 }
const task = (id = 'task-1', version = 1, currentPlan = 0): GovernedTask => ({ id, goal: '清洗收入数据并生成报告', input_refs: [input], budget, status: 'draft', version, current_plan_version: currentPlan, result_refs: [], created_at: '', updated_at: '' })
const plan: PlanSpec = { schema_version: 1, budget, nodes: [
  { id: 'publish', capability: 'lifecycle.data-asset', capability_version: 1, depends_on: ['clean'], inputs: [{ from_node: 'clean', kind: 'cleaned_asset' }], output_kinds: ['published_asset'], tools: ['asset_publish'], permissions: ['read', 'write'], max_attempts: 1, max_calls_per_attempt: 1, max_cost_micros_per_attempt: 0, timeout_seconds: 30 },
  { id: 'clean', capability: 'lifecycle.data-cleaning', capability_version: 1, depends_on: [], inputs: [{ asset: input, kind: 'raw_asset' }], output_kinds: ['cleaned_asset'], tools: ['clean'], permissions: ['read'], max_attempts: 2, max_calls_per_attempt: 2, max_cost_micros_per_attempt: 0, timeout_seconds: 30 },
] }
const validation = { valid: true, execution_enabled: false as const, authorization_scope: 'lifecycle_asset', origin_revalidated: false, topological_order: ['clean', 'publish'], approval_required: ['publish'], input_refs: [input], reserved_attempts: 3, reserved_calls: 5, reserved_cost_micros: 0, reserved_seconds: 90 }
const proposal = (): Proposal => ({ task_id: 'task-1', task_version: 1, decision: { kind: 'proposal', explanation: '检查后清洗，产出需审批。', plan }, validation, execution_enabled: false, planning_calls: 1, saved: false })
const summary = (id = 'task-1', version = 1, planVersion = 0): TaskSummary => ({ task_id: id, task_version: version, current_plan_version: planVersion, status: 'draft', planned_nodes: planVersion ? 2 : 0, children: [], result_refs: [], warnings: [], execution_enabled: false })
const saved = () => ({ task_id: 'task-1', version: 1, snapshot: plan, content_hash: 'real-saved-hash', created_at: '' })
const asset = (id = 'asset-1', name = '收入快照') => ({ asset_id: id, version: 2, name, kind: 'raw_asset', status: 'available' })
const page = (items: unknown[], next_cursor = '') => ({ data: { items, next_cursor, has_more: Boolean(next_cursor) } })
const deferred = <T,>() => { let resolve!: (data: T) => void; const promise = new Promise<T>(r => { resolve = r }); return { promise, resolve } }
const button = (wrapper: VueWrapper, label: string) => wrapper.findAll('button').find(item => item.text() === label)!
async function setup() { const wrapper = mount(TaskWorkbench, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } }); await flushPromises(); return wrapper }
async function fill(wrapper: VueWrapper) { await wrapper.get('[aria-label="任务目标"]').setValue('清洗收入数据并生成报告'); await wrapper.get('[aria-label="可选资产"]').setValue('asset-1:2'); await button(wrapper, '添加输入').trigger('click') }
async function begin(wrapper: VueWrapper) { await fill(wrapper); await wrapper.get('form[aria-label="新建规划任务"]').trigger('submit'); await flushPromises() }

describe('Supervisor task planning', () => {
  beforeEach(() => {
    vi.clearAllMocks(); route.query = {}; auth.authEpoch++; auth.user.id = 1; auth.currentTenant.id = 1
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    api.readiness.mockImplementation((id: string) => Promise.resolve({ data: { task_id: id, task_version: 2, plan_version: 1, execution_enabled: false, allowed: false, blockers: [] } }))
    api.capabilities.mockResolvedValue({ data: { capabilities: [], execution_enabled: false } })
    api.list.mockResolvedValue(page([])); lifecycle.assets.mockResolvedValue(page([asset()]))
    api.assess.mockResolvedValue({ data: { ready: true, questions: [], execution_enabled: false } })
    api.create.mockResolvedValue({ data: task() }); api.get.mockResolvedValue({ data: task() })
    api.propose.mockResolvedValue({ data: proposal() }); api.summary.mockResolvedValue({ data: summary() })
    api.appendPlan.mockResolvedValue({ data: { plan: saved(), validation } }); api.plan.mockResolvedValue({ data: saved() })
  })
  it('validates explicit specialist parameters before draft creation and never starts on preview', async () => {
    api.validate.mockResolvedValue({ data: validation })
    const wrapper = await setup()
    wrapper.findComponent(DomainTaskForm).vm.$emit('preview', { goal: 'explicit expert', input_refs: [input], budget }, plan)
    await flushPromises()
    expect(api.validate).toHaveBeenCalledWith(plan, expect.any(AbortSignal))
    expect(api.create).toHaveBeenCalledWith({ goal: 'explicit expert', input_refs: [input], budget }, expect.any(AbortSignal))
    expect(api.propose).not.toHaveBeenCalled(); expect(api.appendPlan).not.toHaveBeenCalled(); expect(api.start).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('参数来自本页明确填写的领域预置')
    wrapper.unmount()
  })
  it('does not create a specialist draft if its validation arrives after identity changes', async () => {
    const pending = deferred<{ data: typeof validation }>(); api.validate.mockReturnValueOnce(pending.promise)
    const wrapper = await setup()
    wrapper.findComponent(DomainTaskForm).vm.$emit('preview', { goal: 'old tenant expert', input_refs: [input], budget }, plan)
    await flushPromises(); auth.authEpoch++; auth.currentTenant.id = 2; await flushPromises()
    pending.resolve({ data: validation }); await flushPromises()
    expect(api.create).not.toHaveBeenCalled(); expect(wrapper.text()).not.toContain('old tenant expert'); wrapper.unmount()
  })
  it('asks for missing inputs without creating a task or spending a planning call', async () => {
    api.assess.mockResolvedValue({ data: { ready: false, questions: [{ id: 'goal', question: '最终需要什么产物？' }, { id: 'inputs', question: '请选择输入资产。' }], execution_enabled: false } })
    const wrapper = await setup()
    await wrapper.get('form[aria-label="新建规划任务"]').trigger('submit'); await flushPromises()
    expect(wrapper.text()).toContain('最终需要什么产物？'); expect(wrapper.text()).toContain('请在上方修改')
    expect(api.create).not.toHaveBeenCalled(); expect(api.propose).not.toHaveBeenCalled()
    expect(wrapper.findAll('button').some(item => /^(开始执行|批准|运行任务)$/.test(item.text()))).toBe(false)
    wrapper.unmount()
  })
  it('previews a validated candidate and persists only after explicit save using the original task version', async () => {
    const wrapper = await setup(); await begin(wrapper)
    expect(api.assess).toHaveBeenCalledWith({ goal: task().goal, input_refs: [input], budget }, expect.any(AbortSignal))
    expect(api.propose).toHaveBeenCalledWith('task-1', 1, [], expect.any(AbortSignal))
    expect(api.appendPlan).not.toHaveBeenCalled(); expect(wrapper.text()).toContain('尚未保存')
    expect(wrapper.findAll('.plan-nodes h3').map(item => item.text())).toEqual(['数据清洗', '资产发布'])
    expect(wrapper.text()).toContain('需要人工审批'); expect(wrapper.text()).toContain('asset_publish')
    // Same task URL represents saving another version without a route remount.
    route.query = { task_id: 'task-1' }; await flushPromises()
    api.get.mockResolvedValue({ data: task('task-1', 2, 1) }); api.summary.mockResolvedValue({ data: summary('task-1', 2, 1) })
    await button(wrapper, '保存计划').trigger('click'); await flushPromises()
    expect(api.appendPlan).toHaveBeenCalledWith('task-1', 1, plan, expect.any(AbortSignal))
    expect(api.get).toHaveBeenCalledWith('task-1', expect.any(AbortSignal)); expect(api.plan).toHaveBeenCalledWith('task-1', 1, expect.any(AbortSignal))
    expect(wrapper.text()).toContain('已保存计划 v1'); expect(wrapper.text()).not.toContain('尚未保存')
    expect(wrapper.text()).toContain('任务版本 2'); expect(wrapper.text()).toContain('暂无已确认的结果引用')
    expect(navigation.replace).not.toHaveBeenCalled()
    expect(navigation.leave?.()).toBe(true); wrapper.unmount()
  })
  it('shows deterministic budget adjustments and saves the exact compiled candidate only on explicit save', async () => {
    const compiled: PlanSpec = {
      ...plan, budget: { ...budget, max_attempts: 3, max_calls: 5, max_cost_micros: 300, timeout_seconds: 90 },
      nodes: plan.nodes.map(node => ({ ...node, max_cost_micros_per_attempt: 100 })),
    }
    api.propose.mockResolvedValue({ data: {
      ...proposal(), decision: { ...proposal().decision, plan: compiled }, validation: { ...validation, reserved_cost_micros: 300 },
      budget_adjustments: [
        { field: 'max_attempts', before: 2, after: 3, reason: 'node_retry_reservations' },
        { field: 'max_calls', before: 3, after: 5, reason: 'node_retry_reservations' },
        { field: 'max_cost_micros', before: 0, after: 300, reason: 'node_retry_reservations' },
        { field: 'timeout_seconds', before: 60, after: 90, reason: 'node_retry_reservations' },
      ],
    } })
    const wrapper = await setup(); await begin(wrapper)
    const note = wrapper.get('[aria-label="候选预算补齐说明"]')
    expect(note.text()).toContain('已补齐节点与重试预留')
    expect(note.text()).toContain('总尝试次数：2 → 3 次'); expect(note.text()).toContain('总调用次数：3 → 5 次')
    expect(note.text()).toContain('成本额度：0 → 300 微成本单位'); expect(note.text()).toContain('总时限：60 → 90 秒')
    expect(note.text()).toContain('尚未保存或启动任务')
    expect(wrapper.findComponent({ name: 'TaskPlanPreview' }).props('plan')).toEqual(compiled)
    expect(api.create.mock.calls[0][0].budget).toEqual(budget)
    expect(api.appendPlan).not.toHaveBeenCalled(); expect(api.start).not.toHaveBeenCalled()
    route.query = { task_id: 'task-1' }; await flushPromises()
    api.get.mockResolvedValue({ data: task('task-1', 2, 1) }); api.summary.mockResolvedValue({ data: summary('task-1', 2, 1) })
    api.plan.mockResolvedValue({ data: { ...saved(), snapshot: compiled } })
    await button(wrapper, '保存计划').trigger('click'); await flushPromises()
    expect(api.appendPlan).toHaveBeenCalledWith('task-1', 1, compiled, expect.any(AbortSignal))
    expect(api.start).not.toHaveBeenCalled()
    expect(wrapper.find('[aria-label="候选预算补齐说明"]').exists()).toBe(false)
    expect(wrapper.findComponent({ name: 'TaskPlanPreview' }).props('plan')).toEqual(compiled)
    wrapper.unmount()
  })
  it('keeps question text and prior answers across clarification rounds with repeated question IDs', async () => {
    const first = { ...proposal(), decision: { kind: 'clarify', explanation: '<img src=x onerror=alert(1)>', questions: [{ id: 'q1', question: '要保留原始行吗？', options: ['是', '否'] }] } }
    const second = { ...proposal(), decision: { kind: 'clarify', explanation: '继续确认', questions: [{ id: 'q1', question: '报告要包含什么？' }] } }
    api.propose.mockResolvedValueOnce({ data: first }).mockResolvedValueOnce({ data: second }).mockResolvedValue({ data: proposal() })
    const wrapper = await setup(); await begin(wrapper)
    expect(wrapper.find('img').exists()).toBe(false); expect(wrapper.text()).toContain('<img src=x onerror=alert(1)>')
    await wrapper.get('[aria-label="要保留原始行吗？"]').setValue('是')
    await wrapper.get('form[aria-label="补充规划信息"]').trigger('submit'); await flushPromises()
    expect(api.propose.mock.calls[1][2]).toEqual([{ question_id: 'q1', question: '要保留原始行吗？', answer: '是' }])
    await wrapper.get('[aria-label="报告要包含什么？"]').setValue('质量变化')
    await wrapper.get('form[aria-label="补充规划信息"]').trigger('submit'); await flushPromises()
    expect(api.propose.mock.calls[2][2]).toEqual([{ question_id: 'q1', question: '要保留原始行吗？', answer: '是' }, { question_id: expect.stringMatching(/^r\d+\.q1\./), question: '报告要包含什么？', answer: '质量变化' }])
    expect(api.create).toHaveBeenCalledTimes(1); expect(api.appendPlan).not.toHaveBeenCalled(); wrapper.unmount()
  })
  it('restores the saved plan by a read-only deep link and does not equate a bound child with completed execution', async () => {
    route.query = { task_id: 'task-1' }; api.get.mockResolvedValue({ data: task('task-1', 2, 1) })
    api.summary.mockResolvedValue({ data: { ...summary('task-1', 2, 1), children: [{ id: 'child-1', node_id: 'clean', plan_version: 1, attempt: 1, domain: 'lifecycle', domain_id: 'run-1', dispatch_status: 'bound', domain_status: 'unknown', receipt_present: true }], warnings: ['领域状态尚未核实'] } })
    const wrapper = await setup()
    expect(wrapper.text()).toContain('real-saved-hash'); expect(wrapper.text()).toContain('领域状态：尚未核实')
    expect(wrapper.findAll('.plan-nodes h3').map(item => item.text())).toEqual(['数据清洗', '资产发布'])
    expect(wrapper.text()).toContain('已有提交回执，仍需核实领域结果')
    expect(api.assess).not.toHaveBeenCalled(); expect(api.create).not.toHaveBeenCalled(); expect(api.propose).not.toHaveBeenCalled()
    expect(button(wrapper, '生成候选计划')).toBeUndefined(); wrapper.unmount()
  })
  it('drops a stale linked task response after switching to a denied task', async () => {
    const old = deferred<{ data: GovernedTask }>()
    route.query = { task_id: 'old-task' }; api.get.mockImplementationOnce(() => old.promise).mockRejectedValue({ response: { status: 403 } })
    const wrapper = await setup(); route.query = { task_id: 'denied-task' }; await flushPromises()
    old.resolve({ data: { ...task('old-task'), goal: 'OLD_PRIVATE_GOAL' } }); await flushPromises()
    expect(wrapper.text()).toContain('当前不可访问'); expect(wrapper.text()).not.toContain('OLD_PRIVATE_GOAL')
    expect(api.summary).not.toHaveBeenCalled(); expect(api.propose).not.toHaveBeenCalled(); wrapper.unmount()
  })
  it.each(['summary', 'plan'] as const)('clears initial private details if %s becomes inaccessible after the task read succeeds', async endpoint => {
    route.query = { task_id: 'task-1' }; api.get.mockResolvedValue({ data: { ...task('task-1', 2, 1), goal: 'PRIVATE_REVOKED_GOAL' } })
    api.summary.mockResolvedValue({ data: summary('task-1', 2, 1) }); api[endpoint].mockRejectedValue({ response: { status: 403 } })
    const wrapper = await setup()
    expect(wrapper.text()).toContain('当前不可访问'); expect(wrapper.text()).not.toContain('PRIVATE_REVOKED_GOAL')
    expect(wrapper.text()).not.toContain('已保存计划 v1'); expect(wrapper.find('.task-goal').exists()).toBe(false)
    expect(api.readiness).not.toHaveBeenCalled(); wrapper.unmount()
  })
  it('does not continue drafting as a different tenant when create completes late', async () => {
    const old = deferred<{ data: GovernedTask }>(); api.create.mockImplementationOnce(() => old.promise)
    const wrapper = await setup(); await begin(wrapper)
    const signal = api.create.mock.calls[0][1] as AbortSignal
    auth.authEpoch++; auth.currentTenant.id = 2; await flushPromises()
    old.resolve({ data: { ...task(), goal: 'OLD_PRIVATE_GOAL' } }); await flushPromises()
    expect(signal.aborted).toBe(true); expect(api.propose).not.toHaveBeenCalled()
    expect(wrapper.text()).not.toContain('OLD_PRIVATE_GOAL'); expect((wrapper.get('[aria-label="任务目标"]').element as HTMLTextAreaElement).value).toBe('')
    wrapper.unmount()
  })
  it('clears a pending previous-tenant candidate and ignores its eventual response', async () => {
    const old = deferred<{ data: Proposal }>(); api.propose.mockImplementationOnce(() => old.promise)
    const wrapper = await setup(); await begin(wrapper)
    auth.authEpoch++; auth.currentTenant.id = 2; await flushPromises()
    old.resolve({ data: { ...proposal(), decision: { ...proposal().decision, explanation: 'OLD_PRIVATE_PLAN' } } }); await flushPromises()
    expect(wrapper.text()).not.toContain('OLD_PRIVATE_PLAN'); expect(button(wrapper, '保存计划')).toBeUndefined(); wrapper.unmount()
  })
  it('coalesces repeated submission while assessment is pending', async () => {
    const assess = deferred<{ data: { ready: boolean; questions: []; execution_enabled: false } }>(); api.assess.mockImplementationOnce(() => assess.promise)
    const wrapper = await setup(); await fill(wrapper)
    await wrapper.get('form[aria-label="新建规划任务"]').trigger('submit'); await wrapper.get('form[aria-label="新建规划任务"]').trigger('submit')
    expect(api.assess).toHaveBeenCalledTimes(1)
    assess.resolve({ data: { ready: true, questions: [], execution_enabled: false } }); await flushPromises()
    expect(api.create).toHaveBeenCalledTimes(1); expect(api.propose).toHaveBeenCalledTimes(1); wrapper.unmount()
  })
  it.each([['conflict', { response: { status: 409 } }], ['uncertain network outcome', new Error('网络断开')]])('requires reconciliation after %s during save and does not automatically resubmit', async (_label, cause) => {
    const wrapper = await setup(); await begin(wrapper); api.appendPlan.mockRejectedValue(cause)
    await button(wrapper, '保存计划').trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('保存结果尚未确认'); expect(button(wrapper, '保存计划').attributes('disabled')).toBeDefined()
    expect(api.appendPlan).toHaveBeenCalledTimes(1)
    api.get.mockResolvedValue({ data: task('task-1', 2, 1) }); api.summary.mockResolvedValue({ data: summary('task-1', 2, 1) })
    await button(wrapper, '刷新任务状态').trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('已保存计划 v1'); expect(button(wrapper, '保存计划')).toBeUndefined()
    expect(api.appendPlan).toHaveBeenCalledTimes(1); wrapper.unmount()
  })
  it('loads later asset pages and rejects invalid integer budgets before calling the API', async () => {
    lifecycle.assets.mockResolvedValueOnce(page([asset()], 'page-two')).mockResolvedValueOnce(page([asset('asset-2', '后页资产')]))
    const wrapper = await setup(); await button(wrapper, '加载更多资产').trigger('click'); await flushPromises()
    expect(lifecycle.assets).toHaveBeenLastCalledWith('', 'page-two'); expect(wrapper.get('[aria-label="可选资产"]').text()).toContain('后页资产')
    await wrapper.get('[aria-label="最多并发数"]').setValue(2.5); await wrapper.get('form[aria-label="新建规划任务"]').trigger('submit'); await flushPromises()
    expect(wrapper.text()).toContain('之间的整数'); expect(api.assess).not.toHaveBeenCalled(); wrapper.unmount()
  })
  it('shows unsupported and invalid candidate outcomes without offering save', async () => {
    api.propose.mockResolvedValue({ data: { ...proposal(), decision: { kind: 'unsupported', explanation: '暂不支持写入外部数仓。' } } })
    const wrapper = await setup(); await begin(wrapper)
    expect(wrapper.text()).toContain('暂不支持写入外部数仓'); expect(button(wrapper, '保存计划')).toBeUndefined()
    api.propose.mockResolvedValue({ data: { ...proposal(), task_id: 'wrong-task' } })
    await button(wrapper, '重新生成候选').trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('任务身份或状态不匹配'); expect(button(wrapper, '保存计划')).toBeUndefined(); wrapper.unmount()
  })
  it('does not load or mutate task data without an authenticated workspace', async () => {
    auth.currentTenant.id = 0; const wrapper = await setup()
    expect(wrapper.text()).toContain('请登录并选择工作空间'); expect(api.list).not.toHaveBeenCalled(); expect(lifecycle.assets).not.toHaveBeenCalled(); wrapper.unmount()
  })
  it('treats a valid question ID named constructor as ordinary input and requires an explicit answer', async () => {
    api.propose.mockResolvedValueOnce({ data: { ...proposal(), decision: { kind: 'clarify', explanation: '确认范围', questions: [{ id: 'constructor', question: '保留哪些字段？' }] } } })
    const wrapper = await setup(); await begin(wrapper)
    expect((wrapper.get('[aria-label="保留哪些字段？"]').element as HTMLTextAreaElement).value).toBe('')
    await wrapper.get('form[aria-label="补充规划信息"]').trigger('submit'); await flushPromises()
    expect(wrapper.text()).toContain('请完整回答澄清问题'); expect(api.propose).toHaveBeenCalledTimes(1)
    await wrapper.get('[aria-label="保留哪些字段？"]').setValue('收入与日期')
    await wrapper.get('form[aria-label="补充规划信息"]').trigger('submit'); await flushPromises()
    expect(api.propose.mock.calls[1][2]).toEqual([{ question_id: 'constructor', question: '保留哪些字段？', answer: '收入与日期' }]); wrapper.unmount()
  })
})
