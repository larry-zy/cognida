<!-- 来源分块查看器 Modal —— 拆分自 GraphView.vue〔M14〕
     职责：展示实体/关系关联的来源分块内容（加载中/错误/内容三态）。
     可见性通过 v-model 与容器双向绑定；标题与条目由容器传入并异步填充。 -->
<template>
  <UiModal v-model="visible" :title="title" size="lg">
    <div class="gv-chunk-viewer">
      <div v-for="item in items" :key="item.id" class="gv-chunk-card">
        <div class="gv-chunk-card__head">
          <span class="gv-chunk-card__id">{{ item.id }}</span>
        </div>
        <div v-if="item.loading" class="gv-chunk-card__state">加载中…</div>
        <div v-else-if="item.error" class="gv-chunk-card__state gv-chunk-card__state--error">{{ item.error }}</div>
        <pre v-else class="gv-chunk-card__body">{{ item.content }}</pre>
      </div>
      <div v-if="items.length === 0" class="gv-chunk-card__state">暂无分块</div>
    </div>
    <template #footer>
      <UiButton variant="secondary" @click="visible = false">关闭</UiButton>
    </template>
  </UiModal>
</template>

<script setup lang="ts">
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import type { ChunkViewerItem } from './types'

defineProps<{
  title: string
  items: ChunkViewerItem[]
}>()

const visible = defineModel<boolean>({ default: false })
</script>

<style scoped>
/* ===== 来源分块查看器 ===== */
.gv-chunk-viewer {
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-height: 60vh;
  overflow-y: auto;
}

.gv-chunk-card {
  border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-sm);
  background: var(--color-bg-elevated);
}

.gv-chunk-card__head {
  padding: 8px 10px;
  border-bottom: 1px solid var(--color-border-subtle);
}

.gv-chunk-card__id {
  font-family: 'IBM Plex Mono', monospace;
  font-size: 11px;
  color: var(--color-text-muted);
}

.gv-chunk-card__state {
  padding: 16px;
  text-align: center;
  color: var(--color-text-muted);
  font-size: 12px;
}

.gv-chunk-card__state--error {
  color: var(--color-danger);
}

.gv-chunk-card__body {
  margin: 0;
  padding: 12px;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: 'IBM Plex Mono', monospace;
  font-size: 12px;
  line-height: 1.6;
  color: var(--color-text-secondary);
  max-height: 320px;
  overflow-y: auto;
}
</style>
