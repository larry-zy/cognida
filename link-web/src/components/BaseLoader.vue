<template>
  <div class="base-loader" :class="classes" :style="loaderStyle">
    <!-- 圆形加载器 -->
    <template v-if="type === 'spinner' || type === 'circular'">
      <svg class="loader-spinner" viewBox="0 0 50 50">
        <circle
          class="spinner-path"
          cx="25"
          cy="25"
          r="20"
          fill="none"
          :stroke="color"
          :stroke-width="strokeWidth"
          stroke-linecap="round"
          stroke-dasharray="90, 150"
          stroke-dashoffset="0"
        />
      </svg>
    </template>

    <!-- 点状加载器 -->
    <template v-else-if="type === 'dots' || type === 'pulse'">
      <div class="loader-dots">
        <span class="dot" :style="{ background: color }"></span>
        <span class="dot" :style="{ background: color }"></span>
        <span class="dot" :style="{ background: color }"></span>
      </div>
    </template>

    <!-- 进度条加载器 -->
    <template v-else-if="type === 'bar' || type === 'progress'">
      <div class="loader-bar-container">
        <div
          class="loader-bar"
          :style="{
            width: progress + '%',
            background: color
          }"
        ></div>
      </div>
    </template>

    <!-- 波浪加载器 -->
    <template v-else-if="type === 'wave'">
      <div class="loader-wave">
        <span class="wave-bar" :style="{ background: color }"></span>
        <span class="wave-bar" :style="{ background: color }"></span>
        <span class="wave-bar" :style="{ background: color }"></span>
        <span class="wave-bar" :style="{ background: color }"></span>
        <span class="wave-bar" :style="{ background: color }"></span>
      </div>
    </template>

    <!-- 文字加载器 -->
    <template v-else-if="type === 'text'">
      <div class="loader-text" :style="{ color }">
        <span class="text-char">L</span>
        <span class="text-char">o</span>
        <span class="text-char">a</span>
        <span class="text-char">d</span>
        <span class="text-char">i</span>
        <span class="text-char">n</span>
        <span class="text-char">g</span>
      </div>
    </template>

    <!-- 加载文字 -->
    <span v-if="text" class="loader-text-label">{{ text }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  type?: 'spinner' | 'circular' | 'dots' | 'pulse' | 'bar' | 'progress' | 'wave' | 'text'
  size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl' | number
  color?: string
  text?: string
  progress?: number
  strokeWidth?: number
}

const props = withDefaults(defineProps<Props>(), {
  type: 'spinner',
  size: 'md',
  color: '#22d3ee',
  progress: 0,
  strokeWidth: 4
})

const classes = computed(() => [
  `type-${props.type}`,
  `size-${typeof props.size === 'number' ? 'custom' : props.size}`
])

const loaderStyle = computed(() => {
  if (typeof props.size === 'number') {
    return {
      '--loader-size': props.size + 'px'
    }
  }
  return {}
})
</script>

<style scoped>
/* ==================== 基础样式 ==================== */
.base-loader {
  display: inline-flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
}

/* ==================== 尺寸变体 ==================== */
.size-xs {
  --loader-size: 16px;
}

.size-sm {
  --loader-size: 24px;
}

.size-md {
  --loader-size: 32px;
}

.size-lg {
  --loader-size: 48px;
}

.size-xl {
  --loader-size: 64px;
}

.size-custom {
  --loader-size: var(--loader-size);
}

/* ==================== Spinner/Circular ==================== */
.loader-spinner {
  width: var(--loader-size);
  height: var(--loader-size);
  animation: rotate 2s linear infinite;
}

.spinner-path {
  animation: dash 1.5s ease-in-out infinite;
}

@keyframes rotate {
  100% {
    transform: rotate(360deg);
  }
}

@keyframes dash {
  0% {
    stroke-dasharray: 1, 150;
    stroke-dashoffset: 0;
  }
  50% {
    stroke-dasharray: 90, 150;
    stroke-dashoffset: -35;
  }
  100% {
    stroke-dasharray: 90, 150;
    stroke-dashoffset: -124;
  }
}

/* ==================== Dots/Pulse ==================== */
.loader-dots {
  display: flex;
  gap: calc(var(--loader-size) / 4);
}

.dot {
  width: calc(var(--loader-size) / 3);
  height: calc(var(--loader-size) / 3);
  border-radius: 50%;
  animation: pulseDots 1.4s ease-in-out infinite;
}

.dot:nth-child(1) {
  animation-delay: -0.32s;
}

.dot:nth-child(2) {
  animation-delay: -0.16s;
}

@keyframes pulseDots {
  0%, 80%, 100% {
    transform: scale(0.5);
    opacity: 0.5;
  }
  40% {
    transform: scale(1);
    opacity: 1;
  }
}

/* ==================== Bar/Progress ==================== */
.loader-bar-container {
  width: calc(var(--loader-size) * 5);
  height: calc(var(--loader-size) / 4);
  background: rgba(255, 255, 255, 0.1);
  border-radius: 9999px;
  overflow: hidden;
}

.loader-bar {
  height: 100%;
  border-radius: 9999px;
  transition: width 0.3s ease;
  animation: shimmer 1.5s infinite;
}

@keyframes shimmer {
  0% {
    background-position: -200% 0;
  }
  100% {
    background-position: 200% 0;
  }
}

/* ==================== Wave ==================== */
.loader-wave {
  display: flex;
  align-items: flex-end;
  gap: calc(var(--loader-size) / 10);
  height: var(--loader-size);
}

.wave-bar {
  width: calc(var(--loader-size) / 8);
  background: #22d3ee;
  border-radius: calc(var(--loader-size) / 16);
  animation: waveAnimation 1.2s ease-in-out infinite;
}

.wave-bar:nth-child(1) { animation-delay: -1.1s; }
.wave-bar:nth-child(2) { animation-delay: -1s; }
.wave-bar:nth-child(3) { animation-delay: -0.9s; }
.wave-bar:nth-child(4) { animation-delay: -0.8s; }
.wave-bar:nth-child(5) { animation-delay: -0.7s; }

@keyframes waveAnimation {
  0%, 100% {
    height: 20%;
  }
  50% {
    height: 100%;
  }
}

/* ==================== Text ==================== */
.loader-text {
  display: flex;
  font-size: calc(var(--loader-size) * 0.5);
  font-weight: 600;
  letter-spacing: 2px;
}

.text-char {
  animation: textBounce 1.4s ease-in-out infinite;
}

.text-char:nth-child(1) { animation-delay: -1.3s; }
.text-char:nth-child(2) { animation-delay: -1.2s; }
.text-char:nth-child(3) { animation-delay: -1.1s; }
.text-char:nth-child(4) { animation-delay: -1s; }
.text-char:nth-child(5) { animation-delay: -0.9s; }
.text-char:nth-child(6) { animation-delay: -0.8s; }
.text-char:nth-child(7) { animation-delay: -0.7s; }

@keyframes textBounce {
  0%, 80%, 100% {
    transform: translateY(0);
  }
  40% {
    transform: translateY(-10px);
  }
}

/* ==================== 文字标签 ==================== */
.loader-text-label {
  font-size: 0.875rem;
  color: #606070;
}

/* ==================== 遮罩层样式 ==================== */
.base-loader.is-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(4px);
  z-index: 9999;
}
</style>
