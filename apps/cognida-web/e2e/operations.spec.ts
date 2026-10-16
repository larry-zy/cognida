import { randomUUID } from 'node:crypto'
import { expect, test, type Page } from '@playwright/test'
import { attachJSON, createInput, getData, loginThroughUI } from './lifecycle.helpers'
type OperationJob = { id: string; status: string; committed_version?: number }
type OperationVersion = { version: number; table: { rows: { type: string; text: string }[][] } }
async function prepare(page: Page) {
  const pending = page.waitForResponse(r => new URL(r.url()).pathname === '/api/v1/operations/plans' && r.request().method() === 'POST')
  await page.getByRole('button', { name: '生成待确认计划', exact: true }).click()
  const response = await pending
  expect(response.ok(), await response.text()).toBe(true)
  const job = (await response.json()).data as OperationJob
  await expect(page).toHaveURL(new RegExp(`job_id=${job.id}`))
  await expect(page.getByRole('heading', { name: '计划与真实提交回执' })).toBeVisible()
  return job
}
test('governed ETL and SQL preserve exact values, require confirmation, retain and restore prior versions', async ({ page, playwright }, info) => {
  test.skip(process.env.COGNIDA_E2E_OPERATIONS !== '1', 'Requires the isolated operations worker environment')
  const api = await loginThroughUI(page, playwright)
  try {
    const input = await createInput(page, { id: 'operation-input', source: 'table', columns: ['code', 'amount', 'decimal_text'], column_types: { code: 'text', amount: 'number', decimal_text: 'text' }, rows: [['00123', '2.00', '9007199254740993.123456789012345678'], ['ABC', '4.00', '2.00']] }, '受控操作精确输入')
    const target = `agent_etl_e2e_${randomUUID().replaceAll('-', '').slice(0, 16)}`
    await page.goto('/operations')
    const form = page.getByRole('region', { name: '准备操作计划' })
    await form.getByLabel('受管目标名称', { exact: true }).fill(target)
    await form.getByLabel('已授权输入资产 ID', { exact: true }).fill(input.asset_id)
    const first = await prepare(page)
    const targets = await getData<{ items: { name: string; version: number }[] }>(api, '/operations/targets')
    expect(targets.items.find(t => t.name === target)?.version).toBe(0)
    await page.reload()
    const confirm = page.getByRole('button', { name: '明确确认并提交', exact: true })
    await expect(confirm).toBeDisabled()
    await page.getByText(/^提交后的内容预览/).click()
    await expect(page.getByText('9007199254740993.123456789012345678', { exact: true })).toBeVisible()
    async function commit(job: OperationJob) {
      await page.getByRole('checkbox').check()
      const accepted = page.waitForResponse(r => new URL(r.url()).pathname === `/api/v1/operations/jobs/${job.id}/confirm` && r.request().method() === 'POST')
      await confirm.click()
      expect((await accepted).ok()).toBe(true)
      await expect.poll(async () => (await getData<OperationJob>(api, `/operations/jobs/${job.id}`)).status, { timeout: 90_000 }).toBe('completed')
      await page.getByRole('button', { name: '重新核对状态', exact: true }).click()
      await expect(page.getByRole('link', { name: '查看真实结果产物 ↗' })).toBeVisible()
      return getData<OperationJob>(api, `/operations/jobs/${job.id}`)
    }
    const committed = await commit(first)
    // A fresh planning surface creates a new immutable plan; refreshing a receipt never executes.
    await page.goto('/operations')
    await form.getByLabel(/^操作类型/).selectOption('update')
    await form.getByLabel('受管目标名称', { exact: true }).fill(target)
    await form.getByLabel('预期当前版本', { exact: true }).fill('1')
    const values = form.getByRole('region', { name: '写入字段' })
    await values.getByLabel('列名', { exact: true }).fill('amount')
    await values.getByLabel(/^类型/).selectOption('number')
    await values.getByLabel('值', { exact: true }).fill('9007199254740993.123456789012345678')
    const where = form.getByRole('region', { name: '等值筛选条件' })
    await where.getByLabel('条件列', { exact: true }).fill('code')
    await where.getByLabel('条件值', { exact: true }).fill('00123')
    const update = await prepare(page), updated = await commit(update)
    const v2 = await getData<OperationVersion>(api, `/operations/targets/${target}/versions/2`)
    expect(v2.table.rows[0]![1]!).toEqual({ type: 'number', text: '9007199254740993.123456789012345678' })
    const v1 = await getData<OperationVersion>(api, `/operations/targets/${target}/versions/1`)
    expect(v1.table.rows[0]![0]!.text).toBe('00123')
    expect(v1.table.rows[0]![2]!.text).toBe('9007199254740993.123456789012345678')
    await page.goto('/operations')
    await form.getByLabel(/^操作类型/).selectOption('rollback')
    await form.getByLabel('受管目标名称', { exact: true }).fill(target)
    await form.getByLabel('预期当前版本', { exact: true }).fill('2')
    await form.getByLabel('保留的历史版本', { exact: true }).fill('1')
    const rollback = await prepare(page), restored = await commit(rollback)
    expect(restored.committed_version).toBe(3)
    const active = await getData<OperationVersion>(api, `/operations/targets/${target}/versions/0`)
    expect(active.table.rows).toEqual(v1.table.rows)
    await page.reload()
    await expect(page.getByText(/已提交版本 v3/)).toBeVisible()
    await page.screenshot({ path: info.outputPath('governed-operation-restored.png'), fullPage: true })
    await attachJSON(info, 'actual-sql-version-receipts', { input, target, committed, updated, restored, v1, v2, active })
  } finally { await api.dispose() }
})
