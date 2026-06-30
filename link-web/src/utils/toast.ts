/**
 * Toast 通知工具
 * 替代 Element Plus 的 ElMessage
 */

import { createApp, h } from 'vue'
import UiToast from '@/components/ui/UiToast.vue'

let appInstance: any = null
let container: HTMLElement | null = null

function ensureInstance() {
  if (!appInstance) {
    container = document.createElement('div')
    document.body.appendChild(container)

    const app = createApp({
      render() {
        return h(UiToast)
      }
    })

    appInstance = app.mount(container)
  }
  return appInstance
}

export type ToastType = 'success' | 'error' | 'warning' | 'info'

interface ToastOptions {
  type?: ToastType
  title?: string
  message: string
  duration?: number
  closable?: boolean
}

function show(options: ToastOptions): void
function show(message: string, type?: ToastType, title?: string): void
function show(arg1: string | ToastOptions, type?: ToastType, title?: string): void {
  const instance = ensureInstance()

  let options: ToastOptions
  if (typeof arg1 === 'string') {
    options = {
      message: arg1,
      type,
      title
    }
  } else {
    options = arg1
  }

  instance.addToast(options)
}

export const toastSuccess = (message: string, title?: string) => show(message, 'success', title)
export const toastError = (message: string, title?: string) => show(message, 'error', title)
export const toastWarning = (message: string, title?: string) => show(message, 'warning', title)
export const toastInfo = (message: string, title?: string) => show(message, 'info', title)

// 导出兼容 ElMessage 的 API
export default {
  success: toastSuccess,
  error: toastError,
  warning: toastWarning,
  info: toastInfo,
  show
}

// 同时导出 ElMessage 别名以便直接替换
export const ElMessage = {
  success: toastSuccess,
  error: toastError,
  warning: toastWarning,
  info: toastInfo,
  show
}
