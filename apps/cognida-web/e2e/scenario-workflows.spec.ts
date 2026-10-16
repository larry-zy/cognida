import {randomUUID} from 'node:crypto'
import {readFile} from 'node:fs/promises'
import {expect,test,type Page} from '@playwright/test'
import {loginThroughUI,getData,attachJSON} from './lifecycle.helpers'
import {writeScenario,prepareScenarioInput,publishScenarioStandard,openScenarioLabeling,waitInvocation} from './scenario.helpers'
async function action(page:Page,path:string,button:string){const wait=page.waitForResponse(r=>new URL(r.url()).pathname===`/api/v1${path}`&&r.request().method()==='POST',{timeout:120000});await page.getByRole('button',{name:button,exact:true}).click();const response=await wait;const body=await response.text();if(path.startsWith('/huggingface/'))await attachJSON(test.info(),`hf-${path.split('/').pop()}-response`,{request:response.request().postDataJSON(),status:response.status(),request_id:response.headers()['x-request-id'],body});expect(response.ok(),`${path}: ${response.status()} ${body}`).toBe(true);return JSON.parse(body).data}
async function captureSFT(page:Page,info:any,prefix:string){
 await expect(page.locator('body')).not.toHaveClass(/page-transitioning/)
 await expect(page.locator('.platform-container')).toHaveCSS('opacity','1')
 // The application scrolls inside content-wrapper. A full main-element shot is
 // clipped by that ancestor, so retain actual viewports of the readable sections.
 for(const [label,suffix] of [['整批审核说明','review'],['数据集样本','rows'],['已发布版本','published']]){
  const anchor=label==='整批审核说明'?page.getByLabel(label,{exact:true}):page.getByRole('heading',{name:label,exact:true})
  await anchor.evaluate(element=>element.scrollIntoView({block:'center'}))
  await expect(anchor).toBeInViewport()
  await page.screenshot({path:info.outputPath(`${prefix}-${suffix}.png`)})
 }
}
async function captureAnnotation(page:Page,info:any,content:string,path:string){
 await expect(page.getByRole('button',{name:'生成预标注',exact:true})).toBeEnabled()
 await page.locator('.wb-list button').filter({hasText:content.slice(0,70)}).click()
 await expect(page.getByRole('button',{name:'保存标注',exact:true})).toBeEnabled()
 await expect(page.locator('body')).not.toHaveClass(/page-transitioning/)
 await expect(page.locator('.platform-container')).toHaveCSS('opacity','1')
 const heading=page.getByRole('heading',{name:'审核工作区',exact:true})
 await heading.evaluate(element=>element.scrollIntoView({block:'start'}))
 await expect(heading).toBeInViewport()
 await page.screenshot({path:info.outputPath(path)})
}
async function publishQAThroughUI(page:Page){await page.goto('/label-standards');await page.getByLabel('标准名称',{exact:true}).fill(`QA-${randomUUID()}`);await page.getByLabel('任务类型',{exact:true}).selectOption('qa');await page.getByLabel('总则与判定规则',{exact:true}).fill('保留来源中已存在的问题和参考答案；不生成答案。逐批检查问题、回答、工具调用和数据划分后审核。');const head=await action(page,'/label-standards','保存草稿');expect(head.constraints).toEqual({max_labels:0,max_entities:0,max_relations:0,allow_empty:false});const version=await action(page,`/label-standards/${head.id}/publish`,'发布此修订');expect(version.standard_id).toBe(head.id);expect(version.status).toBe('published');await expect(page.getByRole('link',{name:'构建 SFT 数据集',exact:true}).last()).toBeVisible();return version}
async function reviewPublishExport(page:Page,api:any,info:any,dataset:any){await expect(page.getByRole('button',{name:'发布固定版本',exact:true})).toBeDisabled();await page.getByLabel('我已检查数据内容、分组与切分，确认整批审核结论',{exact:true}).check();const approved=await action(page,`/sft-datasets/${dataset.id}/review`,'审核通过');expect(approved.status).toBe('approved');const version=await action(page,`/sft-datasets/${dataset.id}/publish`,'发布固定版本');expect(version.status).toBe('published');expect(version.content_digest).toHaveLength(64);const downloading=page.waitForEvent('download');await page.getByRole('button',{name:'导出此版本 JSONL',exact:true}).click();const download=await downloading;const file=info.outputPath('sft-export.jsonl');await download.saveAs(file);const rows=(await readFile(file,'utf8')).trim().split('\n').map(line=>JSON.parse(line));const exact=await getData<any>(api,`/sft-dataset-versions/${version.id}`);expect(exact.content_digest).toBe(version.content_digest);await page.getByRole('link',{name:'打开此数据集记录链接',exact:true}).click();await page.reload();await expect(page.getByRole('heading',{name:dataset.name,exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'导出此版本 JSONL',exact:true})).toBeVisible();return{approved,version,rows}}
test.describe('independent prepared-data and published-standard workflows',()=>{
 test.beforeEach(()=>test.skip(process.env.COGNIDA_E2E_SCENARIO!=='1','Requires isolated scenario services; no model generation is used'))
 test('existing asset → independent cleaning UI → typed preparation UI → published classification standard → annotation',async({page,playwright},info)=>{const api=await loginThroughUI(page,playwright);try{
  const source=await writeScenario(api,'/lifecycle/assets',{name:'Existing independent input',input:{kind:'table',columns:['query','amount_value','ignored'],rows:[['  查询火车时刻  ',12.5,'not mapped'],['取消火车提醒',3,'not mapped']]}})
  await page.goto(`/data-preparation?asset=${source.asset_id}:${source.version}`)
  await expect(page.getByRole('button',{name:'开始标准化',exact:true})).toBeDisabled()
  await page.getByRole('link',{name:'使用此数据进行清洗',exact:true}).click()
  await expect(page.getByLabel(/^输入资产/)).toHaveValue(`${source.asset_id}:${source.version}`)
  await expect(page.getByLabel(/^专家/)).toHaveValue('data-cleaning')
  const submitted=await action(page,'/lifecycle/invocations','开始执行'),clean=await waitInvocation(api,submitted.id)
  const cleanRef=clean.output.refs.find((r:any)=>r.kind==='cleaned_asset');expect(cleanRef).toBeTruthy()
  await page.goto(`/data-preparation?asset=${cleanRef.asset_id}:${cleanRef.version}`)
  await expect(page.getByLabel('来源资产',{exact:true})).toHaveValue(`${cleanRef.asset_id}:${cleanRef.version}`)
  await page.getByLabel('显式选择、重命名并校验字段类型',{exact:true}).check()
  for(const [i,original,target,type] of [[1,'query','text','text'],[2,'amount_value','amount','number']] as const){
   await page.getByRole('button',{name:'＋ 添加字段映射',exact:true}).click()
   await page.getByLabel(`字段映射${i} 原字段名`,{exact:true}).fill(original)
   await page.getByLabel(`字段映射${i} 目标字段名`,{exact:true}).fill(target)
   await page.getByLabel(`字段映射${i} 类型`,{exact:true}).selectOption(type)
   await page.getByLabel(`字段映射${i} 必填`,{exact:true}).selectOption('true')
  }
  const preparing=await action(page,`/lifecycle/assets/${cleanRef.asset_id}/prepare`,'开始标准化'),done=await waitInvocation(api,preparing.id)
  const prepared=done.output.refs.find((r:any)=>r.kind==='cleaned_asset');expect(prepared.provenance.preparation.rows).toBe(2)
  const preparedData=await getData<any>(api,`/lifecycle/assets/${prepared.asset_id}?version=${prepared.version}`)
  expect(preparedData.content.columns).toEqual(['text','amount']);expect(preparedData.content.rows).toEqual([['查询火车时刻',12.5],['取消火车提醒',3]])
  await page.goto('/label-standards');await page.getByLabel('标准名称',{exact:true}).fill(`火车请求-${randomUUID()}`);await page.getByLabel('总则与判定规则',{exact:true}).fill('仅标注明确包含火车的请求；不推断未提及的交通工具。')
  await page.getByRole('button',{name:'添加标签',exact:true}).click();await page.getByLabel('标签 ID',{exact:true}).fill('train');await page.getByLabel('标签名称',{exact:true}).fill('火车请求');await page.getByLabel('定义',{exact:true}).fill('明确提及火车的出行或提醒请求。');await page.getByLabel('匹配关键词（每行一项）',{exact:true}).fill('火车');await page.getByLabel('标签名称',{exact:true}).focus()
  const head=await action(page,'/label-standards','保存草稿'),standard=await action(page,`/label-standards/${head.id}/publish`,'发布此修订')
  await openScenarioLabeling(page,prepared,standard);await page.getByLabel('任务名称',{exact:true}).fill(`独立标注-${randomUUID()}`)
  const task=await action(page,'/annotation-tasks','创建标注任务');expect(task.source_ref_id).toBe(`${prepared.asset_id}:${prepared.version}`);expect(task.standard_version_id).toBe(standard.id)
  await action(page,`/annotation-tasks/${task.id}/preannotate`,'生成预标注')
  const rows=await getData<any>(api,`/annotation-tasks/${task.id}/items`)
  await attachJSON(info,'existing-clean-prepare-standard-annotation',{source,clean,done,preparedData,standard,task,rows})
  expect(rows.items).toHaveLength(2);for(const row of rows.items){expect(row.review_status).toBe('pending');expect(row.annotation.labels).toEqual(['train'])}
  await captureAnnotation(page,info,rows.items[0].content,'independent-classification.png')
 }finally{await api.dispose()}})
 test('QA standard UI → prepared existing table → SFT review → immutable export and refresh',async({page,playwright},info)=>{const api=await loginThroughUI(page,playwright);try{
  const standard=await publishQAThroughUI(page)
  const source=await writeScenario(api,'/lifecycle/assets',{name:'SFT explicit existing QA',input:{kind:'table',columns:['question','answer','split','group'],rows:[['如何开启提醒？','在设置中开启提醒。','train','a'],['如何关闭通知？','在设置中关闭通知。','test','b']]}})
  const prepared=await prepareScenarioInput(api,source)
  await page.goto(`/sft-datasets?source_ref=${prepared.asset_id}:${prepared.version}&standard_version_id=${standard.id}`)
  await expect(page.getByLabel('已发布标准版本',{exact:true})).toHaveValue(standard.id)
  const name=`SFT-QA-${randomUUID()}`;await page.getByLabel('数据集名称',{exact:true}).fill(name);await page.getByLabel('问题列',{exact:true}).fill('question');await page.getByLabel('参考答案列',{exact:true}).fill('answer');await page.getByLabel('split 列（可选）',{exact:true}).fill('split');await page.getByLabel('分组列（可选）',{exact:true}).fill('group')
  const dataset=await action(page,'/sft-datasets','构建待审核数据集');expect(dataset.status).toBe('draft');expect(dataset.row_count).toBe(2);expect(dataset.split_counts).toEqual({train:1,validation:0,test:1})
  const evidence=await reviewPublishExport(page,api,info,dataset);expect(evidence.rows).toEqual([{messages:[{role:'user',content:'如何开启提醒？'},{role:'assistant',content:'在设置中开启提醒。'}],split:'train'},{messages:[{role:'user',content:'如何关闭通知？'},{role:'assistant',content:'在设置中关闭通知。'}],split:'test'}])
  await attachJSON(info,'qa-standard-sft-evidence',{standard,source,prepared,dataset,...evidence});await captureSFT(page,info,'sft-qa')
 }finally{await api.dispose()}})
 test('existing messages/tool-calling JSON → SFT audit retains exact tool arguments',async({page,playwright},info)=>{const api=await loginThroughUI(page,playwright);try{
  const standard=await publishScenarioStandard(api,'qa')
  const messages=[{role:'user',content:'查询北京天气'},{role:'assistant',tool_calls:[{id:'weather-call',type:'function',function:{name:'weather',arguments:'{"city":"北京"}'}}]},{role:'tool',tool_call_id:'weather-call',content:'晴'},{role:'assistant',content:'北京天气晴'}]
  const tools=[{type:'function',function:{name:'weather',description:'Read weather',parameters:{type:'object',properties:{city:{type:'string'}},required:['city'],additionalProperties:false}}}]
  const source=await writeScenario(api,'/lifecycle/assets',{name:'Existing tool calling conversations',input:{kind:'table',columns:['messages','tools'],rows:[[JSON.stringify(messages),JSON.stringify(tools)]]}}),prepared=await prepareScenarioInput(api,source)
  await page.goto(`/sft-datasets?source_ref=${prepared.asset_id}:${prepared.version}&standard_version_id=${standard.id}`);await expect(page.getByLabel('已发布标准版本',{exact:true})).toHaveValue(standard.id);await page.getByLabel('数据集名称',{exact:true}).fill(`SFT-tools-${randomUUID()}`);await page.getByLabel('数据格式',{exact:true}).selectOption('messages');await page.getByLabel('messages 列',{exact:true}).fill('messages');await page.getByLabel('tools 列（可选）',{exact:true}).fill('tools');const dataset=await action(page,'/sft-datasets','构建待审核数据集');const evidence=await reviewPublishExport(page,api,info,dataset);expect(evidence.rows).toEqual([{messages,tools,split:'train'}]);await attachJSON(info,'tool-calling-sft-evidence',{standard,prepared,dataset,...evidence});await captureSFT(page,info,'sft-toolcalling')
 }finally{await api.dispose()}})
 test('HF fixed public sample → real clean → prepare → standard-bound annotation without gold fields',async({page,playwright},info)=>{const api=await loginThroughUI(page,playwright);try{
  const standard=await publishScenarioStandard(api,'classification',[{id:'card',name:'银行卡问题',keywords:['card']}])
  await page.goto('/hf-datasets');await page.getByLabel('数据集仓库',{exact:true}).fill('mteb/banking77');await page.getByLabel('revision',{exact:true}).fill('18072d2685ea682290f7b8924d94c62acc19c0b2');const reading=page.waitForResponse(r=>new URL(r.url()).pathname==='/api/v1/huggingface/dataset',{timeout:60000});await page.getByRole('button',{name:'读取固定版本',exact:true}).click();const read=await reading;expect(read.ok(),await read.text()).toBe(true)
  await page.getByLabel('配置',{exact:true}).selectOption('default');await page.getByLabel('数据切分',{exact:true}).selectOption('train');await page.getByLabel('data/train-00000-of-00001.parquet',{exact:false}).check();await page.getByLabel('最多导入行数',{exact:true}).fill('3');await page.getByLabel('文本字段',{exact:true}).fill('text');const preview=await action(page,'/huggingface/preview','预览选定字段');expect(preview.manifest.revision).toBe('18072d2685ea682290f7b8924d94c62acc19c0b2');expect(preview.manifest.rows).toBe(3);expect(preview.columns).toEqual(['source_id','text']);expect(preview.manifest.excluded_columns).toContain('label');expect(preview.manifest.excluded_columns).toContain('label_text')
  const imported=await action(page,'/huggingface/imports','确认预览并导入');expect(imported.status).toBe('succeeded');expect(imported.manifest.rows).toBe(3);expect(imported.output_refs).toHaveLength(1);const source=await getData<any>(api,`/lifecycle/assets/${imported.output_refs[0].asset_id}?version=${imported.output_refs[0].version}`);expect(source.content.columns).toEqual(['source_id','text']);expect(source.content.rows).toHaveLength(3)
  await page.getByRole('link',{name:'清洗此资产',exact:true}).click();await expect(page.getByLabel(/^输入资产/)).toHaveValue(`${source.ref.asset_id}:${source.ref.version}`);await expect(page.getByLabel(/^专家/)).toHaveValue('data-cleaning');const waiting=page.waitForResponse(r=>new URL(r.url()).pathname==='/api/v1/lifecycle/invocations'&&r.request().method()==='POST',{timeout:120000});await page.getByRole('button',{name:'开始执行',exact:true}).click();const response=await waiting;expect(response.ok(),await response.text()).toBe(true);const clean=await waitInvocation(api,(await response.json()).data.id);const cleanRef=clean.output.refs.find((r:any)=>r.kind==='cleaned_asset');expect(cleanRef).toBeTruthy()
  await page.goto(`/data-preparation?asset=${cleanRef.asset_id}:${cleanRef.version}`);await expect(page.getByLabel('来源资产',{exact:true})).toHaveValue(`${cleanRef.asset_id}:${cleanRef.version}`);const prep=await action(page,`/lifecycle/assets/${cleanRef.asset_id}/prepare`,'开始标准化');const done=await waitInvocation(api,prep.id);const prepared=done.output.refs.find((r:any)=>r.kind==='cleaned_asset');expect(prepared.provenance.preparation.rows).toBe(3)
  await openScenarioLabeling(page,prepared,standard);await page.getByLabel('任务名称',{exact:true}).fill(`HF-banking-${randomUUID()}`);const task=await action(page,'/annotation-tasks','创建标注任务');expect(task.standard_version_id).toBe(standard.id);await action(page,`/annotation-tasks/${task.id}/preannotate`,'生成预标注');const rows=await getData<any>(api,`/annotation-tasks/${task.id}/items`);expect(rows.items).toHaveLength(3);expect(rows.items.every((r:any)=>r.review_status==='pending')).toBe(true);await attachJSON(info,'hf-clean-prepare-annotation-evidence',{preview,imported,source,clean,done,standard,task,rows});await captureAnnotation(page,info,rows.items[0].content,'hf-prepared-annotation.png')
 }finally{await api.dispose()}})
})
