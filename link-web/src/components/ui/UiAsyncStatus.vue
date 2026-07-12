<template>
  <transition name="async-status-fade">
    <span
      v-if="status !== 'idle'"
      class="ui-async-status"
      :class="`ui-async-status--${status}`"
      role="status"
      aria-live="polite"
    >
      <!-- pending：旋转图标 -->
      <UiIcon v-if="status === 'pending'" name="RefreshIcon" :size="14" spin class="ui-async-status__icon" />
      <!-- success：对勾 -->
      <UiIcon v-else-if="status === 'success'" name="CheckIcon" :size="14" class="ui-async-status__icon" />
      <!-- error：叉 -->
      <UiIcon v-else name="XIcon" :size="14" class="ui-async-status__icon" />

      <span class="ui-async-status__text">{{ text }}</span>

      <!-- error 态可选重试 -->
      <button
        v-if="status === 'error' && showRetry"
        type="button"
        class="ui-async-status__retry"
        @click="emit('retry')"
      >
        重试
      </button>
    </span>
  </transition>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import UiIcon from '@/components/icons/UiIcon.vue'
import type { AsyncStatus } from '@/composables/useAsyncTask'

interface Props {
  /** 状态机当前状态 */
  status: AsyncStatus
  /** 就地展示文案（一般直接传状态机的 message） */
  message?: string
  /** error 态是否显示「重试」按钮 */
  showRetry?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  message: '',
  showRetry: true,
})

const emit = defineEmits<{ retry: [] }>()

const fallback: Record<AsyncStatus, string> = {
  idle: '',
  pending: '处理中…',
  success: '完成',
  error: '操作失败',
}

const text = computed(() => props.message || fallback[props.status])
</script>

<style scoped>
.ui-async-status {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  padding: 2px var(--space-2);
  font-size: var(--text-xs);
  line-height: 1.4;
  border-radius: var(--radius-full);
  white-space: nowrap;
  vertical-align: middle;
}

.ui-async-status__icon {
  flex-shrink: 0;
}

.ui-async-status__text {
  font-weight: 500;
}

/* pending */
.ui-async-status--pending {
  color: var(--color-primary, #2563eb);
  background: color-mix(in srgb, var(--color-primary, #2563eb) 10%, transparent);
}

/* success */
.ui-async-status--success {
  color: var(--color-success, #16a34a);
  background: color-mix(in srgb, var(--color-success, #16a34a) 12%, transparent);
}

/* error */
.ui-async-status--error {
  color: var(--color-danger, #dc2626);
  background: color-mix(in srgb, var(--color-danger, #dc2626) 12%, transparent);
}

.ui-async-status__retry {
  margin-left: var(--space-1);
  padding: 0 var(--space-1);
  font-size: var(--text-xs);
  font-weight: 600;
  color: inherit;
  background: transparent;
  border: none;
  border-bottom: 1px solid currentColor;
  cursor: pointer;
}

.async-status-fade-enter-active,
.async-status-fade-leave-active {
  transition: opacity var(--duration-fast, 0.15s) ease;
}
.async-status-fade-enter-from,
.async-status-fade-leave-to {
  opacity: 0;
}
</style>
