<template>
  <div :class="wrapperClasses">
    <div class="ui-pagination__info" v-if="showInfo">
      {{ infoText }}
    </div>

    <div class="ui-pagination__controls">
      <button
        :class="['ui-pagination__prev', { 'ui-pagination__btn--disabled': currentPage === 1 }]"
        :disabled="currentPage === 1"
        @click="handlePrev"
      >
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <polyline points="15 18 9 12 15 6" />
        </svg>
      </button>

      <div class="ui-pagination__pages">
        <button
          v-for="page in visiblePages"
          :key="page"
          :class="['ui-pagination__page', { 'ui-pagination__page--active': page === currentPage }]"
          @click="handlePageClick(page)"
        >
          {{ page }}
        </button>
      </div>

      <button
        :class="['ui-pagination__next', { 'ui-pagination__btn--disabled': currentPage === totalPages }]"
        :disabled="currentPage === totalPages"
        @click="handleNext"
      >
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <polyline points="9 18 15 12 9 6" />
        </svg>
      </button>
    </div>

    <div v-if="showSizeChanger" class="ui-pagination__sizes">
      <select
        :value="pageSize"
        class="ui-pagination__size-select"
        @change="handleSizeChange"
      >
        <option v-for="size in pageSizeOptions" :key="size" :value="size">
          {{ size }} 条/页
        </option>
      </select>
    </div>

    <div v-if="showJumper" class="ui-pagination__jumper">
      跳至
      <input
        ref="inputRef"
        v-model="jumpPage"
        type="number"
        class="ui-pagination__input"
        :min="1"
        :max="totalPages"
        @keyup.enter="handleJump"
      >
      页
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'

interface Props {
  page?: number
  pageSize?: number
  total?: number
  pageSizes?: number[]
  showInfo?: boolean
  showSizeChanger?: boolean
  showJumper?: boolean
  maxVisiblePages?: number
  size?: 'sm' | 'md' | 'lg'
}

const props = withDefaults(defineProps<Props>(), {
  page: 1,
  pageSize: 10,
  total: 0,
  pageSizes: () => [10, 20, 50, 100],
  showInfo: false,
  showSizeChanger: false,
  showJumper: false,
  maxVisiblePages: 7,
  size: 'md'
})

const emit = defineEmits<{
  'update:page': [page: number]
  'update:pageSize': [pageSize: number]
  'change': [page: number, pageSize: number]
}>()

const currentPage = ref(props.page)
const jumpPage = ref('')
const inputRef = ref<HTMLInputElement>()

const totalPages = computed(() => Math.ceil(props.total / props.pageSize))

const infoText = computed(() => {
  const start = (currentPage.value - 1) * props.pageSize + 1
  const end = Math.min(currentPage.value * props.pageSize, props.total)
  return `${start}-${end} / 共 ${props.total} 条`
})

const visiblePages = computed(() => {
  const pages: number[] = []
  const half = Math.floor(props.maxVisiblePages / 2)

  let start = Math.max(1, currentPage.value - half)
  let end = Math.min(totalPages.value, start + props.maxVisiblePages - 1)

  if (end - start < props.maxVisiblePages - 1) {
    start = Math.max(1, end - props.maxVisiblePages + 1)
  }

  for (let i = start; i <= end; i++) {
    pages.push(i)
  }

  return pages
})

const pageSizeOptions = computed(() => props.pageSizes)

const wrapperClasses = computed(() => [
  'ui-pagination',
  `ui-pagination--${props.size}`
])

function handlePrev() {
  if (currentPage.value > 1) {
    changePage(currentPage.value - 1)
  }
}

function handleNext() {
  if (currentPage.value < totalPages.value) {
    changePage(currentPage.value + 1)
  }
}

function handlePageClick(page: number) {
  changePage(page)
}

function handleSizeChange(event: Event) {
  const target = event.target as HTMLSelectElement
  const newPageSize = parseInt(target.value, 10)
  emit('update:pageSize', newPageSize)
  emit('change', 1, newPageSize)
  currentPage.value = 1
}

function handleJump() {
  const page = parseInt(jumpPage.value, 10)
  if (page >= 1 && page <= totalPages.value) {
    changePage(page)
    jumpPage.value = ''
  }
}

function changePage(page: number) {
  currentPage.value = page
  emit('update:page', page)
  emit('change', page, props.pageSize)
}

watch(() => props.page, (value) => {
  currentPage.value = value
})
</script>

<style scoped>
.ui-pagination {
  display: flex;
  align-items: center;
  gap: var(--space-4);
}

.ui-pagination__info {
  font-size: var(--text-sm);
  color: var(--text-muted);
}

.ui-pagination__controls {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.ui-pagination__prev,
.ui-pagination__next {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  padding: 0;
  background: var(--bg-tertiary);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
  cursor: pointer;
  transition: all var(--duration-fast);
}

.ui-pagination__prev:hover:not(.ui-pagination__btn--disabled),
.ui-pagination__next:hover:not(.ui-pagination__btn--disabled) {
  background: var(--bg-hover);
  border-color: var(--border-strong);
  color: var(--text-primary);
}

.ui-pagination__btn--disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.ui-pagination__prev svg,
.ui-pagination__next svg {
  width: 16px;
  height: 16px;
}

.ui-pagination__pages {
  display: flex;
  align-items: center;
  gap: var(--space-1);
}

.ui-pagination__page {
  min-width: 32px;
  height: 32px;
  padding: 0 var(--space-2);
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
  font-size: var(--text-sm);
  cursor: pointer;
  transition: all var(--duration-fast);
}

.ui-pagination__page:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.ui-pagination__page--active {
  background: var(--primary);
  color: #000;
  font-weight: 600;
}

.ui-pagination__sizes {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.ui-pagination__size-select {
  padding: var(--space-2) var(--space-3);
  background: var(--bg-tertiary);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
  font-size: var(--text-sm);
  cursor: pointer;
  transition: all var(--duration-fast);
}

.ui-pagination__size-select:hover {
  border-color: var(--border-strong);
}

.ui-pagination__size-select:focus {
  outline: none;
  border-color: var(--primary);
}

.ui-pagination__jumper {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--text-sm);
  color: var(--text-secondary);
}

.ui-pagination__input {
  width: 48px;
  padding: var(--space-2);
  background: var(--bg-tertiary);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
  font-size: var(--text-sm);
  text-align: center;
  transition: all var(--duration-fast);
}

.ui-pagination__input:hover {
  border-color: var(--border-strong);
}

.ui-pagination__input:focus {
  outline: none;
  border-color: var(--primary);
}

/* Sizes */
.ui-pagination--sm .ui-pagination__prev,
.ui-pagination--sm .ui-pagination__next,
.ui-pagination--sm .ui-pagination__page {
  width: 28px;
  height: 28px;
  font-size: var(--text-xs);
}

.ui-pagination--lg .ui-pagination__prev,
.ui-pagination--lg .ui-pagination__next,
.ui-pagination--lg .ui-pagination__page {
  width: 36px;
  height: 36px;
  font-size: var(--text-base);
}

/* Responsive */
@media (max-width: 767px) {
  .ui-pagination {
    flex-wrap: wrap;
    justify-content: center;
  }

  .ui-pagination__info,
  .ui-pagination__sizes,
  .ui-pagination__jumper {
    display: none;
  }
}
</style>
