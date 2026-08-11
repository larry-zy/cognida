<template>
  <div class="graph-list-page">
    <UiPageHeader
      title="知识图谱"
      :breadcrumb="[
        { label: '知识库', to: '/knowledge' },
        { label: '图谱', to: '' }
      ]"
    >
      <template #actions>
        <UiButton variant="primary" @click="showSelectKBDialog">
          <template #icon><component :is="PlusIcon" /></template>
          选择知识库查看图谱
        </UiButton>
      </template>
    </UiPageHeader>

    <div class="graph-list-container">
      <UiCard class="intro-card">
        <div class="intro-content">
          <div class="intro-icon">
            <component :is="ShareIcon" />
          </div>
          <div class="intro-text">
            <h3>什么是知识图谱？</h3>
            <p>知识图谱是从文档中自动提取的实体（人名、地名、概念等）和它们之间的关系（包含、关联、依赖等）构成的可视化网络。</p>
          </div>
        </div>
      </UiCard>

      <div class="kb-list-section">
        <h3>选择知识库查看图谱</h3>
        <UiTable
          :data="knowledgeBases"
          :loading="loading"
          :columns="tableColumns"
        >
          <template #cell-actions="{ row }">
            <UiButton variant="primary" size="sm" @click="goToGraph(row.id)">
              <template #icon><component :is="ShareIcon" /></template>
              查看图谱
            </UiButton>
          </template>
        </UiTable>

        <UiEmpty
          v-if="!loading && knowledgeBases.length === 0"
          title="暂无知识库"
          description="创建您的第一个知识库开始使用"
        >
          <UiButton variant="primary" @click="goToKnowledgeList">
            创建知识库
          </UiButton>
        </UiEmpty>
      </div>
    </div>

    <!-- 选择知识库对话框 -->
    <UiModal
      v-model="selectKBDialogVisible"
      title="选择知识库查看图谱"
      size="md"
    >
      <UiTable
        :data="knowledgeBases"
        :columns="modalColumns"
        @row-click="handleSelectKB"
      >
        <!-- 行点击选库为鼠标交互；额外提供键盘可聚焦的操作按钮，供键盘/读屏用户选库 -->
        <template #cell-actions="{ row }">
          <UiButton
            variant="primary"
            size="sm"
            :aria-label="`查看知识库 ${row.name} 的图谱`"
            @click="handleSelectKB(row)"
          >
            <template #icon><component :is="ShareIcon" /></template>
            查看图谱
          </UiButton>
        </template>
      </UiTable>
      <template #footer>
        <UiButton variant="secondary" @click="selectKBDialogVisible = false">
          取消
        </UiButton>
      </template>
    </UiModal>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useRouter } from 'vue-router'
import { PlusIcon, ShareIcon } from '@/components/icons/icons'
import { useKnowledgeStore } from '@/stores/knowledge'
import { toastError } from '@/utils/toast'
import type { KnowledgeBase } from '@/types'

// UI Components
import UiPageHeader from '@/components/layout/UiPageHeader.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiCard from '@/components/ui/UiCard.vue'
import UiTable from '@/components/ui/UiTable.vue'
import UiEmpty from '@/components/ui/UiEmpty.vue'
import UiModal from '@/components/ui/UiModal.vue'

const router = useRouter()
// 知识库列表统一走带缓存 store（〔FE-10〕），只读消费。
const knowledgeStore = useKnowledgeStore()
const { knowledgeBases, loading } = storeToRefs(knowledgeStore)
const selectKBDialogVisible = ref(false)

const tableColumns = computed(() => [
  { key: 'name', title: '知识库名称', minWidth: 200 },
  { key: 'description', title: '描述', minWidth: 300 },
  {
    key: 'status',
    title: '状态',
    width: 100,
    formatter: (value: number) => {
      return value === 1 ? '启用' : value === 0 ? '禁用' : '未知'
    }
  },
  {
    key: 'actions',
    title: '操作',
    width: 100
  }
])

const modalColumns = computed(() => [
  { key: 'name', title: '知识库名称' },
  { key: 'description', title: '描述' },
  { key: 'actions', title: '操作', width: 120 }
])

async function loadKnowledgeBases() {
  try {
    await knowledgeStore.loadKnowledgeBases()
  } catch (error) {
    console.error('Failed to load knowledge bases:', error)
    toastError('加载知识库列表失败')
  }
}

function goToGraph(kbId: string) {
  router.push(`/graphs/${kbId}`)
}

function showSelectKBDialog() {
  if (knowledgeBases.value.length === 0) {
    loadKnowledgeBases()
  } else {
    selectKBDialogVisible.value = true
  }
}

function handleSelectKB(row: KnowledgeBase) {
  selectKBDialogVisible.value = false
  goToGraph(row.id)
}

function goToKnowledgeList() {
  router.push('/knowledge')
}

onMounted(() => {
  loadKnowledgeBases()
})
</script>

<style scoped>
.graph-list-page {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: transparent;
}

.graph-list-container {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  padding: var(--space-6);
}

.intro-card {
  background: rgba(255, 255, 255, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.1);
  backdrop-filter: blur(10px);
}

.intro-content {
  display: flex;
  gap: var(--space-6);
  align-items: flex-start;
}

.intro-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  background: rgba(34, 211, 238, 0.1);
  border-radius: var(--radius-lg);
  color: var(--primary);
}

.intro-icon svg {
  width: 24px;
  height: 24px;
}

.intro-text {
  flex: 1;
}

.intro-text h3 {
  font-size: var(--text-lg);
  font-weight: 600;
  color: var(--text-primary);
  margin: 0 0 var(--space-2) 0;
}

.intro-text p {
  color: var(--text-secondary);
  line-height: 1.6;
  margin: 0;
}

.kb-list-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.kb-list-section h3 {
  font-size: var(--text-lg);
  font-weight: 600;
  color: var(--text-primary);
  margin: 0;
}
</style>
