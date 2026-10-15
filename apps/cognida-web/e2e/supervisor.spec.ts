import { test, expect } from '@playwright/test'
import { attachJSON, createInput, getData, loginThroughUI } from './lifecycle.helpers'

// This opt-in scenario spends one real model request and persists an isolated
// draft/plan. It does not intercept requests or invoke domain executors.
test('supervisor: real source → model candidate → explicit save → refresh', async ({ page, playwright }, info) => {
  test.skip(process.env.COGNIDA_E2E_SUPERVISOR !== '1', 'Explicit real-model Supervisor acceptance is required')
  const api = await loginThroughUI(page, playwright)
  try {
    const marker = `supervisor-ui-${Date.now()}`
    const asset = await createInput(page, {
      id: marker, source: 'text',
      text: 'Synthetic planning sample. North: 100 units. South: 200 units. North: 100 units. This sample is only used to plan quality checks and cleaning; it must not be published.',
    }, marker)
    await page.goto('/tasks')
    await expect(page.getByRole('heading', { name: '先明确目标，再审阅计划', exact: true })).toBeVisible()
    await page.getByLabel('任务目标', { exact: true }).fill('按顺序先调用 data-quality 对输入做质检，再调用 data-cleaning 清洗，保留 cleaned_asset 和 quality_report 候选。使用现有受控算子的默认规则，无需自定义代码，不发布、不分析。只生成两个节点的计划，各尝试一次；清洗同时绑定质检返回的原始数据和质量报告。')
    await page.getByLabel('可选资产', { exact: true }).selectOption(`${asset.asset_id}:${asset.version}`)
    await page.getByRole('button', { name: '添加输入', exact: true }).click()
    const proposed = page.waitForResponse(response => response.url().includes('/supervisor/propose') && response.request().method() === 'POST', { timeout: 55_000 })
    await page.getByRole('button', { name: '核对并生成候选计划', exact: true }).click()
    const response = await proposed
    expect(response.status(), 'actual Supervisor model/API response').toBe(200)
    const candidate = (await response.json()).data
    expect(candidate.decision.kind).toBe('proposal')
    expect(candidate.execution_enabled).toBe(false)
    expect(candidate.saved).toBe(false)
    expect(candidate.validation.valid).toBe(true)
    expect(candidate.decision.plan.nodes.map((node: { capability: string }) => node.capability).sort()).toEqual(['lifecycle.data-cleaning', 'lifecycle.data-quality'])
    const taskID = candidate.task_id as string
    type Summary = { task_id: string; task_version: number; status: string; current_plan_version: number; children: unknown[]; execution_enabled: boolean }
    const preview = await getData<Summary>(api, `/governed-tasks/${taskID}/supervisor/summary`)
    expect(preview.current_plan_version).toBe(0)
    expect(preview.children).toEqual([])
    expect(preview.status).toBe('draft')
    await expect(page.getByRole('heading', { name: '候选计划', exact: true })).toBeVisible()
    const discardDialog = page.waitForEvent('dialog').then(async dialog => {
      expect(dialog.message()).toContain('未保存')
      await dialog.dismiss()
    })
    await page.getByRole('link', { name: '打开此任务链接 ↗', exact: true }).click()
    await discardDialog
    await expect(page.getByRole('heading', { name: '候选计划', exact: true })).toBeVisible()
    await page.getByRole('heading', { name: '候选计划', exact: true }).scrollIntoViewIfNeeded()
    await page.screenshot({ path: info.outputPath('supervisor-candidate.png'), fullPage: true, animations: 'disabled' })
    const savedResponse = page.waitForResponse(response => new URL(response.url()).pathname === `/api/v1/governed-tasks/${taskID}/plans` && response.request().method() === 'POST')
    await page.getByRole('button', { name: '保存计划', exact: true }).click()
    expect((await savedResponse).ok()).toBe(true)
    await expect(page.getByRole('heading', { name: '已保存计划 v1', exact: true })).toBeVisible()
    await expect(page).toHaveURL(new RegExp(`task_id=${taskID}`))
    const writes: string[] = []
    const observe = (request: import('@playwright/test').Request) => {
      if (['POST', 'PUT', 'PATCH', 'DELETE'].includes(request.method()) && request.url().includes('/governed-tasks')) writes.push(request.url())
    }
    page.on('request', observe)
    await page.reload()
    await expect(page.getByRole('heading', { name: '已保存计划 v1', exact: true })).toBeVisible()
    await expect(page.getByRole('heading', { name: '当前状态汇总', exact: true })).toBeVisible()
    expect(writes, 'refresh must not regenerate or save another plan').toEqual([])
    page.off('request', observe)
    const final = await getData<Summary>(api, `/governed-tasks/${taskID}/supervisor/summary`)
    expect(final.current_plan_version).toBe(1)
    expect(final.children).toEqual([])
    expect(['draft', 'ready']).toContain(final.status)
    const capabilities = await getData<{ execution_enabled: boolean }>(api, '/governed-tasks/capabilities')
    expect(final.execution_enabled).toBe(capabilities.execution_enabled)
    const stored = await getData<{ execution_plan_version?: number; started_at?: string }>(api, `/governed-tasks/${taskID}`)
    expect(stored.execution_plan_version || 0, 'saving must not bind an execution plan').toBe(0)
    expect(stored.started_at, 'saving must not start domain execution').toBeFalsy()
    await attachJSON(info, 'supervisor-actual-plan', { asset, candidate, preview, final, stored })
    await expect(page.locator('.platform-container')).toHaveCSS('opacity', '1')
    await expect(page.locator('body')).not.toHaveClass(/page-transitioning/)
    await page.getByRole('heading', { name: '已保存计划 v1', exact: true }).scrollIntoViewIfNeeded()
    await page.screenshot({ path: info.outputPath('supervisor-saved-desktop.png'), fullPage: true, animations: 'disabled' })
    await page.setViewportSize({ width: 390, height: 844 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await page.getByRole('heading', { name: '已保存计划 v1', exact: true }).scrollIntoViewIfNeeded()
    await page.screenshot({ path: info.outputPath('supervisor-saved-mobile.png'), fullPage: true, animations: 'disabled' })
  } finally { await api.dispose() }
})
