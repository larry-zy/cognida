import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const { getList } = vi.hoisted(() => ({ getList: vi.fn() }))
vi.mock('@/api/knowledge', () => ({ knowledgeApi: { getList } }))
vi.mock('@/utils/request', () => ({ http: {}, resetAuthFailure: vi.fn() }))
vi.mock('@/utils/security', () => ({ storage: { get: () => null, set: vi.fn(), remove: vi.fn() } }))
import { useKnowledgeStore } from './knowledge'
import { useAuthStore } from './auth'

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(r => { resolve = r })
  return { promise, resolve }
}

describe('knowledge cache identity isolation', () => {
  beforeEach(() => { setActivePinia(createPinia()); getList.mockReset() })

  it('clears cached data immediately on logout and fetches for the next tenant', async () => {
    const auth = useAuthStore()
    auth.token = 'token'
    auth.currentTenant = { id: 1, name: 'A', created_at: '', updated_at: '' }
    const store = useKnowledgeStore()
    getList.mockResolvedValueOnce({ data: [{ id: 'private-A' }] }).mockResolvedValueOnce({ data: [{ id: 'private-B' }] })
    await store.loadKnowledgeBases()
    await store.loadKnowledgeBases()
    expect(getList).toHaveBeenCalledTimes(1)
    auth.clearAuth()
    expect(store.knowledgeBases).toEqual([])
    auth.token = 'new-token'
    auth.currentTenant = { id: 2, name: 'B', created_at: '', updated_at: '' }
    await store.loadKnowledgeBases()
    expect(store.knowledgeBases.map(kb => kb.id)).toEqual(['private-B'])
  })

  it('ignores old tenant responses and keeps the new request in flight', async () => {
    const auth = useAuthStore()
    auth.currentTenant = { id: 1, name: 'A', created_at: '', updated_at: '' }
    const store = useKnowledgeStore()
    const old = deferred<{ data: { id: string }[] }>()
    const current = deferred<{ data: { id: string }[] }>()
    getList.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    const oldLoad = store.loadKnowledgeBases()
    auth.currentTenant = { id: 2, name: 'B', created_at: '', updated_at: '' }
    const newLoad = store.loadKnowledgeBases()
    old.resolve({ data: [{ id: 'private-A' }] })
    await oldLoad
    expect(store.knowledgeBases).toEqual([])
    expect(store.loading).toBe(true)
    const duplicateLoad = store.loadKnowledgeBases()
    expect(getList).toHaveBeenCalledTimes(2)
    current.resolve({ data: [{ id: 'private-B' }] })
    await Promise.all([newLoad, duplicateLoad])
    expect(store.knowledgeBases.map(kb => kb.id)).toEqual(['private-B'])
  })

  it('does not let an older refresh overwrite a newer successful response', async () => {
    const store = useKnowledgeStore()
    const old = deferred<{ data: { id: string }[] }>()
    getList.mockReturnValueOnce(old.promise).mockResolvedValueOnce({ data: [{ id: 'latest' }] })
    const oldLoad = store.loadKnowledgeBases()
    await store.refresh()
    old.resolve({ data: [{ id: 'outdated' }] })
    await oldLoad
    expect(store.knowledgeBases.map(kb => kb.id)).toEqual(['latest'])
  })

  it('keeps a same-identity profile refresh from discarding the in-flight list', async () => {
    const auth = useAuthStore()
    auth.token = 'token'
    auth.user = { id: 10, username: 'owner' } as NonNullable<typeof auth.user>
    auth.currentTenant = { id: 1, name: 'A', created_at: '', updated_at: '' }
    const store = useKnowledgeStore()
    const response = deferred<{ data: { items: { id: string }[] } }>()
    getList.mockReturnValueOnce(response.promise)
    const pending = store.loadKnowledgeBases()
    auth.user = { ...auth.user, username: 'refreshed owner' }
    auth.currentTenant = { ...auth.currentTenant, name: 'refreshed tenant' }
    expect(store.loading).toBe(true)
    const duplicate = store.loadKnowledgeBases()
    expect(getList).toHaveBeenCalledTimes(1)
    response.resolve({ data: { items: [{ id: 'current-knowledge-base' }] } })
    await Promise.all([pending, duplicate])
    expect(store.knowledgeBases.map(kb => kb.id)).toEqual(['current-knowledge-base'])
    expect(store.loading).toBe(false)
    auth.user = { ...auth.user }
    auth.currentTenant = { ...auth.currentTenant }
    await store.loadKnowledgeBases()
    expect(store.knowledgeBases.map(kb => kb.id)).toEqual(['current-knowledge-base'])
    expect(getList).toHaveBeenCalledTimes(1)
  })
})
