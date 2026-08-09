/**
 * Confirm 确认对话框工具
 * 替代 Element Plus 的 ElMessageBox
 */

import { createApp, h } from 'vue'
import UiConfirm from '@/components/ui/UiConfirm.vue'

export type ConfirmType = 'warning' | 'danger' | 'success' | 'info'

interface ConfirmOptions {
  title?: string
  message: string
  type?: ConfirmType
  confirmButtonText?: string
  cancelButtonText?: string
  showCancelButton?: boolean
}

interface MessageBox {
  confirm: (options: ConfirmOptions) => Promise<void>
  alert: (message: string, title?: string) => Promise<void>
}

function showConfirm(options: ConfirmOptions): Promise<void> {
  return new Promise((resolve, reject) => {
    // 创建容器
    const container = document.createElement('div')
    document.body.appendChild(container)

    // 创建 Vue 应用
    const app = createApp({
      render() {
        return h(UiConfirm, {
          show: true,
          ...options,
          'onUpdate:show': (show: boolean) => {
            if (!show) {
              reject('cancel')
              cleanup()
            }
          },
          onConfirm: () => {
            resolve()
            cleanup()
          },
          onCancel: () => {
            reject('cancel')
            cleanup()
          }
        })
      }
    })

    app.mount(container)

    function cleanup() {
      app.unmount()
      container.remove()
    }
  })
}

function showAlert(message: string, title?: string): Promise<void> {
  return showConfirm({
    message,
    title,
    showCancelButton: false,
    type: 'info'
  })
}

// 导出兼容 ElMessageBox 的 API
export const ElMessageBox: MessageBox = {
  confirm: showConfirm,
  alert: showAlert
}

export default ElMessageBox
