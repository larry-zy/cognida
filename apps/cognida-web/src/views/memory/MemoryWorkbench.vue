<template>
  <div class="workbench memory-workbench">
    <header class="wb-head">
      <div>
        <span class="wb-kicker">PERSONAL CONTEXT</span>
        <h1>我的长期记忆</h1>
        <p>保存事实与偏好，让新的对话从这里接续。你可以随时修正或遗忘。</p>
      </div>
      <button class="primary" :disabled="busy" @click="newMemory">＋ 新建记忆</button>
    </header>
    <div class="memory-boundary"><span class="memory-dot" />仅当前租户下的你可见 · 只保存你明确提交的内容</div>
    <div v-if="error" role="alert" class="wb-error">{{ error }}</div>
    <div v-if="notice" role="status" class="wb-success">{{ notice }}</div>
    <div class="wb-grid memory-grid">
      <aside class="wb-panel">
        <form class="memory-filters" aria-label="搜索记忆" @submit.prevent="run(() => load())">
          <label>记忆类别<select v-model="category" aria-label="筛选类别" @change="run(() => load())"><option value="">全部</option><option value="preference">我的偏好</option><option value="fact">事实与背景</option></select></label>
          <label>查找内容<input v-model="query" aria-label="查找记忆" maxlength="200" placeholder="输入关键词" /></label>
          <button :disabled="busy">查找</button>
        </form>
        <div class="wb-list memory-list" aria-label="记忆列表">
          <button v-for="memory in memories" :key="memory.id" :class="{ selected: current?.id === memory.id }" :disabled="busy" @click="run(() => open(memory.id))">
            <span class="memory-category">{{ labels[memory.category] }}</span>
            <strong>{{ memory.content }}</strong>
            <small>{{ formatDate(memory.updated_at) }} · v{{ memory.version }}</small>
          </button>
        </div>
        <p v-if="!memories.length" class="wb-empty">{{ busy ? '正在加载…' : '还没有匹配的记忆。从一条偏好或背景信息开始。' }}</p>
        <button v-if="nextCursor" :disabled="busy" @click="run(() => load(true))">加载更多</button>
      </aside>
      <section class="memory-editor">
        <form class="wb-panel" aria-label="编辑记忆" @submit.prevent="run(save)">
          <div class="wb-section-head memory-section-head"><h2>{{ current ? '编辑记忆' : '添加一条记忆' }}</h2><span v-if="current" class="wb-badge">v{{ current.version }}</span></div>
          <p class="memory-hint">记录你希望助手在以后对话中参考的信息。</p>
          <div class="wb-form">
            <label>类别<select v-model="form.category" aria-label="记忆类别" :disabled="busy"><option value="preference">我的偏好</option><option value="fact">事实与背景</option></select></label>
            <label>优先程度<select v-model.number="form.importance" aria-label="优先程度" :disabled="busy"><option :value="0.3">一般</option><option :value="0.6">重要</option><option :value="1">优先参考</option></select></label>
          </div>
          <label class="memory-content-label">内容<textarea v-model="form.content" aria-label="记忆内容" required maxlength="4000" rows="7" :disabled="busy" placeholder="例如：回答时先给结论，再展开说明。" /></label>
          <div class="memory-content-footer"><small>{{ [...form.content].length }} / 4000</small><small v-if="current">来源：{{ current.source === 'tool' ? '对话中的明确保存' : '管理页面' }}</small></div>
          <div class="wb-actions memory-actions">
            <button class="primary" :disabled="busy || !form.content.trim()">{{ current ? '保存修改' : '保存记忆' }}</button>
            <button v-if="current" type="button" :disabled="busy" @click="run(() => open(current!.id))">重新读取</button>
            <button v-if="current" type="button" class="memory-forget" :disabled="busy" @click="confirmDelete = !confirmDelete">遗忘这条</button>
          </div>
          <div v-if="confirmDelete && current" class="memory-delete-confirm" role="alertdialog" aria-label="确认遗忘">
            <p>这条记忆将被删除，后续对话不再召回。</p><button type="button" class="memory-forget" :disabled="busy" @click="run(forget)">确认遗忘</button><button type="button" @click="confirmDelete = false">保留</button>
          </div>
        </form>
        <section class="wb-panel memory-recall" aria-label="召回预览">
          <h2>在一个新问题里试试</h2><p class="memory-hint">查看哪些已保存资料会与问题相关。相关程度不代表内容已核实。</p>
          <form class="memory-recall-form" @submit.prevent="run(recall)"><input v-model="recallQuery" aria-label="召回问题" required maxlength="2000" placeholder="例如：我习惯怎样的回答方式？" /><button :disabled="busy || !recallQuery.trim()">预览召回</button></form>
          <template v-if="recallResult">
            <p class="memory-hint">{{ recallResult.method === 'lexical' ? '关键词匹配' : '关键词与向量相似度' }} · 最多检查最近更新的 {{ recallResult.candidate_limit }} 条</p>
            <p v-if="recallResult.truncated" class="memory-hint">记忆较多，本次只检索了有界候选范围。</p>
            <p v-if="!recallResult.items.length" class="wb-empty">没有相关记忆。可以换一个更具体的问题。</p>
            <article v-for="item in recallResult.items" :key="item.memory.id" class="memory-match"><span class="memory-category">{{ labels[item.memory.category] }}</span><p>{{ item.memory.content }}</p><button :disabled="busy" @click="run(() => open(item.memory.id))">查看与修正</button></article>
          </template>
        </section>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { memoryApi, type MemoryInput, type PersonalMemory, type MemoryRecall } from '@/api/memory'
import { useAuthStore } from '@/stores/auth'
import '@/components/workbench/workbench.css'

const auth = useAuthStore()
const labels = { fact: '事实与背景', preference: '我的偏好' }
const fresh = (): MemoryInput => ({ category: 'preference', content: '', importance: 0.6 })
const form = reactive(fresh())
const memories = ref<PersonalMemory[]>([]), current = ref<PersonalMemory | null>(null)
const category = ref(''), query = ref(''), nextCursor = ref(''), recallQuery = ref('')
const recallResult = ref<MemoryRecall | null>(null), error = ref(''), notice = ref('')
const confirmDelete = ref(false), pending = ref(0), busy = computed(() => pending.value > 0)
let scopeEpoch = 0, listEpoch = 0, editorEpoch = 0, recallEpoch = 0
let createAttempt: { fingerprint: string; key: string } | null = null
function formatDate(value: string) { return new Date(value).toLocaleDateString('zh-CN') }
async function run(action: () => Promise<void>) {
  const epoch = scopeEpoch
  pending.value++; error.value = ''; notice.value = ''
  try { await action() } catch (e) { if (epoch === scopeEpoch) error.value = e instanceof Error ? e.message : '操作失败，请重试' }
  finally { if (epoch === scopeEpoch) pending.value-- }
}
async function load(append = false) {
  const epoch = scopeEpoch, request = ++listEpoch
  const res = await memoryApi.list({ category: category.value, q: query.value, cursor: append ? nextCursor.value : '', page_size: 20 })
  if (epoch !== scopeEpoch || request !== listEpoch) return
  memories.value = append ? [...memories.value, ...(res.data?.items || [])] : res.data?.items || []
  nextCursor.value = res.data?.next_cursor || ''
}
function newMemory() { editorEpoch++; current.value = null; Object.assign(form, fresh()); confirmDelete.value = false; createAttempt = null }
async function open(id: string) {
  const epoch = scopeEpoch, request = ++editorEpoch
  const res = await memoryApi.get(id)
  if (epoch !== scopeEpoch || request !== editorEpoch || !res.data) return
  current.value = res.data; Object.assign(form, { content: res.data.content, category: res.data.category, importance: res.data.importance }); confirmDelete.value = false; createAttempt = null
}
async function save() {
  const epoch = scopeEpoch, request = editorEpoch, value = { ...form, content: form.content.trim() }
  let res
  if (current.value) res = await memoryApi.update(current.value.id, value, current.value.version)
  else {
    const fingerprint = JSON.stringify(value)
    if (!createAttempt || createAttempt.fingerprint !== fingerprint) createAttempt = { fingerprint, key: crypto.randomUUID() }
    res = await memoryApi.create(value, createAttempt.key)
  }
  if (epoch !== scopeEpoch || request !== editorEpoch || !res.data) return
  current.value = res.data; Object.assign(form, value); createAttempt = null; recallResult.value = null; recallEpoch++
  notice.value = '记忆已保存，新对话会使用最新内容。'; await load()
}
async function forget() {
  if (!current.value) return
  const epoch = scopeEpoch, request = editorEpoch
  await memoryApi.remove(current.value.id, current.value.version)
  if (epoch !== scopeEpoch || request !== editorEpoch) return
  newMemory(); recallResult.value = null; recallEpoch++; notice.value = '已遗忘这条记忆。'; await load()
}
async function recall() {
  const epoch = scopeEpoch, request = ++recallEpoch
  const res = await memoryApi.recall(recallQuery.value.trim())
  if (epoch !== scopeEpoch || request !== recallEpoch) return
  recallResult.value = res.data || null
}
watch([() => auth.authEpoch, () => auth.user?.id, () => auth.currentTenant?.id], () => {
  scopeEpoch++; listEpoch++; recallEpoch++; pending.value = 0; newMemory()
  memories.value = []; nextCursor.value = ''; recallResult.value = null; recallQuery.value = ''; query.value = ''; category.value = ''; error.value = ''; notice.value = ''
  void run(() => load())
}, { immediate: true })
onBeforeUnmount(() => { scopeEpoch++; listEpoch++; editorEpoch++; recallEpoch++ })
</script>

<style scoped>
.memory-boundary { display:flex; align-items:center; gap:9px; color:var(--text-secondary,#617068); font-size:12px; margin:-8px 0 24px; }
.memory-dot { width:6px; height:6px; background:#588067; border-radius:50%; }
.memory-grid { grid-template-columns:minmax(260px,0.85fr) minmax(0,1.75fr); align-items:start; }
.memory-filters { display:grid; gap:12px; }
.memory-filters label,.memory-content-label { display:grid; gap:7px; color:var(--text-secondary,#59645f); font-size:12px; }
.memory-list { margin-top:24px; }
.memory-list strong { white-space:pre-wrap; display:-webkit-box; -webkit-line-clamp:3; -webkit-box-orient:vertical; overflow:hidden; font-weight:500; line-height:1.65; }
.memory-category { display:block; font-size:11px; color:#69816f; letter-spacing:.04em; margin-bottom:5px; }
.memory-editor { display:grid; gap:20px; min-width:0; }
.memory-section-head { margin-top:0; }
.memory-hint { color:var(--text-secondary,#68756e); font-size:12px; line-height:1.7; margin:10px 0 18px; }
.memory-content-label { margin-top:18px; }
.memory-content-label textarea { width:100%; resize:vertical; min-height:160px; font:inherit; line-height:1.8; box-sizing:border-box; }
.memory-content-footer { display:flex; justify-content:space-between; gap:12px; margin-top:8px; color:var(--text-secondary,#7a827d); font-size:11px; }
.memory-actions { margin-top:24px; flex-wrap:wrap; }
.memory-forget { color:#a3493d; }
.memory-delete-confirm { padding:14px; background:rgba(166,73,61,.06); margin-top:16px; font-size:12px; }
.memory-delete-confirm button { margin-right:8px; }
.memory-recall-form { display:flex; gap:10px; }
.memory-recall-form input { flex:1; min-width:0; }
.memory-match { border-top:1px solid var(--border-color,#e4e9e5); padding:16px 0; }
.memory-match p { white-space:pre-wrap; overflow-wrap:anywhere; font-size:13px; line-height:1.75; }
@media(max-width:800px) { .memory-grid { grid-template-columns:1fr; } .memory-recall-form { flex-direction:column; } }
</style>
