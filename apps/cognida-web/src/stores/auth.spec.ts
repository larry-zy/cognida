import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { storage } from '@/utils/security'

const { post, saved } = vi.hoisted(() => ({ post: vi.fn(), saved: new Map<string, unknown>() }))
vi.mock('@/utils/request', () => ({ http: { post }, resetAuthFailure: vi.fn() }))
vi.mock('@/utils/security', () => ({ storage: {
  get: (key: string) => saved.get(key) ?? null,
  set: (key: string, value: unknown) => saved.set(key, value),
  remove: (key: string) => saved.delete(key)
} }))
import { useAuthStore } from './auth'

describe('authentication tenant state', () => {
  beforeEach(() => { setActivePinia(createPinia()); saved.clear(); post.mockReset() })

  it('clears the cached tenant list when the user signs out', () => {
    const auth = useAuthStore()
    auth.tenants = [{ id: 1, name: 'old tenant', created_at: '', updated_at: '' }]
    storage.set('tenants', auth.tenants)
    auth.clearAuth()
    expect(auth.tenants).toEqual([])
    expect(storage.get('tenants')).toBeNull()
  })

  it('does not retain the previous users tenant on a tenantless login', async () => {
    const auth = useAuthStore()
    auth.currentTenant = { id: 1, name: 'old tenant', created_at: '', updated_at: '' }
    post.mockResolvedValue({ data: { access_token: 'new-token', refresh_token: 'new-refresh', user: { id: 'new' } } })
    await auth.login({ email: 'new@example.com', password: 'test' })
    expect(auth.currentTenant).toBeNull()
    expect(storage.get('current_tenant')).toBeNull()
    expect(auth.token).toBe('new-token')
  })
  it('changes tenant only after the server issues a matching identity', async () => {
    const auth=useAuthStore();auth.token='original';auth.refreshToken='old-refresh';auth.currentTenant={id:1,name:'first',created_at:'',updated_at:''}
    const next={id:2,name:'second',created_at:'',updated_at:''}
    post.mockRejectedValueOnce(new Error('denied'))
    await expect(auth.setCurrentTenant(next)).rejects.toThrow('denied')
    expect(auth.currentTenant.id).toBe(1);expect(auth.token).toBe('original')
    post.mockResolvedValueOnce({data:{tenant_id:2,access_token:'tenant-two',refresh_token:'refresh-two',user:{id:7,tenant_id:2}}})
    await auth.setCurrentTenant(next)
    expect(auth.currentTenant.id).toBe(2);expect(auth.token).toBe('tenant-two');expect(storage.get('current_tenant')).toEqual(next)
  })
  it('ignores a refresh response that predates a successful tenant switch', async()=>{
    const auth=useAuthStore();auth.token='old';auth.refreshToken='original-refresh'
    let finish!:(value:unknown)=>void
    post.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve}))
    const pending=auth.refreshAccessToken()
    auth.token='switched';auth.refreshToken='switched-refresh'
    finish({data:{access_token:'stale',refresh_token:'stale-refresh'}})
    expect(await pending).toBe(false);expect(auth.token).toBe('switched')
  })

})
