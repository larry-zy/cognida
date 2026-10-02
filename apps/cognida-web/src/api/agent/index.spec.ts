import { beforeEach, describe, expect, it, vi } from 'vitest'

const { readSSE } = vi.hoisted(() => ({ readSSE: vi.fn() }))
vi.mock('@/utils/sse', () => ({ readSSE }))
vi.mock('@/utils/request', () => ({ http: {} }))
import { streamDataChat, streamKnowledgeChat } from './index'

describe('agent stream request contracts', () => {
  beforeEach(() => {
    readSSE.mockReset().mockImplementation(async function* () {
      yield { event: 'done', data: '{"answer":"ok"}' }
    })
  })

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
})
