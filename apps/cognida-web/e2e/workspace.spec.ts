import { test, expect } from '@playwright/test'
import { attachJSON, getData, loginThroughUI } from './lifecycle.helpers'

test('workspace: real counts → run detail deep link → report', async ({ page, playwright }, info) => {
  const api = await loginThroughUI(page, playwright)
  try {
    await expect(page.getByRole('heading', { name: '工作总览', exact: true })).toBeVisible()
    const runs = await getData<{ items: Array<{ id: string; status: string }> }>(api, '/lifecycle/runs?page_size=50')
    const assets = await getData<{ items: unknown[] }>(api, '/lifecycle/assets?kind=published_asset&page_size=20')
    await expect(page.locator('[data-section="runs"] .source-status__count')).toHaveText(`${runs.items.length}条已读取`)
    await expect(page.locator('[data-section="assets"] .source-status__count')).toHaveText(`${assets.items.length}条已读取`)
    await expect(page.locator('.source-status [role="alert"]')).toHaveCount(0)
    await attachJSON(info, 'overview-real-counts', { runs: runs.items.length, assets: assets.items.length })
    await page.screenshot({ path: info.outputPath('workspace-desktop.png'), fullPage: true })
    const runLink = page.getByRole('link', { name: /^查看处理流程 / }).first()
    expect(runs.items.length, 'run lifecycle scenarios before the workspace acceptance').toBeGreaterThan(0)
    {
      await expect(runLink).toBeVisible()
      const href = await runLink.getAttribute('href')
      expect(href).toMatch(/^\/pipelines\?run_id=/)
      const writes: string[] = []
      const observe = (request: import('@playwright/test').Request) => { if (request.method() === 'POST' && /\/lifecycle\//.test(request.url())) writes.push(request.url()) }
      page.on('request', observe)
      await runLink.click()
      await expect(page).toHaveURL(new RegExp(`run_id=${new URL(href!, 'http://localhost').searchParams.get('run_id')}`))
      await expect(page.locator('.lifecycle-workbench .execution-guide')).toBeVisible()
      await expect(page.locator('.pipeline-stage')).toHaveCount(7)
      expect(writes, 'opening a run from overview must not submit or restart it').toEqual([])
      page.off('request', observe)
      await page.getByRole('button', { name: '查看分析报告', exact: true }).click()
      await expect(page.getByRole('heading', { name: '分析报告', exact: true })).toBeVisible()
      await expect(page.getByRole('heading', { name: '分析报告', exact: true })).toBeInViewport()
      await expect(page.locator('.asset-result')).toBeFocused()
      await page.screenshot({ path: info.outputPath('pipeline-deep-link.png'), fullPage: true })
    }
  } finally { await api.dispose() }
})

test('workspace: mobile navigation and collection dialog remain keyboard accessible', async ({ page, playwright }, info) => {
  await page.setViewportSize({ width: 390, height: 844 })
  const api = await loginThroughUI(page, playwright)
  try {
    await expect(page.locator('[data-section="collection"] .source-status__count')).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await page.screenshot({ path: info.outputPath('workspace-mobile.png'), fullPage: true })
    const toggle = page.locator('.platform-mobile-bar button')
    await toggle.click()
    await expect(toggle).toHaveAttribute('aria-expanded', 'true')
    await expect(page.locator('.rail__mobile-close')).toBeFocused()
    await page.keyboard.press('Escape')
    await expect(toggle).toBeFocused()
    await expect(toggle).toHaveAttribute('aria-expanded', 'false')
    await toggle.click()
    await page.getByRole('navigation', { name: '主导航' }).getByRole('link', { name: '网页采集', exact: true }).click()
    await expect(page).toHaveURL(/\/collection-tasks$/)
    const create = page.getByRole('button', { name: '创建采集任务', exact: true })
    await create.click()
    const dialog = page.getByRole('dialog', { name: '创建网页采集任务' })
    await expect(dialog).toBeVisible()
    await expect(dialog).toBeFocused()
    await page.keyboard.press('Shift+Tab')
    expect(await dialog.evaluate(element => element.contains(document.activeElement))).toBe(true)
    await page.keyboard.press('Tab')
    await expect(dialog.getByRole('button', { name: '关闭对话框' })).toBeFocused()
    await page.screenshot({ path: info.outputPath('collection-create-mobile.png'), fullPage: true })
    await page.keyboard.press('Escape')
    await expect(dialog).toHaveCount(0)
    await expect(create).toBeFocused()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  } finally { await api.dispose() }
})

test('workspace: run a real quality expert and reopen its result by deep link', async ({ page, playwright }, info) => {
  const api = await loginThroughUI(page, playwright)
  try {
    const assets = await getData<{ items: Array<{ asset_id: string; version: number }> }>(api, '/lifecycle/assets?kind=published_asset&page_size=20')
    expect(assets.items.length, 'published lifecycle assets are required').toBeGreaterThan(0)
    const asset = assets.items[0]!
    await page.goto('/experts')
    await page.getByLabel(/^专家/).selectOption('data-quality')
    await page.getByLabel('仅显示已发布资产', { exact: true }).check()
    await page.getByLabel(/^输入资产/).selectOption(`${asset.asset_id}:${asset.version}`)
    const startedResponse = page.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/lifecycle/invocations' && response.request().method() === 'POST')
    await page.getByRole('button', { name: '开始执行', exact: true }).click()
    const response = await startedResponse
    expect(response.ok()).toBe(true)
    const started = (await response.json()).data
    expect(started.id).toBeTruthy()
    type ExpertResult = { invocation: { id: string; status: string; output?: { refs: Array<{ kind: string }> } } }
    let completed!: ExpertResult
    await expect.poll(async () => {
      completed = await getData<ExpertResult>(api, `/lifecycle/invocations/${started.id}`)
      return completed.invocation.status
    }, { timeout: 45_000 }).toBe('succeeded')
    expect(completed.invocation.output?.refs.some(ref => ref.kind === 'quality_report')).toBe(true)
    await attachJSON(info, 'completed-quality-expert', completed)
    await page.goto(`/experts?invocation_id=${encodeURIComponent(started.id)}`)
    await expect(page.locator('.execution-guide')).toBeVisible()
    await expect(page.locator('.lifecycle-workbench')).toContainText(started.id)
    await expect(page.getByRole('button', { name: '批准并继续', exact: true })).toHaveCount(0)
    await expect(page.getByRole('button', { name: '重试失败任务', exact: true })).toHaveCount(0)
    await page.screenshot({ path: info.outputPath('expert-result-deep-link.png'), fullPage: true })
  } finally { await api.dispose() }
})
