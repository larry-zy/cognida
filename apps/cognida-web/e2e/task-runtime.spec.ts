import { randomUUID } from 'node:crypto'
import { test, expect, type APIRequestContext, type Page, type TestInfo } from '@playwright/test'
import { attachJSON, createInput, getData, loginThroughUI, type AssetRef } from './lifecycle.helpers'

type Ref = { asset_id: string; version: number; kind?: string }
type Task = { id: string; version: number; status: string; current_plan_version: number; execution_plan_version: number; started_at?: string }
type Capability = { id: string; version: number; tools: string[]; permissions: string[]; input_kinds: string[]; output_kinds: string[]; execution?: { profile: string; base_calls: number; calls_per_input: number; max_timeout_seconds: number } }
type Input = { asset?: Ref; from_node?: string; kind: string }
type Node = { id: string; capability: string; capability_version: number; depends_on: string[]; inputs: Input[]; output_kinds: string[]; tools: string[]; permissions: string[]; max_attempts: number; max_calls_per_attempt: number; max_cost_micros_per_attempt: number; timeout_seconds: number }
type Summary = { task_id: string; task_version: number; status: string; current_plan_version: number; terminal: boolean; can_cancel: boolean; nodes: Array<{ node_id: string; capability: string; status: string; attempt: number; error?: string; output_refs: Ref[]; domain_kind?: string; domain_id?: string }>; children: Array<{ node_id: string; domain_kind: string; domain_id: string; attempt: number; domain_status: string; dispatch_status: string }>; result_refs: Ref[]; warnings: string[] }
type Readiness = { task_id: string; task_version: number; plan_version: number; allowed: boolean; execution_enabled: boolean; blockers: unknown[] }

async function postData<T>(api: APIRequestContext, path: string, body: unknown, idempotent = false): Promise<T> {
  const response = await api.post(`/api/v1${path}`, { data: body, headers: idempotent ? { 'Idempotency-Key': randomUUID() } : undefined })
  expect(response.ok(), `POST ${path}: ${response.status()} ${await response.text()}`).toBe(true)
  return (await response.json()).data as T
}
async function createFixture(page: Page, api: APIRequestContext, shape: 'chain' | 'parallel' | 'cancel') {
  const marker = `task-runtime-${shape}-${Date.now()}`
  let asset: AssetRef
  if (shape === 'parallel') {
    // Tiny text can finish before the second real dispatch even with two slots.
    // This legal 100,000-cell fixture exercises actual Python work long enough
    // to observe overlap; bulk preparation uses the authenticated input API.
    const columns = Array.from({ length: 10 }, (_, column) => `field_${column}`)
    const rows = Array.from({ length: 10_000 }, (_, row) => columns.map((_, column) => `synthetic-row-${row}-field-${column}-value`))
    const input = { kind: 'table', columns, rows }
    expect(Buffer.byteLength(JSON.stringify(input))).toBeLessThan(4 * 1024 * 1024)
    asset = await postData<AssetRef>(api, '/lifecycle/assets', { input, name: marker }, true)
    const stored = await getData<{ content: { rows: unknown[]; columns: string[] } }>(api, `/lifecycle/assets/${asset.asset_id}?version=${asset.version}`)
    expect(stored.content.rows).toHaveLength(10_000); expect(stored.content.columns).toHaveLength(10)
  } else {
    asset = await createInput(page, { id: marker, source: 'text', text: `Synthetic controlled execution fixture ${marker}. North revenue is 100 units. South revenue is 200 units. The sample supports data quality checks, deterministic whitespace cleaning, rules enrichment and artifact integrity review. No publication or model analysis is requested.` }, marker)
  }
  const registered = await getData<{ capabilities: Capability[] }>(api, '/governed-tasks/capabilities')
  const capability = (name: string) => {
    const result = registered.capabilities.find(item => item.id === `lifecycle.${name}`)
    expect(result?.execution, `${name} must have a real execution adapter`).toBeDefined()
    return result!
  }
  const node = (id: string, name: string, inputs: Input[], outputKinds: string[], dependsOn: string[] = []): Node => {
    const cap = capability(name), execution = cap.execution!
    for (const input of inputs) expect(cap.input_kinds).toContain(input.kind)
    for (const output of outputKinds) expect(cap.output_kinds).toContain(output)
    return { id, capability: cap.id, capability_version: cap.version, depends_on: dependsOn, inputs, output_kinds: outputKinds, tools: cap.tools, permissions: cap.permissions, max_attempts: 1, max_calls_per_attempt: execution.base_calls + execution.calls_per_input * inputs.length, max_cost_micros_per_attempt: 0, timeout_seconds: Math.min(30, execution.max_timeout_seconds) }
  }
  const source = { asset: { asset_id: asset.asset_id, version: asset.version }, kind: 'raw_asset' }
  const nodes = shape === 'chain' ? [
    node('quality', 'data-quality', [source], ['raw_asset', 'quality_report']),
    node('clean', 'data-cleaning', [{ from_node: 'quality', kind: 'raw_asset' }, { from_node: 'quality', kind: 'quality_report' }], ['cleaned_asset', 'quality_report'], ['quality']),
    node('enrich', 'data-enrichment', [{ from_node: 'clean', kind: 'cleaned_asset' }], ['enriched_asset'], ['clean']),
    // The current evaluation contract accepts quality reports, not enriched data.
    // Waiting for enrichment still verifies the declared four-node dependency DAG.
    node('evaluate', 'data-evaluation', [{ from_node: 'clean', kind: 'quality_report' }], ['evaluation'], ['clean', 'enrich']),
  ] : Array.from({ length: shape === 'parallel' ? 2 : 32 }, (_, index) => node(`quality-${index + 1}`, 'data-quality', [source], ['raw_asset', 'quality_report']))
  const budget = { max_nodes: nodes.length, max_concurrency: shape === 'parallel' ? 2 : 1, max_attempts: nodes.length, max_calls: nodes.reduce((sum, node) => sum + node.max_calls_per_attempt, 0), max_cost_micros: 0, timeout_seconds: nodes.reduce((sum, node) => sum + node.timeout_seconds, 0) }
  const plan = { schema_version: 1, budget, nodes }
  const task = await postData<Task>(api, '/governed-tasks', { goal: marker, input_refs: [{ asset_id: asset.asset_id, version: asset.version }], budget }, true)
  await postData(api, `/governed-tasks/${task.id}/plans`, { expected_version: task.version, plan })
  const saved = await getData<Task>(api, `/governed-tasks/${task.id}`)
  expect(saved.current_plan_version).toBe(1); expect(saved.execution_plan_version || 0).toBe(0)
  const readiness = await getData<Readiness>(api, `/governed-tasks/${task.id}/execution-readiness`)
  expect(readiness).toMatchObject({ task_id: saved.id, task_version: saved.version, plan_version: 1, execution_enabled: true, allowed: true, blockers: [] })
  await page.goto(`/tasks?task_id=${encodeURIComponent(task.id)}`)
  await expect(page.getByRole('button', { name: '启动任务', exact: true })).toBeVisible()
  return { task: saved, plan, asset, readiness }
}
async function startThroughUI(page: Page, task: Task) {
  const observed = page.waitForResponse(response => new URL(response.url()).pathname === `/api/v1/governed-tasks/${task.id}/start` && response.request().method() === 'POST')
  await page.getByRole('button', { name: '启动任务', exact: true }).click()
  const response = await observed
  expect(response.ok()).toBe(true)
  expect(response.request().postDataJSON()).toEqual({ expected_version: task.version, plan_version: task.current_plan_version })
  expect(response.request().headers()['idempotency-key']).toBeTruthy()
  const started = (await response.json()).data as Task
  expect(started.execution_plan_version).toBe(1)
  return started
}
async function waitForTerminal(api: APIRequestContext, id: string): Promise<Summary> {
  let latest!: Summary
  await expect.poll(async () => {
    const response = await api.get(`/api/v1/governed-tasks/${id}/supervisor/summary`)
    if (response.status() === 409) return false // A concurrent controller revision requires another real read.
    expect(response.status()).toBe(200); latest = (await response.json()).data as Summary
    return latest.terminal
  }, { timeout: 120_000, intervals: [250, 500, 1000, 2000], message: `task ${id} reaches an authoritative terminal state` }).toBe(true)
  return latest
}
async function cleanupRunning(api: APIRequestContext, id: string | undefined) {
  if (!id) return
  // Cancel only a task created by this test; do not stop shared services or erase evidence.
  const response = await api.get(`/api/v1/governed-tasks/${id}`)
  if (!response.ok()) return
  const current = (await response.json()).data as Task
  if (['running', 'awaiting_approval', 'partially_completed', 'reconcile_required'].includes(current.status)) await api.post(`/api/v1/governed-tasks/${id}/cancel`, { data: { expected_version: current.version } })
}
async function attachExecution(info: TestInfo, api: APIRequestContext, fixture: { task: Task; asset: AssetRef; plan: unknown }, final: Summary) {
  const domains = []
  for (const child of final.children) {
    expect(child.domain_kind).toBe('invocation'); expect(child.domain_id).toBeTruthy()
    const detail = await getData<{ invocation: { id: string; status: string; created_at: string; updated_at: string }; approvals: unknown[] }>(api, `/lifecycle/invocations/${child.domain_id}`)
    expect(detail.invocation.id).toBe(child.domain_id); expect(detail.invocation.status).toBe(child.domain_status)
    domains.push(detail)
  }
  await attachJSON(info, 'actual-runtime-evidence', { fixture, final, domains })
  return domains
}
async function screenshotExecution(page: Page, info: TestInfo, filename: string) {
  // The workspace scrolls inside its own container. Keep the heading below
  // the sticky page bar and capture the real viewport without obscuring it.
  await page.locator('.execution-panel').evaluate(element => {
    element.scrollIntoView({ block: 'start' })
    for (let parent = element.parentElement; parent; parent = parent.parentElement) {
      if (parent.scrollHeight > parent.clientHeight && /auto|scroll/.test(getComputedStyle(parent).overflowY)) {
        parent.scrollTop = Math.max(0, parent.scrollTop - 120)
        break
      }
    }
  })
  await page.screenshot({ path: info.outputPath(filename), fullPage: true })
}

test.describe('real governed task execution', () => {
  test.beforeEach(() => { test.skip(process.env.COGNIDA_E2E_TASK_EXECUTION !== '1', 'Requires explicit isolated execution-controller acceptance') })
  test('four real expert nodes → terminal results → reload recovery', async ({ page, playwright }, info) => {
    const api = await loginThroughUI(page, playwright); let taskID: string | undefined
    try {
      const fixture = await createFixture(page, api, 'chain'); taskID = fixture.task.id
      const before = await getData<Summary>(api, `/governed-tasks/${taskID}/supervisor/summary`)
      expect(before.children).toEqual([])
      await startThroughUI(page, fixture.task)
      const writes: string[] = []
      page.on('request', request => { if (['POST', 'PUT', 'PATCH', 'DELETE'].includes(request.method()) && request.url().includes('/governed-tasks')) writes.push(request.url()) })
      await page.reload(); await expect(page.getByRole('heading', { name: '当前状态汇总', exact: true })).toBeVisible()
      const final = await waitForTerminal(api, taskID)
      expect(final.status).toBe('completed'); expect(final.nodes).toHaveLength(4); expect(final.nodes.every(node => node.status === 'succeeded')).toBe(true)
      expect(final.children).toHaveLength(4); expect(new Set(final.children.map(child => child.domain_id)).size).toBe(4)
      expect(final.nodes.map(node => node.capability).sort()).toEqual(['lifecycle.data-cleaning', 'lifecycle.data-enrichment', 'lifecycle.data-evaluation', 'lifecycle.data-quality'])
      const outputs = new Map<string, { ref: AssetRef; content: unknown }>()
      for (const node of final.nodes) {
        const kinds: string[] = []
        for (const ref of node.output_refs) {
          const key = `${ref.asset_id}:${ref.version}`
          if (!outputs.has(key)) outputs.set(key, await getData(api, `/lifecycle/assets/${ref.asset_id}?version=${ref.version}`))
          const actual = outputs.get(key)!
          expect(actual.content, `${node.node_id} output ${key} has actual stored content`).toBeDefined()
          kinds.push(actual.ref.kind)
        }
        expect(kinds).toEqual(expect.arrayContaining(fixture.plan.nodes.find(item => item.id === node.node_id)!.output_kinds))
      }
      await attachJSON(info, 'actual-runtime-output-assets', [...outputs.values()])
      expect(final.nodes.every(node => node.attempt === 1)).toBe(true)
      await expect(page.locator('.execution-panel .node-progress')).toContainText('已成功 4 / 4')
      await expect(page.locator('.execution-panel > .execution-heading .wb-badge')).toHaveText('已完成')
      expect(writes, 'reload and polling must never resubmit task commands').toEqual([])
      await attachExecution(info, api, fixture, final)
      await screenshotExecution(page, info, 'runtime-four-node-results.png')
      await page.setViewportSize({ width: 390, height: 844 })
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
      await screenshotExecution(page, info, 'runtime-mobile-results.png')
      const first = final.nodes.find(node => node.node_id === 'quality')!
      await page.locator('.runtime-nodes').getByRole('link', { name: '查看领域任务 ↗', exact: true }).first().click()
      await expect(page).toHaveURL(new RegExp(`/experts\\?invocation_id=${first.domain_id}`))
      await expect(page.getByRole('heading', { name: '执行详情', exact: true })).toBeVisible()
    } finally { await cleanupRunning(api, taskID); await api.dispose() }
  })
  test('two independent quality roots dispatch under an explicit concurrency budget', async ({ page, playwright }, info) => {
    const api = await loginThroughUI(page, playwright); let taskID: string | undefined
    try {
      const fixture = await createFixture(page, api, 'parallel'); taskID = fixture.task.id
      expect(fixture.plan.budget.max_concurrency).toBe(2); expect(fixture.plan.nodes.every(node => node.depends_on.length === 0)).toBe(true)
      await startThroughUI(page, fixture.task)
      const final = await waitForTerminal(api, taskID)
      expect(final.status).toBe('completed'); expect(final.nodes).toHaveLength(2); expect(final.children).toHaveLength(2)
      expect(final.nodes.every(node => node.status === 'succeeded' && node.attempt === 1)).toBe(true)
      const domains = await attachExecution(info, api, fixture, final)
      const starts = domains.map(detail => Date.parse(detail.invocation.created_at)), ends = domains.map(detail => Date.parse(detail.invocation.updated_at))
      expect(Math.max(...starts), 'both domain tasks must have been created before the first terminal completion').toBeLessThan(Math.min(...ends))
      await expect(page.locator('.execution-panel .node-progress')).toContainText('已成功 2 / 2')
      await screenshotExecution(page, info, 'runtime-parallel-roots.png')
    } finally { await cleanupRunning(api, taskID); await api.dispose() }
  })
  test('cancel request remains distinct from the confirmed cancellation outcome', async ({ page, playwright }, info) => {
    const api = await loginThroughUI(page, playwright); let taskID: string | undefined
    try {
      const fixture = await createFixture(page, api, 'cancel'); taskID = fixture.task.id
      await startThroughUI(page, fixture.task)
      await expect.poll(async () => {
        const response = await api.get(`/api/v1/governed-tasks/${taskID}/supervisor/summary`)
        if (response.status() === 409) return 0
        expect(response.status()).toBe(200)
        return ((await response.json()).data as Summary).children.length
      }, { timeout: 20_000, intervals: [100, 250, 500] }).toBeGreaterThan(0)
      let cancellation!: Task
      for (let attempt = 0; attempt < 3; attempt++) {
        const responsePromise = page.waitForResponse(response => new URL(response.url()).pathname === `/api/v1/governed-tasks/${taskID}/cancel` && response.request().method() === 'POST')
        await page.getByRole('button', { name: '申请取消', exact: true }).click()
        const response = await responsePromise
        if (response.status() === 409 && attempt < 2) {
          // A worker may advance the task version before the click. Recover via
          // the visible read action, then make another explicit UI submission.
          await page.getByRole('button', { name: '重新核对执行状态', exact: true }).click()
          continue
        }
        expect(response.ok()).toBe(true); cancellation = (await response.json()).data as Task; break
      }
      expect(['cancel_requested', 'cancelled', 'partially_completed']).toContain(cancellation.status)
      const final = await waitForTerminal(api, taskID)
      const succeeded = final.nodes.filter(node => node.status === 'succeeded')
      expect(final.status).toBe(succeeded.length ? 'partially_completed' : 'cancelled'); expect(final.can_cancel).toBe(false)
      const retained = new Set(final.result_refs.map(ref => `${ref.asset_id}:${ref.version}`))
      for (const node of succeeded) {
        expect(node.output_refs.length).toBeGreaterThan(0)
        for (const ref of node.output_refs) expect(retained.has(`${ref.asset_id}:${ref.version}`), 'cancellation must retain confirmed successful outputs').toBe(true)
      }
      const terminalLabel = succeeded.length ? '部分完成' : '已取消'
      await expect(page.locator('.execution-panel > .execution-heading .wb-badge')).toHaveText(terminalLabel)
      await expect(page.getByRole('button', { name: '申请取消', exact: true })).toHaveCount(0)
      await page.reload(); await expect(page.locator('.execution-panel > .execution-heading .wb-badge')).toHaveText(terminalLabel)
      await attachJSON(info, 'actual-cancellation-evidence', { fixture, cancellation, final })
      await screenshotExecution(page, info, 'runtime-cancellation-terminal.png')
    } finally { await cleanupRunning(api, taskID); await api.dispose() }
  })
})
