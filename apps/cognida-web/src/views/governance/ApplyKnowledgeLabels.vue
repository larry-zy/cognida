<template>
 <section class="wb-panel" aria-label="应用知识标签"><h3>应用到来源知识</h3><p class="wb-footnote">仅对真实知识来源生效。服务端会重新核验审核修订、知识权限及当前正文；成功后可按标签筛选检索。</p>
 <button type="button" :disabled="busy || result.review_status !== 'approved'" @click="apply">{{ busy ? '正在核验…' : '应用已审核标签' }}</button>
 <p v-if="error" role="alert" class="wb-error">{{ error }}</p><p v-if="receipt" role="status">已应用 · 回执 {{ receipt.id }} · 标签 {{ receipt.labels.join('、') || '无（清除旧标签）' }}</p></section>
</template>
<script setup lang="ts">
import { ref, watch, onBeforeUnmount } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { lifecycleApi } from '@/api/lifecycle'
import { governanceApi, type TagResult } from '@/api/governance'
const props = defineProps<{ result: TagResult; sourceRef: string }>()
const busy = ref(false), error = ref(''), receipt = ref<{ id: string; labels: string[] }>()
const auth = useAuthStore()
let generation = 0
watch(() => `${auth.authEpoch}:${auth.user?.id}:${auth.currentTenant?.id}:${props.sourceRef}:${props.result.id}:${props.result.version}`, () => { generation++; busy.value=false; receipt.value = undefined; error.value = '' })
onBeforeUnmount(() => { generation++ })
async function apply() { if(busy.value || props.result.review_status!=='approved')return; const epoch = generation, resultID = props.result.id, reviewVersion = props.result.version, sourceRef = props.sourceRef; busy.value = true; error.value = ''; try {
 const [id, version] = sourceRef.split(':'); const source = await lifecycleApi.asset(id, Number(version)); const provenance = (source.data?.ref as { provenance?: { source_type: string; source_id: string } })?.provenance
 if (provenance?.source_type !== 'knowledge' || !provenance.source_id) throw new Error('当前来源不是可应用标签的真实知识，请从知识条目创建来源资产')
 if (epoch !== generation) return
 const response = await governanceApi.action<{ id: string; labels: string[] }>(`/tagging-results/${resultID}/apply`, { knowledge_id: provenance.source_id, expected_version: reviewVersion })
 if (epoch === generation) receipt.value = response.data
 } catch (e) { if (epoch === generation) error.value = e instanceof Error ? e.message : '标签应用失败' } finally { if(epoch===generation)busy.value = false } }
</script>
