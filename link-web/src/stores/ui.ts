import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useUIStore = defineStore('pic', () => {
  // 侧边栏状态
  const sidebarCollapsed = ref(false)

  // 加载状态
  const loading = ref(false)

  // 当前选中的会话ID
  const currentSessionId = ref<string>('')

  // 显示/隐藏侧边栏
  function toggleSidebar() {
    sidebarCollapsed.value = !sidebarCollapsed.value
  }

  function setSidebar(collapsed: boolean) {
    sidebarCollapsed.value = collapsed
  }

  // 设置加载状态
  function setLoading(value: boolean) {
    loading.value = value
  }

  // 设置当前会话
  function setCurrentSession(sessionId: string) {
    currentSessionId.value = sessionId
  }

  return {
    sidebarCollapsed,
    loading,
    currentSessionId,
    toggleSidebar,
    setSidebar,
    setLoading,
    setCurrentSession
  }
})
