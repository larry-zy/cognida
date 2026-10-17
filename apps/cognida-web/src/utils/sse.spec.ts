import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
const state = vi.hoisted(() => ({
  auth: { authEpoch: 1, user: { id: 1 }, currentTenant: { id: 1 }, token: 'old', refreshAccessToken: vi.fn() },
  failure: vi.fn(),
}))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => state.auth }))
vi.mock('@/utils/request', () => ({ handleAuthFailure: state.failure }))
vi.mock('@/utils/security', () => ({ storage: { get: (key: string) => key === 'token' ? state.auth.token : state.auth.currentTenant } }))
import { readSSE } from './sse'
function deferred<T>() { let resolve!: (value: T) => void; return { promise: new Promise<T>(r => { resolve = r }), resolve: (value: T) => resolve(value) } }
async function consume(signal?: AbortSignal) { const result = []; for await (const message of readSSE('/stream', { query: 'fixture' }, signal)) result.push(message); return result }
const ok = () => new Response('event: done\ndata: {"answer":"ok"}\n\n', { status: 200 })
describe('SSE authentication scope', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    Object.assign(state.auth, { authEpoch: 1, user: { id: 1 }, currentTenant: { id: 1 }, token: 'old' })
    vi.stubGlobal('fetch', vi.fn())
  })
  afterEach(() => vi.unstubAllGlobals())
  it.each(['identity', 'tenant', 'abort'] as const)('does not replay or log out after %s changes during refresh', async change => {
    const refresh = deferred<boolean>()
    vi.mocked(fetch).mockResolvedValue(new Response(null, { status: 401 }))
    state.auth.refreshAccessToken.mockReturnValue(refresh.promise)
    const controller = new AbortController()
    const result = consume(controller.signal)
    const assertion = expect(result).rejects.toMatchObject({ name: 'AbortError' })
    await flushPromises()
    if (change === 'identity') { state.auth.authEpoch++; state.auth.user.id = 2; state.auth.token = 'new-login' }
    if (change === 'tenant') state.auth.currentTenant.id = 2
    if (change === 'abort') controller.abort()
    refresh.resolve(change === 'tenant')
    await assertion
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(state.failure).not.toHaveBeenCalled()
  })
  it('ignores an old 401 arriving after a login without even starting refresh', async () => {
    const response = deferred<Response>(); vi.mocked(fetch).mockReturnValue(response.promise)
    const result = consume(); const assertion = expect(result).rejects.toMatchObject({ name: 'AbortError' })
    state.auth.authEpoch++; state.auth.token = 'new-login'
    response.resolve(new Response(null, { status: 401 })); await assertion
    expect(state.auth.refreshAccessToken).not.toHaveBeenCalled(); expect(state.failure).not.toHaveBeenCalled()
  })
  it('discards buffered events when identity changes after the stream opens', async () => {
    vi.mocked(fetch).mockResolvedValue(new Response('data: first\n\ndata: private-second\n\n'))
    const stream = readSSE('/stream')
    expect((await stream.next()).value).toEqual({ event: '', data: 'first' })
    state.auth.authEpoch++
    await expect(stream.next()).rejects.toMatchObject({ name: 'AbortError' })
  })
  it('refreshes and replays once in the same scope with current headers and signal', async () => {
    const controller = new AbortController()
    vi.mocked(fetch).mockResolvedValueOnce(new Response(null, { status: 401 })).mockResolvedValueOnce(ok())
    state.auth.refreshAccessToken.mockImplementation(async () => { state.auth.token = 'refreshed'; return true })
    expect(await consume(controller.signal)).toEqual([{ event: 'done', data: '{"answer":"ok"}' }])
    expect(fetch).toHaveBeenCalledTimes(2)
    expect(vi.mocked(fetch).mock.calls[1][1]).toMatchObject({ signal: controller.signal, headers: { Authorization: 'Bearer refreshed', 'X-Tenant-ID': '1' } })
    expect(state.failure).not.toHaveBeenCalled()
  })
  it('still expires the current login when refresh is rejected', async () => {
    vi.mocked(fetch).mockResolvedValue(new Response(null, { status: 401 }))
    state.auth.refreshAccessToken.mockResolvedValue(false)
    await expect(consume()).rejects.toMatchObject({ status: 401 })
    expect(state.failure).toHaveBeenCalledTimes(1)
  })
})
