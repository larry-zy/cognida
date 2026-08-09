<template>
  <div :class="borderClasses" :style="borderStyle">
    <div class="ui-animated-border__content">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  color?: string
  variant?: 'gradient' | 'solid' | 'glow'
  speed?: 'slow' | 'medium' | 'fast'
  radius?: string
}

const props = withDefaults(defineProps<Props>(), {
  color: '#22d3ee',
  variant: 'gradient',
  speed: 'medium',
  radius: 'var(--radius-lg)'
})

const borderClasses = computed(() => [
  'ui-animated-border',
  `ui-animated-border--${props.variant}`,
  `ui-animated-border--${props.speed}`
])

const borderStyle = computed(() => ({
  '--border-color': props.color,
  '--border-radius': props.radius
}))
</script>

<style scoped>
.ui-animated-border {
  position: relative;
  border-radius: var(--border-radius);
  padding: 2px;
  overflow: hidden;
}

.ui-animated-border__content {
  position: relative;
  z-index: 1;
  border-radius: calc(var(--border-radius) - 2px);
  background: var(--bg-secondary);
  padding: var(--space-4);
}

/* Gradient variant */
.ui-animated-border--gradient::before {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: var(--border-radius);
  padding: 2px;
  background: linear-gradient(
    90deg,
    transparent,
    var(--border-color),
    transparent
  );
  background-size: 200% 100%;
  -webkit-mask: linear-gradient(#fff 0 0) content-box, linear-gradient(#fff 0 0);
  -webkit-mask-composite: xor;
  mask-composite: exclude;
  animation: borderMove 3s linear infinite;
}

.ui-animated-border--gradient.ui-animated-border--slow::before {
  animation-duration: 4s;
}

.ui-animated-border--gradient.ui-animated-border--fast::before {
  animation-duration: 2s;
}

@keyframes borderMove {
  0% {
    background-position: 200% 0;
  }
  100% {
    background-position: -200% 0;
  }
}

/* Solid variant */
.ui-animated-border--solid::before {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: var(--border-radius);
  padding: 2px;
  background: var(--border-color);
  -webkit-mask: linear-gradient(#fff 0 0) content-box, linear-gradient(#fff 0 0);
  -webkit-mask-composite: xor;
  mask-composite: exclude;
  opacity: 0.5;
  transition: opacity var(--duration-base);
}

.ui-animated-border--solid:hover::before {
  opacity: 1;
}

/* Glow variant */
.ui-animated-border--glow::before {
  content: '';
  position: absolute;
  inset: -2px;
  border-radius: var(--border-radius);
  background: var(--border-color);
  filter: blur(8px);
  opacity: 0;
  transition: opacity var(--duration-base);
  z-index: 0;
}

.ui-animated-border--glow:hover::before {
  opacity: 0.5;
}

.ui-animated-border--glow::after {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: var(--border-radius);
  padding: 2px;
  background: var(--border-color);
  -webkit-mask: linear-gradient(#fff 0 0) content-box, linear-gradient(#fff 0 0);
  -webkit-mask-composite: xor;
  mask-composite: exclude;
  opacity: 0.5;
  transition: opacity var(--duration-base);
}

.ui-animated-border--glow:hover::after {
  opacity: 1;
}
</style>
