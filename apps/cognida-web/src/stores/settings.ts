import { defineStore } from 'pinia'
import { ref } from 'vue'
import { storage } from '@/utils/security'

interface Settings {
  theme: 'light' | 'dark' | 'auto'
  language: 'zh-CN' | 'en-US'
  fontSize: number
  sidebarCollapsed: boolean
  showThinking: boolean
  autoSave: boolean
}

const defaultSettings: Settings = {
  theme: 'light',
  language: 'zh-CN',
  fontSize: 14,
  sidebarCollapsed: false,
  showThinking: true,
  autoSave: true
}

export const useSettingsStore = defineStore('settings', () => {
  // 状态
  const settings = ref<Settings>({ ...defaultSettings })

  // 初始化
  function init() {
    const saved = storage.get<Settings>('app_settings')
    if (saved) {
      settings.value = { ...defaultSettings, ...saved }
    }
    applyTheme()
  }

  // 更新设置
  function updateSetting<K extends keyof Settings>(key: K, value: Settings[K]) {
    settings.value[key] = value
    saveSettings()

    // 特殊处理
    if (key === 'theme') {
      applyTheme()
    }
  }

  // 更新多个设置
  function updateSettings(newSettings: Partial<Settings>) {
    settings.value = { ...settings.value, ...newSettings }
    saveSettings()
    applyTheme()
  }

  // 保存到本地存储
  function saveSettings() {
    storage.set('app_settings', settings.value)
  }

  // 应用主题
  function applyTheme() {
    const theme = settings.value.theme
    let actualTheme: 'light' | 'dark' = theme === 'auto'
      ? (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
      : theme

    document.documentElement.classList.toggle('dark', actualTheme === 'dark')
  }

  // 重置设置
  function resetSettings() {
    settings.value = { ...defaultSettings }
    saveSettings()
    applyTheme()
  }

  return {
    settings,
    init,
    updateSetting,
    updateSettings,
    resetSettings
  }
})
