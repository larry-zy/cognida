<!--
  A2UIRenderer —— 生成式 UI（A2UI 子集）的入口渲染器。

  消费后端 `ui` SSE 事件下发的 UISpec：把扁平的 components 列表建成 id→节点
  映射，连同 dataModel 通过 provide 注入，再从 root 节点开始递归渲染。

  设计原则（数据按引用）：LLM/模板只产出「布局结构 + {path} 绑定」，所有数字
  都由 A2UINode 从 dataModel 里按 JSON Pointer 取值，前端不做任何数值加工，
  杜绝数字幻觉。genMode 仅用于角标展示（llm=AI 定制 / template=标准模板）。
-->
<template>
  <div v-if="rootId" class="a2ui">
    <div class="a2ui__bar">
      <span class="a2ui__title">{{ spec.title || '智能可视化' }}</span>
      <span :class="['a2ui__badge', `a2ui__badge--${spec.genMode || 'template'}`]">
        {{ spec.genMode === 'llm' ? 'AI 定制布局' : '标准模板' }}
      </span>
    </div>
    <!-- 过期降级占位：绑定超会话 TTL / result_id 过期时提示，快照数据仍可见 -->
    <UiAlert v-if="surfaceNotice" class="a2ui__notice" :type="surfaceNoticeType" :title="surfaceNotice" />
    <div class="a2ui__body" :class="{ 'a2ui__body--loading': pageLoading }">
      <A2UINode :id="rootId" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, provide, ref, watch } from 'vue'
import A2UINode from './A2UINode.vue'
import UiAlert from '@/components/ui/UiAlert.vue'
import { getUISurfacePage } from '@/api/agent'
import {
  A2UI_CTX,
  type A2UISpec,
  type A2UIComponent,
  type A2UIDataModel,
  type A2UIAction,
} from './a2ui-context'

const props = defineProps<{ spec: A2UISpec }>()

// 非分页交互（Button/Confirm/Form/Filter）向宿主视图抛出，由 Phase 4/5 流程消费。
const emit = defineEmits<{ action: [action: A2UIAction] }>()

const nodes = ref<Record<string, A2UIComponent>>({})
const dataModel = ref<A2UIDataModel>({})

// 会话/数据过期降级提示（"会话已过期" / "数据已过期，可重跑"）
const surfaceNotice = ref('')
const surfaceNoticeType = ref<'warning' | 'error'>('warning')
const pageLoading = ref(false)

// 把扁平组件列表索引成 id→节点，并暴露 dataModel 给递归节点。
watch(
  () => props.spec,
  (spec) => {
    const map: Record<string, A2UIComponent> = {}
    for (const c of spec?.components || []) map[c.id] = c
    nodes.value = map
    dataModel.value = spec?.dataModel || {}
    surfaceNotice.value = ''
  },
  { immediate: true, deep: true },
)

const rootId = computed(() => (nodes.value['root'] ? 'root' : ''))

// onAction：paginate 在此消化（按 surface+token 回源取页，不重跑查询），
// 其余动作补齐 surface 后向上抛。
function onAction(action: A2UIAction) {
  if (action.name === 'paginate') {
    void fetchPage(Number(action.params?.page) || 1, Number(action.params?.page_size) || 20)
    return
  }
  emit('action', { ...action, surface: props.spec?.surface })
}

// fetchPage：cursor 分页回源 Result Store；过期时展示降级占位（快照仍在）。
async function fetchPage(page: number, pageSize: number) {
  const surface = props.spec?.surface
  const token = dataModel.value?.meta?.surface_token
  if (!surface || !token) {
    // 无绑定（历史消息重现 / 绑定写失败降级）：仅有界快照可看，翻页不可用
    surfaceNotice.value = '会话已过期，仅显示数据快照'
    surfaceNoticeType.value = 'warning'
    return
  }
  pageLoading.value = true
  try {
    const res = await getUISurfacePage(surface, {
      token: String(token),
      cursor: (page - 1) * pageSize,
      page_size: pageSize,
    })
    if (res.status === 'session_expired') {
      surfaceNotice.value = res.message || '会话已过期'
      surfaceNoticeType.value = 'warning'
    } else if (res.status === 'data_expired') {
      surfaceNotice.value = res.message || '数据已过期，可重跑'
      surfaceNoticeType.value = 'error'
    } else if (res.status === 'ok' && Array.isArray(res.rows)) {
      // 用回源页替换表格数据；总行数用于分页组件的 total 绑定
      surfaceNotice.value = ''
      dataModel.value = {
        ...dataModel.value,
        table: { columns: res.columns || dataModel.value?.table?.columns || [], rows: res.rows },
        meta: { ...dataModel.value?.meta, row_count: res.row_count },
      }
    }
  } catch (e) {
    console.error('[A2UI] 分页回源失败:', e)
    surfaceNotice.value = '数据加载失败，请稍后重试'
    surfaceNoticeType.value = 'error'
  } finally {
    pageLoading.value = false
  }
}

provide(A2UI_CTX, { nodes, dataModel, onAction })
</script>

<style scoped>
.a2ui {
  border: 1px solid var(--color-border, #dee0e3);
  border-radius: 10px;
  overflow: hidden;
  background: var(--color-bg-container, #fff);
  margin-top: 8px;
}
.a2ui__bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  background: var(--color-bg-secondary, #f7f8fa);
  border-bottom: 1px solid var(--color-border, #dee0e3);
}
.a2ui__title {
  font-size: 13px;
  font-weight: 600;
  color: var(--color-text-primary, #1f2329);
}
.a2ui__badge {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 10px;
  font-weight: 500;
}
.a2ui__badge--llm {
  color: var(--color-primary, #3370ff);
  background: rgba(51, 112, 255, 0.1);
}
.a2ui__badge--template {
  color: var(--color-text-tertiary, #8f959e);
  background: var(--color-fill-secondary, #eef0f3);
}
.a2ui__notice {
  margin: 10px 12px 0;
}
.a2ui__body {
  padding: 14px;
}
.a2ui__body--loading {
  opacity: 0.55;
  pointer-events: none;
}
</style>
