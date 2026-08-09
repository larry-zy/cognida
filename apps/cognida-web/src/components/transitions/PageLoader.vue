<template>
  <Transition name="loader-fade">
    <div v-if="loading" class="page-loader">
      <div class="page-loader__content">
        <div class="page-loader__spinner">
          <div class="spinner-ring"></div>
          <div class="spinner-ring"></div>
          <div class="spinner-ring"></div>
          <div class="spinner-ring"></div>
        </div>
        <p v-if="text" class="page-loader__text">{{ text }}</p>
      </div>
    </div>
  </Transition>
</template>

<script setup lang="ts">
interface Props {
  loading: boolean
  text?: string
}

withDefaults(defineProps<Props>(), {
  text: ''
})
</script>

<style scoped>
.page-loader {
  position: fixed;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--bg-primary);
  z-index: 9999;
}

.page-loader__content {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--space-6);
}

.page-loader__spinner {
  position: relative;
  width: 60px;
  height: 60px;
}

.spinner-ring {
  position: absolute;
  width: 100%;
  height: 100%;
  border: 3px solid transparent;
  border-top-color: var(--primary);
  border-radius: 50%;
  animation: spin 1.5s cubic-bezier(0.68, -0.55, 0.265, 1.55) infinite;
}

.spinner-ring:nth-child(1) {
  animation-delay: -0.45s;
}

.spinner-ring:nth-child(2) {
  animation-delay: -0.3s;
}

.spinner-ring:nth-child(3) {
  animation-delay: -0.15s;
}

@keyframes spin {
  0% {
    transform: rotate(0deg) scale(1);
  }
  50% {
    transform: rotate(180deg) scale(0.6);
  }
  100% {
    transform: rotate(360deg) scale(1);
  }
}

.page-loader__text {
  font-size: var(--text-sm);
  color: var(--text-secondary);
  margin: 0;
}

.loader-fade-enter-active,
.loader-fade-leave-active {
  transition: opacity 0.3s ease;
}

.loader-fade-enter-from,
.loader-fade-leave-to {
  opacity: 0;
}

.loader-fade-leave-to {
  opacity: 0;
}
</style>
