import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { reactive } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'

const api = vi.hoisted(() => ({ runs: vi.fn(), invocations: vi.fn(), assets: vi.fn(), collection: vi.fn() }))
vi.mock('@/api/overview', () => ({ overviewApi: api }))
const auth = reactive({ authEpoch: 1, user: { id: 1 }, currentTenant: { id: 1 } })
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
import HomeView from './HomeView.vue'
const empty = () => ({ items: [], hasMore: false })
const run = (id: string, status = 'awaiting_approval') => ({ id, status, template_id: '完整数据流程', created_at: '2026-10-15T08:00:00Z', updated_at: '2026-10-15T08:10:00Z' })
async function setup() {
  const router = createRouter({ history: createMemoryHistory(), routes: ['/home', '/pipelines', '/experts', '/collection-tasks', '/analysis', '/knowledge', '/kb-assistant', '/subscriptions'].map(path => ({ path, component: { template: '<div />' } })) })
  await router.push('/home'); await router.isReady()
  const wrapper = mount(HomeView, { global: { plugins: [router] } })
  return { wrapper, router }
}

describe('real workspace overview states', () => {
  beforeEach(() => {
    vi.clearAllMocks(); auth.authEpoch++; auth.user.id = 1; auth.currentTenant.id = 1
    for (const read of Object.values(api)) read.mockResolvedValue(empty())
  })
  it('has useful empty-state entry points and scoped counts without invented history totals', async () => {
    const { wrapper } = await setup(); await flushPromises()
    expect(wrapper.text()).toContain('当前读取范围内没有待处理事项')
    expect(wrapper.text()).toContain('不代表历史总量')
    expect(wrapper.findAll('.source-status__count')).toHaveLength(4)
    expect(wrapper.get('nav[aria-label="数据工作流程"] a').attributes('href')).toBe('/collection-tasks')
    expect(wrapper.text()).toContain('还没有可查看的发布资产')
    wrapper.unmount()
  })
  it('keeps loading, failures, permissions and successful modules distinct and retries just one module', async () => {
    let resolveRuns!: (value: unknown) => void
    api.runs.mockImplementationOnce(() => new Promise(resolve => { resolveRuns = resolve }))
    api.invocations.mockRejectedValue({ response: { status: 403 } })
    api.collection.mockRejectedValueOnce(new Error('采集服务暂时不可用')).mockResolvedValue(empty())
    api.assets.mockResolvedValue({ items: [{ asset_id: 'published', version: 2, name: '可用资产', created_at: '2026-10-15T08:00:00Z' }], hasMore: false })
    const { wrapper } = await setup(); await flushPromises()
    expect(wrapper.get('[data-section="runs"]').text()).toContain('正在读取')
    expect(wrapper.get('[data-section="runs"]').find('.source-status__count').exists()).toBe(false)
    expect(wrapper.get('[data-section="invocations"]').text()).toContain('没有查看此模块的权限')
    expect(wrapper.get('[data-section="invocations"]').find('.source-status__count').exists()).toBe(false)
    expect(wrapper.text()).toContain('可用资产')
    await wrapper.get('[data-section="collection"] button').trigger('click'); await flushPromises()
    expect(api.collection).toHaveBeenCalledTimes(2); expect(api.assets).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-section="collection"]').text()).toContain('0条已读取')
    resolveRuns(empty()); await flushPromises(); wrapper.unmount()
  })
  it('opens the real approval, expert, collection and published-version destinations', async () => {
    api.runs.mockResolvedValue({ items: [run('run-1')], hasMore: true })
    api.invocations.mockResolvedValue({ items: [{ ...run('call-1', 'failed'), agent_id: 'data-quality', error: '质量连接失败' }], hasMore: false })
    api.collection.mockResolvedValue({ items: [{ id: 'task-1', status: 'draft', created_at: '2026-10-15T08:00:00Z' }], hasMore: false })
    api.assets.mockResolvedValue({ items: [{ asset_id: 'asset-1', version: 3, name: '收入表', created_at: '2026-10-15T08:00:00Z' }], hasMore: false })
    const { wrapper, router } = await setup(); await flushPromises()
    expect(wrapper.text()).toContain('还有更多')
    const links = wrapper.findAll('[aria-label="待处理任务"] a')
    expect(links.map(link => link.attributes('href'))).toEqual(expect.arrayContaining(['/pipelines?run_id=run-1', '/experts?invocation_id=call-1', '/collection-tasks?task_id=task-1']))
    await links.find(link => link.text().includes('前往审批'))!.trigger('click'); await flushPromises()
    expect(router.currentRoute.value.query.run_id).toBe('run-1')
    expect(wrapper.get('.asset-list a').attributes('href')).toBe('/analysis?asset=asset-1:3')
    wrapper.unmount()
  })
  it('aborts and drops prior-tenant requests and clears previous private records', async () => {
    let resolveOld!: (value: unknown) => void
    api.runs.mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve })).mockResolvedValue(empty())
    api.assets.mockResolvedValueOnce({ items: [{ asset_id: 'private', version: 1, name: 'OLD_PRIVATE_ASSET', created_at: '' }], hasMore: false }).mockResolvedValue(empty())
    const { wrapper } = await setup(); await flushPromises()
    const oldSignal = api.runs.mock.calls[0][0] as AbortSignal
    expect(wrapper.text()).toContain('OLD_PRIVATE_ASSET')
    auth.authEpoch++; auth.currentTenant.id = 2; await flushPromises()
    expect(oldSignal.aborted).toBe(true); expect(wrapper.text()).not.toContain('OLD_PRIVATE_ASSET')
    resolveOld({ items: [run('OLD_PRIVATE_RUN')], hasMore: false }); await flushPromises()
    expect(wrapper.text()).not.toContain('OLD_PRIVATE_RUN')
    wrapper.unmount()
  })
  it('does not read without an authenticated workspace', async () => {
    auth.currentTenant.id = 0
    const { wrapper } = await setup(); await flushPromises()
    expect(api.runs).not.toHaveBeenCalled(); expect(wrapper.text()).toContain('请登录并选择工作空间')
    wrapper.unmount()
  })
  it('aborts outstanding reads when leaving the page', async () => {
    api.runs.mockImplementation(() => new Promise(() => {}))
    const { wrapper } = await setup()
    const signal = api.runs.mock.calls[0][0] as AbortSignal
    wrapper.unmount()
    expect(signal.aborted).toBe(true)
  })

  it('keeps completed collections with partial failures actionable without changing their terminal status', async () => {
    api.collection.mockResolvedValue({ items: [
      { id: 'partial-task', status: 'completed', error_message: '部分来源失败：2/3 可用', created_at: '2026-10-15T08:00:00Z' },
      { id: 'success-task', status: 'completed', created_at: '2026-10-15T08:00:00Z' }
    ], hasMore: false })
    const { wrapper } = await setup(); await flushPromises()
    const pending = wrapper.get('[aria-label="待处理任务"]')
    expect(pending.findAll('li')).toHaveLength(1)
    expect(pending.text()).toContain('采集部分失败')
    expect(pending.text()).toContain('部分来源失败：2/3 可用')
    expect(pending.get('.attention-dot').classes()).toContain('tone-warning')
    expect(pending.get('a').attributes('href')).toBe('/collection-tasks?task_id=partial-task')
    expect(pending.get('a').text()).toContain('查看来源')
    expect(wrapper.get('.recent-list').text()).toContain('采集已结束')
    wrapper.unmount()
  })

})
