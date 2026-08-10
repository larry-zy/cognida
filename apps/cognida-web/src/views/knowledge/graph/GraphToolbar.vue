<!-- 图谱顶部工具栏 —— 拆分自 GraphView.vue〔M14〕
     职责：标题/面包屑、搜索实体、添加节点/关系、导出/全屏/清空图谱。
     搜索框及其聚焦（focusSearch）为本组件内部行为；其余操作以 emits 上抛容器。 -->
<template>
  <div class="gv-topbar">
    <span class="gv-topbar__title">知识图谱</span>
    <nav v-if="!embedded" class="gv-breadcrumb">
      <span>知识库</span>
      <span class="gv-breadcrumb__sep">/</span>
      <span>{{ kbId }}</span>
      <span class="gv-breadcrumb__sep">/</span>
      <span class="gv-breadcrumb__cur">图谱</span>
    </nav>

    <div class="gv-topbar__right">
      <!-- 搜索实体 -->
      <div class="gv-search" @click="focusSearch">
        <svg class="gv-search__icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="11" cy="11" r="7"/><path d="M21 21l-4.3-4.3"/>
        </svg>
        <input
          ref="searchInputRef"
          v-model="searchText"
          class="gv-search__input"
          placeholder="搜索实体…"
          @keyup.enter="$emit('search')"
        />
      </div>

      <div class="gv-topbar__divider"></div>

      <!-- 添加节点 -->
      <button class="gv-btn-sec" @click="$emit('add-node')">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="4"/><path d="M12 2v4M12 18v4M2 12h4M18 12h4"/>
        </svg>
        添加节点
      </button>

      <!-- 添加关系 -->
      <button class="gv-btn-sec" @click="$emit('add-relation')">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M5 12h14M14 7l5 5-5 5"/>
        </svg>
        添加关系
      </button>

      <div class="gv-topbar__divider"></div>

      <!-- 导出 -->
      <button class="gv-icon-btn" title="导出" @click="$emit('export')">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
          <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
          <polyline points="7 10 12 15 17 10"/>
          <line x1="12" y1="15" x2="12" y2="3"/>
        </svg>
      </button>

      <!-- 全屏 -->
      <button class="gv-icon-btn" title="全屏" @click="$emit('toggle-fullscreen')">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
          <path d="M8 3H5a2 2 0 0 0-2 2v3M21 8V5a2 2 0 0 0-2-2h-3M3 16v3a2 2 0 0 0 2 2h3M16 21h3a2 2 0 0 0 2-2v-3"/>
        </svg>
      </button>

      <!-- 清空图谱 -->
      <button class="gv-icon-btn gv-icon-btn--danger" title="清空图谱" @click="$emit('clear-graph')">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
          <polyline points="3 6 5 6 21 6"/>
          <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
        </svg>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

defineProps<{
  embedded?: boolean
  kbId: string
}>()

defineEmits<{
  (e: 'search'): void
  (e: 'add-node'): void
  (e: 'add-relation'): void
  (e: 'export'): void
  (e: 'toggle-fullscreen'): void
  (e: 'clear-graph'): void
}>()

// 搜索关键字（与容器双向绑定）
const searchText = defineModel<string>('searchText', { default: '' })

const searchInputRef = ref<HTMLInputElement>()
function focusSearch() {
  searchInputRef.value?.focus()
}
</script>

<style scoped>
/* ===== 顶部工具栏 ===== */
.gv-topbar {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 0 20px;
  height: 54px;
  flex: 0 0 54px;
  /* 顶栏分隔线与 mockup 一致：发丝线 05% */
  border-bottom: 1px solid var(--color-border-subtle);
}

.gv-topbar__title {
  font-family: var(--font-display);
  font-size: 16px;
  font-weight: 700;
  color: var(--color-text-primary);
  white-space: nowrap;
}

.gv-breadcrumb {
  display: flex;
  align-items: center;
  gap: 5px;
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--color-text-muted);
}

.gv-breadcrumb__sep {
  color: var(--color-border-default);
}

.gv-breadcrumb__cur {
  color: var(--color-text-secondary);
}

.gv-topbar__right {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 8px;
}

/* 搜索框 */
.gv-search {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 6px 11px;
  border-radius: var(--radius-sm);
  background: var(--color-bg-primary);
  border: 1px solid var(--color-border-subtle);
  width: 180px;
  cursor: text;
  transition: border-color var(--duration-fast) var(--ease-default);
}

.gv-search:focus-within {
  border-color: var(--color-border-default);
}

.gv-search__icon {
  width: 13px;
  height: 13px;
  flex: 0 0 13px;
  color: var(--color-text-muted);
}

.gv-search__input {
  flex: 1;
  background: transparent;
  border: none;
  outline: none;
  font-size: 12px;
  font-family: var(--font-mono);
  color: var(--color-text-secondary);
  min-width: 0;
}

.gv-search__input::placeholder {
  color: var(--color-text-muted);
}

/* 顶栏分隔线 */
.gv-topbar__divider {
  width: 1px;
  height: 18px;
  background: var(--color-border-default);
  flex: 0 0 1px;
}

/* hairline 次级按钮（添加节点/关系）：border 对应 mockup var(--hair) = --border-default */
.gv-btn-sec {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 13px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--color-border-default);
  background: transparent;
  color: var(--color-text-secondary);
  font-size: 12px;
  cursor: pointer;
  white-space: nowrap;
  transition:
    color var(--duration-fast) var(--ease-default),
    border-color var(--duration-fast) var(--ease-default),
    background var(--duration-fast) var(--ease-default);
}

.gv-btn-sec svg {
  width: 13px;
  height: 13px;
}

.gv-btn-sec:hover {
  color: var(--color-text-primary);
  background: var(--color-bg-elevated);
  border-color: var(--color-border-default);
}

/* 图标按钮（30×30） */
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

/* ===== 清空图谱按钮（危险态） ===== */
.gv-icon-btn--danger:hover {
  color: var(--color-danger);
  background: rgba(192, 138, 138, 0.08);
}
</style>
