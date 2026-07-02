<template>
  <div class="agent-list-container">
    <div class="header">
      <h2>Agent 管理</h2>
      <UiButton variant="primary" icon>
        <template #icon><el-icon><Plus /></el-icon></template>
        创建 Agent
      </UiButton>
    </div>

    <UiEmpty description="暂无 Agent" />
  </div>
</template>

<script setup lang="ts">
import { Plus } from '@element-plus/icons-vue'
import { UiButton, UiEmpty } from '@/components/ui'
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
