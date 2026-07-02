<template>
  <div class="stats-row">
    <div class="stat-card">
      <div class="stat-value">{{ stats.total }}</div>
      <div class="stat-label">总任务数</div>
    </div>
    <div class="stat-card">
      <div class="stat-value success">{{ stats.success }}</div>
      <div class="stat-label">已完成</div>
    </div>
    <div class="stat-card">
      <div class="stat-value warning">{{ stats.running }}</div>
      <div class="stat-label">执行中</div>
    </div>
    <div class="stat-card">
      <div class="stat-value danger">{{ stats.failed }}</div>
      <div class="stat-label">失败</div>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{ stats: { total: number; success: number; running: number; failed: number } }>()
</script>

<style scoped>
.stats-row {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  margin-bottom: 24px;
}

.stat-card {
  padding: 20px;
  background: linear-gradient(135deg, #22d3ee 0%, #14b8a6 100%);
  border-radius: 12px;
  color: #0f0f12;
  text-align: center;
}

.stat-card:nth-child(2) {
  background: linear-gradient(135deg, #34d399 0%, #14b8a6 100%);
}

.stat-card:nth-child(3) {
  background: linear-gradient(135deg, #fbbf24 0%, #f59e0b 100%);
}

.stat-card:nth-child(4) {
  background: linear-gradient(135deg, #f87171 0%, #ef4444 100%);
}

.stat-value {
  font-size: 32px;
  font-weight: 700;
  margin-bottom: 8px;
}

.stat-label {
  font-size: 14px;
  opacity: 0.9;
}
</style>
