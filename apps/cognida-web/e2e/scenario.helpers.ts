import {randomUUID} from 'node:crypto'
import {expect,type APIRequestContext,type Page} from '@playwright/test'
import {getData} from './lifecycle.helpers'
export interface ScenarioRef {asset_id:string;version:number;kind?:string}
export async function writeScenario<T=any>(api:APIRequestContext,path:string,data:unknown):Promise<T>{const response=await api.post(`/api/v1${path}`,{data,headers:{'Idempotency-Key':randomUUID()}});expect(response.ok(),`${path}: ${response.status()} ${await response.text()}`).toBe(true);return (await response.json()).data}
/** Real cleanup + standardization, never a forged provenance flag. */
export async function prepareScenarioInput(api:APIRequestContext,source:ScenarioRef):Promise<ScenarioRef>{
 const clean=await writeScenario(api,'/lifecycle/invocations',{agent_id:'data-cleaning',input_refs:[{asset_id:source.asset_id,version:source.version}],options:{operations:['trim','normalize_ws'],min_score:0}})
 const cleanRun=await waitInvocation(api,clean.id)
 const cleanRef=cleanRun.output.refs.find((r:ScenarioRef)=>r.kind==='cleaned_asset');expect(cleanRef).toBeTruthy()
 const prepare=await writeScenario(api,`/lifecycle/assets/${cleanRef.asset_id}/prepare`,{version:cleanRef.version,fields:[],normalize_newlines:true})
 const preparedRun=await waitInvocation(api,prepare.id)
 const ref=preparedRun.output.refs.find((r:ScenarioRef)=>r.kind==='cleaned_asset');expect(ref).toBeTruthy()
 const read=await getData<any>(api,`/lifecycle/assets/${ref.asset_id}?version=${ref.version}`)
 expect(read.ref.provenance.preparation.version).toBe(1);expect(read.ref.provenance.preparation.output_digest).toHaveLength(64)
 return ref
}
export async function waitInvocation(api:APIRequestContext,id:string){let invocation:any;await expect.poll(async()=>{invocation=(await getData<any>(api,`/lifecycle/invocations/${id}`)).invocation;return ['succeeded','failed','cancelled','quarantined'].includes(invocation.status)},{timeout:60000,intervals:[250,500,1000]}).toBe(true);expect(invocation.status,JSON.stringify(invocation)).toBe('succeeded');return invocation}
export async function publishScenarioStandard(api:APIRequestContext,type:string,labels:{id:string;name:string;keywords?:string[];terms?:string[]}[]=[]) {
 const input={name:`E2E ${type} ${randomUUID()}`,task_type:type,guidelines:'Use only the supplied source text and the explicit label definitions. Preserve question and reference-answer wording. Uncertain cases remain pending human review.',labels:labels.map(l=>({...l,definition:`The source explicitly matches ${l.name}.`,include_criteria:['Explicit literal evidence in the original source'],exclude_criteria:['No evidence or conflicting meaning'],keywords:l.keywords||[],terms:l.terms||[],exclude_keywords:[],examples:[]})),constraints:{max_labels:['classification','tagging'].includes(type)?Math.max(1,labels.length):0,max_entities:['named_entity','relation','tagging'].includes(type)?30:0,max_relations:type==='relation'?30:0,allow_empty:type!=='qa'}}
 const head=await writeScenario(api,'/label-standards',input)
 const version=await writeScenario(api,`/label-standards/${head.id}/publish`,{expected_version:head.version})
 expect(version.status).toBe('published');expect(version.standard_id).toBe(head.id);expect(version.content_digest).toHaveLength(64)
 return version
}
export async function openScenarioLabeling(page:Page,source:ScenarioRef,version:{id:string;task_type:string}){await page.goto(`/labeling?source_ref=${encodeURIComponent(`${source.asset_id}:${source.version}`)}&standard_version_id=${encodeURIComponent(version.id)}&kind=${version.task_type==='tagging'?'tagging':'annotation'}`);await expect(page.getByLabel('来源资产',{exact:true})).toHaveValue(`${source.asset_id}:${source.version}`);await expect(page.getByLabel('已发布标准版本',{exact:true})).toHaveValue(version.id);await expect(page.getByText('任务类型：',{exact:false})).toBeVisible()}
