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
    <div class="a2ui__body">
      <A2UINode :id="rootId" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, provide, ref, watch } from 'vue'
import A2UINode from './A2UINode.vue'
import {
  A2UI_CTX,
  type A2UISpec,
  type A2UIComponent,
  type A2UIDataModel,
} from './a2ui-context'

const props = defineProps<{ spec: A2UISpec }>()

const nodes = ref<Record<string, A2UIComponent>>({})
const dataModel = ref<A2UIDataModel>({})

// 把扁平组件列表索引成 id→节点，并暴露 dataModel 给递归节点。
watch(
  () => props.spec,
  (spec) => {
    const map: Record<string, A2UIComponent> = {}
    for (const c of spec?.components || []) map[c.id] = c
    nodes.value = map
    dataModel.value = spec?.dataModel || {}
  },
  { immediate: true, deep: true },
)

const rootId = computed(() => (nodes.value['root'] ? 'root' : ''))

provide(A2UI_CTX, { nodes, dataModel })
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
.a2ui__body {
  padding: 14px;
}
</style>
