import { randomUUID } from 'node:crypto'
import { test, expect, type APIRequestContext } from '@playwright/test'
import { attachJSON, createInput, getData, loginThroughUI } from './lifecycle.helpers'
type Ref = { asset_id: string; version: number }
type Task = { id: string; version: number; status: string; current_plan_version: number; execution_plan_version: number; deadline_at?: string }
type Capability = { id: string; version: number; tools: string[]; permissions: string[]; output_kinds: string[]; execution: { base_calls: number; calls_per_input: number; max_timeout_seconds: number } }
type Summary = { terminal: boolean; status: string; task_version: number; children: { id: string; node_id: string; plan_version: number; domain_id: string; domain_status: string }[]; nodes: { node_id: string; capability: string; status: string; output_refs: Ref[] }[] }
async function post<T>(api: APIRequestContext, path: string, body: unknown): Promise<T> { const r = await api.post(`/api/v1${path}`, { data: body, headers: { 'Idempotency-Key': randomUUID() } }); expect(r.ok(), await r.text()).toBe(true); return (await r.json()).data }
test('running replan preserves real Sandbox execution and changes only the future expert', async ({ page, playwright }, info) => {
 test.skip(process.env.COGNIDA_E2E_REPLAN !== '1', 'Requires explicitly accepted isolated Sandbox and task runtime')
 const api = await loginThroughUI(page, playwright); let taskID: string | undefined
 try {
  const source = await createInput(page, { id: 'replan-code', source: 'text', text: "import json, time\ntime.sleep(14)\njson.dump({'kind':'text','text':'North revenue is 100. South revenue is 200. Original Sandbox receipt must survive a parent replan.'}, open('/output/result.json','w'))" }, `replan-${Date.now()}`)
  const profileData = await getData<{ enabled: boolean; profile: { ID: string; Version: number; Image: string; MaxResources: { timeout_seconds: number; output_files: number; [key: string]: number } } }>(api, '/sandbox/profiles')
  expect(profileData.enabled).toBe(true); const profile = profileData.profile
  const meta = await getData<{ Ref: Ref; SHA256: string; SizeBytes: number }>(api, `/sandbox/inputs/${source.asset_id}?version=${source.version}`)
  const caps = (await getData<{ capabilities: Capability[] }>(api, '/governed-tasks/capabilities')).capabilities
  const sandbox = caps.find(cap => cap.id === 'sandbox.execute')!, quality = caps.find(cap => cap.id === 'lifecycle.data-quality')!
  expect(sandbox.execution).toBeDefined(); expect(quality.execution).toBeDefined()
  const node = (id: string, cap: Capability, inputs: unknown[], depends: string[]) => ({ id, capability: cap.id, capability_version: cap.version, depends_on: depends, inputs, output_kinds: cap.output_kinds, tools: cap.tools, permissions: cap.permissions, max_attempts: 1, max_calls_per_attempt: cap.execution.base_calls + cap.execution.calls_per_input * inputs.length, max_cost_micros_per_attempt: 0, timeout_seconds: Math.min(30, cap.execution.max_timeout_seconds) })
  const first = { ...node('original', sandbox, [{ asset: meta.Ref, kind: 'raw_asset' }], []), parameters: { sandbox: { profile_id: profile.ID, profile_version: profile.Version, image_sha256: profile.Image.slice(profile.Image.lastIndexOf('@') + 1), code: { name: 'code', ref: meta.Ref, sha256: meta.SHA256, size_bytes: meta.SizeBytes }, inputs: [], resources: { ...profile.MaxResources, timeout_seconds: 30, output_files: Math.min(16, profile.MaxResources.output_files) } } } }
  const future = { ...node('future', quality, [{ from_node: 'original', kind: 'cleaned_asset' }], ['original']), output_kinds: ['cleaned_asset', 'quality_report'] }
  const budget = { max_nodes: 2, max_concurrency: 1, max_attempts: 2, max_calls: 32, max_cost_micros: 0, timeout_seconds: 90 }, plan = { schema_version: 1, budget, nodes: [first, future] }
  const created = await post<Task>(api, '/governed-tasks', { goal: 'Keep the actual running execution and explicitly revise the future step', input_refs: [meta.Ref], budget }); taskID = created.id
  await post(api, `/governed-tasks/${taskID}/plans`, { expected_version: created.version, plan })
  await page.goto(`/tasks?task_id=${taskID}`)
  const starting = page.waitForResponse(r => new URL(r.url()).pathname === `/api/v1/governed-tasks/${taskID}/start` && r.request().method() === 'POST')
  await page.getByRole('button', { name: '启动任务', exact: true }).click(); expect((await starting).ok()).toBe(true)
  let before!: Summary
  await expect.poll(async () => {
   const response = await api.get(`/api/v1/governed-tasks/${taskID}/supervisor/summary`)
   // A concurrent controller update can temporarily prevent a consistent summary.
   if (response.status() === 409) return undefined
   expect(response.ok(), `Initial summary: ${response.status()} ${await response.text()}`).toBe(true)
   before = (await response.json()).data
   return before.children.find(child => child.node_id === 'original')?.domain_id
  }, { timeout: 20_000, intervals: [250, 500] }).toBeTruthy()
  expect(before.children).toHaveLength(1); const original = before.children[0]!, started = await getData<Task>(api, `/governed-tasks/${taskID}`)
  await page.getByRole('button', { name: '重新核对执行状态', exact: true }).click()
  const panel = page.getByRole('region', { name: '调整后续步骤', exact: true })
  await panel.getByRole('button', { name: '编辑未执行步骤', exact: true }).click()
  await expect(panel).toContainText('1 个节点已锁定'); expect(await panel.getByLabel('original 处理能力', { exact: true }).count()).toBe(0)
  await panel.getByLabel('future 处理能力', { exact: true }).selectOption('lifecycle.data-enrichment')
  await panel.getByLabel('调整原因', { exact: true }).fill('原始运行保持不变，将尚未预留的后续质检调整为固定规则增强。')
  await panel.getByRole('checkbox').check()
  const saving = page.waitForResponse(r => new URL(r.url()).pathname === `/api/v1/governed-tasks/${taskID}/replan` && r.request().method() === 'POST')
  await panel.getByRole('button', { name: '确认重规划', exact: true }).click(); const saved = await saving; expect(saved.ok(), await saved.text()).toBe(true)
  const revision = (await saved.json()).data.plan; expect(revision.version).toBe(2); expect(revision.snapshot.nodes[0]).toEqual(first); expect(revision.snapshot.budget).toEqual(budget)
  await page.reload(); await expect(page.getByRole('heading', { name: '已保存计划 v2', exact: true })).toBeVisible()
  let final!: Summary
  await expect.poll(async () => { const r = await api.get(`/api/v1/governed-tasks/${taskID}/supervisor/summary`); if (r.status() === 409) return false; expect(r.ok()).toBe(true); final = (await r.json()).data; return final.terminal }, { timeout: 90_000, intervals: [500, 1000] }).toBe(true)
  expect(final.status, JSON.stringify(final)).toBe('completed'); expect(final.children).toHaveLength(2)
  expect(final.children.find(child => child.node_id === 'original')).toMatchObject({ id: original.id, domain_id: original.domain_id, plan_version: 1, domain_status: 'succeeded' })
  expect(final.children.find(child => child.node_id === 'future')).toMatchObject({ plan_version: 2, domain_status: 'succeeded' })
  expect(final.nodes.find(node => node.node_id === 'future')?.capability).toBe('lifecycle.data-enrichment')
  const latest = await getData<Task>(api, `/governed-tasks/${taskID}`); expect(latest.deadline_at).toBe(started.deadline_at)
  const domain = await getData<{ state: string; output_refs: Ref[] }>(api, `/sandbox/runs/${original.domain_id}`); expect(domain.state).toBe('succeeded'); expect(domain.output_refs).toHaveLength(1)
  const actual = await getData<{ content: { text: string } }>(api, `/lifecycle/assets/${domain.output_refs[0]!.asset_id}?version=${domain.output_refs[0]!.version}`); expect(actual.content.text).toContain('Original Sandbox receipt must survive')
  await page.reload(); await expect(page.locator('.execution-panel .node-progress')).toContainText('已成功 2 / 2')
  await attachJSON(info, 'replan-actual-domain-truth', { source, before, revision, final, latest, domain, actual }); await page.screenshot({ path: info.outputPath('replan-completed.png'), fullPage: true })
 } finally {
  if (taskID) { const r = await api.get(`/api/v1/governed-tasks/${taskID}`); if (r.ok()) { const task = (await r.json()).data as Task; if (['running', 'reconcile_required', 'awaiting_approval'].includes(task.status)) await api.post(`/api/v1/governed-tasks/${taskID}/cancel`, { data: { expected_version: task.version } }) } }
  await api.dispose()
 }
})
