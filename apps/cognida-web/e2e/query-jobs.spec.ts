import { randomUUID } from 'node:crypto'
import { test, expect } from '@playwright/test'
import { attachJSON, getData, loginThroughUI } from './lifecycle.helpers'

// Real isolated Hive + MySQL + HTTP + UI, without BigQuery cloud billing.
test('Hive durable query → exact results → reload and same-key replay', async ({ page, playwright }, info) => {
  test.skip(process.env.COGNIDA_E2E_QUERY_JOBS !== '1', 'Requires the isolated retained-session Hive fixture on 62308')
  const api = await loginThroughUI(page, playwright)
  let sourceID = ''
  try {
    const created = await api.post('/api/v1/datasources', { data: { name: `query-hive-${randomUUID()}`, type: 'hive', host: '127.0.0.1', port: 62308, database_name: 'cognida_query_test', username: 'hive', password: '', extra: { auth: 'NONE' } }, headers: { 'Idempotency-Key': randomUUID() } })
    expect(created.ok(), await created.text()).toBe(true)
    sourceID = (await created.json()).data.id
    await page.goto('/datasource')
    const panel = page.getByRole('region', { name: '持久只读查询', exact: true })
    await panel.getByRole('combobox', { name: '查询数据源', exact: true }).selectOption(sourceID)
    await panel.getByLabel('只读 SQL', { exact: true }).fill('SELECT code, amount, big, missing FROM samples ORDER BY code')
    await panel.getByLabel('最多结果行', { exact: true }).fill('10')
    const sent = page.waitForResponse(r => new URL(r.url()).pathname === '/api/v1/query-jobs' && r.request().method() === 'POST')
    await panel.getByRole('button', { name: '提交有界查询', exact: true }).click()
    const response = await sent
    expect(response.ok(), await response.text()).toBe(true)
    const job = (await response.json()).data
    const key = response.request().headers()['idempotency-key']
    const payload = response.request().postDataJSON()
    let actual: any
    await expect.poll(async () => { actual = await getData<any>(api, `/query-jobs/${job.id}`); return actual.status }, { timeout: 40_000 }).toBe('succeeded')
    expect(actual.cleanup_pending).toBe(false)
    expect(actual.remote_status).toBe('succeeded')
    expect(actual.row_count).toBe(2)
    await panel.getByRole('button', { name: '核对查询状态', exact: true }).click()
    await panel.getByRole('button', { name: '读取已保存结果', exact: true }).click()
    await expect(panel.getByRole('cell', { name: '00123', exact: true })).toBeVisible()
    await expect(panel.getByRole('cell', { name: '9007199254740993.123456789', exact: true })).toBeVisible()
    await expect(panel.getByRole('cell', { name: '9223372036854775807', exact: true })).toBeVisible()
    const replay = await api.post('/api/v1/query-jobs', { data: payload, headers: { 'Idempotency-Key': key! } })
    expect(replay.ok(), await replay.text()).toBe(true)
    expect((await replay.json()).data.id).toBe(job.id)
    const conflict = await api.post('/api/v1/query-jobs', { data: { ...payload, max_rows: 9 }, headers: { 'Idempotency-Key': key! } })
    expect(conflict.status()).toBe(409)
    const first = await getData<any>(api, `/query-jobs/${job.id}/results?page_size=1`)
    expect(first.next_cursor).toBeTruthy()
    const second = await getData<any>(api, `/query-jobs/${job.id}/results?page_size=1&cursor=${encodeURIComponent(first.next_cursor)}`)
    expect(second.rows).toHaveLength(1)
    expect(second.next_cursor || '').toBe('')
    expect(first.rows[0].code).toBe('00123')
    await page.reload()
    await panel.getByRole('button', { name: new RegExp(`^${job.id} ·`) }).click()
    await panel.getByRole('button', { name: '读取已保存结果', exact: true }).click()
    await expect(panel.getByRole('cell', { name: '00123', exact: true })).toBeVisible()
    await attachJSON(info, 'hive-query-real-durable-evidence', { job: actual, first, second, replay_same_id: true, conflicting_budget_status: conflict.status(), remote: 'real HiveServer2 NONE, retained sessions; not BigQuery live' })
    await page.screenshot({ path: info.outputPath('hive-query-exact-results.png'), fullPage: true })
  } finally {
    if (sourceID) { const deleted = await api.delete(`/api/v1/datasources/${sourceID}`); expect(deleted.ok()).toBe(true) }
    await api.dispose()
  }
})
