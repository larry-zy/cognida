import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, RouterLinkStub, type VueWrapper } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { defineComponent, reactive } from 'vue'
const api = vi.hoisted(() => ({ list: vi.fn(), getById: vi.fn(), getRun: vi.fn(), create: vi.fn(), start: vi.fn(), cancel: vi.fn() }))
const lifecycle = vi.hoisted(() => ({ knowledgeBases: vi.fn() }))
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))
const route = vi.hoisted(() => ({ query: {} as Record<string, unknown> }))
const navigation = vi.hoisted(() => ({ leave: vi.fn() }))
vi.mock('vue-router', () => ({ useRoute: () => reactive(route), onBeforeRouteLeave: navigation.leave }))
vi.mock('@/api/collection', () => ({ collectionApi: api }))
vi.mock('@/api/lifecycle', () => ({ lifecycleApi: lifecycle }))
vi.mock('@/utils/toast', () => ({ default: toast }))
vi.mock('@/utils/confirm', () => ({ ElMessageBox: { confirm: vi.fn().mockResolvedValue(undefined) } }))
import { useAuthStore } from '@/stores/auth'
import CollectionTaskList from './CollectionTaskList.vue'
const Select = defineComponent({
  props: ['modelValue', 'options'], emits: ['update:modelValue'],
  template: `<select :value="modelValue" @change="$emit('update:modelValue', $event.target.value)"><option value="">选择</option><option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option></select>`,
})
const task = (id: string, status = 'completed') => ({ id, knowledge_base_id: 'kb-one', status })
const payload = (id = 'task-one') => ({
  task: task(id), sources: [{ id: 'source-one', url: 'https://example.com' }, { id: 'source-two', url: 'https://example.org' }],
  run: { id: `run-${id}`, status: 'completed', started_at: '2026-10-15T01:00:00Z', completed_at: '2026-10-15T01:01:00Z' },
  items: [
    { id: `item-${id}`, run_id: `run-${id}`, source_url: 'https://example.com', status: 'accepted', knowledge_id: 'document-one', normalized_object_key: 'normalized/one', normalized_size: 120, http_status: 200, content_type: 'text/html', content_hash: 'content-fingerprint' },
    { id: 'item-failed', run_id: `run-${id}`, source_url: 'https://example.org', status: 'failed', error_reason: 'Remote source is unavailable' },
  ],
  summary: { outcome: 'partial', total: 2, accepted: 1, skipped: 0, rejected: 0, failed: 1, pending: 0, usable: 1 },
})
let wrappers: VueWrapper[] = []
function setup() {
  const pinia = createPinia()
  const wrapper = mount(CollectionTaskList, { global: { plugins: [pinia], stubs: { teleport: true, RouterLink: RouterLinkStub, UiSelect: Select } } })
  wrappers.push(wrapper)
  return { wrapper, auth: useAuthStore(pinia) }
}
function button(wrapper: VueWrapper, label: string) {
  const found = wrapper.findAll('button').find(node => node.text() === label)
  if (!found) throw new Error(`Button not found: ${label}`)
  return found
}
async function fillCreate(wrapper: VueWrapper) {
  await button(wrapper, '创建采集任务').trigger('click')
  await wrapper.get('form select').setValue('kb-one')
  await wrapper.get('form textarea').setValue('https://example.com')
}
describe('collection source to lifecycle flow', () => {
  beforeEach(() => {
    vi.resetAllMocks(); vi.useFakeTimers()
    route.query = {}
    api.list.mockResolvedValue({ data: { tasks: [task('task-one'), task('task-two')] } })
    api.getById.mockImplementation((id: string) => Promise.resolve({ data: payload(id) }))
    lifecycle.knowledgeBases.mockResolvedValue({ data: { items: [{ id: 'kb-one', name: 'First knowledge base' }], total: 1, page: 1, size: 100 } })
  })
  afterEach(() => { wrappers.forEach(wrapper => wrapper.unmount()); wrappers = []; vi.useRealTimers(); vi.restoreAllMocks() })
  it('reads knowledge base pages and allows selecting the later page', async () => {
    lifecycle.knowledgeBases.mockImplementation((page: number) => Promise.resolve({ data: { items: [{ id: page === 1 ? 'kb-one' : 'kb-two', name: page === 1 ? 'First knowledge base' : 'Second knowledge base' }], total: 2, page, size: 1 } }))
    const { wrapper } = setup(); await flushPromises()
    await button(wrapper, '创建采集任务').trigger('click')
    expect(wrapper.get('form select').text()).toContain('First knowledge base')
    await button(wrapper, '加载更多知识库').trigger('click'); await flushPromises()
    expect(lifecycle.knowledgeBases).toHaveBeenNthCalledWith(2, 2)
    expect(wrapper.get('form select').text()).toContain('Second knowledge base')
    expect(wrapper.text()).not.toContain('加载更多知识库')
  })
  it('shows persisted provenance, partial failure and reviewable navigation links', async () => {
    const { wrapper } = setup(); await flushPromises()
    await button(wrapper, 'task-one').trigger('click'); await flushPromises()
    expect(api.getById).toHaveBeenCalledTimes(1)
    for (const text of ['run-task-one', 'item-task-one', '部分来源采集失败', '已采集 1 · 跳过 0 · 拒绝 0 · 失败 1', 'Remote source is unavailable', 'content-fingerprint']) expect(wrapper.text()).toContain(text)
    const links = wrapper.findAllComponents(RouterLinkStub)
    expect(links.find(link => link.text() === '转入流水线')!.props('to')).toEqual({ path: '/pipelines', query: { source: 'collection', run_id: 'run-task-one', item_id: 'item-task-one', name: 'https://example.com' } })
    expect(links.filter(link => link.text() === '转入流水线')).toHaveLength(1)
    expect(links.find(link => link.text().includes('查看知识库'))!.props('to')).toBe('/knowledge/kb-one')
    expect(api.create).not.toHaveBeenCalled()
  })
  it('does not overwrite a newer selected task with an old detail response', async () => {
    let finish!: (value: unknown) => void
    api.getById.mockImplementation((id: string) => id === 'task-one' ? new Promise(resolve => { finish = resolve }) : Promise.resolve({ data: payload(id) }))
    const { wrapper } = setup(); await flushPromises()
    await button(wrapper, 'task-one').trigger('click'); await button(wrapper, 'task-two').trigger('click'); await flushPromises()
    finish({ data: payload('task-one') }); await flushPromises()
    expect(wrapper.text()).toContain('run-task-two'); expect(wrapper.text()).not.toContain('run-task-one')
  })
  it('polls running details and stops once terminal', async () => {
    const running = payload(); running.task.status = 'running'; running.run.status = 'running'; running.summary.outcome = 'running'
    api.getById.mockResolvedValueOnce({ data: running }).mockResolvedValue({ data: payload() })
    const { wrapper } = setup(); await flushPromises()
    await button(wrapper, 'task-one').trigger('click'); await flushPromises()
    await vi.advanceTimersByTimeAsync(4000); expect(api.getById).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(8000); expect(api.getById).toHaveBeenCalledTimes(2)
  })
  it('reuses an interrupted creation key and rejects unsafe or excessive URLs', async () => {
    api.create.mockRejectedValue(new Error('Temporary interruption'))
    const { wrapper } = setup(); await flushPromises(); await fillCreate(wrapper)
    await button(wrapper, '创建').trigger('click'); await flushPromises()
    await button(wrapper, '创建').trigger('click'); await flushPromises()
    expect(api.create).toHaveBeenCalledTimes(2)
    expect(api.create.mock.calls[0][0].idempotency_key).toBe(api.create.mock.calls[1][0].idempotency_key)
    await wrapper.get('form textarea').setValue('https://user:password@example.com')
    await button(wrapper, '创建').trigger('click'); await flushPromises(); expect(api.create).toHaveBeenCalledTimes(2)
    await wrapper.get('form textarea').setValue(Array.from({ length: 51 }, (_, index) => `https://example.com/${index}`).join('\n'))
    await button(wrapper, '创建').trigger('click'); await flushPromises(); expect(api.create).toHaveBeenCalledTimes(2)
    expect(toast.error).toHaveBeenCalledWith('请填写 1 到 50 个 HTTPS 地址')
  })
  it('discards old tenant knowledge and detail responses', async () => {
    let oldKB!: (value: unknown) => void, oldDetail!: (value: unknown) => void
    lifecycle.knowledgeBases.mockReturnValueOnce(new Promise(resolve => { oldKB = resolve })).mockResolvedValue({ data: { items: [{ id: 'new-kb', name: 'New tenant KB' }], total: 1, page: 1, size: 100 } })
    api.getById.mockImplementationOnce(() => new Promise(resolve => { oldDetail = resolve }))
    const { wrapper, auth } = setup(); await flushPromises()
    await button(wrapper, 'task-one').trigger('click')
    api.list.mockResolvedValue({ data: { tasks: [] } }); auth.authEpoch++; await flushPromises()
    oldKB({ data: { items: [{ id: 'old-kb', name: 'Old tenant private KB' }], total: 1, page: 1, size: 100 } })
    oldDetail({ data: payload('old-private-task') }); await flushPromises()
    await button(wrapper, '创建采集任务').trigger('click')
    expect(wrapper.text()).toContain('New tenant KB'); expect(wrapper.text()).not.toContain('Old tenant private KB'); expect(wrapper.text()).not.toContain('old-private-task')
  })
  it('surfaces detail failure and supports a safe retry', async () => {
    api.getById.mockRejectedValueOnce(new Error('Denied for this tenant'))
    const { wrapper } = setup(); await flushPromises()
    await button(wrapper, 'task-one').trigger('click'); await flushPromises(); expect(wrapper.text()).toContain('Denied for this tenant')
    await button(wrapper, '重试加载详情').trigger('click'); await flushPromises(); expect(wrapper.text()).toContain('run-task-one')
  })
  it('distinguishes initial loading from an actionable empty list', async () => {
    let finish!: (value: unknown) => void
    api.list.mockReturnValue(new Promise(resolve => { finish = resolve }))
    const { wrapper } = setup(); await flushPromises()
    expect(wrapper.text()).toContain('正在加载采集任务…')
    expect(wrapper.text()).not.toContain('从第一个网页开始')
    finish({ data: { tasks: [] } }); await flushPromises()
    await button(wrapper, '新建第一个采集任务').trigger('click')
    expect(wrapper.find('form').exists()).toBe(true)
  })
  it('opens task links read-only and fences stale results when the query changes', async () => {
    route.query = { task_id: 'old-private' }
    let finish!: (value: unknown) => void
    api.getById.mockImplementation((id: string) => id === 'old-private' ? new Promise(resolve => { finish = resolve }) : Promise.reject(new Error('Task not found or not authorized')))
    const { wrapper } = setup(); await flushPromises()
    expect(wrapper.text()).toContain('正在加载任务详情')
    reactive(route).query = { task_id: 'new-task' }; await flushPromises()
    expect(wrapper.text()).toContain('Task not found or not authorized')
    finish({ data: payload('old-private') }); await flushPromises()
    expect(wrapper.text()).not.toContain('run-old-private')
    expect(api.create).not.toHaveBeenCalled(); expect(api.start).not.toHaveBeenCalled()
  })
  it('keeps a closed creation draft and warns on page leave until it is submitted', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    api.create.mockResolvedValue({ data: task('created-task', 'draft') })
    const { wrapper } = setup(); await flushPromises(); await fillCreate(wrapper)
    await button(wrapper, '稍后继续').trigger('click')
    await button(wrapper, '创建采集任务').trigger('click')
    expect((wrapper.get('form textarea').element as HTMLTextAreaElement).value).toBe('https://example.com')
    const guard = navigation.leave.mock.calls.at(-1)![0]
    expect(guard()).toBe(false); expect(confirm).toHaveBeenCalledOnce()
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(api.create).toHaveBeenCalledOnce()
    expect(guard()).toBe(true)
  })
  it('filters failed sources and prepares a fresh retry draft without starting it', async () => {
    const { wrapper } = setup(); await flushPromises()
    await button(wrapper, 'task-one').trigger('click'); await flushPromises()
    expect(wrapper.get('progress').attributes('value')).toBe('2')
    await button(wrapper, '失败与拒绝').trigger('click')
    expect(wrapper.findAllComponents(RouterLinkStub).some(link => link.text() === '转入流水线')).toBe(false)
    expect(wrapper.text()).toContain('Remote source is unavailable')
    await button(wrapper, '新建任务重新采集').trigger('click')
    expect((wrapper.get('form textarea').element as HTMLTextAreaElement).value).toBe('https://example.org')
    expect(api.create).not.toHaveBeenCalled(); expect(api.start).not.toHaveBeenCalled()
  })
  it('opens an exact run deep link and exposes candidate continuation without claiming publication', async () => {
    route.query = { run_id: 'run-task-one' }; const result = payload(); result.items[0]!.status = 'candidate'; result.items[0]!.knowledge_id = ''; api.getRun.mockResolvedValue({ data: result })
    const { wrapper } = setup(); await flushPromises(); expect(api.getRun).toHaveBeenCalledWith('run-task-one'); expect(api.getById).not.toHaveBeenCalled(); expect(wrapper.text()).toContain('候选 · 待审批'); expect(wrapper.text()).toContain('展示指定运行'); expect(wrapper.text()).toContain('转入流水线'); expect(wrapper.text()).not.toContain('已入库'); expect(api.start).not.toHaveBeenCalled()
  })

})
