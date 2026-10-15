import { test, expect } from '@playwright/test'
import { attachJSON, getData, loadScenarios, loginThroughUI } from './lifecycle.helpers'

interface CollectionTask { id: string; knowledge_base_id: string; status: string; error_message?: string }
interface CollectionDetail {
  task: CollectionTask
  run?: { id: string; status: string }
  sources: Array<{ id: string; url: string }>
  items?: Array<{ id: string; run_id: string; source_url: string; canonical_url?: string; status: string; knowledge_id?: string; normalized_object_key?: string; error_reason?: string }>
  summary?: { outcome: string; total: number; usable: number; failed: number; rejected: number }
}
const sourceURL = process.env.COGNIDA_E2E_COLLECTION_URL
const existingTaskID = process.env.COGNIDA_E2E_COLLECTION_TASK_ID
const knowledgeBaseID = process.env.COGNIDA_E2E_KNOWLEDGE_BASE_ID || loadScenarios()[0]?.target_knowledge_base_id

test('collection UI: inspect actual source outcome → open pipeline draft', async ({ page, playwright }, info) => {
  test.skip(!sourceURL && !existingTaskID, 'Set COGNIDA_E2E_COLLECTION_TASK_ID for an existing task, or COGNIDA_E2E_COLLECTION_URL to create a real task.')
  if (!existingTaskID && (!knowledgeBaseID || knowledgeBaseID.startsWith('REPLACE_'))) throw new Error('Collection creation requires an explicit authorized target knowledge base')
  if (!existingTaskID) {
    const url = new URL(sourceURL!)
    if (url.protocol !== 'https:' || url.username || url.password) throw new Error('Collection fixture URL must be HTTPS without credentials')
  }
  const api = await loginThroughUI(page, playwright)
  let taskID = ''
  try {
    const detail = page.locator('.ui-modal').filter({ has: page.locator('.ui-modal__title', { hasText: '采集任务详情' }) })
    if (existingTaskID) {
      taskID = existingTaskID
      await page.goto('/collection-tasks')
      await page.getByRole('row').filter({ hasText: taskID.slice(0, 8) }).getByRole('button', { name: '详情', exact: true }).click()
      await expect(detail).toContainText(taskID)
    } else {
      const kb = await getData<{ id: string; name: string }>(api, `/knowledge-bases/${encodeURIComponent(knowledgeBaseID!)}`)
      const knowledgeList = page.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/knowledge-bases' && response.request().method() === 'GET')
      await page.goto('/collection-tasks')
      await knowledgeList
      await page.getByRole('button', { name: '创建采集任务', exact: true }).click()
      const create = page.locator('.ui-modal').filter({ has: page.locator('.ui-modal__title', { hasText: '创建网页采集任务' }) })
      const escapedName = kb.name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
      const option = page.locator('.ui-select__option').filter({ hasText: new RegExp(`^\\s*${escapedName}\\s*$`) })
      for (;;) {
        await create.locator('.ui-select__trigger').click()
        if (await option.count()) { await option.click(); break }
        await create.locator('.ui-modal__title').click()
        const more = create.getByRole('button', { name: '加载更多知识库', exact: true })
        await expect(more, 'target knowledge base must be present in authorized pages').toBeVisible()
        await more.click()
        // Wait for the true page request; the list may remove its final "load more" button.
        await expect.poll(async () => {
          const loading = await more.count() ? await more.isDisabled() : false
          return !loading
        }).toBe(true)
      }
      await create.getByRole('textbox').fill(sourceURL!)
      const createdResponse = page.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/collection-tasks' && response.request().method() === 'POST')
      await create.getByRole('button', { name: '创建', exact: true }).click()
      const response = await createdResponse
      expect(response.ok(), 'real UI collection creation').toBe(true)
      const task: CollectionTask = (await response.json()).data
      taskID = task.id
      expect(task.knowledge_base_id).toBe(knowledgeBaseID)
      expect(task.status).toBe('draft')
      await attachJSON(info, 'created-collection-task', task)
      await expect(detail).toBeVisible()
      await expect(detail).toContainText(taskID)
      await expect(detail).toContainText(sourceURL!)
      const startResponse = page.waitForResponse(response => new URL(response.url()).pathname === `/api/v1/collection-tasks/${taskID}/start` && response.request().method() === 'POST')
      await detail.getByRole('button', { name: '开始采集', exact: true }).click()
      expect((await startResponse).ok(), 'real UI collection start').toBe(true)
    }
    let outcome!: CollectionDetail
    await expect.poll(async () => {
      outcome = await getData<CollectionDetail>(api, `/collection-tasks/${encodeURIComponent(taskID)}`)
      return ['completed', 'failed', 'cancelled'].includes(outcome.task.status)
    }, { timeout: Math.max(5000, Math.min(info.timeout - 30_000, 180_000)), intervals: [500, 1000, 2000], message: 'collection persists a terminal source outcome' }).toBe(true)
    await attachJSON(info, 'collection-outcome', outcome)
    expect(outcome.task.status, JSON.stringify(outcome.items?.map(item => ({ status: item.status, error: item.error_reason })))).toBe('completed')
    expect(outcome.summary?.outcome).toBe('success')
    expect(outcome.summary?.total).toBeGreaterThan(0)
    expect(outcome.summary?.usable).toBe(outcome.summary?.total)
    if (!existingTaskID) expect(outcome.summary?.total).toBe(1)
    expect(outcome.run?.id).toBeTruthy()
    const item = outcome.items?.find(item => ['accepted', 'skipped'].includes(item.status) && item.knowledge_id && item.normalized_object_key)
    expect(item, 'collection produced a durable usable item').toBeDefined()
    await detail.getByRole('button', { name: '刷新', exact: true }).click()
    await expect(detail).toContainText(outcome.run!.id)
    await expect(detail).toContainText(item!.id)
    await expect(detail).toContainText(item!.knowledge_id!)
    await page.screenshot({ path: info.outputPath('collection-source-outcome.png'), fullPage: true })
    // Observe real writes while following the source link. Opening the draft must not submit it.
    const automaticWrites: string[] = []
    const observeWrite = (request: import('@playwright/test').Request) => {
      if (request.method() === 'POST' && /^\/api\/v1\/lifecycle\/(assets|runs)$/.test(new URL(request.url()).pathname)) automaticWrites.push(request.url())
    }
    page.on('request', observeWrite)
    await detail.getByRole('row').filter({ hasText: item!.id }).getByRole('link', { name: '转入流水线', exact: true }).click()
    await expect(page).toHaveURL(/\/pipelines\?/)
    const location = new URL(page.url())
    expect(location.searchParams.get('source')).toBe('collection')
    expect(location.searchParams.get('run_id')).toBe(item!.run_id || outcome.run!.id)
    expect(location.searchParams.get('item_id')).toBe(item!.id)
    const input = page.locator('details.wb-panel').filter({ has: page.locator('summary', { hasText: '创建输入资产' }) })
    await expect(input).toHaveAttribute('open', '')
    await expect(input.getByLabel(/^数据类型/)).toHaveValue('collection')
    await expect(input.getByLabel('运行 / 评测任务 ID', { exact: true })).toHaveValue(item!.run_id || outcome.run!.id)
    await expect(input.getByLabel('采集条目 ID', { exact: true })).toHaveValue(item!.id)
    await expect(input.getByRole('button', { name: '创建资产', exact: true })).toBeEnabled()
    expect(automaticWrites, 'source navigation only prepares a draft').toEqual([])
    page.off('request', observeWrite)
    await page.screenshot({ path: info.outputPath('collection-pipeline-draft.png'), fullPage: true })
  } finally {
    if (taskID) {
      try { await attachJSON(info, 'last-collection-state', await getData<CollectionDetail>(api, `/collection-tasks/${encodeURIComponent(taskID)}`)) } catch { /* Keep the original failure. */ }
    }
    await api.dispose()
  }
})
