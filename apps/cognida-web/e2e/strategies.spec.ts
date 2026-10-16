import { randomUUID } from 'node:crypto'
import { test, expect, type APIRequestContext, type Page } from '@playwright/test'
import { attachJSON, createInput, getData, loginThroughUI } from './lifecycle.helpers'

const slot = '/strategies/orchestration.default'
async function post(api: APIRequestContext, path: string, data: unknown) {
  const response = await api.post(`/api/v1${path}`, { data, headers: { 'Idempotency-Key': randomUUID() } })
  expect(response.ok(), `${path}: ${response.status()} ${await response.text()}`).toBe(true)
  return (await response.json()).data
}
async function clickWrite(page: Page, path: string, name: string) {
  const response = page.waitForResponse(r => new URL(r.url()).pathname === `/api/v1${path}` && r.request().method() === 'POST')
  await page.getByRole('button', { name, exact: true }).click()
  const result = await response
  expect(result.ok(), `${path}: ${result.status()} ${await result.text()}`).toBe(true)
  await expect(page.getByRole('button', { name: '刷新策略', exact: true })).toBeEnabled()
  return (await result.json()).data
}
async function panel(page: Page) {
  await page.goto('/suggestions')
  await page.getByRole('combobox', { name: '策略消费者', exact: true }).selectOption('orchestration')
  await expect(page.getByRole('button', { name: '刷新策略', exact: true })).toBeEnabled()
}
async function selectVersion(page: Page, id: string) {
  const version = page.locator('button.version').filter({ hasText: id })
  for (let count = 0; count < 100; count++) {
    if (await version.count()) {
      await version.click()
      return
    }
    const more = page.getByRole('button', { name: '更多版本', exact: true })
    await expect(more, `version ${id} must be discoverable through actual cursor pages`).toBeVisible()
    await more.click()
    await expect(page.getByRole('button', { name: '刷新策略', exact: true })).toBeEnabled()
  }
  throw new Error(`Version ${id} was not found within 100 real cursor pages`)
}
async function start(page: Page, api: APIRequestContext, id: string) {
  await page.goto(`/tasks?task_id=${id}`)
  const response = page.waitForResponse(r => new URL(r.url()).pathname === `/api/v1/governed-tasks/${id}/start` && r.request().method() === 'POST')
  await page.getByRole('button', { name: '启动任务', exact: true }).click()
  expect((await response).ok()).toBe(true)
  let summary: any
  await expect.poll(async () => {
    const r = await api.get(`/api/v1/governed-tasks/${id}/supervisor/summary`)
    if (r.status() === 409) return false
    expect(r.ok()).toBe(true); summary = (await r.json()).data
    return summary.terminal
  }, { timeout: 150_000 }).toBe(true)
  expect(summary.status, JSON.stringify(summary)).toBe('completed')
  expect(summary.children).toHaveLength(1)
  expect(summary.children[0].domain_kind).toBe('invocation')
  const domain = await getData<any>(api, `/lifecycle/invocations/${summary.children[0].domain_id}`)
  expect(domain.invocation.status).toBe('succeeded')
  return { summary, domain }
}

test('real fixed parent trials → regression gates → explicit approval, activation and rollback', async ({ page, playwright }, info) => {
  test.skip(process.env.COGNIDA_E2E_STRATEGIES !== '1', 'Requires isolated strategy workers and real Python lifecycle compute')
  const api = await loginThroughUI(page, playwright)
  const created: string[] = []
  try {
    const asset = await createInput(page, { id: 'policy-evidence', source: 'text', text: 'Synthetic policy trial evidence. North revenue is 100 units and South revenue is 200 units. This text is used only for deterministic quality checks. No external publication or model call is requested.' }, `strategy-${randomUUID()}`)
    await panel(page)
    const initial = await api.get(`/api/v1${slot}`)
    if (initial.status() === 404) await clickWrite(page, `${slot}/baseline`, '登记服务端现有默认基线')
    else expect(initial.ok()).toBe(true)
    const head = await getData<any>(api, slot)
    const base = await getData<any>(api, `${slot}/versions/${head.active_id}`)
    // A behavior-preserving candidate proves the full approval path without
    // inventing model quality improvements or changing the fixed regression gate.
    await page.getByLabel('来源证据资产引用（每行一个）', { exact: true }).fill(`${asset.asset_id}:${asset.version}`)
    await page.getByLabel('受限策略内容 JSON', { exact: true }).fill('{"max_parallel":2}')
    const candidate = await clickWrite(page, `${slot}/versions`, '保存候选，不激活')
    expect((await getData<any>(api, slot)).active_id).toBe(base.id)
    const premature = await api.post(`/api/v1${slot}/versions/${candidate.id}/activate`, { data: { digest: candidate.digest, expected_revision: head.revision, reason: 'must reject missing review', approval_id: '', rollback: false }, headers: { 'Idempotency-Key': randomUUID() } })
    expect([404, 409]).toContain(premature.status())

    const caps = await getData<any>(api, '/governed-tasks/capabilities')
    const cap = caps.capabilities.find((c: any) => c.id === 'lifecycle.data-quality')
    expect(cap.execution).toBeTruthy()
    const budget = { max_nodes: 2, max_concurrency: 2, max_attempts: 1, max_calls: cap.execution.base_calls + cap.execution.calls_per_input, max_cost_micros: 0, timeout_seconds: 180 }
    const plan = { schema_version: 1, budget, nodes: [{ id: 'quality', capability: cap.id, capability_version: cap.version, depends_on: [], inputs: [{ asset: { asset_id: asset.asset_id, version: asset.version }, kind: 'raw_asset' }], output_kinds: ['raw_asset', 'quality_report'], tools: cap.tools, permissions: cap.permissions, max_attempts: 1, max_calls_per_attempt: budget.max_calls, max_cost_micros_per_attempt: 0, timeout_seconds: 30 }] }
    const results: any[] = []
    for (const version of [base, candidate]) {
      const task = await post(api, '/governed-tasks', { goal: 'Compare one fixed synthetic quality task', input_refs: [{ asset_id: asset.asset_id, version: asset.version }], budget })
      created.push(task.id)
      await post(api, `/governed-tasks/${task.id}/plans`, { expected_version: task.version, plan })
      await panel(page); await selectVersion(page, version.id)
      await page.getByLabel('待启动的固定 DAG 任务 ID', { exact: true }).fill(task.id)
      const trial = await clickWrite(page, `${slot}/versions/${version.id}/trials`, '固定到隔离编排试验，不启动')
      expect(trial.execution_started).toBe(false)
      expect((await getData<any>(api, `/governed-tasks/${task.id}/supervisor/summary`)).children).toEqual([])
      results.push(await start(page, api, task.id))
    }
    await panel(page); await selectVersion(page, candidate.id)
    await page.getByLabel('已完成基线评测 ID', { exact: true }).fill(created[0])
    await page.getByLabel('已完成候选评测 ID', { exact: true }).fill(created[1])
    const regression = await clickWrite(page, `${slot}/versions/${candidate.id}/regressions`, '核对真实回归结果')
    await attachJSON(info, 'strategy-actual-pair-before-approval', { base, candidate, plan, results, regression })
    expect(regression.payload.evidence.quality_metric).toBe('successful_node_fraction')
    expect(regression.payload.evidence.cost_metric).toBe('parent_wall_time_ms_not_billing')
    expect(regression.payload.passed, JSON.stringify(regression.payload)).toBe(true)
    await page.getByRole('combobox', { name: '回归记录', exact: true }).selectOption(regression.id)
    await page.getByLabel('审核／切换原因', { exact: true }).fill('Review actual identical synthetic task pair; retain permission and scope gates.')
    const approval = await clickWrite(page, `${slot}/versions/${candidate.id}/approvals`, '批准当前确切摘要')
    expect(approval.payload.digest).toBe(candidate.digest)
    await page.getByRole('combobox', { name: '批准回执', exact: true }).selectOption(approval.id)
    await clickWrite(page, `${slot}/versions/${candidate.id}/activate`, '确认激活此版本')
    expect((await getData<any>(api, slot)).active_id).toBe(candidate.id)
    await page.reload(); await page.getByRole('combobox', { name: '策略消费者', exact: true }).selectOption('orchestration')
    await expect(page.getByRole('region', { name: '受治理策略版本' })).toContainText(candidate.id)
    await selectVersion(page, base.id)
    await page.getByLabel('审核／切换原因', { exact: true }).fill('Restore the previous approved baseline after isolated acceptance.')
    await clickWrite(page, `${slot}/versions/${base.id}/activate`, '确认回退到此历史版本')
    const restored = await getData<any>(api, slot)
    expect(restored.active_id).toBe(base.id)
    const pins: any[] = [], cursors = new Set<string>()
    let cursor = ''
    for (let page = 0; page < 100; page++) {
      const events = await getData<any>(api, `${slot}/events?kind=pin&page_size=100&cursor=${encodeURIComponent(cursor)}`)
      pins.push(...(events.items || []))
      if (pins.some((e: any) => e.version_id === candidate.id && e.payload.effective_body.max_parallel === 2)) break
      const next = events.next_cursor || ''
      if (!next) break
      expect(cursors.has(next), 'pin pagination must advance').toBe(false)
      cursors.add(next); cursor = next
    }
    expect(pins.some((e: any) => e.version_id === candidate.id && e.payload.effective_body.max_parallel === 2)).toBe(true)
    await attachJSON(info, 'strategy-real-approval-and-rollback', { approval, restored, pins })
    await page.screenshot({ path: info.outputPath('strategies-actual-regression-and-rollback.png'), fullPage: true })
  } finally {
    for (const id of created) {
      const r = await api.get(`/api/v1/governed-tasks/${id}`)
      if (r.ok()) { const t = (await r.json()).data; if (['running', 'reconcile_required'].includes(t.status)) await api.post(`/api/v1/governed-tasks/${id}/cancel`, { data: { expected_version: t.version } }) }
    }
    await api.dispose()
  }
})
