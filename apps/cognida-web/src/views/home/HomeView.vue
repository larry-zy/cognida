<template>
  <div class="workbench overview">
    <header class="wb-head overview-head">
      <div>
        <span class="wb-kicker">WORKSPACE / 工作空间</span>
        <h1>工作总览</h1>
        <p>从采集到发布，接着处理手上的工作。</p>
      </div>
      <div class="overview-refresh">
        <span v-if="updatedAt" class="wb-muted">更新于 {{ formatTime(updatedAt) }}</span>
        <button :disabled="loading || !hasIdentity" @click="refresh">{{ loading ? '正在更新…' : '刷新总览' }}</button>
      </div>
    </header>

    <div v-if="!hasIdentity" class="overview-empty" role="status">请登录并选择工作空间后查看任务。</div>
    <template v-else>
      <nav class="flow-path" aria-label="数据工作流程">
        <router-link v-for="step in flowSteps" :key="step.path" :to="step.path">
          <span class="flow-path__number">{{ step.number }}</span>
          <span><strong>{{ step.title }}</strong><small>{{ step.description }}</small></span>
          <span aria-hidden="true" class="flow-path__arrow">↗</span>
        </router-link>
      </nav>

      <section class="source-status" aria-label="数据读取状态">
        <article v-for="section in sections" :key="section.key" :data-section="section.key" :aria-busy="section.state.status === 'loading'">
          <div class="source-status__head"><h2>{{ section.title }}</h2><router-link :to="section.path" :aria-label="`打开${section.title}`">{{ section.state.hasMore && section.key !== 'collection' ? '完整列表 ↗' : '打开 ↗' }}</router-link></div>
          <template v-if="section.state.status === 'ready'">
            <p class="source-status__count">{{ section.state.items.length }}<span>条已读取</span></p>
            <p class="source-status__note">{{ section.range }}<span v-if="section.state.hasMore"> · {{ section.key === 'collection' ? '已达读取上限' : '还有更多' }}</span></p>
          </template>
          <p v-else-if="section.state.status === 'loading'" class="source-status__message" role="status">正在读取{{ section.title }}…</p>
          <div v-else-if="section.state.status === 'error'" class="source-status__error" role="alert">
            <p>{{ section.state.error }}</p><button @click="retry(section.key)">重试{{ section.title }}</button>
          </div>
        </article>
      </section>

      <div class="overview-columns">
        <section class="wb-panel attention-panel" aria-labelledby="attention-title">
          <div class="panel-heading"><div><span class="wb-kicker">NEXT ACTION</span><h2 id="attention-title">优先处理</h2></div><span v-if="attention.length" class="count-label">{{ attention.length }} 项</span></div>
          <p class="panel-description">根据本页已读取的任务筛选。进入详情核对内容后，再审批或重试。</p>
          <p v-if="workLoading && !attention.length" class="overview-empty" role="status">正在检查本页任务…</p>
          <div v-else-if="!attention.length" class="overview-empty">
            <strong>{{ workHasError ? '部分任务暂时无法读取' : '当前读取范围内没有待处理事项' }}</strong>
            <p>{{ workHasError ? '请在上方重试失败的模块，其他已读取内容仍可查看。' : '你可以开始新的采集，或查看已有资产。' }}</p>
            <router-link v-if="!workHasError" to="/collection-tasks" class="inline-action">前往网页采集 →</router-link>
          </div>
          <ul v-else class="attention-list" aria-label="待处理任务">
            <li v-for="item in attention.slice(0, 8)" :key="item.key">
              <span class="attention-dot" :class="`tone-${item.tone}`" aria-hidden="true"></span>
              <div class="attention-list__body"><span class="item-eyebrow">{{ item.category }} · {{ item.statusLabel }}</span><h3>{{ item.title }}</h3><p>{{ item.reason }}</p></div>
              <router-link :to="item.to" class="inline-action">{{ item.action }} →</router-link>
            </li>
          </ul>
          <p v-if="attention.length > 8" class="wb-muted">另有 {{ attention.length - 8 }} 项，请在对应任务列表继续查看。</p>
          <p v-if="workHasError && attention.length" class="wb-muted">部分模块未读取成功，待处理列表可能不完整。</p>
        </section>

        <section class="wb-panel asset-panel" aria-labelledby="assets-title">
          <div class="panel-heading"><div><span class="wb-kicker">READY TO USE</span><h2 id="assets-title">已发布资产</h2></div><router-link to="/analysis" class="inline-action">资产分析 ↗</router-link></div>
          <p class="panel-description">查看版本和发布回执，再进入分析。</p>
          <p v-if="assets.status === 'loading'" class="overview-empty" role="status">正在读取发布资产…</p>
          <p v-else-if="assets.status === 'error'" class="overview-empty">发布资产暂不可用，请在上方重试。</p>
          <div v-else-if="!assets.items.length" class="overview-empty"><strong>还没有可查看的发布资产</strong><p>完成处理流程并批准发布后，资产会出现在这里。</p><router-link to="/pipelines" class="inline-action">前往处理与发布 →</router-link></div>
          <ul v-else class="asset-list" aria-label="已发布资产">
            <li v-for="asset in assets.items.slice(0, 5)" :key="`${asset.asset_id}:${asset.version}`">
              <router-link :to="{ path: '/analysis', query: { asset: `${asset.asset_id}:${asset.version}` } }">
                <span class="asset-list__icon" aria-hidden="true">↗</span><span><strong>{{ asset.name || shortId(asset.asset_id) }}</strong><small>版本 {{ asset.version }} · {{ formatTime(asset.created_at) }}</small></span>
              </router-link>
            </li>
          </ul>
          <div class="asset-destinations"><router-link to="/knowledge">查看知识库 →</router-link><router-link to="/subscriptions">报告与看护 →</router-link></div>
        </section>
      </div>

      <section class="wb-panel recent-panel" aria-labelledby="recent-title">
        <div class="panel-heading"><div><span class="wb-kicker">WORK RECORDS</span><h2 id="recent-title">任务与运行</h2></div><span class="wb-muted">本页已读取记录 · 按更新时间排列 · 显示前 12 条</span></div>
        <p v-if="workLoading && !recent.length" class="overview-empty" role="status">正在读取任务记录…</p>
        <p v-else-if="!recent.length" class="overview-empty">{{ workHasError ? '部分记录读取失败，请重试对应模块。' : '还没有任务记录。从上方选择采集或处理入口开始。' }}</p>
        <ul v-else class="recent-list" aria-label="任务与运行">
          <li v-for="item in recent.slice(0, 12)" :key="item.key">
            <span class="item-category">{{ item.category }}</span><div class="recent-list__name"><strong>{{ item.title }}</strong><small>{{ shortId(item.id) }}</small></div>
            <span class="status-label" :class="`tone-${item.tone}`">{{ item.statusLabel }}</span><time :datetime="item.time">{{ formatTime(item.time) }}</time><router-link :to="item.to" class="inline-action" :aria-label="`查看${item.category} ${item.id}`">查看详情 →</router-link>
          </li>
        </ul>
      </section>
      <p class="overview-scope">仅展示当前账号在当前工作空间有权读取的记录。数量来自本页读取范围，不代表历史总量。</p>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import type { RouteLocationRaw } from 'vue-router'
import { overviewApi, type OverviewPage } from '@/api/overview'
import type { AssetRef, Invocation } from '@/api/lifecycle'
import type { CollectionTask } from '@/api/collection'
import { useAuthStore } from '@/stores/auth'

interface Section<T> { items: T[]; status: 'loading' | 'ready' | 'error'; error: string; hasMore: boolean }
type SectionKey = 'runs' | 'invocations' | 'collection' | 'assets'
interface Activity { needsAttention: boolean; key: string; id: string; category: string; title: string; status: string; statusLabel: string; tone: string; reason: string; action: string; time: string; to: RouteLocationRaw }
const makeSection = <T,>(): Section<T> => ({ items: [], status: 'loading', error: '', hasMore: false })
const runs = reactive(makeSection<Invocation>()), invocations = reactive(makeSection<Invocation>()), collection = reactive(makeSection<CollectionTask>()), assets = reactive(makeSection<AssetRef>())
const state = { runs, invocations, collection, assets }
const auth = useAuthStore()
const hasIdentity = computed(() => Boolean(auth.user?.id && auth.currentTenant?.id))
const updatedAt = ref('')
const sections = computed(() => [
  { key: 'runs' as const, title: '处理流程', range: '本页最多读取 50 次运行', path: '/pipelines', state: runs },
  { key: 'invocations' as const, title: '专家任务', range: '本页最多读取 50 次调用', path: '/experts', state: invocations },
  { key: 'collection' as const, title: '网页采集', range: '最近最多 100 项任务', path: '/collection-tasks', state: collection },
  { key: 'assets' as const, title: '已发布资产', range: '本页最多读取 20 个资产版本', path: '/analysis', state: assets }
])
const loading = computed(() => hasIdentity.value && sections.value.some(section => section.state.status === 'loading'))
const workLoading = computed(() => [runs, invocations, collection].some(section => section.status === 'loading'))
const workHasError = computed(() => [runs, invocations, collection].some(section => section.status === 'error'))
const flowSteps = [
  { number: '01', title: '采集来源', description: '网页与数据连接', path: '/collection-tasks' },
  { number: '02', title: '处理与审批', description: '质量、清洗与发布', path: '/pipelines' },
  { number: '03', title: '知识检索', description: '查看引用与原文', path: '/kb-assistant' },
  { number: '04', title: '报告与看护', description: '分析资产、跟踪变化', path: '/subscriptions' }
]
const statusLabels: Record<string, string> = { queued: '排队中', running: '执行中', waiting: '等待中', awaiting_approval: '待审批', succeeded: '已完成', failed: '失败', cancelled: '已取消', quarantined: '质量未通过', draft: '待启动', completed: '采集已结束' }
function tone(status: string) { return ['failed', 'quarantined'].includes(status) ? 'danger' : ['awaiting_approval', 'draft'].includes(status) ? 'warning' : ['succeeded', 'completed'].includes(status) ? 'success' : 'neutral' }
function shortId(id: string) { return id.length > 20 ? `${id.slice(0, 12)}…${id.slice(-4)}` : id }
function formatTime(value: string) { const d = new Date(value); return Number.isNaN(d.getTime()) ? '时间未提供' : d.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }) }
function lifecycleActivity(item: Invocation, kind: 'runs' | 'invocations'): Activity {
  const approval = item.status === 'awaiting_approval'
  return { needsAttention: ['awaiting_approval', 'failed', 'quarantined'].includes(item.status), key: `${kind}:${item.id}`, id: item.id, category: kind === 'runs' ? '处理流程' : '专家任务', title: item.template_id || item.agent_id || shortId(item.id), status: item.status, statusLabel: statusLabels[item.status] || item.status, tone: tone(item.status), reason: item.error || (approval ? '需要人工核对发布目标与证据，审批后才能继续。' : item.status === 'quarantined' ? '质量门禁未通过，查看问题后调整输入或处理参数。' : '查看运行详情，确认失败原因和可重试的步骤。'), action: approval ? '前往审批' : '查看原因', time: item.updated_at || item.created_at, to: { path: kind === 'runs' ? '/pipelines' : '/experts', query: { [kind === 'runs' ? 'run_id' : 'invocation_id']: item.id } } }
}
function collectionActivity(item: CollectionTask): Activity {
  // A completed collection may contain failed sources; the backend carries its
  // partial-outcome explanation on error_message while retaining completed.
  const partial = item.status === 'completed' && Boolean(item.error_message?.trim())
  return { needsAttention: partial || ['failed', 'draft'].includes(item.status), key: `collection:${item.id}`, id: item.id, category: '网页采集', title: `采集任务 ${shortId(item.id)}`, status: item.status, statusLabel: partial ? '采集部分失败' : statusLabels[item.status] || item.status, tone: partial ? 'warning' : tone(item.status), reason: item.error_message || (item.status === 'draft' ? '来源已保存，核对目标知识库后开始采集。' : '打开详情查看每个来源的结果与失败原因。'), action: partial ? '查看来源' : item.status === 'draft' ? '继续采集' : '查看原因', time: item.updated_at || item.created_at || '', to: { path: '/collection-tasks', query: { task_id: item.id } } }
}
const recent = computed<Activity[]>(() => [
  ...(runs.status === 'ready' ? runs.items.map(item => lifecycleActivity(item, 'runs')) : []),
  ...(invocations.status === 'ready' ? invocations.items.map(item => lifecycleActivity(item, 'invocations')) : []),
  ...(collection.status === 'ready' ? collection.items.map(collectionActivity) : [])
].sort((a, b) => (Date.parse(b.time) || 0) - (Date.parse(a.time) || 0)))
const attention = computed(() => recent.value.filter(item => item.needsAttention).sort((a, b) => Number(b.status === 'awaiting_approval') - Number(a.status === 'awaiting_approval')))
let generation = 0, disposed = false
const controllers = new Map<SectionKey, AbortController>()
function errorMessage(error: unknown): string {
  const status = (error as { response?: { status?: number } })?.response?.status
  if (status === 403) return '没有查看此模块的权限，请联系工作空间管理员。'
  if (status === 401) return '登录已失效，请重新登录。'
  return error instanceof Error ? error.message : '读取失败，请稍后重试。'
}
async function load<T>(key: SectionKey, target: Section<T>, reader: (signal: AbortSignal) => Promise<OverviewPage<T>>) {
  controllers.get(key)?.abort()
  const controller = new AbortController(), requestGeneration = generation
  controllers.set(key, controller)
  Object.assign(target, makeSection<T>())
  try {
    const page = await reader(controller.signal)
    if (disposed || requestGeneration !== generation || controllers.get(key) !== controller) return
    Object.assign(target, { items: page.items, hasMore: page.hasMore, status: 'ready' })
    updatedAt.value = new Date().toISOString()
  } catch (error) {
    if (disposed || requestGeneration !== generation || controllers.get(key) !== controller || controller.signal.aborted) return
    target.status = 'error'; target.error = errorMessage(error)
  } finally { if (controllers.get(key) === controller) controllers.delete(key) }
}
function retry(key: SectionKey) {
  if (!hasIdentity.value) return Promise.resolve()
  switch (key) {
    case 'runs': return load(key, runs, overviewApi.runs)
    case 'invocations': return load(key, invocations, overviewApi.invocations)
    case 'collection': return load(key, collection, overviewApi.collection)
    case 'assets': return load(key, assets, overviewApi.assets)
  }
}
async function refresh() { await Promise.all((Object.keys(state) as SectionKey[]).map(retry)) }
watch(() => [auth.authEpoch, auth.user?.id, auth.currentTenant?.id], () => {
  generation++; controllers.forEach(controller => controller.abort()); controllers.clear(); updatedAt.value = ''
  Object.values(state).forEach(section => Object.assign(section, makeSection()))
  if (hasIdentity.value) void refresh()
}, { immediate: true })
onBeforeUnmount(() => { disposed = true; generation++; controllers.forEach(controller => controller.abort()); controllers.clear() })
</script>

<style scoped>
.overview { max-width: 1440px; }
.overview-head h1 { font-size: 32px; }
.overview-refresh { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.flow-path { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); border: 1px solid var(--border-default); border-radius: 9px; overflow: hidden; margin-bottom: 24px; background: var(--bg-tertiary); }
.flow-path a { display: flex; align-items: center; gap: 12px; padding: 19px 18px; color: var(--text-primary); text-decoration: none; border-right: 1px solid var(--border-default); }
.flow-path a:last-child { border-right: 0; }
.flow-path a:hover { background: var(--primary-dim); }
.flow-path__number { font-family: var(--font-mono); font-size: 12px; color: var(--primary); }
.flow-path strong { display: block; font-size: 14px; }
.flow-path small { display: block; color: var(--text-muted); font-size: 11px; margin-top: 6px; }
.flow-path__arrow { margin-left: auto; color: var(--text-muted); }
.source-status { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 16px; margin-bottom: 24px; }
.source-status article { border-top: 2px solid var(--border-default); padding: 15px 2px 0; min-width: 0; }
.source-status__head { display: flex; justify-content: space-between; gap: 10px; }
.source-status h2 { margin: 0; color: var(--text-secondary); font-size: 12px; font-weight: 500; }
.source-status__head a { color: var(--primary); font-size: 11px; text-decoration: none; }
.source-status__count { font-family: var(--font-display); font-size: 28px; margin: 10px 0 3px; }
.source-status__count span { font-family: var(--font-body); font-size: 11px; color: var(--text-muted); margin-left: 8px; }
.source-status__note, .source-status__message, .source-status__error { font-size: 11px; color: var(--text-muted); line-height: 1.6; }
.source-status__message { min-height: 54px; }
.source-status__error { color: var(--danger); }
.source-status__error button { padding: 5px 8px; font-size: 11px; }
.overview-columns { display: grid; grid-template-columns: minmax(0, 1.6fr) minmax(260px, 1fr); gap: 20px; margin-bottom: 20px; }
.panel-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.panel-heading h2 { margin: 6px 0 0; font-family: var(--font-display); font-size: 20px; }
.panel-heading .wb-kicker { font-size: 9px; }
.count-label { color: var(--primary); font-size: 12px; padding: 5px 8px; background: var(--primary-dim); border-radius: 4px; }
.panel-description { font-size: 12px; color: var(--text-muted); line-height: 1.7; margin: 16px 0 8px; }
.overview-empty { padding: 26px 0; color: var(--text-muted); font-size: 13px; line-height: 1.8; }
.overview-empty strong { color: var(--text-secondary); font-weight: 500; }
.overview-empty p { margin: 6px 0 12px; }
.attention-list, .asset-list, .recent-list { list-style: none; padding: 0; margin: 0; }
.attention-list li { display: flex; align-items: center; gap: 12px; padding: 18px 0; border-bottom: 1px solid var(--border-subtle); }
.attention-list li:last-child { border-bottom: 0; }
.attention-dot { width: 7px; height: 7px; flex-shrink: 0; border-radius: 50%; background: currentColor; }
.attention-list__body { min-width: 0; flex: 1; }
.item-eyebrow { font-size: 10px; color: var(--text-muted); }
.attention-list h3 { font-size: 13px; margin: 5px 0; overflow-wrap: anywhere; }
.attention-list p { font-size: 11px; color: var(--text-muted); line-height: 1.7; margin: 0; overflow-wrap: anywhere; }
.inline-action { font-size: 12px; white-space: nowrap; color: var(--primary); text-decoration: none; }
.inline-action:hover, .asset-destinations a:hover { text-decoration: underline; }
.tone-danger { color: var(--danger); }.tone-warning { color: var(--warning, #b38336); }.tone-success { color: var(--success, #4b8f77); }.tone-neutral { color: var(--text-muted); }
.asset-list li a { display: flex; gap: 12px; align-items: center; padding: 18px 0; color: var(--text-primary); text-decoration: none; border-bottom: 1px solid var(--border-subtle); }
.asset-list strong { display: block; font-size: 13px; font-weight: 500; overflow-wrap: anywhere; }
.asset-list small { display: block; font-size: 11px; margin-top: 6px; color: var(--text-muted); }
.asset-list__icon { color: var(--primary); padding: 9px; background: var(--primary-dim); border-radius: 5px; }
.asset-destinations { display: flex; flex-wrap: wrap; gap: 18px; margin-top: 22px; }
.asset-destinations a { color: var(--primary); text-decoration: none; font-size: 12px; }
.recent-list { margin-top: 18px; }
.recent-list li { display: grid; grid-template-columns: 80px minmax(0, 1fr) 90px 110px 80px; align-items: center; gap: 16px; padding: 15px 0; border-bottom: 1px solid var(--border-subtle); }
.recent-list li:last-child { border-bottom: 0; }
.item-category { font-size: 11px; color: var(--text-muted); }
.recent-list__name strong { font-size: 12px; display: block; font-weight: 500; overflow-wrap: anywhere; }.recent-list__name small { font-family: var(--font-mono); font-size: 10px; color: var(--text-muted); display: block; margin-top: 5px; }
.status-label, .recent-list time { font-size: 11px; }.recent-list time { color: var(--text-muted); }
.overview-scope { font-size: 11px; color: var(--text-muted); line-height: 1.8; margin: 18px 0 0; }
a:focus-visible, button:focus-visible { outline: 2px solid var(--primary); outline-offset: 4px; }
@media (max-width: 1100px) { .flow-path { grid-template-columns: repeat(2, minmax(0, 1fr)); }.flow-path a:nth-child(2) { border-right: 0; }.flow-path a:nth-child(-n+2) { border-bottom: 1px solid var(--border-default); }.overview-columns { grid-template-columns: 1fr; } }
@media (max-width: 640px) { .overview-head { flex-direction: column; align-items: flex-start; gap: 8px; }.source-status { grid-template-columns: repeat(2, minmax(0, 1fr)); }.flow-path a { padding: 14px 10px; gap: 8px; }.flow-path small { font-size: 10px; }.flow-path__arrow { display: none; }.wb-panel { padding: 16px; }.attention-list li { flex-wrap: wrap; }.attention-list .inline-action { margin-left: 19px; }.recent-list li { grid-template-columns: 1fr auto; gap: 9px; }.item-category, .recent-list time { display: none; }.recent-list__name { grid-column: 1; }.recent-list .inline-action { grid-column: 2; }.panel-heading { flex-wrap: wrap; }.panel-heading > .wb-muted { font-size: 10px; } }
</style>
