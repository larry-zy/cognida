import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useBatchUpload } from '@/composables/useBatchUpload'
import { knowledgeApi } from '@/api/knowledge'

vi.mock('@/api/knowledge', () => ({
  knowledgeApi: {
    checkFile: vi.fn(),
    uploadFile: vi.fn()
  }
}))

vi.mock('@/utils/hash', () => ({
  computeFileHash: vi.fn().mockResolvedValue('deadbeef')
}))

function makeFile(name: string): File {
  return new File([new Uint8Array(4)], name)
}

/** 返回一个可从外部手动 resolve 的 Promise，用于控制并发时序 */
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: any) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

beforeEach(() => {
  vi.mocked(knowledgeApi.checkFile).mockReset()
  vi.mocked(knowledgeApi.uploadFile).mockReset()
})

describe('useBatchUpload/成功与去重', () => {
  it('非重复文件：预检通过后上传成功，状态与进度正确更新', async () => {
    vi.mocked(knowledgeApi.checkFile).mockResolvedValue({ data: { duplicate: false } } as any)
    vi.mocked(knowledgeApi.uploadFile).mockImplementation(async (_id, _form, onProgress) => {
      onProgress?.(100)
      return { data: { knowledge_id: 'k1', status: 'processing' } } as any
    })

    const batch = useBatchUpload('kb-1')
    batch.setFiles([makeFile('a.md')])
    await batch.start()

    expect(batch.items.value[0].status).toBe('success')
    expect(batch.items.value[0].progress).toBe(100)
    expect(batch.summary.value).toMatchObject({ total: 1, success: 1, duplicate: 0, error: 0, settled: 1 })
  })

  it('命中重复文件时不发起真正上传，状态标记为 duplicate', async () => {
    vi.mocked(knowledgeApi.checkFile).mockResolvedValue({
      data: { duplicate: true, title: '已有文档.md' }
    } as any)

    const batch = useBatchUpload('kb-1')
    batch.setFiles([makeFile('dup.md')])
    await batch.start()

    expect(batch.items.value[0].status).toBe('duplicate')
    expect(batch.items.value[0].message).toContain('已有文档.md')
    expect(knowledgeApi.uploadFile).not.toHaveBeenCalled()
  })

  it('上传阶段并发竞态返回 409 时按重复处理', async () => {
    vi.mocked(knowledgeApi.checkFile).mockResolvedValue({ data: { duplicate: false } } as any)
    vi.mocked(knowledgeApi.uploadFile).mockRejectedValue({
      response: { status: 409, data: { message: '该文件已存在于知识库中' } }
    })

    const batch = useBatchUpload('kb-1')
    batch.setFiles([makeFile('race.md')])
    await batch.start()

    expect(batch.items.value[0].status).toBe('duplicate')
    expect(batch.items.value[0].message).toBe('该文件已存在于知识库中')
  })

  it('命中 429 限流时按 Retry-After 退避后自动重试，最终成功', async () => {
    vi.mocked(knowledgeApi.checkFile).mockResolvedValue({ data: { duplicate: false } } as any)
    vi.mocked(knowledgeApi.uploadFile)
      .mockRejectedValueOnce({ response: { status: 429, headers: { 'retry-after': '0.01' } } })
      .mockRejectedValueOnce({ response: { status: 429, headers: { 'retry-after': '0.01' } } })
      .mockResolvedValueOnce({ data: { knowledge_id: 'k3', status: 'processing' } } as any)

    const batch = useBatchUpload('kb-1')
    batch.setFiles([makeFile('rate-limited.md')])
    await batch.start()

    expect(knowledgeApi.uploadFile).toHaveBeenCalledTimes(3)
    expect(batch.items.value[0].status).toBe('success')
  })

  it('429 重试次数耗尽后标记 error，且不会无限重试', async () => {
    vi.mocked(knowledgeApi.checkFile).mockResolvedValue({ data: { duplicate: false } } as any)
    vi.mocked(knowledgeApi.uploadFile).mockRejectedValue({
      response: { status: 429, headers: { 'retry-after': '0.01' } }
    })

    const batch = useBatchUpload('kb-1', { maxRateLimitRetries: 2 })
    batch.setFiles([makeFile('always-limited.md')])
    await batch.start()

    // 首次尝试 + 2 次重试 = 3 次调用
    expect(knowledgeApi.uploadFile).toHaveBeenCalledTimes(3)
    expect(batch.items.value[0].status).toBe('error')
    expect(batch.items.value[0].message).toContain('请求过于频繁')
  })

  it('上传失败时标记 error 并保留错误信息，不影响其他文件', async () => {
    vi.mocked(knowledgeApi.checkFile).mockResolvedValue({ data: { duplicate: false } } as any)
    vi.mocked(knowledgeApi.uploadFile)
      .mockRejectedValueOnce(new Error('网络中断'))
      .mockResolvedValueOnce({ data: { knowledge_id: 'k2', status: 'processing' } } as any)

    const batch = useBatchUpload('kb-1', { concurrency: 1 })
    batch.setFiles([makeFile('bad.md'), makeFile('good.md')])
    await batch.start()

    expect(batch.items.value[0].status).toBe('error')
    expect(batch.items.value[0].message).toBe('网络中断')
    expect(batch.items.value[1].status).toBe('success')
    expect(batch.summary.value).toMatchObject({ total: 2, success: 1, error: 1, settled: 2 })
  })
})

describe('useBatchUpload/并发控制', () => {
  it('并发数限制为 concurrency，不会同时发起超过上限的上传请求', async () => {
    vi.mocked(knowledgeApi.checkFile).mockResolvedValue({ data: { duplicate: false } } as any)

    const deferreds = [deferred<any>(), deferred<any>(), deferred<any>()]
    let callIndex = 0
    let inFlight = 0
    let maxInFlight = 0

    vi.mocked(knowledgeApi.uploadFile).mockImplementation(async () => {
      inFlight += 1
      maxInFlight = Math.max(maxInFlight, inFlight)
      const d = deferreds[callIndex++]
      const result = await d.promise
      inFlight -= 1
      return result
    })

    const batch = useBatchUpload('kb-1', { concurrency: 2 })
    batch.setFiles([makeFile('a.md'), makeFile('b.md'), makeFile('c.md')])
    const runPromise = batch.start()

    // 等待前两个 worker 都已进入 uploadFile 的 in-flight 计数
    await vi.waitFor(() => expect(inFlight).toBe(2))
    expect(maxInFlight).toBe(2)

    deferreds[0].resolve({ data: {} })
    await vi.waitFor(() => expect(inFlight).toBe(2)) // 第三个文件顶上第一个的名额
    deferreds[1].resolve({ data: {} })
    deferreds[2].resolve({ data: {} })
    await runPromise

    expect(maxInFlight).toBe(2)
    expect(batch.summary.value.success).toBe(3)
  })
})
