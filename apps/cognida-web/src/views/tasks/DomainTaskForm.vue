<template>
  <form class="wb-stack domain-form" aria-label="领域专家任务" @submit.prevent="preview">
    <p class="wb-muted">固定方案后预览专家任务，也可使用下方 Supervisor 规划多步协作。预览和保存均不会启动执行。</p>
    <fieldset :disabled="busy || loading">
      <label class="wb-label">领域能力<select v-model="selected" aria-label="领域能力"><option value="">选择能力</option><option v-for="cap in options" :key="cap.id" :value="cap.id">{{ capabilityLabel(cap.id) }}</option></select></label>
      <template v-if="capability?.parameter_kind === 'feature'">
        <label class="wb-label">特征定义<select v-model="featureID" aria-label="专家特征定义"><option value="">选择已有定义</option><option v-for="item in features" :key="item.id" :value="item.id">{{ item.name }} · v{{ item.version }}</option></select></label>
        <label v-if="feature" class="wb-label">固定版本<input v-model.number="version" type="number" min="1" :max="feature.version" aria-label="专家特征版本" /></label>
        <p class="wb-muted">先在<router-link to="/features">特征工程</router-link>配置表达式或窗口并预览。这里执行指定版本的物化，保存实际值与回执。</p>
        <button v-if="featureCursor" type="button" @click="loadFeatures(true)">加载更多特征</button>
      </template>
      <template v-else-if="capability">
        <label class="wb-label">处理资产<select v-model="assetID" aria-label="领域处理资产"><option value="">选择固定版本</option><option v-for="asset in eligibleAssets" :key="key(asset)" :value="key(asset)">{{ asset.name || asset.asset_id }} · {{ assetLabel(asset.kind) }} · v{{ asset.version }}</option></select></label>
        <template v-if="selected === 'governance.qa'"><div class="wb-form"><label class="wb-label">问题列<input v-model="questionColumn" required aria-label="问题列" /></label><label class="wb-label">参考答案列<input v-model="referenceColumn" required aria-label="参考答案列" /></label></div><p class="wb-muted">从所选表格提取已有问题和答案，生成待审核样本，不虚构参考答案。</p></template>
        <template v-else><label class="wb-label">标签与规则<textarea v-model="labels" rows="4" maxlength="12000" placeholder="每行：标签名称 | 触发词1,触发词2" aria-label="标签与规则" /></label><p class="wb-muted">每行一个标签，竖线后为逗号分隔的{{ selected === 'governance.named_entity' ? '实体字典词' : '关键词' }}。规则候选保留证据，等待人工审核。</p></template>
      </template>
    </fieldset>
    <p v-if="loading" class="wb-muted" role="status">正在读取固定定义与来源…</p>
    <p v-if="error" class="wb-error" role="alert">{{ error }}</p>
    <button class="primary" :disabled="busy || !capability || loading">预览领域计划</button>
  </form>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { governanceApi, type Feature, type Label } from '@/api/governance'
import { lifecycleApi, type AssetRef } from '@/api/lifecycle'
import type { Capability, DraftInput, NodeParameters, PlanSpec } from '@/api/taskcontrol'
import { assetLabel, capabilityLabel } from './labels'
const props = defineProps<{ capabilities: Capability[]; assets: AssetRef[]; busy: boolean }>()
const emit = defineEmits<{ preview: [draft: DraftInput, plan: PlanSpec] }>()
const selected = ref(''), assetID = ref(''), labels = ref(''), featureID = ref(''), version = ref(1)
const questionColumn = ref('question'), referenceColumn = ref('reference_answer')
const error = ref(''), loading = ref(false), features = ref<Feature[]>([]), featureCursor = ref('')
let generation = 0
const options = computed(() => props.capabilities.filter(cap => cap.parameter_kind && cap.execution))
const capability = computed(() => options.value.find(cap => cap.id === selected.value))
const feature = computed(() => features.value.find(item => item.id === featureID.value))
const key = (asset: AssetRef) => `${asset.asset_id}:${asset.version}`
const eligibleAssets = computed(() => props.assets.filter(asset => asset.status === 'available' && capability.value?.input_kinds.includes(asset.kind)))
async function loadFeatures(more = false) {
  const current = ++generation; loading.value = true; error.value = ''
  try { const result = (await governanceApi.list<Feature>('/features', more ? featureCursor.value : '')).data; if (current === generation && result) { features.value = more ? [...features.value, ...result.items] : result.items; featureCursor.value = result.next_cursor || '' } }
  catch { if (current === generation) error.value = '读取特征失败，请重新选择能力重试。' }
  finally { if (current === generation) loading.value = false }
}
watch(selected, () => { error.value = ''; assetID.value = ''; featureID.value = ''; generation++; loading.value = false; if (capability.value?.parameter_kind === 'feature') void loadFeatures() })
watch(featureID, () => { version.value = feature.value?.version || 1 })
onBeforeUnmount(() => { generation++ })
async function preview() {
  error.value = ''
  const cap = capability.value
  if (!cap?.execution || props.busy || loading.value) return
  let asset = eligibleAssets.value.find(item => key(item) === assetID.value), parameters: NodeParameters
  const current = ++generation
  if (cap.parameter_kind === 'feature') {
    if (!feature.value || !Number.isSafeInteger(version.value) || version.value < 1 || version.value > feature.value.version) { error.value = '请选择特征及有效版本。'; return }
    parameters = { feature: { feature_id: feature.value.id, version: version.value } }
    loading.value = true
    try {
      const fixed = (await governanceApi.get<Feature>(`/features/${encodeURIComponent(feature.value.id)}/versions/${version.value}`)).data
      if (current !== generation) return
      if (!fixed) throw new Error()
      const split = fixed.source_ref_id.lastIndexOf(':')
      const id = fixed.source_ref_id.slice(0, split), sourceVersion = Number(fixed.source_ref_id.slice(split + 1))
      asset = (await lifecycleApi.asset(id, sourceVersion)).data?.ref
      if (current !== generation) return
      if (!asset || asset.status !== 'available') throw new Error()
    } catch { if (current === generation) error.value = '固定特征版本或来源读取失败，请刷新后重试。'; return }
    finally { if (current === generation) loading.value = false }
  } else {
    if (!asset) { error.value = '请选择可访问的资产版本。'; return }
    if (selected.value === 'governance.qa') {
      const question = questionColumn.value.trim(), reference = referenceColumn.value.trim()
      if (!question || !reference || question === reference) { error.value = '问题列和参考答案列须非空且不同。'; return }
      parameters = { governance: { qa_mapping: { question_column: question, reference_answer_column: reference } } }
    } else {
      const rows = labels.value.split('\n').map(row => row.trim()).filter(Boolean)
      const schema: Label[] = rows.map((row, i) => { const [name, terms] = row.split('|'); const words = terms?.split(/[,，]/).map(item => item.trim()).filter(Boolean) || []; return { id: `label-${i + 1}`, name: name!.trim(), ...(selected.value === 'governance.named_entity' ? { terms: words } : { keywords: words }) } })
      if (schema.length < 1 || schema.length > 30 || schema.some(item => !item.name || Array.from(item.name).length > 80) || new Set(schema.map(item => item.name)).size !== schema.length) { error.value = '请输入 1–30 个不重复标签，每个名称最多 80 字符。'; return }
      if (selected.value === 'governance.named_entity' && schema.some(item => !item.terms?.length)) { error.value = '实体预标注每个标签都需要明确字典词。'; return }
      parameters = { governance: { label_schema: schema } }
    }
  }
  if (!asset || current !== generation) return
  const refs = [{ asset_id: asset.asset_id, version: asset.version }]
  const calls = cap.execution.base_calls + cap.execution.calls_per_input
  const timeout = Math.min(300, cap.execution.max_timeout_seconds)
  const budget = { max_nodes: 1, max_concurrency: 1, max_attempts: 1, max_calls: calls, max_cost_micros: 0, timeout_seconds: timeout }
  const plan: PlanSpec = { schema_version: 1, budget, nodes: [{ id: 'work', capability: cap.id, capability_version: cap.version, depends_on: [], inputs: [{ asset: refs[0]!, kind: asset.kind }], output_kinds: [...cap.output_kinds], tools: [...cap.tools], permissions: [...cap.permissions], max_attempts: 1, max_calls_per_attempt: calls, max_cost_micros_per_attempt: 0, timeout_seconds: timeout, parameters }] }
  emit('preview', { goal: `${capabilityLabel(cap.id)}：${(cap.parameter_kind === 'feature' ? feature.value?.name : asset.name) || '固定资产版本'}`, input_refs: refs, budget }, plan)
}
</script>
<style scoped>
.domain-form fieldset { border: 0; margin: 0; padding: 0; display: grid; gap: 16px; } .domain-form p { font-size: 12px; line-height: 1.8; } .domain-form label { font-size: 13px; }
</style>
