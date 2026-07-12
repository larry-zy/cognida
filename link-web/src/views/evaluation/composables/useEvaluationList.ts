// ==================== 测评列表页核心逻辑 ====================
// 集中管理测评任务的数据加载、创建、删除、详情查看与 SSE 实时进度。
// 视图组件只负责渲染，所有状态与副作用由此 composable 提供。

import { ref, computed } from 'vue'
import toast from '@/utils/toast'
import { ElMessageBox } from '@/utils/confirm'
import { evaluationApi } from '@/api/evaluation'
import { knowledgeApi } from '@/api/knowledge'
import { modelApi } from '@/api/model'
import { listAgents, type AgentSummary } from '@/api/agent'
import { connectTaskProgress, type SSEConnection } from '@/utils/sse'
import type {
  EvaluationTask,
  EvaluationDetail,
  CreateEvaluationRequest,
  SSEProgressEvent
} from '@/types'

export function useEvaluationList() {
  const loading = ref(false)
  const creating = ref(false)
  const evaluations = ref<EvaluationTask[]>([])
  const datasets = ref<string[]>([])
  const knowledgeBases = ref<Array<Record<string, any>>>([])
  const chatModels = ref<Array<Record<string, any>>>([])
  const agents = ref<AgentSummary[]>([])
  const currentDetail = ref<EvaluationDetail | null>(null)

  const sseClient = ref<SSEConnection | null>(null)

  // ---------- 派生数据 ----------
  const stats = computed(() => ({
    total: evaluations.value.length,
    success: evaluations.value.filter(t => t.status === 'completed').length,
    running: evaluations.value.filter(t => t.status === 'running').length,
    failed: evaluations.value.filter(t => t.status === 'failed').length
  }))

  // 任务级聚合指标改由 EvaluationDetailDialog 依据 detail 动态计算
  // （注册表驱动，见 evaluation-config.ts taskAggregate*），此处不再预聚合。

  // ---------- 数据加载 ----------
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
        // 后端返回分页对象 { models, total, page, page_size }，取 models 数组
        chatModels.value = (res.data as any).models || []
      }
    } catch (error) {
      console.error('Failed to load models:', error)
    }
  }

  async function loadAgents() {
    try {
      const res = await listAgents()
      if (res.data) {
        agents.value = (res.data as any).agents || []
      }
    } catch (error) {
      console.error('Failed to load agents:', error)
    }
  }

  function loadAll() {
    loadEvaluations()
    loadDatasets()
    loadKnowledgeBases()
    loadModels()
    loadAgents()
  }

  // ---------- 创建 ----------
  /**
   * 提交创建请求。请求体（含 config）已由创建对话框构建完成。
   * @returns 是否创建成功（成功时对话框应关闭）
   */
  async function submitEvaluation(request: CreateEvaluationRequest): Promise<boolean> {
    creating.value = true
    try {
      const res = await evaluationApi.create(request)
      if (res.data) {
        toast.success('测评任务创建成功')
        await loadEvaluations()
        // 自动打开详情并连接 SSE
        await viewDetail((res.data as any).task || res.data)
        return true
      }
      return false
    } catch (error) {
      toast.error((error as Error).message || '创建失败')
      return false
    } finally {
      creating.value = false
    }
  }

  // ---------- 详情 ----------
  async function viewDetail(task: EvaluationTask) {
    try {
      const res = await evaluationApi.getResult(task.task_id)
      if (res.data) {
        currentDetail.value = res.data
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
    const task = currentDetail.value?.task
    if (task) {
      await refreshDetail(task)
      await viewDetail(task)
      await loadEvaluations()
    }
  }

  async function deleteEvaluation(id: string) {
    try {
      await ElMessageBox.confirm({
        title: '删除确认',
        message: '删除测评任务后，相关数据也将被删除，此操作不可恢复。确定要删除吗？',
        type: 'warning',
        confirmButtonText: '确定删除',
        cancelButtonText: '取消'
      })
      await evaluationApi.delete(id)
      toast.success('删除成功')
      await loadEvaluations()
    } catch (error: any) {
      if (error !== 'cancel') {
        toast.error((error as Error).message || '删除失败')
      }
    }
  }

  // ---------- SSE 实时进度 ----------
  function connectSSE(taskId: string) {
    disconnectSSE()
    sseClient.value = connectTaskProgress(
      taskId,
      {
        onMessage: (event) => {
          try {
            const data = JSON.parse(event.data) as SSEProgressEvent
            const index = evaluations.value.findIndex(e => e.task_id === data.task_id)
            if (index !== -1) {
              evaluations.value[index].status = data.data.status as any
              evaluations.value[index].success_count = data.data.finished
              evaluations.value[index].total_count = data.data.total
            }
            if (currentDetail.value?.task_id === data.task_id) {
              currentDetail.value.status = data.data.status as any
              currentDetail.value.success_count = data.data.finished
              currentDetail.value.total_count = data.data.total
              if (data.data.status === 'completed' || data.data.status === 'failed') {
                disconnectSSE()
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

  return {
    // state
    loading,
    creating,
    evaluations,
    datasets,
    knowledgeBases,
    chatModels,
    agents,
    currentDetail,
    // computed
    stats,
    // actions
    loadAll,
    loadAgents,
    loadEvaluations,
    submitEvaluation,
    viewDetail,
    refreshDetail,
    refreshCurrentDetail,
    deleteEvaluation,
    disconnectSSE
  }
}
