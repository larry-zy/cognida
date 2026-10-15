import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { reactive } from 'vue'

const api = vi.hoisted(() => ({ list: vi.fn(), get: vi.fn(), create: vi.fn(), update: vi.fn(), remove: vi.fn(), recall: vi.fn() }))
vi.mock('@/api/memory', () => ({ memoryApi: api }))
const auth = reactive({ authEpoch: 1, user: { id: 1 }, currentTenant: { id: 1 } })
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
import MemoryWorkbench from './MemoryWorkbench.vue'

const row = (id = 'memory-1', content = '中文回答', version = 1) => ({ id, content, version, category: 'preference', importance: 0.6, source: 'user', created_at: '2026-10-15T00:00:00Z', updated_at: '2026-10-15T00:00:00Z' })
const emptyPage = () => ({ data: { items: [], next_cursor: '', has_more: false } })
describe('personal memory workbench', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    auth.authEpoch++; auth.user.id = 1; auth.currentTenant.id = 1
    api.list.mockResolvedValue(emptyPage())
    api.get.mockResolvedValue({ data: row() })
    api.create.mockResolvedValue({ data: row() })
    api.update.mockResolvedValue({ data: row('memory-1', '英语回答', 2) })
    api.remove.mockResolvedValue({ data: { deleted: true } })
  })
  it('creates explicit memory and edits it with the server version', async () => {
    const wrapper = mount(MemoryWorkbench); await flushPromises()
    await wrapper.get('textarea[aria-label="记忆内容"]').setValue('中文回答')
    await wrapper.get('form[aria-label="编辑记忆"]').trigger('submit'); await flushPromises()
    expect(api.create).toHaveBeenCalledWith({ content: '中文回答', category: 'preference', importance: 0.6 }, expect.any(String))
    expect(wrapper.get('[role="status"]').text()).toContain('已保存')
    await wrapper.get('textarea[aria-label="记忆内容"]').setValue('英语回答')
    await wrapper.get('form[aria-label="编辑记忆"]').trigger('submit'); await flushPromises()
    expect(api.update).toHaveBeenCalledWith('memory-1', expect.objectContaining({ content: '英语回答' }), 1)
    expect(wrapper.text()).toContain('v2'); wrapper.unmount()
  })
  it('preserves input and reuses the idempotency key after a transient create failure', async () => {
    api.create.mockRejectedValueOnce(new Error('网络暂时不可用'))
    const wrapper = mount(MemoryWorkbench); await flushPromises()
    await wrapper.get('textarea[aria-label="记忆内容"]').setValue('保留输入')
    await wrapper.get('form[aria-label="编辑记忆"]').trigger('submit'); await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('网络暂时不可用')
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('保留输入')
    await wrapper.get('form[aria-label="编辑记忆"]').trigger('submit'); await flushPromises()
    expect(api.create.mock.calls[0][1]).toBe(api.create.mock.calls[1][1]); wrapper.unmount()
  })
  it('shows a conflict without discarding the proposed correction', async () => {
    api.list.mockResolvedValue({ data: { items: [row()], next_cursor: '' } })
    api.update.mockRejectedValue(new Error('记忆已被修改，请刷新后重试'))
    const wrapper = mount(MemoryWorkbench); await flushPromises()
    await wrapper.get('[aria-label="记忆列表"] button').trigger('click'); await flushPromises()
    await wrapper.get('textarea').setValue('未保存修正')
    await wrapper.get('form[aria-label="编辑记忆"]').trigger('submit'); await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('已被修改')
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('未保存修正'); wrapper.unmount()
  })
  it('forgets a selected memory and clears old recall output', async () => {
    api.list.mockResolvedValueOnce({ data: { items: [row()], next_cursor: '' } }).mockResolvedValue(emptyPage())
    api.recall.mockResolvedValue({ data: { items: [{ memory: row(), score: 1, method: 'lexical' }], method: 'lexical', candidate_limit: 500 } })
    const wrapper = mount(MemoryWorkbench); await flushPromises()
    await wrapper.get('[aria-label="记忆列表"] button').trigger('click'); await flushPromises()
    await wrapper.get('[aria-label="召回问题"]').setValue('中文')
    await wrapper.get('.memory-recall-form').trigger('submit'); await flushPromises()
    expect(wrapper.findAll('.memory-match')).toHaveLength(1)
    await wrapper.findAll('button').find(b => b.text() === '遗忘这条')!.trigger('click')
    await wrapper.findAll('button').find(b => b.text() === '确认遗忘')!.trigger('click'); await flushPromises()
    expect(api.remove).toHaveBeenCalledWith('memory-1', 1); expect(wrapper.findAll('.memory-match')).toHaveLength(0)
    expect(wrapper.text()).toContain('已遗忘'); wrapper.unmount()
  })
  it('drops a previous identity response and clears personal editor state immediately', async () => {
    let resolveOld!: (value: unknown) => void
    api.list.mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve })).mockResolvedValue(emptyPage())
    const wrapper = mount(MemoryWorkbench)
    await wrapper.get('textarea').setValue('OLD_PRIVATE_DRAFT')
    auth.authEpoch++; auth.currentTenant.id = 2; await flushPromises()
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('')
    resolveOld({ data: { items: [row('old', 'OLD_PRIVATE_RESPONSE')], next_cursor: '' } }); await flushPromises()
    expect(wrapper.text()).not.toContain('OLD_PRIVATE_RESPONSE'); expect(wrapper.text()).not.toContain('OLD_PRIVATE_DRAFT')
    wrapper.unmount()
  })
})
