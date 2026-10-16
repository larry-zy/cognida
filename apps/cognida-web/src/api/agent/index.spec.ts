import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { readSSE } = vi.hoisted(() => ({ readSSE: vi.fn() }))
vi.mock('@/utils/sse', () => ({ readSSE }))
vi.mock('@/utils/request', () => ({ http: {} }))
vi.mock('@/utils/security', () => ({ storage: { get: (key: string) => key === 'token' ? 'test-token' : key === 'current_tenant' ? { id: 7 } : null } }))
import { confirmOperation, getUISurfacePage, streamDataChat, streamKnowledgeChat } from './index'

describe('agent stream request contracts', () => {
  beforeEach(() => {
    readSSE.mockReset().mockImplementation(async function* () {
      yield { event: 'done', data: '{"answer":"ok"}' }
    })
  })
  afterEach(() => { vi.unstubAllEnvs(); vi.unstubAllGlobals() })

  it('sends the selected external data source and abort signal', async () => {
    const controller = new AbortController()
    const events = []
    for await (const event of streamDataChat({ query: 'sales', session_id: 'session-1', datasource_id: 'warehouse' }, { signal: controller.signal })) events.push(event)
    expect(readSSE).toHaveBeenCalledWith(expect.stringContaining('/agent/text2sql/stream'), {
      query: 'sales', session_id: 'session-1', datasource_id: 'warehouse'
    }, controller.signal)
    expect(events).toEqual([{ event: 'done', answer: 'ok' }])
  })

  it('omits empty data source IDs so default routing still works', async () => {
    for await (const _event of streamDataChat({ query: 'sales', datasource_id: '' })) { /* drain */ }
    expect(readSSE.mock.calls[0][1]).toEqual({ query: 'sales' })
  })

  it('preserves knowledge scope and explicit graph opt-out', async () => {
    for await (const _event of streamKnowledgeChat({ query: 'policy', kb_ids: ['kb-1'], kb_scope_mode: 'hybrid', graph_enabled: false })) { /* drain */ }
    expect(readSSE.mock.calls[0][1]).toEqual({ query: 'policy', kb_ids: ['kb-1'], kb_scope_mode: 'hybrid', graph_enabled: false })
  })

  it('keeps both authenticated streams on the page proxy despite a legacy development URL', async () => {
    vi.stubEnv('DEV', true)
    vi.stubEnv('VITE_API_BASE_URL', 'http://localhost:8080/api/v1')
    for await (const _event of streamKnowledgeChat({ query: 'policy', kb_ids: ['kb-1'] })) { /* drain */ }
    for await (const _event of streamDataChat({ query: 'sales', datasource_id: 'warehouse' })) { /* drain */ }
    expect(readSSE.mock.calls.map(call => call[0])).toEqual(['/api/v1/agent/knowledge/stream', '/api/v1/agent/text2sql/stream'])
  })

  it('uses the same backend for source pages and explicit operation confirmations', async () => {
    vi.stubEnv('DEV', true)
    vi.stubEnv('VITE_API_BASE_URL', 'http://localhost:8080/api/v1')
    const fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ data: { status: 'ok' } }) })
    vi.stubGlobal('fetch', fetch)
    await getUISurfacePage('surface/one', { token: 'page-receipt', cursor: 2 })
    await confirmOperation({ pending_action_id: 'operation', token: 'confirmation-receipt', session_id: 'session' })
    expect(fetch.mock.calls.map(call => call[0])).toEqual([
      '/api/v1/agent/ui/surfaces/surface%2Fone/page?token=page-receipt&cursor=2',
      '/api/v1/agent/operations/confirm'
    ])
    expect(fetch.mock.calls[1][1].method).toBe('POST')
    for (const call of fetch.mock.calls) expect(call[1].headers).toMatchObject({ Authorization: 'Bearer test-token', 'X-Tenant-ID': '7' })
    expect(fetch).toHaveBeenCalledTimes(2)
  })
})
