import { test, expect } from '@playwright/test'
import { loginThroughUI, createInput, getData, attachJSON, selectPagedAsset } from './lifecycle.helpers'

test.describe('controlled development Sandbox', () => {
  test.beforeEach(() => test.skip(process.env.COGNIDA_E2E_SANDBOX !== '1', 'Requires an explicitly accepted isolated Docker profile'))
  test('fixed source → actual isolated Python → candidate → refresh → cancel', async ({page,playwright},info) => {
    const api=await loginThroughUI(page,playwright)
    try {
      const input=await createInput(page,{id:'sandbox-source',source:'table',columns:['code','amount'],rows:[['00123','9007199254740993.123456789']]},`sandbox-${Date.now()}`)
      await page.goto('/sandbox')
      await expect(page.getByRole('heading',{name:'新建执行'})).toBeVisible()
      await selectPagedAsset(page, page.getByLabel('输入快照',{exact:true}), `${input.asset_id}:${input.version}`, '加载更多输入')
      await page.getByLabel('Python 脚本',{exact:true}).fill("import json, os\nassert os.getuid() != 0\nv=json.load(open('/inputs/data'))\nv['rows'][0].append('review-candidate')\nv['columns'].append('label')\njson.dump(v,open('/output/features.json','w'))")
      const submitted=page.waitForResponse(r=>new URL(r.url()).pathname==='/api/v1/sandbox/runs'&&r.request().method()==='POST')
      await page.getByRole('button',{name:'保存快照并执行',exact:true}).click()
      const response=await submitted;expect(response.ok(),await response.text()).toBe(true)
      const id=(await response.json()).data.id
      await expect(page).toHaveURL(new RegExp(`run_id=${id}`))
      let run:any
      await expect.poll(async()=>{run=await getData(api,`/sandbox/runs/${id}`);return run.state},{timeout:60000}).toBe('succeeded')
      expect(run.output_refs).toHaveLength(1)
      const artifact=await getData<any>(api,`/lifecycle/assets/${run.output_refs[0].asset_id}?version=${run.output_refs[0].version}`)
      expect(artifact.ref.kind).toBe('cleaned_asset');expect(artifact.ref.policy.approved).not.toBe(true)
      expect(artifact.content.rows[0]).toEqual(['00123','9007199254740993.123456789','review-candidate'])
      await page.reload();await expect(page.getByLabel('脚本执行详情')).toContainText('已完成')
      await expect(page.getByLabel('脚本执行详情').getByRole('link',{name:'查看候选产物 →'})).toBeVisible()
      await attachJSON(info,'actual-sandbox-candidate',{run,artifact});await page.screenshot({path:info.outputPath('sandbox-candidate.png'),fullPage:true})
      await page.getByLabel('Python 脚本',{exact:true}).fill('while True: pass')
      const next=page.waitForResponse(r=>new URL(r.url()).pathname==='/api/v1/sandbox/runs'&&r.request().method()==='POST')
      await page.getByRole('button',{name:'保存快照并执行',exact:true}).click()
      const second=await next;expect(second.ok(),await second.text()).toBe(true);const cancelID=(await second.json()).data.id
      await expect(page).toHaveURL(new RegExp(`run_id=${cancelID}`))
      let beforeCancel!: {state:string;output_refs?:unknown[]}, afterCancel: {state:string;output_refs?:unknown[]}|undefined
      // Exercise cancellation after launch, rather than only removing queued work.
      await expect.poll(async()=>{beforeCancel=await getData(api,`/sandbox/runs/${cancelID}`);expect(beforeCancel.output_refs||[]).toHaveLength(0);return beforeCancel.state},{timeout:20000}).toBe('running')
      await attachJSON(info,'sandbox-before-cancel',beforeCancel)
      await page.getByRole('button',{name:'申请取消',exact:true}).click()
      try {
        await expect.poll(async()=>{afterCancel=await getData<{state:string;output_refs?:unknown[]}>(api,`/sandbox/runs/${cancelID}`);expect(afterCancel.output_refs||[]).toHaveLength(0);return afterCancel.state},{timeout:30000}).toBe('cancelled')
      } finally { if(afterCancel) await attachJSON(info,'sandbox-after-cancel',afterCancel) }
      await expect(page.getByLabel('脚本执行详情')).toContainText('已取消')
    } finally {await api.dispose()}
  })
  test('typed domain task → Sandbox distribution → actual candidate → restored domain detail', async ({page,playwright},info) => {
    const api=await loginThroughUI(page,playwright)
    try {
      const source=await createInput(page,{id:'sandbox-task-code',source:'text',text:"import json, os\nassert os.getuid() != 0\njson.dump({'kind':'table','columns':['code','amount'],'rows':[['00123','9007199254740993.123456789']]},open('/output/result.json','w'))"},`sandbox-task-${Date.now()}`)
      await page.goto('/tasks')
      await page.getByText('使用领域专家：数据采集、准备、SFT 与治理',{exact:true}).click()
      await page.getByLabel(/^领域能力/).selectOption('sandbox.execute')
      for(let pageIndex=0;pageIndex<100;pageIndex++) {
        if(await page.getByLabel('Sandbox代码资产',{exact:true}).locator(`option[value="${source.asset_id}:${source.version}"]`).count()) break
        const more=page.getByRole('button',{name:'加载更多资产',exact:true})
        await expect(more).toBeVisible()
        const loaded=page.waitForResponse(r=>new URL(r.url()).pathname==='/api/v1/lifecycle/assets'&&r.request().method()==='GET')
        await more.click();expect((await loaded).ok()).toBe(true)
      }
      await page.getByLabel('Sandbox代码资产',{exact:true}).selectOption(`${source.asset_id}:${source.version}`)
      const action=async(path:string,label:string)=>{
        const pending=page.waitForResponse(r=>new URL(r.url()).pathname===`/api/v1${path}`&&r.request().method()==='POST')
        await page.getByRole('button',{name:label,exact:true}).click()
        const r=await pending;expect(r.ok(),await r.text()).toBe(true);return (await r.json()).data
      }
      const draft=await action('/governed-tasks','预览领域计划')
      await action(`/governed-tasks/${draft.id}/plans`,'保存计划')
      await action(`/governed-tasks/${draft.id}/start`,'启动任务')
      let summary:any
      await expect.poll(async()=>{
        const response=await api.get(`/api/v1/governed-tasks/${draft.id}/supervisor/summary`)
        if(response.status()===409)return false
        expect(response.ok(),await response.text()).toBe(true);summary=(await response.json()).data;return summary.terminal
      },{timeout:90000}).toBe(true)
      expect(summary.status,JSON.stringify(summary)).toBe('completed')
      expect(summary.nodes[0]).toMatchObject({capability:'sandbox.execute',status:'succeeded',domain_kind:'sandbox_run'})
      await page.reload()
      await page.locator('.runtime-nodes').getByRole('link',{name:'查看领域任务 ↗'}).click()
      await expect(page).toHaveURL(/\/sandbox\?run_id=/)
      const runID=new URL(page.url()).searchParams.get('run_id')!
      const run=await getData<any>(api,`/sandbox/runs/${runID}`)
      expect(run.state).toBe('succeeded');expect(run.output_refs).toHaveLength(1)
      const output=run.output_refs[0]
      const artifact=await getData<any>(api,`/lifecycle/assets/${output.asset_id}?version=${output.version}`)
      expect(artifact.ref.kind).toBe('cleaned_asset');expect(artifact.content.rows[0]).toEqual(['00123','9007199254740993.123456789'])
      await attachJSON(info,'governed-sandbox-task',{draft,summary,run,artifact})
      await page.screenshot({path:info.outputPath('governed-sandbox.png'),fullPage:true})
    } finally {await api.dispose()}
  })
})
