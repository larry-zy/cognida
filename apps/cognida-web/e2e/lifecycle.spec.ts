import { test, expect } from '@playwright/test'
import {
  assertNoPublication, attachJSON, createInput, getData, loadScenarios, loginThroughUI, waitForRun, verifyVectorRetrieval,
  type AssetRef, type RunDetail,
} from './lifecycle.helpers'

interface AssetDocument {
  ref: AssetRef
  content: {
    kind?: 'text' | 'table'
    text?: string
    columns?: string[]
    rows?: unknown[][]
    publication?: { publication_status?: string; knowledge_id?: string; knowledge_base_id?: string; storage?: string }
  }
}
interface AnalysisDocument {
  ref: AssetRef
  content: {
    source_ref: string
    character_count?: number
    record_count?: number
    columns?: Array<{ name: string; numeric_count?: number; mean?: number; min?: number; max?: number }>
    prompt?: string
    narrative?: string
    findings?: Array<{ text: string; evidence_ids: string[] }>
    evidence?: Array<{ id: string; content: string }>
    decision?: { verification_status: string }
  }
}
const stages = ['access', 'quality', 'cleaning', 'enrichment', 'publication', 'analysis', 'evaluation']
const scenarios = loadScenarios()
for (const scenario of scenarios) {
  test(`${scenario.source} / ${scenario.id}: create → seven stages → approve → publish → report`, async ({ page, playwright }, info) => {
    const api = await loginThroughUI(page, playwright)
    let runID = ''
    let lastDetail: RunDetail | undefined
    try {
      const name = `e2e-${scenario.id}-${Date.now()}`
      const marker = scenario.source === 'text' ? `COGNIDA_ACCEPTANCE_${Date.now()}_${info.workerIndex}` : undefined
      const input = await createInput(page, marker ? { ...scenario, text: `${marker}\n${scenario.text || ''}` } : scenario, name)
      await attachJSON(info, 'created-input-ref', input)
      const inputDocument = await getData<AssetDocument>(api, `/lifecycle/assets/${encodeURIComponent(input.asset_id)}?version=${input.version}`)
      expect(['text', 'table']).toContain(inputDocument.content.kind)
      if (scenario.expected?.text_includes) expect(inputDocument.content.text).toContain(scenario.expected.text_includes)
      if (scenario.source === 'table') {
        expect(inputDocument.content.columns).toEqual(scenario.columns)
        expect(inputDocument.content.rows).toEqual(scenario.rows)
      }
      await page.getByLabel(/^流水线模板/).selectOption('web-knowledge-to-insight')
      await page.getByLabel(/^输入资产/).selectOption(`${input.asset_id}:${input.version}`)
      if (inputDocument.content.kind === 'text') {
        if (!scenario.target_knowledge_base_id || scenario.target_knowledge_base_id.startsWith('REPLACE_')) throw new Error('Text publication requires an explicit authorized target_knowledge_base_id fixture or COGNIDA_E2E_KNOWLEDGE_BASE_ID')
        const target = page.getByLabel(/^目标知识库/)
        await expect(target).toBeVisible()
        await expect(page.getByRole('button', { name: '正在加载知识库…', exact: true })).toHaveCount(0)
        // Follow the real pagination control; never inject target options into the DOM.
        while (!(await target.locator('option').evaluateAll((options, id) => options.some(option => (option as HTMLOptionElement).value === id), scenario.target_knowledge_base_id))) {
          const more = page.getByRole('button', { name: '加载更多知识库', exact: true })
          await expect(more, 'fixture knowledge base must be in the authorized pages').toBeVisible()
          await more.click()
          await expect(page.getByRole('button', { name: '正在加载知识库…', exact: true })).toHaveCount(0)
        }
        await target.selectOption(scenario.target_knowledge_base_id)
      }
      await page.locator('[data-stage="analysis"] textarea').fill(scenario.analysis_prompt || '')
      const observedRun = page.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/lifecycle/runs' && response.request().method() === 'POST')
      await page.getByRole('button', { name: '开始执行', exact: true }).click()
      const started = await observedRun
      expect(started.ok(), 'real pipeline start').toBe(true)
      runID = (await started.json()).data.id
      expect(runID).toBeTruthy()
      await attachJSON(info, 'run-reference', { run_id: runID, source: scenario.source, name })
      lastDetail = await waitForRun(api, runID, 'approval', Math.max(5000, Math.min(info.timeout - 30_000, 240_000)))
      await attachJSON(info, 'before-publication-approval', lastDetail)
      expect(lastDetail.stages.map(stage => stage.stage_key).sort()).toEqual([...stages].sort())
      expect(lastDetail.approvals.some(approval => ['pending', 'awaiting_approval'].includes(approval.status)), `expected publication approval, run status=${lastDetail.run.status}, error=${lastDetail.run.error || ''}`).toBe(true)
      await assertNoPublication(api, lastDetail)
      expect(lastDetail.stages.find(stage => stage.stage_key === 'publication')?.status).not.toBe('succeeded')
      await expect(page.getByRole('button', { name: '批准并继续', exact: true })).toBeVisible()
      await page.screenshot({ path: info.outputPath('before-approval.png'), fullPage: true })
      await page.getByLabel('审核说明', { exact: true }).fill(`Playwright isolated acceptance: ${scenario.id}`)
      await page.getByRole('button', { name: '批准并继续', exact: true }).click()
      lastDetail = await waitForRun(api, runID, 'terminal', Math.max(5000, Math.min(info.timeout - 30_000, 240_000)))
      await attachJSON(info, 'completed-run', lastDetail)
      expect(lastDetail.run.status, JSON.stringify(lastDetail.stages.map(stage => ({ stage: stage.stage_key, status: stage.status, error: stage.error })))).toBe('succeeded')
      expect(lastDetail.stages).toHaveLength(7)
      expect(lastDetail.stages.every(stage => stage.status === 'succeeded')).toBe(true)
      expect(lastDetail.approvals.every(approval => approval.status === 'approved')).toBe(true)
      const execution = page.locator('section.wb-panel').filter({ has: page.getByRole('heading', { name: '执行详情', exact: true }) })
      await expect(execution.locator('.wb-section-head .wb-badge')).toHaveText('已完成')
      await expect(execution.locator('.pipeline-stage')).toHaveCount(7)
      expect((await execution.locator('.pipeline-stage strong small').allTextContents()).map(value => value.trim())).toEqual(stages)
      for (const badge of await execution.locator('.pipeline-stage > .wb-badge').all()) await expect(badge).toHaveText('已完成')
      const publishedRef = lastDetail.stages.find(stage => stage.stage_key === 'publication')?.output?.refs.find(ref => ref.kind === 'published_asset')
      const insightRef = lastDetail.stages.find(stage => stage.stage_key === 'analysis')?.output?.refs.find(ref => ref.kind === 'insight')
      expect(publishedRef, 'publication emitted an immutable published asset').toBeDefined()
      expect(insightRef, 'analysis emitted an insight asset').toBeDefined()
      const published = await getData<AssetDocument>(api, `/lifecycle/assets/${encodeURIComponent(publishedRef!.asset_id)}?version=${publishedRef!.version}`)
      expect(published.ref.policy?.approved).toBe(true)
      expect(published.content.publication, 'real publication receipt').toBeDefined()
      expect(published.content.publication?.publication_status).toBe('published')
      if (published.content.kind === 'table') expect(published.content.publication?.storage).toBe('versioned_object_store')
      for (const cell of scenario.expected?.published_exact_strings || []) {
        expect(published.content.kind, 'exact string assertions require a published table').toBe('table')
        expect(Number.isInteger(cell.row) && cell.row >= 0, 'published cell row is a zero-based nonnegative integer').toBe(true)
        const columnIndex = published.content.columns?.indexOf(cell.column) ?? -1
        expect(columnIndex, `published column ${cell.column} exists`).toBeGreaterThanOrEqual(0)
        const value = published.content.rows?.[cell.row]?.[columnIndex]
        expect(typeof value, `published row ${cell.row} column ${cell.column} preserves string type`).toBe('string')
        expect(value, `published row ${cell.row} column ${cell.column} preserves exact digits`).toBe(cell.value)
      }
      if (published.content.kind === 'text') {
        const receipt = published.content.publication!
        expect(receipt.knowledge_base_id).toBe(scenario.target_knowledge_base_id)
        expect(receipt.knowledge_id).toBeTruthy()
        const document = await getData<{ parse_status: string; chunk_count: number }>(api, `/knowledge-bases/${encodeURIComponent(receipt.knowledge_base_id!)}/knowledge/${encodeURIComponent(receipt.knowledge_id!)}`)
        expect(document.parse_status).toBe('completed')
        expect(document.chunk_count).toBeGreaterThan(0)
      }
      const report = await getData<AnalysisDocument>(api, `/lifecycle/assets/${encodeURIComponent(insightRef!.asset_id)}?version=${insightRef!.version}`)
      expect(report.content.source_ref).toBe(`${publishedRef!.asset_id}:${publishedRef!.version}`)
      if (inputDocument.content.kind === 'text') expect(report.content.character_count).toBeGreaterThan(0)
      else expect(report.content.record_count).toBeGreaterThan(0)
      if (scenario.expected?.record_count !== undefined) expect(report.content.record_count).toBe(scenario.expected.record_count)
      if (scenario.expected?.numeric_column) {
        const column = report.content.columns?.find(column => column.name === scenario.expected!.numeric_column)
        expect(column).toBeDefined()
        expect(column!.numeric_count).toBe(scenario.expected.numeric_count)
        if (scenario.expected.mean !== undefined) expect(column!.mean).toBe(scenario.expected.mean)
        if (scenario.expected.numeric_min !== undefined) expect(column!.min).toBe(scenario.expected.numeric_min)
        if (scenario.expected.numeric_max !== undefined) expect(column!.max).toBe(scenario.expected.numeric_max)
      }
      if (scenario.analysis_prompt?.trim()) {
        expect(report.content.prompt).toBe(scenario.analysis_prompt.trim())
        expect(report.content.narrative?.trim()).toBeTruthy()
        expect(report.content.evidence?.length).toBeGreaterThan(0)
        expect(report.content.findings?.length).toBeGreaterThan(0)
        const evidenceIDs = new Set(report.content.evidence!.map(item => item.id))
        for (const finding of report.content.findings!) {
          expect(finding.evidence_ids.length).toBeGreaterThan(0)
          for (const id of finding.evidence_ids) expect(evidenceIDs.has(id)).toBe(true)
        }
        expect(report.content.decision?.verification_status).toBe('citations_checked_interpretation_unverified')
      }
      await attachJSON(info, 'published-ref-and-receipt', { ref: published.ref, publication: published.content.publication })
      await attachJSON(info, 'analysis-report', report)
      await execution.getByRole('button', { name: '查看分析报告产物', exact: true }).click()
      const renderedReport = page.getByRole('article', { name: '分析报告', exact: true })
      await expect(renderedReport).toBeVisible()
      await expect(renderedReport).toContainText(report.content.source_ref)
      if (scenario.expected?.numeric_column) {
        const numericRow = renderedReport.getByRole('row').filter({ has: page.getByRole('cell', { name: scenario.expected.numeric_column, exact: true }) })
        await expect(numericRow.getByRole('cell').nth(2)).toHaveText(String(scenario.expected.numeric_count))
        if (scenario.expected.mean !== undefined) await expect(numericRow.getByRole('cell').nth(3)).toHaveText(String(scenario.expected.mean))
        if (scenario.expected.numeric_min !== undefined) await expect(numericRow.getByRole('cell').nth(4)).toHaveText(String(scenario.expected.numeric_min))
        if (scenario.expected.numeric_max !== undefined) await expect(numericRow.getByRole('cell').nth(5)).toHaveText(String(scenario.expected.numeric_max))
      }
      if (scenario.analysis_prompt?.trim()) await expect(renderedReport.locator('.report-narrative')).toHaveText(report.content.narrative!)
      await renderedReport.screenshot({ path: info.outputPath('completed-analysis-report.png') })
      if (published.content.kind === 'text') {
        const receipt = published.content.publication!
        const query = scenario.search_query || `${marker || ''} ${(published.content.text || '').slice(0, 300)}`.trim()
        expect(query, 'text publication provides a real search query').toBeTruthy()
        await verifyVectorRetrieval(page, info, receipt.knowledge_base_id!, receipt.knowledge_id!, query, marker)
      }
    } finally {
      if (runID) {
        try { await attachJSON(info, 'last-run-state', await getData<RunDetail>(api, `/lifecycle/runs/${encodeURIComponent(runID)}`)) } catch { /* Preserve the original assertion error. */ }
      }
      await api.dispose()
    }
  })
}
