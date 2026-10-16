<template>
 <main class="workbench"><header class="wb-head"><div><span class="wb-kicker">DATA PREPARATION</span><h1>数据准备</h1><p>选择已有清洗产物，固定列名和数据类型。标准化不猜类型、不删除记录，可独立使用。</p></div></header><ScenarioPath current="/data-preparation" />
 <p v-if="error" class="wb-error" role="alert">{{ error }}</p><p v-if="notice" class="wb-success" role="status">{{ notice }}</p>
 <div class="wb-grid"><section class="wb-panel wb-stack"><h2>选择输入</h2><ScenarioAssetSelect v-model="source" :disabled="busy" @selected="select" /><p v-if="selected && selected.kind!=='cleaned_asset'" class="wb-footnote">此资产尚未清洗。先检查并清洗，完成后将清洗产物转入本页。</p><RouterLink v-if="selected && selected.kind!=='cleaned_asset'" :to="{path:'/experts',query:{asset:source,agent_id:'data-cleaning'}}">使用此数据进行清洗</RouterLink><p v-if="selected && isPreparedAsset(selected)" class="wb-success">此版本已完成准备，可直接标注或构建 SFT；无需重复采集或准备。</p></section>
 <div class="wb-stack"><form class="wb-panel wb-stack" @submit.prevent="submit"><h2>标准化设置</h2><fieldset class="scenario-fieldset" :disabled="busy || !selected || selected.kind!=='cleaned_asset'"><label class="wb-checkbox"><input v-model="normalize" type="checkbox" />统一换行符为 LF</label><label class="wb-checkbox"><input v-model="mapping" type="checkbox" />显式选择、重命名并校验字段类型</label><p class="wb-footnote">不启用映射时保留全部字段。类型用于校验原值，不会将字符串转换成数值或布尔；JSON 类型要求原单元格为有效 JSON 字符串。文本原文变化后会产生新版本。</p><RecordEditor v-if="mapping" v-model="fields" title="字段映射" :fields="editorFields" /><button class="primary" type="submit">{{ busy?'正在提交…':'开始标准化' }}</button></fieldset></form>
 <section v-if="invocation" class="wb-panel wb-stack" aria-label="标准化执行结果"><div class="wb-section-head"><h2>标准化执行结果</h2><span class="wb-badge">{{ status(invocation.status) }}</span><button :disabled="busy" @click="refresh">刷新状态</button></div><RouterLink :to="{path:'/data-preparation',query:{invocation_id:invocation.id}}">打开此准备记录链接</RouterLink><p class="wb-footnote">执行 ID：{{ invocation.id }}。恢复此记录不会再次执行。</p><p v-if="invocation.error" class="wb-error">{{ invocation.error }}</p><p v-if="invocation.output?.summary">{{ invocation.output.summary }}</p><div v-for="ref in preparedRefs" :key="`${ref.asset_id}:${ref.version}`" class="standard-version"><strong>准备产物 · v{{ ref.version }}</strong><p class="wb-footnote">{{ ref.asset_id }}</p><div class="wb-actions"><RouterLink :to="{path:'/labeling',query:{source_ref:`${ref.asset_id}:${ref.version}`}}">用此数据标注</RouterLink><RouterLink :to="{path:'/sft-datasets',query:{source_ref:`${ref.asset_id}:${ref.version}`}}">构建 SFT 数据集</RouterLink><RouterLink :to="{path:'/analysis',query:{asset:`${ref.asset_id}:${ref.version}`}}">查看产物内容</RouterLink></div></div><p v-if="!preparedRefs.length" class="wb-footnote">尚未获得准备产物，以真实执行结果为准。</p></section>
 <section v-if="selected && isPreparedAsset(selected)" class="wb-panel"><h2>已有准备产物</h2><div class="wb-actions"><RouterLink :to="{path:'/labeling',query:{source_ref:source}}">用此数据标注</RouterLink><RouterLink :to="{path:'/sft-datasets',query:{source_ref:source}}">构建 SFT 数据集</RouterLink></div></section></div></div></main>
</template>
<script setup lang="ts">
import { computed,onMounted,onBeforeUnmount,ref,watch } from 'vue'
import { useRoute } from 'vue-router'
import { lifecycleApi,isPreparedAsset,type AssetRef,type Invocation,type PreparationField } from '@/api/lifecycle'
import RecordEditor,{type EditorField} from '@/components/workbench/RecordEditor.vue'
import ScenarioAssetSelect from './ScenarioAssetSelect.vue'
import ScenarioPath from './ScenarioPath.vue'
import {useScenarioSession} from './useScenarioSession'
import '@/components/workbench/workbench.css'
import './scenario.css'
const route=useRoute(),source=ref(''),selected=ref<AssetRef>(),normalize=ref(true),mapping=ref(false),fields=ref<Record<string,unknown>[]>([]),invocation=ref<Invocation>()
const session=useScenarioSession(()=>{source.value='';selected.value=undefined;invocation.value=undefined;fields.value=[];mapping.value=false;void initialize()}),{busy,error,notice}=session
const editorFields:EditorField[]=[{key:'source',label:'原字段名'},{key:'name',label:'目标字段名'},{key:'type',label:'类型',options:['any','text','number','boolean','json'].map(value=>({value,label:({any:'保留原值',text:'文本',number:'精确数值',boolean:'布尔',json:'JSON'}[value]||value)}))},{key:'required',label:'必填',options:[{value:'true',label:'是'},{value:'false',label:'否'}]}]
const preparedRefs=computed(()=>(invocation.value?.output?.refs||[]).filter(r=>r.kind==='cleaned_asset'))
const status=(v:string)=>({queued:'排队中',running:'标准化中',succeeded:'已完成',failed:'失败',cancelled:'已取消',quarantined:'已隔离'}[v]||v)
function select(a?:AssetRef){session.invalidate();selected.value=a;invocation.value=undefined}
async function submit(){if(!selected.value||selected.value.kind!=='cleaned_asset')return;const asset=selected.value;await session.run(async valid=>{const mapped:PreparationField[]=mapping.value?fields.value.map(f=>({source:String(f.source||'').trim(),name:String(f.name||'').trim(),type:(String(f.type||'any')) as PreparationField['type'],required:f.required===true||f.required==='true'})):[];if(mapping.value&&(!mapped.length||mapped.some(f=>!f.source||!f.name)||new Set(mapped.map(f=>f.name)).size!==mapped.length))throw new Error('请填写唯一的目标字段名和对应来源字段');const r=await lifecycleApi.prepare(asset.asset_id,{version:asset.version,fields:mapped,normalize_newlines:normalize.value});if(!valid())return;if(!r.data)throw new Error('未收到执行回执，请核对状态后再提交');invocation.value=r.data;notice.value='已提交标准化，结果以执行回执为准。'})}
async function read(id:string){await session.run(async valid=>{let r;try{r=await lifecycleApi.get('invocations',id)}catch(cause){if(valid())invocation.value=undefined;throw cause}if(!valid())return;if(!r.data||!('invocation' in r.data))throw new Error('未找到标准化执行');if(r.data.invocation.agent_id!=='data-preparation')throw new Error('该记录不是标准化执行');invocation.value=r.data.invocation})}
async function refresh(){if(invocation.value)await read(invocation.value.id)}
async function initialize(){session.invalidate();invocation.value=undefined;selected.value=undefined;source.value=typeof route.query.asset==='string'?route.query.asset:'';if(typeof route.query.invocation_id==='string')await read(route.query.invocation_id)}
watch(()=>[route.query.asset,route.query.invocation_id],initialize)
let timer:ReturnType<typeof setInterval>|undefined
onMounted(()=>{void initialize();timer=setInterval(()=>{if(!busy.value&&invocation.value&&['queued','running'].includes(invocation.value.status))void refresh()},3000)})
onBeforeUnmount(()=>clearInterval(timer))
</script>
