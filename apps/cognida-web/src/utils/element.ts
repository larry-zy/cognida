/**
 * Element Plus 按需导入工具
 * 用于过渡期间，某些页面仍使用 Element Plus 组件
 */
import { ElMessage as _ElMessage, ElMessageBox as _ElMessageBox } from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'

export const locale = zhCn

// 导出常用的 API
export const ElMessage = {
  success: (message: string) => _ElMessage.success({ message, duration: 3000 }),
  error: (message: string) => _ElMessage.error({ message, duration: 3000 }),
  warning: (message: string) => _ElMessage.warning({ message, duration: 3000 }),
  info: (message: string) => _ElMessage.info({ message, duration: 3000 })
}

export const ElMessageBox = {
  confirm: (message: string, title?: string, options?: any) => {
    return _ElMessageBox.confirm(message, title || '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
      ...options
    })
  }
}

// 类型导出
export type { FormInstance, FormRules } from 'element-plus'
