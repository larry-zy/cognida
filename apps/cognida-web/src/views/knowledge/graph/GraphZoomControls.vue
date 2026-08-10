<!-- 图谱缩放控件（右下角） —— 拆分自 GraphView.vue〔M14〕
     职责：放大/缩小/适应屏幕。操作以 emits 上抛容器（容器持有 vis-network 实例）。 -->
<template>
  <div class="gv-zoom">
    <button class="gv-icon-btn" title="放大" @click="$emit('zoom-in')">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="11" cy="11" r="7"/><path d="M21 21l-4.3-4.3"/>
        <path d="M8 11h6M11 8v6"/>
      </svg>
    </button>
    <button class="gv-icon-btn" title="缩小" @click="$emit('zoom-out')">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="11" cy="11" r="7"/><path d="M21 21l-4.3-4.3"/>
        <path d="M8 11h6"/>
      </svg>
    </button>
    <button class="gv-icon-btn" title="适应屏幕" @click="$emit('fit')">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M8 3H5a2 2 0 0 0-2 2v3M21 8V5a2 2 0 0 0-2-2h-3M3 16v3a2 2 0 0 0 2 2h3M16 21h3a2 2 0 0 0 2-2v-3"/>
      </svg>
    </button>
  </div>
</template>

<script setup lang="ts">
defineEmits<{
  (e: 'zoom-in'): void
  (e: 'zoom-out'): void
  (e: 'fit'): void
}>()
</script>

<style scoped>
/* 缩放控件（右下角）：mockup 无 shadow，纯 --ink-2 底色 */
.gv-zoom {
  position: absolute;
  right: 16px;
  bottom: 16px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  background: var(--color-bg-tertiary);
  border-radius: var(--radius-sm);
  padding: 4px;
}

/* 图标按钮（基类，供缩放按钮复用） */
.gv-icon-btn {
  width: 30px;
  height: 30px;
  border-radius: var(--radius-sm);
  border: none;
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition:
    color var(--duration-fast) var(--ease-default),
    background var(--duration-fast) var(--ease-default);
}

.gv-icon-btn svg {
  width: 15px;
  height: 15px;
}

.gv-icon-btn:hover {
  color: var(--color-text-primary);
  background: var(--color-bg-elevated);
}

.gv-zoom .gv-icon-btn {
  width: 28px;
  height: 28px;
}

.gv-zoom .gv-icon-btn svg {
  width: 13px;
  height: 13px;
}
</style>
