<template>
  <div class="base-table-wrapper" :class="wrapperClasses">
    <!-- 工具栏 -->
    <div v-if="$slots.toolbar || showToolbar" class="table-toolbar">
      <slot name="toolbar">
        <div class="toolbar-left">
          <slot name="toolbar-left" />
        </div>
        <div class="toolbar-right">
          <slot name="toolbar-right" />
        </div>
      </slot>
    </div>

    <!-- 表格容器 -->
    <div class="table-container" :class="{ 'is-loading': loading }">
      <!-- 加载遮罩 -->
      <div v-if="loading" class="table-loading">
        <BaseLoader :size="loaderSize" />
      </div>

      <!-- 表格 -->
      <div class="table-scroll">
        <table class="base-table" :class="tableClasses">
          <!-- 表头 -->
          <thead>
            <tr>
              <!-- 选择列 -->
              <th v-if="selectable" class="table-select">
                <input
                  type="checkbox"
                  :checked="isAllSelected"
                  :indeterminate="isSomeSelected"
                  @change="toggleSelectAll"
                />
              </th>

              <!-- 序号列 -->
              <th v-if="showIndex" class="table-index">#</th>

              <!-- 数据列 -->
              <th
                v-for="column in columns"
                :key="column.key"
                :class="[
                  `align-${column.align || 'left'}`,
                  { 'is-sortable': column.sortable }
                ]"
                :style="{ width: column.width }"
                @click="column.sortable && handleSort(column.key)"
              >
                <span class="th-content">
                  <slot :name="`header-${column.key}`" :column="column">
                    {{ column.title }}
                  </slot>
                  <span v-if="column.sortable" class="sort-icon">
                    <svg
                      v-show="sortKey === column.key && sortOrder === 'asc'"
                      viewBox="0 0 24 24"
                      fill="currentColor"
                    >
                      <path d="M12 3l10 16H2z"/>
                    </svg>
                    <svg
                      v-show="sortKey === column.key && sortOrder === 'desc'"
                      viewBox="0 0 24 24"
                      fill="currentColor"
                    >
                      <path d="M12 21L2 5h20z"/>
                    </svg>
                  </span>
                </span>
              </th>

              <!-- 操作列 -->
              <th v-if="$slots.action" class="table-action">{{ actionTitle }}</th>
            </tr>
          </thead>

          <!-- 表体 -->
          <tbody>
            <tr
              v-for="(row, index) in displayData"
              :key="getRowKey(row, index)"
              :class="{
                'is-selected': isRowSelected(row),
                'is-clickable': clickable
              }"
              @click="handleRowClick(row, index)"
            >
              <!-- 选择列 -->
              <td v-if="selectable" class="table-select">
                <input
                  type="checkbox"
                  :checked="isRowSelected(row)"
                  @change="toggleRowSelection(row)"
                  @click.stop
                />
              </td>

              <!-- 序号列 -->
              <td v-if="showIndex" class="table-index">
                {{ (currentPage - 1) * pageSize + index + 1 }}
              </td>

              <!-- 数据列 -->
              <td
                v-for="column in columns"
                :key="column.key"
                :class="`align-${column.align || 'left'}`"
              >
                <slot :name="`cell-${column.key}`" :row="row" :column="column" :index="index">
                  {{ getCellValue(row, column.key) }}
                </slot>
              </td>

              <!-- 操作列 -->
              <td v-if="$slots.action" class="table-action">
                <slot name="action" :row="row" :index="index" />
              </td>
            </tr>

            <!-- 空状态 -->
            <tr v-if="displayData.length === 0 && !loading">
              <td :colspan="columnCount" class="table-empty">
                <div class="empty-state">
                  <slot name="empty">
                    <div class="empty-icon">
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                        <path d="M3 3v18h18M18 17V9M13 17V5M8 17v-3"/>
                      </svg>
                    </div>
                    <p class="empty-text">{{ emptyText }}</p>
                  </slot>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 分页 -->
    <div v-if="pagination && totalPages > 1" class="table-pagination">
      <div class="pagination-info">
        共 {{ total }} 条记录
      </div>
      <div class="pagination-controls">
        <BaseButton
          size="sm"
          variant="ghost"
          :disabled="currentPage === 1"
          @click="currentPage--"
        >
          上一页
        </BaseButton>
        <span class="pagination-pages">{{ currentPage }} / {{ totalPages }}</span>
        <BaseButton
          size="sm"
          variant="ghost"
          :disabled="currentPage === totalPages"
          @click="currentPage++"
        >
          下一页
        </BaseButton>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, useSlots } from 'vue'
import BaseButton from './BaseButton.vue'
import BaseLoader from './BaseLoader.vue'

interface Column {
  key: string
  title: string
  width?: string
  align?: 'left' | 'center' | 'right'
  sortable?: boolean
}

interface Props {
  data: any[]
  columns: Column[]
  rowKey?: string
  selectable?: boolean
  showIndex?: boolean
  clickable?: boolean
  loading?: boolean
  showToolbar?: boolean
  pagination?: boolean
  pageSize?: number
  emptyText?: string
  actionTitle?: string
  variant?: 'default' | 'bordered' | 'striped' | 'hover'
  size?: 'sm' | 'md' | 'lg'
}

const props = withDefaults(defineProps<Props>(), {
  rowKey: 'id',
  selectable: false,
  showIndex: false,
  clickable: false,
  loading: false,
  showToolbar: false,
  pagination: false,
  pageSize: 10,
  emptyText: '暂无数据',
  actionTitle: '操作',
  variant: 'default',
  size: 'md'
})

const emit = defineEmits<{
  'row-click': [row: any, index: number]
  'selection-change': [selection: any[]]
  'sort': [key: string, order: 'asc' | 'desc']
}>()

// 选中的行
const selectedRows = ref<any[]>([])

// 排序
const sortKey = ref<string>('')
const sortOrder = ref<'asc' | 'desc'>('asc')

// 分页
const currentPage = ref(1)

const wrapperClasses = computed(() => [`variant-${props.variant}`, `size-${props.size}`])

const tableClasses = computed(() => [`variant-${props.variant}`, `size-${props.size}`])

// 显示的数据（分页后）
const displayData = computed(() => {
  if (!props.pagination) return props.data
  const start = (currentPage.value - 1) * props.pageSize
  const end = start + props.pageSize
  return props.data.slice(start, end)
})

// 总条数
const total = computed(() => props.data.length)

// 总页数
const totalPages = computed(() => Math.ceil(total.value / props.pageSize))

const slots = useSlots()

// 列数
const columnCount = computed(() => {
  let count = props.columns.length
  if (props.selectable) count++
  if (props.showIndex) count++
  if (slots.action) count++
  return count
})

// 加载器大小
const loaderSize = computed(() => {
  if (props.size === 'sm') return 'sm'
  if (props.size === 'lg') return 'lg'
  return 'md'
})

// 是否全选
const isAllSelected = computed(() =>
  selectedRows.value.length === props.data.length && props.data.length > 0
)

// 是否部分选中
const isSomeSelected = computed(() =>
  selectedRows.value.length > 0 && selectedRows.value.length < props.data.length
)

// 获取行唯一键
function getRowKey(row: any, index: number) {
  return row[props.rowKey] ?? index
}

// 获取单元格值
function getCellValue(row: any, key: string) {
  return key.split('.').reduce((obj, k) => obj?.[k], row)
}

// 判断行是否选中
function isRowSelected(row: any): boolean {
  return selectedRows.value.some(r => getRowKey(r, 0) === getRowKey(row, 0))
}

// 切换行选择
function toggleRowSelection(row: any) {
  const key = getRowKey(row, 0)
  const index = selectedRows.value.findIndex(r => getRowKey(r, 0) === key)
  if (index >= 0) {
    selectedRows.value.splice(index, 1)
  } else {
    selectedRows.value.push(row)
  }
  emit('selection-change', selectedRows.value)
}

// 全选/取消全选
function toggleSelectAll() {
  if (isAllSelected.value) {
    selectedRows.value = []
  } else {
    selectedRows.value = [...props.data]
  }
  emit('selection-change', selectedRows.value)
}

// 排序
function handleSort(key: string) {
  if (sortKey.value === key) {
    sortOrder.value = sortOrder.value === 'asc' ? 'desc' : 'asc'
  } else {
    sortKey.value = key
    sortOrder.value = 'asc'
  }
  emit('sort', key, sortOrder.value)
}

// 行点击
function handleRowClick(row: any, index: number) {
  if (props.clickable) {
    emit('row-click', row, index)
  }
}

// 监听分页变化
watch(currentPage, () => {
  // 可以触发加载事件
})

// 监听数据变化，重置选中
watch(() => props.data, () => {
  selectedRows.value = []
  currentPage.value = 1
})
</script>

<style scoped>
/* ==================== 包裹器 ==================== */
.base-table-wrapper {
  display: flex;
  flex-direction: column;
  background: var(--color-bg-secondary);
  border: 1px solid var(--color-border-primary);
  border-radius: var(--radius-lg);
  overflow: hidden;
}

/* ==================== 工具栏 ==================== */
.table-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--spacing-md) var(--spacing-lg);
  border-bottom: 1px solid var(--color-border-secondary);
  gap: var(--spacing-md);
}

.toolbar-left,
.toolbar-right {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

/* ==================== 表格容器 ==================== */
.table-container {
  position: relative;
  flex: 1;
  overflow: hidden;
}

.table-container.is-loading {
  pointer-events: none;
}

.table-loading {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.3);
  backdrop-filter: blur(2px);
  z-index: 10;
}

.table-scroll {
  overflow-x: auto;
  overflow-y: auto;
  max-height: 500px;
}

/* ==================== 表格 ==================== */
.base-table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--text-sm);
}

.base-table th,
.base-table td {
  padding: var(--spacing-md) var(--spacing-lg);
  text-align: left;
  border-bottom: 1px solid var(--color-border-secondary);
}

.base-table th {
  font-weight: 600;
  color: var(--color-text-secondary);
  background: var(--color-bg-tertiary);
  position: sticky;
  top: 0;
  z-index: 5;
  white-space: nowrap;
}

.base-table td {
  color: var(--color-text-primary);
}

/* 对齐 */
.align-left {
  text-align: left;
}

.align-center {
  text-align: center;
}

.align-right {
  text-align: right;
}

/* ==================== 变体样式 ==================== */

/* Bordered - 带边框 */
.variant-bordered.base-table th,
.variant-bordered.base-table td {
  border-right: 1px solid var(--color-border-secondary);
}

.variant-bordered.base-table th:last-child,
.variant-bordered.base-table td:last-child {
  border-right: none;
}

/* Striped - 斑马纹 */
.variant-striped.base-table tbody tr:nth-child(even) {
  background: var(--color-bg-elevated);
}

/* Hover - 悬停效果 */
.variant-hover.base-table tbody tr:hover {
  background: var(--color-bg-elevated);
}

/* ==================== 行状态 ==================== */
.base-table tbody tr.is-selected {
  background: rgba(99, 102, 241, 0.15);
}

.base-table tbody tr.is-clickable {
  cursor: pointer;
}

.base-table tbody tr.is-clickable:hover {
  background: var(--color-bg-elevated);
}

/* ==================== 特殊列 ==================== */
.table-select,
.table-index,
.table-action {
  width: 50px;
  text-align: center;
}

.table-select input[type="checkbox"] {
  width: 16px;
  height: 16px;
  cursor: pointer;
  accent-color: var(--color-primary);
}

/* ==================== 排序图标 ==================== */
.is-sortable {
  cursor: pointer;
  user-select: none;
}

.is-sortable:hover .th-content {
  color: var(--color-text-primary);
}

.th-content {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.sort-icon {
  display: inline-flex;
  align-items: center;
  color: var(--color-text-muted);
}

.sort-icon svg {
  width: 12px;
  height: 12px;
}

/* ==================== 空状态 ==================== */
.table-empty {
  padding: var(--spacing-2xl) !important;
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: var(--color-text-muted);
}

.empty-icon {
  width: 64px;
  height: 64px;
  opacity: 0.5;
  margin-bottom: var(--spacing-md);
}

.empty-icon svg {
  width: 100%;
  height: 100%;
}

.empty-text {
  font-size: var(--text-base);
  margin: 0;
}

/* ==================== 分页 ==================== */
.table-pagination {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--spacing-md) var(--spacing-lg);
  border-top: 1px solid var(--color-border-secondary);
}

.pagination-info {
  font-size: var(--text-sm);
  color: var(--color-text-muted);
}

.pagination-controls {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.pagination-pages {
  font-size: var(--text-sm);
  color: var(--color-text-secondary);
  padding: 0 var(--spacing-sm);
}

/* ==================== 尺寸变体 ==================== */
.size-sm.base-table th,
.size-sm.base-table td {
  padding: var(--spacing-sm) var(--spacing-md);
  font-size: var(--text-xs);
}

.size-lg.base-table th,
.size-lg.base-table td {
  padding: var(--spacing-lg) var(--spacing-xl);
  font-size: var(--text-base);
}

/* ==================== 响应式 ==================== */
@media (max-width: 640px) {
  .table-toolbar {
    flex-direction: column;
    align-items: stretch;
  }

  .table-pagination {
    flex-direction: column;
    gap: var(--spacing-sm);
  }

  .pagination-controls {
    width: 100%;
    justify-content: center;
  }
}
</style>
