<template>
  <div class="workbench operations-view">
    <header class="wb-header"><div><span class="wb-eyebrow">受控 SQL / ETL</span><h1>先核对计划，再提交数据变更</h1><p>仅管理当前用户的派生目标。原始业务表与外部数仓保持只读；每次成功切换都保留旧版本。</p></div><button :disabled="loading" @click="loadLists()">刷新列表</button></header>
    <p v-if="error" class="wb-error" role="alert">{{ error }}</p><p v-if="notice" role="status">{{ notice }}</p>
    <div class="operations-grid"><section class="wb-panel" aria-label="准备操作计划"><h2>准备一份可审核计划</h2><form class="wb-stack" @submit.prevent="prepare"><fieldset :disabled="busy"><div class="wb-form">
      <label class="wb-label">操作类型<select v-model="draft.action"><option value="etl">ETL：固定输入写入新版本</option><option value="insert">插入一行</option><option value="update">按等值条件更新</option><option value="delete">按等值条件删除</option><option value="rollback">切回保留的历史内容</option></select></label>
      <label class="wb-label">受管目标名称<input v-model="draft.target" list="managed-targets" required pattern="[A-Za-z][A-Za-z0-9_]{0,63}" placeholder="agent_etl_sales" /></label><datalist id="managed-targets"><option v-for="target in targets" :key="target.id" :value="target.name">当前 v{{ target.version }}</option></datalist>
      <label class="wb-label">预期当前版本<input v-model.number="draft.expected_version" type="number" min="0" required /></label>
      <template v-if="draft.action === 'etl'"><label class="wb-label">已授权输入资产 ID<input v-model="assetID" required placeholder="选择处理产物或查询结果的资产 ID" /></label><label class="wb-label">输入资产版本<input v-model.number="assetVersion" type="number" min="1" required /></label></template>
      <label v-if="draft.action === 'rollback'" class="wb-label">保留的历史版本<input v-model.number="rollbackVersion" type="number" min="1" required /></label>
    </div><p class="wb-muted">新目标的当前版本为 0。已有目标必须填写列表中的真实版本；发生并发变化时需要重新核对。ETL 最多 1,000 行、32 列。</p>
    <section v-if="['insert', 'update'].includes(draft.action)" aria-label="写入字段"><h3>写入字面值</h3><div v-for="(item, index) in values" :key="index" class="cell-row"><label>列名<input v-model="item.column" required /></label><label>类型<select v-model="item.type"><option value="text">文本</option><option value="number">精确数值</option><option value="boolean">布尔值</option><option value="null">空值</option></select></label><label>值<input v-model="item.text" :disabled="item.type === 'null'" :required="item.type !== 'null'" /></label><button type="button" @click="values.splice(index, 1)">移除</button></div><button type="button" @click="values.push(cell())">添加写入字段</button></section>
    <section v-if="['update', 'delete'].includes(draft.action)" aria-label="等值筛选条件"><h3>必须满足全部等值条件</h3><div v-for="(item, index) in where" :key="index" class="cell-row"><label>条件列<input v-model="item.column" required /></label><label>条件类型<select v-model="item.type"><option value="text">文本</option><option value="number">精确数值</option><option value="boolean">布尔值</option><option value="null">空值</option></select></label><label>条件值<input v-model="item.text" :disabled="item.type === 'null'" :required="item.type !== 'null'" /></label><button type="button" @click="where.splice(index, 1)">移除</button></div><button type="button" @click="where.push(cell())">添加等值条件</button><p class="wb-muted">数值按原始十进制文字精确匹配。这里不支持任意脚本、表达式或无条件更新。</p></section>
    </fieldset><button class="primary" :disabled="busy">生成待确认计划</button></form></section>
    <section class="wb-panel" aria-label="我的受管目标"><h2>受管目标</h2><p v-if="loading" role="status">正在读取…</p><p v-if="targetsError" class="wb-error" role="alert">{{ targetsError }}</p><p v-else-if="!loading && !targets.length" class="wb-muted">尚未提交派生目标；先用已授权表格资产准备 ETL。</p><ul><li v-for="target in targets" :key="target.id"><strong>{{ target.name }}</strong> · 当前 v{{ target.version }} <button :disabled="busy" @click="chooseTarget(target)">填入目标</button></li></ul><h3>已读取的操作</h3><p v-if="listError" class="wb-error" role="alert">{{ listError }}</p><p v-else-if="!loading && !jobs.length" class="wb-muted">尚无操作计划。</p><ul><li v-for="item in jobs" :key="item.id"><button :disabled="busy" @click="open(item.id)">{{ item.plan.request.target }} · {{ statusLabel(item.status) }}</button></li></ul><button v-if="cursor" :disabled="loading" @click="loadLists(true)">继续读取操作</button></section></div>
    <section v-if="jobID" class="wb-panel" aria-labelledby="receipt-title"><div class="wb-section-head"><h2 id="receipt-title">计划与真实提交回执</h2><button :disabled="busy || reading" @click="refresh">重新核对状态</button></div><p v-if="reading" role="status">正在核对持久记录…</p><template v-if="current"><p><strong>{{ statusLabel(current.status) }}</strong> · {{ current.id }}</p><p v-if="current.error" class="wb-error">{{ current.error }}</p><template v-if="current.plan?.request?.target"><dl class="receipt-grid"><div><dt>目标</dt><dd>{{ current.plan.request.target }}</dd></div><div><dt>预期当前版本</dt><dd>{{ current.plan.request.expected_version }}</dd></div><div><dt>计划影响行数</dt><dd>{{ current.plan.estimated_affected }}</dd></div><div><dt>计划有效至</dt><dd>{{ current.expires_at }}</dd></div></dl><p class="wb-muted">物理表由服务端分配；新内容核验成功后，数据提交、目标切换与审计回执在同一事务完成。</p><code class="plan-hash">{{ current.plan.hash }}</code><pre>{{ current.plan.sql }}</pre><ul aria-label="绑定参数"><li v-for="(item, i) in current.plan.parameters" :key="i">参数 {{ i + 1 }} · {{ item.type }}：{{ item.text }}</li></ul><details><summary>提交后的内容预览（{{ current.plan.after.rows.length }} 行）</summary><div class="table-scroll"><table><thead><tr><th v-for="name in current.plan.after.columns" :key="name">{{ name }}</th></tr></thead><tbody><tr v-for="(row, index) in current.plan.after.rows.slice(0, 20)" :key="index"><td v-for="(value, i) in row" :key="i">{{ value.type === 'null' ? 'NULL' : value.text }}</td></tr></tbody></table></div><p class="wb-muted">最多展示前 20 行；精确数字与带前导零的文本均保留原文。</p></details></template>
      <div v-if="current.status === 'planned'" class="confirmation"><label><input v-model="acknowledged" type="checkbox" :disabled="busy" />我已核对目标、预期版本、影响行数和计划摘要，确认提交这份计划。</label><button class="primary" :disabled="busy || !acknowledged" @click="confirm">明确确认并提交</button><p class="wb-muted">刷新或打开链接不会执行。确认结果未知时仅重新读取原任务，不能换键重复提交。</p></div>
      <p v-if="current.status === 'completed'">已提交版本 v{{ current.committed_version }}，实际 SQL 影响／切换记录 {{ current.affected_rows }} 行。旧版本仍保留，可另行准备回滚计划。</p><div class="source-actions"><button v-if="!terminal(current.status)" :disabled="busy" @click="cancel">取消此操作</button><RouterLink v-if="current.output_ref" :to="{ path: '/analysis', query: { asset: `${current.output_ref.asset_id}:${current.output_ref.version}` } }">查看真实结果产物 ↗</RouterLink></div>
    </template></section>
  </div>
</template>
<script setup lang="ts">
import { onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { operationApi as api, type Cell, type OperationJob, type OperationRequest, type OperationTarget } from '@/api/governedops'
import '@/components/workbench/workbench.css'
const route = useRoute(), router = useRouter(), auth = useAuthStore()
const draft = reactive<OperationRequest>({ action: 'etl', target: '', expected_version: 0 })
const cell = () => ({ column: '', type: 'text' as Cell['type'], text: '' })
const values = ref([cell()]), where = ref([cell()]), assetID = ref(''), assetVersion = ref(1), rollbackVersion = ref(1)
const targets = ref<OperationTarget[]>([]), jobs = ref<OperationJob[]>([]), cursor = ref(''), current = ref<OperationJob | null>(null), jobID = ref('')
const loading = ref(false), reading = ref(false), busy = ref(false), error = ref(''), notice = ref(''), targetsError = ref(''), listError = ref(''), acknowledged = ref(false)
let epoch = 0, readSequence = 0, listSequence = 0, timer: ReturnType<typeof setTimeout> | undefined, pending: { fingerprint: string; key: string } | null = null
const controllers = new Set<AbortController>()
function controller() { const c = new AbortController(); controllers.add(c); return c }
const terminal = (status: string) => ['completed', 'failed', 'timed_out', 'cancelled'].includes(status)
const statusLabel = (status: string) => ({ planned: '等待明确确认', queued: '已确认，等待执行', running: '正在提交，结果待核对', completed: '提交完成', failed: '未完成提交', timed_out: '已超时', cancelled: '已取消' } as Record<string, string>)[status] || status
function accessDenied(cause: unknown) { const e = cause as { response?: { status?: number } } | null; return [401, 403, 404].includes(e?.response?.status || 0) }
function failure(cause: unknown) { const e = cause as { response?: { status?: number } }; return accessDenied(cause) ? '操作或来源当前不可访问，请核对身份与权限。' : e.response?.status === 409 ? '计划、确认摘要或目标版本已变化，请重新读取并核对。' : '请求未确认成功，请读取原操作状态；准备重试会复用相同幂等键。' }
async function loadLists(more = false) {
  const generation = epoch, sequence = ++listSequence, c = controller()
  loading.value = true
  const results = await Promise.allSettled([api.targets(c.signal), api.list(more ? cursor.value : '', c.signal)])
  controllers.delete(c)
  if (generation !== epoch || sequence !== listSequence) return
  const [a, b] = results
  if (a.status === 'fulfilled' && a.value.data) {
    targets.value = a.value.data.items
    targetsError.value = ''
  } else {
    if (a.status === 'rejected' && accessDenied(a.reason)) targets.value = []
    targetsError.value = targets.value.length ? '受管目标列表读取失败；下方为上次读取的内容。' : '受管目标列表读取失败。'
  }
  if (b.status === 'fulfilled' && b.value.data) {
    jobs.value = more ? [...jobs.value, ...b.value.data.items.filter(j => !jobs.value.some(old => old.id === j.id))] : b.value.data.items
    cursor.value = b.value.data.next_cursor || ''
    listError.value = ''
  } else {
    if (b.status === 'rejected' && accessDenied(b.reason)) { jobs.value = []; cursor.value = '' }
    listError.value = jobs.value.length ? '操作列表读取失败；下方为上次读取的内容。' : '操作列表读取失败。'
  }
  loading.value = false
}
function chooseTarget(target: OperationTarget) { draft.target = target.name; draft.expected_version = target.version }
function schedule() { clearTimeout(timer); if (current.value && ['queued', 'running'].includes(current.value.status)) timer = setTimeout(refresh, 4000) }
async function refresh() { if (!jobID.value) return; const generation = epoch, sequence = ++readSequence, id = jobID.value, c = controller(); reading.value = true; try { const response = await api.get(id, c.signal); if (generation !== epoch || sequence !== readSequence || id !== jobID.value) return; if (!response.data) throw new Error('missing receipt'); current.value = response.data; error.value = ''; schedule() } catch (cause) { if (generation === epoch && sequence === readSequence) { current.value = null; acknowledged.value = false; error.value = failure(cause); clearTimeout(timer) } } finally { controllers.delete(c); if (generation === epoch && sequence === readSequence) reading.value = false } }
async function open(id: string) { clearTimeout(timer); current.value = null; acknowledged.value = false; jobID.value = id; await router.replace({ query: { ...route.query, job_id: id } }); if (jobID.value === id) await refresh() }
async function action(work: (signal: AbortSignal, generation: number) => Promise<void>) { if (busy.value) return; const generation = epoch, c = controller(); busy.value = true; error.value = ''; notice.value = ''; try { await work(c.signal, generation) } catch (cause) { if (generation === epoch) error.value = failure(cause) } finally { controllers.delete(c); if (generation === epoch) busy.value = false } }
function fields(items: ReturnType<typeof cell>[]) { const result: Record<string, Cell> = {}; for (const item of items) { if (!item.column || Object.prototype.hasOwnProperty.call(result, item.column)) throw new Error('duplicate field'); result[item.column] = { type: item.type, text: item.type === 'null' ? '' : item.text } } if (!Object.keys(result).length) throw new Error('missing field'); return result }
async function prepare() { await action(async (signal, generation) => { const input: OperationRequest = { action: draft.action, target: draft.target, expected_version: draft.expected_version }; if (input.action === 'etl') input.input_ref = { asset_id: assetID.value, version: assetVersion.value }; if (['insert', 'update'].includes(input.action)) input.values = fields(values.value); if (['update', 'delete'].includes(input.action)) input.where = fields(where.value); if (input.action === 'rollback') input.rollback_version = rollbackVersion.value; const fingerprint = JSON.stringify(input); if (!pending || pending.fingerprint !== fingerprint) pending = { fingerprint, key: crypto.randomUUID() }; const response = await api.prepare(input, pending.key, signal); if (generation !== epoch) return; if (!response.data) throw new Error('missing plan'); pending = null; await open(response.data.id); if (generation === epoch) { notice.value = '计划已保存，尚未提交数据变更。'; await loadLists() } }) }
async function confirm() { const item = current.value; if (!item || !acknowledged.value || item.status !== 'planned') return; acknowledged.value = false; await action(async (signal, generation) => { try { await api.confirm(item, signal) } finally { if (generation === epoch) await refresh() } }) }
async function cancel() { const item = current.value; if (!item || terminal(item.status)) return; await action(async (signal, generation) => { const response = await api.cancel(item.id, signal); if (generation === epoch && item.id === jobID.value && response.data) { current.value = response.data; acknowledged.value = false; schedule(); await loadLists() } }) }
function reset() { epoch++; readSequence++; listSequence++; controllers.forEach(c => c.abort()); controllers.clear(); clearTimeout(timer); pending = null; current.value = null; jobs.value = []; targets.value = []; cursor.value = ''; jobID.value = ''; acknowledged.value = false; error.value = ''; notice.value = ''; targetsError.value = ''; listError.value = ''; loading.value = false; busy.value = false; reading.value = false; draft.target = ''; draft.expected_version = 0; assetID.value = ''; values.value = [cell()]; where.value = [cell()] }
function initialize() { jobID.value = typeof route.query.job_id === 'string' ? route.query.job_id : ''; void loadLists(); if (jobID.value) void refresh() }
watch(() => [auth.authEpoch, auth.user?.id, auth.currentTenant?.id], () => { reset(); initialize() })
watch(() => route.query.job_id, value => { const id = typeof value === 'string' ? value : ''; if (id !== jobID.value) { current.value = null; acknowledged.value = false; jobID.value = id; clearTimeout(timer); if (id) void refresh() } })
onMounted(initialize); onBeforeUnmount(reset)
</script>
<style scoped>
.operations-grid{display:grid;grid-template-columns:minmax(0,1.4fr) minmax(260px,1fr);gap:20px}.operations-view fieldset{border:0;padding:0;margin:0}.operations-view input:not([type=checkbox]),.operations-view select{width:100%;box-sizing:border-box}.cell-row{display:grid;grid-template-columns:1fr 100px 1.3fr auto;gap:8px;align-items:end;margin:10px 0}.cell-row label{display:grid;gap:4px;font-size:12px}.receipt-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:12px}.receipt-grid dt{color:var(--text-muted);font-size:12px}.receipt-grid dd{margin:5px 0;overflow-wrap:anywhere}.operations-view ul{padding-left:20px}.operations-view li{margin:10px 0}.plan-hash{display:block;overflow-wrap:anywhere;font-size:12px}.operations-view pre{white-space:pre-wrap;overflow-wrap:anywhere;background:var(--bg-secondary);padding:14px;border-radius:8px}.confirmation{display:grid;gap:12px;padding:16px;margin:18px 0;border:1px solid var(--border-color);border-radius:10px}.confirmation .primary{justify-self:start}.table-scroll{overflow:auto}.table-scroll table{border-collapse:collapse}.table-scroll th,.table-scroll td{padding:8px 14px;text-align:left;border-bottom:1px solid var(--border-color);white-space:nowrap}.source-actions{display:flex;gap:16px;align-items:center;margin-top:16px}@media(max-width:900px){.operations-grid{grid-template-columns:1fr}.receipt-grid{grid-template-columns:repeat(2,minmax(0,1fr))}}@media(max-width:600px){.cell-row{grid-template-columns:1fr 1fr}.receipt-grid{grid-template-columns:1fr}}
</style>
