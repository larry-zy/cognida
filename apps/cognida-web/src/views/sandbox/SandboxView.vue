<template>
 <div class="workbench">
  <header class="wb-head"><div><span class="wb-kicker">CONTROLLED EXECUTION</span><h1>脚本执行</h1><p>为清洗或特征计算固定输入快照，检查运行结果，再进入审批与发布。</p></div><button :disabled="loading" @click="refresh()">刷新任务</button></header>
  <p v-if="error" role="alert" class="wb-error">{{ error }}</p>
  <div v-if="!enabled" class="wb-panel"><h2>执行环境未启用</h2><p>{{ reason }}</p><p class="wb-muted">预置清洗与特征算子可继续在原工作台使用。</p><router-link to="/features">打开特征工程 →</router-link></div>
  <div v-else class="sandbox-grid">
   <form class="wb-panel sandbox-editor" @submit.prevent="submit">
    <div class="wb-section-head"><h2>新建执行</h2><span class="wb-badge">受控开发环境</span></div>
    <p class="wb-muted">代码与来源按版本保存。结果先成为候选资产，发布仍需审批。</p>
    <label>执行名称<input v-model="name" required maxlength="120" :disabled="busy" /></label>
    <label for="sandbox-input">输入快照</label><select id="sandbox-input" v-model="input" :disabled="busy"><option value="">无外部输入</option><option v-for="a in assets" :key="`${a.asset_id}:${a.version}`" :value="`${a.asset_id}:${a.version}`">{{ a.name || a.asset_id }} · v{{ a.version }}</option></select>
    <button v-if="assetCursor" type="button" :disabled="busy" @click="loadAssets(true)">加载更多输入</button>
    <label>Python 脚本<textarea v-model="code" required rows="15" spellcheck="false" :disabled="busy" /></label>
    <p class="wb-footnote">输入为 /inputs/data 中的 JSON 快照；将 JSON 或 UTF-8 文本写入 /output。输入只读，运行期间禁网。</p>
    <label>最长运行秒数<input v-model.number="seconds" type="number" min="1" :max="profile?.MaxResources.timeout_seconds || 60" required :disabled="busy" /></label>
    <div class="budget-strip"><span>1 核</span><span>256 MiB 内存</span><span>32 个进程</span><span>16 个输出文件</span></div>
    <button class="primary" type="submit" :disabled="busy || !profile">{{ busy ? '正在提交…' : '保存快照并执行' }}</button>
   </form>
   <section class="sandbox-history" aria-label="执行任务">
    <div class="wb-panel"><div class="wb-section-head"><h2>执行记录</h2><span class="wb-muted">{{ runs.length }} 条</span></div>
     <p v-if="!runs.length" class="wb-muted">还没有执行记录。首次运行会在此保留状态与候选产物。</p>
     <button v-for="run in runs" :key="run.id" class="run-row" :aria-pressed="selected?.id === run.id" @click="open(run.id)"><span>{{ run.id.slice(0,8) }}</span><strong>{{ stateLabel(run.state) }}</strong><small>{{ new Date(run.deadline).toLocaleString() }}</small></button>
     <button v-if="cursor" @click="refresh(true)">加载更多记录</button>
    </div>
    <div v-if="selected" class="wb-panel" aria-label="脚本执行详情" aria-live="polite">
     <div class="wb-section-head"><h2>执行详情</h2><span class="wb-badge">{{ stateLabel(selected.state) }}</span></div>
     <p class="run-id">{{ selected.id }}</p><p v-if="selected.cancel_requested">已申请取消，正在核对容器终止状态。</p><p v-if="selected.error_code" class="wb-error">{{ selected.error_code }}</p>
     <button v-if="!terminal(selected.state)" :disabled="busy || selected.cancel_requested" @click="cancel">申请取消</button>
     <div v-for="(a,i) in selected.manifest?.artifacts || []" :key="a.path" class="artifact-row"><strong>{{ a.path }}</strong><span>{{ a.size_bytes }} 字节</span><code>{{ a.sha256 }}</code><router-link v-if="selected.output_refs?.[i]" :to="{path:'/analysis',query:{asset:`${selected.output_refs[i].asset_id}:${selected.output_refs[i].version}`}}">查看候选产物 →</router-link></div>
    </div>
   </section>
  </div>
 </div>
</template>
<script setup lang="ts">
import {computed,onBeforeUnmount,onMounted,ref,watch} from 'vue'
import {useRoute,useRouter} from 'vue-router'
import {useAuthStore} from '@/stores/auth'
import {lifecycleApi,type AssetRef} from '@/api/lifecycle'
import {sandboxApi,type Profile,type SandboxRun,type InputMetadata} from '@/api/sandbox'
import '@/components/workbench/workbench.css'
const route=useRoute(),router=useRouter(),auth=useAuthStore()
const enabled=ref(false),reason=ref('正在读取环境状态…'),profile=ref<Profile>(),assets=ref<AssetRef[]>([]),assetCursor=ref(''),runs=ref<SandboxRun[]>([]),cursor=ref(''),selected=ref<SandboxRun>(),error=ref(''),busy=ref(false),loading=ref(false)
const name=ref('自定义数据处理'),input=ref(''),seconds=ref(30),code=ref("import json\n\n# 选择输入后，使用 json.load(open('/inputs/data')) 读取快照。\nresult = {'kind': 'text', 'text': '脚本已完成。'}\nwith open('/output/result.json', 'w') as f:\n    json.dump(result, f, ensure_ascii=False)\n")
let epoch=0,alive=true,timer:ReturnType<typeof setInterval>|undefined
const identity=computed(()=>`${auth.user?.id}:${auth.currentTenant?.id}`)
const terminal=(s:string)=>['succeeded','failed','cancelled','timed_out'].includes(s)
const stateLabel=(s:string)=>({queued:'排队中',running:'运行中',succeeded:'已完成',failed:'执行失败',cancelled:'已取消',timed_out:'已超时',reconcile_required:'待核对'}[s]||s)
const message=(e:unknown)=>e instanceof Error?e.message:'操作失败，请核对当前状态后重试'
async function loadAssets(more=false){const at=epoch;try{const d=(await lifecycleApi.assets('',more?assetCursor.value:'')).data;if(alive&&at===epoch&&d){assets.value=more?[...assets.value,...d.items]:d.items;assetCursor.value=d.next_cursor||''}}catch(e){if(at===epoch)error.value=message(e)}}
async function refresh(more=false){if(loading.value)return;const at=epoch;loading.value=true;try{const d=(await sandboxApi.list(more?cursor.value:'')).data;if(alive&&at===epoch&&d){runs.value=more?[...runs.value,...d.items]:d.items;cursor.value=d.next_cursor||''}}catch(e){if(at===epoch)error.value=message(e)}finally{if(at===epoch)loading.value=false}}
async function open(id:string){const at=epoch;try{const d=(await sandboxApi.get(id)).data;if(alive&&at===epoch&&d){selected.value=d;await router.replace({query:{run_id:id}})}}catch(e){if(at===epoch){selected.value=undefined;error.value=message(e)}}}
async function init(){const at=epoch;try{const d=(await sandboxApi.profiles()).data;if(!alive||at!==epoch||!d)return;enabled.value=d.enabled;reason.value=d.reason||'';profile.value=d.profile;if(d.enabled){await Promise.all([loadAssets(),refresh()]);if(alive&&at===epoch&&typeof route.query.run_id==='string')await open(route.query.run_id)}}catch(e){if(at===epoch)error.value=message(e)}}
function boundInput(name:string,m:InputMetadata){return {name,ref:m.Ref,sha256:m.SHA256,size_bytes:m.SizeBytes}}
async function submit(){if(busy.value||!profile.value)return;const at=epoch,p=profile.value;busy.value=true;error.value='';try{
 const created=(await lifecycleApi.createAsset({kind:'text',text:code.value},name.value+' · 代码')).data;if(!alive||at!==epoch||!created)return
 const meta=(await sandboxApi.metadata(created)).data;if(!alive||at!==epoch||!meta)return
 const inputs=[];if(input.value){const split=input.value.lastIndexOf(':');const m=(await sandboxApi.metadata({asset_id:input.value.slice(0,split),version:Number(input.value.slice(split+1))})).data;if(!alive||at!==epoch||!m)return;inputs.push(boundInput('data',m))}
 const d=(await sandboxApi.submit({schema_version:'1',profile_id:p.ID,profile_version:p.Version,image_sha256:p.Image.split('@')[1],code:boundInput('code',meta),inputs,resources:{...p.MaxResources,timeout_seconds:seconds.value}})).data
 if(!alive||at!==epoch||!d)return;await open(d.id);await refresh()
 }catch(e){if(at===epoch)error.value=message(e)}finally{if(at===epoch)busy.value=false}}
async function cancel(){const run=selected.value;if(!run||busy.value)return;const at=epoch;busy.value=true;try{await sandboxApi.cancel(run);if(alive&&at===epoch)await open(run.id)}catch(e){if(at===epoch)error.value=message(e)}finally{if(at===epoch)busy.value=false}}
watch(identity,()=>{epoch++;selected.value=undefined;runs.value=[];assets.value=[];input.value='';profile.value=undefined;enabled.value=false;busy.value=false;loading.value=false;void init()})
onMounted(()=>{void init();timer=setInterval(()=>{if(selected.value&&!terminal(selected.value.state))void open(selected.value.id)},2000)})
onBeforeUnmount(()=>{alive=false;epoch++;if(timer)clearInterval(timer)})
</script>
<style scoped>
.sandbox-grid{display:grid;grid-template-columns:minmax(0,1.2fr) minmax(280px,.8fr);gap:20px;align-items:start}.sandbox-editor{display:grid;gap:16px}.sandbox-editor label{display:grid;gap:7px;font-weight:600}.sandbox-editor textarea{font-family:ui-monospace,monospace;font-size:13px;line-height:1.65;resize:vertical;min-height:250px}.sandbox-history{display:grid;gap:20px;min-width:0}.budget-strip{display:flex;flex-wrap:wrap;gap:8px}.budget-strip span{padding:5px 9px;border:1px solid var(--wb-border,#ddd);border-radius:5px;font-size:12px}.run-row{display:grid;grid-template-columns:1fr auto;width:100%;gap:5px;padding:14px 8px!important;text-align:left;border-bottom:1px solid #e5e7eb!important;border-radius:0!important}.run-row small{grid-column:1/-1;color:#6b7280}.run-row[aria-pressed=true]{background:#eff6ff}.run-id,.artifact-row code{overflow-wrap:anywhere;font-size:12px}.artifact-row{display:grid;gap:8px;padding:14px 0;border-top:1px solid #e5e7eb}@media(max-width:900px){.sandbox-grid{grid-template-columns:1fr}}
</style>
