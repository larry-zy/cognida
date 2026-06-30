<template>
  <div class="agent-list-container">
    <div class="header">
      <h2>Agent 管理</h2>
      <el-button type="primary" :icon="Plus">创建 Agent</el-button>
    </div>

    <el-empty description="暂无 Agent" />
  </div>
</template>

<script setup lang="ts">
import { Plus } from '@element-plus/icons-vue'
</script>

<style scoped>
.agent-list-container {
  padding: 24px;
  background: rgba(0, 0, 0, 0.2);
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: var(--radius-lg);
}

.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.header h2 {
  font-size: 20px;
  font-weight: 600;
  color: var(--color-text-primary);
}
</style>
