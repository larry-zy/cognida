<template><section v-if="type==='qa'||type==='rag'"><label>隔离评测策略种类<select v-model="kind"><option value="">不使用候选策略</option><option v-for="item in kinds" :key="item" :value="item">{{item}}</option></select></label><label v-if="kind">固定策略版本<select :value="modelValue" @change="emit('update:modelValue',($event.target as HTMLSelectElement).value)"><option value="">选择版本</option><option v-for="v in versions.filter(v=>!v.disabled_at)" :key="v.id" :value="v.id">v{{v.version}} · {{v.id}}</option></select></label><button v-if="cursor" type="button" :disabled="busy" @click="load(true)">更多策略版本</button><p v-if="error" role="alert">{{error}}</p><p v-if="kind">候选只用于该隔离评测，不会激活。与基线使用相同固定数据集、模型及 grader；清洗按实际规则处理问题，检索按真实参数取证。</p></section></template>
<script setup lang="ts">
import{computed,ref,watch,onBeforeUnmount}from'vue'
import{strategyApi,type StrategyVersion}from'@/api/governance/strategies'
import{useAuthStore}from'@/stores/auth'
const props=defineProps<{type:string;modelValue:string;open:boolean}>(),emit=defineEmits<{'update:modelValue':[string]}>(),auth=useAuthStore(),kind=ref(''),versions=ref<StrategyVersion[]>([]),cursor=ref(''),busy=ref(false),error=ref('')
const kinds=computed(()=>props.type==='rag'?['retrieval']:['cleaning','skill','playbook','experience']);let epoch=0
function reset(){epoch++;versions.value=[];cursor.value='';busy.value=false;error.value='';emit('update:modelValue','')}
watch(()=>`${props.type}:${props.open}:${auth.authEpoch}:${auth.user?.id}:${auth.currentTenant?.id}`,()=>{kind.value='';reset()})
watch(kind,()=>{reset();if(kind.value&&props.open)void load()})
onBeforeUnmount(()=>{epoch++})
async function load(append=false){if(busy.value||!props.open||!kind.value)return;const own=epoch;busy.value=true;try{const r=await strategyApi.versions(kind.value+'.default',append?cursor.value:'');if(own!==epoch)return;versions.value=append?[...versions.value,...r.data?.items||[]]:r.data?.items||[];cursor.value=r.data?.next_cursor||''}catch(e){if(own===epoch)error.value=e instanceof Error?e.message:'策略版本加载失败'}finally{if(own===epoch)busy.value=false}}
</script>
