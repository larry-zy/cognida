<template>
 <section class="reviewed-versions"><h3>已审核的固定版本</h3><p>版本内的样本、审核修订和来源清单不可修改。评测只使用评测 / 测试划分，来源撤权后停止读取。</p><button :disabled="busy" @click="load(false)">刷新版本</button>
 <p v-if="error" role="alert">{{ error }}</p><p v-if="!versions.length && !busy">暂无审核数据集版本。可在标注工作台选择已审核样本发布。</p>
 <article v-for="version in versions" :key="version.id"><strong>{{ version.name }} · v{{ version.version }}</strong><span>{{ version.profile }} · {{ version.samples?.length || version.qa_pairs?.length || 0 }} 个样本</span><small>版本 {{ version.id }}<br />清单 {{ version.manifest_digest }}</small><button @click="download(version.id)">核验权限并下载清单</button><details><summary>样本划分与审核修订</summary><ul><li v-for="sample in version.samples" :key="sample.id">{{ sample.id }} · {{ sample.split }} · 审核修订 {{ sample.review_version }}</li></ul></details></article>
 <button v-if="cursor" :disabled="busy" @click="load(true)">加载更多版本</button></section>
</template>
<script setup lang="ts">
import { onMounted, ref, watch, onBeforeUnmount } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { datasetVersionsApi, type DatasetVersion } from '@/api/evaluation/versions'
const versions = ref<DatasetVersion[]>([]), cursor = ref(''), busy = ref(false), error = ref('')
const auth = useAuthStore(); let generation=0
watch(() => `${auth.authEpoch}:${auth.user?.id}:${auth.currentTenant?.id}`, () => { generation++; versions.value=[]; cursor.value=""; error.value=""; busy.value=false; void load() })
onBeforeUnmount(() => {generation++})
async function load(append = false) { const epoch=++generation; busy.value = true; error.value = ''; try { const response = await datasetVersionsApi.list(append ? cursor.value : ''); if(epoch!==generation)return; versions.value = append ? [...versions.value, ...(response.data?.items || [])] : response.data?.items || []; cursor.value = response.data?.next_cursor || "" } catch (e) { if(epoch!==generation)return; versions.value = []; error.value = e instanceof Error ? e.message : '来源不可访问或版本加载失败' } finally { if(epoch===generation)busy.value = false } }
async function download(id: string) { const epoch=generation; error.value = ''; try { const response = await datasetVersionsApi.read(id); if(epoch!==generation)return; const url = URL.createObjectURL(new Blob([JSON.stringify(response.data, null, 2)], { type: 'application/json' })); const a = document.createElement('a'); a.href = url; a.download = `dataset-${id}.json`; a.click(); URL.revokeObjectURL(url) } catch (e) { if(epoch!==generation)return; error.value = e instanceof Error ? e.message : '来源不可访问' } }
onMounted(() => load())
</script>
<style scoped>.reviewed-versions{padding:24px;border:1px solid var(--color-border);border-radius:12px;margin-bottom:24px}.reviewed-versions article{display:grid;gap:8px;padding:16px 0;border-bottom:1px solid var(--color-border)}small{overflow-wrap:anywhere;color:var(--color-text-muted)}button{width:fit-content;padding:8px 12px;margin:8px 0}</style>
