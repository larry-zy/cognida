import {createHash,randomUUID} from 'node:crypto'
import {readFile} from 'node:fs/promises'
import {expect,test,type APIRequestContext,type Page,type TestInfo} from '@playwright/test'
import {attachJSON,getData,loginThroughUI,selectPagedAsset,verifyVectorRetrieval} from './lifecycle.helpers'
import {openScenarioLabeling,publishScenarioStandard,waitInvocation,writeScenario} from './scenario.helpers'

const fixed=JSON.parse(await readFile(new URL('./fixtures/techqa-support.json',import.meta.url),'utf8'))
const hash=(text:string)=>createHash('sha256').update(text).digest('hex')
const refID=(r:any)=>`${r.asset_id}:${r.version}`
async function clickWrite(page:Page,info:TestInfo,path:string,name:string){
 const waiting=page.waitForResponse(r=>new URL(r.url()).pathname===`/api/v1${path}`&&r.request().method()==='POST',{timeout:120000})
 await page.getByRole('button',{name,exact:true}).click()
 const response=await waiting,body=await response.text()
 await attachJSON(info,`http-${path.replaceAll('/','_')}`,{status:response.status(),body})
 expect(response.ok(),`${path}: ${response.status()} ${body}`).toBe(true)
 return JSON.parse(body).data
}
async function capture(page:Page,info:TestInfo,anchor:ReturnType<Page['getByRole']>,name:string){
 await expect(page.locator('body')).not.toHaveClass(/page-transitioning/)
 await expect(page.locator('.platform-container')).toHaveCSS('opacity','1')
 await anchor.evaluate(el=>el.scrollIntoView({block:'center'}));await expect(anchor).toBeInViewport()
 await page.screenshot({path:info.outputPath(`${name}.png`)})
}
async function qaStandard(page:Page,info:TestInfo){
 await page.goto('/label-standards')
 await page.getByLabel('标准名称',{exact:true}).fill(`TechQA support ${randomUUID()}`)
 await page.getByLabel('任务类型',{exact:true}).selectOption('qa')
 await page.getByLabel('总则与判定规则',{exact:true}).fill('保留固定来源中的技术支持问题与既有答案，不补答。空答案拒绝整批构建。人工检查来源、内容和训练划分后才能发布。')
 const head=await clickWrite(page,info,'/label-standards','保存草稿')
 return clickWrite(page,info,`/label-standards/${head.id}/publish`,'发布此修订')
}
async function finishTask(api:APIRequestContext,id:string,info:TestInfo){
 let summary:any
 await expect.poll(async()=>{
  const response=await api.get(`/api/v1/governed-tasks/${id}/supervisor/summary`)
  if(response.status()===409)return false // concurrent task/summary versions: read only, no write replay
  expect(response.status()).toBe(200);summary=(await response.json()).data
  await attachJSON(info,'task-latest-summary',summary)
  return summary.terminal===true
 },{timeout:240000,intervals:[500,1000,2000]}).toBe(true)
 expect(summary.status,JSON.stringify(summary)).toBe('completed')
 expect(summary.nodes).toHaveLength(4)
 for(const node of summary.nodes){expect(node.status).toBe('succeeded');expect(node.attempt).toBe(1);expect(node.domain_id).toBeTruthy()}
 expect(new Set(summary.nodes.map((n:any)=>n.domain_id)).size).toBe(4)
 return summary
}
function checkPlan(plan:any,request:any,sft:any){
 const order=['huggingface.import','lifecycle.data-cleaning','lifecycle.data-preparation','governance.sft']
 expect(plan.nodes).toHaveLength(4)
 const nodes=order.map(cap=>{const found=plan.nodes.filter((n:any)=>n.capability===cap);expect(found).toHaveLength(1);return found[0]})
 expect(nodes[0].parameters).toEqual({huggingface:{request}})
 expect(nodes[0].max_calls_per_attempt).toBe(11);expect(nodes[3].max_calls_per_attempt).toBe(3)
 expect(nodes[0].inputs||[]).toEqual([])
 expect(nodes[0].depends_on||[]).toEqual([])
 expect(nodes[3].parameters).toEqual({sft})
 for(let i=1;i<4;i++){
  expect(nodes[i].depends_on).toEqual([nodes[i-1].id])
  expect(nodes[i].inputs).toEqual([{from_node:nodes[i-1].id,kind:i===1?'raw_asset':'cleaned_asset'}])
 }
 for(const node of nodes){expect(node.max_attempts).toBe(1);expect(node.max_cost_micros_per_attempt).toBe(0)}
 return nodes
}

// A real, fixed two-record functionality acceptance, not a quality benchmark.
// The third original row is marked is_impossible, has answer '-' and no
// contexts. It is recorded as a source-data limitation, never rewritten to
// blank text or claimed to be rejected by generic nonempty-answer validation.
test('technical support: real Supervisor HF → clean → prepare → SFT, then reviewed context knowledge and Agent answer',async({page,playwright},info)=>{
 test.skip(process.env.COGNIDA_E2E_SUPPORT_SUPERVISOR!=='1','Requires explicitly enabled isolated HF, Supervisor model, workers, Milvus and Agent services')
 test.setTimeout(600000)
 const api=await loginThroughUI(page,playwright)
 let kb=''
 try{
  const standard=await qaStandard(page,info),name=`TechQA candidate ${randomUUID().slice(0,8)}`
  await page.goto('/tasks')
  await page.locator('summary').filter({hasText:'使用领域专家：数据采集、准备、SFT 与治理'}).click()
  await page.getByLabel('领域能力',{exact:true}).selectOption('huggingface.import')
  await page.getByLabel('任务 HF 仓库',{exact:true}).fill(fixed.dataset_id)
  await page.getByLabel('任务 HF revision',{exact:true}).fill(fixed.revision)
  const discovering=page.waitForResponse(r=>new URL(r.url()).pathname==='/api/v1/huggingface/dataset',{timeout:60000})
  await page.getByRole('button',{name:'读取任务 HF 来源',exact:true}).click()
  const discovery=await discovering;expect(discovery.status(),await discovery.text()).toBe(200)
  const metadata=(await discovery.json()).data
  expect(metadata.repo_id).toBe(fixed.dataset_id);expect(metadata.revision).toBe(fixed.revision)
  expect(metadata.files.find((f:any)=>f.path===fixed.file)).toMatchObject({size:fixed.file_bytes,supported:true})
  const config=metadata.configs.find((c:any)=>c.splits.some((s:any)=>s.files.includes(fixed.file)))
  expect(config).toBeTruthy();const split=config.splits.find((s:any)=>s.files.includes(fixed.file))
  await page.getByLabel('任务 HF 配置',{exact:true}).selectOption(config.name)
  await page.getByLabel('任务 HF 切分',{exact:true}).selectOption(split.name)
  await page.getByLabel(`${fixed.file} ·`,{exact:false}).check()
  await page.getByLabel('任务 HF 最多行数',{exact:true}).fill('2')
  await page.getByLabel('任务 HF 字节上限',{exact:true}).fill('8388608')
  const columns=[{source:'id',target:'source_id'},{source:'question',target:'question'},{source:'answer',target:'reference_answer'},{source:'contexts',target:'contexts_json',encoding:'json'}]
  for(const [index,column]of columns.entries()){
   await page.getByRole('button',{name:'＋ 添加任务 HF 字段',exact:true}).click()
   await page.getByLabel(`任务 HF 字段${index+1} 原字段`,{exact:true}).fill(column.source)
   await page.getByLabel(`任务 HF 字段${index+1} 目标字段`,{exact:true}).fill(column.target)
   if(column.encoding)await page.getByLabel(`任务 HF 字段${index+1} 编码`,{exact:true}).selectOption(column.encoding)
  }
  await page.getByLabel('同时组合清洗、准备与 SFT 候选',{exact:true}).check()
  await page.getByLabel('任务 SFT 名称',{exact:true}).fill(name)
  await page.getByLabel('标注标准',{exact:true}).selectOption(standard.standard_id)
  await page.getByLabel('已发布标准版本',{exact:true}).selectOption(standard.id)
  const previewing=page.waitForResponse(r=>new URL(r.url()).pathname==='/api/v1/huggingface/preview'&&r.request().method()==='POST',{timeout:120000})
  const drafting=page.waitForResponse(r=>new URL(r.url()).pathname==='/api/v1/governed-tasks'&&r.request().method()==='POST')
  await page.getByRole('button',{name:'预览领域计划',exact:true}).click()
  const previewResponse=await previewing;expect(previewResponse.status(),await previewResponse.text()).toBe(200)
  const preview=(await previewResponse.json()).data,request=previewResponse.request().postDataJSON()
  expect(preview.manifest.rows).toBe(2);expect(preview.manifest.files[0]).toMatchObject({path:fixed.file,sha256:fixed.file_sha256,bytes:fixed.file_bytes})
  expect(preview.manifest.excluded_columns).toContain('is_impossible')
  const draftResponse=await drafting;expect(draftResponse.status(),await draftResponse.text()).toBe(201)
  const draft=(await draftResponse.json()).data
  expect(draft.input_refs||[]).toEqual([]);expect(draft.current_plan_version||0).toBe(0)
  await expect(page.getByText('本次规划调用 0 次 · 未执行领域任务',{exact:true})).toBeVisible()
  const before=await getData<any>(api,`/governed-tasks/${draft.id}/supervisor/summary`);expect(before.children||[]).toEqual([])
  // Replace the local preset by a genuine model proposal. No intercepted route,
  // injected plan, automatic save or alternate fallback is accepted here.
  page.once('dialog',dialog=>dialog.accept())
  const candidate=await clickWrite(page,info,`/governed-tasks/${draft.id}/supervisor/propose`,'重新生成候选')
  expect(candidate.planning_calls).toBe(1);expect(candidate.decision.kind).toBe('proposal');expect(candidate.validation.valid).toBe(true);expect(candidate.saved).toBe(false)
  const sft={name,standard_version_id:standard.id,format:'qa',columns:{question:'question',reference_answer:'reference_answer'}}
  const nodes=checkPlan(candidate.decision.plan,request,sft)
  await capture(page,info,page.getByRole('heading',{name:'候选计划',exact:true}),'support-supervisor-candidate')
  await clickWrite(page,info,`/governed-tasks/${draft.id}/plans`,'保存计划')
  await expect(page.getByRole('heading',{name:'已保存计划 v1',exact:true})).toBeVisible()
  const saved=await getData<any>(api,`/governed-tasks/${draft.id}/plans/1`)
  expect(saved.snapshot).toEqual(candidate.decision.plan)
  await expect(page.getByRole('button',{name:'启动任务',exact:true})).toBeEnabled()
  await clickWrite(page,info,`/governed-tasks/${draft.id}/start`,'启动任务')
  const mutations:string[]=[];const observe=(r:any)=>{if(r.url().includes('/governed-tasks')&&['POST','PUT','DELETE','PATCH'].includes(r.method()))mutations.push(r.url())}
  page.on('request',observe);await page.reload();await expect(page.getByRole('heading',{name:'当前状态汇总',exact:true})).toBeVisible();expect(mutations).toEqual([]);page.off('request',observe)
  const summary=await finishTask(api,draft.id,info)
  await page.getByRole('button',{name:'重新核对执行状态',exact:true}).click();await expect(page.getByText('已成功 4 / 4 个节点',{exact:false})).toBeVisible()
  await expect(page.getByRole('button',{name:'重新核对执行状态',exact:true})).toBeEnabled()
  await capture(page,info,page.getByRole('heading',{name:'当前状态汇总',exact:true}),'support-four-domain-results')
  await page.getByRole('list',{name:'节点执行状态',exact:true}).screenshot({path:info.outputPath('support-four-node-receipts.png')})
  const hfNode=summary.nodes.find((n:any)=>n.capability==='huggingface.import'),sftNode=summary.nodes.find((n:any)=>n.capability==='governance.sft')
  expect(hfNode.domain_kind).toBe('huggingface_import');expect(sftNode.domain_kind).toBe('sft_execution')
  const hfLink=page.getByRole('list',{name:'节点执行状态',exact:true}).locator(`a[href="/hf-datasets?import_id=${hfNode.domain_id}"]`);await expect(hfLink).toHaveCount(1);await hfLink.click()
  await expect(page.getByText('已导入',{exact:true})).toBeVisible()
  const imported=await getData<any>(api,`/huggingface/imports/${hfNode.domain_id}`)
  expect(imported.status).toBe('succeeded');expect(imported.request).toEqual(request);expect(imported.manifest.files[0].sha256).toBe(fixed.file_sha256)
  expect(imported.manifest.rows).toBe(2);expect(imported.output_refs).toHaveLength(1)
  expect(hfNode.output_refs).toEqual(imported.output_refs.map((r:any)=>({asset_id:r.asset_id,version:r.version})))
  const source=await getData<any>(api,`/lifecycle/assets/${imported.output_refs[0].asset_id}?version=${imported.output_refs[0].version}`)
  expect(source.content.columns).toEqual(columns.map(c=>c.target));expect(source.content.rows).toHaveLength(2)
  const originals=source.content.rows.map((row:any[])=>Object.fromEntries(source.content.columns.map((c:string,i:number)=>[c,row[i]])))
  for(const [index,row]of originals.entries()){
   expect(row.source_id).toBe(fixed.rows[index].id);expect(row.question).toBe(fixed.rows[index].question);expect(row.reference_answer).toBe(fixed.rows[index].answer)
   expect(JSON.parse(row.contexts_json).map((d:any)=>hash(d.text))).toEqual(fixed.rows[index].contexts_text_sha256)
  }
  const cleanNode=summary.nodes.find((n:any)=>n.capability==='lifecycle.data-cleaning'),prepareNode=summary.nodes.find((n:any)=>n.capability==='lifecycle.data-preparation')
  // Summary deliberately contains RefIDs, not AssetRef metadata. Resolve the
  // exact returned references through the authorized asset API before using kind.
  const cleanOutputs=await Promise.all(cleanNode.output_refs.map((r:any)=>getData<any>(api,`/lifecycle/assets/${r.asset_id}?version=${r.version}`)))
  const prepareOutputs=await Promise.all(prepareNode.output_refs.map((r:any)=>getData<any>(api,`/lifecycle/assets/${r.asset_id}?version=${r.version}`)))
  const cleaned=cleanOutputs.filter((a:any)=>a.ref.kind==='cleaned_asset'),preparations=prepareOutputs.filter((a:any)=>a.ref.kind==='cleaned_asset')
  expect(cleaned).toHaveLength(1);expect(preparations).toHaveLength(1)
  const cleanRef=cleaned[0].ref,prepared=preparations[0],preparedRef=prepared.ref
  expect(prepared.ref.provenance.preparation.source_ref_id).toBe(refID(cleanRef))
  const wrapper=await getData<any>(api,`/governance-executions/${sftNode.domain_id}`);expect(wrapper.status).toBe('completed');expect(wrapper.domain_kind).toBe('sft_dataset')
  await page.goto(`/tasks?task_id=${draft.id}`);await page.getByRole('list',{name:'节点执行状态',exact:true}).locator(`a[href="/sft-datasets?execution_id=${sftNode.domain_id}"]`).click()
  const dataset=await getData<any>(api,`/sft-datasets/${wrapper.domain_id}`)
  expect(dataset.status).toBe('draft');expect(dataset.source_ref_id).toBe(refID(preparedRef));expect(dataset.standard_version_id).toBe(standard.id);expect(dataset.row_count).toBe(2)
  await expect(page.getByRole('heading',{name,exact:true})).toBeVisible()
  await expect(page.getByRole('button',{name:'发布固定版本',exact:true})).toBeDisabled()
  await page.getByLabel('我已检查数据内容、分组与切分，确认整批审核结论',{exact:true}).check()
  const approved=await clickWrite(page,info,`/sft-datasets/${dataset.id}/review`,'审核通过');expect(approved.status).toBe('approved')
  const version=await clickWrite(page,info,`/sft-datasets/${dataset.id}/publish`,'发布固定版本');expect(version.status).toBe('published')
  const downloading=page.waitForEvent('download');await page.getByRole('button',{name:'导出此版本 JSONL',exact:true}).click();const file=info.outputPath('support-sft.jsonl');await(await downloading).saveAs(file)
  const exported=(await readFile(file,'utf8')).trim().split('\n').map(line=>JSON.parse(line))
  expect(exported).toEqual(originals.map((r:any)=>({messages:[{role:'user',content:r.question.trim()},{role:'assistant',content:r.reference_answer.trim()}],split:'train'})))
  await page.reload();await expect(page.getByRole('button',{name:'导出此版本 JSONL',exact:true})).toBeVisible()
  await capture(page,info,page.getByRole('heading',{name:'数据集样本',exact:true}),'support-sft-reviewed')
  const completedTask=await getData<any>(api,`/governed-tasks/${draft.id}`),publishedDataset=await getData<any>(api,`/sft-datasets/${dataset.id}`)
  await attachJSON(info,'support-restore-manifest',{task_id:draft.id,task_version:completedTask.version,execution_plan_version:completedTask.execution_plan_version,status:completedTask.status,children:summary.children.map((child:any)=>({id:child.id,node_id:child.node_id,domain_kind:child.domain_kind,domain_id:child.domain_id,attempt:child.attempt})),nodes:summary.nodes.map((node:any)=>({node_id:node.node_id,capability:node.capability,status:node.status,domain_kind:node.domain_kind,domain_id:node.domain_id,attempt:node.attempt})),hf:{id:imported.id,status:imported.status,request:imported.request,manifest:imported.manifest,output_refs:imported.output_refs},sft:{execution_id:sftNode.domain_id,dataset_id:dataset.id,dataset_version:publishedDataset.version,source_ref_id:dataset.source_ref_id,standard_version_id:dataset.standard_version_id,content_digest:dataset.content_digest,row_count:dataset.row_count,published_version_id:version.id,published_content_digest:version.content_digest,name:dataset.name}})
  await attachJSON(info,'hf-supervisor-sft-evidence',{fixed,metadata,preview,request,draft,candidate,nodes,saved,summary,imported,source,prepared,wrapper,dataset,version,exported})
  // Knowledge takes only actual original contexts via a product projection. QA
  // answers remain in the SFT branch and are never submitted as KB content.
  const knowledge=await writeScenario(api,'/knowledge-bases',{name:`TechQA contexts ${randomUUID()}`,is_public:false,graph_enabled:false});kb=knowledge.id
  await knowledgeBranch(page,api,info,{kb,kbName:knowledge.name,cleanRef,originals})
 }finally{
  try{if(kb){const response=await api.delete(`/api/v1/knowledge-bases/${kb}`);await attachJSON(info,'support-knowledge-cleanup',{kb,status:response.status()});expect(response.ok()).toBe(true)}}finally{await api.dispose()}
 }
})

test('technical support: original unanswered record keeps its sentinel and empty contexts fail the preparation UI',async({page,playwright},info)=>{
 test.skip(process.env.COGNIDA_E2E_SUPPORT_SUPERVISOR!=='1','Requires explicitly enabled isolated HF and lifecycle services')
 test.setTimeout(180000)
 const api=await loginThroughUI(page,playwright)
 try{
  // Setup reads the real immutable file once. There is no second preview or
  // handcrafted empty source; original '-' is not interpreted as a blank answer.
  const metadata=await getData<any>(api,`/huggingface/dataset?repo_id=${encodeURIComponent(fixed.dataset_id)}&revision=${fixed.revision}`)
  expect(metadata.revision).toBe(fixed.revision)
  const config=metadata.configs.find((c:any)=>c.splits.some((s:any)=>s.files.includes(fixed.file)))
  expect(config).toBeTruthy();const split=config.splits.find((s:any)=>s.files.includes(fixed.file))
  const request={repo_id:fixed.dataset_id,revision:fixed.revision,config:config.name,split:split.name,files:[fixed.file],max_rows:3,max_bytes:8388608,field_mapping:{columns:[{source:'id',target:'source_id'},{source:'answer',target:'reference_answer'},{source:'contexts',target:'contexts_json',encoding:'json'},{source:'is_impossible',target:'is_impossible'}]}}
  const imported=await writeScenario(api,'/huggingface/imports',request)
  await attachJSON(info,'negative-original-import',{request,imported})
  expect(imported.status).toBe('succeeded');expect(imported.manifest.rows).toBe(3)
  expect(imported.manifest.files[0]).toMatchObject({sha256:fixed.file_sha256,bytes:fixed.file_bytes})
  expect(imported.output_refs).toHaveLength(1)
  const sourceRef=imported.output_refs[0],source=await getData<any>(api,`/lifecycle/assets/${sourceRef.asset_id}?version=${sourceRef.version}`)
  expect(source.content.rows).toHaveLength(3)
  const original=Object.fromEntries(source.content.columns.map((c:string,i:number)=>[c,source.content.rows[2][i]]))
  expect(original).toMatchObject({source_id:fixed.negative_row.id,reference_answer:fixed.negative_row.answer,is_impossible:true})
  expect(JSON.parse(original.contexts_json)).toEqual([])
  const clean=await writeScenario(api,'/lifecycle/invocations',{agent_id:'data-cleaning',input_refs:[{asset_id:sourceRef.asset_id,version:sourceRef.version}],options:{operations:['trim'],min_score:0}})
  const cleanRun=await waitInvocation(api,clean.id),cleanRef=cleanRun.output.refs.find((r:any)=>r.kind==='cleaned_asset')
  expect(cleanRef).toBeTruthy()
  const cleaned=await getData<any>(api,`/lifecycle/assets/${cleanRef.asset_id}?version=${cleanRef.version}`)
  expect(cleaned.content.rows).toHaveLength(3)
  expect(cleaned.content.rows[2][cleaned.content.columns.indexOf('reference_answer')]).toBe('-')
  expect(JSON.parse(cleaned.content.rows[2][cleaned.content.columns.indexOf('contexts_json')])).toEqual([])
  await page.goto(`/data-preparation?asset=${refID(cleanRef)}`)
  await expect(page.getByLabel('来源资产',{exact:true})).toHaveValue(refID(cleanRef))
  await page.getByLabel('将资料列整理为知识正文',{exact:true}).check()
  await page.getByLabel('资料 JSON 列',{exact:true}).fill('contexts_json')
  await page.getByLabel('数组对象的正文字段',{exact:true}).fill('text')
  await page.getByLabel('来源 ID 列',{exact:true}).fill('source_id')
  const submitted=await clickWrite(page,info,`/lifecycle/assets/${cleanRef.asset_id}/prepare`,'开始标准化')
  let failed:any
  try{
   await expect.poll(async()=>{failed=(await getData<any>(api,`/lifecycle/invocations/${submitted.id}`)).invocation;return ['succeeded','failed','cancelled','quarantined'].includes(failed.status)},{timeout:60000,intervals:[250,500,1000]}).toBe(true)
  }finally{await attachJSON(info,'negative-empty-contexts-projection',{original,source,cleanRun,cleaned,submitted,final:failed})}
  expect(failed.status).toBe('failed');expect(failed.output?.refs||[]).toEqual([])
  expect(failed.error).toContain('require 1-256 source documents in total')
  await page.getByRole('button',{name:'刷新状态',exact:true}).click()
  const result=page.getByRole('region',{name:'标准化执行结果',exact:true})
  await expect(result.locator('.wb-badge')).toHaveText('失败')
  await expect(result.getByText('尚未获得准备产物，以真实执行结果为准。',{exact:true})).toBeVisible()
  await expect(result.getByRole('link',{name:'用此数据标注',exact:true})).toHaveCount(0)
  await capture(page,info,result.getByRole('heading',{name:'标准化执行结果',exact:true}),'original-empty-contexts-rejected')
 }finally{await api.dispose()}
})

async function knowledgeBranch(page:Page,api:APIRequestContext,info:TestInfo,input:{kb:string;kbName:string;cleanRef:any;originals:any[]}){
 const {kb,kbName,cleanRef,originals}=input
 const cleaned=await getData<any>(api,`/lifecycle/assets/${cleanRef.asset_id}?version=${cleanRef.version}`)
 const contextIndex=cleaned.content.columns.indexOf('contexts_json'),idIndex=cleaned.content.columns.indexOf('source_id')
 expect(contextIndex).toBeGreaterThanOrEqual(0);expect(idIndex).toBeGreaterThanOrEqual(0)
 // Independent original-file hashes prove cleaning did not rewrite source docs.
 for(const[index,row]of cleaned.content.rows.entries())expect(JSON.parse(row[contextIndex]).map((d:any)=>hash(d.text))).toEqual(fixed.rows[index].contexts_text_sha256)
 await page.goto(`/data-preparation?asset=${refID(cleanRef)}`)
 await expect(page.getByLabel('来源资产',{exact:true})).toHaveValue(refID(cleanRef))
 await page.getByLabel('将资料列整理为知识正文',{exact:true}).check()
 await page.getByLabel('资料 JSON 列',{exact:true}).fill('contexts_json')
 await page.getByLabel('数组对象的正文字段',{exact:true}).fill('text')
 await page.getByLabel('来源 ID 列',{exact:true}).fill('source_id')
 const submitted=await clickWrite(page,info,`/lifecycle/assets/${cleanRef.asset_id}/prepare`,'开始标准化')
 const run=await waitInvocation(api,submitted.id),preparedRef=run.output.refs.find((r:any)=>r.kind==='cleaned_asset')
 expect(preparedRef).toBeTruthy()
 const prepared=await getData<any>(api,`/lifecycle/assets/${preparedRef.asset_id}?version=${preparedRef.version}`)
 expect(prepared.content.kind).toBe('text')
 expect(prepared.ref.provenance.preparation).toMatchObject({source_ref_id:refID(cleanRef),text_projection:{column:'contexts_json',text_field:'text',id_column:'source_id'}})
 const onlyContexts=cleaned.content.rows.flatMap((row:any[],ri:number)=>JSON.parse(row[contextIndex]).map((d:any,di:number)=>`[Source row ${ri+1}, id ${JSON.stringify(String(row[idIndex]))}, document ${di+1}]\n${d.text.replace(/\r\n?/g,'\n')}`)).join('\n\n')
 expect(prepared.content.text,'KB payload must be exactly the selected contexts with documented source separators; no question/answer columns are appended').toBe(onlyContexts)
 const standard=await publishScenarioStandard(api,'tagging',[{id:'technical-support',name:'technical-support',keywords:['Streams','IBM']}])
 await openScenarioLabeling(page,preparedRef,standard)
 await page.getByLabel('任务名称',{exact:true}).fill(`TechQA source documents ${randomUUID().slice(0,8)}`)
 const task=await clickWrite(page,info,'/tagging-tasks','创建并执行打标')
 let items:any[]=[]
 await expect.poll(async()=>{items=(await getData<any>(api,`/tagging-tasks/${task.id}/results`)).items;return items.length},{timeout:30000}).toBe(1)
 expect(items[0].content).toBe(onlyContexts);expect(items[0].review_status).toBe('pending')
 await page.getByRole('button',{name:'刷新结果',exact:true}).click()
 await page.locator('.wb-list button').filter({hasText:items[0].content.slice(0,70)}).click()
 const reviewed=await clickWrite(page,info,`/tagging-results/${items[0].id}/review`,'审核通过')
 expect(reviewed.review_status).toBe('approved')
 // Real publication is separate and pauses at the actual approval boundary.
 await page.goto(`/experts?asset=${refID(preparedRef)}&agent_id=data-asset`)
 await expect(page.getByLabel(/^输入资产/)).toHaveValue(refID(preparedRef))
 await page.getByLabel('目标知识库',{exact:false}).selectOption(kb)
 const publicationStart=await clickWrite(page,info,'/lifecycle/invocations','开始执行')
 let detail:any
 await expect.poll(async()=>{detail=await getData<any>(api,`/lifecycle/invocations/${publicationStart.id}`);return detail.invocation.status},{timeout:30000}).toBe('awaiting_approval')
 expect(detail.invocation.output?.refs||[]).toEqual([])
 const approval=detail.approvals.find((a:any)=>['pending','awaiting_approval'].includes(a.status));expect(approval).toBeTruthy()
 await expect(page.getByRole('button',{name:'批准并继续',exact:true})).toBeVisible()
 await page.getByLabel('审核说明',{exact:true}).fill('已核对固定 TechQA 来源与 contexts 投影；正文未追加问答列，标签已由人工审核。')
 await clickWrite(page,info,`/lifecycle/approvals/${approval.id}/resolve`,'批准并继续')
 const publishedRun=await waitInvocation(api,publicationStart.id),publishedRef=publishedRun.output.refs.find((r:any)=>r.kind==='published_asset')
 expect(publishedRef).toBeTruthy()
 const published=await getData<any>(api,`/lifecycle/assets/${publishedRef.asset_id}?version=${publishedRef.version}`)
 expect(published.content.text).toBe(onlyContexts);expect(published.content.publication).toMatchObject({publication_status:'published',knowledge_base_id:kb})
 const knowledgeID=published.content.publication.knowledge_id;expect(knowledgeID).toBeTruthy()
 // The optional bridge uses the authoritative published ref, never a manually
 // supplied knowledge ID or same-text search to invent a parent relationship.
 await page.goto(`/labeling?kind=tagging&task_id=${task.id}`)
 await page.locator('.wb-list button').filter({hasText:items[0].content.slice(0,70)}).click()
 await page.getByLabel('关联后续发布的知识产物',{exact:true}).check()
 await selectPagedAsset(page,page.getByLabel('已发布知识产物',{exact:true}),refID(publishedRef))
 const targetReading=page.waitForResponse(r=>new URL(r.url()).pathname===`/api/v1/tagging-results/${items[0].id}/apply-target`)
 const applied=await clickWrite(page,info,`/tagging-results/${items[0].id}/apply`,'应用已审核标签')
 const targetResponse=await targetReading;expect(targetResponse.status()).toBe(200);const target=(await targetResponse.json()).data
 expect(target.publication_ref_id).toBe(refID(publishedRef));expect(target.knowledge_id).toBe(knowledgeID)
 expect(applied.knowledge_id).toBe(knowledgeID);expect(applied.labels).toContain('technical-support')
 await verifyVectorRetrieval(page,info,kb,knowledgeID,'IBM Streams system service ODBCINI SQL08003','ODBCINI')
 await page.getByLabel('已审核知识标签',{exact:true}).fill('technical-support')
 const filtered=await clickWrite(page,info,'/knowledge/search','搜索')
 expect(filtered.items.some((i:any)=>i.knowledge_id===knowledgeID&&i.sources.includes('milvus_vector'))).toBe(true)
 for(const item of filtered.items)expect(item.sources).not.toContain('database_lexical')
 await page.getByLabel('已审核知识标签',{exact:true}).fill(`missing-${randomUUID()}`)
 const excluded=await clickWrite(page,info,'/knowledge/search','搜索');expect(excluded.items).toEqual([])
 await attachJSON(info,'support-reviewed-knowledge',{originalSourceIds:originals.map(r=>r.source_id),cleanRef,prepared,run,standard,task,items,reviewed,publicationStart,approval,publishedRun,published,target,applied,filtered,excluded})
 await page.goto('/kb-assistant')
 await page.getByRole('button',{name:'手动',exact:true}).click()
 await page.locator('.kb-row').filter({hasText:kbName}).click()
 const query=`${fixed.rows[0].question}\nUse only the selected knowledge base. Explain the documented cause and workaround, and cite the retrieved source. Do not invent missing commands.`
 await page.getByPlaceholder('就当前知识库提问…').fill(query)
 const streaming=page.waitForResponse(r=>new URL(r.url()).pathname==='/api/v1/agent/knowledge/stream'&&r.request().method()==='POST',{timeout:120000})
 await page.locator('.btn-send').click();const response=await streaming
 expect(response.status()).toBe(200);expect(response.request().postDataJSON()).toMatchObject({query,kb_ids:[kb],kb_scope_mode:'manual',graph_enabled:false})
 expect(response.headers()['content-type']).toMatch(/^text\/event-stream\s*;\s*charset=utf-8$/i)
 const stream=await response.text()
 const events=stream.split(/\r?\n\r?\n/).flatMap(frame=>{const lines=frame.split(/\r?\n/),data=lines.filter(l=>l.startsWith('data:')).map(l=>l.slice(5).trimStart()).join('\n');if(!data||data==='[DONE]')return[];const event=lines.find(l=>l.startsWith('event:'))?.slice(6).trim();return[{event,...JSON.parse(data)}]})
 await attachJSON(info,'support-agent-actual-events',{query,kb,knowledgeID,events})
 expect(events.filter(e=>e.event==='error'||(e.event==='step'&&e.type==='error'))).toEqual([])
 const done=events.filter(e=>e.event==='done').at(-1);expect(done).toBeTruthy()
 expect(done.persistence_failed).toBe(false);expect(done.partial===true).toBe(false);const answer=done.answer||done.content;expect(answer).toMatch(/ODBCINI/i);expect(answer).toMatch(/streamtool/i)
 const actualRetrievals=events.filter(e=>e.event==='step'&&e.type==='tool_result'&&e.tool_name==='rag_query'&&e.status==='success'&&!e.error)
 const contexts=actualRetrievals.flatMap(e=>JSON.parse(e.tool_output).chunks||[])
 expect(contexts.length).toBeGreaterThan(0)
 // Verify durable state through the same authorized session API used by the
 // page, rather than accepting the terminal persistence flag alone.
 expect(done.session_id).toBeTruthy()
 const session=await getData<any>(api,`/sessions/${encodeURIComponent(done.session_id)}/detail`)
 await attachJSON(info,'support-agent-persisted-session',session)
 expect(session.session.id).toBe(done.session_id)
 const assistants=session.messages.filter((message:any)=>message.role==='assistant')
 expect(assistants).toHaveLength(1);expect(assistants[0].session_id).toBe(done.session_id);expect(assistants[0].content).toBe(answer)
 const savedSteps=JSON.parse(assistants[0].agent_steps).steps
 const retrievalShape=(step:any)=>({tool_id:step.tool_id,tool_name:step.tool_name,status:step.status,output:JSON.parse(step.tool_output)})
 expect(savedSteps.filter((step:any)=>step.type==='tool_result'&&step.tool_name==='rag_query'&&step.status==='success'&&!step.error).map(retrievalShape)).toEqual(actualRetrievals.map(retrievalShape))
 const chunks=(await getData<any>(api,`/knowledge-bases/${kb}/chunks?knowledge_id=${knowledgeID}&page=1&page_size=100`)).items
 const chunkIDs=new Set(chunks.map((c:any)=>c.id))
 expect(contexts.some(c=>c.knowledge_id===knowledgeID&&c.knowledge_base_id===kb&&chunkIDs.has(c.chunk_id)&&c.content.includes('ODBCINI'))).toBe(true)
 await expect(page.locator('.composer__textarea')).toBeEnabled({timeout:120000})
 await expect(page.locator('.bubble--ai').last()).toContainText('ODBCINI')
 await expect(page.getByRole('region',{name:'回答检索来源'}).last()).toBeVisible()
 await capture(page,info,page.locator('.bubble--ai').last(),'support-agent-answer')
 const sources=page.getByRole('region',{name:'回答检索来源'}).last()
 await sources.locator('summary').first().click()
 await sources.screenshot({path:info.outputPath('support-agent-source-receipts.png')})
}
