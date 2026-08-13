import { defineStore } from 'pinia'
import { ref } from 'vue'
import { storage } from '@/utils/security'
import { i18n } from '@/i18n'

interface Settings {
  theme: 'light' | 'dark' | 'auto'
  language: 'zh-CN' | 'en-US'
  fontSize: number
  sidebarCollapsed: boolean
  showThinking: boolean
  autoSave: boolean
}

// fontSize 直接作为根元素 font-size(px)。设计 token（--text-*）均以 rem 定义、
// 相对根字号缩放，故默认值必须对齐设计基线 16px（--text-base: 1rem），否则
// 启动 init() 时会整体缩放界面。
const defaultSettings: Settings = {
  theme: 'light',
  language: 'zh-CN',
  fontSize: 16,
  sidebarCollapsed: false,
  showThinking: true,
  autoSave: true
}

export const useSettingsStore = defineStore('settings', () => {
  // 状态
  const settings = ref<Settings>({ ...defaultSettings })

  // 初始化：读取本地持久化并应用到界面（在 main.ts 启动时调用）
  function init() {
    const saved = storage.get<Settings>('app_settings')
    if (saved) {
      settings.value = { ...defaultSettings, ...saved }
    }
    applyAll()
  }

  // 更新单个设置
  function updateSetting<K extends keyof Settings>(key: K, value: Settings[K]) {
    settings.value[key] = value
    saveSettings()
    applyAll()
  }

  // 更新多个设置
  function updateSettings(newSettings: Partial<Settings>) {
    settings.value = { ...settings.value, ...newSettings }
    saveSettings()
    applyAll()
  }

  // 保存到本地存储
  function saveSettings() {
    storage.set('app_settings', settings.value)
  }

  // 将全部偏好应用到界面
  function applyAll() {
    applyTheme()
    applyLanguage()
    applyFontSize()
  }

  // 应用主题（切换 <html> 的 dark class）
  function applyTheme() {
    const theme = settings.value.theme
    const actualTheme: 'light' | 'dark' = theme === 'auto'
      ? (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
      : theme

    document.documentElement.classList.toggle('dark', actualTheme === 'dark')
  }

  // 应用语言（同步 vue-i18n 当前 locale）
  function applyLanguage() {
    i18n.global.locale.value = settings.value.language
  }

  // 应用字体大小（设置根字号，rem token 随之缩放）
  function applyFontSize() {
    document.documentElement.style.fontSize = `${settings.value.fontSize}px`
  }

  // 重置设置
  function resetSettings() {
    settings.value = { ...defaultSettings }
    saveSettings()
    applyAll()
  }

  return {
    settings,
    init,
    updateSetting,
    updateSettings,
    resetSettings
  }
})
