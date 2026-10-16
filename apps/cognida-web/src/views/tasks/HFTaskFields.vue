<template>
  <div class="wb-stack">
    <fieldset :disabled="loading">
      <div class="wb-form">
        <label>HF 仓库<input v-model="repo" required aria-label="任务 HF 仓库" placeholder="组织/数据集" /></label>
        <label>revision<input v-model="revision" required aria-label="任务 HF revision" placeholder="main 或固定提交 SHA" /></label>
        <label>HF 访问凭据<select v-model="credential" aria-label="任务 HF 凭据"><option value="">匿名公开访问</option><option v-for="item in credentials" :key="item.id" :value="item.id">{{ item.name }}</option></select></label>
      </div>
      <button v-if="credentialNext" type="button" @click="loadCredentials(true)">更多 HF 凭据</button>
      <button type="button" @click="inspect">读取任务 HF 来源</button>
      <template v-if="dataset">
        <p class="wb-muted">已解析提交：{{ dataset.revision }} · 许可证 {{ dataset.license || '未声明，请核实' }}</p>
        <p v-for="warning in dataset.warnings" :key="warning" class="wb-muted">{{ warning }}</p>
        <div class="wb-form"><label>配置<select v-model="config" aria-label="任务 HF 配置"><option value="">明确选择配置</option><option v-for="item in dataset.configs" :key="item.name">{{ item.name }}</option></select></label><label>切分<select v-model="split" aria-label="任务 HF 切分"><option value="">明确选择切分</option><option v-for="item in splits" :key="item.name">{{ item.name }}</option></select></label></div>
        <label v-for="file in files" :key="file.path" class="wb-checkbox"><input v-model="selectedFiles" type="checkbox" :value="file.path" :disabled="!file.supported" />{{ file.path }} · {{ file.size ?? '未知大小' }} 字节</label>
        <div class="wb-form"><label>最多行数<input v-model.number="maxRows" type="number" min="1" max="1000" aria-label="任务 HF 最多行数" /></label><label>总下载上限（字节）<input v-model.number="maxBytes" type="number" min="1" max="16777216" aria-label="任务 HF 字节上限" /></label></div>
        <label>字段方式<select v-model="mappingMode" aria-label="任务 HF 字段方式"><option value="columns">显式列映射</option><option value="text">文本与来源 ID</option></select></label>
        <RecordEditor v-if="mappingMode === 'columns'" v-model="mapping" title="任务 HF 字段" :fields="mappingFields" />
        <div v-else class="wb-form"><label>文本字段<input v-model="textField" aria-label="任务 HF 文本字段" /></label><label>ID 字段（可选）<input v-model="idField" aria-label="任务 HF ID 字段" /></label></div>
      </template>
    </fieldset>
    <p v-if="loading" role="status" class="wb-muted">正在核对固定来源…</p><p v-if="error" role="alert" class="wb-error">{{ error }}</p>
    <p class="wb-muted">预览领域计划会先读取真实有界样本，不创建导入。只有明确映射的列会进入任务；JSON 嵌套字段需选择 JSON 编码。单文件最多 8 MiB，最多 4 文件；原答案仅在显式选择后用于监督数据。</p>
    <details v-if="previewed"><summary>最近核对的样本（{{ previewed.preview_rows }} 行）</summary><p>排除字段：{{ previewed.manifest.excluded_columns.join('、') || '无' }}；{{ previewed.manifest.sampled ? '有界样本' : '选定范围' }}</p><pre class="wb-code">{{ JSON.stringify({columns:previewed.columns,rows:previewed.rows},null,2) }}</pre></details>
  </div>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { huggingfaceApi, type HFDataset, type HFPreview, type HFRequest } from '@/api/scenario/huggingface'
import { apiSourceApi, type APICredential } from '@/api/apisource'
import RecordEditor, { type EditorField } from '@/components/workbench/RecordEditor.vue'
const repo=ref(''),revision=ref('main'),credential=ref(''),credentials=ref<APICredential[]>([]),credentialNext=ref(''),dataset=ref<HFDataset>(),config=ref(''),split=ref(''),selectedFiles=ref<string[]>([]),maxRows=ref(100),maxBytes=ref(8388608),mappingMode=ref('columns'),mapping=ref<Record<string,unknown>[]>([]),textField=ref(''),idField=ref(''),previewed=ref<HFPreview>(),loading=ref(false),error=ref('')
const mappingFields:EditorField[]=[{key:'source',label:'原字段'},{key:'target',label:'目标字段'},{key:'encoding',label:'编码',options:[{value:'json',label:'JSON 字符串'}]}]
const splits=computed(()=>dataset.value?.configs.find(item=>item.name===config.value)?.splits||[])
const files=computed(()=>{const allowed=splits.value.find(item=>item.name===split.value)?.files||[];return dataset.value?.files.filter(item=>allowed.includes(item.path))||[]})
let epoch=0, credentialEpoch=0, disposed=false
watch([repo,revision,credential],()=>{epoch++;dataset.value=undefined;previewed.value=undefined;config.value='';split.value='';selectedFiles.value=[];loading.value=false},{flush:'sync'})
watch(config,()=>{split.value='';selectedFiles.value=[]},{flush:'sync'});watch(split,()=>{selectedFiles.value=[]},{flush:'sync'})
watch([config,split,selectedFiles,maxRows,maxBytes,mappingMode,mapping,textField,idField],()=>{previewed.value=undefined},{deep:true,flush:'sync'})
async function loadCredentials(more=false){const current=++credentialEpoch;try{const result=(await apiSourceApi.credentials(more?credentialNext.value:'')).data;if(disposed||current!==credentialEpoch)return;const next=(result?.items||[]).filter(item=>item.enabled&&item.kind==='bearer'&&item.origin.replace(/\/$/,'')==='https://huggingface.co');credentials.value=more?[...credentials.value,...next]:next;credentialNext.value=result?.next_cursor||''}catch{if(!disposed&&current===credentialEpoch)error.value='凭据读取失败，匿名公开来源仍可使用。'}}
async function inspect(){if(loading.value)return;const current=++epoch;loading.value=true;error.value='';dataset.value=undefined;previewed.value=undefined;try{const result=(await huggingfaceApi.dataset(repo.value.trim(),revision.value.trim(),credential.value||undefined)).data;if(disposed||current!==epoch)return;if(!result||result.repo_id!==repo.value.trim()||!/^[a-f0-9]{40}$/.test(result.revision))throw new Error('来源或固定提交不匹配');dataset.value=result;config.value='';split.value='';selectedFiles.value=[]}catch(e){if(!disposed&&current===epoch)error.value=e instanceof Error?e.message:'读取来源失败'}finally{if(!disposed&&current===epoch)loading.value=false}}
async function prepare(){
 if(loading.value||!dataset.value)throw new Error('请先读取并明确选择 HF 固定来源。')
 if(!config.value||!split.value||!selectedFiles.value.length||selectedFiles.value.length>4||selectedFiles.value.some(path=>!files.value.some(item=>item.path===path&&item.supported)))throw new Error('请选择配置、切分和 1–4 个支持的静态文件。')
 if(!Number.isSafeInteger(maxRows.value)||maxRows.value<1||maxRows.value>1000||!Number.isSafeInteger(maxBytes.value)||maxBytes.value<1||maxBytes.value>16777216)throw new Error('HF 行数或字节预算超出上限。')
 const columns=mapping.value.map(item=>({source:String(item.source||'').trim(),target:String(item.target||'').trim(),...(item.encoding==='json'?{encoding:'json' as const}:{})}))
 if(mappingMode.value==='columns'&&(!columns.length||columns.some(item=>!item.source||!item.target)||new Set(columns.map(item=>item.target)).size!==columns.length))throw new Error('请填写明确且目标不重复的列映射。')
 if(mappingMode.value==='text'&&!textField.value.trim())throw new Error('请明确填写文本字段。')
 const request:HFRequest={repo_id:dataset.value.repo_id,revision:dataset.value.revision,config:config.value,split:split.value,files:[...selectedFiles.value],field_mapping:mappingMode.value==='columns'?{columns}:{text_field:textField.value.trim(),...(idField.value.trim()?{id_field:idField.value.trim()}:{})},max_rows:maxRows.value,max_bytes:maxBytes.value,...(credential.value?{credential_id:credential.value}:{})}
 const current=epoch;loading.value=true
 try{const preview=(await huggingfaceApi.preview(request)).data;if(disposed||current!==epoch)throw new Error('来源已切换，请重新预览。');if(!preview||preview.manifest.repo_id!==request.repo_id||preview.manifest.revision!==request.revision||preview.manifest.config!==request.config||preview.manifest.split!==request.split)throw new Error('预览返回的固定来源不匹配。');previewed.value=preview;return {request,columns:preview.columns}}finally{if(!disposed&&current===epoch)loading.value=false}
}
onMounted(()=>void loadCredentials());onBeforeUnmount(()=>{disposed=true;epoch++;credentialEpoch++})
defineExpose({prepare})
</script>
