<template>
 <section class="wb-panel" aria-label="应用知识标签"><h3>应用到来源知识</h3><p class="wb-footnote">仅对真实知识来源生效。服务端会重新核验审核修订、知识权限及当前正文；成功后可按标签筛选检索。</p>
 <button type="button" :disabled="busy || result.review_status !== 'approved'" @click="apply">{{ busy ? '正在核验…' : '应用已审核标签' }}</button>
 <p v-if="error" role="alert" class="wb-error">{{ error }}</p><p v-if="receipt" role="status">已应用 · 回执 {{ receipt.id }} · 标签 {{ receipt.labels.join('、') || '无（清除旧标签）' }}</p></section>
</template>
<script setup lang="ts">
import { ref, watch, onBeforeUnmount } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { governanceApi, type TagResult } from '@/api/governance'
const props = defineProps<{ result: TagResult; sourceRef: string }>()
const busy = ref(false), error = ref(''), receipt = ref<{ id: string; labels: string[] }>()
const auth = useAuthStore()
let generation = 0
const selection = () => `${auth.authEpoch}:${auth.user?.id}:${auth.currentTenant?.id}:${props.sourceRef}:${props.result.id}:${props.result.version}`
watch(selection, () => { generation++; busy.value=false; receipt.value = undefined; error.value = '' })
onBeforeUnmount(() => { generation++ })
async function apply() { if(busy.value || props.result.review_status!=='approved')return; const epoch = generation, selected = selection(), resultID = props.result.id, reviewVersion = props.result.version, sourceRef = props.sourceRef; const current = () => epoch === generation && selected === selection(); busy.value = true; error.value = ''; try {
 const target = (await governanceApi.get<{ result_id:string; review_version:number; knowledge_id:string; source_ref_id:string; content_digest:string }>(`/tagging-results/${encodeURIComponent(resultID)}/apply-target?expected_version=${reviewVersion}`)).data
 if (!current()) return
 if (!target || target.result_id !== resultID || target.review_version !== reviewVersion || target.source_ref_id !== sourceRef || !target.knowledge_id) throw new Error('当前来源不是可应用标签的已核验知识，请重新读取审核结果')
 const response = await governanceApi.action<{ id: string; labels: string[] }>(`/tagging-results/${encodeURIComponent(resultID)}/apply`, { knowledge_id: target.knowledge_id, expected_version: reviewVersion })
 if (current()) receipt.value = response.data
 } catch (e) { if (current()) error.value = e instanceof Error ? e.message : '标签应用失败' } finally { if(current())busy.value = false } }
</script>
