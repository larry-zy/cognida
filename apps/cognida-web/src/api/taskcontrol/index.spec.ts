import { beforeEach, describe, expect, it, vi } from 'vitest'
const transport = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }))
vi.mock('@/utils/request', () => ({ http: transport }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ user: { id: 1 }, currentTenant: { id: 1 } }) }))
import { taskControlApi, type DraftInput, type PlanSpec } from './index'

const draft: DraftInput = { goal: '清洗快照', input_refs: [{ asset_id: 'asset-1', version: 2 }], budget: { max_nodes: 2, max_concurrency: 1, max_attempts: 2, max_calls: 4, max_cost_micros: 0, timeout_seconds: 60 } }
describe('task control HTTP adapter', () => {
  beforeEach(() => { vi.clearAllMocks(); transport.get.mockResolvedValue({ data: {} }); transport.post.mockResolvedValue({ data: {} }) })
  it('reuses a draft creation key after an uncertain response and includes cancellation scope', async () => {
    const signal = new AbortController().signal
    transport.post.mockRejectedValueOnce(new Error('connection lost'))
    await expect(taskControlApi.create(draft, signal)).rejects.toThrow('connection lost')
    await taskControlApi.create(draft, signal)
    const first = transport.post.mock.calls[0], second = transport.post.mock.calls[1]
    expect(first[0]).toBe('/governed-tasks'); expect(first[2].headers['Idempotency-Key']).toBeTruthy()
    expect(first[2].headers['Idempotency-Key']).toBe(second[2].headers['Idempotency-Key']); expect(first[2].signal).toBe(signal)
  })
  it('keeps proposal and persistence separate and binds both to an exact task version', async () => {
    const signal = new AbortController().signal, answers = [{ question_id: 'q1', question: '是否保留原行？', answer: '是' }]
    await taskControlApi.assess(draft, signal)
    expect(transport.post).toHaveBeenLastCalledWith('/governed-tasks/supervisor/assess', draft, { signal })
    await taskControlApi.propose('task/id', 3, answers, signal)
    expect(transport.post).toHaveBeenLastCalledWith('/governed-tasks/task%2Fid/supervisor/propose', { expected_version: 3, answers }, { signal })
    const plan: PlanSpec = { schema_version: 1, budget: draft.budget, nodes: [] }
    await taskControlApi.appendPlan('task/id', 3, plan, signal)
    expect(transport.post).toHaveBeenLastCalledWith('/governed-tasks/task%2Fid/plans', { expected_version: 3, plan }, { signal })
    await taskControlApi.list('cursor-2', signal)
    expect(transport.get).toHaveBeenLastCalledWith('/governed-tasks', { params: { cursor: 'cursor-2', page_size: 20 }, signal })
    await taskControlApi.summary('task/id', signal)
    expect(transport.get).toHaveBeenLastCalledWith('/governed-tasks/task%2Fid/supervisor/summary', { signal })
    expect(transport.post.mock.calls.every(([path]) => !/\/(run|approve|execute|cancel)$/.test(path))).toBe(true)
  })
  it('checks readiness without mutations and binds idempotent start to both task and plan versions', async () => {
    const signal = new AbortController().signal
    await taskControlApi.readiness('task/id', signal)
    expect(transport.get).toHaveBeenLastCalledWith('/governed-tasks/task%2Fid/execution-readiness', { signal })
    expect(transport.post).not.toHaveBeenCalled()
    transport.post.mockRejectedValueOnce(new Error('unknown start outcome'))
    await expect(taskControlApi.start('task/id', 4, 2, signal)).rejects.toThrow('unknown start outcome')
    await taskControlApi.start('task/id', 4, 2, signal)
    const first = transport.post.mock.calls[0], second = transport.post.mock.calls[1]
    expect(first.slice(0, 2)).toEqual(['/governed-tasks/task%2Fid/start', { expected_version: 4, plan_version: 2 }])
    expect(first[2].headers['Idempotency-Key']).toBeTruthy()
    expect(second[2].headers['Idempotency-Key']).toBe(first[2].headers['Idempotency-Key'])
    expect(second[2].signal).toBe(signal)
    await taskControlApi.cancel('task/id', 5, signal)
    expect(transport.post).toHaveBeenLastCalledWith('/governed-tasks/task%2Fid/cancel', { expected_version: 5 }, { signal })
  })
})
