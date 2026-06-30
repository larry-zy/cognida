<template>
  <div class="evaluation-list-container">
    <div class="header">
      <h2>大模型测评</h2>
      <el-button type="primary" :icon="Plus" @click="showCreateDialog = true">
        创建测评任务
      </el-button>
    </div>

    <!-- 统计卡片 -->
    <div class="stats-row">
      <div class="stat-card">
        <div class="stat-value">{{ stats.total }}</div>
        <div class="stat-label">总任务数</div>
      </div>
      <div class="stat-card">
        <div class="stat-value success">{{ stats.success }}</div>
        <div class="stat-label">已完成</div>
      </div>
      <div class="stat-card">
        <div class="stat-value warning">{{ stats.running }}</div>
        <div class="stat-label">执行中</div>
      </div>
      <div class="stat-card">
        <div class="stat-value danger">{{ stats.failed }}</div>
        <div class="stat-label">失败</div>
      </div>
    </div>

    <!-- 任务列表 -->
    <el-table
      :data="evaluations"
      v-loading="loading"
      stripe
      style="width: 100%"
      @row-click="viewDetail"
    >
      <el-table-column prop="task_id" label="任务ID" width="200" show-overflow-tooltip />
      <el-table-column prop="dataset_id" label="数据集" width="150" />
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="EvaluationStatusType[row.status as EvaluationStatus]">
            {{ EvaluationStatusText[row.status as EvaluationStatus] }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="进度" width="200">
        <template #default="{ row }">
          <el-progress
            :percentage="getProgress(row)"
            :status="row.status === 'failed' ? 'exception' : (row.status === 'completed' ? 'success' : '')"
          />
        </template>
      </el-table-column>
      <el-table-column prop="created_at" label="创建时间" width="180">
        <template #default="{ row }">
          {{ formatTime(row.created_at) }}
        </template>
      </el-table-column>
      <el-table-column label="操作" width="150" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click.stop="viewDetail(row)">查看</el-button>
          <el-button
            v-if="row.status === 'running'"
            link
            type="primary"
            @click.stop="refreshDetail(row)"
          >
            刷新
          </el-button>
          <el-button
            link
            type="danger"
            @click.stop="deleteEvaluation(row.id)"
          >
            删除
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-empty v-if="!loading && evaluations.length === 0" description="暂无测评任务">
      <el-button type="primary" @click="showCreateDialog = true">创建第一个测评任务</el-button>
    </el-empty>

    <!-- 创建测评对话框 -->
    <el-dialog
      v-model="showCreateDialog"
      title="创建测评任务"
      width="600px"
      :close-on-click-modal="false"
    >
      <el-form :model="formData" :rules="formRules" ref="formRef" label-width="120px">
        <el-form-item label="评测类型" prop="type">
          <el-select v-model="formData.type" placeholder="请选择评测类型" style="width: 100%">
            <el-option label="QA 评测" value="qa" />
            <el-option label="RAG 评测" value="rag" />
            <el-option label="Agent 评测" value="agent" />
          </el-select>
          <div class="form-tip">选择评测类型</div>
        </el-form-item>

        <el-form-item label="数据集ID" prop="dataset_id">
          <el-select v-model="formData.dataset_id" placeholder="请选择数据集" style="width: 100%">
            <el-option label="default (默认数据集)" value="default" />
            <el-option
              v-for="dataset in datasets"
              :key="dataset"
              :label="dataset"
              :value="dataset"
            />
          </el-select>
          <div class="form-tip">可选择现有数据集或使用默认数据集</div>
        </el-form-item>

        <el-form-item label="知识库">
          <el-select
            v-model="formData.kb_id"
            placeholder="请选择知识库（可选）"
            clearable
            style="width: 100%"
          >
            <el-option
              v-for="kb in knowledgeBases"
              :key="kb.id"
              :label="kb.name"
              :value="kb.id"
            />
          </el-select>
          <div class="form-tip">留空则自动创建临时知识库</div>
        </el-form-item>

        <el-form-item label="对话模型">
          <el-select
            v-model="formData.model_id"
            placeholder="请选择对话模型"
            style="width: 100%"
          >
            <el-option label="默认模型" value="" />
            <el-option
              v-for="model in chatModels"
              :key="model.id"
              :label="model.name || model.display_name"
              :value="model.id"
            />
          </el-select>
          <div class="form-tip">选择"默认模型"使用系统默认配置</div>
        </el-form-item>

        <!-- 评测器配置 -->
        <el-form-item label="评测器">
          <el-checkbox-group v-model="selectedGraders">
            <!-- 生成评测器 -->
            <div class="grader-group">
              <div class="grader-group-title">文本生成质量</div>
              <el-checkbox label="rouge">ROUGE (rouge_1/rouge_2/rouge_l)</el-checkbox>
              <el-checkbox label="bleu">BLEU (bleu_1/bleu_4)</el-checkbox>
            </div>

            <!-- 检索评测器 (仅 RAG 类型) -->
            <div class="grader-group" v-if="formData.type === 'rag'">
              <div class="grader-group-title">检索质量</div>
              <el-checkbox label="precision">精确率</el-checkbox>
              <el-checkbox label="recall">召回率</el-checkbox>
              <el-checkbox label="ndcg">NDCG 排序质量</el-checkbox>
              <el-checkbox label="mrr">MRR 倒数排名</el-checkbox>
              <el-checkbox label="map">MAP 平均精确率</el-checkbox>
            </div>

            <!-- 语义评测器 -->
            <div class="grader-group">
              <div class="grader-group-title">语义质量</div>
              <el-checkbox label="semantic_similarity">语义相似度</el-checkbox>
              <el-checkbox label="semantic_relevance">语义相关性</el-checkbox>
            </div>

            <!-- LLM 评测器 -->
            <div class="grader-group">
              <div class="grader-group-title">AI 评分 (LLM-as-Judge)</div>
              <el-checkbox label="llm_judge">LLM 裁判 (多维度)</el-checkbox>
              <el-checkbox label="llm_factual">事实正确性</el-checkbox>
              <el-checkbox label="llm_safety">内容安全性</el-checkbox>
            </div>
          </el-checkbox-group>
          <div class="form-tip">选择启用的评测器，每个评测器计算对应的指标</div>
        </el-form-item>

        <!-- LLM-as-Judge 配置 -->
        <el-form-item label="裁判评分维度" v-if="selectedGraders.includes('llm_judge')">
          <el-checkbox-group v-model="llmJudgeDimensions">
            <el-checkbox label="accuracy">准确性</el-checkbox>
            <el-checkbox label="completeness">完整性</el-checkbox>
            <el-checkbox label="clarity">清晰度</el-checkbox>
            <el-checkbox label="relevance">相关性</el-checkbox>
            <el-checkbox label="helpfulness">帮助程度</el-checkbox>
          </el-checkbox-group>
          <div class="form-tip">选择 LLM 裁判的评分维度（1-5 分）</div>
        </el-form-item>

        <!-- 高级配置 -->
        <el-form-item label="高级配置">
          <el-input
            v-model="advancedConfig"
            type="textarea"
            :rows="3"
            placeholder='JSON 格式，例如: {"max_retries": 3, "timeout": 30}'
          />
          <div class="form-tip">可选的高级配置参数（JSON 格式）</div>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="showCreateDialog = false">取消</el-button>
        <el-button type="primary" @click="createEvaluation" :loading="creating">
          创建任务
        </el-button>
      </template>
    </el-dialog>

    <!-- 测评结果对话框 -->
    <el-dialog
      v-model="showDetailDialog"
      title="测评结果"
      width="1200px"
      :close-on-click-modal="false"
    >
      <div v-if="currentDetail">
        <!-- 任务信息 -->
        <div class="detail-section">
          <h4>任务信息</h4>
          <el-descriptions :column="2" border>
            <el-descriptions-item label="任务ID">{{ currentDetail.task_id }}</el-descriptions-item>
            <el-descriptions-item label="数据集">{{ currentDetail.dataset_id }}</el-descriptions-item>
            <el-descriptions-item label="评测类型">{{ currentDetail.type?.toUpperCase() }}</el-descriptions-item>
            <el-descriptions-item label="状态">
              <el-tag :type="EvaluationStatusType[currentDetail.status]">
                {{ EvaluationStatusText[currentDetail.status] }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="进度">
              {{ currentDetail.success_count + currentDetail.failure_count }} / {{ currentDetail.total_count }}
            </el-descriptions-item>
            <el-descriptions-item label="成功率">
              {{ currentDetail.total_count > 0 ? ((currentDetail.success_count / currentDetail.total_count) * 100).toFixed(1) : 0 }}%
            </el-descriptions-item>
            <el-descriptions-item label="创建时间">
              {{ formatTime(currentDetail.created_at) }}
            </el-descriptions-item>
            <el-descriptions-item label="结束时间">
              {{ (currentDetail.status === 'completed' || currentDetail.status === 'failed') ? formatTime(currentDetail.updated_at) : '-' }}
            </el-descriptions-item>
          </el-descriptions>
        </div>

        <!-- 聚合指标 -->
        <div class="detail-section" v-if="aggregateMetrics">
          <h4>整体评测指标</h4>
          <div class="metrics-grid">
            <div class="metric-item">
              <div class="metric-label">成功率</div>
              <div class="metric-value">{{ aggregateMetrics.successRate.toFixed(1) }}%</div>
            </div>
            <div v-if="aggregateMetrics.rouge1 !== null" class="metric-item">
              <div class="metric-label">ROUGE-1 (平均)</div>
              <div class="metric-value">{{ (aggregateMetrics.rouge1 * 100).toFixed(2) }}%</div>
            </div>
            <div v-if="aggregateMetrics.rougeL !== null" class="metric-item">
              <div class="metric-label">ROUGE-L (平均)</div>
              <div class="metric-value">{{ (aggregateMetrics.rougeL * 100).toFixed(2) }}%</div>
            </div>
            <div v-if="aggregateMetrics.bleu1 !== null" class="metric-item">
              <div class="metric-label">BLEU-1 (平均)</div>
              <div class="metric-value">{{ (aggregateMetrics.bleu1 * 100).toFixed(2) }}%</div>
            </div>
            <div v-if="aggregateMetrics.precision !== null" class="metric-item">
              <div class="metric-label">Precision (平均)</div>
              <div class="metric-value">{{ (aggregateMetrics.precision * 100).toFixed(2) }}%</div>
            </div>
            <div v-if="aggregateMetrics.recall !== null" class="metric-item">
              <div class="metric-label">Recall (平均)</div>
              <div class="metric-value">{{ (aggregateMetrics.recall * 100).toFixed(2) }}%</div>
            </div>
            <div v-if="aggregateMetrics.llmScore !== null" class="metric-item">
              <div class="metric-label">LLM 评分 (平均)</div>
              <div class="metric-value">{{ aggregateMetrics.llmScore.toFixed(2) }}</div>
            </div>
            <div v-if="aggregateMetrics.semanticSim !== null" class="metric-item">
              <div class="metric-label">语义相似度 (平均)</div>
              <div class="metric-value">{{ (aggregateMetrics.semanticSim * 100).toFixed(2) }}%</div>
            </div>
          </div>
        </div>

        <!-- QA 结果列表 -->
        <div class="detail-section" v-if="currentDetail.qa_results && currentDetail.qa_results.length > 0">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px;">
            <h4>QA 结果详情 ({{ currentDetail.qa_results.length }} 条)</h4>
            <el-button size="small" @click="expandAllResults">{{ allExpanded ? '收起全部' : '展开全部' }}</el-button>
          </div>

          <el-table :data="currentDetail.qa_results" stripe max-height="400"
                    :row-key="(row: any) => row.id"
                    :expand-row-keys="expandedRows"
                    @expand-change="handleExpandChange">
            <el-table-column type="expand" width="50">
              <template #default="{ row }">
                <div class="qa-detail-content">
                  <div class="qa-detail-section">
                    <h5>问题</h5>
                    <p>{{ row.question }}</p>
                  </div>
                  <div class="qa-detail-section">
                    <h5>参考答案</h5>
                    <p>{{ row.reference_answer }}</p>
                  </div>
                  <div class="qa-detail-section">
                    <h5>生成答案</h5>
                    <p>{{ row.generated_answer }}</p>
                  </div>

                  <!-- 指标展示 -->
                  <div class="qa-detail-section" v-if="hasMetrics(row)">
                    <h5>评测指标</h5>
                    <div class="qa-metrics-grid">
                      <div v-if="row.rouge_1 !== undefined && row.rouge_1 !== null" class="qa-metric-item">
                        <span class="metric-label">ROUGE-1:</span>
                        <span class="metric-value">{{ (row.rouge_1 * 100).toFixed(2) }}%</span>
                      </div>
                      <div v-if="row.rouge_l !== undefined && row.rouge_l !== null" class="qa-metric-item">
                        <span class="metric-label">ROUGE-L:</span>
                        <span class="metric-value">{{ (row.rouge_l * 100).toFixed(2) }}%</span>
                      </div>
                      <div v-if="row.bleu_1 !== undefined && row.bleu_1 !== null" class="qa-metric-item">
                        <span class="metric-label">BLEU-1:</span>
                        <span class="metric-value">{{ (row.bleu_1 * 100).toFixed(2) }}%</span>
                      </div>
                      <div v-if="row.precision !== undefined && row.precision !== null" class="qa-metric-item">
                        <span class="metric-label">Precision:</span>
                        <span class="metric-value">{{ (row.precision * 100).toFixed(2) }}%</span>
                      </div>
                      <div v-if="row.recall !== undefined && row.recall !== null" class="qa-metric-item">
                        <span class="metric-label">Recall:</span>
                        <span class="metric-value">{{ (row.recall * 100).toFixed(2) }}%</span>
                      </div>
                      <div v-if="row.llm_score !== undefined && row.llm_score !== null" class="qa-metric-item">
                        <span class="metric-label">LLM Score:</span>
                        <span class="metric-value">{{ row.llm_score.toFixed(2) }}</span>
                      </div>
                      <div v-if="row.semantic_similarity !== undefined && row.semantic_similarity !== null" class="qa-metric-item">
                        <span class="metric-label">相似度:</span>
                        <span class="metric-value">{{ (row.semantic_similarity * 100).toFixed(2) }}%</span>
                      </div>
                    </div>
                  </div>

                  <!-- LLM 推理 -->
                  <div class="qa-detail-section" v-if="row.llm_reasoning">
                    <h5>AI 评分理由</h5>
                    <p class="reasoning-text">{{ row.llm_reasoning }}</p>
                  </div>

                  <!-- 错误信息 -->
                  <div class="qa-detail-section" v-if="row.error">
                    <h5>错误信息</h5>
                    <el-alert type="error" :closable="false">{{ row.error }}</el-alert>
                  </div>
                </div>
              </template>
            </el-table-column>
            <el-table-column prop="question" label="问题" min-width="200" show-overflow-tooltip />
            <el-table-column label="状态" width="80">
              <template #default="{ row }">
                <el-tag :type="row.success ? 'success' : 'danger'" size="small">
                  {{ row.success ? '成功' : '失败' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="ROUGE-1" width="90" align="right">
              <template #default="{ row }">
                <span v-if="row.rouge_1 !== null && row.rouge_1 !== undefined">
                  {{ (row.rouge_1 * 100).toFixed(1) }}%
                </span>
                <span v-else>-</span>
              </template>
            </el-table-column>
            <el-table-column label="BLEU-1" width="90" align="right">
              <template #default="{ row }">
                <span v-if="row.bleu_1 !== null && row.bleu_1 !== undefined">
                  {{ (row.bleu_1 * 100).toFixed(1) }}%
                </span>
                <span v-else>-</span>
              </template>
            </el-table-column>
            <el-table-column label="LLM评分" width="90" align="right">
              <template #default="{ row }">
                <span v-if="row.llm_score !== null && row.llm_score !== undefined">
                  {{ row.llm_score.toFixed(1) }}
                </span>
                <span v-else>-</span>
              </template>
            </el-table-column>
          </el-table>
        </div>

        <!-- 检索指标 -->
        <div v-if="currentDetail.metric?.retrieval_metrics" class="detail-section">
          <h4>检索指标</h4>
          <div class="metrics-grid">
            <div class="metric-item">
              <div class="metric-label">Precision</div>
              <div class="metric-value">{{ formatPercent(currentDetail.metric.retrieval_metrics.precision) }}</div>
            </div>
            <div class="metric-item">
              <div class="metric-label">Recall</div>
              <div class="metric-value">{{ formatPercent(currentDetail.metric.retrieval_metrics.recall) }}</div>
            </div>
            <div class="metric-item">
              <div class="metric-label">NDCG@3</div>
              <div class="metric-value">{{ formatPercent(currentDetail.metric.retrieval_metrics.ndcg3) }}</div>
            </div>
            <div class="metric-item">
              <div class="metric-label">NDCG@10</div>
              <div class="metric-value">{{ formatPercent(currentDetail.metric.retrieval_metrics.ndcg10) }}</div>
            </div>
            <div class="metric-item">
              <div class="metric-label">MRR</div>
              <div class="metric-value">{{ formatPercent(currentDetail.metric.retrieval_metrics.mrr) }}</div>
            </div>
            <div class="metric-item">
              <div class="metric-label">MAP</div>
              <div class="metric-value">{{ formatPercent(currentDetail.metric.retrieval_metrics.map) }}</div>
            </div>
          </div>
        </div>

        <!-- 生成指标 -->
        <div v-if="currentDetail.metric?.generation_metrics" class="detail-section">
          <h4>生成指标</h4>
          <div class="metrics-grid">
            <div class="metric-item">
              <div class="metric-label">BLEU-1</div>
              <div class="metric-value">{{ formatPercent(currentDetail.metric.generation_metrics.bleu1) }}</div>
            </div>
            <div class="metric-item">
              <div class="metric-label">BLEU-2</div>
              <div class="metric-value">{{ formatPercent(currentDetail.metric.generation_metrics.bleu2) }}</div>
            </div>
            <div class="metric-item">
              <div class="metric-label">BLEU-4</div>
              <div class="metric-value">{{ formatPercent(currentDetail.metric.generation_metrics.bleu4) }}</div>
            </div>
            <div class="metric-item">
              <div class="metric-label">ROUGE-1</div>
              <div class="metric-value">{{ formatPercent(currentDetail.metric.generation_metrics.rouge1) }}</div>
            </div>
            <div class="metric-item">
              <div class="metric-label">ROUGE-2</div>
              <div class="metric-value">{{ formatPercent(currentDetail.metric.generation_metrics.rouge2) }}</div>
            </div>
            <div class="metric-item">
              <div class="metric-label">ROUGE-L</div>
              <div class="metric-value">{{ formatPercent(currentDetail.metric.generation_metrics.rougeL) }}</div>
            </div>
          </div>
        </div>

        <!-- 加载中状态 -->
        <div v-if="currentDetail.status === 'running'" class="loading-state">
          <el-icon class="is-loading"><Loading /></el-icon>
          <span>测评执行中，实时更新中...</span>
          <el-tag type="info" size="small" style="margin-left: 8px">
            SSE 已连接
          </el-tag>
        </div>

        <!-- 失败信息 -->
        <div v-if="currentDetail.status === 'failed' && currentDetail.task?.error_message" class="error-state">
          <el-alert type="error" :closable="false">
            {{ currentDetail.task.error_message }}
          </el-alert>
        </div>
      </div>

      <template #footer>
        <el-button @click="showDetailDialog = false; disconnectSSE()">关闭</el-button>
        <el-button
          v-if="currentDetail?.status === 'running'"
          type="primary"
          @click="refreshCurrentDetail"
        >
          刷新
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, onUnmounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from '@/utils/element'
import { Plus, Loading } from '@element-plus/icons-vue'
import { evaluationApi } from '@/api/evaluation'
import { knowledgeApi } from '@/api/knowledge'
import { modelApi } from '@/api/model'
import { connectTaskProgress, type SSEClient } from '@/utils/sse'
import {
  EvaluationStatus,
  EvaluationStatusText,
  EvaluationStatusType,
  type EvaluationTask,
  type EvaluationDetail,
  type CreateEvaluationRequest,
  type SSEProgressEvent
} from '@/types'

const loading = ref(false)
const creating = ref(false)
const showCreateDialog = ref(false)
const showDetailDialog = ref(false)
const evaluations = ref<EvaluationTask[]>([])
const datasets = ref<string[]>([])
const knowledgeBases = ref<any[]>([])
const chatModels = ref<any[]>([])
const currentDetail = ref<EvaluationDetail | null>(null)
const formRef = ref()

// 配置选项
const selectedGraders = ref<string[]>(['rouge', 'bleu'])
const llmJudgeDimensions = ref<string[]>(['accuracy', 'relevance'])
const advancedConfig = ref('')
const expandedRows = ref<number[]>([])
const allExpanded = ref(false)

// SSE 客户端
const sseClient = ref<SSEClient | null>(null)

// 统计数据
const stats = computed(() => ({
  total: evaluations.value.length,
  success: evaluations.value.filter(t => t.status === 'completed').length,
  running: evaluations.value.filter(t => t.status === 'running').length,
  failed: evaluations.value.filter(t => t.status === 'failed').length
}))

// 聚合指标
const aggregateMetrics = computed(() => {
  if (!currentDetail.value?.qa_results || currentDetail.value.qa_results.length === 0) {
    return null
  }

  const results = currentDetail.value.qa_results
  const sum = (arr: number[]) => arr.reduce((a, b) => a + b, 0)
  const avg = (arr: number[]) => arr.length > 0 ? sum(arr) / arr.length : 0
  const safeNum = (v: any) => v === null || v === undefined ? NaN : Number(v)

  const rouge1 = results.map(r => safeNum(r.rouge_1)).filter(n => !isNaN(n))
  const rougeL = results.map(r => safeNum(r.rouge_l)).filter(n => !isNaN(n))
  const bleu1 = results.map(r => safeNum(r.bleu_1)).filter(n => !isNaN(n))
  const precision = results.map(r => safeNum(r.precision)).filter(n => !isNaN(n))
  const recall = results.map(r => safeNum(r.recall)).filter(n => !isNaN(n))
  const llmScore = results.map(r => safeNum(r.llm_score)).filter(n => !isNaN(n))
  const semanticSim = results.map(r => safeNum(r.semantic_similarity)).filter(n => !isNaN(n))

  return {
    rouge1: rouge1.length > 0 ? avg(rouge1) : null,
    rougeL: rougeL.length > 0 ? avg(rougeL) : null,
    bleu1: bleu1.length > 0 ? avg(bleu1) : null,
    precision: precision.length > 0 ? avg(precision) : null,
    recall: recall.length > 0 ? avg(recall) : null,
    llmScore: llmScore.length > 0 ? avg(llmScore) : null,
    semanticSim: semanticSim.length > 0 ? avg(semanticSim) : null,
    successRate: results.length > 0
      ? (results.filter(r => r.success).length / results.length) * 100
      : 0
  }
})

// 表单数据
const formData = reactive<CreateEvaluationRequest>({
  dataset_id: 'default',
  type: 'qa',
  kb_id: '',
  model_id: '',
  config: {
    graders: ['rouge', 'bleu'],
    llm_judge: {
      enabled: false,
      dimensions: ['accuracy', 'relevance']
    }
  }
})

const formRules = {
  type: [
    { required: true, message: '请选择评测类型', trigger: 'change' }
  ],
  dataset_id: [
    { required: true, message: '请选择数据集', trigger: 'change' }
  ]
}

// 检查结果是否有指标
function hasMetrics(row: any): boolean {
  return row.rouge_1 !== null || row.rouge_l !== null ||
         row.bleu_1 !== null || row.precision !== null ||
         row.recall !== null || row.llm_score !== null ||
         row.semantic_similarity !== null
}

// 展开/收起全部结果
function expandAllResults() {
  if (allExpanded.value) {
    expandedRows.value = []
    allExpanded.value = false
  } else {
    if (currentDetail.value?.qa_results) {
      expandedRows.value = currentDetail.value.qa_results.map((r: any) => r.id)
      allExpanded.value = true
    }
  }
}

// 处理展开变化
function handleExpandChange(row: any, expanded: any[]) {
  const index = expanded.indexOf(row.id)
  if (index === -1 && expanded.includes(row.id)) {
    expandedRows.value = [...expanded]
  } else if (index !== -1 && !expanded.includes(row.id)) {
    expandedRows.value = expanded.filter((id: any) => id !== row.id)
  }
}

function getProgress(task: EvaluationTask): number {
  if (task.total_count === 0) return 0
  return Math.round(((task.success_count + task.failure_count) / task.total_count) * 100)
}

function formatTime(timeStr: string): string {
  if (!timeStr) return '-'
  const date = new Date(timeStr)
  return date.toLocaleString('zh-CN')
}

function formatPercent(value: number): string {
  return (value * 100).toFixed(2) + '%'
}

async function loadEvaluations() {
  loading.value = true
  try {
    const res = await evaluationApi.list({ page: 1, page_size: 100 })
    if (res.data) {
      evaluations.value = res.data.tasks || []
    }
  } catch (error) {
    console.error('Failed to load evaluations:', error)
  } finally {
    loading.value = false
  }
}

async function loadDatasets() {
  try {
    const res = await evaluationApi.listDatasets()
    if (res.data) {
      datasets.value = (res.data as any).datasets?.map((d: any) => d.id) || []
    }
  } catch (error) {
    console.error('Failed to load datasets:', error)
  }
}

async function loadKnowledgeBases() {
  try {
    const res = await knowledgeApi.getList()
    if (res.data) {
      knowledgeBases.value = (res.data as any).items || []
    }
  } catch (error) {
    console.error('Failed to load knowledge bases:', error)
  }
}

async function loadModels() {
  try {
    const res = await modelApi.getList('chat')
    if (res.data) {
      chatModels.value = res.data as any || []
    }
  } catch (error) {
    console.error('Failed to load models:', error)
  }
}

async function createEvaluation() {
  if (!formData.dataset_id) {
    ElMessage.warning('请选择数据集')
    return
  }

  creating.value = true
  try {
    // 构建配置对象
    const config: any = {}

    // 添加选中的评测器
    if (selectedGraders.value.length > 0) {
      config.graders = selectedGraders.value
    }

    // 添加 LLM-as-Judge 配置
    if (selectedGraders.value.includes('llm_judge')) {
      config.llm_judge = {
        enabled: true,
        dimensions: llmJudgeDimensions.value.length > 0 ? llmJudgeDimensions.value : ['accuracy', 'relevance']
      }
    }

    // 解析高级配置 JSON
    if (advancedConfig.value.trim()) {
      try {
        const advanced = JSON.parse(advancedConfig.value)
        Object.assign(config, advanced)
      } catch (e) {
        ElMessage.error('高级配置 JSON 格式错误')
        creating.value = false
        return
      }
    }

    const request: CreateEvaluationRequest = {
      dataset_id: formData.dataset_id,
      type: formData.type,
      kb_id: formData.kb_id || undefined,
      agent_id: formData.agent_id,
      model_id: formData.model_id,
      config: Object.keys(config).length > 0 ? config : undefined
    }

    const res = await evaluationApi.create(request)
    if (res.data) {
      ElMessage.success('测评任务创建成功')
      showCreateDialog.value = false
      await loadEvaluations()
      // 自动打开详情并连接 SSE
      if (res.data) {
        await viewDetail((res.data as any).task || res.data)
      }
    }
  } catch (error: any) {
    ElMessage.error((error as Error).message || '创建失败')
  } finally {
    creating.value = false
  }
}

async function viewDetail(task: EvaluationTask) {
  showDetailDialog.value = true

  try {
    const res = await evaluationApi.getResult(task.task_id)
    if (res.data) {
      currentDetail.value = res.data

      // 如果任务正在运行，连接 SSE
      if (res.data.status === 'running') {
        connectSSE(task.task_id)
      }
    }
  } catch (error) {
    console.error('Failed to load detail:', error)
  }
}

async function refreshDetail(task: EvaluationTask) {
  try {
    const res = await evaluationApi.getResult(task.task_id)
    if (res.data && res.data.task) {
      const index = evaluations.value.findIndex(e => e.task_id === task.task_id)
      if (index !== -1) {
        evaluations.value[index] = { ...evaluations.value[index], ...res.data.task }
      }
    }
  } catch (error) {
    console.error('Failed to refresh:', error)
  }
}

async function refreshCurrentDetail() {
  if (currentDetail.value && currentDetail.value.task) {
    const task = currentDetail.value.task
    await refreshDetail(task)
    await viewDetail(task)
    await loadEvaluations()
  }
}

async function deleteEvaluation(id: string) {
  try {
    await ElMessageBox.confirm('删除测评任务后，相关数据也将被删除，此操作不可恢复。确定要删除吗？', '删除确认', {
      confirmButtonText: '确定删除',
      cancelButtonText: '取消',
      type: 'warning'
    })

    await evaluationApi.delete(id)
    ElMessage.success('删除成功')
    await loadEvaluations()
  } catch (error: any) {
    if (error !== 'cancel') {
      ElMessage.error((error as Error).message || '删除失败')
    }
  }
}

// SSE 连接
function connectSSE(taskId: string) {
  // 先断开之前的连接
  disconnectSSE()

  sseClient.value = connectTaskProgress(
    taskId,
    {
      onMessage: (event) => {
        try {
          const data = JSON.parse(event.data) as SSEProgressEvent

          // 更新任务列表中的任务状态
          const index = evaluations.value.findIndex(e => e.task_id === data.task_id)
          if (index !== -1) {
            evaluations.value[index].status = data.data.status as any
            evaluations.value[index].success_count = data.data.finished
            evaluations.value[index].total_count = data.data.total
          }

          // 更新当前详情
          if (currentDetail.value?.task_id === data.task_id) {
            currentDetail.value.status = data.data.status as any
            currentDetail.value.success_count = data.data.finished
            currentDetail.value.total_count = data.data.total

            // 如果完成或失败，断开 SSE
            if (data.data.status === 'completed' ||
                data.data.status === 'failed') {
              disconnectSSE()
              // 重新获取完整结果
              refreshCurrentDetail()
            }
          }
        } catch (e) {
          console.error('Failed to parse SSE message:', e)
        }
      },
      onError: (event) => {
        console.error('SSE error:', event)
      }
    },
    {
      autoReconnect: true,
      reconnectDelay: 3000
    }
  )
}

function disconnectSSE() {
  if (sseClient.value) {
    sseClient.value.disconnect()
    sseClient.value = null
  }
}

onMounted(() => {
  loadEvaluations()
  loadDatasets()
  loadKnowledgeBases()
  loadModels()
})

onUnmounted(() => {
  disconnectSSE()
})
</script>

<style scoped>
.evaluation-list-container {
  padding: 24px;
  background: rgba(0, 0, 0, 0.2);
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: var(--radius-lg);
}

.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.header h2 {
  font-size: 20px;
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.stats-row {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  margin-bottom: 24px;
}

.stat-card {
  padding: 20px;
  background: linear-gradient(135deg, #22d3ee 0%, #14b8a6 100%);
  border-radius: 12px;
  color: #0f0f12;
  text-align: center;
}

.stat-card:nth-child(2) {
  background: linear-gradient(135deg, #34d399 0%, #14b8a6 100%);
}

.stat-card:nth-child(3) {
  background: linear-gradient(135deg, #fbbf24 0%, #f59e0b 100%);
}

.stat-card:nth-child(4) {
  background: linear-gradient(135deg, #f87171 0%, #ef4444 100%);
}

.stat-value {
  font-size: 32px;
  font-weight: 700;
  margin-bottom: 8px;
}

.stat-label {
  font-size: 14px;
  opacity: 0.9;
}

.form-tip {
  font-size: 12px;
  color: var(--color-text-muted);
  margin-top: 4px;
  line-height: 1.4;
}

.detail-section {
  margin-bottom: 24px;
}

.detail-section h4 {
  font-size: 16px;
  font-weight: 600;
  color: var(--color-text-primary);
  margin-bottom: 12px;
}

.metrics-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
}

.metric-item {
  padding: 16px;
  background: rgba(0, 0, 0, 0.25);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 12px;
  text-align: center;
  transition: all var(--transition-base);
}

.metric-item:hover {
  border-color: rgba(34, 211, 238, 0.3);
}

.metric-label {
  font-size: 14px;
  color: var(--color-text-secondary);
  margin-bottom: 8px;
}

.metric-value {
  font-size: 24px;
  font-weight: 600;
  color: #22d3ee;
}

.loading-state {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 40px;
  color: var(--color-text-muted);
}

.loading-state .el-icon {
  margin-right: 8px;
  font-size: 24px;
}

.error-state {
  margin-top: 16px;
}

:deep(.el-table) {
  cursor: pointer;
}

:deep(.el-table__row):hover {
  background-color: rgba(34, 211, 238, 0.05);
}

/* QA 详细内容样式 */
.qa-detail-content {
  padding: 16px 24px;
  background: rgba(0, 0, 0, 0.15);
}

.qa-detail-section {
  margin-bottom: 16px;
}

.qa-detail-section:last-child {
  margin-bottom: 0;
}

.qa-detail-section h5 {
  font-size: 14px;
  font-weight: 600;
  color: #22d3ee;
  margin-bottom: 8px;
}

.qa-detail-section p {
  font-size: 14px;
  color: var(--color-text-secondary);
  line-height: 1.6;
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
}

.qa-metrics-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 12px;
}

.qa-metric-item {
  padding: 10px 14px;
  background: rgba(0, 0, 0, 0.25);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 8px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.qa-metric-item .metric-label {
  font-size: 13px;
  color: var(--color-text-secondary);
  font-weight: 500;
}

.qa-metric-item .metric-value {
  font-size: 16px;
  color: #22d3ee;
  font-weight: 600;
}

.reasoning-text {
  font-size: 13px !important;
  color: var(--color-text-secondary) !important;
  background: rgba(0, 0, 0, 0.2);
  padding: 12px;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.1);
}

:deep(.el-alert) {
  border-radius: 8px;
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

.grader-group :deep(.el-checkbox) {
  display: block;
  margin-bottom: 6px;
  margin-left: 16px;
}

.grader-group :deep(.el-checkbox:last-child) {
  margin-bottom: 0;
}

:deep(.el-checkbox-group) {
  display: block;
}
</style>
