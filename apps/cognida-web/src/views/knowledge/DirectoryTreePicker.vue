<template>
  <UiModal v-model="visible" title="选择要上传的文件" size="lg" :mask-closable="false">
    <UiEmpty v-if="totalCount === 0" description="该文件夹下没有可上传的文件" />

    <template v-else>
      <div class="tree-picker__toolbar">
        <UiButton variant="ghost" size="sm" @click="toggleAll(true)">全选</UiButton>
        <UiButton variant="ghost" size="sm" @click="toggleAll(false)">取消全选</UiButton>
        <UiText size="sm" type="info">
          已选 {{ checkedCount }} / {{ totalCount }} 个文件，共 {{ formatFileSize(checkedSize) }}
        </UiText>
      </div>

      <UiDivider />

      <div class="tree-picker__tree">
        <DirectoryTreeNode v-for="child in root.children" :key="child.path" :node="child" />
      </div>

      <UiText v-if="skippedCount > 0" type="warning" size="sm" class="tree-picker__skipped">
        已过滤 {{ skippedCount }} 个不支持格式或超过 50MB 大小限制的文件，不会被上传
      </UiText>
    </template>

    <template #footer>
      <UiButton variant="secondary" @click="visible = false">取消</UiButton>
      <UiButton variant="primary" :disabled="checkedCount === 0" @click="handleConfirm">
        开始上传（{{ checkedCount }}）
      </UiButton>
    </template>
  </UiModal>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { UiModal, UiButton, UiText, UiEmpty, UiDivider } from '@/components'
import { formatFileSize } from '@/utils'
import DirectoryTreeNode from './DirectoryTreeNode.vue'
import {
  buildFileTree,
  filterAcceptableFiles,
  setSubtreeChecked,
  collectCheckedFiles,
  countFiles,
  type FileTreeNode
} from '@/utils/directoryTree'

const props = defineProps<{
  modelValue: boolean
  files: File[]
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  confirm: [files: File[]]
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit('update:modelValue', v)
})

const emptyRoot = (): FileTreeNode => ({ name: '', path: '', isFile: false, children: [], checked: false })
const root = ref<FileTreeNode>(emptyRoot())
const skippedCount = ref(0)

// 每次传入新的文件列表（重新选了一次文件夹）都重建树；webkitdirectory 无法用 accept
// 过滤类型，故这里统一按知识库支持的扩展名/大小预先过滤
watch(
  () => props.files,
  (files) => {
    const { accepted, rejectedByType, rejectedBySize } = filterAcceptableFiles(files)
    skippedCount.value = rejectedByType.length + rejectedBySize.length
    root.value = buildFileTree(accepted)
  },
  { immediate: true }
)

const totalCount = computed(() => countFiles(root.value))
const checkedFiles = computed(() => collectCheckedFiles(root.value))
const checkedCount = computed(() => checkedFiles.value.length)
const checkedSize = computed(() => checkedFiles.value.reduce((sum, f) => sum + f.size, 0))

function toggleAll(checked: boolean) {
  setSubtreeChecked(root.value, checked)
}

function handleConfirm() {
  emit('confirm', checkedFiles.value)
  visible.value = false
}
</script>

<style scoped>
.tree-picker__toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.tree-picker__tree {
  max-height: 360px;
  overflow-y: auto;
}

.tree-picker__skipped {
  display: block;
  margin-top: 8px;
}
</style>
