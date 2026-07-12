import { ref, computed, readonly, type Ref, type ComputedRef } from 'vue'
import toast from '@/utils/toast'

// ========================================
// useAsyncTask —— 关键异步动作的状态机
// ========================================
//
// 统一替代各页面「布尔 loading + 一闪而过的 toast」的写法。
// 状态流转：idle → pending → success | error（success/error 可再次 run 回到 pending）。
// 就地状态由 <UiAsyncStatus> 常驻展示；toast 仍用于即时提醒（可关）。

export type AsyncStatus = 'idle' | 'pending' | 'success' | 'error'

export interface RunOptions<T> {
  /** pending 阶段就地展示的文案，默认「处理中…」 */
  pendingMessage?: string
  /** 成功文案：字符串或 (result) => string */
  successMessage?: string | ((result: T) => string)
  /** 失败文案：字符串或 (err) => string，默认取 err.message */
  errorMessage?: string | ((err: any) => string)
  /** 成功时是否弹 toast（默认：有 successMessage 才弹） */
  toastOnSuccess?: boolean
  /** 失败时是否弹 toast（默认 true） */
  toastOnError?: boolean
}

export interface AsyncTask<T> {
  status: Readonly<Ref<AsyncStatus>>
  result: Ref<T | null>
  error: Ref<any>
  /** 当前阶段的就地展示文案（pending/success/error 各自的消息） */
  message: Readonly<Ref<string>>
  isIdle: ComputedRef<boolean>
  isPending: ComputedRef<boolean>
  isSuccess: ComputedRef<boolean>
  isError: ComputedRef<boolean>
  /** 执行一次异步动作；失败被吞掉（返回 undefined），状态落到 error */
  run: (fn: () => Promise<T>, opts?: RunOptions<T>) => Promise<T | undefined>
  /** 重置回 idle，清空结果/错误/文案 */
  reset: () => void
}

export function useAsyncTask<T = any>(defaults: RunOptions<T> = {}): AsyncTask<T> {
  const status = ref<AsyncStatus>('idle')
  const result = ref<T | null>(null) as Ref<T | null>
  const error = ref<any>(null)
  const message = ref('')

  const isIdle = computed(() => status.value === 'idle')
  const isPending = computed(() => status.value === 'pending')
  const isSuccess = computed(() => status.value === 'success')
  const isError = computed(() => status.value === 'error')

  async function run(fn: () => Promise<T>, opts: RunOptions<T> = {}): Promise<T | undefined> {
    const o = { ...defaults, ...opts }
    status.value = 'pending'
    error.value = null
    message.value = o.pendingMessage ?? '处理中…'
    try {
      const r = await fn()
      result.value = r
      status.value = 'success'
      const msg = typeof o.successMessage === 'function' ? o.successMessage(r) : o.successMessage
      message.value = msg ?? '完成'
      if (msg && o.toastOnSuccess !== false) toast.success(msg)
      return r
    } catch (e: any) {
      error.value = e
      status.value = 'error'
      const msg = typeof o.errorMessage === 'function'
        ? o.errorMessage(e)
        : (o.errorMessage ?? e?.message ?? '操作失败')
      message.value = msg
      if (o.toastOnError !== false) toast.error(msg)
      return undefined
    }
  }

  function reset() {
    status.value = 'idle'
    result.value = null
    error.value = null
    message.value = ''
  }

  return {
    status: readonly(status) as Readonly<Ref<AsyncStatus>>,
    result,
    error,
    message: readonly(message) as Readonly<Ref<string>>,
    isIdle,
    isPending,
    isSuccess,
    isError,
    run,
    reset,
  }
}
