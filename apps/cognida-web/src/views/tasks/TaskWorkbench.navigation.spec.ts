import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent, reactive } from 'vue'
import { createMemoryHistory, createRouter, RouterView, useRoute } from 'vue-router'
import type { GovernedTask, PlanSpec, Proposal } from '@/api/taskcontrol'

const api = vi.hoisted(() => ({ capabilities: vi.fn(), list: vi.fn(), get: vi.fn(), assess: vi.fn(), create: vi.fn(), propose: vi.fn(), appendPlan: vi.fn(), plan: vi.fn(), summary: vi.fn() }))
const lifecycle = vi.hoisted(() => ({ assets: vi.fn() }))
vi.mock('@/api/taskcontrol', () => ({ taskControlApi: api }))
vi.mock('@/api/lifecycle', () => ({ lifecycleApi: lifecycle }))
const auth = reactive({ authEpoch: 1, user: { id: 1 }, currentTenant: { id: 1 } })
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
import TaskWorkbench from './TaskWorkbench.vue'

const input = { asset_id: 'asset-1', version: 1 }
const budget = { max_nodes: 8, max_concurrency: 2, max_attempts: 16, max_calls: 64, max_cost_micros: 1000000, timeout_seconds: 600 }
const task = (id = 'task-1', version = 1, currentPlan = 0): GovernedTask => ({ id, goal: `${id} 清理空白`, input_refs: [input], budget, status: 'draft', version, current_plan_version: currentPlan, result_refs: [], created_at: '', updated_at: '' })
const plan: PlanSpec = { schema_version: 1, budget, nodes: [{ id: 'clean', capability: 'lifecycle.data-cleaning', capability_version: 1, depends_on: [], inputs: [{ asset: input, kind: 'raw_asset' }], output_kinds: ['cleaned_asset'], tools: ['clean'], permissions: ['read'], max_attempts: 1, max_calls_per_attempt: 1, max_cost_micros_per_attempt: 0, timeout_seconds: 30 }] }
const candidate: Proposal = { task_id: 'task-1', task_version: 1, decision: { kind: 'proposal', explanation: '未保存的方案', plan }, validation: { valid: true, execution_enabled: false, authorization_scope: 'lifecycle_asset', origin_revalidated: false, topological_order: ['clean'], approval_required: [], input_refs: [input], reserved_attempts: 1, reserved_calls: 1, reserved_cost_micros: 0, reserved_seconds: 30 }, execution_enabled: false, planning_calls: 1, saved: false }
const saved = { task_id: 'task-1', version: 1, snapshot: plan, content_hash: 'saved-plan-hash', created_at: '' }
const wrappers: VueWrapper[] = []
const button = (wrapper: VueWrapper, text: string) => wrapper.findAll('button').find(item => item.text() === text)!
async function setup(path = '/tasks', keyed = true) {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/tasks', component: TaskWorkbench }, { path: '/:pathMatch(.*)*', component: { template: '<p>Other page</p>' } }] })
  await router.push(path); await router.isReady()
  // Reproduce the platform's fullPath-keyed view, and verify a retained host.
  const host = defineComponent({ components: { RouterView }, setup: () => ({ route: useRoute(), keyed }), template: '<RouterView :key="keyed ? route.fullPath : undefined" />' })
  const mountHost = () => { const wrapper = mount(host, { global: { plugins: [router] } }); wrappers.push(wrapper); return wrapper }
  const wrapper = mountHost(); await flushPromises(); return { wrapper, router, mountHost }
}
async function begin(wrapper: VueWrapper) {
  await wrapper.get('[aria-label="任务目标"]').setValue('task-1 清理空白')
  await wrapper.get('[aria-label="可选资产"]').setValue('asset-1:1')
  await button(wrapper, '添加输入').trigger('click')
  await wrapper.get('form[aria-label="新建规划任务"]').trigger('submit'); await flushPromises()
}
describe('task planning with real route transitions', () => {
  beforeEach(() => {
    vi.clearAllMocks(); vi.spyOn(window, 'confirm').mockReturnValue(false)
    api.capabilities.mockResolvedValue({ data: { capabilities: [], execution_enabled: false } })
    api.list.mockResolvedValue({ data: { items: [task('task-2')], next_cursor: '' } })
    lifecycle.assets.mockResolvedValue({ data: { items: [{ ...input, name: '输入快照', kind: 'raw_asset', status: 'available' }], next_cursor: '' } })
    api.assess.mockResolvedValue({ data: { ready: true, questions: [], execution_enabled: false } })
    api.create.mockResolvedValue({ data: task() }); api.propose.mockResolvedValue({ data: candidate })
    api.get.mockImplementation((id: string) => Promise.resolve({ data: task(id) }))
    api.summary.mockImplementation((id: string) => Promise.resolve({ data: { task_id: id, task_version: 1, status: 'draft', current_plan_version: 0, planned_nodes: 0, children: [], result_refs: [], warnings: [], execution_enabled: false } }))
    api.appendPlan.mockResolvedValue({ data: { plan: saved, validation: candidate.validation } }); api.plan.mockResolvedValue({ data: saved })
  })
  afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()); vi.restoreAllMocks() })
  it('protects an unsaved candidate on its same-page link and synchronizes confirmed list selection with the URL', async () => {
    const { wrapper, router, mountHost } = await setup(); await begin(wrapper)
    await wrapper.get('a[href="/tasks?task_id=task-1"]').trigger('click'); await flushPromises()
    expect(window.confirm).toHaveBeenCalledTimes(1); expect(router.currentRoute.value.fullPath).toBe('/tasks')
    expect(wrapper.text()).toContain('未保存的方案'); expect(api.get).not.toHaveBeenCalled()
    await wrapper.get('.task-list button').trigger('click'); await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/tasks'); expect(wrapper.text()).toContain('未保存的方案')
    vi.mocked(window.confirm).mockReturnValue(true)
    await wrapper.get('.task-list button').trigger('click'); await flushPromises()
    expect(router.currentRoute.value.query.task_id).toBe('task-2'); expect(wrapper.get('.task-goal').text()).toBe('task-2 清理空白')
    expect(wrapper.text()).not.toContain('未保存的方案')
    wrapper.unmount(); const restored = mountHost(); await flushPromises()
    expect(restored.get('.task-goal').text()).toBe('task-2 清理空白'); expect(api.create).toHaveBeenCalledTimes(1)
  })
  it.each([true, false])('reconciles an explicit first save and restores the saved plan (fullPath remount: %s)', async keyed => {
    const { wrapper, router, mountHost } = await setup('/tasks', keyed); await begin(wrapper)
    api.get.mockResolvedValue({ data: task('task-1', 2, 1) })
    api.summary.mockResolvedValue({ data: { task_id: 'task-1', task_version: 2, status: 'ready', current_plan_version: 1, planned_nodes: 1, children: [], result_refs: [], warnings: [], execution_enabled: false } })
    await button(wrapper, '保存计划').trigger('click'); await flushPromises()
    expect(api.appendPlan).toHaveBeenCalledTimes(1); expect(window.confirm).not.toHaveBeenCalled()
    expect(router.currentRoute.value.query.task_id).toBe('task-1'); expect(api.get).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('任务版本 2'); expect(wrapper.text()).toContain('已保存计划 v1'); expect(wrapper.text()).not.toContain('尚未保存')
    wrapper.unmount(); const restored = mountHost(); await flushPromises()
    expect(restored.text()).toContain('saved-plan-hash'); expect(api.create).toHaveBeenCalledTimes(1); expect(api.propose).toHaveBeenCalledTimes(1)
  })
  it('protects same-route history navigation and aborts a pending request only after confirmed departure', async () => {
    const { wrapper, router } = await setup('/tasks?task_id=task-2')
    await router.push('/tasks'); await flushPromises()
    let resolve!: (value: { data: Proposal }) => void
    api.propose.mockImplementation(() => new Promise(result => { resolve = result }))
    await begin(wrapper); const signal = api.propose.mock.calls[0][3] as AbortSignal
    router.back(); await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/tasks'); expect(signal.aborted).toBe(false)
    expect(window.confirm).toHaveBeenCalledWith(expect.stringContaining('请求尚未结束'))
    vi.mocked(window.confirm).mockReturnValue(true)
    await router.push('/elsewhere'); await flushPromises(); expect(signal.aborted).toBe(true)
    resolve({ data: candidate }); await flushPromises()
    expect(wrapper.text()).toBe('Other page'); expect(api.appendPlan).not.toHaveBeenCalled()
  })
})
