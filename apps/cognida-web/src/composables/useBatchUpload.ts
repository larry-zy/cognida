import { ref, computed, type Ref } from 'vue'
import { knowledgeApi } from '@/api/knowledge'
import { computeFileHash } from '@/utils/hash'

export type BatchItemStatus = 'pending' | 'checking' | 'duplicate' | 'uploading' | 'success' | 'error'

export interface BatchUploadItem {
  file: File
  status: BatchItemStatus
  /** 0-100，仅 uploading 阶段有意义 */
  progress: number
  /** 重复提示 / 失败原因 */
  message: string
}

export interface UseBatchUploadOptions {
  /** 同时处理（哈希预检 + 上传）的文件数，默认 3 */
  concurrency?: number
  /** 命中 429 限流后的最大重试次数，默认 5 */
  maxRateLimitRetries?: number
}

const DEFAULT_RETRY_AFTER_SECONDS = 1
const MAX_RETRY_AFTER_SECONDS = 5

function sleep(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

/** 解析后端 429 响应的 Retry-After（秒），缺失/非法时回退默认值，并封顶避免退避过久卡住整批 */
function parseRetryAfterMs(error: any): number {
  const header = error?.response?.headers?.['retry-after']
  const seconds = Number(header)
  const safeSeconds = Number.isFinite(seconds) && seconds > 0 ? seconds : DEFAULT_RETRY_AFTER_SECONDS
  return Math.min(safeSeconds, MAX_RETRY_AFTER_SECONDS) * 1000
}

/**
 * 批量上传：并发跑「哈希预检 → 上传」流水线，每个文件独立计入状态机，
 * 一个文件失败不影响其余文件，避免合并成单个大请求导致一坏全坏。
 * 文件多、并发高时容易连续撞上后端全局限流（429），故按 Retry-After 退避重试，
 * 而不是直接判定失败——否则一旦撞上限流窗口，整批会在同一个窗口内接连失败。
 */
export function useBatchUpload(kbId: Ref<string> | string, options: UseBatchUploadOptions = {}) {
  const concurrency = options.concurrency ?? 3
  const maxRateLimitRetries = options.maxRateLimitRetries ?? 5
  const items = ref<BatchUploadItem[]>([])
  const running = ref(false)

  const summary = computed(() => {
    const list = items.value
    return {
      total: list.length,
      success: list.filter((i) => i.status === 'success').length,
      duplicate: list.filter((i) => i.status === 'duplicate').length,
      error: list.filter((i) => i.status === 'error').length,
      settled: list.filter((i) =>
        i.status === 'success' || i.status === 'duplicate' || i.status === 'error'
      ).length
    }
  })

  function setFiles(files: File[]) {
    items.value = files.map((file) => ({ file, status: 'pending', progress: 0, message: '' }))
  }

  function resolveKbId(): string {
    return typeof kbId === 'string' ? kbId : kbId.value
  }

  async function uploadOne(item: BatchUploadItem) {
    const id = resolveKbId()
    let retries = 0

    while (true) {
      try {
        item.status = 'checking'
        const hash = await computeFileHash(item.file)
        const check = await knowledgeApi.checkFile(id, hash)
        if (check.data?.duplicate) {
          item.status = 'duplicate'
          item.message = `已存在于知识库中（${check.data.title || item.file.name}）`
          return
        }

        item.status = 'uploading'
        const formData = new FormData()
        formData.append('file', item.file)
        await knowledgeApi.uploadFile(id, formData, (percent) => {
          item.progress = percent
        })
        item.status = 'success'
        return
      } catch (error: any) {
        const status = error?.response?.status
        // 并发竞态下后端可能返回 409（预检通过后被他人抢先上传相同文件）
        if (status === 409) {
          item.status = 'duplicate'
          item.message = error.response.data?.message || '已存在于知识库中'
          return
        }
        // 429：按 Retry-After 退避重试，而非直接判失败——批量场景下很容易连续撞上同一限流窗口
        if (status === 429 && retries < maxRateLimitRetries) {
          retries += 1
          const waitMs = parseRetryAfterMs(error)
          item.message = `请求过于频繁，${Math.round(waitMs / 1000)} 秒后自动重试（第 ${retries} 次）`
          await sleep(waitMs)
          continue
        }
        item.status = 'error'
        item.message = status === 429
          ? '请求过于频繁，重试多次仍失败，请稍后手动重试'
          : (error?.message || '上传失败')
        return
      }
    }
  }

  async function start() {
    if (running.value) return
    running.value = true
    const queue = items.value.filter((i) => i.status === 'pending')
    let cursor = 0

    async function worker() {
      while (cursor < queue.length) {
        const item = queue[cursor]
        cursor += 1
        await uploadOne(item)
      }
    }

    const workerCount = Math.max(1, Math.min(concurrency, queue.length))
    await Promise.all(Array.from({ length: workerCount }, worker))
    running.value = false
  }

  return { items, running, summary, setFiles, start }
}
