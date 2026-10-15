import { beforeEach, describe, expect, it, vi } from 'vitest'
const get = vi.hoisted(() => vi.fn())
vi.mock('@/utils/request', () => ({ http: { get } }))
import { overviewApi } from './index'

describe('overview read adapters', () => {
  beforeEach(() => get.mockReset())
  it('reads authorized existing lists with explicit page bounds and cancellation', async () => {
    const signal = new AbortController().signal
    get.mockResolvedValue({ data: { items: [], next_cursor: 'next', has_more: true } })
    expect(await overviewApi.runs(signal)).toEqual({ items: [], hasMore: true })
    expect(get).toHaveBeenLastCalledWith('/lifecycle/runs', { params: { page_size: 50 }, signal })
    await overviewApi.invocations(signal)
    expect(get).toHaveBeenLastCalledWith('/lifecycle/invocations', { params: { page_size: 50 }, signal })
    await overviewApi.assets(signal)
    expect(get).toHaveBeenLastCalledWith('/lifecycle/assets', { params: { kind: 'published_asset', page_size: 20 }, signal })
    get.mockResolvedValue({ data: { tasks: [] } })
    expect(await overviewApi.collection(signal)).toEqual({ items: [], hasMore: false })
    expect(get).toHaveBeenLastCalledWith('/collection-tasks', { signal })
  })
  it('does not turn permission or malformed responses into empty counts', async () => {
    const denied = { response: { status: 403 } }
    get.mockRejectedValue(denied)
    await expect(overviewApi.runs()).rejects.toBe(denied)
    get.mockResolvedValue({})
    await expect(overviewApi.runs()).rejects.toThrow('有效列表')
    await expect(overviewApi.collection()).rejects.toThrow('有效采集列表')
  })
})
