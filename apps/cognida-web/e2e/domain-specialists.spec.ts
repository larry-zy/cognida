import { randomUUID } from 'node:crypto'
import { test, expect, type APIRequestContext, type Page } from '@playwright/test'
import { attachJSON, createInput, getData, loginThroughUI, selectPagedAsset } from './lifecycle.helpers'

type Ref = { asset_id: string; version: number }
async function post<T = any>(api: APIRequestContext, path: string, data: unknown, key = randomUUID()): Promise<T> {
  const response = await api.post(`/api/v1${path}`, { data, headers: { 'Idempotency-Key': key } })
  expect(response.ok(), `${path}: ${response.status()} ${await response.text()}`).toBe(true)
  return (await response.json()).data
}
async function action(page: Page, path: string, click: () => Promise<unknown>, method = 'POST') {
  const pending = page.waitForResponse(r => new URL(r.url()).pathname === `/api/v1${path}` && r.request().method() === method)
  await click(); const response = await pending
  expect(response.ok(), `${path}: ${response.status()} ${await response.text()}`).toBe(true)
  return (await response.json()).data
}
async function terminal(api: APIRequestContext, id: string) {
  let summary: any
  await expect.poll(async () => {
    const response = await api.get(`/api/v1/governed-tasks/${id}/supervisor/summary`)
    if (response.status() === 409) return false
    expect(response.ok()).toBe(true); summary = (await response.json()).data
    return summary.terminal
  }, { timeout: 120_000, intervals: [500, 1000, 2000] }).toBe(true)
  expect(summary.status, JSON.stringify(summary)).toBe('completed')
  return summary
}
async function tableInput(page: Page) {
  return createInput(page, { id: 'feature-source', source: 'table', columns: ['customer', 'occurred', 'amount', 'question', 'reference_answer'], column_types: { amount: 'number' }, rows: [['A', '2026-01-01T00:00:00Z', 10, '北京是什么？', '北京是中国的首都。'], ['A', '2026-01-01T00:30:00Z', 20, '营收是多少？', '20'], ['B', '2026-01-01T00:30:00Z', 7, '城市名称？', '北京']] }, `domain-${Date.now()}`)
}

test.describe('real feature engineering and labeling specialists', () => {
  test.beforeEach(() => { test.skip(process.env.COGNIDA_E2E_DOMAIN_SPECIALISTS !== '1', 'Requires disposable specialist acceptance environment') })
  test('window preview → fixed version → materialization → historical query → expert task', async ({ page, playwright }, info) => {
    const api = await loginThroughUI(page, playwright)
    try {
      const source = await tableInput(page); const sourceRef = `${source.asset_id}:${source.version}`
      await page.goto('/features')
      await page.getByLabel('名称', { exact: true }).fill(`一小时收入-${Date.now()}`)
      await selectPagedAsset(page, page.getByLabel(/^来源资产/), sourceRef)
      await page.getByLabel(/^计算方式/).selectOption('window_aggregate')
      await page.getByLabel('来源字段', { exact: true }).fill('amount')
      await page.getByLabel(/^实体标识字段/).fill('customer')
      await page.getByLabel(/^事件时间字段/).fill('occurred')
      await page.getByLabel('窗口秒数', { exact: true }).fill('3600')
      const preview = await action(page, '/features/preview', () => page.getByRole('button', { name: '预览计算结果', exact: true }).click())
      expect(preview.values.map((item: any) => item.value)).toEqual([10, 30, 7])
      await expect(page.getByRole('region', { name: '特征预览' })).toContainText('未写入')
      const feature = await action(page, '/features', () => page.getByRole('button', { name: '注册特征', exact: true }).click())
      const before = new Date(Date.now() - 1000).toISOString()
      const receipt = await action(page, `/features/${feature.id}/materialize`, () => page.getByRole('button', { name: '计算并物化所选版本' }).click())
      expect(receipt.count).toBe(3); expect(receipt.output_ref_id).toBeTruthy()
      await expect(page.getByText('已物化 3 个特征值')).toBeVisible()
      const query = { entity_ids: ['A', 'B'], features: [{ feature_id: feature.id, version: 1 }], as_of: before }
      const historical = await post(api, '/feature-values/query', query)
      expect(historical.items).toEqual([])
      const current = await post(api, '/feature-values/query', { ...query, as_of: new Date().toISOString() })
      expect(current.items.map((row: any) => row.value)).toEqual([30, 7])
      await page.getByLabel(/^计算方式/).selectOption('expression')
      const expression = page.locator('fieldset.expression-editor').first()
      await expression.getByLabel(/^运算/).first().selectOption('mul')
      await expression.getByLabel('字段名', { exact: true }).fill('amount')
      await expression.getByLabel('常量值', { exact: true }).fill('2')
      // Field/expression outputs need unique entity IDs if no event time is configured.
      await page.getByLabel(/^实体标识字段/).fill('')
      const doubled = await action(page, '/features/preview', () => page.getByRole('button', { name: '预览计算结果', exact: true }).click())
      expect(doubled.values.map((item: any) => item.value)).toEqual([20, 40, 14])
      await action(page, `/features/${feature.id}/versions`, () => page.getByRole('button', { name: '创建新版本', exact: true }).click())
      await page.screenshot({ path: info.outputPath('feature-definition.png'), fullPage: true })
      await page.goto('/tasks')
      await page.getByText('使用领域专家：特征工程、知识打标与样本标注', { exact: true }).click()
      await page.getByLabel(/^领域能力/).selectOption('governance.feature-engineering')
      await page.getByLabel(/^专家特征定义/).selectOption(feature.id)
      await page.getByLabel('专家特征版本', { exact: true }).fill('2')
      const draft = await action(page, '/governed-tasks', () => page.getByRole('button', { name: '预览领域计划', exact: true }).click())
      await expect(page.getByRole('heading', { name: '候选计划', exact: true })).toBeVisible()
      await action(page, `/governed-tasks/${draft.id}/plans`, () => page.getByRole('button', { name: '保存计划', exact: true }).click())
      await action(page, `/governed-tasks/${draft.id}/start`, () => page.getByRole('button', { name: '启动任务', exact: true }).click())
      const summary = await terminal(api, draft.id)
      expect(summary.nodes[0]).toMatchObject({ capability: 'governance.feature-engineering', status: 'succeeded', domain_kind: 'feature_execution' })
      await page.locator('.runtime-nodes').getByRole('link', { name: '查看领域任务 ↗' }).click()
      await expect(page).toHaveURL(/\/features\?execution_id=/)
      await expect(page.getByLabel('特征版本', { exact: true })).toHaveValue('2')
      await attachJSON(info, 'feature-and-expert-receipts', { preview, receipt, historical, current, summary })
    } finally { await api.dispose() }
  })

  test('classification, entity, knowledge tags and QA → evidence → manual review/export', async ({ page, playwright }, info) => {
    const api = await loginThroughUI(page, playwright)
    try {
      const source = await createInput(page, { id: 'label-text', source: 'text', text: '😀北京订单营收增长。' }, `标注-${Date.now()}`)
      const evidence: unknown[] = []
      for (const kind of ['classification', 'named_entity', 'tagging']) {
        await page.goto('/labeling')
        if (kind === 'tagging') await page.getByRole('button', { name: '智能打标', exact: true }).click()
        await page.getByRole('button', { name: '＋ 新建任务', exact: true }).click()
        await page.getByLabel('任务名称', { exact: true }).fill(`${kind}-${Date.now()}`)
        await selectPagedAsset(page, page.getByLabel(/^来源资产/), `${source.asset_id}:${source.version}`)
        if (kind !== 'tagging') await page.getByLabel(/^标注类型/).selectOption(kind)
        await page.getByRole('button', { name: '＋ 添加标签', exact: true }).click()
        await page.getByLabel('标签1 标签名称', { exact: true }).fill(kind === 'named_entity' ? '城市' : '销售')
        await page.getByLabel(kind === 'named_entity' ? '标签1 实体词条' : '标签1 匹配关键词', { exact: true }).fill(kind === 'named_entity' ? '北京' : '营收')
        const path = kind === 'tagging' ? '/tagging-tasks' : '/annotation-tasks'
        const task = await action(page, path, () => page.getByRole('button', { name: kind === 'tagging' ? '创建并执行打标' : '创建标注任务', exact: true }).click())
        if (kind !== 'tagging') await action(page, `${path}/${task.id}/preannotate`, () => page.getByRole('button', { name: '生成预标注', exact: true }).click())
        let items: any[] = []
        await expect.poll(async () => { items = (await getData<any>(api, `${path}/${task.id}/${kind === 'tagging' ? 'results' : 'items'}`)).items; return items.length > 0 && items[0].provenance?.decision_status === 'matched' }, { timeout: 30_000 }).toBe(true)
        await page.getByRole('button', { name: '刷新结果', exact: true }).click()
        await page.locator('.wb-list button').filter({ hasText: '😀北京订单营收增长。' }).click()
        await expect(page.getByLabel('规则命中依据')).toContainText(kind === 'named_entity' ? '北京' : '营收')
        const reviewPath = kind === 'tagging' ? `/tagging-results/${items[0].id}/review` : `${path}/${task.id}/items/${items[0].id}/review`
        await action(page, reviewPath, () => page.getByRole('button', { name: '审核通过', exact: true }).click())
        const approved = await getData<any>(api, `${path}/${task.id}/${kind === 'tagging' ? 'results' : 'items'}`)
        expect(approved.items[0].review_status).toBe('approved'); expect(approved.items[0].provenance.confidence ?? null).toBeNull()
        evidence.push({ kind, task, approved })
      }
      const qaSource = await createInput(page, { id: 'qa-table', source: 'table', columns: ['q', 'a'], rows: [['北京是什么？', '北京是中国的首都。']] }, `QA-${Date.now()}`)
      await page.goto('/labeling')
      await page.getByLabel('任务名称', { exact: true }).fill(`QA-${Date.now()}`)
      await selectPagedAsset(page, page.getByLabel(/^来源资产/), `${qaSource.asset_id}:${qaSource.version}`)
      await page.getByLabel(/^标注类型/).selectOption('qa')
      await page.getByLabel('问题列', { exact: true }).fill('q'); await page.getByLabel('参考答案列', { exact: true }).fill('a')
      const qa = await action(page, '/annotation-tasks', () => page.getByRole('button', { name: '创建标注任务', exact: true }).click())
      await action(page, `/annotation-tasks/${qa.id}/preannotate`, () => page.getByRole('button', { name: '生成预标注', exact: true }).click())
      await page.locator('.wb-list button').filter({ hasText: '北京是什么？' }).click()
      await page.getByLabel('参考答案', { exact: true }).fill('北京是中国的首都，也是直辖市。')
      let rows = (await getData<any>(api, `/annotation-tasks/${qa.id}/items`)).items
      await action(page, `/annotation-tasks/${qa.id}/items/${rows[0].id}`, () => page.getByRole('button', { name: '保存标注', exact: true }).click(), 'PUT')
      await action(page, `/annotation-tasks/${qa.id}/items/${rows[0].id}/review`, () => page.getByRole('button', { name: '审核通过', exact: true }).click())
      const download = page.waitForEvent('download')
      await page.getByRole('button', { name: '导出 JSONL', exact: true }).click()
      const file = await download; await file.saveAs(info.outputPath('approved-qa.jsonl'))
      const exported = await api.get(`/api/v1/annotation-tasks/${qa.id}/export`)
      expect(exported.ok()).toBe(true); expect(await exported.text()).toContain('也是直辖市')
      await action(page, `/annotation-tasks/${qa.id}/preannotate`, () => page.getByRole('button', { name: '生成预标注', exact: true }).click())
      rows = (await getData<any>(api, `/annotation-tasks/${qa.id}/items`)).items
      expect(rows[0]).toMatchObject({ review_status: 'approved', provenance: { source: 'human' }, annotation: { qa: { reference_answer: '北京是中国的首都，也是直辖市。' } } })
      await attachJSON(info, 'reviewed-label-and-qa-evidence', { evidence, qa, rows })
      await page.screenshot({ path: info.outputPath('qa-review.png'), fullPage: true })
    } finally { await api.dispose() }
  })

  test('multiple specialists share a durable DAG and real result bindings', async ({ page, playwright }, info) => {
    const api = await loginThroughUI(page, playwright)
    try {
      const source = await tableInput(page), ref: Ref = { asset_id: source.asset_id, version: source.version }
      const feature = await post(api, '/features', { name: 'DAG amount', type: 'numerical', source_ref_id: `${ref.asset_id}:${ref.version}`, definition: { kind: 'field', field: 'amount' } })
      const registry = await getData<any>(api, '/governed-tasks/capabilities')
      function node(id: string, capability: string, inputs: any[], depends_on: string[] = [], parameters?: unknown) {
        const cap = registry.capabilities.find((v: any) => v.id === capability)
        return { id, capability, capability_version: cap.version, inputs, depends_on, output_kinds: capability === 'lifecycle.data-quality' ? [inputs[0].kind, 'quality_report'] : cap.output_kinds, tools: cap.tools, permissions: cap.permissions, max_attempts: 1, max_calls_per_attempt: cap.execution.base_calls + inputs.length * cap.execution.calls_per_input, max_cost_micros_per_attempt: 0, timeout_seconds: 60, ...(parameters ? { parameters } : {}) }
      }
      const nodes = [node('quality', 'lifecycle.data-quality', [{ asset: ref, kind: 'raw_asset' }]), node('features', 'governance.feature-engineering', [{ asset: ref, kind: 'raw_asset' }], [], { feature: { feature_id: feature.id, version: 1 } }), node('clean', 'lifecycle.data-cleaning', [{ from_node: 'quality', kind: 'raw_asset' }, { from_node: 'quality', kind: 'quality_report' }], ['quality']), node('tag', 'governance.tagging', [{ from_node: 'clean', kind: 'cleaned_asset' }], ['clean'], { governance: { label_schema: [{ id: 'city', name: '城市', keywords: ['北京'] }] } })]
      const budget = { max_nodes: nodes.length, max_concurrency: 2, max_attempts: nodes.length, max_calls: nodes.reduce((n, v) => n + v.max_calls_per_attempt, 0), max_cost_micros: 0, timeout_seconds: 240 }
      const task = await post(api, '/governed-tasks', { goal: '真实质量、清洗、知识打标及特征工程协作', input_refs: [ref], budget })
      await post(api, `/governed-tasks/${task.id}/plans`, { expected_version: task.version, plan: { schema_version: 1, budget, nodes } })
      await page.goto(`/tasks?task_id=${task.id}`)
      await action(page, `/governed-tasks/${task.id}/start`, () => page.getByRole('button', { name: '启动任务', exact: true }).click())
      const summary = await terminal(api, task.id)
      expect(summary.nodes).toHaveLength(4); expect(summary.nodes.every((v: any) => v.status === 'succeeded' && v.attempt === 1)).toBe(true)
      const results = []
      for (const n of summary.nodes) for (const output of n.output_refs) results.push(await getData(api, `/lifecycle/assets/${output.asset_id}?version=${output.version}`))
      expect(results.length).toBeGreaterThanOrEqual(6)
      await page.reload(); await expect(page.locator('.execution-panel')).toContainText('已成功 4 / 4')
      await attachJSON(info, 'multi-specialist-real-results', { summary, results })
      await page.screenshot({ path: info.outputPath('multi-specialist-dag.png'), fullPage: true })
    } finally { await api.dispose() }
  })
})
