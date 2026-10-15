import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, RouterLinkStub, type VueWrapper } from '@vue/test-utils'
const api = vi.hoisted(() => ({ getResult: vi.fn(), getQAResults: vi.fn() }))
vi.mock('@/api/evaluation', () => ({ evaluationApi: api }))
vi.mock('vue-router', () => ({ useRoute: () => ({ params: { id: 'evaluation-1' }, query: {} }), useRouter: () => ({ push: vi.fn() }) }))
vi.mock('@/utils/sse', () => ({ connectTaskProgress: vi.fn(() => ({ disconnect: vi.fn() })) }))
import EvaluationDetail from './EvaluationDetail.vue'
let wrapper: VueWrapper | undefined
afterEach(() => wrapper?.unmount())
describe('evaluation result import', () => {
  it.each(['completed', 'failed'])('offers an explicit import only for a completed task: %s', async (status) => {
    api.getResult.mockResolvedValue({ data: { task: { task_id: 'evaluation-1', status, total_count: 3, success_count: 3 }, dataset_name: 'QA set' } })
    api.getQAResults.mockResolvedValue({ data: { results: [], total: 0 } })
    wrapper = mount(EvaluationDetail, { global: { stubs: { RouterLink: RouterLinkStub, 'el-icon': true }, directives: { loading: {} } } })
    await flushPromises()
    const links = wrapper.findAllComponents(RouterLinkStub).filter(link => link.text() === '转入流水线')
    expect(links).toHaveLength(status === 'completed' ? 1 : 0)
    if (status === 'completed') expect(links[0].props('to')).toEqual({ path: '/pipelines', query: { source: 'evaluation', run_id: 'evaluation-1', name: 'QA set评测结果' } })
  })
})
