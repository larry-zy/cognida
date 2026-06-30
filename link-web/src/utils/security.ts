/**
 * 安全相关工具函数
 */

/**
 * XSS防护：转义HTML特殊字符
 */
export function escapeHtml(text: string): string {
  const map: Record<string, string> = {
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#039;'
  }
  return text.replace(/[&<>"']/g, (m) => map[m])
}

/**
 * 检查URL是否安全
 */
export function isSafeUrl(url: string): boolean {
  try {
    const parsed = new URL(url)
    // 只允许http和https协议
    return ['http:', 'https:'].includes(parsed.protocol)
  } catch {
    return false
  }
}

/**
 * 生成随机字符串
 */
export function generateRandomString(length: number = 16): string {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789'
  let result = ''
  for (let i = 0; i < length; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length))
  }
  return result
}

/**
 * 本地存储封装（带加密）
 */
const STORAGE_PREFIX = 'link_'

export const storage = {
  set<T>(key: string, value: T): void {
    try {
      const str = JSON.stringify(value)
      localStorage.setItem(STORAGE_PREFIX + key, btoa(encodeURIComponent(str)))
    } catch (e) {
      console.error('Storage set error:', e)
    }
  },

  get<T>(key: string): T | null {
    try {
      const item = localStorage.getItem(STORAGE_PREFIX + key)
      if (!item) return null
      const str = decodeURIComponent(atob(item))
      return JSON.parse(str) as T
    } catch (e) {
      console.error('Storage get error:', e)
      return null
    }
  },

  remove(key: string): void {
    localStorage.removeItem(STORAGE_PREFIX + key)
  },

  clear(): void {
    // 只清除当前应用的数据
    Object.keys(localStorage)
      .filter(k => k.startsWith(STORAGE_PREFIX))
      .forEach(k => localStorage.removeItem(k))
  }
}

/**
 * 复制到剪贴板
 */
export async function copyToClipboard(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard) {
      await navigator.clipboard.writeText(text)
      return true
    } else {
      // 兼容旧浏览器
      const textarea = document.createElement('textarea')
      textarea.value = text
      textarea.style.position = 'fixed'
      textarea.style.opacity = '0'
      document.body.appendChild(textarea)
      textarea.select()
      const success = document.execCommand('copy')
      document.body.removeChild(textarea)
      return success
    }
  } catch (e) {
    console.error('Copy failed:', e)
    return false
  }
}
