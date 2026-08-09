<template>
  <component :is="tag" :class="textClasses" :style="textStyle">
    <slot />
  </component>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  tag?: keyof HTMLElementTagNameMap
  variant?: 'primary' | 'secondary' | 'success' | 'warning' | 'danger' | 'custom'
  from?: string
  to?: string
  angle?: number
  animate?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  tag: 'span',
  variant: 'primary',
  angle: 135,
  animate: false
})

const textClasses = computed(() => [
  'ui-gradient-text',
  `ui-gradient-text--${props.variant}`,
  { 'ui-gradient-text--animate': props.animate }
])

const textStyle = computed(() => {
  if (props.variant === 'custom' && props.from && props.to) {
    return {
      '--gradient-from': props.from,
      '--gradient-to': props.to,
      '--gradient-angle': `${props.angle}deg`
    }
  }

  const gradients: Record<string, { from: string; to: string }> = {
    primary: { from: '#22d3ee', to: '#14b8a6' },
    secondary: { from: '#818cf8', to: '#6366f1' },
    success: { from: '#10b981', to: '#059669' },
    warning: { from: '#f59e0b', to: '#d97706' },
    danger: { from: '#ef4444', to: '#dc2626' }
  }

  const gradient = props.variant !== 'custom' ? (gradients[props.variant] || gradients.primary) : gradients.primary
  return {
    '--gradient-from': gradient.from,
    '--gradient-to': gradient.to,
    '--gradient-angle': `${props.angle}deg`
  }
})
</script>

<style scoped>
.ui-gradient-text {
  background: linear-gradient(
    var(--gradient-angle, 135deg),
    var(--gradient-from, #22d3ee),
    var(--gradient-to, #14b8a6)
  );
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
  display: inline;
}

.ui-gradient-text--animate {
  background-size: 200% 200%;
  animation: gradientShift 3s ease infinite;
}

@keyframes gradientShift {
  0%, 100% {
    background-position: 0% 50%;
  }
  50% {
    background-position: 100% 50%;
  }
}
</style>
