<template>
  <section class="wb-panel" aria-label="发布审核数据集版本">
    <h3>发布审核数据集版本</h3>
    <p class="wb-footnote">固定选中样本的审核修订、来源、标签方案和划分。后续编辑不会改变本版本；来源撤权后版本不可读取或继续评测。</p>
    <form @submit.prevent="publish">
      <div class="wb-form"><label>数据集 ID<input v-model="datasetID" required maxlength="100" /></label><label>版本名称<input v-model="name" required maxlength="128" /></label><label>样本用途<select v-model="profile"><option v-for="option in profiles" :key="option" :value="option">{{ option }}</option></select></label><label>数据划分<select v-model="algorithm"><option value="none">不适用训练划分（全量评测）</option><option value="source_hash_v1">按来源分组 · 固定种子</option><option value="source_time_v1">按来源时间划分</option></select></label></div>
      <div v-if="algorithm === 'source_hash_v1'" class="wb-form"><label>随机种子<input v-model="seed" required /></label><label>训练比例 %<input v-model.number="train" type="number" min="1" max="98" /></label><label>验证比例 %<input v-model.number="validation" type="number" min="1" max="98" /></label></div>
      <div v-if="algorithm === 'source_time_v1'" class="wb-form"><label>训练截止<input v-model="trainBefore" type="datetime-local" required /></label><label>验证截止<input v-model="validationBefore" type="datetime-local" required /></label></div>
      <p>仅展示当前已加载且审核通过的样本。已选择 {{ selected.length }} 项。</p>
      <label v-for="item in approved" :key="item.id" class="wb-checkbox"><input v-model="selected" type="checkbox" :value="item.id" />{{ item.content.slice(0, 90) }} · 修订 {{ item.version }}</label>
      <p v-if="!approved.length" class="wb-muted">请先审核样本，再发布版本。</p>
      <button class="primary" :disabled="busy || !selected.length">{{ busy ? '正在核验并发布…' : '发布不可变版本' }}</button>
    </form>
    <p v-if="error" role="alert" class="wb-error">{{ error }}</p>
    <p v-if="published" role="status">已发布 {{ published.name }} · v{{ published.version }} · {{ published.samples.length }} 项。版本 ID：{{ published.id }}<br />清单摘要：{{ published.manifest_digest }}</p>
  </section>
</template>
<script setup lang="ts">
import { computed, ref, watch, onBeforeUnmount } from 'vue'
import { useAuthStore } from '@/stores/auth'
import type { AnnotationItem, AnnotationTask } from '@/api/governance'
import { datasetVersionsApi, type DatasetVersion, type DatasetSplit } from '@/api/evaluation/versions'
const props = defineProps<{ task: AnnotationTask; items: AnnotationItem[] }>()
const approved = computed(() => props.items.filter(i => i.review_status === 'approved'))
const profiles = computed(() => props.task.type === 'qa' ? ['qa', 'rag', 'agent', 'sql'] : [props.task.type])
const datasetID = ref(''), name = ref(''), profile = ref(props.task.type), algorithm = ref<DatasetSplit['algorithm']>('none'), seed = ref('reviewed-v1'), train = ref(70), validation = ref(15), trainBefore = ref(''), validationBefore = ref(''), selected = ref<string[]>([]), busy = ref(false), error = ref(''), published = ref<DatasetVersion>()
const auth=useAuthStore()
let generation = 0
watch(() => `${auth.authEpoch}:${auth.user?.id}:${auth.currentTenant?.id}:${props.task.id}`, () => { generation++; busy.value=false; error.value=""; selected.value = []; published.value = undefined; profile.value = props.task.type; datasetID.value = ''; name.value = '' })
onBeforeUnmount(() => { generation++ })
watch(approved, () => { selected.value = selected.value.filter(id => approved.value.some(i => i.id === id)) })
async function publish() {
  if(busy.value || !selected.value.length)return
  const epoch = generation; busy.value = true; error.value = ''
  try {
    const split: DatasetSplit = { algorithm: algorithm.value, near_duplicate_threshold: 0.85 }
    if (algorithm.value === 'source_hash_v1') Object.assign(split, { seed: seed.value, train_percent: train.value, validation_percent: validation.value })
    if (algorithm.value === 'source_time_v1') Object.assign(split, { train_before: new Date(trainBefore.value).toISOString(), validation_before: new Date(validationBefore.value).toISOString() })
    const response = await datasetVersionsApi.publish({ dataset_id: datasetID.value, name: name.value, profile: profile.value, split, samples: approved.value.filter(i => selected.value.includes(i.id)).map(i => ({ task_id: props.task.id, item_id: i.id, expected_version: i.version })) })
    if (epoch === generation) published.value = response.data
  } catch (e) { if (epoch === generation) error.value = e instanceof Error ? e.message : '发布失败，请刷新审核修订并检查来源权限' } finally { if(epoch===generation)busy.value = false }
}
</script>
