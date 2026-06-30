import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { evaluationApi } from '@/api/evaluation'
import type {
  EvaluationTask,
  EvaluationDetail,
  DatasetInfo,
  QAResult
} from '@/types'
import { EvaluationStatus } from '@/types'

export const useEvaluationStore = defineStore('evaluation', () => {
  // ========================================
  // 任务列表状态
  // ========================================
  const tasks = ref<EvaluationTask[]>([])
  const tasksLoading = ref(false)
  const tasksError = ref<string | null>(null)

  const runningTasks = computed(() =>
    tasks.value.filter(t => t.status === EvaluationStatus.Running)
  )
  const completedTasks = computed(() =>
    tasks.value.filter(t => t.status === EvaluationStatus.Completed)
  )
  const failedTasks = computed(() =>
    tasks.value.filter(t => t.status === EvaluationStatus.Failed)
  )
  const hasRunningTasks = computed(() => runningTasks.value.length > 0)

  // 统计数据
  const stats = computed(() => ({
    total: tasks.value.length,
    running: runningTasks.value.length,
    completed: completedTasks.value.length,
    failed: failedTasks.value.length
  }))

  // 获取任务列表
  async function fetchTasks(params?: { page?: number; page_size?: number }) {
    tasksLoading.value = true
    tasksError.value = null
    try {
      const res = await evaluationApi.list(params)
      if (res.data) {
        tasks.value = res.data.tasks || []
      }
    } catch (error: any) {
      tasksError.value = error.message || '获取任务列表失败'
      console.error('Fetch tasks error:', error)
    } finally {
      tasksLoading.value = false
    }
  }

  // 删除任务
  async function deleteTask(id: string) {
    try {
      await evaluationApi.delete(id)
      tasks.value = tasks.value.filter(t => t.task_id !== id)
      return true
    } catch (error: any) {
      console.error('Delete task error:', error)
      throw error
    }
  }

  // 更新任务状态（用于 SSE 更新）
  function updateTask(taskId: string, updates: Partial<EvaluationTask>) {
    const index = tasks.value.findIndex(t => t.task_id === taskId)
    if (index !== -1) {
      tasks.value[index] = { ...tasks.value[index], ...updates }
    }
  }

  // ========================================
  // 当前任务状态
  // ========================================
  const currentTask = ref<EvaluationTask | null>(null)
  const currentDetail = ref<EvaluationDetail | null>(null)
  const currentLoading = ref(false)
  const qaResults = ref<QAResult[]>([])
  const qaLoading = ref(false)

  // 获取任务详情
  async function fetchTaskDetail(taskId: string) {
    currentLoading.value = true
    try {
      const res = await evaluationApi.getResult(taskId)
      if (res.data) {
        currentDetail.value = res.data
        currentTask.value = res.data.task ?? null
      }
    } catch (error: any) {
      console.error('Fetch task detail error:', error)
      throw error
    } finally {
      currentLoading.value = false
    }
  }

  // 获取 QA 级别结果
  async function fetchQAResults(taskId: string, params?: { page?: number; page_size?: number }) {
    qaLoading.value = true
    try {
      const res = await evaluationApi.getQAResults(taskId, params)
      if (res.data) {
        qaResults.value = res.data.results || []
      }
    } catch (error: any) {
      console.error('Fetch QA results error:', error)
      throw error
    } finally {
      qaLoading.value = false
    }
  }

  // 清空当前任务
  function clearCurrentTask() {
    currentTask.value = null
    currentDetail.value = null
    qaResults.value = []
  }

  // ========================================
  // 数据集状态
  // ========================================
  const datasets = ref<DatasetInfo[]>([])
  const datasetsLoading = ref(false)
  const currentDataset = ref<DatasetInfo | null>(null)
  const datasetPreview = ref<any[]>([])

  // 获取数据集列表
  async function fetchDatasets() {
    datasetsLoading.value = true
    try {
      const res = await evaluationApi.listDatasets()
      if (res.data) {
        datasets.value = res.data.datasets || []
      }
    } catch (error: any) {
      console.error('Fetch datasets error:', error)
      throw error
    } finally {
      datasetsLoading.value = false
    }
  }

  // 获取数据集详情
  async function fetchDataset(id: string) {
    try {
      const res = await evaluationApi.getDataset(id)
      if (res.data) {
        currentDataset.value = res.data
      }
    } catch (error: any) {
      console.error('Fetch dataset error:', error)
      throw error
    }
  }

  // 预览数据集样本
  async function fetchDatasetPreview(id: string, limit: number = 5) {
    try {
      const res = await evaluationApi.previewDataset(id, limit)
      if (res.data) {
        datasetPreview.value = res.data.qapairs || []
      }
    } catch (error: any) {
      console.error('Fetch dataset preview error:', error)
      throw error
    }
  }

  // ========================================
  // 创建任务
  // ========================================
  const creating = ref(false)

  async function createTask(data: any) {
    creating.value = true
    try {
      const res = await evaluationApi.create(data)
      if (res.data) {
        // 添加到任务列表
        if (res.data.task) {
          tasks.value.unshift(res.data.task)
        }
        return res.data
      }
    } catch (error: any) {
      console.error('Create task error:', error)
      throw error
    } finally {
      creating.value = false
    }
  }

  return {
    // 任务列表
    tasks,
    tasksLoading,
    tasksError,
    runningTasks,
    completedTasks,
    failedTasks,
    hasRunningTasks,
    stats,
    fetchTasks,
    deleteTask,
    updateTask,

    // 当前任务
    currentTask,
    currentDetail,
    currentLoading,
    qaResults,
    qaLoading,
    fetchTaskDetail,
    fetchQAResults,
    clearCurrentTask,

    // 数据集
    datasets,
    datasetsLoading,
    currentDataset,
    datasetPreview,
    fetchDatasets,
    fetchDataset,
    fetchDatasetPreview,

    // 创建
    creating,
    createTask
  }
})
