<template>
  <main class="research-page">
    <header class="research-heading">
      <div><div class="eyebrow">RESEARCH / 研究工作台</div><h1>从问题走向证据</h1><p>拆解复杂问题，检索授权来源，形成可追溯的研究报告。</p></div>
      <span class="research-mark" aria-hidden="true">[ R ]</span>
    </header>
    <div class="research-layout">
      <section class="research-brief" aria-labelledby="brief-title">
        <h2 id="brief-title"><span>01</span> 定义研究</h2>
        <label for="research-question">你想研究什么？</label>
        <textarea id="research-question" v-model="query" maxlength="4000" rows="6" :disabled="running" placeholder="例如：比较两种供应链方案的成本、风险和落地条件，并指出仍需补充的证据。" />
        <div class="field-hint">{{ query.length }} / 4000</div>
        <label for="research-kbs">知识库范围</label>
        <el-select id="research-kbs" v-model="kbIds" multiple collapse-tags collapse-tags-tooltip placeholder="选择本次明确授权范围" :disabled="running || loadingKBs" aria-label="知识库范围">
          <el-option v-for="kb in knowledgeBases" :key="kb.id" :value="kb.id" :label="kb.name" />
        </el-select>
        <button v-if="hasMoreKBs" class="inline-button" :disabled="loadingKBs || running" @click="loadKnowledgeBases(true)">继续读取知识库</button>
        <p class="scope-note">至少选择一个知识库；保存后范围固定，补充检索也不会扩大范围。</p>
        <label class="toggle-row"><input v-model="graphEnabled" type="checkbox" :disabled="running" /><span>加入图谱关联证据<small>仅使用所选范围中有原文出处的关系</small></span></label>
        <label class="toggle-row"><input v-model="webEnabled" type="checkbox" :disabled="running" /><span>补充网络搜索<small>需管理员配置可用的搜索服务</small></span></label>
        <div class="research-bounds"><span>最多 3 个子问题</span><span>最长 2 分钟</span></div>
        <button v-if="!running" class="start-button" :disabled="!query.trim() || !kbIds.length || loadingKBs || !!kbError || submitting" @click="start">{{ submitting ? '正在保存…' : '开始研究' }} <span aria-hidden="true">↗</span></button>
        <button v-else class="cancel-button" :disabled="cancelling || run?.status === 'cancel_requested'" @click="cancel">{{ run?.status === 'cancel_requested' ? '已申请取消，等待确认' : cancelling ? '正在申请…' : '取消研究' }}</button>
        <p v-if="kbError" class="error" role="alert">{{ kbError }} <button class="inline-button" @click="loadKnowledgeBases()">重试</button></p>
      </section>
      <section class="research-output" aria-labelledby="report-title" aria-live="polite" :aria-busy="running">
        <header class="output-heading"><h2 id="report-title"><span>02</span> 研究记录</h2><span v-if="run" class="report-state">{{ stateLabel }}</span></header>
        <p v-if="run" class="run-receipt">研究记录 {{ run.id }} · v{{ run.version }} <button class="inline-button" :disabled="loadingRun" @click="refresh">重新核对</button></p>
        <div v-if="running || stage" class="progress-block">
          <div class="stage-track"><span v-for="(item, index) in stages" :key="item.key" :class="{ active: displayStage === item.key, passed: stageIndex > index || run?.status === 'succeeded' }">{{ item.label }}</span></div>
          <p>{{ detail }}</p>
          <ol v-if="questions.length" class="subquestions"><li v-for="question in questions" :key="question">{{ question }}</li></ol>
        </div>
        <p v-if="error" class="error" role="alert">{{ error }}</p>
        <div v-if="!running && !report && !error && !stage" class="research-empty"><span aria-hidden="true">[ 1 → n → 1 ]</span><h3>让结论有迹可循</h3><p>研究过程会展示子问题、来源摘录与引用核对结果。没有足够证据时，会明确告诉你。</p></div>
        <article v-if="report" class="report">
          <section v-if="report.metadata.draft_repair" class="warnings" aria-label="草稿结构修复">
            <strong>结构修复尝试 {{ report.metadata.draft_repair.attempts }} 次 · {{ repairLabels[report.metadata.draft_repair.status] }}</strong>
            <p>触发原因 {{ report.metadata.draft_repair.trigger.stage }} · {{ report.metadata.draft_repair.trigger.code }}</p>
            <p v-if="report.metadata.draft_repair.failure">修复失败 {{ report.metadata.draft_repair.failure.stage }} · {{ report.metadata.draft_repair.failure.code }}</p>
            <p>结构修复完成不代表报告成功，引用核对与独立语义审查仍须分别通过。</p>
            <p>引用核对：{{ report.metadata.citation_checked ? '已通过' : '未通过' }}；独立语义审查：{{ report.metadata.semantic_review ? '已记录通过，以实际审查来源范围为准' : '未记录通过' }}。</p>
          </section>
          <section v-if="report.metadata.supplemental_skipped_reason" class="warnings" aria-label="补充调查未执行">
            <strong>补充调查未执行 · {{ budgetLabels[report.metadata.supplemental_skipped_reason] }}</strong>
            <p>{{ report.metadata.draft_repair ? '调用预算已用于结构修复及首次审查。' : '剩余调用预算不足。' }}本次未执行补充检索或第二轮审查，现有证据不足与冲突仍需核实。</p>
          </section>
          <section v-if="report.metadata.supplemental_review_incomplete" class="warnings" aria-label="补充审查未完成">
            <strong>部分结果 · 补充审查未完成</strong>
            <p>已完成的审查覆盖 {{ report.metadata.reviewed_source_ids?.length || 0 }} 个来源：{{ report.metadata.reviewed_source_ids?.join('、') }}。</p>
            <p>新增未审来源：{{ report.metadata.unreviewed_source_ids?.join('、') || '无新增来源；本次复审仍未完成' }}。摘要与分析保留自首轮，不能当作对全部来源的最终结论。</p>
            <p v-if="report.metadata.failure">失败阶段 {{ report.metadata.failure.stage }} · {{ report.metadata.failure.code }}</p>
          </section>
          <div v-if="report.warnings?.length" class="warnings"><strong>来源说明</strong><ul><li v-for="warning in report.warnings" :key="warning">{{ warning }}</li></ul></div>
          <template v-if="report.executive_summary || report.detailed_analysis || report.key_findings?.length">
            <h3>研究摘要</h3><p class="report-copy">{{ report.executive_summary }}</p>
            <h3>关键发现</h3><ol class="findings"><li v-for="finding in report.key_findings" :key="finding">{{ finding }}</li></ol>
            <h3>详细分析</h3><div class="report-copy">{{ report.detailed_analysis }}</div>
            <div class="verification"><strong>{{ report.metadata.citation_checked ? '引用与摘录已核对' : '引用尚未核对' }}</strong><p>{{ report.metadata.verification_limit }}</p></div>
          </template>
          <section v-if="report.claims?.length" class="claim-review" aria-label="主张证据审查"><h3>主张与证据</h3><article v-for="claim in report.claims" :key="claim.id" class="source-card"><div class="source-title"><span class="source-id">{{ claim.id }}</span><strong>{{ claim.statement }}</strong><small>{{ claimLabels[claim.status] }}</small></div><p>{{ claim.reason }}</p><p>可区分来源组 {{ claim.independent_sources }} · 同一文档或转载不按多票计数</p><blockquote v-for="(evidence, index) in claim.evidence" :key="index">[{{ evidence.source_id }}] {{ relationLabels[evidence.relation] }}：{{ evidence.quote }}</blockquote></article></section>
          <section v-if="report.unverified?.length" class="warnings"><h3>待核实事项</h3><ul><li v-for="item in report.unverified" :key="item">{{ item }}</li></ul></section>
          <details v-if="report.metadata.supplemental_review_incomplete && report.review_history?.length" class="verification">
            <summary>查看已完成的历史审查快照</summary>
            <section v-for="snapshot in report.review_history" :key="snapshot.round">
              <h3>第 {{ snapshot.round }} 轮 · {{ snapshot.source_ids.length }} 个来源</h3>
              <p>仅覆盖 {{ snapshot.source_ids.join('、') }}，不包含此后的未审来源。</p>
              <article v-for="claim in snapshot.claims" :key="claim.id"><strong>{{ claim.id }} · {{ claim.statement }} · {{ claimLabels[claim.status] }}（历史意见）</strong><p>{{ claim.reason }}</p><blockquote v-for="(evidence, index) in claim.evidence" :key="index">[{{ evidence.source_id }}] {{ relationLabels[evidence.relation] }}：{{ evidence.quote }}</blockquote></article>
            </section>
          </details>
          <section v-if="report.sources?.length" class="sources" aria-label="研究来源"><h3>来源档案 <small>{{ report.sources.length }}</small></h3>
            <article v-for="source in report.sources" :key="source.id" class="source-card">
              <div class="source-title"><span class="source-id">{{ source.id }}</span><strong>{{ source.title || '知识库文档' }}</strong><small>{{ sourceLabels[source.type] || source.type }}</small></div>
              <blockquote>{{ source.excerpt }}</blockquote><p v-if="source.independence_group">来源组 {{ source.independence_group }} · {{ source.independence_basis }}</p><p v-if="source.acquired_at">获取时间 {{ source.acquired_at }}</p>
              <a v-if="safeURL(source.url)" :href="safeURL(source.url)" target="_blank" rel="noopener noreferrer">查看网络来源 ↗</a>
              <router-link v-else-if="source.knowledge_base_id" :to="`/knowledge/${encodeURIComponent(source.knowledge_base_id)}`">查看知识库 ↗</router-link>
              <details v-if="source.chunk_id"><summary>原文定位</summary><p>文档 {{ source.knowledge_id }}<br />分块 {{ source.chunk_id }}</p></details>
            </article>
          </section>
          <section v-if="report.citation_checks?.length" class="citation-checks"><h3>引用核对</h3><div v-for="(check, index) in report.citation_checks" :key="index"><strong>[{{ check.source_id }}] {{ check.statement }}</strong><blockquote>{{ check.quote }}</blockquote></div></section>
        </article>
      </section>
    </div>
  </main>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { knowledgeApi } from '@/api/knowledge'
import { researchApi, type ResearchReport, type ResearchRun } from '@/api/research'
import type { KnowledgeBase } from '@/types'
const auth = useAuthStore(), route = useRoute(), router = useRouter()
const query = ref(''), kbIds = ref<string[]>([]), graphEnabled = ref(false), webEnabled = ref(false)
const knowledgeBases = ref<KnowledgeBase[]>([]), loadingKBs = ref(false), kbError = ref('')
const hasMoreKBs = ref(false)
let kbPage = 0
const stage = ref(''), detail = ref(''), error = ref(''), questions = ref<string[]>([])
const run = ref<ResearchRun>(), report = ref<ResearchReport | null>(null), submitting = ref(false), cancelling = ref(false), loadingRun = ref(false)
const terminal = new Set(['succeeded', 'partial', 'failed', 'cancelled', 'timed_out'])
const running = computed(() => !!run.value && !terminal.has(run.value.status))
const stateLabel = computed(() => ({ queued: '已排队', running: '研究中', cancel_requested: '取消确认中', cancelled: '已取消 · 保留已收集证据', succeeded: '报告已完成', partial: '部分完成 · 存在待核实事项', failed: '研究失败 · 保留已收集证据', timed_out: '已超时 · 保留已收集证据' }[run.value?.status || ''] || '状态待核对'))
let generation = 0, kbGeneration = 0, controller: AbortController | undefined, timer: ReturnType<typeof setTimeout> | undefined
let pendingKey = '', pendingBody = ''
const displayStage = computed(() => stage.value === 'repair_validating' ? 'repairing' : stage.value)
const stages = computed(() => [
  { key: 'decomposing', label: '拆解问题' }, { key: 'researching', label: '收集证据' }, { key: 'synthesizing', label: '综合分析' },
  ...(report.value?.metadata.draft_repair || displayStage.value === 'repairing' ? [{ key: 'repairing', label: '结构修复' }] : []),
  { key: 'reviewing', label: '审查主张' },
])
const stageIndex = computed(() => stages.value.findIndex(item => item.key === displayStage.value))
const sourceLabels: Record<string, string> = { rag: '知识库', graph: '知识图谱', web: '网络' }
const claimLabels = { supported: '有证据支持', conflict: '存在冲突', insufficient: '证据不足' }, relationLabels = { supports: '支持', contradicts: '冲突', context: '仅背景信息' }
const repairLabels = { reserved: '修复调用已预留', completed: '结构修复已完成', failed: '结构修复失败' }
const budgetLabels = { model_call_budget_exhausted: '模型调用预算已用尽', call_budget_exhausted: '总调用预算已用尽' }
function safeURL(raw?: string): string | undefined { try { const url = new URL(raw || ''); return ['http:', 'https:'].includes(url.protocol) && !url.username && !url.password ? url.href : undefined } catch { return undefined } }
function stopReading() { generation++; controller?.abort(); controller = undefined; if (timer) clearTimeout(timer); timer = undefined; loadingRun.value = false }
function apply(value: ResearchRun) {
  if (run.value?.id === value.id && value.version < run.value.version) return
  run.value = value; stage.value = value.stage; detail.value = stateLabel.value
  if (value.report) { report.value = value.report; questions.value = value.report.sub_questions || [] }
  if (value.submission?.request) { const input = value.submission.request; query.value = input.query; kbIds.value = [...input.kb_ids]; graphEnabled.value = input.graph_enabled; webEnabled.value = input.enable_web_search }
  error.value = value.error || ''
}
async function loadKnowledgeBases(more = false) {
  const current = ++kbGeneration; loadingKBs.value = true; kbError.value = ''
  const page = more ? kbPage + 1 : 1
  if (!more) { knowledgeBases.value = []; hasMoreKBs.value = false }
  try {
    const response = await knowledgeApi.getPage(page, 100)
    if (current !== kbGeneration) return
    const data = response.data
    if (!data || !Array.isArray(data.items) || data.page !== page || data.size < 1 || !Number.isSafeInteger(data.total) || data.total < 0) throw new Error('invalid knowledge page')
    const items = data.items.filter(kb => kb.status !== 0)
    knowledgeBases.value = more ? [...knowledgeBases.value, ...items.filter(kb => !knowledgeBases.value.some(old => old.id === kb.id))] : items
    kbPage = page; hasMoreKBs.value = data.items.length > 0 && page * data.size < data.total
  }
  catch { if (current === kbGeneration) kbError.value = '知识库列表加载失败，请重试。' }
  finally { if (current === kbGeneration) loadingKBs.value = false }
}
async function read(id: string, current: number) {
  if (current !== generation) return
  controller?.abort(); const request = new AbortController(); controller = request; loadingRun.value = true
  try {
    const response = await researchApi.get(id, request.signal)
    if (current !== generation || request.signal.aborted || !response.data) return
    apply(response.data)
  } catch (cause) {
    if (current !== generation || request.signal.aborted) return
    const status = (cause as { response?: { status?: number } })?.response?.status
    if (status === 401 || status === 403 || status === 404) { run.value = undefined; report.value = null; questions.value = []; stage.value = ''; error.value = '研究记录或来源已不可访问，原证据已隐藏。'; return }
    error.value = '状态读取失败；没有重新执行研究。可再次核对。'
  } finally { if (current === generation) loadingRun.value = false }
  if (current === generation && running.value) timer = setTimeout(() => { void read(id, current) }, 2000)
}
function refresh() { const id = run.value?.id || (typeof route.query.run_id === 'string' ? route.query.run_id : ''); if (!id) return; stopReading(); void read(id, generation) }
async function start() {
  if (!query.value.trim() || !kbIds.value.length || running.value || submitting.value || kbError.value) return
  stopReading(); const current = generation; const request = new AbortController(); controller = request
  const body = { query: query.value.trim(), kb_ids: [...kbIds.value], graph_enabled: graphEnabled.value, enable_web_search: webEnabled.value }
  const serialized = JSON.stringify(body); if (serialized !== pendingBody || !pendingKey) { pendingBody = serialized; pendingKey = crypto.randomUUID() }
  submitting.value = true; error.value = ''
  try {
    const response = await researchApi.create(body, pendingKey, request.signal)
    if (current !== generation || request.signal.aborted || !response.data) return
    report.value = null; questions.value = []; apply(response.data)
    await router.replace({ query: { ...route.query, run_id: response.data.id } })
    if (current === generation) { submitting.value = false; stopReading(); void read(response.data.id, generation) }
  } catch { if (current === generation && !request.signal.aborted) error.value = '提交结果未确认。重试相同问题将复用幂等键，不会重复创建研究。' }
  finally { if (current === generation) submitting.value = false }
}
async function cancel() {
  if (!run.value || cancelling.value || !running.value) return
  const id = run.value.id; stopReading(); const current = generation; const request = new AbortController(); controller = request; cancelling.value = true
  try { const response = await researchApi.cancel(id, request.signal); if (current === generation && response.data) apply(response.data) }
  catch { if (current === generation && !request.signal.aborted) error.value = '取消结果尚未确认，正在读取实际状态。' }
  finally { if (current === generation) { cancelling.value = false; void read(id, current) } }
}
watch(
  [() => auth.authEpoch, () => auth.user?.id, () => auth.currentTenant?.id],
  () => {
    stopReading()
    kbGeneration++
    run.value = undefined
    report.value = null
    query.value = ''
    kbIds.value = []
    questions.value = []
    error.value = ''
    stage.value = ''
    detail.value = ''
    pendingKey = ''
    pendingBody = ''
    submitting.value = false
    cancelling.value = false
    void loadKnowledgeBases()
    if (typeof route.query.run_id === 'string') void read(route.query.run_id, generation)
  },
  { immediate: true },
)
watch(() => route.query.run_id, id => { stopReading(); run.value = undefined; report.value = null; questions.value = []; error.value = ''; stage.value = ''; submitting.value = false; cancelling.value = false; if (typeof id === 'string') void read(id, generation) })
onBeforeUnmount(() => { stopReading(); kbGeneration++ })
</script>

<style scoped lang="scss">
.research-page { max-width: 1440px; margin: 0 auto; padding: 32px; color: var(--text-primary); }
.research-heading { display: flex; justify-content: space-between; align-items: center; margin-bottom: 32px; padding-bottom: 28px; border-bottom: 1px solid var(--border-strong); h1 { font-family: Georgia, 'Songti SC', serif; font-size: 34px; font-weight: 500; margin: 12px 0; letter-spacing: 1px; } p { color: var(--text-secondary); margin: 0; line-height: 1.8; } }
.eyebrow { font: 11px monospace; color: var(--primary); letter-spacing: 2px; }
.research-mark { font: 48px Georgia, serif; color: var(--text-muted); padding: 0 12px; }
.research-layout { display: grid; grid-template-columns: 330px minmax(0, 1fr); gap: 40px; align-items: start; }
h2 { margin: 0 0 24px; font-size: 17px; font-weight: 600; span { color: var(--primary); font: 12px monospace; margin-right: 12px; } }
.research-brief { padding: 26px; background: var(--bg-tertiary); border: 1px solid var(--border-default); border-radius: 6px; label { display: block; font-size: 13px; margin: 20px 0 10px; } textarea { width: 100%; box-sizing: border-box; background: var(--bg-elevated); border: 1px solid var(--border-strong); border-radius: 4px; padding: 12px; line-height: 1.7; resize: vertical; font: inherit; font-size: 14px; color: inherit; min-height: 160px; &:focus { outline: 2px solid var(--primary); outline-offset: 2px; } } .el-select { width: 100%; } }
.field-hint { text-align: right; color: var(--text-secondary); font: 11px monospace; margin: 8px 0; }
.scope-note { color: var(--text-secondary); font-size: 12px; line-height: 1.6; }
.research-brief .toggle-row { display: flex; gap: 9px; align-items: flex-start; line-height: 1.5; small { display: block; color: var(--text-secondary); font-size: 11px; margin-top: 4px; } input { accent-color: var(--primary); margin-top: 3px; } }
.research-bounds { display: flex; justify-content: space-between; border-top: 1px solid var(--border-strong); padding-top: 20px; margin: 26px 0 16px; font-size: 11px; color: var(--text-secondary); }
.start-button,.cancel-button { width: 100%; padding: 14px; border: 0; border-radius: 4px; cursor: pointer; font: inherit; font-size: 14px; background: var(--primary); color: var(--on-primary); display: flex; justify-content: space-between; &:disabled { opacity: .45; cursor: not-allowed; } }
.cancel-button { background: var(--bg-elevated); color: var(--text-primary); border: 1px solid var(--border-strong); justify-content: center; }
.research-output { min-width: 0; overflow-wrap: anywhere; }
.run-receipt { color: var(--text-secondary); font-size: 12px; line-height: 1.8; }
.output-heading { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; }
.report-state { font-size: 12px; color: var(--primary); }
.stage-track { display: flex; gap: 7px; span { flex: 1; padding: 12px 2px; font-size: 12px; color: var(--text-secondary); border-top: 2px solid var(--border-strong); } .active { border-color: var(--primary); color: var(--primary); font-weight: 600; } .passed { border-color: var(--primary); } }
.progress-block { padding-bottom: 20px; p { font-size: 13px; color: var(--text-secondary); } }
.subquestions { line-height: 1.8; padding-left: 20px; font-size: 13px; }
.research-empty { padding: 80px 35px; text-align: center; color: var(--text-secondary); > span { color: var(--primary); font: 30px Georgia, serif; } h3 { font-weight: 500; font-size: 21px; color: var(--text-primary); margin: 28px 0 10px; } p { max-width: 350px; margin: auto; font-size: 14px; line-height: 1.9; } }
.error { color: var(--danger); font-size: 13px; line-height: 1.7; background: var(--bg-tertiary); padding: 12px; border-radius: 4px; }
.inline-button { font: inherit; color: inherit; cursor: pointer; border: none; text-decoration: underline; background: transparent; }
.report { padding: 4px 24px 24px; background: var(--bg-tertiary); border: 1px solid var(--border-default); border-radius: 6px; h3 { scroll-margin-top: 20px; font: 600 18px Georgia, 'Songti SC', serif; margin: 28px 0 14px; } .report-copy { white-space: pre-wrap; overflow-wrap: anywhere; line-height: 1.9; font-size: 14px; } }
.findings { padding-left: 22px; li { padding: 5px 0; line-height: 1.9; font-size: 14px; } }
.warnings,.verification { padding: 16px 20px; background: var(--bg-elevated); border-left: 3px solid var(--warning); font-size: 12px; line-height: 1.8; margin: 18px 0; p { margin: 6px 0 0; } ul { margin-bottom: 0; padding-left: 16px; } }
.verification { border-color: var(--primary); }
.source-card { border-top: 1px solid var(--border-default); padding: 18px 0; font-size: 13px; line-height: 1.8; a { color: var(--primary); } details { margin-top: 10px; color: var(--text-secondary); overflow-wrap: anywhere; } }
.source-title { display: flex; flex-wrap: wrap; align-items: baseline; gap: 10px; strong { flex: 1 1 180px; min-width: 0; font-size: 14px; } small { color: var(--text-secondary); } }
.source-id { font: 12px monospace; color: var(--primary); }
blockquote { margin: 12px 0; padding-left: 14px; border-left: 2px solid var(--border-strong); white-space: pre-wrap; overflow-wrap: anywhere; line-height: 1.8; color: var(--text-secondary); }
.citation-checks { font-size: 13px; line-height: 1.8; }
@media(max-width: 980px) { .research-layout { grid-template-columns: 280px minmax(0, 1fr); gap: 22px; } .research-page { padding: 24px; } }
@media(max-width: 720px) { .research-layout { grid-template-columns: 1fr; } .research-page { padding: 16px; } .research-heading h1 { font-size: 28px; } .research-mark { display: none; } .research-empty { padding: 45px 20px; } .report { padding: 4px 16px 16px; } .stage-track { flex-wrap: wrap; span { flex-basis: 25%; } } }
</style>
