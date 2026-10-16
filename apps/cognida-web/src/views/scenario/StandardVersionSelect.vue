<template>
  <div class="wb-stack">
    <div class="wb-form">
      <label>标注标准<select aria-label="标注标准" v-model="standardID" :disabled="disabled || loading" @change="selectStandard"><option value="">选择标准库中的标准</option><option v-for="item in standards" :key="item.id" :value="item.id">{{ item.name }} · {{ standardTypeLabels[item.task_type] || item.task_type }}</option></select></label>
      <label>已发布标准版本<select aria-label="已发布标准版本" :value="modelValue" :disabled="disabled || loading || !standardID" @change="selectVersion(($event.target as HTMLSelectElement).value)"><option value="">选择已发布版本</option><option v-for="version in eligible" :key="version.id" :value="version.id">{{ version.name }} · 修订 {{ version.standard_revision }} · {{ version.content_digest.slice(0, 8) }}</option></select></label>
    </div>
    <div class="wb-actions"><button v-if="next" type="button" :disabled="disabled || loading" @click="loadStandards(true)">更多标准</button><button v-if="versionNext" type="button" :disabled="disabled || loading" @click="loadVersions(true)">更多已发布版本</button><RouterLink to="/label-standards">管理标注标准</RouterLink></div>
    <p v-if="selected" class="wb-footnote">{{ selected.guidelines }}<br />已冻结 {{ selected.labels?.length || 0 }} 个标签。标准版本 ID：{{ selected.id }}</p>
    <p v-if="error" class="wb-error" role="alert">{{ error }} <button type="button" :disabled="loading" @click="initialize">重新读取标准</button></p>
  </div>
</template>
<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { standardsApi, standardTypeLabels, type LabelStandard, type StandardVersion, type StandardTaskType } from '@/api/governance/standards'
import { useScenarioSession } from './useScenarioSession'
const props = defineProps<{ modelValue: string; taskTypes?: StandardTaskType[]; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: string]; selected: [value: StandardVersion | undefined] }>()
const standards=ref<LabelStandard[]>([]), versions=ref<StandardVersion[]>([]), standardID=ref(''), next=ref(''), versionNext=ref(''), selected=ref<StandardVersion>()
const session=useScenarioSession(()=>{ clear(); void initialize() }), {busy:loading,error}=session
const eligible=computed(()=>versions.value.filter(v=>v.status==='published' && (!props.taskTypes || props.taskTypes.includes(v.task_type))))
function clear(){standards.value=[];versions.value=[];standardID.value='';next.value='';versionNext.value='';selected.value=undefined;emit('update:modelValue','');emit('selected',undefined)}
async function loadStandards(append=false){await session.run(async current=>{const r=await standardsApi.list(append?next.value:'');if(!current())return;standards.value=append?[...standards.value,...(r.data?.items||[])]:r.data?.items||[];next.value=r.data?.next_cursor||''})}
async function loadVersions(append=false){await session.run(async current=>{const id=standardID.value;if(!id)return;const r=await standardsApi.versions(id,append?versionNext.value:'');if(!current()||id!==standardID.value)return;versions.value=append?[...versions.value,...(r.data?.items||[])]:r.data?.items||[];versionNext.value=r.data?.next_cursor||''})}
async function selectStandard(){session.invalidate();versions.value=[];selected.value=undefined;versionNext.value='';emit('update:modelValue','');emit('selected',undefined);await loadVersions()}
async function selectVersion(id:string){session.invalidate();selected.value=undefined;emit('selected',undefined);if(!id){emit('update:modelValue','');return;}await session.run(async current=>{const r=await standardsApi.version(id);if(!current())return;if(!r.data||r.data.status!=='published'||(props.taskTypes&&!props.taskTypes.includes(r.data.task_type)))throw new Error('请选择适用于当前任务的已发布标准版本');standardID.value=r.data.standard_id;if(!standards.value.some(v=>v.id===r.data!.standard_id)){const head=await standardsApi.get(r.data.standard_id);if(!current())return;if(head.data)standards.value.unshift(head.data)}if(!versions.value.some(v=>v.id===id))versions.value.unshift(r.data);selected.value=r.data;emit('update:modelValue',id);emit('selected',r.data)})}
async function initialize(){session.invalidate();const current=session.capture(),id=props.modelValue;await loadStandards();if(current()&&id&&id===props.modelValue)await selectVersion(id)}
watch(()=>props.modelValue,id=>{if(id&&id!==selected.value?.id)void selectVersion(id);else if(!id&&selected.value){selected.value=undefined;emit('selected',undefined)}})
watch(()=>props.taskTypes?.join(','),()=>{if(selected.value&&props.taskTypes&&!props.taskTypes.includes(selected.value.task_type)){session.invalidate();selected.value=undefined;emit('update:modelValue','');emit('selected',undefined)}})
onMounted(initialize)
</script>
