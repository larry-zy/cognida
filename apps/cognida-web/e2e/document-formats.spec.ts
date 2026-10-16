import { randomUUID } from 'node:crypto'
import { execFileSync } from 'node:child_process'
import { resolve } from 'node:path'
import { test, expect } from '@playwright/test'
import { loginThroughUI, getData, attachJSON, waitForRun, assertNoPublication, verifyVectorRetrieval, type AssetRef, type RunDetail } from './lifecycle.helpers'

interface Fixture { format: string; path: string; marker: string; expected: string[] }
interface DocumentState { id: string; parse_status: string; chunk_count: number; error_message?: string }
test('eight real document formats → parsed knowledge → approval → seven stages → report and Milvus', async ({ page, playwright }, info) => {
  test.skip(process.env.COGNIDA_E2E_DOCUMENT_FORMATS !== '1', 'Requires isolated Go/Python/Milvus workers and actual file parsers')
  test.setTimeout(1_200_000)
  const marker = `DOCMATRIX_${randomUUID().slice(0, 8)}`
  const generated: { files: Fixture[]; no_text_pdf: string } = JSON.parse(execFileSync(process.env.COGNIDA_E2E_PYTHON || resolve('../../services/cognida-python/.venv/bin/python'), [resolve('e2e/document-fixtures.py'), info.outputPath('real-files'), marker], { encoding: 'utf8', timeout: 30_000 }))
  const api = await loginThroughUI(page, playwright)
  const evidence: unknown[] = []
  let kb = '', currentRun = ''
  try {
    const created = await api.post('/api/v1/knowledge-bases', { data: { name: marker, description: 'Isolated real file upload and lifecycle acceptance', is_public: false, graph_enabled: false }, headers: { 'Idempotency-Key': randomUUID() } })
    expect(created.ok(), await created.text()).toBe(true)
    kb = (await created.json()).data.id
    for (const fixture of generated.files) {
      await page.goto(`/knowledge/${kb}`)
      await page.getByRole('tab', { name: '文档', exact: true }).click()
      const uploaded = page.waitForResponse(r => new URL(r.url()).pathname === `/api/v1/knowledge-bases/${kb}/knowledge/file` && r.request().method() === 'POST')
      await page.locator('input[type=file]:not([webkitdirectory])').setInputFiles(fixture.path)
      await page.getByRole('dialog', { name: '上传文档', exact: true }).getByRole('button', { name: '上传', exact: true }).click()
      const response = await uploaded
      expect(response.ok(), await response.text()).toBe(true)
      const knowledgeID = (await response.json()).data.knowledge_id as string
      let document!: DocumentState
      await expect.poll(async () => {
        document = await getData<DocumentState>(api, `/knowledge-bases/${kb}/knowledge/${knowledgeID}`)
        return document.parse_status
      }, { timeout: 120_000, intervals: [500, 1000, 2000] }).not.toMatch(/^(pending|processing)$/)
      expect(document.parse_status, document.error_message).toBe('completed')
      expect(document.chunk_count).toBeGreaterThan(0)
      await page.reload()
      await page.getByRole('tab', { name: '文档', exact: true }).click()
      const row = page.getByRole('row').filter({ hasText: `${fixture.marker}.${fixture.format}` })
      await expect(row).toContainText('已完成')
      await row.getByRole('link', { name: '转入流水线', exact: true }).click()
      await expect(page).toHaveURL(new RegExp(`knowledge_id=${knowledgeID}`))
      const inputPanel = page.locator('details.wb-panel').filter({ has: page.locator('summary', { hasText: '创建输入资产' }) })
      if (await inputPanel.getAttribute('open') === null) await inputPanel.locator('summary').click()
      const form = inputPanel.locator('form')
      await expect(form.getByLabel('知识文档 ID', { exact: true })).toHaveValue(knowledgeID)
      await form.getByLabel('资产名称', { exact: true }).fill(`parsed-${fixture.marker}`)
      const inputResponse = page.waitForResponse(r => new URL(r.url()).pathname === '/api/v1/lifecycle/assets' && r.request().method() === 'POST')
      await form.getByRole('button', { name: '创建资产', exact: true }).click()
      const inputReply = await inputResponse
      expect(inputReply.ok(), await inputReply.text()).toBe(true)
      const input = (await inputReply.json()).data as AssetRef
      const imported = await getData<{ content: { text: string } }>(api, `/lifecycle/assets/${input.asset_id}?version=${input.version}`)
      for (const expected of fixture.expected) expect(imported.content.text, `${fixture.format}: parsed source evidence`).toContain(expected)
      expect(imported.content.text).not.toContain('SHOULD_NOT_PARSE')
      await page.getByLabel(/^流水线模板/).selectOption('web-knowledge-to-insight')
      await page.getByLabel(/^输入资产/).selectOption(`${input.asset_id}:${input.version}`)
      const target = page.getByLabel(/^目标知识库/)
      await expect(target).toBeVisible()
      await expect(page.getByRole('button', { name: '正在加载知识库…', exact: true })).toHaveCount(0)
      while (!(await target.locator('option').evaluateAll((options, id) => options.some(option => (option as HTMLOptionElement).value === id), kb))) {
        const more = page.getByRole('button', { name: '加载更多知识库', exact: true })
        await expect(more).toBeVisible(); await more.click()
        await expect(page.getByRole('button', { name: '正在加载知识库…', exact: true })).toHaveCount(0)
      }
      await target.selectOption(kb)
      const started = page.waitForResponse(r => new URL(r.url()).pathname === '/api/v1/lifecycle/runs' && r.request().method() === 'POST')
      await page.getByRole('button', { name: '开始执行', exact: true }).click()
      const startReply = await started
      expect(startReply.ok(), await startReply.text()).toBe(true)
      currentRun = (await startReply.json()).data.id
      const pending = await waitForRun(api, currentRun, 'approval', 120_000)
      expect(pending.approvals.some(a => ['pending', 'awaiting_approval'].includes(a.status)), JSON.stringify(pending)).toBe(true)
      await assertNoPublication(api, pending)
      await expect(page.getByRole('button', { name: '批准并继续', exact: true })).toBeVisible()
      await page.getByLabel('审核说明', { exact: true }).fill(`Actual parsed ${fixture.format} source reviewed in isolated acceptance`)
      await page.getByRole('button', { name: '批准并继续', exact: true }).click()
      const complete = await waitForRun(api, currentRun, 'terminal', 120_000)
      expect(complete.run.status, JSON.stringify(complete)).toBe('succeeded')
      expect(complete.stages).toHaveLength(7)
      expect(complete.stages.every(stage => stage.status === 'succeeded')).toBe(true)
      const published = complete.stages.find(stage => stage.stage_key === 'publication')!.output!.refs.find(ref => ref.kind === 'published_asset')!
      const insight = complete.stages.find(stage => stage.stage_key === 'analysis')!.output!.refs.find(ref => ref.kind === 'insight')!
      const publication = await getData<{ content: { text: string; publication: { knowledge_id: string; knowledge_base_id: string; publication_status: string } } }>(api, `/lifecycle/assets/${published.asset_id}?version=${published.version}`)
      expect(publication.content.publication.publication_status).toBe('published')
      for (const expected of fixture.expected) expect(publication.content.text, `${fixture.format}: published evidence`).toContain(expected)
      const report = await getData<{ content: { character_count: number; source_ref: string } }>(api, `/lifecycle/assets/${insight.asset_id}?version=${insight.version}`)
      expect(report.content.character_count).toBeGreaterThan(0)
      expect(report.content.source_ref).toBe(`${published.asset_id}:${published.version}`)
      await page.getByRole('button', { name: '查看分析报告产物', exact: true }).click()
      await expect(page.getByRole('article', { name: '分析报告', exact: true })).toContainText(report.content.source_ref)
      await page.screenshot({ path: info.outputPath(`${fixture.format}-report.png`), fullPage: true })
      await verifyVectorRetrieval(page, info, kb, publication.content.publication.knowledge_id, `${fixture.marker} The North region recorded revenue of 100 units in January. The South region recorded revenue of 200 units in February.`, fixture.marker)
      evidence.push({ format: fixture.format, source_document: knowledgeID, run: complete, input, published, report })
      await attachJSON(info, `${fixture.format}-complete-evidence`, evidence.at(-1))
      currentRun = ''
    }
    // A PDF without extractable text must persist a real parser failure, with no fabricated chunks.
    await page.goto(`/knowledge/${kb}`)
    await page.getByRole('tab', { name: '文档', exact: true }).click()
    const uploaded = page.waitForResponse(r => new URL(r.url()).pathname === `/api/v1/knowledge-bases/${kb}/knowledge/file` && r.request().method() === 'POST')
    await page.locator('input[type=file]:not([webkitdirectory])').setInputFiles(generated.no_text_pdf)
    await page.getByRole('dialog', { name: '上传文档', exact: true }).getByRole('button', { name: '上传', exact: true }).click()
    const noText = await uploaded
    expect(noText.ok()).toBe(true)
    const noTextID = (await noText.json()).data.knowledge_id
    let failed!: DocumentState
    await expect.poll(async () => { failed = await getData<DocumentState>(api, `/knowledge-bases/${kb}/knowledge/${noTextID}`); return failed.parse_status }, { timeout: 60_000 }).toBe('failed')
    expect(failed.chunk_count).toBe(0)
    expect(failed.error_message).toContain('no extractable text')
    await attachJSON(info, 'no-text-pdf-honest-failure', failed)
  } finally {
    if (currentRun) { try { await attachJSON(info, 'last-run', await getData<RunDetail>(api, `/lifecycle/runs/${currentRun}`)) } catch { /* retain primary failure */ } }
    await attachJSON(info, 'document-format-matrix', { knowledge_base_id: kb, evidence, retained_for_review: true })
    await api.dispose()
  }
})
