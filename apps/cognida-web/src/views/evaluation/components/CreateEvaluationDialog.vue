<template>
  <UiModal v-model="visible" title="创建测评任务" size="lg" :mask-closable="false">
    <UiForm :model="formData" label-position="left" label-width="120px">
      <!-- 评测类型 -->
      <UiFormItem label="评测类型">
        <UiSelect v-model="formData.type" :options="EVALUATION_TYPE_OPTIONS" placeholder="请选择评测类型" />
        <div class="form-tip">选择评测类型</div>
      </UiFormItem>

      <!-- 被测 Agent（agent / sql 类型显示且必填） -->
      <UiFormItem v-if="runsAgent" label="被测 Agent">
        <UiSelect v-model="formData.agent_id" :options="agentOptions" placeholder="请选择运行中的 Agent" />
        <div class="form-tip">选择作为评测对象的运行中 Agent（必填）</div>
      </UiFormItem>

      <!-- 数据源（agent / sql 类型；决定 runner 命中的库，需与生产对话一致） -->
      <UiFormItem v-if="runsAgent" label="数据源">
        <UiSelect v-model="formData.datasource_id" :options="datasourceOptions" placeholder="请选择数据源" />
        <div class="form-tip">
          测评 Agent 将查询此数据源，须与 DataAgent 对话选用的库一致；“当前库”走内置业务库。
          <template v-if="formData.type === 'sql'">SQL 评测还会在此库上只读执行金标准/生成 SQL 算执行准确率。</template>
        </div>
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

      <!-- 基础运行指标（agent / sql 类型；自动采集，无需勾选） -->
      <UiFormItem v-if="runsAgent" label="基础运行指标">
        <div class="grader-group runtime-group">
          <div class="grader-group-title">运行时自动采集</div>
          <div v-for="m in runtimeMetrics" :key="m.key" class="runtime-metric">
            <span class="runtime-metric-name">{{ m.label }}</span>
            <span class="runtime-metric-desc">{{ m.desc }}</span>
          </div>
        </div>
        <div class="form-tip">
          Agent 评测运行时由执行器自动采集以上基础指标，无需勾选，结果详情会一并展示。
        </div>
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
  AGENT_RUNTIME_METRICS,
  METRIC_DISPLAY,
  buildGraderGroups,
  type GraderGroup
} from '../evaluation-config'
import { evaluationApi } from '@/api/evaluation'
import type { AgentSummary } from '@/api/agent'
import type { Datasource } from '@/api/datasource'
import type { CreateEvaluationRequest } from '@/types'

const props = defineProps<{
  modelValue: boolean
  datasets: Array<{ id: string; name?: string; eval_type: string }>
  knowledgeBases: Array<Record<string, any>>
  chatModels: Array<Record<string, any>>
  agents: AgentSummary[]
  datasources: Datasource[]
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
  // 模块已转向以 Agent 测评为主，默认突出 Agent；QA/RAG/SQL 仍可切换
  type: 'agent' as 'qa' | 'rag' | 'agent' | 'sql',
  dataset_id: 'default',
  kb_id: '',
  agent_id: '',
  // 空字符串 = “当前库”（内置业务库），非空则透传给 runner 路由外部数据源
  datasource_id: '',
  model_id: ''
})
// agent 与 sql 都由被测 Agent 生成答案/SQL，且都需选 Agent + 数据源、采集运行时基础指标。
// sql 额外在 Go 侧只读执行金标准/生成 SQL 算执行准确率，表单交互与 agent 一致。
const runsAgent = computed(() => formData.type === 'agent' || formData.type === 'sql')
// 基础运行指标说明（label 复用 METRIC_DISPLAY，避免与结果展示漂移）
const runtimeMetrics = AGENT_RUNTIME_METRICS.map(m => ({
  key: m.key,
  label: METRIC_DISPLAY[m.key]?.label ?? m.key,
  desc: m.desc
}))

const selectedGraders = ref<string[]>(['rouge', 'bleu'])
// 默认覆盖 Agent 关注的多个维度：正确 / 完整 / 相关 / 有用
const llmJudgeDimensions = ref<string[]>(['accuracy', 'completeness', 'relevance', 'helpfulness'])
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
  // agent 类型默认勾选核心轨迹/工具评测器 + LLM 裁判多维度，
  // 保证开箱即产出完整 Agent 指标（含语义打分）。
  if (formData.type === 'agent' && selectedGraders.value.length === 0) {
    selectedGraders.value = [
      'answer_accuracy',
      'tool_selection',
      'tool_order',
      'llm_judge'
    ].filter(v => available.has(v))
  }
  // sql 类型默认勾选 SQL 生成质量三件套（精确/结构/执行准确率），开箱即产出完整评分。
  if (formData.type === 'sql' && selectedGraders.value.length === 0) {
    selectedGraders.value = [
      'sql_exact_match',
      'sql_component_match',
      'sql_execution_accuracy'
    ].filter(v => available.has(v))
  }
}

onMounted(() => loadCatalog(formData.type))
watch(() => formData.type, (t) => loadCatalog(t))

// ---------- 下拉选项 ----------
// 仅展示与当前评测类型匹配的数据集：后端要求数据集 eval_type 与任务类型一致，
// 否则创建时会返回类型不匹配错误。default 为通用占位，始终保留。
const datasetOptions = computed(() => [
  { label: 'default (默认数据集)', value: 'default' },
  ...props.datasets
    .filter(d => d.eval_type === formData.type)
    .map(d => ({
      label: d.name ? `${d.name}（${d.id}）` : d.id,
      value: d.id
    }))
])

// 切换评测类型后，若当前所选数据集不再属于该类型，回退到 default，避免提交越界。
watch(() => formData.type, () => {
  const stillValid = datasetOptions.value.some(o => o.value === formData.dataset_id)
  if (!stillValid) formData.dataset_id = 'default'
})

const kbOptions = computed(() =>
  props.knowledgeBases.map(kb => ({ label: kb.name, value: kb.id }))
)

const agentOptions = computed(() =>
  props.agents.map(a => ({
    label: a.description ? `${a.name}（${a.description}）` : a.name,
    value: a.id
  }))
)

// 数据源下拉：与 DataAgentView 一致，“当前库”占位表示内置业务库（空 id）。
const datasourceOptions = computed(() => [
  { label: '当前库（内置业务库）', value: '' },
  ...props.datasources.map(d => ({ label: `${d.name}（${d.database_name}）`, value: d.id }))
])

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
  // agent / sql 类型必须选择被测 Agent；qa/rag 不要求，行为不变
  if (runsAgent.value && !formData.agent_id) {
    toast.warning('请选择被测 Agent')
    return
  }
  const config: Record<string, any> = {}
  if (selectedGraders.value.length > 0) config.graders = [...selectedGraders.value]
  // Agent / SQL 评测透传数据源，runner 据此路由到与生产对话相同的库（空 = 内置业务库）
  if (runsAgent.value && formData.datasource_id) {
    config.datasource_id = formData.datasource_id
  }
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
    agent_id: runsAgent.value ? formData.agent_id : undefined,
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

/* 基础运行指标：只读说明列表 */
.runtime-group {
  margin-bottom: 0;
}

.runtime-metric {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-left: 16px;
  margin-bottom: 6px;
}

.runtime-metric:last-child {
  margin-bottom: 0;
}

.runtime-metric-name {
  min-width: 96px;
  font-size: 13px;
  color: #22d3ee;
}

.runtime-metric-desc {
  font-size: 12px;
  color: var(--color-text-muted);
}
</style>
