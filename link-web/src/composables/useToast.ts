import { h, render } from 'vue'
import UiToast from '@/components/ui/UiToast.vue'

type ToastType = 'success' | 'error' | 'warning' | 'info'

let toastInstance: any = null

function getToastInstance() {
  if (!toastInstance) {
    // 创建临时容器
    const container = document.createElement('div')
    document.body.appendChild(container)

    // 渲染组件
    const vnode = h(UiToast)
    render(vnode, container)
    toastInstance = vnode.component
  }

  return toastInstance.exposed
}

export function toast(message: string, type?: ToastType, title?: string) {
  const instance = getToastInstance()
  if (instance) {
    return instance.addToast({ message, type, title })
  }
}

export const toastSuccess = (message: string, title?: string) => toast(message, 'success', title)
export const toastError = (message: string, title?: string) => toast(message, 'error', title)
export const toastWarning = (message: string, title?: string) => toast(message, 'warning', title)
export const toastInfo = (message: string, title?: string) => toast(message, 'info', title)

// 导出类似于 Element Plus ElMessage 的 API
export default {
  success: toastSuccess,
  error: toastError,
  warning: toastWarning,
  info: toastInfo,
  show: toast
}
