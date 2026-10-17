import { test, expect, type Page, type Route } from '@playwright/test'

// Controlled HTTP/SSE fixtures test browser state races, not backend/model quality.
const user = { id: 1, username: 'Fixture Reader', email: 'fixture@example.test' }
const tenants = [1, 2].map(id => ({ id, name: `Fixture workspace ${id}`, role: 'owner', created_at: '', updated_at: '' }))
const pageData = { items: [], next_cursor: '', has_more: false }
const asset = { asset_id: 'fixture-input', version: 1, name: 'Fixture input', kind: 'raw_asset', status: 'available' }
const budget = { max_nodes: 8, max_concurrency: 2, max_attempts: 16, max_calls: 64, max_cost_micros: 1000000, timeout_seconds: 600 }
const plan = { schema_version: 1, budget, nodes: [{ id: 'quality', capability: 'lifecycle.data-quality', capability_version: 1, depends_on: [], inputs: [{ asset: { asset_id: asset.asset_id, version: 1 }, kind: 'raw_asset' }], output_kinds: ['quality_report'], tools: ['profile'], permissions: ['read'], max_attempts: 1, max_calls_per_attempt: 2, max_cost_micros_per_attempt: 0, timeout_seconds: 30 }] }
const taskDraft = { id: 'fixture-task', goal: '保留未保存的候选', input_refs: [{ asset_id: asset.asset_id, version: 1 }], budget, status: 'draft', version: 1, current_plan_version: 0, result_refs: [], created_at: '', updated_at: '' }
const validation = { valid: true, execution_enabled: false, topological_order: ['quality'], approval_required: [], input_refs: taskDraft.input_refs, reserved_attempts: 1, reserved_calls: 2, reserved_cost_micros: 0, reserved_seconds: 30 }
const session = { id: 'history-a', title: 'Fixture history', agent_type: 'text2sql', created_at: '', updated_at: '', status: 1 }
const oldMessage = { id: 'old-answer', session_id: session.id, role: 'assistant', content: 'OLD HISTORY MUST NOT REPLACE NEW TURN', token_count: 1, created_at: '' }
const json = (route: Route, data: unknown, status = 200) => route.fulfill({ status, json: { code: status === 200 ? 0 : status, data } })
const sse = (route: Route, data: object) => route.fulfill({ status: 200, contentType: 'text/event-stream; charset=utf-8', body: `event: done\ndata: ${JSON.stringify(data)}\n\n` })
function gate() { let release!: () => void; return { promise: new Promise<void>(r => { release = r }), release: () => release() } }
type Override = (route: Route, path: string) => Promise<boolean>
async function fixture(page: Page, override?: Override) {
  const unexpected: string[] = [], writes: { path: string; body: unknown }[] = []
  await page.addInitScript(({ user, tenants }) => {
    for (const [key, value] of Object.entries({ token: 'fixture-only-token', refresh_token: 'fixture-only-refresh', user, current_tenant: tenants[0], tenants })) {
      localStorage.setItem(`cognida_${key}`, btoa(encodeURIComponent(JSON.stringify(value))))
    }
    localStorage.setItem('link:rail-collapsed', '0')
  }, { user, tenants })
  // Never let a fixture typo reach a real API, even if Vite has a proxy configured.
  await page.route('**/api/v1/**', async route => {
    const path = new URL(route.request().url()).pathname.replace('/api/v1', '')
    if (route.request().method() !== 'GET') writes.push({ path, body: route.request().postDataJSON() })
    if (await override?.(route, path)) return
    if (path === '/user/profile') return json(route, user)
    if (path === '/user/tenants') return json(route, { items: tenants, next_cursor: '' })
    if (path === '/auth/switch-tenant') return json(route, { user, tenant_id: 2, access_token: 'fixture-tenant-2', refresh_token: 'fixture-refresh-2' })
    if (path === '/datasources') return json(route, { items: [{ id: 'fixture-external', name: '外部测试库', type: 'postgres', status: 'active' }], total: 1, page: 1 })
    if (path === '/sessions') return json(route, { sessions: [session], total: 1, page: 1, size: 50 })
    if (path === '/sessions/history-a/detail') return json(route, { session, messages: [oldMessage] })
    if (path === '/governed-tasks/capabilities') return json(route, { capabilities: [], execution_enabled: false })
    if (path === '/governed-tasks/supervisor/assess') return json(route, { ready: true, questions: [], execution_enabled: false })
    if (path === '/governed-tasks' && route.request().method() === 'POST') return json(route, taskDraft)
    if (path === '/governed-tasks/fixture-task/supervisor/propose') return json(route, { task_id: taskDraft.id, task_version: 1, decision: { kind: 'proposal', explanation: 'Fixture candidate only', plan }, validation, execution_enabled: false, planning_calls: 1, saved: false })
    if (path === '/governed-tasks/fixture-task/supervisor/summary') return json(route, { task_id: taskDraft.id, task_version: 1, current_plan_version: 0, status: 'draft', planned_nodes: 0, children: [], result_refs: [], warnings: [], execution_enabled: false })
    if (path === '/lifecycle/assets') return json(route, { ...pageData, items: [asset] })
    if (['/governed-tasks', '/lifecycle/runs', '/lifecycle/invocations', '/collection-tasks'].includes(path)) return json(route, pageData)
    unexpected.push(`${route.request().method()} ${path}`)
    return route.fulfill({ status: 503, json: { code: 503, message: 'No fixture for this request' } })
  })
  return { unexpected, writes }
}
async function openData(page: Page) {
  await page.goto('/analysis/chat')
  await expect(page.locator('.composer__textarea')).toBeVisible()
  await expect(page.locator('#workspace')).toHaveValue('1')
}
async function send(page: Page, text: string) {
  await page.locator('.composer__textarea').fill(text)
  await page.locator('.btn-send').click()
}

test('same-identity membership reload preserves draft and candidate; a workspace switch clears them', async ({ page }, info) => {
  const f = await fixture(page)
  await page.goto('/tasks')
  await page.getByLabel('任务目标', { exact: true }).fill(taskDraft.goal)
  await page.getByLabel('可选资产', { exact: true }).selectOption('fixture-input:1')
  await page.getByRole('button', { name: '添加输入', exact: true }).click()
  const reloadMembership = async () => {
    await page.locator('.rail__toggle').click()
    const loaded = page.waitForResponse(response => response.url().includes('/user/tenants'))
    await page.locator('.rail__toggle').click()
    await loaded
    await expect(page.locator('#workspace')).toHaveValue('1')
  }
  await reloadMembership()
  await expect(page.getByLabel('任务目标', { exact: true })).toHaveValue(taskDraft.goal)
  await page.getByRole('button', { name: '核对并生成候选计划', exact: true }).click()
  await expect(page.locator('.proposal-panel')).toContainText('尚未保存')
  await reloadMembership()
  await expect(page.locator('.proposal-panel')).toContainText('尚未保存')
  expect(f.writes.filter(item => item.path.endsWith('/supervisor/propose'))).toHaveLength(1)
  expect(f.writes.some(item => /\/(plans|start)$/.test(item.path))).toBe(false)
  await page.screenshot({ path: info.outputPath('fixture-candidate-preserved.png') })
  page.on('dialog', dialog => dialog.accept())
  await page.locator('#workspace').selectOption('2')
  await expect(page).toHaveURL(/\/home$/)
  await page.getByRole('link', { name: '任务规划', exact: true }).click()
  await expect(page.getByLabel('任务目标', { exact: true })).toHaveValue('')
  await expect(page.locator('.proposal-panel')).toHaveCount(0)
  expect(f.unexpected).toEqual([])
})

test('Data Agent keeps the selected source and incomplete/save-failure receipt across session URL sync', async ({ page }, info) => {
  let calls = 0
  const f = await fixture(page, async (route, path) => {
    if (path !== '/agent/text2sql/stream') return false
    calls++
    await sse(route, { session_id: 'created-session', answer: calls === 1 ? 'Fixture partial answer' : 'Fixture follow-up answer', partial: calls === 1, persistence_failed: calls === 1, terminated_by: calls === 1 ? 'max_iter' : undefined })
    return true
  })
  await openData(page)
  await page.locator('.ds-selector__trigger').click()
  await page.locator('.ds-selector__option').filter({ hasText: '外部测试库' }).click()
  await send(page, 'first fixture question')
  await expect(page).toHaveURL(/\/analysis\/chat\/created-session$/)
  await expect(page.locator('.ds-selector__trigger')).toHaveText('外部测试库')
  await expect(page.getByRole('status')).toContainText('本轮未完成')
  await expect(page.getByRole('status')).toContainText('对话保存失败')
  await expect(page.locator('.bubble--ai')).toHaveText('Fixture partial answer')
  await page.screenshot({ path: info.outputPath('fixture-partial-save-warning.png') })
  await send(page, 'second fixture question')
  await expect(page.locator('.bubble--ai').last()).toHaveText('Fixture follow-up answer')
  expect(f.writes.filter(item => item.path === '/agent/text2sql/stream').map(item => item.body)).toEqual([
    { query: 'first fixture question', datasource_id: 'fixture-external' },
    { query: 'second fixture question', session_id: 'created-session', datasource_id: 'fixture-external' },
  ])
  expect(f.unexpected).toEqual([])
})

test('late historical messages cannot replace a newly completed turn', async ({ page }) => {
  const history = gate()
  let historyStarted = false
  const f = await fixture(page, async (route, path) => {
    if (path === '/sessions/history-a/detail') {
      historyStarted = true; await history.promise
      await json(route, { session, messages: [oldMessage] }); return true
    }
    if (path === '/agent/text2sql/stream') { await sse(route, { answer: 'New fixture answer', session_id: session.id }); return true }
    return false
  })
  await openData(page)
  await page.locator('.session__title').filter({ hasText: session.title }).click()
  await expect.poll(() => historyStarted).toBe(true)
  await send(page, 'New fixture question')
  await expect(page.locator('.bubble--ai')).toHaveText('New fixture answer')
  const read = page.waitForResponse(response => response.url().includes('/sessions/history-a/detail'))
  history.release(); await read
  await expect(page.locator('.bubble--user')).toHaveText('New fixture question')
  await expect(page.locator('.bubble--ai')).toHaveText('New fixture answer')
  await expect(page.getByText(oldMessage.content)).toHaveCount(0)
  expect(f.unexpected).toEqual([])
})

test('an obsolete SSE 401/refresh cannot log out the newly selected workspace', async ({ page }) => {
  const refresh = gate()
  let refreshing = false
  const f = await fixture(page, async (route, path) => {
    if (path === '/agent/text2sql/stream') { await json(route, null, 401); return true }
    if (path === '/auth/refresh') { refreshing = true; await refresh.promise; await json(route, null, 500); return true }
    return false
  })
  await openData(page)
  await send(page, 'old-scope fixture question')
  await expect.poll(() => refreshing).toBe(true)
  await page.locator('#workspace').selectOption('2')
  await expect(page).toHaveURL(/\/home$/)
  const read = page.waitForResponse(response => response.url().includes('/auth/refresh'))
  refresh.release(); await read
  await expect(page.locator('#workspace')).toHaveValue('2')
  await expect(page.getByRole('heading', { name: '工作总览', exact: true })).toBeVisible()
  expect(f.writes.filter(item => item.path === '/agent/text2sql/stream')).toHaveLength(1)
  expect(f.unexpected).toEqual([])
})
