<template>
  <div class="platform-container">
    <!-- 左侧导航栏（可收缩：展开显示图标+标题，收起仅图标） -->
    <button v-if="mobileOpen" class="rail-backdrop" tabindex="-1" aria-label="关闭导航菜单" @click="closeMobileNavigation"></button>
    <aside ref="navigationPanel" id="workspace-navigation" class="rail" :class="{ 'rail--collapsed': collapsed, 'rail--mobile-open': mobileOpen }">
      <button class="rail__mobile-close" aria-label="关闭导航菜单" @click="closeMobileNavigation">关闭菜单 ×</button>
      <div class="rail__head">
        <router-link to="/home" class="rail__logo" aria-label="Cognida 工作总览">
          <span>C</span>
        </router-link>
        <span class="rail__brand" v-show="!collapsed">Cognida</span>
        <button
          class="rail__toggle"
          :title="collapsed ? '展开菜单' : '收起菜单'"
          :aria-label="collapsed ? '展开菜单' : '收起菜单'"
          :aria-expanded="!collapsed"
          @click="toggleCollapse"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"
            stroke-linecap="round" stroke-linejoin="round">
            <path v-if="collapsed" d="M9 6l6 6-6 6" />
            <path v-else d="M15 6l-6 6 6 6" />
          </svg>
        </button>
      </div>

      <TenantSwitcher v-if="!collapsed || mobileOpen" />
      <nav class="rail__nav" aria-label="主导航">
        <section v-for="group in navigationGroups" :key="group.title" class="rail__group" :aria-label="group.title">
          <h2 class="rail__group-title" v-show="!collapsed">{{ group.title }}</h2>
          <router-link
            v-for="item in group.items" :key="item.path" :to="item.path"
            class="rail__item" :class="{ active: activePath === item.path }"
            :aria-current="activePath === item.path ? 'page' : undefined"
            :aria-label="item.title" :title="collapsed ? item.title : undefined"
            @click="mobileOpen = false"
          >
            <!-- Only compiled icon paths enter v-html; never user or API data. -->
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"
              stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" v-html="ICONS[item.icon as keyof typeof ICONS]" />
            <span class="rail__label" v-show="!collapsed">{{ item.title }}</span>
          </router-link>
        </section>
      </nav>

      <div class="rail__footer">
        <div class="rail__profile">
          <div class="rail__avatar" :title="`${authStore.username} · ${authStore.email}`">
            <span>{{ authStore.username.charAt(0).toUpperCase() }}</span>
          </div>
          <span class="rail__label rail__username" v-show="!collapsed">{{ authStore.username }}</span>
        </div>
        <button class="rail__item rail__logout" aria-label="退出登录" :title="collapsed ? '退出登录' : undefined" @click="handleLogout">
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
    <main class="main-content" :inert="mobileOpen ? true : undefined">
      <div class="platform-mobile-bar"><router-link to="/home">Cognida</router-link><button ref="mobileToggle" aria-controls="workspace-navigation" :aria-expanded="mobileOpen" @click="openMobileNavigation">导航菜单</button></div>
      <div class="content-wrapper">
        <router-view :key="`${authStore.authEpoch}:${authStore.user?.id}:${authStore.currentTenant?.id}:${route.fullPath}`" />
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import TenantSwitcher from '@/components/workbench/TenantSwitcher.vue'
import '@/components/workbench/workbench.css'
import { useRouter, useRoute } from 'vue-router'
import toast from '@/utils/toast'
import { ElMessageBox } from '@/utils/confirm'
import { useAuthStore } from '@/stores/auth'
import { activeNavigationPath, navigationGroups } from './navigation'
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
  collection: '<path d="M4 5h16v14H4z"/><path d="M7 9h10"/><path d="M7 13h7"/><path d="M16 17l2 2 3-4"/>',
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

const activePath = computed(() => activeNavigationPath(route.path))
const mobileOpen = ref(false)
const mobileToggle = ref<HTMLButtonElement>()
const navigationPanel = ref<HTMLElement>()
async function openMobileNavigation() {
  mobileOpen.value = true
  await nextTick()
  navigationPanel.value?.querySelector<HTMLButtonElement>('.rail__mobile-close')?.focus()
}
async function closeMobileNavigation() {
  mobileOpen.value = false
  await nextTick()
  mobileToggle.value?.focus()
}
function handleEscape(event: KeyboardEvent) {
  if (event.key === 'Escape' && mobileOpen.value) { event.preventDefault(); void closeMobileNavigation() }
}
watch(() => route.fullPath, () => { mobileOpen.value = false })
let desktopViewport: MediaQueryList | undefined
function handleDesktopViewport(event: MediaQueryListEvent) {
  if (event.matches) mobileOpen.value = false
}
onMounted(() => {
  document.addEventListener('keydown', handleEscape)
  desktopViewport = window.matchMedia?.('(min-width: 768px)')
  desktopViewport?.addEventListener('change', handleDesktopViewport)
})
onBeforeUnmount(() => {
  document.removeEventListener('keydown', handleEscape)
  desktopViewport?.removeEventListener('change', handleDesktopViewport)
})

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
  min-height: 0;
  overflow-y: auto;
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

.rail__group { width: 100%; }
.rail__group + .rail__group { margin-top: 14px; padding-top: 10px; border-top: 1px solid var(--border-subtle); }
.rail__group-title { margin: 0 11px 7px; font-size: 10px; font-weight: 500; letter-spacing: .08em; color: var(--text-muted); }
.rail--collapsed .rail__group { display: flex; flex-direction: column; align-items: center; }
.rail__item:focus-visible, .rail__toggle:focus-visible { outline: 2px solid var(--primary); outline-offset: -2px; }
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
.platform-mobile-bar, .rail-backdrop, .rail__mobile-close { display: none; }
@media (max-width: 767px) {
  .platform-container { padding: 0; gap: 0; height: 100dvh; }
  .main-content { border-radius: 0; }
  .content-wrapper { padding: 20px 16px; }
  .platform-mobile-bar { display: flex; align-items: center; justify-content: space-between; padding: 12px 16px; border-bottom: 1px solid var(--border-default); }
  .platform-mobile-bar a { font-family: var(--font-display); color: var(--text-primary); font-weight: 700; text-decoration: none; }
  .platform-mobile-bar button { color: var(--text-primary); background: var(--bg-elevated); border: 1px solid var(--border-default); border-radius: 5px; padding: 8px 12px; }
  /* Visibility must change immediately: a transition leaves the drawer hidden
     when nextTick moves focus, so browsers silently discard that focus. */
  .rail { position: fixed; z-index: 50; top: 12px; left: 12px; bottom: 12px; width: min(260px, calc(100vw - 48px)); padding: 16px 12px; transform: translateX(calc(-100% - 24px)); visibility: hidden; transition: transform .2s ease; box-shadow: 0 16px 60px #0004; }
  .rail--mobile-open { visibility: visible; transform: translateX(0); }
  .rail-backdrop { display: block; position: fixed; z-index: 49; inset: 0; border: 0; background: #0006; }
  .rail__toggle { display: none; }
  .rail__mobile-close { display: block; border: 0; background: transparent; color: var(--text-secondary); align-self: flex-end; padding: 0 0 14px; font-size: 12px; }
  .rail--collapsed { align-items: stretch; }
  .rail--collapsed .rail__head { flex-direction: row; padding: 0 2px; }
  .rail--collapsed .rail__brand, .rail--collapsed .rail__label, .rail--collapsed .rail__group-title { display: block !important; }
  .rail--collapsed .rail__group { align-items: stretch; }
  .rail--collapsed .rail__item { width: 100%; padding: 0 11px; justify-content: flex-start; gap: 12px; }
  .rail--collapsed .rail__footer { align-items: stretch; }
  .rail--collapsed .rail__profile { justify-content: flex-start; padding: 4px 11px; }
}
@media (prefers-reduced-motion: reduce) { .rail { transition: none; } }
</style>
