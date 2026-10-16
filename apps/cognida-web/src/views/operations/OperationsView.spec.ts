import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick, reactive } from 'vue'
const api = vi.hoisted(() => ({ targets: vi.fn(), list: vi.fn(), get: vi.fn(), prepare: vi.fn(), confirm: vi.fn(), cancel: vi.fn() }))
vi.mock('@/api/governedops', () => ({ operationApi: api }))
const scope = reactive({ authEpoch: 0, user: { id: 1 }, currentTenant: { id: 1 } })
const route = reactive({ query: {} as Record<string, string> })
vi.mock('@/stores/auth', () => ({ useAuthStore: () => scope }))
vi.mock('vue-router', () => ({ useRoute: () => route, useRouter: () => ({ replace: vi.fn(async (r: { query: Record<string, string> }) => { route.query = r.query }) }) }))
import View from './OperationsView.vue'
const planned = { id: 'plan', version: 1, status: 'planned', plan: { request: { action: 'etl', target: 'agent_etl_sales', expected_version: 0 }, hash: 'immutable-hash', sql: 'INSERT fixed authorized snapshot', parameters: [], source_refs: [{ asset_id: 'input', version: 1 }], after: { columns: ['code', 'amount'], types: { code: 'text', amount: 'number' }, rows: [[{ type: 'text', text: '00123' }, { type: 'number', text: '9007199254740993.123456789012345678' }]] }, estimated_affected: 1 }, expires_at: '2026-10-16T10:00:00Z', affected_rows: 0 }
const setup = () => mount(View, { global: { stubs: { RouterLink: { props: ['to'], template: '<a><slot /></a>' } } } })
const button = (w: ReturnType<typeof setup>, text: string) => w.findAll('button').find(b => b.text() === text)!
describe('governed operation confirmation workbench', () => {
  beforeEach(() => { vi.clearAllMocks(); scope.authEpoch++; scope.currentTenant.id = 1; route.query = {}; api.targets.mockResolvedValue({ data: { items: [] } }); api.list.mockResolvedValue({ data: { items: [] } }); api.get.mockResolvedValue({ data: planned }) })
  it('shows independent errors and an honest empty state', async () => { api.targets.mockRejectedValue(new Error('offline')); const w = setup(); await nextTick(); expect(w.text()).toContain('正在读取'); await flushPromises(); expect(w.text()).toContain('受管目标列表读取失败'); expect(w.text()).toContain('尚无操作计划'); expect(w.text()).not.toContain('尚未提交派生目标'); w.unmount() })
  it('loads an exact immutable plan on refresh without confirming it', async () => { route.query = { job_id: 'plan' }; const w = setup(); await flushPromises(); expect(w.text()).toContain('9007199254740993.123456789012345678'); expect(w.text()).toContain('00123'); expect(button(w, '明确确认并提交').attributes('disabled')).toBeDefined(); expect(api.confirm).not.toHaveBeenCalled(); expect(api.prepare).not.toHaveBeenCalled(); w.unmount() })
  it('requires explicit confirmation and reads the actual receipt', async () => { route.query = { job_id: 'plan' }; const w = setup(); await flushPromises(); api.confirm.mockResolvedValue({ data: { ...planned, status: 'queued' } }); api.get.mockResolvedValue({ data: { ...planned, status: 'completed', committed_version: 1, affected_rows: 1, output_ref: { asset_id: 'result', version: 1 } } }); await w.get('input[type=checkbox]').setValue(true); await button(w, '明确确认并提交').trigger('click'); await flushPromises(); expect(api.confirm).toHaveBeenCalledWith(expect.objectContaining({ id: 'plan', version: 1, plan: expect.objectContaining({ hash: 'immutable-hash' }) }), expect.any(AbortSignal)); expect(w.text()).toContain('已提交版本 v1'); expect(w.text()).toContain('查看真实结果产物'); w.unmount() })
  it('reuses the preparation key after a lost response and never auto-confirms', async () => { api.prepare.mockRejectedValueOnce(new Error('lost')).mockResolvedValueOnce({ data: planned }); const w = setup(); await flushPromises(); const form = w.get('form'); await form.find('input[list]').setValue('agent_etl_sales'); await form.find('input[placeholder="选择处理产物或查询结果的资产 ID"]').setValue('asset'); await form.trigger('submit'); await flushPromises(); await form.trigger('submit'); await flushPromises(); expect(api.prepare.mock.calls[0]![1]).toEqual(api.prepare.mock.calls[1]![1]); expect(api.confirm).not.toHaveBeenCalled(); expect(w.text()).toContain('尚未提交数据变更'); w.unmount() })
  it('hides revoked output and discards late reads from the prior identity', async () => { route.query = { job_id: 'plan' }; api.get.mockResolvedValueOnce({ data: { ...planned, status: 'completed', output_ref: { asset_id: 'result', version: 1 } } }); const w = setup(); await flushPromises(); expect(w.text()).toContain('查看真实结果产物'); api.get.mockRejectedValueOnce({ response: { status: 403 } }); await button(w, '重新核对状态').trigger('click'); await flushPromises(); expect(w.text()).not.toContain('查看真实结果产物'); let resolve!: (x: unknown) => void; api.get.mockImplementationOnce(() => new Promise(r => { resolve = r })); await button(w, '重新核对状态').trigger('click'); api.get.mockRejectedValue({ response: { status: 404 } }); scope.currentTenant.id = 2; await flushPromises(); resolve({ data: planned }); await flushPromises(); expect(w.text()).not.toContain('immutable-hash'); expect(w.text()).toContain('当前不可访问'); w.unmount() })
  it.each([401, 403, 404])('clears revoked list data and pagination after HTTP %s', async (status) => {
    api.targets.mockResolvedValueOnce({ data: { items: [{ id: 'private-target', name: 'private_target', version: 2 }] } })
    api.list.mockResolvedValueOnce({ data: { items: [{ ...planned, plan: { ...planned.plan, request: { ...planned.plan.request, target: 'private_history' } } }], next_cursor: 'next-private-page' } })
    const w = setup(); await flushPromises()
    expect(w.text()).toContain('private_target'); expect(w.text()).toContain('private_history')
    api.targets.mockRejectedValue({ response: { status } }); api.list.mockRejectedValue({ response: { status } })
    await button(w, '刷新列表').trigger('click'); await flushPromises()
    expect(w.text()).not.toContain('private_target'); expect(w.text()).not.toContain('private_history')
    expect(w.findAll('button').some(b => b.text() === '继续读取操作')).toBe(false)
    expect(w.text()).toContain('受管目标列表读取失败'); expect(w.text()).toContain('操作列表读取失败')
    w.unmount()
  })
  it('keeps independent successful lists and labels retained server-error data as stale', async () => {
    api.targets.mockResolvedValueOnce({ data: { items: [{ id: 'target', name: 'last_target', version: 2 }] } })
    api.list.mockResolvedValueOnce({ data: { items: [planned] } })
    const w = setup(); await flushPromises()
    api.targets.mockRejectedValue({ response: { status: 503 } }); api.list.mockRejectedValue({ response: { status: 403 } })
    await button(w, '刷新列表').trigger('click'); await flushPromises()
    expect(w.text()).toContain('last_target'); expect(w.text()).toContain('下方为上次读取的内容')
    expect(w.text()).not.toContain('agent_etl_sales')
    api.targets.mockRejectedValue({ response: { status: 403 } }); api.list.mockResolvedValue({ data: { items: [planned] } })
    await button(w, '刷新列表').trigger('click'); await flushPromises()
    expect(w.text()).not.toContain('last_target'); expect(w.text()).toContain('agent_etl_sales')
    w.unmount()
  })
  it('does not overwrite a newly opened operation with a late cancellation response', async () => {
    route.query = { job_id: 'plan' }
    const w = setup(); await flushPromises()
    let resolve!: (value: unknown) => void
    api.cancel.mockImplementationOnce(() => new Promise(r => { resolve = r }))
    await button(w, '取消此操作').trigger('click')
    api.get.mockResolvedValue({ data: { ...planned, id: 'other-plan', plan: { ...planned.plan, hash: 'other-hash' } } })
    route.query = { job_id: 'other-plan' }; await flushPromises()
    resolve({ data: { ...planned, status: 'cancelled' } }); await flushPromises()
    expect(w.text()).toContain('other-hash'); expect(w.text()).not.toContain('已取消')
    w.unmount()
  })

})
