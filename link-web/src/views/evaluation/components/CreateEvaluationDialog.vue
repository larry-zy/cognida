<template>
  <UiModal v-model="visible" title="创建测评任务" size="lg" :mask-closable="false">
    <UiForm :model="formData" label-position="left" label-width="120px">
      <!-- 评测类型 -->
      <UiFormItem label="评测类型">
        <UiSelect v-model="formData.type" :options="EVALUATION_TYPE_OPTIONS" placeholder="请选择评测类型" />
        <div class="form-tip">选择评测类型</div>
      </UiFormItem>

      <!-- 数据集 -->
      <UiFormItem label="数据集ID">
        <UiSelect v-model="formData.dataset_id" :options="datasetOptions" placeholder="请选择数据集" />
        <div class="form-tip">可选择现有数据集或使用默认数据集</div>
      </UiFormItem>

      <!-- 知识库 -->
      <UiFormItem label="知识库">
        <UiSelect v-model="formData.kb_id" :options="kbOptions" clearable placeholder="请选择知识库（可选）" />
        <div class="form-tip">留空则自动创建临时知识库</div>
      </UiFormItem>

      <!-- 对话模型 -->
      <UiFormItem label="对话模型">
        <UiSelect v-model="formData.model_id" :options="modelOptions" placeholder="请选择对话模型" />
        <div class="form-tip">选择"默认模型"使用系统默认配置</div>
      </UiFormItem>

      <!-- 评测器（后端注册表驱动，按评测类型过滤） -->
      <UiFormItem label="评测器">
        <div v-if="loadingGraders" class="form-tip">正在加载可用指标…</div>
        <template v-for="group in displayGroups" :key="group.title">
          <div class="grader-group">
            <div class="grader-group-title">{{ group.title }}</div>
            <UiCheckbox
              v-for="opt in group.options"
              :key="opt.value"
              v-model="selectedGraders"
              :value="opt.value"
              class="grader-checkbox"
            >
              {{ opt.label }}
            </UiCheckbox>
          </div>
        </template>
        <div v-if="!loadingGraders && displayGroups.length === 0" class="form-tip">
          该评测类型暂无可用指标
        </div>
        <div class="form-tip">选择启用的评测器，每个评测器计算对应的指标</div>
      </UiFormItem>

      <!-- LLM 裁判维度 -->
      <UiFormItem v-if="selectedGraders.includes('llm_judge')" label="裁判评分维度">
        <UiCheckbox
          v-for="opt in LLM_JUDGE_DIMENSIONS"
          :key="opt.value"
          v-model="llmJudgeDimensions"
          :value="opt.value"
          class="grader-checkbox"
        >
          {{ opt.label }}
        </UiCheckbox>
        <div class="form-tip">选择 LLM 裁判的评分维度（1-5 分）</div>
      </UiFormItem>

      <!-- 高级配置 -->
      <UiFormItem label="高级配置">
        <UiTextarea
          v-model="advancedConfig"
          :rows="3"
          placeholder='JSON 格式，例如: {"max_retries": 3, "timeout": 30}'
        />
        <div class="form-tip">可选的高级配置参数（JSON 格式）</div>
      </UiFormItem>
    </UiForm>

    <template #footer>
      <UiButton variant="ghost" @click="visible = false">取消</UiButton>
      <UiButton variant="primary" :loading="creating" @click="handleSubmit">创建任务</UiButton>
    </template>
  </UiModal>
</template>

<script setup lang="ts">
import { reactive, ref, computed, watch, onMounted } from 'vue'
import { UiModal, UiForm, UiFormItem, UiSelect, UiCheckbox, UiTextarea, UiButton } from '@/components'
import toast from '@/utils/toast'
import {
  GRADER_GROUPS,
  LLM_JUDGE_DIMENSIONS,
  EVALUATION_TYPE_OPTIONS,
  buildGraderGroups,
  type GraderGroup
} from '../evaluation-config'
import { evaluationApi } from '@/api/evaluation'
import type { CreateEvaluationRequest } from '@/types'

const props = defineProps<{
  modelValue: boolean
  datasets: string[]
  knowledgeBases: Array<Record<string, any>>
  chatModels: Array<Record<string, any>>
  creating: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'submit', request: CreateEvaluationRequest): void
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit('update:modelValue', v)
})

// ---------- 表单状态 ----------
const formData = reactive({
  type: 'qa' as 'qa' | 'rag' | 'agent',
  dataset_id: 'default',
  kb_id: '',
  model_id: ''
})
const selectedGraders = ref<string[]>(['rouge', 'bleu'])
const llmJudgeDimensions = ref<string[]>(['accuracy', 'relevance'])
const advancedConfig = ref('')

// ---------- 后端驱动的可用指标目录 ----------
const displayGroups = ref<GraderGroup[]>([])
const loadingGraders = ref(false)

/** 前端回退目录：仅在后端目录接口不可用时按类型过滤展示。 */
function fallbackGroups(type: string): GraderGroup[] {
  return GRADER_GROUPS.filter(g => !g.onlyType || g.onlyType === type)
    .map(g => ({ title: g.title, options: g.options }))
}

// 请求序号：快速切换评测类型时，丢弃过期响应，避免慢请求覆盖新目录/选择。
let catalogSeq = 0

/** 拉取指定评测类型的可用指标目录（注册表唯一事实来源）。 */
async function loadCatalog(type: string): Promise<void> {
  const seq = ++catalogSeq
  loadingGraders.value = true
  let groups: GraderGroup[]
  try {
    const catalog = await evaluationApi.getGraders(type)
    groups = buildGraderGroups(catalog?.data?.graders ?? [])
  } catch {
    // 后端不可用时降级到前端回退目录，不阻断创建流程
    groups = fallbackGroups(type)
  }
  if (seq !== catalogSeq) return // 已被更新的请求取代，丢弃过期结果
  displayGroups.value = groups
  loadingGraders.value = false
  pruneSelected()
}

/** 剔除当前目录中不存在的已选指标，避免提交无效指标。 */
function pruneSelected(): void {
  const available = new Set(
    displayGroups.value.flatMap(g => g.options.map(o => o.value))
  )
  selectedGraders.value = selectedGraders.value.filter(v => available.has(v))
}

onMounted(() => loadCatalog(formData.type))
watch(() => formData.type, (t) => loadCatalog(t))

// ---------- 下拉选项 ----------
const datasetOptions = computed(() => [
  { label: 'default (默认数据集)', value: 'default' },
  ...props.datasets.map(d => ({ label: d, value: d }))
])

const kbOptions = computed(() =>
  props.knowledgeBases.map(kb => ({ label: kb.name, value: kb.id }))
)

const modelOptions = computed(() => [
  { label: '默认模型', value: '' },
  ...props.chatModels.map(m => ({ label: m.name || m.display_name, value: m.id }))
])

// ---------- 提交 ----------
function handleSubmit(): void {
  if (!formData.dataset_id) {
    toast.warning('请选择数据集')
    return
  }
  const config: Record<string, any> = {}
  if (selectedGraders.value.length > 0) config.graders = [...selectedGraders.value]
  if (selectedGraders.value.includes('llm_judge')) {
    config.llm_judge = {
      enabled: true,
      dimensions: llmJudgeDimensions.value.length
        ? [...llmJudgeDimensions.value]
        : ['accuracy', 'relevance']
    }
  }
  if (advancedConfig.value.trim()) {
    try {
      Object.assign(config, JSON.parse(advancedConfig.value))
    } catch {
      toast.error('高级配置 JSON 格式错误')
      return
    }
  }
  const request: CreateEvaluationRequest = {
    dataset_id: formData.dataset_id,
    type: formData.type,
    kb_id: formData.kb_id || undefined,
    model_id: formData.model_id,
    config: Object.keys(config).length ? config : undefined
  }
  emit('submit', request)
}
</script>

<style scoped>
.form-tip {
  font-size: 12px;
  color: var(--color-text-muted);
  margin-top: 4px;
  line-height: 1.4;
}

/* 评测器分组样式 */
.grader-group {
  margin-bottom: 12px;
  padding: 12px;
  background: rgba(0, 0, 0, 0.2);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 8px;
}

.grader-group:last-child {
  margin-bottom: 0;
}

.grader-group-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--color-text-secondary);
  margin-bottom: 8px;
  padding-bottom: 6px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
}

.grader-checkbox {
  /* 用 flex 保持每项独占一行；不能用 block，否则覆盖 UiCheckbox 的
     inline-flex 布局，勾选框与文字会上下堆叠 */
  display: flex;
  margin-bottom: 6px;
  margin-left: 16px;
}

.grader-checkbox:last-child {
  margin-bottom: 0;
}
</style>
