import {prepareScenarioInput,publishScenarioStandard,openScenarioLabeling} from './scenario.helpers'
import { randomUUID } from 'node:crypto'
import { test, expect, type Page } from '@playwright/test'
import { loginThroughUI, createInput, getData, attachJSON } from './lifecycle.helpers'

async function action(page: Page, path: string, name: string, method = 'POST') {
  const waiting = page.waitForResponse(r => new URL(r.url()).pathname === `/api/v1${path}` && r.request().method() === method)
  await page.getByRole('button', { name, exact: true }).click()
  const response = await waiting
  expect(response.ok(), `${path}: ${response.status()} ${await response.text()}`).toBe(true)
  return (await response.json()).data
}

test.describe('reviewed knowledge labels and immutable datasets', () => {
  test.beforeEach(() => test.skip(process.env.COGNIDA_E2E_GOVERNANCE_RELEASE !== '1', 'Requires isolated real workers, model and vector index'))
  test('real model suggestion → human review → immutable QA version → draft change preserves version', async ({ page, playwright }, info) => {
    const api = await loginThroughUI(page, playwright)
    try {
      const source = await createInput(page, { id: 'qa-release', source: 'table', columns: ['question', 'answer'], rows: [['What is the capital of France?', 'Paris'], ['What color is the clear daytime sky?', 'Blue']] }, `QA-release-${randomUUID()}`)
      const prepared=await prepareScenarioInput(api,source), standard=await publishScenarioStandard(api,'qa')
      await openScenarioLabeling(page,prepared,standard)
      await page.getByLabel('任务名称', { exact: true }).fill(`QA release ${randomUUID()}`)
      await page.getByLabel('问题列', { exact: true }).fill('question')
      await page.getByLabel('参考答案列', { exact: true }).fill('answer')
      const task = await action(page, '/annotation-tasks', '创建标注任务')
      await action(page, `/annotation-tasks/${task.id}/preannotate`, '生成预标注')
      const rows = (await getData<any>(api, `/annotation-tasks/${task.id}/items`)).items
      expect(rows).toHaveLength(2)
      let modelEvidence: any
      for (let index = 0; index < rows.length; index++) {
        const item = rows[index]
        await page.locator('.wb-list button').filter({ hasText: item.content.slice(0, 70) }).click()
        if (index === 0) {
          await page.getByLabel('标注指南', { exact: true }).fill('Preserve the exact original question and reference answer. Do not invent or paraphrase either field.')
          modelEvidence = await action(page, `/annotation-tasks/${task.id}/items/${item.id}/suggest`, '调用模型生成当前样本建议')
          expect(modelEvidence.provenance.model_call_id).toBeTruthy()
          expect(modelEvidence.review_status).not.toBe('approved')
        }
        await action(page, `/annotation-tasks/${task.id}/items/${item.id}/review`, '审核通过')
        await expect(page.getByRole('button', { name: '调用模型生成当前样本建议', exact: true })).toBeDisabled()
      }
      const panel = page.getByRole('region', { name: '发布审核数据集版本' })
      await panel.getByLabel('数据集 ID', { exact: true }).fill(`qa-release-${randomUUID()}`)
      const name = `Fixed QA ${randomUUID().slice(0, 8)}`
      await panel.getByLabel('版本名称', { exact: true }).fill(name)
      await panel.getByLabel(/^样本用途/).selectOption('qa')
      for (const checkbox of await panel.locator('input[type="checkbox"]').all()) await checkbox.check()
      const version = await action(page, '/evaluation/dataset-versions', '发布不可变版本')
      expect(version.samples).toHaveLength(2); expect(version.manifest_digest).toHaveLength(64)
      const before = await getData<any>(api, `/evaluation/dataset-versions/${version.id}`)
      await page.getByLabel('参考答案', { exact: true }).fill('A later human correction that must not rewrite the fixed version.')
      const edited = await action(page, `/annotation-tasks/${task.id}/items/${rows[1].id}`, '保存标注', 'PUT')
      expect(edited.review_status).not.toBe('approved')
      const after = await getData<any>(api, `/evaluation/dataset-versions/${version.id}`)
      expect(after).toEqual(before)
      await page.goto('/datasets')
      const versions = page.locator('.reviewed-versions')
      await expect(versions.getByRole('button', { name: '刷新版本', exact: true })).toBeEnabled()
      await expect(versions.getByRole('alert')).toHaveCount(0)
      while (!(await versions.getByText(`${name} · v1`, { exact: true }).count())) {
        await versions.getByRole('button', { name: '加载更多版本', exact: true }).click()
        await expect(versions.getByRole('button', { name: '刷新版本', exact: true })).toBeEnabled()
      }
      await expect(versions.getByText(`${name} · v1`, { exact: true })).toBeVisible()
      await page.reload(); await expect(page.locator('.reviewed-versions')).toBeVisible()
      await attachJSON(info, 'model-and-immutable-dataset', { modelEvidence, before, edited, after })
      await page.screenshot({ path: info.outputPath('reviewed-dataset.png'), fullPage: true })
    } finally { await api.dispose() }
  })

  test('knowledge tag review → apply receipt → real vector label filter', async ({ page, playwright }, info) => {
    const api = await loginThroughUI(page, playwright)
    let kb = ''
    try {
      const marker = `LABEL-${randomUUID()}`
      const created = await api.post('/api/v1/knowledge-bases', { data: { name: marker, is_public: false, graph_enabled: false }, headers: { 'Idempotency-Key': randomUUID() } })
      expect(created.ok()).toBe(true); kb = (await created.json()).data.id
      const text = `Cognida ${marker} is an isolated labeling fixture. Its topic is data governance. This exact text is the source for the reviewed knowledge category.`
      const uploaded = await api.post(`/api/v1/knowledge-bases/${kb}/knowledge/file`, { multipart: { file: { name: `${marker}.txt`, mimeType: 'text/plain', buffer: Buffer.from(text) } } })
      expect(uploaded.ok()).toBe(true); const knowledgeID = (await uploaded.json()).data.knowledge_id
      await expect.poll(async () => (await getData<any>(api, `/knowledge-bases/${kb}/knowledge/${knowledgeID}`)).parse_status, { timeout: 60_000 }).toBe('completed')
      const source = await createInput(page, { id: marker, source: 'knowledge', knowledge_id: knowledgeID }, marker)
      const prepared=await prepareScenarioInput(api,source), standard=await publishScenarioStandard(api,'tagging',[{id:'governance',name:'数据治理',keywords:['governance']}])
      await openScenarioLabeling(page,prepared,standard)
      await page.getByLabel('任务名称', { exact: true }).fill(marker)
      const task = await action(page, '/tagging-tasks', '创建并执行打标')
      let items: any[] = []
      await expect.poll(async () => { items = (await getData<any>(api, `/tagging-tasks/${task.id}/results`)).items; return items.length }, { timeout: 30_000 }).toBe(1)
      await page.getByRole('button', { name: '刷新结果', exact: true }).click()
      await page.getByRole('button', { name: new RegExp(`^Cognida ${marker}`) }).click()
      await expect(page.getByRole('button', { name: '应用已审核标签', exact: true })).toBeDisabled()
      await action(page, `/tagging-results/${items[0].id}/review`, '审核通过')
      const receipt = await action(page, `/tagging-results/${items[0].id}/apply`, '应用已审核标签')
      expect(receipt.knowledge_id).toBe(knowledgeID); expect(receipt.labels.length).toBeGreaterThan(0)
      await page.goto(`/knowledge/${kb}`); await page.getByRole('tab', { name: '检索', exact: true }).click()
      await page.getByPlaceholder('输入要搜索的内容...').fill(marker)
      await page.getByLabel(/^检索模式/).selectOption('vector')
      await page.getByLabel('已审核知识标签', { exact: true }).fill(receipt.labels[0])
      const thresholds = page.locator('.search-options .ui-input-number__input')
      await thresholds.nth(1).fill('0'); await thresholds.nth(1).blur()
      let matches: any
      const attempts: any[] = []
      let fixedRequest: unknown
      // Index visibility may lag the real application receipt. Read the same
      // query through the UI; never re-apply labels or accept a lexical fallback.
      await expect.poll(async () => {
        const waiting = page.waitForResponse(r => new URL(r.url()).pathname === '/api/v1/knowledge/search' && r.request().method() === 'POST')
        await page.getByRole('button', { name: '搜索', exact: true }).click()
        const response = await waiting
        const request = response.request().postDataJSON()
        const body = await response.json()
        attempts.push({ request, status: response.status(), body })
        await attachJSON(info, 'knowledge-label-retrieval-attempts', { kb, knowledgeID, receipt, attempts })
        expect(response.status()).toBe(200)
        expect(request).toMatchObject({ retrieval_mode: 'vector', kb_ids: [kb] })
        if (fixedRequest === undefined) fixedRequest = request
        else expect(request).toEqual(fixedRequest)
        matches = body.data
        return matches.items.some((row: any) => row.knowledge_id === knowledgeID && row.sources.includes('milvus_vector'))
      }, { timeout: 60_000, intervals: [1000, 2000, 5000], message: 'reviewed label is visible through real vector retrieval' }).toBe(true)
      expect(matches.items.some((row: any) => row.knowledge_id === knowledgeID && row.sources.includes('milvus_vector'))).toBe(true)
      for (const row of matches.items.filter((row: any) => row.knowledge_id === knowledgeID)) expect(row.sources).not.toContain('database_lexical')
      await expect(page.getByRole('button', { name: '搜索', exact: true })).toBeEnabled()
      await expect(page.locator('.search-results .el-loading-mask')).toBeHidden()
      await expect(page.locator('.search-result-item').filter({ hasText: marker })).toBeVisible()
      await page.screenshot({ path: info.outputPath('knowledge-label-positive-vector.png') })
      await page.getByLabel('已审核知识标签', { exact: true }).fill(`missing-${randomUUID()}`)
      const excluded = await action(page, '/knowledge/search', '搜索')
      expect(excluded.items).toEqual([])
      await attachJSON(info, 'knowledge-label-real-retrieval', { receipt, matches, excluded })
      await expect(page.getByRole('button', { name: '搜索', exact: true })).toBeEnabled()
      await expect(page.locator('.search-results .el-loading-mask')).toBeHidden()
      await expect(page.locator('.search-results').getByText('未找到相关内容', { exact: true })).toBeVisible()
      await page.screenshot({ path: info.outputPath('knowledge-label-filter.png'), fullPage: true })
    } finally {
      try {
        if (kb) { const deleted = await api.delete(`/api/v1/knowledge-bases/${kb}`); await attachJSON(info,'knowledge-fixture-cleanup',{kb,status:deleted.status()}); expect(deleted.ok()).toBe(true) }
      } finally { await api.dispose() }
    }
  })
})
