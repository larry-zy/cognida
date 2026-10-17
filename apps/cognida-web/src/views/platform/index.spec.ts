import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, reactive, ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
const auth = reactive({ authEpoch: 1, username: 'Reader', email: 'reader@example.test', user: { id: 1 }, currentTenant: { id: 1 }, logout: vi.fn() })
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/utils/confirm', () => ({ ElMessageBox: { confirm: vi.fn() } }))
vi.mock('@/utils/toast', () => ({ default: { success: vi.fn() } }))
import Platform from './index.vue'
import AppTransition from '@/components/transitions/AppTransition.vue'
async function setup(path = '/analysis/chat/session-1') {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div>Page content</div>' } }] })
  await router.push(path); await router.isReady()
  const wrapper = mount(Platform, { attachTo: document.body, global: { plugins: [router], stubs: { TenantSwitcher: { template: '<div>Workspace switcher</div>' } } } })
  await flushPromises(); return { wrapper, router }
}
describe('global workspace navigation', () => {
  beforeEach(() => {
    const values = new Map<string, string>()
    vi.stubGlobal('localStorage', { getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => values.set(key, value), clear: () => values.clear() })
  })
  afterEach(() => vi.unstubAllGlobals())
  it('retains the data conversation instance for URL sync but remounts it on identity changes', async () => {
    let mounts = 0
    const Conversation = defineComponent({
      setup() { mounts++; return { source: ref('current') } },
      template: '<input aria-label="chosen source" v-model="source" />',
    })
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: Platform, children: [
      { path: 'analysis/chat/:conversationId?', name: 'DataAgent', component: Conversation },
      { path: ':pathMatch(.*)*', component: { template: '<div />' } },
    ] }] })
    await router.push('/analysis/chat'); await router.isReady()
    const wrapper = mount(AppTransition, { global: { plugins: [router], stubs: { TenantSwitcher: true } } })
    await wrapper.get('input').setValue('external')
    await router.replace('/analysis/chat/new-session'); await flushPromises()
    expect(mounts).toBe(1)
    expect((wrapper.get('input').element as HTMLInputElement).value).toBe('external')
    auth.authEpoch++; auth.currentTenant = { id: 2 }; await flushPromises()
    expect(mounts).toBe(2)
    expect((wrapper.get('input').element as HTMLInputElement).value).toBe('current')
    wrapper.unmount(); auth.currentTenant = { id: 1 }
  })
  it('labels the active destination and keeps grouped links accessible when collapsed', async () => {
    localStorage.setItem('link:rail-collapsed', '1')
    const { wrapper } = await setup()
    const current = wrapper.findAll('nav[aria-label="主导航"] a[aria-current="page"]')
    expect(current).toHaveLength(1); expect(current[0].attributes('href')).toBe('/analysis/chat')
    expect(current[0].attributes('aria-label')).toBe('数据对话')
    expect(wrapper.findAll('nav section')).toHaveLength(4)
    expect(wrapper.get('.rail__toggle').attributes('aria-expanded')).toBe('false')
    await wrapper.get('.rail__toggle').trigger('click')
    expect(wrapper.get('.rail__toggle').attributes('aria-expanded')).toBe('true')
    wrapper.unmount()
  })
  it('opens a mobile menu, focuses its close control, and restores focus with Escape', async () => {
    const { wrapper } = await setup('/home')
    const trigger = wrapper.get('.platform-mobile-bar button')
    await trigger.trigger('click'); await flushPromises()
    expect(trigger.attributes('aria-expanded')).toBe('true')
    expect(wrapper.get('.rail').classes()).toContain('rail--mobile-open')
    expect(wrapper.get('main').attributes('inert')).toBeDefined()
    expect(document.activeElement).toBe(wrapper.get('.rail__mobile-close').element)
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })); await flushPromises()
    expect(trigger.attributes('aria-expanded')).toBe('false')
    expect(wrapper.get('main').attributes('inert')).toBeUndefined()
    expect(document.activeElement).toBe(trigger.element)
    wrapper.unmount()
  })
  it('navigates from the drawer and closes it without changing auth scope', async () => {
    const { wrapper, router } = await setup('/home')
    await wrapper.get('.platform-mobile-bar button').trigger('click')
    await wrapper.get('nav a[href="/pipelines"]').trigger('click'); await flushPromises()
    expect(router.currentRoute.value.path).toBe('/pipelines')
    expect(wrapper.get('.platform-mobile-bar button').attributes('aria-expanded')).toBe('false')
    expect(auth.currentTenant.id).toBe(1)
    wrapper.unmount()
  })
  it('releases the background when an open mobile menu becomes a desktop rail', async () => {
    let change!: (event: { matches: boolean }) => void
    const remove = vi.fn()
    vi.stubGlobal('matchMedia', () => ({ addEventListener: (_: string, listener: typeof change) => { change = listener }, removeEventListener: remove }))
    const { wrapper } = await setup('/home')
    await wrapper.get('.platform-mobile-bar button').trigger('click')
    change({ matches: true }); await flushPromises()
    expect(wrapper.get('main').attributes('inert')).toBeUndefined()
    expect(wrapper.get('.platform-mobile-bar button').attributes('aria-expanded')).toBe('false')
    wrapper.unmount(); expect(remove).toHaveBeenCalled()
  })

})
