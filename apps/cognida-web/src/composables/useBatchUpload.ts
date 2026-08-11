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
}

/**
 * 批量上传：并发跑「哈希预检 → 上传」流水线，每个文件独立计入状态机，
 * 一个文件失败不影响其余文件，避免合并成单个大请求导致一坏全坏。
 */
export function useBatchUpload(kbId: Ref<string> | string, options: UseBatchUploadOptions = {}) {
  const concurrency = options.concurrency ?? 3
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
    } catch (error: any) {
      // 并发竞态下后端可能返回 409（预检通过后被他人抢先上传相同文件）
      if (error?.response?.status === 409) {
        item.status = 'duplicate'
        item.message = error.response.data?.message || '已存在于知识库中'
      } else {
        item.status = 'error'
        item.message = error?.message || '上传失败'
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
