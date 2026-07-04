<template>
  <div class="knowledge-list-view">
    <UiPageHeader
      title="知识库管理"
      description="管理和配置您的知识库，支持文档上传、向量检索和知识图谱"
    >
      <template #actions>
        <UiButton variant="primary" @click="openCreateDialog">
          <template #icon>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M12 5v14M5 12h14"/>
            </svg>
          </template>
          新建知识库
        </UiButton>
      </template>
    </UiPageHeader>

    <!-- 搜索和筛选栏 -->
    <div class="knowledge-list-toolbar">
      <div class="search-box">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="11" cy="11" r="8"/>
          <path d="m21 21-4.35-4.35"/>
        </svg>
        <input
          v-model="searchQuery"
          type="text"
          placeholder="搜索知识库..."
          class="search-input"
        >
      </div>
      <div class="filter-tags">
        <UiTag
          v-for="tag in filterTags"
          :key="tag.key"
          :variant="activeFilter === tag.key ? 'primary' : 'default'"
          clickable
          @click="activeFilter = tag.key"
        >
          {{ tag.label }}
        </UiTag>
      </div>
    </div>

    <!-- 知识库卡片网格 -->
    <div v-if="!loading && filteredKnowledgeBases.length > 0" class="knowledge-grid">
      <div
        v-for="kb in filteredKnowledgeBases"
        :key="kb.id"
        class="knowledge-card"
        @click="goToDetail(kb.id)"
      >
        <div class="knowledge-card__header">
          <div class="knowledge-card__icon">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
              <polyline points="14 2 14 8 20 8"/>
            </svg>
          </div>
          <div class="knowledge-card__status" :class="{ 'status--active': kb.status === 1 }">
            <span>{{ kb.status === 1 ? '启用' : '禁用' }}</span>
          </div>
        </div>

        <h3 class="knowledge-card__title">{{ kb.name }}</h3>
        <p class="knowledge-card__description">{{ kb.description || '暂无描述' }}</p>

        <div class="knowledge-card__meta">
          <div class="meta-item">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
            </svg>
            <span>{{ kb.document_count || 0 }} 文档</span>
          </div>
          <div class="meta-item">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="18" cy="5" r="3"/>
              <circle cx="6" cy="12" r="3"/>
              <circle cx="18" cy="19" r="3"/>
              <line x1="8.59" y1="13.51" x2="15.42" y2="17.49"/>
              <line x1="15.41" y1="6.51" x2="8.59" y2="10.49"/>
            </svg>
            <span>{{ kb.setting?.graph_enabled ? '图谱' : '' }}</span>
          </div>
          <div class="meta-item">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>
            </svg>
            <span>{{ kb.setting?.bm25_enabled ? 'BM25' : '' }}</span>
          </div>
        </div>

        <div class="knowledge-card__actions" @click.stop>
          <UiButton variant="ghost" size="sm" @click="editKnowledgeBase(kb)">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/>
              <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/>
            </svg>
          </UiButton>
          <UiButton variant="ghost" size="sm" @click="deleteKnowledgeBase(kb.id)">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M3 6h18M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
            </svg>
          </UiButton>
        </div>
      </div>
    </div>

    <!-- 加载状态 -->
    <div v-if="loading" class="loading-state">
      <UiLoader variant="spinner" />
    </div>

    <!-- 空状态 -->
    <UiEmpty
      v-if="!loading && filteredKnowledgeBases.length === 0"
      :title="searchQuery ? '未找到匹配的知识库' : '暂无知识库'"
      :description="searchQuery ? '请尝试其他搜索关键词' : '创建您的第一个知识库开始使用'"
    >
      <UiButton variant="primary" @click="openCreateDialog">
        <template #icon>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 5v14M5 12h14"/>
          </svg>
        </template>
        创建知识库
      </UiButton>
    </UiEmpty>

    <!-- 创建/编辑弹窗 -->
    <UiModal v-model="showDialog" :title="editingId ? '编辑知识库' : '新建知识库'" size="lg">
      <UiTabs v-model="activeTab" :tabs="tabs">
        <template #basic>
          <div class="form-section">
            <div class="form-group">
              <label class="form-label">知识库名称 <span class="required">*</span></label>
              <UiInput
                v-model="formData.name"
                placeholder="请输入知识库名称"
                :error="formErrors.name"
              />
            </div>

            <div class="form-group">
              <label class="form-label">描述</label>
              <UiTextarea
                v-model="formData.description"
                placeholder="请输入知识库描述"
                :rows="3"
              />
            </div>

            <div class="form-group">
              <label class="form-label">可见性</label>
              <div class="switch-group">
                <UiSwitch v-model="formData.is_public" />
                <span class="switch-label">{{ formData.is_public ? '公开' : '私有' }}</span>
              </div>
            </div>
          </div>
        </template>

        <template #processing>
          <div class="form-section">
            <!-- 向量检索状态说明 -->
            <UiAlert variant="success" class="embedding-status">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" slot="icon">
                <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/>
                <polyline points="22 4 12 14.01 9 11.01"/>
              </svg>
              <strong>向量检索已启用</strong>
              <p>系统已配置向量化服务（模型：text-embedding-v4），上传的文档将自动进行向量化处理，支持语义检索。</p>
            </UiAlert>

            <h4 class="section-title">分块配置</h4>

            <div class="form-group">
              <label class="form-label">分块大小</label>
              <div class="slider-group">
                <input
                  type="range"
                  v-model.number="formData.chunk_size"
                  min="128"
                  max="2048"
                  step="64"
                  class="slider"
                >
                <span class="slider-value">{{ formData.chunk_size }}</span>
              </div>
              <p class="form-hint">建议值：512-1024，值越小分块越精细</p>
            </div>

            <div class="form-group">
              <label class="form-label">分块重叠</label>
              <div class="slider-group">
                <input
                  type="range"
                  v-model.number="formData.chunk_overlap"
                  min="0"
                  max="512"
                  step="32"
                  class="slider"
                >
                <span class="slider-value">{{ formData.chunk_overlap }}</span>
              </div>
              <p class="form-hint">建议值：50-200，重叠可以保持上下文连贯性</p>
            </div>

            <UiDivider />

            <h4 class="section-title">索引构建</h4>

            <div class="form-group">
              <div class="switch-row">
                <UiSwitch v-model="formData.graph_enabled" />
                <div class="switch-info">
                  <span class="switch-label">构建知识图谱</span>
                  <p class="form-hint">为文档构建知识图谱，支持图谱检索</p>
                </div>
              </div>
            </div>

            <div class="form-group">
              <div class="switch-row">
                <UiSwitch v-model="formData.bm25_enabled" />
                <div class="switch-info">
                  <span class="switch-label">构建 BM25 索引</span>
                  <p class="form-hint">构建 BM25 关键词索引，支持关键词检索</p>
                </div>
              </div>
            </div>
          </div>
        </template>

        <template #info>
          <div class="info-panel">
            <UiAlert variant="info">
              <strong>检索配置已迁移至会话级别</strong>
              <p>知识库的检索配置（如相似度阈值、返回数量、重排序等）现在可以在创建会话时进行调整，实现跨知识库的统一检索配置。</p>
              <p>这样设计的好处：</p>
              <ul>
                <li>支持跨多个知识库的统一检索配置</li>
                <li>不同的会话可以使用不同的检索策略</li>
                <li>知识库专注于数据处理，检索策略按需配置</li>
              </ul>
            </UiAlert>

            <UiDivider />

            <h4>默认检索配置</h4>
            <div class="default-config">
              <div class="config-item">
                <span class="config-label">检索模式</span>
                <span class="config-value">向量检索（必选）+ 可选 BM25/图谱检索</span>
              </div>
              <div class="config-item">
                <span class="config-label">相似度阈值</span>
                <span class="config-value">70%</span>
              </div>
              <div class="config-item">
                <span class="config-label">返回数量</span>
                <span class="config-value">5 条</span>
              </div>
            </div>
          </div>
        </template>
      </UiTabs>

      <template #footer>
        <UiButton variant="ghost" @click="showDialog = false">取消</UiButton>
        <UiButton variant="primary" @click="saveKnowledgeBase" :loading="saving">
          {{ editingId ? '保存' : '创建' }}
        </UiButton>
      </template>
    </UiModal>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from '@/utils/toast'
import { ElMessageBox } from '@/utils/confirm'
import UiPageHeader from '@/components/layout/UiPageHeader.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiTag from '@/components/ui/UiTag.vue'
import UiModal from '@/components/ui/UiModal.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiTextarea from '@/components/ui/UiTextarea.vue'
import UiSwitch from '@/components/ui/UiSwitch.vue'
import UiTabs from '@/components/ui/UiTabs.vue'
import UiDivider from '@/components/layout/UiDivider.vue'
import UiAlert from '@/components/ui/UiAlert.vue'
import UiLoader from '@/components/ui/UiLoader.vue'
import UiEmpty from '@/components/ui/UiEmpty.vue'
import { knowledgeApi } from '@/api/knowledge'
import type { KnowledgeBase, CreateKnowledgeBaseRequest } from '@/types'

const router = useRouter()

const loading = ref(false)
const saving = ref(false)
const showDialog = ref(false)
const editingId = ref<string>('')
const knowledgeBases = ref<KnowledgeBase[]>([])
const searchQuery = ref('')
const activeFilter = ref('all')
const activeTab = ref('basic')

const tabs = [
  { key: 'basic', label: '基本信息' },
  { key: 'processing', label: '数据处理' },
  { key: 'info', label: '配置说明' }
]

const filterTags = [
  { key: 'all', label: '全部' },
  { key: 'active', label: '启用' },
  { key: 'graph', label: '有图谱' }
]

const formData = reactive<CreateKnowledgeBaseRequest>({
  name: '',
  description: '',
  avatar: '',
  is_public: false,
  chunk_size: 512,
  chunk_overlap: 100,
  graph_enabled: false,
  bm25_enabled: false
})

const formErrors = reactive<Record<string, string>>({})

const filteredKnowledgeBases = computed(() => {
  let result = knowledgeBases.value

  // 搜索过滤
  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase()
    result = result.filter(kb =>
      kb.name?.toLowerCase().includes(query) ||
      kb.description?.toLowerCase().includes(query)
    )
  }

  // 标签过滤
  if (activeFilter.value === 'active') {
    result = result.filter(kb => kb.status === 1)
  } else if (activeFilter.value === 'graph') {
    result = result.filter(kb => kb.setting?.graph_enabled)
  }

  return result
})

function openCreateDialog() {
  editingId.value = ''
  resetForm()
  showDialog.value = true
}

function resetForm() {
  Object.assign(formData, {
    name: '',
    description: '',
    avatar: '',
    is_public: false,
    chunk_size: 512,
    chunk_overlap: 100,
    graph_enabled: false,
    bm25_enabled: false
  })
  Object.keys(formErrors).forEach(key => delete formErrors[key])
}

function goToDetail(id: string) {
  router.push(`/knowledge/${id}`)
}

async function loadKnowledgeBases() {
  loading.value = true
  try {
    const res = await knowledgeApi.getList()
    if (res.data) {
      // Handle both array response and paginated response with items
      if (Array.isArray(res.data)) {
        knowledgeBases.value = res.data
      } else if ((res.data as any).items) {
        knowledgeBases.value = (res.data as any).items
      } else {
        knowledgeBases.value = []
      }
    }
  } catch (error) {
    console.error('Failed to load knowledge bases:', error)
  } finally {
    loading.value = false
  }
}

async function saveKnowledgeBase() {
  // 验证
  if (!formData.name?.trim()) {
    formErrors.name = '请输入知识库名称'
    return
  }
  delete formErrors.name

  saving.value = true
  try {
    const request: CreateKnowledgeBaseRequest = {
      name: formData.name,
      description: formData.description,
      avatar: formData.avatar,
      is_public: formData.is_public,
      chunk_size: formData.chunk_size,
      chunk_overlap: formData.chunk_overlap,
      graph_enabled: formData.graph_enabled,
      bm25_enabled: formData.bm25_enabled
    }

    if (editingId.value) {
      await knowledgeApi.update(editingId.value, request)
      ElMessage.success('知识库更新成功')
    } else {
      await knowledgeApi.create(request)
      ElMessage.success('知识库创建成功')
    }

    showDialog.value = false
    await loadKnowledgeBases()
  } catch (error: any) {
    ElMessage.error(error.message || '操作失败')
  } finally {
    saving.value = false
  }
}

async function editKnowledgeBase(kb: KnowledgeBase) {
  editingId.value = kb.id
  // 加载数据
  try {
    const res = await knowledgeApi.getDetail(kb.id)
    if (res.data) {
      const data = res.data as KnowledgeBase
      formData.name = data.name || ''
      formData.description = data.description || ''
      formData.avatar = data.avatar || ''
      formData.is_public = data.is_public || false

      if (data.setting) {
        formData.chunk_size = data.setting.chunk_size ?? 512
        formData.chunk_overlap = data.setting.chunk_overlap ?? 100
        formData.graph_enabled = data.setting.graph_enabled ?? false
        formData.bm25_enabled = data.setting.bm25_enabled ?? false
      }
    }
  } catch (error) {
    console.error('Failed to load knowledge base:', error)
  }
  showDialog.value = true
}

async function deleteKnowledgeBase(id: string) {
  const kb = knowledgeBases.value.find(k => k.id === id)
  const kbName = kb?.name ? `「${kb.name}」` : ''
  try {
    await ElMessageBox.confirm({
      title: '删除知识库',
      message: `删除知识库${kbName}后，其所有文档、分块与向量索引都会被永久删除，此操作不可恢复。确定要删除吗？`,
      type: 'danger',
      confirmButtonText: '确定删除',
      cancelButtonText: '取消'
    })

    await knowledgeApi.delete(id)
    ElMessage.success(`知识库${kbName}已删除`)
    await loadKnowledgeBases()
  } catch (error: any) {
    if (error !== 'cancel') {
      ElMessage.error(error.message || '删除失败')
    }
  }
}

onMounted(() => {
  loadKnowledgeBases()
})
</script>

<style scoped>
.knowledge-list-view {
  width: 100%;
}

/* 搜索和筛选栏 */
.knowledge-list-toolbar {
  display: flex;
  align-items: center;
  gap: var(--space-4);
  margin-bottom: var(--space-6);
  flex-wrap: wrap;
}

.search-box {
  flex: 1;
  min-width: 280px;
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  transition: border-color var(--duration-fast);
}

.search-box:focus-within {
  border-color: var(--primary);
}

.search-box svg {
  width: 18px;
  height: 18px;
  color: var(--text-muted);
}

.search-input {
  flex: 1;
  background: transparent;
  border: none;
  outline: none;
  font-size: var(--text-sm);
  color: var(--text-primary);
}

.search-input::placeholder {
  color: var(--text-muted);
}

.filter-tags {
  display: flex;
  gap: var(--space-2);
}

/* 知识库卡片网格 */
.knowledge-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: var(--space-5);
}

.knowledge-card {
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  padding: var(--space-5);
  cursor: pointer;
  transition: all var(--duration-base);
  display: flex;
  flex-direction: column;
  position: relative;
  overflow: hidden;
}

.knowledge-card::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 3px;
  background: linear-gradient(90deg, var(--primary), var(--primary-dark));
  transform: scaleX(0);
  transform-origin: left;
  transition: transform var(--duration-base);
}

.knowledge-card:hover {
  transform: translateY(-4px);
  border-color: var(--primary);
  box-shadow: var(--shadow-xl);
}

.knowledge-card:hover::before {
  transform: scaleX(1);
}

.knowledge-card__header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: var(--space-4);
}

.knowledge-card__icon {
  width: 48px;
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(34, 211, 238, 0.1);
  border-radius: var(--radius-lg);
  color: var(--primary);
}

.knowledge-card__icon svg {
  width: 24px;
  height: 24px;
}

.knowledge-card__status {
  font-size: var(--text-xs);
  padding: var(--space-1) var(--space-2);
  background: var(--bg-tertiary);
  border-radius: var(--radius-full);
  color: var(--text-muted);
}

.knowledge-card__status.status--active {
  background: rgba(34, 211, 238, 0.15);
  color: var(--primary);
}

.knowledge-card__title {
  font: var(--font-display);
  font-size: var(--text-lg);
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: var(--space-2);
}

.knowledge-card__description {
  font-size: var(--text-sm);
  color: var(--text-secondary);
  line-height: 1.5;
  margin-bottom: var(--space-4);
  flex: 1;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.knowledge-card__meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  margin-bottom: var(--space-4);
}

.meta-item {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  font-size: var(--text-xs);
  color: var(--text-muted);
}

.meta-item svg {
  width: 14px;
  height: 14px;
}

.meta-item.vector-enabled {
  color: var(--primary);
}

/* 向量检索状态提示 */
.embedding-status {
  margin-bottom: var(--space-4);
}

.embedding-status svg {
  width: 20px;
  height: 20px;
  color: var(--color-success);
}

.embedding-status strong {
  display: block;
  margin-bottom: var(--space-1);
  color: var(--color-success);
}

.embedding-status p {
  font-size: var(--text-sm);
  color: var(--text-secondary);
  margin: 0;
  line-height: 1.5;
}

.knowledge-card__actions {
  display: flex;
  gap: var(--space-2);
  padding-top: var(--space-4);
  border-top: 1px solid var(--border-subtle);
}

.knowledge-card__actions button {
  padding: var(--space-2);
}

.knowledge-card__actions svg {
  width: 16px;
  height: 16px;
}

/* 加载状态 */
.loading-state {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-16);
}

/* 表单 */
.form-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.form-label {
  font-size: var(--text-sm);
  font-weight: 500;
  color: var(--text-primary);
}

.required {
  color: var(--color-danger);
}

.form-hint {
  font-size: var(--text-xs);
  color: var(--text-muted);
  margin: 0;
}

.section-title {
  font: var(--font-display);
  font-size: var(--text-base);
  font-weight: 600;
  color: var(--text-primary);
  margin: 0;
}

.switch-group {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

.switch-label {
  font-size: var(--text-sm);
  color: var(--text-secondary);
}

.slider-group {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

.slider {
  flex: 1;
  height: 4px;
  background: var(--bg-tertiary);
  border-radius: var(--radius-full);
  appearance: none;
  cursor: pointer;
}

.slider::-webkit-slider-thumb {
  appearance: none;
  width: 16px;
  height: 16px;
  background: var(--primary);
  border-radius: 50%;
  cursor: pointer;
}

.slider-value {
  min-width: 48px;
  text-align: right;
  font-size: var(--text-sm);
  color: var(--text-secondary);
  font-variant-numeric: tabular-nums;
}

.switch-row {
  display: flex;
  align-items: flex-start;
  gap: var(--space-3);
}

.switch-info {
  flex: 1;
}

/* 信息面板 */
.info-panel {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.info-panel h4 {
  font: var(--font-display);
  font-size: var(--text-base);
  font-weight: 600;
  color: var(--text-primary);
  margin: 0;
}

.info-panel p {
  font-size: var(--text-sm);
  color: var(--text-secondary);
  margin: 0;
  line-height: 1.6;
}

.info-panel ul {
  margin: var(--space-2) 0 0 0;
  padding-left: var(--space-5);
}

.info-panel li {
  font-size: var(--text-sm);
  color: var(--text-secondary);
  margin-bottom: var(--space-1);
}

.default-config {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.config-item {
  display: flex;
  justify-content: space-between;
  padding: var(--space-3);
  background: var(--bg-secondary);
  border-radius: var(--radius-md);
}

.config-label {
  font-size: var(--text-sm);
  color: var(--text-secondary);
}

.config-value {
  font-size: var(--text-sm);
  color: var(--text-primary);
  font-weight: 500;
}

/* 响应式 */
@media (max-width: 767px) {
  .knowledge-grid {
    grid-template-columns: 1fr;
  }

  .knowledge-list-toolbar {
    flex-direction: column;
    align-items: stretch;
  }

  .filter-tags {
    flex-wrap: wrap;
  }
}
</style>
