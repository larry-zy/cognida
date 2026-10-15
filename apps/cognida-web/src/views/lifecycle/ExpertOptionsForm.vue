<template>
  <fieldset class="expert-options wb-stack" :data-stage="stageId">
    <legend>{{ title }}</legend>
    <label v-if="['data-quality', 'data-cleaning'].includes(agentId)" class="wb-label">
      {{ agentId === 'data-cleaning' ? '清洗后质量阈值' : '质量阈值' }}
      <input :value="modelValue.min_score" type="number" min="0" max="100" step="0.1" required data-option="min_score" @input="set('min_score', Number(value($event)))" />
    </label>
    <template v-if="agentId === 'data-cleaning'">
      <div class="wb-stack">
        <span>清洗操作</span>
        <label v-for="operation in cleanOperations" :key="operation.value" class="wb-checkbox">
          <input type="checkbox" :value="operation.value" :checked="modelValue.operations?.includes(operation.value)" @change="toggle('operations', operation.value, checked($event))" />{{ operation.label }}
        </label>
      </div>
      <label class="wb-label">处理列（留空处理全部列）
        <input :value="modelValue.columns?.join(', ')" placeholder="列名，以逗号分隔；文本列为 text" data-option="columns" @input="set('columns', splitValues(value($event)))" />
      </label>
      <label class="wb-label">去重保留
        <select :value="modelValue.keep" data-option="keep" @change="set('keep', value($event) as 'first' | 'last')"><option value="first">第一条</option><option value="last">最后一条</option></select>
      </label>
    </template>
    <template v-if="agentId === 'data-enrichment'">
      <label class="wb-label">富集方法
        <select :value="modelValue.method" data-option="method" @change="set('method', value($event) as 'rules' | 'llm')"><option value="rules">规则与关键词</option><option value="llm">模型分析（需要审批）</option></select>
      </label>
      <p v-if="modelValue.method === 'llm'" class="wb-muted">模型调用需管理员批准；模型未配置时任务会明确失败。</p>
      <div class="wb-stack">
        <span>富集内容</span>
        <label v-for="feature in enrichmentFeatures" :key="feature.value" class="wb-checkbox">
          <input type="checkbox" :value="feature.value" :checked="modelValue.features?.includes(feature.value)" @change="toggle('features', feature.value, checked($event))" />{{ feature.label }}
        </label>
      </div>
      <label class="wb-label">关键词数量<input :value="modelValue.max_keywords" type="number" min="1" max="30" required data-option="max_keywords" @input="set('max_keywords', Number(value($event)))" /></label>
      <label class="wb-label">摘要字数上限<input :value="modelValue.max_summary_chars" type="number" min="20" max="2000" required data-option="max_summary_chars" @input="set('max_summary_chars', Number(value($event)))" /></label>
      <div v-for="(label, index) in modelValue.labels" :key="index" class="wb-stack label-rule">
        <label class="wb-label">标签名称<input :value="label.name" required maxlength="80" :aria-label="`标签 ${index + 1} 名称`" @input="changeLabel(index, { name: value($event) })" /></label>
        <label class="wb-label">匹配关键词<input :value="label.keywords.join(', ')" required :aria-label="`标签 ${index + 1} 关键词`" placeholder="逗号分隔" @input="changeLabel(index, { keywords: splitValues(value($event)) })" /></label>
        <button type="button" @click="set('labels', modelValue.labels?.filter((_, i) => i !== index))">删除标签 {{ index + 1 }}</button>
      </div>
      <button type="button" :disabled="(modelValue.labels?.length || 0) >= 30" @click="set('labels', [...(modelValue.labels || []), { name: '', keywords: [] }])">＋ 添加标签规则</button>
    </template>
    <template v-if="agentId === 'data-asset'">
      <p v-if="!inputKind" class="wb-muted">选择输入资产后确认发布类型。</p>
      <template v-else-if="inputKind === 'text'">
        <label class="wb-label">目标知识库
          <select :value="modelValue.knowledge_base_id || ''" required data-option="knowledge_base_id" @change="set('knowledge_base_id', value($event))">
            <option value="">请选择当前工作空间的知识库</option>
            <option v-for="target in targets" :key="target.id" :value="target.id" :disabled="target.status === 0">{{ target.name }}{{ target.status === 0 ? '（已停用）' : '' }}</option>
          </select>
        </label>
        <p class="wb-muted">审批通过后将文本写入该知识库，执行时再次检查权限。</p>
      </template>
      <template v-else-if="inputKind === 'table'">
        <p class="wb-muted">表格发布为不可变资产快照，无需选择知识库。</p>
        <label class="wb-label">关联目录资产 ID（可选）<input :value="modelValue.catalog_asset_id || ''" data-option="catalog_asset_id" @input="set('catalog_asset_id', value($event))" /></label>
      </template>
    </template>
    <template v-if="agentId === 'data-analysis'">
      <label class="wb-label">分析目标
        <textarea :value="modelValue.prompt || ''" maxlength="4000" rows="4" placeholder="例如：分析近一个月收入的变化与主要原因" data-option="prompt" @input="set('prompt', value($event))" />
      </label>
      <p class="wb-muted">填写目标会调用模型解释当前资产，并展示证据和限制；留空仅计算描述统计。</p>
    </template>
    <p v-if="['data-access', 'data-evaluation'].includes(agentId)" class="wb-muted">使用所选资产及其来源执行，无需额外参数。</p>
  </fieldset>
</template>
<script setup lang="ts">
import type { CleanOperation, EnrichmentFeature, ExpertOptions, PublicationTarget } from "@/api/lifecycle";
import { cleanOperations, enrichmentFeatures, splitValues } from "./expertOptions";
const props = defineProps<{ stageId: string; agentId: string; title: string; modelValue: ExpertOptions; inputKind: string; targets: PublicationTarget[] }>();
const emit = defineEmits<{ "update:modelValue": [ExpertOptions] }>();
const value = (event: Event) => (event.target as HTMLInputElement).value;
const checked = (event: Event) => (event.target as HTMLInputElement).checked;
function set<K extends keyof ExpertOptions>(key: K, value: ExpertOptions[K]) { emit("update:modelValue", { ...props.modelValue, [key]: value }); }
function toggle(key: "operations" | "features", value: CleanOperation | EnrichmentFeature, checked: boolean) {
  const values: string[] = props.modelValue[key] || [];
  emit("update:modelValue", { ...props.modelValue, [key]: checked ? [...values, value] : values.filter((v) => v !== value) });
}
function changeLabel(index: number, update: Partial<{ name: string; keywords: string[] }>) { set("labels", props.modelValue.labels?.map((label, i) => i === index ? { ...label, ...update } : label)); }
</script>
<style scoped>
.expert-options { min-width: 0; padding: 14px; border: 1px solid var(--border-default); border-radius: 6px; }
legend { padding: 0 6px; font-size: 13px; font-weight: 600; }
.label-rule { padding-top: 12px; border-top: 1px solid var(--border-default); }
</style>
