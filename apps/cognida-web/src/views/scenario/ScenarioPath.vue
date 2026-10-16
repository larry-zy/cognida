<template>
  <nav class="scenario-path" aria-label="相关能力（可独立使用）">
    <span class="scenario-path-label">按需使用</span>
    <RouterLink v-for="step in steps" :key="step.path" :to="step.path" :aria-current="current === step.path ? 'page' : undefined">
      {{ step.title }}
    </RouterLink>
  </nav>
</template>
<script setup lang="ts">
defineProps<{ current: string }>()
const steps = [
  { path: '/hf-datasets', title: '采集公开数据' },
  { path: '/data-preparation', title: '清洗与标准化' },
  { path: '/label-standards', title: '发布标注标准' },
  { path: '/labeling', title: '标注与审核' },
  { path: '/sft-datasets', title: '构建 SFT 数据集' },
]
</script>
<style scoped>
.scenario-path { display:flex; flex-wrap:wrap; gap:8px; margin:0 0 22px }
.scenario-path a { display:flex; align-items:center; gap:8px; padding:9px 12px; border:1px solid var(--border-default); border-radius:6px; color:var(--text-secondary); text-decoration:none; font-size:12px }
.scenario-path a[aria-current] { border-color:var(--primary); color:var(--text-primary); background:var(--primary-dim) }
.scenario-path-label { align-self:center; color:var(--text-muted); font-size:12px }
</style>
