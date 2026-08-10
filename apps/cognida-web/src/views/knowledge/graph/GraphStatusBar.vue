<!-- 图谱底部状态栏 —— 拆分自 GraphView.vue〔M14〕
     职责：展示节点/关系/实体类型计数与图谱统计（平均度/最大度/孤立节点/连通分量）及最后同步时间。
     纯展示组件，数据由容器传入。仅在非嵌入模式下由容器渲染。 -->
<template>
  <div class="gv-statusbar">
    <div class="gv-statusbar__left">
      <span class="gv-stat-item">节点 <span class="gv-stat-val">{{ nodeCount }}</span></span>
      <span class="gv-stat-item">关系 <span class="gv-stat-val">{{ edgeCount }}</span></span>
      <span class="gv-stat-item">实体类型 <span class="gv-stat-val">{{ entityTypeCount }}</span></span>
      <span class="gv-stat-item">平均度 <span class="gv-stat-val">{{ stats ? stats.avg_degree.toFixed(1) : '—' }}</span></span>
      <span class="gv-stat-item">最大度 <span class="gv-stat-val">{{ stats ? stats.max_degree : '—' }}</span></span>
      <span class="gv-stat-item">孤立节点 <span class="gv-stat-val">{{ stats ? stats.isolated_nodes : '—' }}</span></span>
      <span class="gv-stat-item">连通分量 <span class="gv-stat-val">{{ stats ? stats.component_count : '—' }}</span></span>
    </div>
    <div class="gv-statusbar__right">
      <span class="gv-status-dot"><i></i></span>
      最后同步 <span class="gv-stat-val">{{ lastSyncTime }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { GraphStatsResponse } from '@/types'

defineProps<{
  nodeCount: number
  edgeCount: number
  entityTypeCount: number
  stats: GraphStatsResponse | null
  lastSyncTime: string
}>()
</script>

<style scoped>
/* ===== 底部状态栏 ===== */
.gv-statusbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
  height: 32px;
  flex: 0 0 32px;
  /* mockup 使用 var(--hair) = --border-default (9% opacity) */
  border-top: 1px solid var(--color-border-default);
}

.gv-statusbar__left {
  display: flex;
  align-items: center;
  gap: 18px;
  font-family: var(--font-mono);
  font-size: 10.5px;
  color: var(--color-text-muted);
}

.gv-stat-item {
  display: flex;
  align-items: center;
  gap: 5px;
}

.gv-stat-val {
  color: var(--color-text-secondary);
  font-family: var(--font-mono);
}

.gv-statusbar__right {
  display: flex;
  align-items: center;
  gap: 6px;
  font-family: var(--font-mono);
  font-size: 10.5px;
  color: var(--color-text-muted);
}

/* 呼吸状态点 */
.gv-status-dot {
  display: flex;
  align-items: center;
}

.gv-status-dot i {
  display: block;
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--color-success);
  animation: pulse 3s ease-in-out infinite;
}

/* 呼吸脉冲动画（本地定义，确保 scoped 作用域可用） */
@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}
</style>
