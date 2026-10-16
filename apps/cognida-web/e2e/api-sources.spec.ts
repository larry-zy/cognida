import { randomUUID } from 'node:crypto'
import { expect, test } from '@playwright/test'
import { attachJSON, getData, loginThroughUI } from './lifecycle.helpers'

test('managed API credentials and fixed sources persist; non-JSON upstream fails visibly', async ({ page, playwright }, info) => {
  test.skip(process.env.COGNIDA_E2E_API_SOURCES !== '1', 'Requires the isolated API ingestion worker environment')
  const api = await loginThroughUI(page, playwright)
  try {
    const suffix = randomUUID(), name = `API review ${suffix}`
    await page.goto('/api-sources')
    await page.getByText('登记新凭据', { exact: true }).click()
    const credentialForm = page.getByRole('form', { name: '登记 API 凭据' })
    await credentialForm.getByLabel('凭据名称', { exact: true }).fill(name)
    await credentialForm.getByLabel('绑定来源', { exact: true }).fill('https://example.com/')
    // Disposable test text, never a user's API credential. The public endpoint
    // deliberately returns HTML; successful authenticated JSON pagination is
    // separately verified against a real TLS fixture in the Go integration test.
    const secret = `synthetic-e2e-${randomUUID()}`
    await credentialForm.getByLabel('密钥', { exact: true }).fill(secret)
    const created = page.waitForResponse(r => new URL(r.url()).pathname === '/api/v1/api-credentials' && r.request().method() === 'POST')
    await credentialForm.getByRole('button', { name: '加密保存凭据' }).click()
    const response = await created
    expect(response.ok()).toBe(true)
    const raw = await response.text()
    expect(raw.includes(secret)).toBe(false)
    const credential = JSON.parse(raw).data
    await expect(credentialForm.getByLabel('密钥', { exact: true })).toHaveValue('')
    await page.getByRole('button', { name: '＋ 配置来源' }).click()
    const form = page.getByRole('region', { name: '配置 API 来源' })
    await form.getByLabel('来源名称', { exact: true }).fill(name)
    await form.getByLabel('已登记凭据', { exact: true }).selectOption(credential.id)
    await form.getByLabel('GET 地址', { exact: true }).fill(`https://example.com/?review=${suffix}`)
    const saved = page.waitForResponse(r => new URL(r.url()).pathname === '/api/v1/api-sources' && r.request().method() === 'POST')
    await form.getByRole('button', { name: '保存来源版本' }).click()
    const savedResponse = await saved
    expect(savedResponse.ok()).toBe(true)
    const source = (await savedResponse.json()).data
    await page.reload()
    const item = page.locator('.source-list > li').filter({ hasText: name })
    await expect(item).toContainText('v1')
    const started = page.waitForResponse(r => new URL(r.url()).pathname === `/api/v1/api-sources/${source.id}/runs` && r.request().method() === 'POST')
    await item.getByRole('button', { name: '采集此版本' }).click()
    const startResponse = await started
    expect(startResponse.ok()).toBe(true)
    const initial = (await startResponse.json()).data
    let run: any
    await expect.poll(async () => { run = await getData(api, `/api-ingestion-runs/${initial.id}`); return ['failed', 'partial', 'timed_out'].includes(run.status) }, { timeout: 140_000 }).toBe(true)
    expect(run.output_refs || []).toHaveLength(0)
    expect(run.watermark_after || '').toBe('')
    await page.reload()
    await expect(page.getByRole('region', { name: '采集回执' })).toContainText(/失败|部分完成|已超时/)
    await expect(page.getByRole('region', { name: '采集回执' })).toContainText('水位未推进')
    await attachJSON(info, 'api-source-real-negative-upstream', { source_id: source.id, run_id: run.id, status: run.status, requests: run.requests, evidence_scope: 'Real public HTTPS non-JSON rejection; successful authenticated pagination is covered by the real TLS integration fixture.' })
    await page.screenshot({ path: info.outputPath('api-source-failure.png'), fullPage: true })
  } finally { await api.dispose() }
})
