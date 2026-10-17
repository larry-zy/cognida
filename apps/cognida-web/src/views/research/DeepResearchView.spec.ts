import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { reactive } from 'vue'
const mocks = vi.hoisted(() => ({ list: vi.fn(), create: vi.fn(), get: vi.fn(), cancel: vi.fn(), replace: vi.fn(), route: { query: {} as Record<string, string> }, auth: {} as { authEpoch?: number; user?: { id: number }; currentTenant?: { id: number } } }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => mocks.auth }))
vi.mock('vue-router', () => ({ useRoute: () => mocks.route, useRouter: () => ({ replace: mocks.replace }) }))
vi.mock('@/api/knowledge', () => ({ knowledgeApi: { getPage: mocks.list } }))
vi.mock('@/api/research', () => ({ researchApi: { create: mocks.create, get: mocks.get, cancel: mocks.cancel } }))
import DeepResearchView from './DeepResearchView.vue'
const report = { query: 'q', success: true, executive_summary: 'Summary [S1]', detailed_analysis: 'Evidence-backed analysis [S1]', key_findings: ['Finding [S1]'], sub_questions: ['Subquestion'], sources: [{ id: 'S1', title: 'Source', type: 'rag', excerpt: 'Original verified source content', knowledge_base_id: 'allowed', chunk_id: 'chunk', independence_group: 'G1' }], metadata: { partial: false, citation_checked: true, fact_checked: false, verification_limit: 'Semantic claims remain unverified' }, claims: [{ id: 'C1', statement: 'Finding', status: 'conflict', reason: 'Sources disagree', independent_sources: 1, evidence: [{ source_id: 'S1', quote: 'Original verified source', relation: 'contradicts' }] }] }
const run = (status = 'succeeded', value: unknown = report) => ({ id: 'r1', version: 2, status, stage: 'reviewing', progress: 1, report: value, submission: { request: { query: 'q', kb_ids: ['allowed'], graph_enabled: false, enable_web_search: false } }, output_refs: [], updated_at: '' })
const setup = () => mount(DeepResearchView, { global: { stubs: { RouterLink: true, ElSelect: { props: ['modelValue'], emits: ['update:modelValue'], template: '<select aria-label="KB" @change="$emit(\'update:modelValue\', [\'allowed\'])"><option>choose</option><option value="allowed">allowed</option></select>' }, ElOption: true } } })
async function fill(wrapper: ReturnType<typeof setup>) { await flushPromises(); await wrapper.get('textarea').setValue('Research this'); await wrapper.get('select').setValue('allowed') }
describe('durable research page', () => {
  beforeEach(() => { vi.resetAllMocks(); mocks.route = reactive({ query: {} }); mocks.auth = reactive({ authEpoch: 1, user: { id: 9 }, currentTenant: { id: 7 } }); mocks.list.mockResolvedValue({ data: { items: [{ id: 'allowed', name: 'Allowed', status: 1 }], total: 1, page: 1, size: 100 } }); mocks.replace.mockResolvedValue(undefined); mocks.get.mockResolvedValue({ data: run() }); mocks.create.mockResolvedValue({ data: run() }) })
  it('preserves the question on same-identity refresh but clears it on tenant change', async () => {
    const w = setup(); await flushPromises()
    await w.get('#research-question').setValue('Unsaved research question')
    mocks.auth.user = { id: 9 }; mocks.auth.currentTenant = { id: 7 }; await flushPromises()
    expect((w.get('#research-question').element as HTMLTextAreaElement).value).toBe('Unsaved research question')
    mocks.auth.currentTenant = { id: 2 }; await flushPromises()
    expect((w.get('#research-question').element as HTMLTextAreaElement).value).toBe('')
    w.unmount()
  })
  it('creates a durable explicit scope and renders claims with limitations', async () => {
    const wrapper = setup(); await fill(wrapper); await wrapper.get('.start-button').trigger('click'); await flushPromises()
    expect(mocks.create.mock.calls[0][0]).toEqual({ query: 'Research this', kb_ids: ['allowed'], graph_enabled: false, enable_web_search: false }); expect(mocks.replace).toHaveBeenCalledWith({ query: { run_id: 'r1' } })
    expect(wrapper.text()).toContain('Original verified source content'); expect(wrapper.text()).toContain('Semantic claims remain unverified'); expect(wrapper.text()).toContain('存在冲突'); expect(wrapper.text()).toContain('来源组 G1'); wrapper.unmount()
  })
  it('reads the real paged knowledge response and exposes further authorized pages', async () => {
    mocks.list.mockResolvedValueOnce({ data: { items: [{ id: 'allowed', name: 'Allowed', status: 1 }], total: 101, page: 1, size: 100 } }).mockResolvedValueOnce({ data: { items: [{ id: 'last', name: 'Last', status: 1 }], total: 101, page: 2, size: 100 } })
    const wrapper = setup(); await flushPromises()
    expect(mocks.list).toHaveBeenCalledWith(1, 100)
    expect(wrapper.text()).not.toContain('列表加载失败')
    await wrapper.findAll('button').find(button => button.text() === '继续读取知识库')!.trigger('click'); await flushPromises()
    expect(mocks.list).toHaveBeenCalledWith(2, 100)
    expect(wrapper.text()).not.toContain('继续读取知识库')
    wrapper.unmount()
  })
  it('restores a partial persisted report after refresh without submitting', async () => {
    mocks.route.query.run_id = 'r1'; mocks.get.mockResolvedValue({ data: run('partial', { ...report, success: false }) }); const wrapper = setup(); await flushPromises()
    expect(mocks.create).not.toHaveBeenCalled(); expect(wrapper.text()).toContain('部分完成'); expect(wrapper.text()).toContain('Original verified source content'); expect(wrapper.text()).not.toContain('报告已完成'); wrapper.unmount()
  })
  it('separates a completed review snapshot from failed supplemental evidence', async () => {
    mocks.route.query.run_id = 'r1'
    const supported = { ...report.claims[0], id: 'C0', statement: 'Initially supported finding', status: 'supported', evidence: [{ source_id: 'S1', quote: 'Original verified source', relation: 'supports' }] }
    mocks.get.mockResolvedValue({ data: run('partial', {
      ...report, success: false,
      sources: [...report.sources, { ...report.sources[0], id: 'S2', title: 'New evidence', excerpt: 'New counter-evidence remains unreviewed', chunk_id: 'new-chunk', independence_group: 'G2' }],
      metadata: { ...report.metadata, partial: true, semantic_review: true, review_rounds: 1, supplemental_review_incomplete: true, reviewed_source_ids: ['S1'], unreviewed_source_ids: ['S2'], failure: { stage: 'supplement_reviewing', code: 'review_quote_mismatch' } },
      review_history: [{ round: 1, source_ids: ['S1'], source_digest: 'fixed-digest', source_groups: { S1: 'G1' }, claims: [supported, ...report.claims] }],
      claims: [{ ...supported, status: 'insufficient', reason: 'Supplement remains unreviewed' }, ...report.claims],
    }) })
    const wrapper = setup(); await flushPromises()
    const warning = wrapper.get('[aria-label="补充审查未完成"]')
    expect(warning.text()).toContain('覆盖 1 个来源：S1')
    expect(warning.text()).toContain('新增未审来源：S2')
    expect(warning.text()).toContain('review_quote_mismatch')
    expect(wrapper.get('[aria-label="主张证据审查"]').text()).toContain('证据不足')
    expect(wrapper.get('[aria-label="主张证据审查"]').text()).toContain('存在冲突')
    expect(wrapper.text()).toContain('历史审查快照')
    expect(wrapper.text()).toContain('有证据支持（历史意见）')
    expect(wrapper.text()).toContain('存在冲突（历史意见）')
    expect(wrapper.get('[aria-label="研究来源"]').text()).toContain('New counter-evidence remains unreviewed')
    expect(wrapper.text()).not.toContain('报告已完成')
    expect(mocks.create).not.toHaveBeenCalled()
    wrapper.unmount()
  })
  it('aborts reads and discards late evidence on identity change', async () => {
    mocks.route.query.run_id = 'r1'; let finish!: (x: unknown) => void; mocks.get.mockImplementationOnce(() => new Promise(resolve => { finish = resolve })); mocks.get.mockRejectedValue({ response: { status: 403 } }); const wrapper = setup(); await flushPromises()
    const signal = mocks.get.mock.calls[0][1] as AbortSignal; mocks.auth.currentTenant = { id: 8 }; await flushPromises(); expect(signal.aborted).toBe(true); finish({ data: run() }); await flushPromises()
    expect(wrapper.text()).not.toContain('Original verified source content'); expect(wrapper.text()).toContain('原证据已隐藏'); wrapper.unmount()
  })
  it.each(['repairing', 'repair_validating'])('shows the real %s stage without passing the independent review', async stage => {
    mocks.route.query.run_id = 'r1'
    const attempts = stage === 'repairing' ? 0 : 1
    mocks.get.mockResolvedValue({ data: { ...run('running', { ...report, success: false, metadata: { ...report.metadata, citation_checked: false, semantic_review: false, draft_repair: { attempts, status: 'reserved', trigger: { stage: 'validating', code: 'draft_required_fields' } } } }), stage } })
    const wrapper = setup(); await flushPromises()
    expect(wrapper.get('.stage-track .active').text()).toBe('结构修复')
    expect(wrapper.findAll('.stage-track .passed').map(item => item.text())).not.toContain('审查主张')
    expect(wrapper.get('[aria-label="草稿结构修复"]').text()).toContain('修复调用已预留')
    expect(wrapper.get('[aria-label="草稿结构修复"]').text()).toContain(`结构修复尝试 ${attempts} 次`)
    expect(wrapper.get('.report-state').text()).toBe('研究中')
    expect(mocks.create).not.toHaveBeenCalled()
    wrapper.unmount()
  })
  it('shows failed repair diagnostics without reporting success or interpreting raw markup', async () => {
    mocks.route.query.run_id = 'r1'
    mocks.get.mockResolvedValue({ data: { ...run('failed', { ...report, success: false, metadata: { ...report.metadata, citation_checked: false, semantic_review: false, draft_repair: { attempts: 1, status: 'failed', trigger: { stage: 'validating', code: '<b>draft_structure</b>' }, failure: { stage: 'repair_validating', code: 'quote_mismatch' } } } }), stage: 'repair_validating' } })
    const wrapper = setup(); await flushPromises()
    const repair = wrapper.get('[aria-label="草稿结构修复"]')
    expect(repair.text()).toContain('结构修复尝试 1 次 · 结构修复失败')
    expect(repair.text()).toContain('repair_validating · quote_mismatch')
    expect(repair.text()).toContain('<b>draft_structure</b>')
    expect(repair.find('b').exists()).toBe(false)
    expect(repair.text()).toContain('引用核对：未通过；独立语义审查：未记录通过')
    expect(wrapper.get('.report-state').text()).toContain('研究失败')
    expect(wrapper.text()).not.toContain('报告已完成')
    expect(wrapper.findAll('.stage-track .passed').map(item => item.text())).not.toContain('审查主张')
    wrapper.unmount()
  })
  it.each([false, true])('does not infer semantic success from completed repair with citation_checked=%s', async citationChecked => {
    mocks.route.query.run_id = 'r1'
    mocks.get.mockResolvedValue({ data: run('failed', { ...report, success: false, metadata: { ...report.metadata, citation_checked: citationChecked, semantic_review: false, draft_repair: { attempts: 1, status: 'completed', trigger: { stage: 'validating', code: 'draft_structure' } } } }) })
    const wrapper = setup(); await flushPromises()
    const repair = wrapper.get('[aria-label="草稿结构修复"]')
    expect(repair.text()).toContain('结构修复已完成')
    expect(repair.text()).toContain(`引用核对：${citationChecked ? '已通过' : '未通过'}`)
    expect(repair.text()).toContain('独立语义审查：未记录通过')
    expect(wrapper.text()).not.toContain('报告已完成')
    wrapper.unmount()
  })
  it.each([
    ['model_call_budget_exhausted', '模型调用预算已用尽'],
    ['call_budget_exhausted', '总调用预算已用尽'],
  ])('explains skipped supplemental work for %s without hiding insufficient evidence', async (reason, label) => {
    mocks.route.query.run_id = 'r1'
    mocks.get.mockResolvedValue({ data: run('partial', { ...report, success: false, metadata: { ...report.metadata, partial: true, semantic_review: true, review_rounds: 1, supplemental_skipped_reason: reason, draft_repair: { attempts: 1, status: 'completed', trigger: { stage: 'validating', code: 'draft_structure' } } }, claims: [{ ...report.claims[0], status: 'insufficient' }] }) })
    const wrapper = setup(); await flushPromises()
    const warning = wrapper.get('[aria-label="补充调查未执行"]')
    expect(warning.text()).toContain(label)
    expect(warning.text()).toContain('调用预算已用于结构修复及首次审查')
    expect(warning.text()).toContain('未执行补充检索或第二轮审查')
    expect(wrapper.get('[aria-label="主张证据审查"]').text()).toContain('证据不足')
    expect(wrapper.get('[aria-label="草稿结构修复"]').text()).toContain('独立语义审查：已记录通过，以实际审查来源范围为准')
    expect(wrapper.text()).not.toContain('报告已完成')
    expect(mocks.create).not.toHaveBeenCalled()
    expect(mocks.cancel).not.toHaveBeenCalled()
    wrapper.unmount()
  })
  it('requests server cancellation and preserves checkpoint evidence only after actual read', async () => {
    mocks.route.query.run_id = 'r1'; mocks.get.mockResolvedValueOnce({ data: run('running') }).mockResolvedValue({ data: run('cancelled', { ...report, success: false }) }); mocks.cancel.mockResolvedValue({ data: run('cancel_requested', undefined) }); const wrapper = setup(); await flushPromises(); await wrapper.get('.cancel-button').trigger('click'); await flushPromises()
    expect(mocks.cancel.mock.calls[0][0]).toBe('r1'); expect(wrapper.text()).toContain('已取消'); expect(wrapper.text()).toContain('Original verified source content'); expect(wrapper.text()).not.toContain('报告已完成'); wrapper.unmount()
  })
  it('keeps the idempotency key for an uncertain same-request retry', async () => {
    mocks.create.mockRejectedValue(new Error('network')); const wrapper = setup(); await fill(wrapper); await wrapper.get('.start-button').trigger('click'); await flushPromises(); await wrapper.get('.start-button').trigger('click'); await flushPromises()
    expect(mocks.create.mock.calls[0][1]).toBe(mocks.create.mock.calls[1][1]); expect(wrapper.text()).toContain('提交结果未确认'); wrapper.unmount()
  })
  it('clears stale evidence when current source access is revoked', async () => {
    mocks.route.query.run_id = 'r1'; const wrapper = setup(); await flushPromises(); mocks.get.mockRejectedValue({ response: { status: 403 } }); await wrapper.get('.run-receipt button').trigger('click'); await flushPromises(); expect(wrapper.text()).not.toContain('Original verified source content'); expect(wrapper.text()).toContain('原证据已隐藏'); wrapper.unmount()
  })
})
