<!-- 图谱图例浮层（左下角） —— 拆分自 GraphView.vue〔M14〕
     职责：展示实体类型 → 颜色的图例。纯展示组件，由容器传入 items。 -->
<template>
  <div class="gv-legend">
    <div class="gv-legend__title">ENTITY TYPE</div>
    <div
      v-for="item in items"
      :key="item.type"
      class="gv-legend__row"
    >
      <span class="gv-legend__dot" :style="{ background: item.color }"></span>
      <span>{{ item.label }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  items: { type: string; label: string; color: string }[]
}>()
</script>

<style scoped>
/* 图例浮层（左下角）：mockup 无 shadow，纯 --ink-2 底色 */
.gv-legend {
  position: absolute;
  left: 16px;
  bottom: 16px;
  background: var(--color-bg-tertiary);
  border-radius: var(--radius-sm);
  padding: 10px 13px;
  min-width: 130px;
}

.gv-legend__title {
  font-family: var(--font-mono);
  font-size: 9.5px;
  letter-spacing: 1.2px;
  color: var(--color-text-muted);
  margin-bottom: 8px;
}

.gv-legend__row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 0;
  font-size: 11.5px;
  color: var(--color-text-secondary);
}

.gv-legend__row + .gv-legend__row {
  border-top: 1px solid var(--color-border-subtle);
}

.gv-legend__dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
  flex: 0 0 9px;
}
</style>
