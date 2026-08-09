<template>
  <component :is="iconComponent" :class="iconClasses" v-bind="$attrs" />
</template>

<script setup lang="ts">
import { computed, h } from 'vue'
import * as icons from './icons'

interface Props {
  name: string
  size?: string | number
  spin?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  size: 16
})

const iconClasses = computed(() => [
  'ui-icon',
  { 'ui-icon--spin': props.spin }
])

const iconComponent = computed(() => {
  const icon = (icons as any)[props.name]
  if (!icon) return null

  return () => h(icon, {
    width: props.size,
    height: props.size,
    class: props.spin ? 'ui-icon--spin' : ''
  })
})
</script>

<style scoped>
.ui-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.ui-icon--spin {
  animation: iconSpin 1s linear infinite;
}

@keyframes iconSpin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}
</style>
