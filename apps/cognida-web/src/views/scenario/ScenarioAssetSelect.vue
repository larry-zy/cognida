<template>
  <div class="wb-stack">
    <label class="wb-label">{{ label || '来源资产' }}<select :aria-label="label || '来源资产'" :value="modelValue" :disabled="disabled || loading" @change="select(($event.target as HTMLSelectElement).value)"><option value="">选择已有{{ preparedOnly ? '准备数据' : cleanedOnly ? '清洗产物' : '数据资产' }}</option><option v-for="asset in eligible" :key="key(asset)" :value="key(asset)">{{ asset.name || asset.asset_id }} · v{{ asset.version }}</option></select></label>
    <div class="wb-actions"><button v-if="cursor" type="button" :disabled="loading || disabled" @click="load(true)">加载更多资产</button><button v-if="error" type="button" :disabled="loading" @click="initialize">重新读取资产</button><RouterLink v-if="preparedOnly" :to="{path:'/data-preparation',query:modelValue?{asset:modelValue}:{}}">准备已有数据</RouterLink><RouterLink v-else-if="cleanedOnly" to="/experts?agent_id=data-cleaning">打开清洗专家</RouterLink></div>
    <p v-if="!loading && !eligible.length" class="wb-footnote">当前页没有可选{{ preparedOnly ? '准备数据' : cleanedOnly ? '清洗产物' : '数据' }}。可加载后续页，或从已有数据准备后返回。</p>
    <p v-if="selected?.provenance?.preparation" class="wb-footnote">已准备 {{ selected.provenance.preparation.rows }} 行 · 固定版本 {{ key(selected) }}</p>
    <p v-if="error" role="alert" class="wb-error">{{ error }}</p>
  </div>
</template>
<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { lifecycleApi, isPreparedAsset, type AssetRef } from '@/api/lifecycle'
import { useScenarioSession } from './useScenarioSession'
const props=defineProps<{modelValue:string;preparedOnly?:boolean;cleanedOnly?:boolean;disabled?:boolean;label?:string}>()
const emit=defineEmits<{'update:modelValue':[value:string];selected:[value:AssetRef|undefined]}>()
const assets=ref<AssetRef[]>([]),cursor=ref(''),selected=ref<AssetRef>(), requestedID=ref('')
const session=useScenarioSession(()=>{assets.value=[];cursor.value='';selected.value=undefined;requestedID.value='';emit('update:modelValue','');emit('selected',undefined);void initialize()}),{busy:loading,error}=session
const key=(a:AssetRef)=>`${a.asset_id}:${a.version}`
const allowed=(a:AssetRef)=>a.status==='available'&&(!props.preparedOnly||isPreparedAsset(a))&&(!props.cleanedOnly||a.kind==='cleaned_asset')
const eligible=computed(()=>assets.value.filter(allowed))
async function load(append=false){await session.run(async current=>{const old=append?cursor.value:'';const r=await lifecycleApi.assets(props.preparedOnly||props.cleanedOnly?'cleaned_asset':'',old);if(!current())return;assets.value=Array.from(new Map([...(append?assets.value:[]),...(r.data?.items||[])].map(a=>[key(a),a])).values());const next=r.data?.next_cursor||'';if(old&&next===old)throw new Error('资产分页游标未推进，请重新读取');cursor.value=next})}
async function select(id:string){requestedID.value=id;session.invalidate();selected.value=undefined;emit('selected',undefined);emit('update:modelValue',id);if(!id)return;await session.run(async current=>{const at=id.lastIndexOf(':'),version=Number(id.slice(at+1));if(at<1||!Number.isSafeInteger(version)||version<1)throw new Error('资产引用格式无效');const r=await lifecycleApi.asset(id.slice(0,at),version);if(!current())return;if(!r.data||!allowed(r.data.ref))throw new Error(props.preparedOnly?'请选择已经完成标准化的清洗资产':'请选择当前可用的清洗产物或数据资产');assets.value=Array.from(new Map([...assets.value,r.data.ref].map(a=>[key(a),a])).values());selected.value=r.data.ref;emit('selected',r.data.ref)})}
async function initialize(){session.invalidate();const current=session.capture(),id=props.modelValue;await load();if(current()&&id===props.modelValue&&id)await select(id)}
watch(()=>props.modelValue,id=>{if(id!==requestedID.value)void select(id)})
onMounted(initialize)
</script>
