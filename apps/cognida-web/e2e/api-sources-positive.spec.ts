import { randomUUID } from 'node:crypto'
import { expect, test } from '@playwright/test'
import { assertNoPublication, attachJSON, getData, loginThroughUI, waitForRun, type AssetRef } from './lifecycle.helpers'
interface APIIngestionRun {
  id: string; status: string; error?: string; record_count: number; requests: number; consistency: string
  source_id: string; source_version: number; config: Record<string, unknown>; deadline: string
  retryable: boolean; bytes: number; watermark_before?: string; watermark_after?: string
  pages: { digest: string; bytes: number; records: number }[]
  output_refs: { asset_id: string; version: number }[]
}
interface APISource { id: string; version: number; config: Record<string, unknown>; watermark?: string }

interface Document {
  ref: AssetRef
  content: { kind?: string; columns?: string[]; rows?: unknown[][]; column_types?: Record<string, string>; publication?: { publication_status: string; storage: string }; source_ref?: string; record_count?: number }
}

test('real public HTTPS JSON → candidate → seven stages → approval → report', async ({ page, playwright }, info) => {
  test.skip(process.env.COGNIDA_E2E_API_SOURCES !== '1', 'Requires isolated API, Python and lifecycle workers with public HTTPS access')
  const api = await loginThroughUI(page, playwright)
  try {
    const name = `Public JSON review ${randomUUID()}`
    await page.goto('/api-sources')
    await page.getByText('登记新凭据', { exact: true }).click()
    const credentials = page.getByRole('form', { name: '登记 API 凭据' })
    await credentials.getByLabel('凭据名称', { exact: true }).fill(name)
    await credentials.getByLabel('绑定来源', { exact: true }).fill('https://jsonplaceholder.typicode.com/')
    // The real public endpoint serves synthetic records without requiring auth.
    // This disposable bearer is not a user's credential or proof of provider
    // authentication; that behavior is covered by the real TLS fixture.
    const secret = `synthetic-public-e2e-${randomUUID()}`
    await credentials.getByLabel('密钥', { exact: true }).fill(secret)
    const credentialResponse = page.waitForResponse(r => new URL(r.url()).pathname === '/api/v1/api-credentials' && r.request().method() === 'POST')
    await credentials.getByRole('button', { name: '加密保存凭据', exact: true }).click()
    const savedCredential = await credentialResponse
    expect(savedCredential.ok()).toBe(true)
    const credentialRaw = await savedCredential.text()
    expect(credentialRaw).not.toContain(secret)
    const credential = JSON.parse(credentialRaw).data
    await expect(credentials.getByLabel('密钥', { exact: true })).toHaveValue('')
    await page.getByRole('button', { name: '＋ 配置来源', exact: true }).click()
    const form = page.getByRole('region', { name: '配置 API 来源' })
    await form.getByLabel('来源名称', { exact: true }).fill(name)
    await form.getByLabel('已登记凭据', { exact: true }).selectOption(credential.id)
    await form.getByLabel('GET 地址', { exact: true }).fill('https://jsonplaceholder.typicode.com/posts?_limit=3')
    await form.getByLabel('记录数组位置', { exact: true }).fill('')
    await form.getByLabel('稳定记录键', { exact: true }).fill('id')
    await form.getByText('一致性、增量与读取上限', { exact: true }).click()
    await form.getByLabel('最多页数', { exact: true }).fill('1')
    await form.getByLabel('最多记录', { exact: true }).fill('3')
    await form.getByLabel('总请求上限', { exact: true }).fill('3')
    const sourceResponse = page.waitForResponse(r => new URL(r.url()).pathname === '/api/v1/api-sources' && r.request().method() === 'POST')
    await form.getByRole('button', { name: '保存来源版本', exact: true }).click()
    const saved = await sourceResponse
    expect(saved.ok(), await saved.text()).toBe(true)
    const source = (await saved.json()).data as APISource
    expect(source.config.limits).toMatchObject({ max_requests: 3, max_pages: 1, max_records: 3 })
    const started = page.waitForResponse(r => new URL(r.url()).pathname === `/api/v1/api-sources/${source.id}/runs` && r.request().method() === 'POST')
    await page.locator('.source-list > li').filter({ hasText: name }).getByRole('button', { name: '采集此版本', exact: true }).click()
    const submitted = await started
    expect(submitted.ok(), await submitted.text()).toBe(true)
    const initial = (await submitted.json()).data as APIIngestionRun
    const observationDeadline = Date.now() + 140_000
    const remaining = () => {
      const milliseconds = observationDeadline - Date.now()
      expect(milliseconds, 'All ingestion attempts share the original 140 second observation window').toBeGreaterThan(0)
      return milliseconds
    }
    const readWithinWindow = async <T,>(path: string): Promise<T> => {
      const response = await api.get(`/api/v1${path}`, { timeout: remaining() })
      expect(response.status(), `GET ${path}: ${await response.text()}`).toBe(200)
      const payload = await response.json()
      expect(payload.data).toBeDefined(); remaining()
      return payload.data as T
    }
    const fixedRun = (run: APIIngestionRun) => {
      expect(run.id).toBe(initial.id); expect(run.source_id).toBe(source.id); expect(run.source_version).toBe(source.version)
      expect(run.config).toEqual(source.config); expect(run.deadline).toBe(initial.deadline)
      expect(run.watermark_before || '').toBe(source.watermark || '')
      expect(JSON.stringify(run)).not.toContain(secret)
    }
    const emptyCheckpoint = (run: APIIngestionRun) => {
      expect(run.pages || []).toHaveLength(0); expect(run.record_count).toBe(0); expect(run.bytes).toBe(0)
      expect(run.output_refs || []).toHaveLength(0)
      expect(run.watermark_after || '').toBe(run.watermark_before || '')
    }
    fixedRun(initial); emptyCheckpoint(initial); expect(initial.requests).toBe(0)
    await attachJSON(info, 'public-json-initial-run', initial)
    let ingestion!: APIIngestionRun
    let lastRequests = 0, attempts = 0
    for (let attempt = 1; attempt <= 3; attempt++) {
      let observed: APIIngestionRun | undefined
      try {
        await expect.poll(async () => {
          observed = await readWithinWindow<APIIngestionRun>(`/api-ingestion-runs/${initial.id}`)
          fixedRun(observed)
          expect(Number.isInteger(observed.requests)).toBe(true)
          expect(observed.requests).toBeGreaterThanOrEqual(lastRequests)
          expect(observed.requests).toBeLessThanOrEqual(lastRequests + 1)
          expect(observed.requests).toBeLessThanOrEqual(attempt)
          lastRequests = observed.requests
          return ['completed', 'failed', 'partial', 'timed_out', 'cancelled'].includes(observed.status)
        }, { timeout: remaining() }).toBe(true)
      } finally {
        // Keep failed attempts as evidence too; never replace their state with success.
        if (observed) await attachJSON(info, `public-json-attempt-${attempt}`, observed)
      }
      ingestion = observed!
      expect(ingestion.requests).toBe(attempt); attempts = attempt
      if (ingestion.status === 'completed') break
      expect(ingestion.status, ingestion.error).toBe('partial')
      expect(ingestion.retryable, ingestion.error).toBe(true)
      emptyCheckpoint(ingestion)
      expect(attempt, 'The explicit three-request budget is exhausted; do not start another run').toBeLessThan(3)
      const head = await readWithinWindow<APISource>(`/api-sources/${source.id}`)
      expect(head.id).toBe(source.id); expect(head.version).toBe(source.version); expect(head.config).toEqual(source.config)
      expect(head.watermark || '').toBe(source.watermark || '')
      await attachJSON(info, `public-json-source-before-resume-${attempt}`, head)
      // This is the product's explicit, budgeted recovery action, not a replay
      // of submission or a claim that the sanitized error was a network fault.
      const receipt = page.getByRole('region', { name: '采集回执' })
      const [resumed] = await Promise.all([
        page.waitForResponse(r => new URL(r.url()).pathname === `/api/v1/api-ingestion-runs/${initial.id}/resume` && r.request().method() === 'POST', { timeout: remaining() }),
        receipt.getByRole('button', { name: '从原快照检查点继续', exact: true }).click({ timeout: Math.min(20_000, remaining()) }),
      ])
      expect(resumed.ok(), `Resume ${resumed.status()}: ${await resumed.text()}`).toBe(true)
      const checkpoint = (await resumed.json()).data as APIIngestionRun
      await attachJSON(info, `public-json-resumed-${attempt}`, checkpoint)
      fixedRun(checkpoint); emptyCheckpoint(checkpoint); expect(checkpoint.requests).toBe(attempt)
      expect(checkpoint.status).toBe('queued')
      remaining()
    }
    expect(ingestion.status, ingestion.error || 'real public JSON ingestion').toBe('completed')
    expect(ingestion.record_count).toBe(3)
    expect(ingestion.requests).toBe(attempts)
    expect(ingestion.pages).toHaveLength(1)
    expect(ingestion.pages[0]!.digest).toMatch(/^[a-f0-9]{64}$/)
    expect(ingestion.pages[0]!.bytes).toBeGreaterThan(0)
    expect(ingestion.pages[0]!.records).toBe(3)
    expect(ingestion.consistency).not.toContain('provider_snapshot')
    expect(JSON.stringify(ingestion)).not.toContain(secret)
    const documents: Document[] = []
    for (const ref of ingestion.output_refs) documents.push(await getData<Document>(api, `/lifecycle/assets/${ref.asset_id}?version=${ref.version}`))
    const candidate = documents.find(doc => doc.ref.kind === 'raw_asset')!
    expect(candidate, 'the real receipt contains a readable raw candidate').toBeDefined()
    expect(candidate.ref.producer_id).toBe(`api-ingestion:${ingestion.id}`)
    expect(candidate.content.kind).toBe('table')
    expect(candidate.content.rows).toHaveLength(3)
    expect(candidate.content.publication).toBeUndefined()
    expect(candidate.ref.policy?.approved).not.toBe(true)
    const idColumn = candidate.content.columns!.indexOf('id')
    expect(idColumn).toBeGreaterThanOrEqual(0)
    expect(candidate.content.rows!.map(row => row[idColumn])).toEqual(['1', '2', '3'])
    expect(candidate.content.column_types?.id).toBe('NUMERIC')
    await page.reload()
    const receipt = page.getByRole('region', { name: '采集回执' })
    await expect(receipt).toContainText('完整候选已提交')
    await expect(receipt).toContainText('来源未提供可验证的跨页一致性保证')
    await receipt.getByRole('link', { name: '进入处理与发布', exact: true }).click()
    // Use the real run output. Do not import another copy of the source.
    const input = `${candidate.ref.asset_id}:${candidate.ref.version}`
    const selection = page.getByLabel(/^输入资产/)
    for (let pageNumber = 0; pageNumber < 100; pageNumber++) {
      await expect(page.getByRole('button', { name: '正在加载资产…', exact: true })).toHaveCount(0)
      if (await selection.locator('option').evaluateAll((options, value) => options.some(option => (option as HTMLOptionElement).value === value), input)) break
      await page.getByRole('button', { name: '加载更多资产', exact: true }).click()
    }
    await selection.selectOption(input)
    await page.getByLabel(/^流水线模板/).selectOption('web-knowledge-to-insight')
    await page.locator('[data-stage="analysis"] textarea').fill('')
    const startedPipeline = page.waitForResponse(r => new URL(r.url()).pathname === '/api/v1/lifecycle/runs' && r.request().method() === 'POST')
    await page.getByRole('button', { name: '开始执行', exact: true }).click()
    const pipelineResponse = await startedPipeline
    expect(pipelineResponse.ok(), await pipelineResponse.text()).toBe(true)
    const run = (await pipelineResponse.json()).data
    const before = await waitForRun(api, run.id, 'approval', 120_000)
    expect(before.approvals.some(row => row.status === 'pending' || row.status === 'awaiting_approval')).toBe(true)
    await assertNoPublication(api, before)
    await page.getByLabel('审核说明', { exact: true }).fill('Real public HTTPS synthetic JSON candidate reviewed in isolated E2E')
    await page.getByRole('button', { name: '批准并继续', exact: true }).click()
    const completed = await waitForRun(api, run.id, 'terminal', 120_000)
    expect(completed.run.status, JSON.stringify(completed.stages)).toBe('succeeded')
    expect(completed.stages.map(stage => stage.stage_key).sort()).toEqual(['access', 'quality', 'cleaning', 'enrichment', 'publication', 'analysis', 'evaluation'].sort())
    expect(completed.stages.every(stage => stage.status === 'succeeded')).toBe(true)
    const publishedRef = completed.stages.find(stage => stage.stage_key === 'publication')!.output!.refs.find(ref => ref.kind === 'published_asset')!
    const reportRef = completed.stages.find(stage => stage.stage_key === 'analysis')!.output!.refs.find(ref => ref.kind === 'insight')!
    const published = await getData<Document>(api, `/lifecycle/assets/${publishedRef.asset_id}?version=${publishedRef.version}`)
    expect(published.ref.policy?.approved).toBe(true)
    expect(published.content.publication?.publication_status).toBe('published')
    expect(published.content.publication?.storage).toBe('versioned_object_store')
    expect(published.content.rows).toHaveLength(3)
    expect(published.content.rows!.map(row => row[published.content.columns!.indexOf('id')])).toEqual(['1', '2', '3'])
    const report = await getData<Document>(api, `/lifecycle/assets/${reportRef.asset_id}?version=${reportRef.version}`)
    expect(report.content.record_count).toBe(3)
    expect(report.content.source_ref).toBe(`${publishedRef.asset_id}:${publishedRef.version}`)
    await page.getByRole('button', { name: '查看分析报告产物', exact: true }).click()
    const renderedReport = page.getByRole('article', { name: '分析报告', exact: true })
    await expect(renderedReport).toBeVisible()
    await renderedReport.screenshot({ path: info.outputPath('api-json-candidate-seven-stage-report.png') })
    await attachJSON(info, 'public-json-candidate-to-report', { source_id: source.id, ingestion, candidate: candidate.ref, completed, published: published.ref, report, evidence_scope: 'Real JSONPlaceholder public HTTPS GET with synthetic records. Public service does not validate the disposable Bearer; authentication/pagination are independently covered by real TLS integration fixtures.' })
  } finally { await api.dispose() }
})
