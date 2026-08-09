<template>
  <component
    :is="tag"
    :class="buttonClasses"
    :disabled="disabled || loading"
    :type="nativeType"
    v-bind="$attrs"
  >
    <span v-if="loading" class="ui-button__loader">
      <svg class="ui-button__spinner" viewBox="0 0 24 24">
        <circle
          cx="12"
          cy="12"
          r="10"
          fill="none"
          stroke="currentColor"
          stroke-width="3"
          stroke-linecap="round"
          stroke-dasharray="60"
          stroke-dashoffset="40"
        />
      </svg>
    </span>
    <span v-if="icon && $slots.icon" class="ui-button__icon">
      <slot name="icon" />
    </span>
    <span v-if="$slots.default" class="ui-button__content">
      <slot />
    </span>
  </component>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  variant?: 'primary' | 'secondary' | 'ghost' | 'danger'
  size?: 'sm' | 'md' | 'lg' | 'xl'
  tag?: 'button' | 'a' | 'router-link'
  nativeType?: 'button' | 'submit' | 'reset'
  disabled?: boolean
  loading?: boolean
  icon?: boolean
  block?: boolean
  squared?: boolean
  circle?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'primary',
  size: 'md',
  tag: 'button',
  nativeType: 'button',
  disabled: false,
  loading: false,
  icon: false,
  block: false,
  squared: false,
  circle: false
})

const buttonClasses = computed(() => [
  'ui-button',
  `ui-button--${props.variant}`,
  `ui-button--${props.size}`,
  {
    'ui-button--disabled': props.disabled,
    'ui-button--loading': props.loading,
    'ui-button--block': props.block,
    'ui-button--squared': props.squared,
    'ui-button--circle': props.circle,
    'ui-button--has-icon': props.icon || props.loading
  }
])
</script>

<style scoped>
.ui-button {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--space-2);
  font-family: var(--font-body);
  font-weight: 600;
  line-height: 1;
  white-space: nowrap;
  cursor: pointer;
  user-select: none;
  text-decoration: none;
  border: none;
  outline: none;
  transition: all var(--duration-fast) ease;
}

.ui-button:focus-visible {
  outline: 2px solid var(--primary);
  outline-offset: 2px;
}

/* ========== Sizes ========== */
.ui-button--sm {
  padding: var(--space-2) var(--space-3);
  font-size: var(--text-sm);
  gap: var(--space-1);
}

.ui-button--sm.ui-button--circle {
  width: 32px;
  height: 32px;
  padding: 0;
}

.ui-button--md {
  padding: var(--space-2) var(--space-4);
  font-size: var(--text-base);
}

.ui-button--md.ui-button--circle {
  width: 40px;
  height: 40px;
  padding: 0;
}

.ui-button--lg {
  padding: var(--space-3) var(--space-6);
  font-size: var(--text-lg);
  gap: var(--space-2);
}

.ui-button--lg.ui-button--circle {
  width: 48px;
  height: 48px;
  padding: 0;
}

.ui-button--xl {
  padding: var(--space-4) var(--space-8);
  font-size: var(--text-xl);
  gap: var(--space-3);
}

.ui-button--xl.ui-button--circle {
  width: 56px;
  height: 56px;
  padding: 0;
}

/* ========== Border Radius ========== */
.ui-button--squared {
  border-radius: 0;
}

.ui-button--circle {
  border-radius: 50%;
}

.ui-button:not(.ui-button--squared):not(.ui-button--circle) {
  border-radius: var(--radius-md);
}

/* ========== Variants ========== */
.ui-button--primary {
  background: linear-gradient(135deg, var(--primary), var(--primary-dark));
  color: #000;
}

.ui-button--primary:hover:not(.ui-button--disabled):not(.ui-button--loading) {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(34, 211, 238, 0.3);
}

.ui-button--primary:active:not(.ui-button--disabled):not(.ui-button--loading) {
  transform: translateY(0) scale(0.98);
}

/* Secondary */
.ui-button--secondary {
  background: var(--bg-tertiary);
  color: var(--text-primary);
  border: 1px solid var(--border-default);
}

.ui-button--secondary:hover:not(.ui-button--disabled):not(.ui-button--loading) {
  transform: translateY(-2px);
  background: var(--bg-elevated);
  border-color: var(--border-strong);
}

.ui-button--secondary:active:not(.ui-button--disabled):not(.ui-button--loading) {
  transform: translateY(0) scale(0.98);
}

/* Ghost */
.ui-button--ghost {
  background: transparent;
  color: var(--primary);
  border: 1px solid transparent;
}

.ui-button--ghost:hover:not(.ui-button--disabled):not(.ui-button--loading) {
  background: rgba(34, 211, 238, 0.1);
}

.ui-button--ghost:active:not(.ui-button--disabled):not(.ui-button--loading) {
  transform: scale(0.98);
}

/* Danger */
.ui-button--danger {
  background: linear-gradient(135deg, var(--danger), #dc2626);
  color: white;
}

.ui-button--danger:hover:not(.ui-button--disabled):not(.ui-button--loading) {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(239, 68, 68, 0.3);
}

.ui-button--danger:active:not(.ui-button--disabled):not(.ui-button--loading) {
  transform: translateY(0) scale(0.98);
}

/* ========== States ========== */
.ui-button--disabled,
.ui-button--loading {
  opacity: 0.5;
  cursor: not-allowed;
  pointer-events: none;
}

/* ========== Block ========== */
.ui-button--block {
  display: flex;
  width: 100%;
}

/* ========== Icon ========== */
.ui-button__icon {
  display: flex;
  align-items: center;
}

.ui-button__icon svg {
  width: 1.2em;
  height: 1.2em;
}

/* ========== Loader ========== */
.ui-button__loader {
  display: flex;
  align-items: center;
}

.ui-button__spinner {
  width: 1em;
  height: 1em;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

/* ========== Content ========== */
.ui-button__content {
  display: flex;
  align-items: center;
}

.ui-button--loading .ui-button__content {
  opacity: 0.7;
}

/* ========== Link tag special styles ========== */
.ui-button:where(a, a.ui-button) {
  /* Reset link styles */
}

.ui-button:where(a):hover {
  text-decoration: none;
}
</style>
