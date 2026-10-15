import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { reactive } from 'vue'
const mocks = vi.hoisted(() => ({ list: vi.fn(), stream: vi.fn(), auth: {} as { user?: { id: number }; currentTenant?: { id: number } } }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => mocks.auth }))
vi.mock('@/api/knowledge', () => ({ knowledgeApi: { getList: mocks.list } }))
vi.mock('@/api/research', () => ({ streamResearch: mocks.stream }))
import DeepResearchView from './DeepResearchView.vue'
const report = { query: 'q', success: true, executive_summary: 'Summary [S1]', detailed_analysis: 'Evidence-backed analysis [S1]', key_findings: ['Finding [S1]'], sub_questions: ['Subquestion'], sources: [{ id: 'S1', title: 'Source', type: 'rag', excerpt: 'Original verified source content', knowledge_base_id: 'allowed', chunk_id: 'chunk' }], metadata: { partial: false, citation_checked: true, fact_checked: false, verification_limit: 'Semantic claims remain unverified' }, citation_checks: [{ statement: 'Finding', source_id: 'S1', quote: 'Original verified source', quote_matched: true }] }
const setup = () => mount(DeepResearchView, { global: { stubs: { RouterLink: true, ElSelect: { template: '<select><slot /></select>' }, ElOption: true } } })
describe('research page', () => {
  beforeEach(() => { vi.resetAllMocks(); mocks.auth = reactive({ user: { id: 9 }, currentTenant: { id: 7 } }); mocks.list.mockResolvedValue({ data: [{ id: 'allowed', name: 'Allowed', status: 1 }] }) })
  it('renders the actual report, source excerpt and verification limitations', async () => {
    mocks.stream.mockImplementation(async function* () { yield { stage: 'complete', detail: 'Done', report } })
    const wrapper = setup(); await flushPromises(); await wrapper.get('textarea').setValue('Research this'); await wrapper.get('.start-button').trigger('click'); await flushPromises()
    expect(mocks.stream.mock.calls[0][0]).toMatchObject({ query: 'Research this', kb_scope_mode: 'manual', options: { enable_web_search: false } })
    expect(wrapper.text()).toContain('Original verified source content'); expect(wrapper.text()).toContain('Semantic claims remain unverified'); expect(wrapper.text()).toContain('引用与摘录已核对'); wrapper.unmount()
  })
  it('aborts a pending request and discards its late report on identity change', async () => {
    let finish!: () => void
    mocks.stream.mockImplementation(async function* () { await new Promise<void>(resolve => { finish = resolve }); yield { stage: 'complete', report } })
    const wrapper = setup(); await flushPromises(); await wrapper.get('textarea').setValue('q'); await wrapper.get('.start-button').trigger('click'); await flushPromises()
    const signal = mocks.stream.mock.calls[0][1] as AbortSignal
    mocks.auth.currentTenant = { id: 8 }; await flushPromises(); expect(signal.aborted).toBe(true); finish(); await flushPromises()
    expect(wrapper.text()).not.toContain('Original verified source content'); expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe(''); wrapper.unmount()
  })
  it('shows no-evidence failure without presenting a completed report', async () => {
    mocks.stream.mockImplementation(async function* () { yield { stage: 'error', error: 'No authorized evidence', detail: 'Failed' } })
    const wrapper = setup(); await flushPromises(); await wrapper.get('textarea').setValue('q'); await wrapper.get('.start-button').trigger('click'); await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('No authorized evidence'); expect(wrapper.text()).not.toContain('报告已完成'); wrapper.unmount()
  })
  it('cancels explicitly and restores the start action', async () => {
    let finish!: () => void
    mocks.stream.mockImplementation(async function* () { await new Promise<void>(resolve => { finish = resolve }); yield { stage: 'complete', report } })
    const wrapper = setup(); await flushPromises(); await wrapper.get('textarea').setValue('q'); await wrapper.get('.start-button').trigger('click'); await flushPromises()
    const signal = mocks.stream.mock.calls[0][1] as AbortSignal
    await wrapper.get('.cancel-button').trigger('click'); expect(signal.aborted).toBe(true); finish(); await flushPromises()
    expect(wrapper.text()).toContain('研究已取消'); expect(wrapper.find('.start-button').exists()).toBe(true); expect(wrapper.find('.report').exists()).toBe(false); wrapper.unmount()
  })
})
