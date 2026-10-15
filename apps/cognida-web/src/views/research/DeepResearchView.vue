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
        <el-select id="research-kbs" v-model="kbIds" multiple collapse-tags collapse-tags-tooltip placeholder="全部已启用知识库" :disabled="running || loadingKBs" aria-label="知识库范围">
          <el-option v-for="kb in knowledgeBases" :key="kb.id" :value="kb.id" :label="kb.name" />
        </el-select>
        <p class="scope-note">未选择时使用当前租户全部已启用知识库。</p>
        <label class="toggle-row"><input v-model="graphEnabled" type="checkbox" :disabled="running" /><span>加入图谱关联证据<small>仅使用所选范围中有原文出处的关系</small></span></label>
        <label class="toggle-row"><input v-model="webEnabled" type="checkbox" :disabled="running" /><span>补充网络搜索<small>需管理员配置可用的搜索服务</small></span></label>
        <div class="research-bounds"><span>最多 3 个子问题</span><span>最长 2 分钟</span></div>
        <button v-if="!running" class="start-button" :disabled="!query.trim() || loadingKBs || !!kbError" @click="start">开始研究 <span aria-hidden="true">↗</span></button>
        <button v-else class="cancel-button" @click="cancel">取消研究</button>
        <p v-if="kbError" class="error" role="alert">{{ kbError }} <button class="inline-button" @click="loadKnowledgeBases">重试</button></p>
      </section>
      <section class="research-output" aria-labelledby="report-title" aria-live="polite" :aria-busy="running">
        <header class="output-heading"><h2 id="report-title"><span>02</span> 研究记录</h2><span v-if="report?.success" class="report-state">{{ report.metadata.partial ? '已完成 · 部分来源不可用' : '报告已完成' }}</span></header>
        <div v-if="running || stage" class="progress-block">
          <div class="stage-track"><span v-for="(item, index) in stages" :key="item.key" :class="{ active: stage === item.key, passed: stageIndex > index || report?.success }">{{ item.label }}</span></div>
          <p>{{ detail }}</p>
          <ol v-if="questions.length" class="subquestions"><li v-for="question in questions" :key="question">{{ question }}</li></ol>
        </div>
        <p v-if="error" class="error" role="alert">{{ error }}</p>
        <div v-if="!running && !report && !error && !stage" class="research-empty"><span aria-hidden="true">[ 1 → n → 1 ]</span><h3>让结论有迹可循</h3><p>研究过程会展示子问题、来源摘录与引用核对结果。没有足够证据时，会明确告诉你。</p></div>
        <article v-if="report" class="report">
          <div v-if="report.warnings?.length" class="warnings"><strong>来源说明</strong><ul><li v-for="warning in report.warnings" :key="warning">{{ warning }}</li></ul></div>
          <template v-if="report.success">
            <h3>研究摘要</h3><p class="report-copy">{{ report.executive_summary }}</p>
            <h3>关键发现</h3><ol class="findings"><li v-for="finding in report.key_findings" :key="finding">{{ finding }}</li></ol>
            <h3>详细分析</h3><div class="report-copy">{{ report.detailed_analysis }}</div>
            <div class="verification"><strong>{{ report.metadata.citation_checked ? '引用与摘录已核对' : '引用尚未核对' }}</strong><p>{{ report.metadata.verification_limit }}</p></div>
          </template>
          <section v-if="report.sources?.length" class="sources" aria-label="研究来源"><h3>来源档案 <small>{{ report.sources.length }}</small></h3>
            <article v-for="source in report.sources" :key="source.id" class="source-card">
              <div class="source-title"><span class="source-id">{{ source.id }}</span><strong>{{ source.title || '知识库文档' }}</strong><small>{{ sourceLabels[source.type] || source.type }}</small></div>
              <blockquote>{{ source.excerpt }}</blockquote>
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
import { useAuthStore } from '@/stores/auth'
import { knowledgeApi } from '@/api/knowledge'
import { streamResearch, type ResearchReport } from '@/api/research'
import type { KnowledgeBase } from '@/types'

const auth = useAuthStore()
const query = ref(''), kbIds = ref<string[]>([]), graphEnabled = ref(false), webEnabled = ref(false)
const knowledgeBases = ref<KnowledgeBase[]>([]), loadingKBs = ref(false), kbError = ref('')
const running = ref(false), stage = ref(''), detail = ref(''), error = ref(''), questions = ref<string[]>([])
const report = ref<ResearchReport | null>(null)
let controller: AbortController | undefined
let epoch = 0
const stages = [{ key: 'decomposing', label: '拆解问题' }, { key: 'researching', label: '收集证据' }, { key: 'synthesizing', label: '综合分析' }, { key: 'validating', label: '核对引用' }]
const stageIndex = computed(() => stages.findIndex(item => item.key === stage.value))
const sourceLabels: Record<string, string> = { rag: '知识库', graph: '知识图谱', web: '网络' }
function safeURL(raw?: string): string | undefined {
  try { const url = new URL(raw || ''); return ['http:', 'https:'].includes(url.protocol) && !url.username && !url.password ? url.href : undefined } catch { return undefined }
}
async function loadKnowledgeBases() {
  const version = ++epoch
  loadingKBs.value = true; kbError.value = ''
  try { const response = await knowledgeApi.getList(); if (version === epoch) knowledgeBases.value = (response.data || []).filter(kb => kb.status !== 0) }
  catch { if (version === epoch) kbError.value = '知识库列表加载失败，请重试。' }
  finally { if (version === epoch) loadingKBs.value = false }
}
async function start() {
  if (!query.value.trim() || running.value || kbError.value) return
  controller?.abort()
  const current = new AbortController(); controller = current
  running.value = true; report.value = null; error.value = ''; stage.value = 'decomposing'; detail.value = '正在规划研究'; questions.value = []
  try {
    for await (const event of streamResearch({ query: query.value.trim(), kb_ids: [...kbIds.value], kb_scope_mode: 'manual', graph_enabled: graphEnabled.value, options: { enable_web_search: webEnabled.value, max_sub_questions: 3, timeout_seconds: 120 } }, current.signal)) {
      if (controller !== current || current.signal.aborted) return
      stage.value = event.stage; detail.value = event.detail
      if (event.sub_questions) questions.value = event.sub_questions
      if (event.report) { report.value = event.report; questions.value = event.report.sub_questions }
      if (event.stage === 'error') error.value = event.error || '研究未完成，请调整问题或来源后重试。'
    }
  } catch (cause) { if (controller === current && !current.signal.aborted) error.value = cause instanceof Error ? cause.message : '研究请求失败' }
  finally { if (controller === current) { running.value = false; controller = undefined } }
}
function cancel() { controller?.abort(); controller = undefined; running.value = false; detail.value = '研究已取消'; stage.value = 'cancelled' }
watch(() => [auth.authEpoch, auth.user?.id, auth.currentTenant?.id], () => {
  cancel(); query.value = ''; kbIds.value = []; knowledgeBases.value = []; report.value = null; questions.value = []; error.value = ''; detail.value = ''; stage.value = ''
  void loadKnowledgeBases()
}, { immediate: true })
onBeforeUnmount(() => { ++epoch; controller?.abort() })
</script>

<style scoped lang="scss">
.research-page { max-width: 1440px; margin: 0 auto; padding: 32px; color: var(--el-text-color-primary); }
.research-heading { display: flex; justify-content: space-between; align-items: center; margin-bottom: 32px; padding-bottom: 28px; border-bottom: 1px solid var(--el-border-color); h1 { font-family: Georgia, 'Songti SC', serif; font-size: 34px; font-weight: 500; margin: 12px 0; letter-spacing: 1px; } p { color: var(--el-text-color-secondary); margin: 0; line-height: 1.8; } }
.eyebrow { font: 11px monospace; color: #557a70; letter-spacing: 2px; }
.research-mark { font: 48px Georgia, serif; color: #8a9b94; padding: 0 12px; }
.research-layout { display: grid; grid-template-columns: 330px minmax(0, 1fr); gap: 40px; align-items: start; }
h2 { margin: 0 0 24px; font-size: 17px; font-weight: 600; span { color: #7e928b; font: 12px monospace; margin-right: 12px; } }
.research-brief { padding: 26px; background: var(--el-fill-color-light); border: 1px solid var(--el-border-color-lighter); border-radius: 6px; label { display: block; font-size: 13px; margin: 20px 0 10px; } textarea { width: 100%; box-sizing: border-box; background: var(--el-bg-color); border: 1px solid var(--el-border-color); border-radius: 4px; padding: 12px; line-height: 1.7; resize: vertical; font: inherit; font-size: 14px; color: inherit; min-height: 160px; &:focus { outline: 2px solid #45796a; outline-offset: 2px; } } .el-select { width: 100%; } }
.field-hint { text-align: right; color: var(--el-text-color-secondary); font: 11px monospace; margin: 8px 0; }
.scope-note { color: var(--el-text-color-secondary); font-size: 12px; line-height: 1.6; }
.research-brief .toggle-row { display: flex; gap: 9px; align-items: flex-start; line-height: 1.5; small { display: block; color: var(--el-text-color-secondary); font-size: 11px; margin-top: 4px; } input { accent-color: #285d4e; margin-top: 3px; } }
.research-bounds { display: flex; justify-content: space-between; border-top: 1px solid var(--el-border-color); padding-top: 20px; margin: 26px 0 16px; font-size: 11px; color: var(--el-text-color-secondary); }
.start-button,.cancel-button { width: 100%; padding: 14px; border: 0; border-radius: 4px; cursor: pointer; font: inherit; font-size: 14px; background: #285d4e; color: #fff; display: flex; justify-content: space-between; &:disabled { opacity: .45; cursor: not-allowed; } }
.cancel-button { background: var(--el-bg-color); color: var(--el-text-color-primary); border: 1px solid var(--el-border-color); justify-content: center; }
.output-heading { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; }
.report-state { font-size: 12px; color: #397764; }
.stage-track { display: flex; gap: 7px; span { flex: 1; padding: 12px 2px; font-size: 12px; color: var(--el-text-color-secondary); border-top: 2px solid var(--el-border-color); } .active { border-color: #285d4e; color: #285d4e; font-weight: 600; } .passed { border-color: #81a799; } }
.progress-block { padding-bottom: 20px; p { font-size: 13px; color: var(--el-text-color-secondary); } }
.subquestions { line-height: 1.8; padding-left: 20px; font-size: 13px; }
.research-empty { padding: 80px 35px; text-align: center; color: var(--el-text-color-secondary); > span { color: #6d8b80; font: 30px Georgia, serif; } h3 { font-weight: 500; font-size: 21px; color: var(--el-text-color-primary); margin: 28px 0 10px; } p { max-width: 350px; margin: auto; font-size: 14px; line-height: 1.9; } }
.error { color: var(--el-color-danger); font-size: 13px; line-height: 1.7; background: var(--el-color-danger-light-9); padding: 12px; border-radius: 4px; }
.inline-button { font: inherit; color: inherit; cursor: pointer; border: none; text-decoration: underline; background: transparent; }
.report { h3 { font: 600 18px Georgia, 'Songti SC', serif; margin: 28px 0 14px; } .report-copy { white-space: pre-wrap; overflow-wrap: anywhere; line-height: 1.9; font-size: 14px; } }
.findings { padding-left: 22px; li { padding: 5px 0; line-height: 1.9; font-size: 14px; } }
.warnings,.verification { padding: 16px 20px; background: var(--el-fill-color-light); border-left: 3px solid #9d9c70; font-size: 12px; line-height: 1.8; margin: 18px 0; p { margin: 6px 0 0; } ul { margin-bottom: 0; padding-left: 16px; } }
.verification { border-color: #6b9687; }
.source-card { border-top: 1px solid var(--el-border-color-lighter); padding: 18px 0; font-size: 12px; a { color: #397764; } details { margin-top: 10px; color: var(--el-text-color-secondary); overflow-wrap: anywhere; } }
.source-title { display: flex; align-items: baseline; gap: 10px; strong { flex: 1; font-size: 13px; } small { color: var(--el-text-color-secondary); } }
.source-id { font: 12px monospace; color: #397764; }
blockquote { margin: 12px 0; padding-left: 14px; border-left: 2px solid var(--el-border-color); white-space: pre-wrap; overflow-wrap: anywhere; line-height: 1.8; color: var(--el-text-color-secondary); }
.citation-checks { font-size: 12px; }
@media(max-width: 980px) { .research-layout { grid-template-columns: 280px minmax(0, 1fr); gap: 22px; } .research-page { padding: 24px; } }
@media(max-width: 720px) { .research-layout { grid-template-columns: 1fr; } .research-page { padding: 16px; } .research-heading h1 { font-size: 28px; } .research-mark { display: none; } .research-empty { padding: 45px 20px; } }
</style>
