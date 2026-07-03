<template>
  <div class="platform-container">
    <!-- 左侧图标导航栏（60px rail） -->
    <aside class="rail">
      <router-link to="/home" class="rail__logo" title="Link">
        <span>L</span>
      </router-link>

      <nav class="rail__nav">
        <router-link
          v-for="item in menuItems"
          :key="item.path"
          :to="item.path"
          class="rail__item"
          :class="{ active: isActive(item.path) }"
          :title="item.title"
        >
          <!-- 内联线性图标（自研，不依赖 Element Plus）。
               v-html 仅允许注入下方 ICONS 写死的编译期常量，严禁接入任何动态/用户数据 -->
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"
            stroke-linecap="round" stroke-linejoin="round" v-html="item.icon" />
        </router-link>
      </nav>

      <div class="rail__footer">
        <div class="rail__avatar" :title="`${authStore.username} · ${authStore.email}`">
          <span>{{ authStore.username.charAt(0).toUpperCase() }}</span>
        </div>
        <button class="rail__item rail__logout" title="退出登录" @click="handleLogout">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"
            stroke-linecap="round" stroke-linejoin="round">
            <path d="M14 4H6a1 1 0 0 0-1 1v14a1 1 0 0 0 1 1h8" />
            <path d="M10 12h11" />
            <path d="M18 9l3 3-3 3" />
          </svg>
        </button>
      </div>
    </aside>

    <!-- 主内容区 -->
    <main class="main-content">
      <div class="content-wrapper">
        <router-view />
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import toast from '@/utils/toast'
import { ElMessageBox } from '@/utils/confirm'
import { useAuthStore } from '@/stores/auth'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()

// 线性图标（24 viewBox 内联 path，stroke 由 currentColor 控制）
const ICONS = {
  home: '<path d="M3 10.5 12 3l9 7.5"/><path d="M5 9.5V21h14V9.5"/><path d="M10 21v-6h4v6"/>',
  kb: '<path d="M4 5h16v11H9l-5 4V5z"/><path d="M8 9h8"/><path d="M8 12h5"/>',
  ai: '<path d="M4 20h16"/><path d="M5 16l4.5-5.5 3 3L18 7"/><path d="M15 7h3v3"/>',
  knowledge: '<path d="M6 3h9l4 4v14H6V3z"/><path d="M15 3v4h4"/><path d="M9 12h6"/><path d="M9 16h6"/>',
  graph: '<circle cx="6" cy="6" r="2.5"/><circle cx="18" cy="8" r="2.5"/><circle cx="12" cy="18" r="2.5"/><path d="M8.4 6.8l7.2 1"/><path d="M7 8.2l4 7.4"/><path d="M16.8 10.1 13 15.9"/>',
  agent: '<circle cx="12" cy="8" r="4"/><path d="M4 21c1.5-4 4.5-6 8-6s6.5 2 8 6"/>',
  evaluation: '<path d="M5 21V11"/><path d="M12 21V5"/><path d="M19 21v-8"/>',
  datasets: '<ellipse cx="12" cy="5" rx="7" ry="2.5"/><path d="M5 5v14c0 1.4 3.1 2.5 7 2.5s7-1.1 7-2.5V5"/><path d="M5 12c0 1.4 3.1 2.5 7 2.5s7-1.1 7-2.5"/>',
  settings: '<path d="M4 7h16"/><path d="M4 12h16"/><path d="M4 17h16"/><circle cx="9" cy="7" r="2"/><circle cx="15" cy="12" r="2"/><circle cx="7" cy="17" r="2"/>'
}

// 菜单项
const menuItems = computed(() => [
  { path: '/home', title: t('menu.home'), icon: ICONS.home },
  { path: '/kb', title: t('menu.kb'), icon: ICONS.kb },
  { path: '/ai', title: t('menu.ai'), icon: ICONS.ai },
  { path: '/knowledge', title: t('menu.knowledge'), icon: ICONS.knowledge },
  { path: '/graphs', title: t('menu.graph'), icon: ICONS.graph },
  { path: '/agent', title: t('menu.agent'), icon: ICONS.agent },
  { path: '/evaluation', title: t('menu.evaluation'), icon: ICONS.evaluation },
  { path: '/datasets', title: t('menu.datasets'), icon: ICONS.datasets },
  { path: '/settings', title: t('menu.settings'), icon: ICONS.settings }
])

// 判断是否激活
function isActive(path: string): boolean {
  return route.path.startsWith(path)
}

// 退出登录
async function handleLogout() {
  try {
    await ElMessageBox.confirm({
      message: '确定要退出登录吗？',
      title: '提示',
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    })

    await authStore.logout()
    toast.success('已退出登录')
    router.push('/login')
  } catch {
    // 用户取消
  }
}
</script>

<style scoped>
.platform-container {
  display: flex;
  width: 100%;
  height: 100vh;
  gap: 12px;
  padding: 12px;
  overflow: hidden;
}

/* ==================== 图标导航栏 ==================== */
.rail {
  width: 60px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 14px 0;
  background: var(--color-bg-secondary);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-lg);
}

.rail__logo {
  width: 34px;
  height: 34px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-md);
  background: var(--primary-dim);
  border: 1px solid var(--color-border-default);
  color: var(--color-primary);
  font-family: var(--font-display);
  font-size: 18px;
  font-weight: 700;
  text-decoration: none;
  margin-bottom: 18px;
}

.rail__nav {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  overflow-y: auto;
  width: 100%;
  padding: 0 10px;
}

.rail__item {
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  border: none;
  border-radius: var(--radius-md);
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
  transition: background var(--transition-fast), color var(--transition-fast);
}

.rail__item svg {
  width: 20px;
  height: 20px;
}

.rail__item:hover {
  background: var(--color-bg-hover);
  color: var(--color-text-secondary);
}

.rail__item.active {
  background: var(--color-bg-active);
  color: var(--color-primary);
}

.rail__footer {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding-top: 12px;
  border-top: 1px solid var(--color-border-subtle);
  width: 100%;
}

.rail__avatar {
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: var(--color-bg-tertiary);
  border: 1px solid var(--color-border-default);
  color: var(--color-text-secondary);
  font-size: 13px;
  font-weight: 600;
  margin: 0 auto;
}

.rail__logout:hover {
  color: var(--color-danger);
}

/* ==================== 主内容区 ==================== */
.main-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--color-bg-secondary);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-lg);
}

.content-wrapper {
  flex: 1;
  overflow-y: auto;
  padding: 24px;
}
</style>
