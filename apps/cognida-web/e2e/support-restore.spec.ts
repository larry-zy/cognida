import {readFile} from 'node:fs/promises'
import {expect,test} from '@playwright/test'
import {attachJSON,getData,loginThroughUI} from './lifecycle.helpers'

// Run only after the operator actually restarted the isolated Go service. This
// case authenticates normally, then performs no domain writes or generation.
test('technical support: completed task and HF/SFT receipts survive a real service restart',async({page,playwright},info)=>{
 const path=process.env.COGNIDA_E2E_RESTORE_TASK_FILE
 test.skip(!path,'Requires a completed support-restore-manifest and an independently observed Go restart')
 const expected=JSON.parse(await readFile(path!,'utf8'))
 expect(expected.task_id).toBeTruthy();expect(expected.status).toBe('completed')
 expect(expected.children).toHaveLength(4);expect(expected.nodes).toHaveLength(4)
 const api=await loginThroughUI(page,playwright),mutations:string[]=[]
 const observe=(request:any)=>{if(new URL(request.url()).pathname.startsWith('/api/v1/')&&!['GET','HEAD','OPTIONS'].includes(request.method()))mutations.push(`${request.method()} ${new URL(request.url()).pathname}`)}
 page.on('request',observe)
 try{
  const task=await getData<any>(api,`/governed-tasks/${expected.task_id}`)
  expect(task).toMatchObject({id:expected.task_id,status:'completed',version:expected.task_version,execution_plan_version:expected.execution_plan_version})
  const summary=await getData<any>(api,`/governed-tasks/${expected.task_id}/supervisor/summary`)
  expect(summary.status).toBe('completed');expect(summary.terminal).toBe(true)
  expect(summary.children).toHaveLength(4);expect(summary.nodes).toHaveLength(4)
  const childShape=(child:any)=>({id:child.id,node_id:child.node_id,domain_kind:child.domain_kind,domain_id:child.domain_id,attempt:child.attempt})
  const nodeShape=(node:any)=>({node_id:node.node_id,capability:node.capability,status:node.status,domain_kind:node.domain_kind,domain_id:node.domain_id,attempt:node.attempt})
  const byNode=(items:any[])=>[...items].sort((a,b)=>a.node_id.localeCompare(b.node_id))
  expect(byNode(summary.children.map(childShape))).toEqual(byNode(expected.children))
  expect(byNode(summary.nodes.map(nodeShape))).toEqual(byNode(expected.nodes))
  const imported=await getData<any>(api,`/huggingface/imports/${expected.hf.id}`)
  for(const key of ['id','status','request','manifest','output_refs'])expect(imported[key]).toEqual(expected.hf[key])
  const wrapper=await getData<any>(api,`/governance-executions/${expected.sft.execution_id}`)
  expect(wrapper).toMatchObject({id:expected.sft.execution_id,status:'completed',domain_kind:'sft_dataset',domain_id:expected.sft.dataset_id})
  const dataset=await getData<any>(api,`/sft-datasets/${expected.sft.dataset_id}`)
  expect(dataset).toMatchObject({id:expected.sft.dataset_id,version:expected.sft.dataset_version,source_ref_id:expected.sft.source_ref_id,standard_version_id:expected.sft.standard_version_id,content_digest:expected.sft.content_digest,row_count:2,latest_version_id:expected.sft.published_version_id})
  const version=await getData<any>(api,`/sft-dataset-versions/${expected.sft.published_version_id}`)
  expect(version.content_digest).toBe(expected.sft.published_content_digest)
  await page.goto(`/tasks?task_id=${expected.task_id}`)
  await expect(page.getByText('已成功 4 / 4 个节点',{exact:false})).toBeVisible()
  await page.reload();await expect(page.getByText('已成功 4 / 4 个节点',{exact:false})).toBeVisible()
  await expect(page.getByRole('list',{name:'节点执行状态',exact:true}).locator(`a[href="/hf-datasets?import_id=${expected.hf.id}"]`)).toHaveCount(1)
  await page.getByRole('list',{name:'节点执行状态',exact:true}).locator(`a[href="/sft-datasets?execution_id=${expected.sft.execution_id}"]`).click()
  await expect(page.getByRole('heading',{name:expected.sft.name,exact:true})).toBeVisible()
  await expect(page.getByRole('button',{name:'导出此版本 JSONL',exact:true})).toBeVisible()
  await expect(page.locator('body')).not.toHaveClass(/page-transitioning/)
  await expect(page.locator('.platform-container')).toHaveCSS('opacity','1')
  const heading=page.getByRole('heading',{name:'数据集样本',exact:true});await heading.evaluate(el=>el.scrollIntoView({block:'center'}))
  await page.screenshot({path:info.outputPath('support-after-service-restart.png')})
  expect(mutations).toEqual([])
  await attachJSON(info,'support-after-service-restart',{expected,task,summary,imported,wrapper,dataset,version,browser_domain_writes:mutations})
 }finally{page.off('request',observe);await api.dispose()}
})
