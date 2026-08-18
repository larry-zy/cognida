<template>
  <div :class="wrapperClasses">
    <div class="ui-table__container">
      <table :class="tableClasses">
        <thead v-if="!hideHeader">
          <tr>
            <th
              v-for="column in columns"
              :key="column.key"
              :class="columnClasses(column)"
              :style="columnStyle(column)"
              @click="handleSort(column)"
            >
              <slot :name="`header-${column.key}`" :column="column">
                <span class="ui-table__header-title">{{ column.title }}</span>
              </slot>
              <span
                v-if="column.sortable"
                :class="sortClasses(column)"
                class="ui-table__sort"
              >
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <polyline points="18 15 12 9 6 15" />
                </svg>
              </span>
            </th>
          </tr>
        </thead>

        <tbody>
          <tr
            v-for="(row, rowIndex) in data"
            :key="rowIndex"
            :class="rowClasses(row, rowIndex)"
            @click="handleRowClick(row, rowIndex)"
          >
            <td
              v-for="column in columns"
              :key="column.key"
              :class="cellClasses(row, column)"
            >
              <slot
                :name="`cell-${column.key}`"
                :row="row"
                :column="column"
                :value="getRowValue(row, column.key)"
              >
                {{ formatCellValue(row, column) }}
              </slot>
            </td>
          </tr>

          <tr v-if="data.length === 0" class="ui-table__empty">
            <td :colspan="columns.length">
              <slot name="empty">
                <div class="ui-table__empty-content">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                    <rect x="3" y="3" width="18" height="18" rx="2" />
                    <line x1="3" y1="9" x2="21" y2="9" />
                    <line x1="9" y1="21" x2="9" y2="9" />
                  </svg>
                  <p>暂无数据</p>
                </div>
              </slot>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-if="showPagination && pagination" class="ui-table__pagination">
      <slot name="pagination">
        <UiPagination
          v-model:page="pagination.page"
          v-model:page-size="pagination.pageSize"
          :total="pagination.total"
          @change="handlePageChange"
        />
      </slot>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import UiPagination from './UiPagination.vue'

interface Column {
  key: string
  title: string
  width?: string | number
  align?: 'left' | 'center' | 'right'
  sortable?: boolean
  sorter?: (a: any, b: any) => number
  fixed?: 'left' | 'right'
  formatter?: (value: any, row: any) => string
}

interface Pagination {
  page: number
  pageSize: number
  total: number
}

interface Props {
  columns: Column[]
  data?: any[]
  rowKey?: string
  stripe?: boolean
  border?: boolean
  hover?: boolean
  size?: 'sm' | 'md' | 'lg'
  hideHeader?: boolean
  showPagination?: boolean
  pagination?: Pagination
  empty?: string
}

const props = withDefaults(defineProps<Props>(), {
  data: () => [],
  stripe: false,
  border: false,
  hover: true,
  size: 'md',
  hideHeader: false,
  showPagination: false
})

const emit = defineEmits<{
  'row-click': [row: any, index: number]
  'sort-change': [column: Column, order: 'asc' | 'desc' | null]
  'page-change': [page: number, pageSize: number]
}>()

const sortColumn = ref<Column | null>(null)
const sortOrder = ref<'asc' | 'desc' | null>(null)

const wrapperClasses = computed(() => [
  'ui-table-wrapper',
  `ui-table-wrapper--${props.size}`,
  {
    'ui-table-wrapper--border': props.border,
    'ui-table-wrapper--stripe': props.stripe
  }
])

const tableClasses = computed(() => [
  'ui-table',
  `ui-table--${props.size}`
])

function columnClasses(column: Column) {
  return [
    'ui-table__th',
    `ui-table__th--${column.align || 'left'}`,
    {
      'ui-table__th--sortable': column.sortable,
      'ui-table__th--fixed': column.fixed
    }
  ]
}

function columnStyle(column: Column) {
  return {
    width: column.width ? (typeof column.width === 'number' ? `${column.width}px` : column.width) : undefined,
    textAlign: column.align || 'left'
  }
}

function sortClasses(column: Column) {
  return [
    {
      'ui-table__sort--asc': sortColumn.value?.key === column.key && sortOrder.value === 'asc',
      'ui-table__sort--desc': sortColumn.value?.key === column.key && sortOrder.value === 'desc'
    }
  ]
}

function rowClasses(_row: any, index: number) {
  return [
    'ui-table__row',
    {
      'ui-table__row--stripe': props.stripe && index % 2 === 1
    }
  ]
}

function cellClasses(_row: any, column: Column) {
  return [
    'ui-table__td',
    `ui-table__td--${column.align || 'left'}`
  ]
}

function getRowValue(row: any, key: string) {
  return key.split('.').reduce((obj, k) => obj?.[k], row)
}

function formatCellValue(row: any, column: Column) {
  const value = getRowValue(row, column.key)
  return column.formatter ? column.formatter(value, row) : value
}

function handleRowClick(row: any, index: number) {
  // 拖选/长按选中文字时，mouseup 也会触发 click；有选区则不当作行点击，避免误开详情。
  const sel = window.getSelection()
  if (sel && !sel.isCollapsed && sel.toString().trim()) {
    return
  }
  emit('row-click', row, index)
}

function handleSort(column: Column) {
  if (!column.sortable) return

  if (sortColumn.value?.key === column.key) {
    if (sortOrder.value === 'asc') {
      sortOrder.value = 'desc'
    } else if (sortOrder.value === 'desc') {
      sortOrder.value = null
      sortColumn.value = null
    } else {
      sortOrder.value = 'asc'
    }
  } else {
    sortColumn.value = column
    sortOrder.value = 'asc'
  }

  emit('sort-change', column, sortOrder.value)
}

function handlePageChange(page: number, pageSize: number) {
  emit('page-change', page, pageSize)
}
</script>

<style scoped>
.ui-table-wrapper {
  width: 100%;
  overflow: hidden;
  background: transparent;
}

.ui-table-wrapper--border {
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
}

.ui-table__container {
  width: 100%;
  overflow-x: auto;
}

.ui-table {
  width: 100%;
  border-collapse: collapse;
  background: transparent;
}

.ui-table--sm {
  font-size: var(--text-sm);
}

.ui-table--md {
  font-size: var(--text-base);
}

.ui-table--lg {
  font-size: var(--text-lg);
}

/* Header */
.ui-table__th {
  position: relative;
  padding: var(--space-4);
  font-weight: 600;
  color: var(--text-secondary);
  background: var(--bg-tertiary);
  border-bottom: 1px solid var(--border-subtle);
  text-align: left;
  white-space: nowrap;
  user-select: none;
}

.ui-table__th--left {
  text-align: left;
}

.ui-table__th--center {
  text-align: center;
}

.ui-table__th--right {
  text-align: right;
}

.ui-table__th--sortable {
  cursor: pointer;
  transition: background var(--duration-fast);
}

.ui-table__th--sortable:hover {
  background: var(--bg-hover);
}

.ui-table__header-title {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
}

.ui-table__sort {
  display: inline-flex;
  align-items: center;
  color: var(--text-muted);
  transition: transform var(--duration-fast);
}

.ui-table__sort svg {
  width: 14px;
  height: 14px;
}

.ui-table__sort--asc svg {
  transform: rotate(180deg);
  color: var(--primary);
}

.ui-table__sort--desc svg {
  transform: rotate(0deg);
  color: var(--primary);
}

/* Row */
.ui-table__row {
  transition: background var(--duration-fast);
}

.ui-table__row--stripe {
  background: rgba(255, 255, 255, 0.02);
}

.ui-table-wrapper:not(.ui-table-wrapper--stripe) .ui-table__row:hover,
.ui-table-wrapper.ui-table-wrapper--stripe .ui-table__row:hover {
  background: var(--bg-hover);
}

.ui-table--sm .ui-table__td,
.ui-table--sm .ui-table__th {
  padding: var(--space-2) var(--space-3);
}

.ui-table--lg .ui-table__td,
.ui-table--lg .ui-table__th {
  padding: var(--space-5) var(--space-6);
}

/* Cell */
.ui-table__td {
  padding: var(--space-4);
  color: var(--text-secondary);
  border-bottom: 1px solid var(--border-subtle);
}

.ui-table__td--left {
  text-align: left;
}

.ui-table__td--center {
  text-align: center;
}

.ui-table__td--right {
  text-align: right;
}

.ui-table__row:last-child .ui-table__td {
  border-bottom: none;
}

/* Empty */
.ui-table__empty {
  background: transparent;
}

.ui-table__empty td {
  padding: var(--space-12);
  text-align: center;
}

.ui-table__empty-content {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--space-4);
  color: var(--text-muted);
}

.ui-table__empty-content svg {
  width: 48px;
  height: 48px;
  opacity: 0.5;
}

.ui-table__empty-content p {
  margin: 0;
}

/* Pagination */
.ui-table__pagination {
  display: flex;
  justify-content: flex-end;
  padding: var(--space-4) var(--space-6);
  border-top: 1px solid var(--border-subtle);
}

/* Scrollbar */
.ui-table__container::-webkit-scrollbar {
  height: 6px;
}

.ui-table__container::-webkit-scrollbar-track {
  background: transparent;
}

.ui-table__container::-webkit-scrollbar-thumb {
  background: var(--border-default);
  border-radius: var(--radius-sm);
}

.ui-table__container::-webkit-scrollbar-thumb:hover {
  background: var(--border-strong);
}

/* Responsive */
@media (max-width: 767px) {
  .ui-table__th,
  .ui-table__td {
    padding: var(--space-3);
  }

  .ui-table__pagination {
    justify-content: center;
  }
}
</style>
