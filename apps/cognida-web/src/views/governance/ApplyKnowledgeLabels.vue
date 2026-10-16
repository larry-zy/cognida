<template>
 <section class="wb-panel" aria-label="应用知识标签"><h3>应用到来源知识</h3><p class="wb-footnote">服务端重新核验审核修订、知识权限及当前正文。已有知识来源可直接应用；先打标后发布的资料，须明确关联真实发布产物，且其来源链和正文必须与审核结果一致。</p>
 <label class="wb-checkbox"><input v-model="usePublication" type="checkbox" :disabled="busy" />关联后续发布的知识产物</label><ScenarioAssetSelect v-if="usePublication" v-model="publicationRef" published-only label="已发布知识产物" :disabled="busy" @selected="publication=$event" />
 <button type="button" :disabled="busy || result.review_status !== 'approved' || (usePublication && !publication)" @click="apply">{{ busy ? '正在核验…' : '应用已审核标签' }}</button>
 <p v-if="error" role="alert" class="wb-error">{{ error }}</p><p v-if="receipt" role="status">已应用 · 回执 {{ receipt.id }} · 标签 {{ receipt.labels.join('、') || '无（清除旧标签）' }}</p></section>
</template>
<script setup lang="ts">
import { ref, watch, onBeforeUnmount } from 'vue'
import ScenarioAssetSelect from '@/views/scenario/ScenarioAssetSelect.vue'
import type { AssetRef } from '@/api/lifecycle'
import { useAuthStore } from '@/stores/auth'
import { governanceApi, type TagResult } from '@/api/governance'
const props = defineProps<{ result: TagResult; sourceRef: string }>()
const busy = ref(false), error = ref(''), receipt = ref<{ id: string; labels: string[] }>()
const auth = useAuthStore()
const usePublication=ref(false),publicationRef=ref(''),publication=ref<AssetRef>()
let generation = 0
const selection = () => `${auth.authEpoch}:${auth.user?.id}:${auth.currentTenant?.id}:${props.sourceRef}:${props.result.id}:${props.result.version}:${usePublication.value}:${publicationRef.value}`
watch(()=>`${auth.authEpoch}:${auth.user?.id}:${auth.currentTenant?.id}:${props.sourceRef}:${props.result.id}`,()=>{usePublication.value=false;publicationRef.value='';publication.value=undefined},{flush:'sync'})
watch(selection, () => { generation++; busy.value=false; receipt.value = undefined; error.value = '' },{flush:'sync'})
onBeforeUnmount(() => { generation++ })
async function apply() { if(busy.value || props.result.review_status!=='approved'||(usePublication.value&&(!publication.value||`${publication.value.asset_id}:${publication.value.version}`!==publicationRef.value)))return; const epoch = generation, selected = selection(), resultID = props.result.id, reviewVersion = props.result.version, sourceRef = props.sourceRef, publishedRef=usePublication.value?publicationRef.value:''; const current = () => epoch === generation && selected === selection(); busy.value = true; error.value = ''; try {
 const target = (await governanceApi.get<{ result_id:string; review_version:number; knowledge_id:string; source_ref_id:string; content_digest:string;publication_ref_id?:string }>(`/tagging-results/${encodeURIComponent(resultID)}/apply-target?expected_version=${reviewVersion}${publishedRef?`&publication_ref_id=${encodeURIComponent(publishedRef)}`:''}`)).data
 if (!current()) return
 if (!target || target.result_id !== resultID || target.review_version !== reviewVersion || target.source_ref_id !== sourceRef || !target.knowledge_id || (target.publication_ref_id||'')!==publishedRef) throw new Error('当前来源不是可应用标签的已核验知识，请重新读取审核结果')
 const response = await governanceApi.action<{ id: string; labels: string[] }>(`/tagging-results/${encodeURIComponent(resultID)}/apply`, { knowledge_id: target.knowledge_id, expected_version: reviewVersion, ...(publishedRef?{publication_ref_id:publishedRef}:{}) })
 if (current()) receipt.value = response.data
 } catch (e) { if (current()) error.value = e instanceof Error ? e.message : '标签应用失败' } finally { if(current())busy.value = false } }
</script>
