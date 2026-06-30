<template>
  <div class="platform-container">
    <!-- 侧边栏 -->
    <aside class="sidebar" :class="{ collapsed: uiStore.sidebarCollapsed }">
      <div class="sidebar-header">
        <div class="logo">
          <el-icon :size="28"><ChatDotRound /></el-icon>
          <span v-show="!uiStore.sidebarCollapsed" class="logo-text">Link</span>
        </div>
      </div>

      <!-- 导航菜单 -->
      <nav class="sidebar-nav">
        <router-link
          v-for="item in menuItems"
          :key="item.path"
          :to="item.path"
          class="nav-item"
          :class="{ active: isActive(item.path) }"
        >
          <el-icon :size="20">
            <component :is="item.icon" />
          </el-icon>
          <span v-show="!uiStore.sidebarCollapsed" class="nav-text">{{ item.title }}</span>
        </router-link>
      </nav>

      <!-- 底部用户信息 -->
      <div class="sidebar-footer">
        <div class="user-info" v-if="!uiStore.sidebarCollapsed">
          <el-avatar :size="32">{{ authStore.username.charAt(0).toUpperCase() }}</el-avatar>
          <div class="user-details">
            <div class="user-name">{{ authStore.username }}</div>
            <div class="user-email">{{ authStore.email }}</div>
          </div>
          <el-button
            link
            :icon="SwitchButton"
            @click="handleLogout"
            title="退出登录"
          />
        </div>
        <el-button
          v-else
          link
          :icon="SwitchButton"
          @click="handleLogout"
          title="退出登录"
        />
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
import { ElMessageBox, ElMessage } from '@/utils/element'
import {
  ChatDotRound,
  ChatLineRound,
  Document,
  Setting,
  User as UserIcon,
  SwitchButton,
  Share,
  DataAnalysis,
  HomeFilled
} from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { useUIStore } from '@/stores/ui'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()
const uiStore = useUIStore()

// 菜单项
const menuItems = computed(() => [
  { path: '/home', title: t('menu.home'), icon: HomeFilled },
  { path: '/ai', title: t('menu.ai'), icon: ChatLineRound },
  { path: '/knowledge', title: t('menu.knowledge'), icon: Document },
  { path: '/graphs', title: t('menu.graph'), icon: Share },
  { path: '/agent', title: t('menu.agent'), icon: UserIcon },
  { path: '/evaluation', title: t('menu.evaluation'), icon: DataAnalysis },
  { path: '/datasets', title: t('menu.datasets'), icon: Document },
  { path: '/settings', title: t('menu.settings'), icon: Setting }
])

// 判断是否激活
function isActive(path: string): boolean {
  return route.path.startsWith(path)
}

// 退出登录
async function handleLogout() {
  try {
    await ElMessageBox.confirm('确定要退出登录吗？', '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    })

    await authStore.logout()
    ElMessage.success('已退出登录')
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
  overflow: hidden;
}

.sidebar {
  width: 240px;
  background: rgba(15, 15, 18, 0.5);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border-right: 1px solid rgba(255, 255, 255, 0.1);
  display: flex;
  flex-direction: column;
  transition: all 0.3s;
  flex-shrink: 0;
}

.sidebar.collapsed {
  width: 60px;
}

.sidebar-header {
  padding: 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
}

.logo {
  display: flex;
  align-items: center;
  gap: 12px;
}

.logo-text {
  font-size: 20px;
  font-weight: 600;
  color: #22d3ee;
}

.sidebar-nav {
  flex: 1;
  padding: 16px 8px;
  overflow-y: auto;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  margin-bottom: 4px;
  border-radius: 8px;
  color: #a0a0b0;
  text-decoration: none;
  transition: all 0.3s;
}

.nav-item:hover {
  background: rgba(34, 211, 238, 0.1);
  color: #22d3ee;
}

.nav-item.active {
  background: rgba(34, 211, 238, 0.2);
  color: #22d3ee;
}

.nav-text {
  font-size: 14px;
  white-space: nowrap;
}

.sidebar-footer {
  padding: 16px;
  border-top: 1px solid rgba(255, 255, 255, 0.1);
}

.user-info {
  display: flex;
  align-items: center;
  gap: 12px;
}

.user-details {
  flex: 1;
  overflow: hidden;
}

.user-name {
  font-size: 14px;
  font-weight: 500;
  color: #f0f0f5;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.user-email {
  font-size: 12px;
  color: #a0a0b0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.main-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.content-wrapper {
  flex: 1;
  overflow-y: auto;
  padding: 24px;
}
</style>
