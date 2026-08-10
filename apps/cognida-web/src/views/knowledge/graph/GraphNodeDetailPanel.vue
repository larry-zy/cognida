<!-- 节点详情抽屉（右侧，宽 300px） —— 拆分自 GraphView.vue〔M14〕
     职责：展示选中节点的基本信息、关联分块入口、关联关系列表及增删改入口。
     所有数据由容器以 props 传入，操作以 emits 上抛（图谱数据与网络实例仍在容器）。 -->
<template>
  <aside class="gv-drawer">
    <div class="gv-drawer__head">
      <span class="gv-drawer__title">节点详情</span>
      <button class="gv-icon-btn" title="关闭" @click="$emit('close')">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M18 6L6 18M6 6l12 12"/>
        </svg>
      </button>
    </div>

    <div v-if="selectedNode" class="gv-drawer__body">

      <!-- 基本信息 -->
      <div class="gv-info-section">
        <div class="gv-info-row">
          <span class="gv-info-label">名称</span>
          <span class="gv-info-value gv-info-value--accent">{{ selectedNode.label }}</span>
        </div>
        <div class="gv-info-row">
          <span class="gv-info-label">类型</span>
          <span class="gv-info-value">{{ selectedNodeType }}</span>
        </div>
        <div class="gv-info-row">
          <span class="gv-info-label">属性</span>
          <span class="gv-info-value gv-info-value--mono gv-info-value--muted">
            {{ selectedNodeAttributes.length > 0 ? selectedNodeAttributes.join(', ') : '-' }}
          </span>
        </div>
        <div class="gv-info-row">
          <span class="gv-info-label">关联分块数</span>
          <button
            v-if="selectedNodeChunks.length > 0"
            class="gv-info-value gv-info-value--mono gv-chunk-link"
            @click="$emit('open-chunk-viewer', `实体「${selectedNode.label}」的来源分块`, selectedNodeChunks)"
          >{{ selectedNodeChunks.length }} · 查看</button>
          <span v-else class="gv-info-value gv-info-value--mono">0</span>
        </div>
      </div>

      <!-- 关联关系 -->
      <div class="gv-section-head">
        关联关系
        <span class="gv-section-head__count">{{ nodeRelations.length }}</span>
      </div>
      <div class="gv-rel-list">
        <div
          v-for="rel in nodeRelations"
          :key="rel.id"
          class="gv-rel-row"
        >
          <span class="gv-rel-type">{{ rel.label || rel.type || '-' }}</span>
          <span class="gv-rel-arrow">→</span>
          <span class="gv-rel-target">{{ rel.target || rel.to || '-' }}</span>
          <div class="gv-rel-actions">
            <button
              v-if="rel.chunk_ids && rel.chunk_ids.length"
              class="gv-rel-btn"
              :title="`查看 ${rel.chunk_ids.length} 个来源分块`"
              @click="$emit('open-chunk-viewer', `关系「${rel.label || rel.type}」的来源分块`, rel.chunk_ids || [])"
            >分块 {{ rel.chunk_ids.length }}</button>
            <button class="gv-rel-btn" @click="$emit('edit-relation', rel)">编辑</button>
            <button class="gv-rel-btn gv-rel-btn--danger" @click="$emit('delete-relation', rel)">删除</button>
          </div>
        </div>
        <div v-if="nodeRelations.length === 0" class="gv-rel-empty">
          暂无关联关系
        </div>
      </div>

    </div>

    <!-- 底部操作 -->
    <div class="gv-drawer__footer">
      <button class="gv-btn-primary" @click="$emit('edit-node')">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="13" height="13">
          <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/>
          <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/>
        </svg>
        编辑节点
      </button>
      <button class="gv-btn-danger" @click="$emit('delete-node')" :disabled="deleting">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="13" height="13">
          <polyline points="3 6 5 6 21 6"/>
          <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
        </svg>
        删除节点
      </button>
    </div>
  </aside>
</template>

<script setup lang="ts">
import type { VisNode, RelationDisplay } from './types'

defineProps<{
  selectedNode: VisNode | null
  selectedNodeType: string
  selectedNodeAttributes: string[]
  selectedNodeChunks: string[]
  nodeRelations: RelationDisplay[]
  deleting: boolean
}>()

defineEmits<{
  (e: 'close'): void
  (e: 'open-chunk-viewer', title: string, ids: string[]): void
  (e: 'edit-relation', rel: RelationDisplay): void
  (e: 'delete-relation', rel: RelationDisplay): void
  (e: 'edit-node'): void
  (e: 'delete-node'): void
}>()
</script>

<style scoped>
/* ===== 节点详情抽屉（右侧固定，300px） ===== */
.gv-drawer {
  width: 300px;
  flex: 0 0 300px;
  border-left: 1px solid var(--color-border-default);
  background: var(--color-bg-tertiary);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.gv-drawer__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px 12px;
  border-bottom: 1px solid var(--color-border-default);
  flex: 0 0 auto;
}

.gv-drawer__title {
  font-family: var(--font-display);
  font-size: 14px;
  font-weight: 700;
  color: var(--color-text-primary);
}

.gv-drawer__body {
  flex: 1;
  overflow-y: auto;
  padding: 0;
}

/* 图标按钮（关闭按钮复用基类） */
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

/* 基本信息 */
.gv-info-section {
  padding: 12px 16px;
  border-bottom: 1px solid var(--color-border-subtle);
}

.gv-info-row {
  display: flex;
  align-items: baseline;
  gap: 10px;
  padding: 6px 0;
  font-size: 12.5px;
}

.gv-info-row + .gv-info-row {
  border-top: 1px solid var(--color-border-subtle);
}

.gv-info-label {
  color: var(--color-text-muted);
  font-size: 11px;
  font-family: var(--font-mono);
  flex: 0 0 70px;
}

.gv-info-value {
  color: var(--color-text-secondary);
  flex: 1;
}

.gv-info-value--accent {
  color: var(--color-primary);
  font-weight: 600;
}

.gv-info-value--mono {
  font-family: var(--font-mono);
  font-size: 11.5px;
}

/* 属性行：比正常值更暗（对应 mockup style="font-size:11px;color:var(--t3)"） */
.gv-info-value--muted {
  color: var(--color-text-muted);
  font-size: 11px;
}

/* 小节标题 */
.gv-section-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 10px 16px 8px;
  font-size: 11.5px;
  font-weight: 600;
  color: var(--color-text-secondary);
  border-bottom: 1px solid var(--color-border-subtle);
}

.gv-section-head__count {
  font-family: var(--font-mono);
  font-size: 10px;
  color: var(--color-text-muted);
  font-weight: 400;
}

/* 关系列表 */
.gv-rel-list {
  padding: 4px 0;
}

.gv-rel-row {
  display: flex;
  align-items: center;
  gap: 0;
  padding: 7px 16px;
  font-size: 12px;
  cursor: pointer;
  transition: background var(--duration-fast) var(--ease-default);
}

.gv-rel-row:hover {
  background: var(--color-bg-elevated);
}

.gv-rel-type {
  font-family: var(--font-mono);
  font-size: 10.5px;
  color: var(--color-text-muted);
  margin-right: 6px;
  min-width: 68px;
}

.gv-rel-arrow {
  color: var(--color-text-muted);
  margin-right: 6px;
  font-family: var(--font-mono);
  font-size: 10px;
}

.gv-rel-target {
  color: var(--color-text-secondary);
  font-size: 12px;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.gv-rel-actions {
  display: flex;
  gap: 4px;
  opacity: 0;
  transition: opacity var(--duration-fast) var(--ease-default);
}

.gv-rel-row:hover .gv-rel-actions {
  opacity: 1;
}

.gv-rel-btn {
  padding: 2px 6px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--color-border-default);
  background: transparent;
  color: var(--color-text-muted);
  font-size: 10px;
  cursor: pointer;
  transition: color var(--duration-fast), background var(--duration-fast);
}

.gv-rel-btn:hover {
  color: var(--color-text-primary);
  background: var(--color-bg-elevated);
}

.gv-rel-btn--danger {
  color: var(--color-danger);
  border-color: var(--color-border-subtle);
}

.gv-rel-btn--danger:hover {
  background: rgba(192, 138, 138, 0.08);
  border-color: var(--color-danger);
}

.gv-rel-empty {
  padding: 24px 16px;
  text-align: center;
  color: var(--color-text-muted);
  font-size: 12px;
}

/* 抽屉底部按钮区 */
.gv-drawer__footer {
  display: flex;
  gap: 8px;
  padding: 12px 16px;
  border-top: 1px solid var(--color-border-default);
  flex: 0 0 auto;
}

/* 银青实心按钮 */
.gv-btn-primary {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 8px;
  border: none;
  border-radius: var(--radius-sm);
  cursor: pointer;
  background: var(--color-primary);
  color: var(--on-primary);
  font-size: 12.5px;
  font-weight: 600;
  transition: background var(--duration-fast) var(--ease-default);
}

.gv-btn-primary:hover {
  background: var(--color-primary-light);
}

/* hairline 危险按钮 */
.gv-btn-danger {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 8px;
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-sm);
  cursor: pointer;
  background: transparent;
  color: var(--color-danger);
  font-size: 12.5px;
  transition:
    border-color var(--duration-fast) var(--ease-default),
    background var(--duration-fast) var(--ease-default);
}

.gv-btn-danger:hover {
  border-color: var(--color-danger);
  background: rgba(192, 138, 138, 0.08);
}

.gv-btn-danger:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* ===== 滚动条：与 mockup 一致（4px 细轨道） ===== */
.gv-drawer__body::-webkit-scrollbar {
  width: 4px;
}

.gv-drawer__body::-webkit-scrollbar-track {
  background: transparent;
}

.gv-drawer__body::-webkit-scrollbar-thumb {
  background: rgba(232, 230, 223, 0.14);
  border-radius: 2px;
}

/* ===== 分块链接（节点详情里可点的分块数） ===== */
.gv-chunk-link {
  border: none;
  background: transparent;
  color: var(--color-accent, #9cb4cd);
  cursor: pointer;
  padding: 0;
  text-align: right;
}

.gv-chunk-link:hover {
  text-decoration: underline;
}
</style>
