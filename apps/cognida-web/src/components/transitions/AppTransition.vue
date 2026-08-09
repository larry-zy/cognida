<template>
  <router-view v-slot="{ Component }">
    <!-- mode="out-in"：旧页面完全离场后新页面再进场。
         页面容器多为 100vh 的普通流布局，若新旧同时渲染会在文档流中
         上下叠放，表现为切页瞬间内容"覆盖/重影"。 -->
    <transition
      :name="transitionName"
      mode="out-in"
      @before-enter="handleBeforeEnter"
      @after-enter="clearTransitioning"
      @enter-cancelled="clearTransitioning"
      @after-leave="clearTransitioning"
      @leave-cancelled="clearTransitioning"
    >
      <component :is="Component" :key="route.path" />
    </transition>
  </router-view>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const route = useRoute()
const router = useRouter()
const transitionName = ref('fade')

// 根据路由深度决定过渡方向
function getRouteDepth(path: string): number {
  return path.split('/').filter(Boolean).length
}

router.afterEach((to, from) => {
  const toDepth = getRouteDepth(to.path)
  const fromDepth = getRouteDepth(from.path)

  if (toDepth > fromDepth) {
    transitionName.value = 'slide-left'
  } else if (toDepth < fromDepth) {
    transitionName.value = 'slide-right'
  } else {
    transitionName.value = 'fade'
  }
  // 兜底：无论上一次过渡是否被打断，进入新路由前先清掉全局锁定类，
  // 避免 page-transitioning 卡死后 pointer-events:none 覆盖所有页面。
  clearTransitioning()
})

function handleBeforeEnter() {
  document.body.classList.add('page-transitioning')
}

function clearTransitioning() {
  document.body.classList.remove('page-transitioning')
}
</script>

<style>
/* Fade 过渡 */
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

/* Slide Left 过渡（前进） */
.slide-left-enter-active,
.slide-left-leave-active {
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.slide-left-enter-from {
  opacity: 0;
  transform: translateX(30px);
}

.slide-left-leave-to {
  opacity: 0;
  transform: translateX(-30px);
}

/* Slide Right 过渡（后退） */
.slide-right-enter-active,
.slide-right-leave-active {
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.slide-right-enter-from {
  opacity: 0;
  transform: translateX(-30px);
}

.slide-right-leave-to {
  opacity: 0;
  transform: translateX(30px);
}

/* Scale 过渡 */
.scale-enter-active,
.scale-leave-active {
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
}

.scale-enter-from,
.scale-leave-to {
  opacity: 0;
  transform: scale(0.95);
}

/* 页面过渡时的样式 */
.page-transitioning {
  overflow: hidden;
  pointer-events: none;
}

.page-transitioning * {
  pointer-events: none !important;
}
</style>
