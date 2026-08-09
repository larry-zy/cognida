<template>
  <div class="platform-container">
    <!-- 左侧导航栏（可收缩：展开显示图标+标题，收起仅图标） -->
    <aside class="rail" :class="{ 'rail--collapsed': collapsed }">
      <div class="rail__head">
        <router-link to="/home" class="rail__logo" title="Link">
          <span>L</span>
        </router-link>
        <span class="rail__brand" v-show="!collapsed">Cognida</span>
        <button
          class="rail__toggle"
          :title="collapsed ? '展开菜单' : '收起菜单'"
          @click="toggleCollapse"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"
            stroke-linecap="round" stroke-linejoin="round">
            <path v-if="collapsed" d="M9 6l6 6-6 6" />
            <path v-else d="M15 6l-6 6 6 6" />
          </svg>
        </button>
      </div>

      <nav class="rail__nav">
        <router-link
          v-for="item in menuItems"
          :key="item.path"
          :to="item.path"
          class="rail__item"
          :class="{ active: isActive(item.path) }"
          :title="collapsed ? item.title : undefined"
        >
          <!-- 内联线性图标（自研，不依赖 Element Plus）。
               v-html 仅允许注入下方 ICONS 写死的编译期常量，严禁接入任何动态/用户数据 -->
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"
            stroke-linecap="round" stroke-linejoin="round" v-html="item.icon" />
          <span class="rail__label" v-show="!collapsed">{{ item.title }}</span>
        </router-link>
      </nav>

      <div class="rail__footer">
        <div class="rail__profile">
          <div class="rail__avatar" :title="`${authStore.username} · ${authStore.email}`">
            <span>{{ authStore.username.charAt(0).toUpperCase() }}</span>
          </div>
          <span class="rail__label rail__username" v-show="!collapsed">{{ authStore.username }}</span>
        </div>
        <button class="rail__item rail__logout" :title="collapsed ? '退出登录' : undefined" @click="handleLogout">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"
            stroke-linecap="round" stroke-linejoin="round">
            <path d="M14 4H6a1 1 0 0 0-1 1v14a1 1 0 0 0 1 1h8" />
            <path d="M10 12h11" />
            <path d="M18 9l3 3-3 3" />
          </svg>
          <span class="rail__label" v-show="!collapsed">退出登录</span>
        </button>
      </div>
    </aside>

    <!-- 主内容区（顶栏由各页面自有，见 ui-preview-*.html 定稿） -->
    <main class="main-content">
      <div class="content-wrapper">
        <router-view />
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import toast from '@/utils/toast'
import { ElMessageBox } from '@/utils/confirm'
import { useAuthStore } from '@/stores/auth'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()

// 菜单收缩状态（持久化到 localStorage）
const RAIL_COLLAPSE_KEY = 'link:rail-collapsed'
const collapsed = ref(localStorage.getItem(RAIL_COLLAPSE_KEY) === '1')

function toggleCollapse() {
  collapsed.value = !collapsed.value
  localStorage.setItem(RAIL_COLLAPSE_KEY, collapsed.value ? '1' : '0')
}

// 线性图标（24 viewBox 内联 path，stroke 由 currentColor 控制）
const ICONS = {
  home: '<path d="M3 10.5 12 3l9 7.5"/><path d="M5 9.5V21h14V9.5"/><path d="M10 21v-6h4v6"/>',
  kb: '<path d="M4 5h16v11H9l-5 4V5z"/><path d="M8 9h8"/><path d="M8 12h5"/>',
  ai: '<path d="M4 20h16"/><path d="M5 16l4.5-5.5 3 3L18 7"/><path d="M15 7h3v3"/>',
  knowledge: '<path d="M6 3h9l4 4v14H6V3z"/><path d="M15 3v4h4"/><path d="M9 12h6"/><path d="M9 16h6"/>',
  graph: '<circle cx="6" cy="6" r="2.5"/><circle cx="18" cy="8" r="2.5"/><circle cx="12" cy="18" r="2.5"/><path d="M8.4 6.8l7.2 1"/><path d="M7 8.2l4 7.4"/><path d="M16.8 10.1 13 15.9"/>',
  evaluation: '<path d="M5 21V11"/><path d="M12 21V5"/><path d="M19 21v-8"/>',
  datasets: '<ellipse cx="12" cy="5" rx="7" ry="2.5"/><path d="M5 5v14c0 1.4 3.1 2.5 7 2.5s7-1.1 7-2.5V5"/><path d="M5 12c0 1.4 3.1 2.5 7 2.5s7-1.1 7-2.5"/>',
  quality: '<path d="M12 3l7 3v5c0 4.5-3 7.5-7 9-4-1.5-7-4.5-7-9V6l7-3z"/><path d="M9 12l2 2 4-4"/>',
  datasource: '<ellipse cx="12" cy="5" rx="7" ry="2.5"/><path d="M5 5v14c0 1.4 3.1 2.5 7 2.5s7-1.1 7-2.5V5"/><path d="M5 12c0 1.4 3.1 2.5 7 2.5s7-1.1 7-2.5"/><path d="M15 14l2 2 3-3"/>',
  audit: '<path d="M6 3h9l4 4v14H6V3z"/><path d="M15 3v4h4"/><path d="M9 13h6"/><path d="M9 16h4"/><circle cx="9" cy="9" r="1"/>',
  trace: '<circle cx="6" cy="6" r="2"/><circle cx="6" cy="18" r="2"/><circle cx="18" cy="12" r="2"/><path d="M6 8v8"/><path d="M8 6h5a3 3 0 0 1 3 3v1"/><path d="M8 18h5a3 3 0 0 0 3-3v-1"/>',
  settings: '<path d="M4 7h16"/><path d="M4 12h16"/><path d="M4 17h16"/><circle cx="9" cy="7" r="2"/><circle cx="15" cy="12" r="2"/><circle cx="7" cy="17" r="2"/>'
}

// 菜单项
const menuItems = computed(() => [
  { path: '/home', title: t('menu.home'), icon: ICONS.home },
  { path: '/kb-assistant', title: t('menu.kb'), icon: ICONS.kb },
  { path: '/data-agent', title: t('menu.dataAgent'), icon: ICONS.ai },
  { path: '/knowledge', title: t('menu.knowledge'), icon: ICONS.knowledge },
  { path: '/graphs', title: t('menu.graph'), icon: ICONS.graph },
  { path: '/evaluation', title: t('menu.evaluation'), icon: ICONS.evaluation },
  { path: '/datasets', title: t('menu.datasets'), icon: ICONS.datasets },
  { path: '/quality', title: t('menu.quality'), icon: ICONS.quality },
  { path: '/datasource', title: t('menu.datasource'), icon: ICONS.datasource },
  { path: '/audit-logs', title: t('menu.audit'), icon: ICONS.audit },
  { path: '/traces', title: t('menu.trace'), icon: ICONS.trace },
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

/* ==================== 导航栏（可收缩） ==================== */
.rail {
  width: 208px;
  flex: 0 0 208px;
  display: flex;
  flex-direction: column;
  padding: 12px 10px;
  background: var(--color-bg-secondary);
  border-radius: var(--radius-md);
  transition: width 0.2s ease, flex-basis 0.2s ease;
}

.rail--collapsed {
  width: 60px;
  flex: 0 0 60px;
  padding: 12px 0;
  align-items: center;
}

/* 顶部：logo + 品牌名 + 收缩按钮 */
.rail__head {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 0 2px;
  margin-bottom: 14px;
}

.rail--collapsed .rail__head {
  flex-direction: column;
  gap: 12px;
  padding: 0;
}

.rail__logo {
  width: 34px;
  height: 34px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 5px;
  background: var(--primary);
  color: var(--on-primary);
  font-family: var(--font-display);
  font-size: 17px;
  font-weight: 700;
  text-decoration: none;
}

.rail__brand {
  flex: 1;
  font-family: var(--font-display);
  font-size: 16px;
  font-weight: 700;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
}

.rail__toggle {
  width: 28px;
  height: 28px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: 5px;
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
}

.rail__toggle svg {
  width: 18px;
  height: 18px;
}

.rail__toggle:hover {
  background: var(--bg-elevated);
  color: var(--text-primary);
}

.rail__nav {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 4px;
  width: 100%;
  padding: 0;
}

.rail__item {
  height: 40px;
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
  padding: 0 11px;
  flex-shrink: 0;
  border: none;
  border-radius: 5px;
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
  text-decoration: none;
  transition: background 0.15s ease, color 0.15s ease;
}

.rail--collapsed .rail__item {
  width: 40px;
  padding: 0;
  justify-content: center;
  gap: 0;
}

.rail__item svg {
  width: 19px;
  height: 19px;
  flex-shrink: 0;
}

.rail__label {
  font-size: 13px;
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
}

.rail__item:hover {
  background: var(--bg-elevated);
  color: var(--text-primary);
}

.rail__item.active {
  background: var(--primary-dim);
  color: var(--primary);
}

.rail__footer {
  display: flex;
  flex-direction: column;
  gap: 4px;
  width: 100%;
}

.rail--collapsed .rail__footer {
  align-items: center;
}

.rail__profile {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 4px 11px;
}

.rail--collapsed .rail__profile {
  padding: 4px 0;
  justify-content: center;
}

.rail__avatar {
  width: 30px;
  height: 30px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 5px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-default);
  color: var(--text-secondary);
  font-size: 12px;
  font-weight: 600;
}

.rail__username {
  color: var(--text-secondary);
}

.rail__logout:hover {
  color: var(--danger);
}

/* ==================== 主内容区 ==================== */
.main-content {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--color-bg-secondary);
  border-radius: var(--radius-md);
}

.content-wrapper {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 24px;
}
</style>
