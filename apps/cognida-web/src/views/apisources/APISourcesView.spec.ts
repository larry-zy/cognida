import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick, reactive } from 'vue'
const api = vi.hoisted(() => ({ sources: vi.fn(), credentials: vi.fn(), run: vi.fn(), start: vi.fn(), createCredential: vi.fn(), create: vi.fn(), update: vi.fn(), cancel: vi.fn(), resume: vi.fn(), enabled: vi.fn(), disableCredential: vi.fn() }))
vi.mock('@/api/apisource', () => ({ apiSourceApi: api }))
const scope = reactive({ authEpoch: 0, user: { id: 1 }, currentTenant: { id: 1 } })
const route = reactive({ path: '/api-sources', query: {} as Record<string, string> })
vi.mock('@/stores/auth', () => ({ useAuthStore: () => scope }))
vi.mock('vue-router', () => ({ useRoute: () => route, useRouter: () => ({ replace: vi.fn(async (value: { query: Record<string, string> }) => { route.query = value.query }) }) }))
import View from './APISourcesView.vue'
const source = { id: 'source', name: 'Orders', version: 1, enabled: true, config: { url: 'https://example.com/records', credential_id: 'credential', items_pointer: '/items', record_key: 'id', pagination: { mode: 'none' }, limits: { max_pages: 10, max_records: 100, max_bytes: 8192, max_page_bytes: 4096, max_requests: 20, timeout_seconds: 120 } } }
const completed = { id: 'run', source_id: 'source', source_version: 1, version: 3, status: 'completed', consistency: 'provider_snapshot_token', pages: [{ number: 1, digest: 'sha256', bytes: 200, records: 2 }], record_count: 2, duplicate_count: 0, deleted_count: 0, output_refs: [{ asset_id: 'candidate', version: 1 }] }
const setup = () => mount(View, { global: { stubs: { RouterLink: { props: ['to'], template: '<a><slot /></a>' } } } })
describe('authenticated JSON API source workbench', () => {
  beforeEach(() => { vi.clearAllMocks(); scope.authEpoch++; scope.currentTenant.id = 1; route.query = {}; api.sources.mockResolvedValue({ data: { items: [] } }); api.credentials.mockResolvedValue({ data: { items: [] } }); api.run.mockResolvedValue({ data: completed }) })
  it('distinguishes an empty list from independent module failures', async () => {
    api.credentials.mockRejectedValue(new Error('offline')); const wrapper = setup(); await nextTick(); expect(wrapper.text()).toContain('正在读取'); await flushPromises(); expect(wrapper.text()).toContain('尚未配置来源'); expect(wrapper.text()).toContain('凭据列表读取失败'); wrapper.unmount()
  })
  it('refreshes a deep-linked receipt without starting ingestion and hides revoked output', async () => {
    route.query = { run_id: 'run' }; const wrapper = setup(); await flushPromises(); expect(wrapper.text()).toContain('完整候选已提交'); expect(wrapper.text()).toContain('查看候选产物'); expect(api.start).not.toHaveBeenCalled(); api.run.mockRejectedValue({ response: { status: 403 } }); await wrapper.findAll('button').find(b => b.text() === '重新读取')!.trigger('click'); await flushPromises(); expect(wrapper.text()).toContain('当前不可访问'); expect(wrapper.text()).not.toContain('查看候选产物'); wrapper.unmount()
  })
  it('uses the same operation key after an uncertain start and reports partial truth', async () => {
    api.sources.mockResolvedValue({ data: { items: [source] } }); api.start.mockRejectedValueOnce(new Error('network')).mockResolvedValueOnce({ data: { id: 'run' } }); api.run.mockResolvedValue({ data: { ...completed, status: 'partial', retryable: true, output_refs: [] } }); const wrapper = setup(); await flushPromises(); const button = () => wrapper.findAll('button').find(b => b.text() === '采集此版本')!; await button().trigger('click'); await flushPromises(); await button().trigger('click'); await flushPromises(); expect(api.start.mock.calls[0]![2]).toBe(api.start.mock.calls[1]![2]); expect(wrapper.text()).toContain('部分完成'); expect(wrapper.text()).toContain('水位未推进'); expect(wrapper.text()).toContain('从原快照检查点继续'); wrapper.unmount()
  })
  it('discards old tenant reads and clears a typed credential secret', async () => {
    let resolve!: (value: unknown) => void; api.sources.mockImplementationOnce(() => new Promise(r => { resolve = r })); const wrapper = setup(); await wrapper.get('input[type="password"]').setValue('synthetic-secret-only'); scope.currentTenant.id = 2; await flushPromises(); resolve({ data: { items: [source] } }); await flushPromises(); expect(wrapper.text()).not.toContain('Orders'); expect((wrapper.get('input[type="password"]').element as HTMLInputElement).value).toBe(''); wrapper.unmount()
  })
  it('can edit a reactive source and save a new fixed version without automatic execution', async () => {
    api.sources.mockResolvedValue({ data: { items: [source] } }); api.update.mockResolvedValue({ data: { ...source, version: 2 } }); const wrapper = setup(); await flushPromises(); await wrapper.findAll('button').find(b => b.text() === '配置')!.trigger('click'); await wrapper.get('section[aria-label="配置 API 来源"] form').trigger('submit'); await flushPromises(); expect(api.update).toHaveBeenCalledWith('source', expect.objectContaining({ expected_version: 1 }), expect.any(AbortSignal)); expect(api.start).not.toHaveBeenCalled(); wrapper.unmount()
  })
})
