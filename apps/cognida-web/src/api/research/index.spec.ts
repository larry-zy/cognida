import { beforeEach, describe, expect, it, vi } from 'vitest'
const mocks = vi.hoisted(() => ({ readSSE: vi.fn() }))
vi.mock('@/utils/sse', () => mocks)
const transport = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }))
vi.mock('@/utils/request', () => ({ http: transport }))
import { researchApi, streamResearch, type ResearchRequest } from './index'
const request: ResearchRequest = { query: 'q', kb_ids: ['allowed'], kb_scope_mode: 'manual', graph_enabled: true, options: { enable_web_search: false, max_sub_questions: 3, timeout_seconds: 120 } }
async function collect() { const values = []; for await (const value of streamResearch(request, new AbortController().signal)) values.push(value); return values }
describe('research stream contract', () => {
  beforeEach(() => vi.resetAllMocks())
  it('returns the terminal report through the authenticated SSE helper', async () => {
    mocks.readSSE.mockImplementation(async function* () { yield { event: 'complete', data: JSON.stringify({ stage: 'complete', report: { success: true, sources: [{ id: 'S1' }] } }) } })
    expect((await collect())[0].report?.success).toBe(true)
    expect(mocks.readSSE).toHaveBeenCalledWith('/api/v1/agent/deep-research/stream', request, expect.any(AbortSignal))
  })
  it('rejects disconnection before a report and empty false success', async () => {
    mocks.readSSE.mockImplementation(async function* () { yield { event: 'researching', data: '{"stage":"researching"}' } })
    await expect(collect()).rejects.toThrow('尚未收到完整研究报告')
    mocks.readSSE.mockImplementation(async function* () { yield { event: 'complete', data: '{"stage":"complete","report":{"success":true,"sources":[]}}' } })
    await expect(collect()).rejects.toThrow('未收到有效报告')
  })
  it('preserves visible error terminal events without restarting the job', async () => {
    mocks.readSSE.mockImplementation(async function* () { yield { event: 'error', data: '{"stage":"error","error":"no evidence"}' } })
    expect((await collect())[0].error).toBe('no evidence')
    expect(mocks.readSSE).toHaveBeenCalledTimes(1)
  })
  it('creates once under an explicit key and only reads or cancels the durable handle', async () => {
    const signal = new AbortController().signal, body = { query: 'q', kb_ids: ['allowed'], graph_enabled: false, enable_web_search: false }
    await researchApi.create(body, 'stable-key', signal); expect(transport.post).toHaveBeenLastCalledWith('/research/runs', body, { headers: { 'Idempotency-Key': 'stable-key' }, signal })
    await researchApi.get('run/id', signal); expect(transport.get).toHaveBeenLastCalledWith('/research/runs/run%2Fid', { signal })
    await researchApi.cancel('run/id', signal); expect(transport.post).toHaveBeenLastCalledWith('/research/runs/run%2Fid/cancel', {}, { signal })
  })

})
