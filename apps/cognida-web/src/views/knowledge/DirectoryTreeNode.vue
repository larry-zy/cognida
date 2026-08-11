<template>
  <div class="tree-node">
    <div class="tree-node__row">
      <button
        v-if="!node.isFile && node.children.length"
        type="button"
        class="tree-node__toggle"
        @click="expanded = !expanded"
      >
        {{ expanded ? '▾' : '▸' }}
      </button>
      <span v-else class="tree-node__toggle-spacer" />

      <UiCheckbox
        :model-value="checked"
        :indeterminate="indeterminate"
        @change="onToggle"
      />

      <span class="tree-node__icon">{{ node.isFile ? '📄' : '📁' }}</span>
      <span class="tree-node__name" :title="node.name">{{ node.name }}</span>
      <span class="tree-node__meta">
        <template v-if="node.isFile">{{ formatFileSize(node.file?.size ?? 0) }}</template>
        <template v-else>{{ fileCount }} 个文件 · {{ formatFileSize(sizeBytes) }}</template>
      </span>
    </div>

    <div v-if="!node.isFile && expanded" class="tree-node__children">
      <DirectoryTreeNode v-for="child in node.children" :key="child.path" :node="child" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { UiCheckbox } from '@/components'
import { formatFileSize } from '@/utils'
import {
  type FileTreeNode,
  isNodeFullyChecked,
  isNodeIndeterminate,
  setSubtreeChecked,
  countFiles,
  sumFileSize
} from '@/utils/directoryTree'

const props = defineProps<{
  node: FileTreeNode
}>()

// 默认展开：批量上传前用户需要看清全貌再决定取消哪些文件
const expanded = ref(true)

const checked = computed(() => isNodeFullyChecked(props.node))
const indeterminate = computed(() => isNodeIndeterminate(props.node))
const fileCount = computed(() => countFiles(props.node))
const sizeBytes = computed(() => sumFileSize(props.node))

function onToggle(value: boolean | string | number | (string | number | boolean)[]) {
  setSubtreeChecked(props.node, Boolean(value))
}
</script>

<style scoped>
.tree-node__row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
}

.tree-node__toggle,
.tree-node__toggle-spacer {
  width: 16px;
  flex-shrink: 0;
}

.tree-node__toggle {
  background: none;
  border: none;
  color: var(--color-text-muted);
  cursor: pointer;
  padding: 0;
  font-size: 12px;
}

.tree-node__icon {
  flex-shrink: 0;
}

.tree-node__name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-text-primary);
}

.tree-node__meta {
  flex-shrink: 0;
  font-size: 12px;
  color: var(--color-text-muted);
}

.tree-node__children {
  margin-left: 24px;
  border-left: 1px solid rgba(255, 255, 255, 0.08);
  padding-left: 8px;
}
</style>
