<template>
  <div :class="loaderClasses">
    <!-- Spinner -->
    <template v-if="variant === 'spinner'">
      <div class="ui-loader__spinner">
        <svg viewBox="0 0 50 50">
          <circle
            cx="25"
            cy="25"
            r="20"
            fill="none"
            :stroke="color"
            stroke-width="4"
            stroke-linecap="round"
            stroke-dasharray="80"
            stroke-dashoffset="60"
          />
        </svg>
      </div>
    </template>

    <!-- Dots -->
    <template v-else-if="variant === 'dots'">
      <div class="ui-loader__dots">
        <span class="ui-loader__dot"></span>
        <span class="ui-loader__dot"></span>
        <span class="ui-loader__dot"></span>
      </div>
    </template>

    <!-- Bar -->
    <template v-else-if="variant === 'bar'">
      <div class="ui-loader__bar">
        <div class="ui-loader__bar-fill"></div>
      </div>
    </template>

    <!-- Pulse -->
    <template v-else-if="variant === 'pulse'">
      <div class="ui-loader__pulse"></div>
    </template>

    <!-- Text -->
    <span v-if="text" class="ui-loader__text">{{ text }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  variant?: 'spinner' | 'dots' | 'bar' | 'pulse'
  size?: 'sm' | 'md' | 'lg'
  color?: string
  text?: string
  fullscreen?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'spinner',
  size: 'md',
  color: 'var(--primary)',
  fullscreen: false
})

const loaderClasses = computed(() => [
  'ui-loader',
  `ui-loader--${props.variant}`,
  `ui-loader--${props.size}`,
  { 'ui-loader--fullscreen': props.fullscreen }
])
</script>

<style scoped>
.ui-loader {
  display: inline-flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--space-4);
}

.ui-loader--fullscreen {
  position: fixed;
  inset: 0;
  z-index: 2000;
  background: rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(4px);
}

/* Spinner */
.ui-loader__spinner {
  display: flex;
  align-items: center;
  justify-content: center;
}

.ui-loader--sm .ui-loader__spinner {
  width: 20px;
  height: 20px;
}

.ui-loader--md .ui-loader__spinner {
  width: 32px;
  height: 32px;
}

.ui-loader--lg .ui-loader__spinner {
  width: 48px;
  height: 48px;
}

.ui-loader__spinner svg {
  width: 100%;
  height: 100%;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

/* Dots */
.ui-loader__dots {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.ui-loader__dot {
  width: 8px;
  height: 8px;
  background: v-bind(color);
  border-radius: 50%;
  animation: dotBounce 1.4s ease-in-out infinite both;
}

.ui-loader--sm .ui-loader__dot {
  width: 6px;
  height: 6px;
}

.ui-loader--lg .ui-loader__dot {
  width: 12px;
  height: 12px;
}

.ui-loader__dot:nth-child(1) {
  animation-delay: -0.32s;
}

.ui-loader__dot:nth-child(2) {
  animation-delay: -0.16s;
}

.ui-loader__dot:nth-child(3) {
  animation-delay: 0s;
}

@keyframes dotBounce {
  0%, 80%, 100% {
    transform: scale(0.6);
    opacity: 0.5;
  }
  40% {
    transform: scale(1);
    opacity: 1;
  }
}

/* Bar */
.ui-loader__bar {
  width: 200px;
  height: 4px;
  background: var(--bg-tertiary);
  border-radius: var(--radius-full);
  overflow: hidden;
}

.ui-loader--sm .ui-loader__bar {
  width: 120px;
  height: 3px;
}

.ui-loader--lg .ui-loader__bar {
  width: 280px;
  height: 6px;
}

.ui-loader__bar-fill {
  height: 100%;
  background: v-bind(color);
  border-radius: var(--radius-full);
  animation: barShrink 1.5s ease-in-out infinite;
}

@keyframes barShrink {
  0% {
    width: 100%;
    transform: translateX(0);
  }
  50% {
    width: 40%;
    transform: translateX(60%);
  }
  100% {
    width: 100%;
    transform: translateX(100%);
  }
}

/* Pulse */
.ui-loader__pulse {
  width: 40px;
  height: 40px;
  background: v-bind(color);
  border-radius: 50%;
  animation: pulse 1.5s ease-in-out infinite;
}

.ui-loader--sm .ui-loader__pulse {
  width: 24px;
  height: 24px;
}

.ui-loader--lg .ui-loader__pulse {
  width: 56px;
  height: 56px;
}

@keyframes pulse {
  0% {
    transform: scale(0.6);
    opacity: 0.6;
  }
  50% {
    transform: scale(1);
    opacity: 1;
  }
  100% {
    transform: scale(0.6);
    opacity: 0.6;
  }
}

/* Text */
.ui-loader__text {
  font-size: var(--text-sm);
  color: var(--text-secondary);
}
</style>
