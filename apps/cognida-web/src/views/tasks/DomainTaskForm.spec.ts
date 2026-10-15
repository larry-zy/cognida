import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { Capability } from '@/api/taskcontrol'
const api = vi.hoisted(() => ({ list: vi.fn(), get: vi.fn(), asset: vi.fn() }))
vi.mock('@/api/governance', () => ({ governanceApi: api }))
vi.mock('@/api/lifecycle', () => ({ lifecycleApi: { asset: api.asset } }))
import DomainTaskForm from './DomainTaskForm.vue'
const capabilities: Capability[] = ['tagging', 'named_entity', 'qa', 'feature-engineering'].map(name => ({ id: `governance.${name}`, version: 1, domain: 'governance', parameter_kind: name === 'feature-engineering' ? 'feature' : 'governance', input_kinds: ['raw_asset'], output_kinds: ['result'], tools: [`governance.${name}`], permissions: ['read'], requires_approval: false, execution: { profile: name, base_calls: name === 'feature-engineering' ? 2 : 33, calls_per_input: 1, max_timeout_seconds: 120 } }))
const asset = { asset_id: 'source', version: 3, kind: 'raw_asset' as const, status: 'available', name: '样本' }
const setup = () => mount(DomainTaskForm, { props: { capabilities, assets: [asset] as never, busy: false }, global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
describe('fixed specialist plans', () => {
  beforeEach(() => { vi.clearAllMocks(); api.list.mockResolvedValue({ data: { items: [{ id: 'feature', version: 2, name: '收入' }] } }); api.get.mockResolvedValue({ data: { id: 'feature', version: 1, source_ref_id: 'source:3' } }); api.asset.mockResolvedValue({ data: { ref: asset } }) })
  it('preserves explicit label rules and budgets without dispatching', async () => {
    const wrapper = setup()
    await wrapper.get('[aria-label="领域能力"]').setValue('governance.tagging')
    await wrapper.get('[aria-label="领域处理资产"]').setValue('source:3')
    await wrapper.get('[aria-label="标签与规则"]').setValue('金融 | 收入,利润\n人事 | 员工')
    await wrapper.get('form').trigger('submit')
    const [draft, plan] = wrapper.emitted('preview')![0] as any[]
    expect(draft.input_refs).toEqual([{ asset_id: 'source', version: 3 }])
    expect(plan.nodes[0].parameters.governance.label_schema[0]).toEqual({ id: 'label-1', name: '金融', keywords: ['收入', '利润'] })
    expect(plan.budget).toMatchObject({ max_calls: 34, max_attempts: 1 })
    expect(api.get).not.toHaveBeenCalled(); wrapper.unmount()
  })
  it('rejects entity labels without a dictionary and QA duplicate columns', async () => {
    const wrapper = setup()
    await wrapper.get('[aria-label="领域能力"]').setValue('governance.named_entity')
    await wrapper.get('[aria-label="领域处理资产"]').setValue('source:3')
    await wrapper.get('[aria-label="标签与规则"]').setValue('公司')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.text()).toContain('明确字典词'); expect(wrapper.emitted('preview')).toBeUndefined()
    await wrapper.get('[aria-label="领域能力"]').setValue('governance.qa')
    await wrapper.get('[aria-label="领域处理资产"]').setValue('source:3')
    await wrapper.get('[aria-label="参考答案列"]').setValue('question')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.text()).toContain('非空且不同'); expect(wrapper.emitted('preview')).toBeUndefined(); wrapper.unmount()
  })
  it('resolves the selected historical definition and uses its real source', async () => {
    const wrapper = setup()
    await wrapper.get('[aria-label="领域能力"]').setValue('governance.feature-engineering'); await flushPromises()
    await wrapper.get('[aria-label="专家特征定义"]').setValue('feature')
    await wrapper.get('[aria-label="专家特征版本"]').setValue(1)
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(api.get).toHaveBeenCalledWith('/features/feature/versions/1'); expect(api.asset).toHaveBeenCalledWith('source', 3)
    const [, plan] = wrapper.emitted('preview')![0] as any[]
    expect(plan.nodes[0].parameters).toEqual({ feature: { feature_id: 'feature', version: 1 } })
    expect(plan.budget.max_calls).toBe(3); wrapper.unmount()
  })
  it('drops feature reads completed after the form is unmounted', async () => {
    let resolve!: (value: unknown) => void
    api.get.mockImplementation(() => new Promise(r => { resolve = r }))
    const wrapper = setup()
    await wrapper.get('[aria-label="领域能力"]').setValue('governance.feature-engineering'); await flushPromises()
    await wrapper.get('[aria-label="专家特征定义"]').setValue('feature')
    await wrapper.get('form').trigger('submit'); wrapper.unmount()
    resolve({ data: { source_ref_id: 'source:3' } }); await flushPromises()
    expect(wrapper.emitted('preview')).toBeUndefined()
    expect(api.asset).not.toHaveBeenCalled()
  })
})
