import { readFileSync } from 'node:fs'
import { writeFile } from 'node:fs/promises'
import { expect, type APIRequestContext, type Locator, type Page, type PlaywrightWorkerArgs, type Response, type TestInfo } from '@playwright/test'
export interface AssetRef { asset_id: string; version: number; kind: string; producer_id: string; policy?: { approved: boolean } }
export interface RunDetail {
  run: { id: string; status: string; error?: string }
  stages: Array<{ id: string; stage_key: string; status: string; error?: string; output?: { refs: AssetRef[] } }>
  approvals: Array<{ id: string; status: string }>
}

export type Source = 'text' | 'table' | 'web' | 'knowledge' | 'collection' | 'datasource' | 'result' | 'evaluation'
export interface Scenario {
  id: string
  source: Source
  text?: string
  columns?: string[]
  column_types?: Record<string, 'text' | 'number'>
  rows?: Array<Array<string | number | null>>
  url?: string
  knowledge_id?: string
  run_id?: string
  item_id?: string
  datasource_id?: string
  schema?: string
  table?: string
  session_id?: string
  result_id?: string
  target_knowledge_base_id?: string
  analysis_prompt?: string
  search_query?: string
  expected?: {
    record_count?: number
    numeric_column?: string
    numeric_count?: number
    mean?: number
    numeric_min?: number
    numeric_max?: number
    text_includes?: string
    /** Zero-based row and column name in the immutable published table. */
    published_exact_strings?: Array<{ row: number; column: string; value: string }>
  }
}
interface FixtureFile {
  target_knowledge_base_id?: string
  analysis_prompt?: string
  scenarios: Scenario[]
}
export function loadScenarios(): Scenario[] {
  const defaults: Scenario[] = [
    {
      id: 'manual-text', source: 'text',
      text: 'Cognida isolated acceptance report. The North region recorded revenue of 100 units in January. The South region recorded revenue of 200 units in February. The West region recorded revenue of 300 units in March. These observations describe the supplied sample only and do not establish causes or future performance.',
      expected: { text_includes: 'Cognida isolated acceptance report' },
    },
    {
      id: 'manual-table', source: 'table', columns: ['region', 'revenue'], column_types: { region: 'text', revenue: 'number' },
      rows: [['North', 100], ['South', 200], ['West', 300]],
      expected: { record_count: 3, numeric_column: 'revenue', numeric_count: 3, mean: 200 },
    },
  ]
  const path = process.env.COGNIDA_E2E_FIXTURES_FILE
  const config: FixtureFile = path ? JSON.parse(readFileSync(path, 'utf8')) : { scenarios: defaults }
  if (!Array.isArray(config.scenarios) || !config.scenarios.length) throw new Error('Fixture scenarios must be a nonempty array')
  const supported: Source[] = ['text', 'table', 'web', 'knowledge', 'collection', 'datasource', 'result', 'evaluation']
  const names = new Set<string>()
  return config.scenarios.map(scenario => {
    if (!scenario.id || names.has(scenario.id) || !supported.includes(scenario.source)) throw new Error('Each fixture needs a unique id and a supported source')
    names.add(scenario.id)
    return {
      ...scenario,
      target_knowledge_base_id: scenario.target_knowledge_base_id || config.target_knowledge_base_id || process.env.COGNIDA_E2E_KNOWLEDGE_BASE_ID,
      analysis_prompt: scenario.analysis_prompt ?? config.analysis_prompt ?? process.env.COGNIDA_E2E_ANALYSIS_PROMPT ?? '',
    }
  })
}
export async function loginThroughUI(page: Page, playwright: PlaywrightWorkerArgs['playwright']): Promise<APIRequestContext> {
  const account = JSON.parse(readFileSync(process.env.COGNIDA_E2E_ACCOUNT_FILE!, 'utf8'))
  const credentials = account.credentials || account
  if (typeof credentials.email !== 'string' || typeof credentials.password !== 'string') throw new Error('Account file must contain email/password or credentials.email/password')
  await page.goto('/login')
  await page.locator('input[type="email"]').fill(credentials.email)
  await page.locator('input[type="password"]').fill(credentials.password)
  // Retry only a rate-limited real UI submission. Keep production limits intact,
  // obey Retry-After, and cap all attempts plus waits at one minute.
  const deadline = Date.now() + 60_000
  let login: Response | undefined
  for (let retry = 0; retry <= 2; retry++) {
    const remaining = deadline - Date.now()
    if (remaining <= 0) throw new Error('UI login retry budget exceeded 60 seconds')
    const [response] = await Promise.all([
      page.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/auth/login' && response.request().method() === 'POST', { timeout: remaining }),
      page.locator('form.login-form button[type="submit"]').click({ timeout: remaining }),
    ])
    login = response
    if (login.status() !== 429) {
      expect(login.status(), 'real UI login HTTP status').toBe(200)
      break
    }
    if (retry === 2) throw new Error('UI login remains rate limited after two retries')
    const retryAfter = login.headers()['retry-after']?.trim()
    if (!retryAfter) throw new Error('Rate-limited UI login did not provide Retry-After')
    const milliseconds = /^\d+(?:\.\d+)?$/.test(retryAfter)
      ? Number(retryAfter) * 1000
      : Date.parse(retryAfter) - Date.now()
    if (!Number.isFinite(milliseconds)) throw new Error('Rate-limited UI login returned invalid Retry-After')
    // A zero or past HTTP date still gets a small bounded pause rather than a busy retry.
    const delay = Math.max(250, milliseconds)
    if (delay >= deadline - Date.now()) throw new Error('Server Retry-After exceeds the remaining 60-second UI login budget')
    await new Promise(resolve => setTimeout(resolve, delay))
  }
  if (!login) throw new Error('UI login did not return a response')
  const auth = (await login.json()).data
  if (!auth?.access_token || !auth.tenant_id) throw new Error('UI login did not return a tenant-bound token')
  await expect(page).toHaveURL(/\/home(?:[?#]|$)/)
  return playwright.request.newContext({
    baseURL: process.env.COGNIDA_E2E_API_URL,
    extraHTTPHeaders: { Authorization: `Bearer ${auth.access_token}`, 'X-Tenant-ID': String(auth.tenant_id) },
  })
}
export async function getData<T>(api: APIRequestContext, path: string): Promise<T> {
  const response = await api.get(`/api/v1${path}`)
  expect(response.status(), `GET ${path}`).toBe(200)
  const payload = await response.json()
  expect(payload.data, `GET ${path} data`).toBeDefined()
  return payload.data as T
}
export async function attachJSON(info: TestInfo, name: string, value: unknown) {
  // Keep API evidence in the case's output directory as well as the HTML
  // report: a later focused run replaces the shared playwright-report folder.
  const path = info.outputPath(`${name.replace(/[^a-zA-Z0-9_.-]/g, '_')}.json`)
  await writeFile(path, JSON.stringify(value, null, 2))
  await info.attach(name, { path, contentType: 'application/json' })
}
/** Select an authorized asset through the page's real cursor controls. */
export async function selectPagedAsset(page: Page, select: Locator, value: string, moreName = '加载更多资产') {
  const more = page.getByRole('button', { name: moreName, exact: true })
  const present = () => select.locator('option').evaluateAll((options, target) => options.some(option => (option as HTMLOptionElement).value === target), value)
  await expect(select).toBeVisible()
  for (let count = 0; count < 100; count++) {
    await expect.poll(async () => (await present()) || ((await more.isVisible()) && (await more.isEnabled())), { message: `Authorized source ${value} must be reachable through cursor pages` }).toBe(true)
    if (await present()) { await select.selectOption(value); return }
    const loaded = page.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/lifecycle/assets' && response.request().method() === 'GET')
    await more.click()
    const response = await loaded
    expect(response.ok(), 'Load the next real authorized asset page').toBe(true)
    await response.finished()
  }
  throw new Error('Source selection exceeded 100 real UI cursor pages')
}
function required(scenario: Scenario, key: keyof Scenario): string {
  const value = scenario[key]
  if (typeof value !== 'string' || !value.trim() || value.startsWith('REPLACE_')) throw new Error(`${scenario.id}: provide fixture field ${key}`)
  return value
}
export async function createInput(page: Page, scenario: Scenario, name: string): Promise<AssetRef> {
  await page.goto('/pipelines')
  const panel = page.locator('details.wb-panel').filter({ has: page.locator('summary', { hasText: '创建输入资产' }) })
  if (await panel.getAttribute('open') === null) await panel.locator('summary').click()
  const form = panel.locator('form')
  await form.getByLabel('资产名称', { exact: true }).fill(name)
  await form.getByLabel(/^数据类型/).selectOption(scenario.source)
  switch (scenario.source) {
    case 'text': await form.getByLabel('文本内容', { exact: true }).fill(required(scenario, 'text')); break
    case 'table': {
      if (!scenario.columns?.length || !scenario.rows?.length) throw new Error(`${scenario.id}: table fixture needs columns and rows`)
      await form.getByLabel('列名', { exact: true }).fill(scenario.columns.join(','))
      for (const column of scenario.columns) await form.getByLabel(`${column} 类型`, { exact: true }).selectOption(scenario.column_types?.[column] || 'text')
      for (const [index, row] of scenario.rows.entries()) {
        if (row.length !== scenario.columns.length) throw new Error(`${scenario.id}: row width does not match columns`)
        await form.getByRole('button', { name: '＋ 添加数据行', exact: true }).click()
        for (const [columnIndex, column] of scenario.columns.entries()) await form.getByLabel(`数据行${index + 1} ${column}`, { exact: true }).fill(String(row[columnIndex] ?? ''))
      }
      break
    }
    case 'web': await form.getByLabel('网页地址', { exact: true }).fill(required(scenario, 'url')); break
    case 'knowledge': await form.getByLabel('知识文档 ID', { exact: true }).fill(required(scenario, 'knowledge_id')); break
    case 'collection':
      await form.getByLabel('运行 / 评测任务 ID', { exact: true }).fill(required(scenario, 'run_id'))
      await form.getByLabel('采集条目 ID', { exact: true }).fill(required(scenario, 'item_id')); break
    case 'evaluation': await form.getByLabel('运行 / 评测任务 ID', { exact: true }).fill(required(scenario, 'run_id')); break
    case 'result':
      await form.getByLabel('会话 ID', { exact: true }).fill(required(scenario, 'session_id'))
      await form.getByLabel('查询结果 ID', { exact: true }).fill(required(scenario, 'result_id')); break
    case 'datasource':
      await form.getByLabel(/^数据源/).selectOption(required(scenario, 'datasource_id'))
      await form.getByLabel('Schema', { exact: true }).fill(scenario.schema || '')
      await form.getByLabel('表名', { exact: true }).fill(required(scenario, 'table'))
      await expect(form).toContainText('最多 1,000 行的快照'); break
  }
  const observedAsset = page.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/lifecycle/assets' && response.request().method() === 'POST')
  await form.getByRole('button', { name: '创建资产', exact: true }).click()
  const response = await observedAsset
  expect(response.ok(), `${scenario.id}: create source asset`).toBe(true)
  const asset: AssetRef = (await response.json()).data
  expect(asset.asset_id).toBeTruthy()
  await expect(page.getByLabel(/^输入资产/)).toHaveValue(`${asset.asset_id}:${asset.version}`)
  return asset
}
export async function waitForRun(api: APIRequestContext, id: string, phase: 'approval' | 'terminal', timeout: number): Promise<RunDetail> {
  let detail!: RunDetail
  await expect.poll(async () => {
    detail = await getData<RunDetail>(api, `/lifecycle/runs/${encodeURIComponent(id)}`)
    const terminal = ['succeeded', 'failed', 'cancelled', 'quarantined'].includes(detail.run.status)
    return terminal || (phase === 'approval' && detail.approvals.some(approval => ['pending', 'awaiting_approval'].includes(approval.status)))
  }, { timeout, intervals: [500, 1000, 2000], message: `run ${id} reaches ${phase}` }).toBe(true)
  return detail
}
export async function assertNoPublication(api: APIRequestContext, detail: RunDetail) {
  expect(detail.stages.flatMap(stage => stage.output?.refs || []).filter(ref => ref.kind === 'published_asset')).toEqual([])
  const producers = new Set(detail.stages.map(stage => stage.id))
  let cursor = ''
  do {
    const page = await getData<{ items: AssetRef[]; next_cursor?: string }>(api, `/lifecycle/assets?kind=published_asset&page_size=100${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`)
    expect(page.items.filter(asset => producers.has(asset.producer_id)), 'no published asset exists for this run before approval').toEqual([])
    if (page.next_cursor && page.next_cursor === cursor) throw new Error('Published asset pagination did not advance')
    cursor = page.next_cursor || ''
  } while (cursor)
}

export async function verifyVectorRetrieval(page: Page, info: TestInfo, kbID: string, knowledgeID: string, query: string, marker?: string) {
  await page.goto(`/knowledge/${encodeURIComponent(kbID)}`)
  await page.getByRole('tab', { name: '检索', exact: true }).click()
  await page.getByPlaceholder('输入要搜索的内容...').fill(query)
  await page.getByLabel(/^检索模式/).selectOption('vector')
  const limits = page.locator('.search-options .ui-input-number__input')
  await limits.nth(0).fill('20')
  await limits.nth(0).blur()
  await limits.nth(1).fill('0')
  await limits.nth(1).blur()
  let matching: Array<{ chunk_id: string; knowledge_id: string; content: string; sources?: string[] }> = []
  // Milvus publication visibility can lag its durable receipt. Retry real UI searches only.
  await expect.poll(async () => {
    const observed = page.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/knowledge/search' && response.request().method() === 'POST')
    await page.getByRole('button', { name: '搜索', exact: true }).click()
    const response = await observed
    expect(response.status(), 'real knowledge search HTTP status').toBe(200)
    expect(response.request().postDataJSON()).toMatchObject({ retrieval_mode: 'vector', kb_ids: [kbID] })
    const result = (await response.json()).data
    await attachJSON(info, 'knowledge-search-response', result)
    matching = (result?.items || []).filter((item: { knowledge_id: string; content: string; sources?: string[] }) => item.knowledge_id === knowledgeID && (!marker || item.content.includes(marker)) && item.sources?.includes('milvus_vector'))
    return matching.length
  }, { timeout: 60_000, intervals: [1000, 2000, 5000], message: 'published document appears through real vector retrieval' }).toBeGreaterThan(0)
  for (const item of matching) {
    expect(item.chunk_id).toBeTruthy()
    expect(item.sources, 'retrieval provenance must prove Milvus vector recall, not merely echo requested mode').toContain('milvus_vector')
    expect(item.sources).not.toContain('database_lexical')
  }
  await expect(page.locator('.search-result-item').first()).toBeVisible()
  if (marker) await expect(page.locator('.search-result-item').filter({ hasText: marker }).first()).toBeVisible()
  await page.screenshot({ path: info.outputPath('published-knowledge-vector-search.png'), fullPage: true })
}
