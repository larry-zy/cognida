<template>
  <!-- 全局背景层 -->
  <div class="app-background">
    <div class="bg-primary"></div>
    <div class="bg-image"></div>
    <div class="bg-overlay"></div>
    <div class="bg-noise"></div>
  </div>

  <!-- 应用内容（带路由过渡） -->
  <div class="app-content">
    <AppTransition />
  </div>

  <!-- 全局 Toast 组件 -->
  <UiToast />
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useAuthStore } from '@/stores/auth'
import AppTransition from '@/components/transitions/AppTransition.vue'
import UiToast from '@/components/ui/UiToast.vue'

const authStore = useAuthStore()

onMounted(() => {
  // 初始化时检查本地存储的Token
  authStore.checkAuth()
})
</script>

<style>
/* ==================== 全局重置 ==================== */
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

html, body, #app {
  width: 100%;
  height: 100%;
  font-family: var(--font-body, -apple-system, BlinkMacSystemFont, 'PingFang SC', 'Segoe UI', sans-serif);
  color: var(--color-text-primary, #e8e6df);
  background: var(--color-bg-primary, #0c0f15);
  overflow-x: hidden;
}

body {
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}

/* ==================== 全局背景 ==================== */
.app-background {
  position: fixed;
  inset: 0;
  z-index: 0;
  overflow: hidden;
}

.app-content {
  position: relative;
  z-index: 10;
  width: 100%;
  height: 100%;
}

.bg-primary {
  position: absolute;
  inset: 0;
  background: #0c0f15;
  z-index: 0;
}

/* 壁纸全透出，仅用一层轻压暗保证文字对比度 */
.bg-image {
  position: absolute;
  inset: 0;
  background-image: url('/img/backgroud.png');
  background-size: cover;
  background-position: center;
  background-repeat: no-repeat;
  z-index: 1;
}

.bg-overlay {
  position: absolute;
  inset: 0;
  background: linear-gradient(180deg, rgba(10, 13, 19, 0.18) 0%, rgba(10, 13, 19, 0.42) 100%);
  z-index: 2;
}

.bg-noise {
  display: none;
}

/* ==================== 滚动条样式 ==================== */
* {
  scrollbar-width: thin;
  scrollbar-color: rgba(255, 255, 255, 0.1) transparent;
}

::-webkit-scrollbar {
  width: 6px;
  height: 6px;
}

::-webkit-scrollbar-track {
  background: transparent;
}

::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.1);
  border-radius: 3px;
}

::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.15);
}

/* ==================== 选中文本样式 ==================== */
::selection {
  background: rgba(156, 180, 205, 0.28);
  color: #e8e6df;
}

::-moz-selection {
  background: rgba(156, 180, 205, 0.28);
  color: #e8e6df;
}

/* ==================== 链接样式 ==================== */
a {
  color: #9cb4cd;
  text-decoration: none;
  transition: color 150ms ease;
}

a:hover {
  color: #b3c9de;
}

/* ==================== 焦点样式 ==================== */
:focus-visible {
  outline: 2px solid #9cb4cd;
  outline-offset: 2px;
}

/* ==================== 动画 ==================== */
@keyframes fadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
}

@keyframes slideUp {
  from {
    opacity: 0;
    transform: translateY(16px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.7; }
}

/* glow 已废弃：仅保留透明度脉冲（状态点用） */
@keyframes glow {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.55; }
}

/* ==================== 响应式 ==================== */
@media (max-width: 768px) {
  html {
    font-size: 14px;
  }
}
</style>
