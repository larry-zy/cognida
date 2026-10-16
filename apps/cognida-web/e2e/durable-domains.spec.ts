import { randomUUID } from 'node:crypto'
import { test, expect, type APIRequestContext, type Page } from '@playwright/test'
import { loginThroughUI, createInput, getData, attachJSON, selectPagedAsset } from './lifecycle.helpers'
async function action(page: Page, path: string, callback: () => Promise<unknown>) {
  const waiting = page.waitForResponse(response => new URL(response.url()).pathname === `/api/v1${path}` && response.request().method() === 'POST')
  await callback(); const response = await waiting; expect(response.ok(), `${path}: ${response.status()} ${await response.text()}`).toBe(true); return (await response.json()).data
}
async function executePreview(page: Page, api: APIRequestContext) {
  const draft = await action(page, '/governed-tasks', () => page.getByRole('button', { name: '预览领域计划', exact: true }).click())
  await action(page, `/governed-tasks/${draft.id}/plans`, () => page.getByRole('button', { name: '保存计划', exact: true }).click())
  await action(page, `/governed-tasks/${draft.id}/start`, () => page.getByRole('button', { name: '启动任务', exact: true }).click())
  let summary: any
  await expect.poll(async () => { const response = await api.get(`/api/v1/governed-tasks/${draft.id}/supervisor/summary`); if (response.status() === 409) return false; expect(response.ok()).toBe(true); summary = (await response.json()).data; return summary.terminal }, { timeout: 150_000 }).toBe(true)
  expect(summary.status, JSON.stringify(summary)).toBe('completed')
  const refresh = page.getByRole('button', { name: '重新核对执行状态', exact: true })
  await refresh.click()
  // Navigation is guarded while the explicit status read is in flight.
  await expect(refresh).toBeEnabled()
  return summary
}
async function domain(page: Page, id: string) { await page.goto('/tasks'); await page.locator('summary').filter({ hasText: '使用领域专家' }).click(); await page.getByLabel('领域能力', { exact: true }).selectOption(id) }
test.describe('durable collection, offline scoring and evidence research', () => {
  test.beforeEach(() => test.skip(process.env.COGNIDA_E2E_DURABLE_DOMAINS !== '1', 'Requires isolated durable domains environment'))
  test('actual HTTPS collection creates a candidate, never a published document', async ({ page, playwright }, info) => {
    const api = await loginThroughUI(page, playwright)
    try {
      await domain(page, 'collection.snapshot')
      await page.getByLabel('采集网页', { exact: true }).fill(process.env.COGNIDA_E2E_COLLECTION_URL || `https://example.com/?cognida=${randomUUID()}`)
      const summary = await executePreview(page, api)
      expect(summary.nodes[0].domain_kind).toBe('collection'); expect(summary.result_refs).toHaveLength(2)
      const run = await getData<any>(api, `/collection-runs/${summary.nodes[0].domain_id}`)
      expect(run.items).toHaveLength(1); expect(run.items[0].knowledge_id || '').toBe(''); expect(run.items[0].asset_ref_id).toBeTruthy()
      const artifact = await getData<any>(api, `/lifecycle/assets/${summary.result_refs[0].asset_id}?version=${summary.result_refs[0].version}`)
      expect(artifact.ref.producer_id).toContain('collection-governed:')
      await page.locator('.runtime-nodes').getByRole('link', { name: '查看领域任务 ↗' }).click(); await expect(page).toHaveURL(/collection-tasks\?run_id=/); await expect(page.getByRole('dialog')).toContainText('转入流水线'); await expect(page.getByRole('dialog')).not.toContainText('已入库')
      await attachJSON(info, 'real-candidate-collection', { summary, run }); await page.screenshot({ path: info.outputPath('collection-candidate.png'), fullPage: true })
    } finally { await api.dispose() }
  })
  test('existing answers are snapshotted and scored by real deterministic Python graders', async ({ page, playwright }, info) => {
    const api = await loginThroughUI(page, playwright)
    try {
      const ref = await createInput(page, { id: 'offline-answer-input', source: 'table', columns: ['question', 'reference_answer', 'generated_answer'], rows: [['What is the code?', '00123', '00123'], ['What is the city?', '北京', '上海']] }, `offline-${Date.now()}`)
      await domain(page, 'evaluation.offline-text-v1'); await selectPagedAsset(page, page.getByLabel('领域处理资产', { exact: true }), `${ref.asset_id}:${ref.version}`)
      await expect(page.getByRole('form', { name: '领域专家任务' })).toContainText('不会调用模型生成答案')
      const summary = await executePreview(page, api); expect(summary.nodes[0].domain_kind).toBe('evaluation'); expect(summary.result_refs).toHaveLength(1)
      const output = summary.result_refs[0], artifact = await getData<any>(api, `/lifecycle/assets/${output.asset_id}?version=${output.version}`)
      expect(artifact.content.mode).toBe('offline_existing_answers'); expect(artifact.content.model_calls).toBe(0); expect(artifact.content.scored_count).toBe(2); expect(artifact.content.scores.map((score: any) => score.exact_match)).toEqual([100, 0]); expect(artifact.content.items[0].generated_answer).toBe('00123')
      await page.locator('.runtime-nodes').getByRole('link', { name: '查看领域任务 ↗' }).click(); await expect(page).toHaveURL(/\/evaluation\/eval-/); await page.reload(); await expect(page.locator('main, .evaluation-detail').first()).toContainText(/完成|completed/)
      await attachJSON(info, 'real-offline-score', { summary, artifact }); await page.screenshot({ path: info.outputPath('offline-score.png'), fullPage: true })
    } finally { await api.dispose() }
  })
  test('real research persists reviewed claims and survives refresh without another generation', async ({ page, playwright }, info) => {
    test.skip(!process.env.COGNIDA_E2E_RESEARCH_KB_ID, 'Needs an authorized indexed evidence corpus and configured real LLM')
    const api = await loginThroughUI(page, playwright)
    try {
      const kb = await getData<any>(api, `/knowledge-bases/${process.env.COGNIDA_E2E_RESEARCH_KB_ID}`)
      // The default corpus is the explicitly published North/South/West fixture.
      // A topic-free request cannot be assumed to retrieve that corpus at the
      // unchanged similarity threshold; other corpora supply their own query.
      await page.goto('/research'); await page.getByLabel('你想研究什么？').fill(process.env.COGNIDA_E2E_RESEARCH_QUERY || 'In the Cognida isolated acceptance report, what revenue did North, South, and West record in January, February, and March? Cite the original statements and distinguish sample observations from claims about causation or future performance.')
      await page.locator('.el-select').click(); await page.getByRole('option', { name: kb.name, exact: true }).click(); await page.getByRole('heading', { name: '从问题走向证据' }).click()
      const created = await action(page, '/research/runs', () => page.getByRole('button', { name: /开始研究/ }).click()); let run: any
      await expect.poll(async () => { run = await getData<any>(api, `/research/runs/${created.id}`); return ['succeeded', 'partial', 'failed', 'timed_out', 'cancelled'].includes(run.status) }, { timeout: 150_000 }).toBe(true)
      await attachJSON(info, 'real-research-evidence-review', run)
      expect(['succeeded', 'partial'], JSON.stringify({ status: run.status, error: run.error })).toContain(run.status); expect(run.report.claims.length).toBeGreaterThan(0); expect(run.report.metadata.semantic_review).toBe(true); expect(run.report.metadata.fact_checked).toBe(false); expect(run.report.metadata.model_calls_used).toBeLessThanOrEqual(4)
      expect(run.report.sources.length).toBeGreaterThan(0)
      expect(run.report.metadata.citation_checked).toBe(true)
      const sources = new Map<string, { excerpt: string }>(run.report.sources.map((source: { id: string; knowledge_base_id: string; excerpt: string }) => {
        expect(source.knowledge_base_id).toBe(kb.id)
        expect(source.excerpt.trim()).not.toBe('')
        return [source.id, source]
      }))
      let matchedEvidence = 0
      for (const claim of run.report.claims) {
        for (const evidence of claim.evidence || []) {
          const source = sources.get(evidence.source_id)
          expect(source, 'Every reviewed citation must identify an actual authorized source').toBeDefined()
          expect(evidence.quote.length).toBeGreaterThanOrEqual(8)
          expect(source!.excerpt).toContain(evidence.quote)
          expect(evidence.quote_matched).toBe(true)
          matchedEvidence++
        }
      }
      expect(matchedEvidence).toBeGreaterThan(0)
      await page.reload()
      const report = page.locator('article.report')
      const claims = report.getByRole('region', { name: '主张证据审查' })
      await expect(claims).toBeVisible()
      await expect(claims).toContainText(run.report.claims[0].statement)
      const restored = await getData<any>(api, `/research/runs/${created.id}`); expect(restored.version).toBe(run.version); expect(restored.report).toEqual(run.report)
      // Playwright visibility permits opacity:0. On reload the real report can
      // arrive before the outer route fade ends, exposing the wallpaper beneath.
      // Wait for the actual transition; do not disable animations or alter CSS.
      await expect(page.locator('body')).not.toHaveClass(/page-transitioning/)
      await expect(page.locator('.platform-container')).toHaveCSS('opacity', '1')
      // The platform scrolls inside .content-wrapper: a screenshot of the entire
      // long article is clipped by that ancestor. Capture the actual viewport at
      // each section, retaining the app theme and its real scrolling behavior.
      await report.getByRole('heading', { name: '研究摘要', exact: true }).evaluate(element => element.scrollIntoView({ block: 'start' }))
      await expect(report.locator('.report-copy').first()).toBeInViewport()
      await page.screenshot({ path: info.outputPath('research-summary.png') })
      await claims.getByRole('heading', { name: '主张与证据', exact: true }).evaluate(element => element.scrollIntoView({ block: 'start' }))
      await expect(claims.locator('.source-card').first()).toBeInViewport()
      await page.screenshot({ path: info.outputPath('research-claims.png') })
    } finally { await api.dispose() }
  })
})
