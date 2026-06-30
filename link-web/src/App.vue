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
  font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
  color: var(--color-text-primary, #f0f0f5);
  background: var(--color-bg-primary, #0f0f12);
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
  background: #0f0f12;
  z-index: 0;
}

.bg-image {
  position: absolute;
  inset: 0;
  background-image: url('/img/backgroud.png');
  background-size: cover;
  background-position: center;
  background-repeat: no-repeat;
  opacity: 0.4;
  z-index: 1;
}

.bg-overlay {
  position: absolute;
  inset: 0;
  background:
    radial-gradient(ellipse at 20% 20%, rgba(34, 211, 238, 0.04) 0%, transparent 50%),
    radial-gradient(ellipse at 80% 80%, rgba(129, 140, 248, 0.03) 0%, transparent 50%);
  z-index: 2;
}

.bg-noise {
  position: absolute;
  inset: 0;
  opacity: 0.02;
  background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 256 256' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='noise'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23noise)'/%3E%3C/svg%3E");
  z-index: 3;
}

.bg-overlay {
  position: absolute;
  inset: 0;
  background:
    radial-gradient(ellipse at 20% 20%, rgba(34, 211, 238, 0.04) 0%, transparent 50%),
    radial-gradient(ellipse at 80% 80%, rgba(129, 140, 248, 0.03) 0%, transparent 50%);
}

.bg-noise {
  position: absolute;
  inset: 0;
  opacity: 0.02;
  background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 256 256' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='noise'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23noise)'/%3E%3C/svg%3E");
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
  background: rgba(34, 211, 238, 0.3);
  color: #f0f0f5;
}

::-moz-selection {
  background: rgba(34, 211, 238, 0.3);
  color: #f0f0f5;
}

/* ==================== 链接样式 ==================== */
a {
  color: #22d3ee;
  text-decoration: none;
  transition: color 150ms ease;
}

a:hover {
  color: #67e8f9;
}

/* ==================== 焦点样式 ==================== */
:focus-visible {
  outline: 2px solid #22d3ee;
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

@keyframes glow {
  0%, 100% {
    box-shadow: 0 0 5px rgba(34, 211, 238, 0.3), 0 0 10px rgba(34, 211, 238, 0.2);
  }
  50% {
    box-shadow: 0 0 15px rgba(34, 211, 238, 0.4), 0 0 25px rgba(34, 211, 238, 0.3);
  }
}

/* ==================== 响应式 ==================== */
@media (max-width: 768px) {
  html {
    font-size: 14px;
  }
}
</style>
