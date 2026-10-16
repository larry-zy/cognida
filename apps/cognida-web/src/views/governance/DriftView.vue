<template>
  <div class="workbench">
    <header class="wb-head">
      <div>
        <span class="wb-kicker">REFERENCE & MONITORING</span>
        <h1>漂移监控</h1>
        <p>固定真实参考快照，比较分布变化；先审核基线，再安排监控和治理。</p>
      </div>
      <button :disabled="busy" @click="initialize">刷新</button>
    </header>
    <p v-if="error" role="alert" class="wb-error">{{ error }}</p>
    <p class="wb-footnote">
      漂移不等于数据质量或模型效果下降。比较使用真实行样本，样本不足会明确标记，不构造随机参考分布。
    </p>
    <div class="wb-tabs" role="tablist" aria-label="漂移操作">
      <button
        v-for="tab in tabs"
        :key="tab.id"
        role="tab"
        :aria-selected="active === tab.id"
        @click="active = tab.id"
      >
        {{ tab.label }}
      </button>
    </div>
    <div class="wb-grid">
      <aside class="wb-panel">
        <h2>{{ tabs.find((t) => t.id === active)?.label }}记录</h2>
        <div class="wb-list">
          <button
            v-for="r in records"
            :key="r.id"
            :class="{ selected: selected?.id === r.id }"
            @click="open(r.id)"
          >
            <strong>{{ recordName(r) }}</strong
            ><small>{{ status(r.status) }} · v{{ r.version }}</small>
          </button>
        </div>
        <p v-if="!records.length" class="wb-empty">暂无可查看记录</p>
        <button v-if="next" :disabled="busy" @click="load(true)">
          加载更多记录
        </button>
      </aside>
      <div class="wb-stack">
        <form
          v-if="active === 'baseline'"
          class="wb-panel"
          @submit.prevent="createBaseline"
        >
          <h2>建立参考基线</h2>
          <div class="wb-form">
            <label
              >基线名称<input
                v-model="name"
                required
                maxlength="128"
                :disabled="busy" /></label
            ><label
              >参考快照<select v-model="source" required :disabled="busy">
                <option value="">请选择</option>
                <option
                  v-for="a in assets"
                  :key="a.asset_id + ':' + a.version"
                  :value="a.asset_id + ':' + a.version"
                >
                  {{ a.name || a.asset_id }} · v{{ a.version }}
                </option>
              </select></label
            ><label
              >抽样上限<input
                v-model.number="sampleLimit"
                type="number"
                min="2"
                max="1000"
                required
                :disabled="busy" /></label
            ><label
              >最小有效样本<input
                v-model.number="minimum"
                type="number"
                min="2"
                :max="sampleLimit"
                required
                :disabled="busy" /></label
            ><label
              >固定抽样种子<input
                v-model="seed"
                maxlength="128"
                required
                :disabled="busy"
            /></label>
          </div>
          <p class="wb-footnote">
            最多读取 10,000
            行完整快照；按固定种子对真实行号排序抽样，保存源摘要。提交后需管理员单独批准。
          </p>
          <button class="primary" :disabled="busy">提交基线候选</button>
        </form>
        <form
          v-else-if="active === 'report'"
          class="wb-panel"
          @submit.prevent="compare"
        >
          <h2>比较当前数据</h2>
          <div class="wb-form">
            <label
              >已批准基线<select v-model="baselineID" required :disabled="busy">
                <option value="">请选择</option>
                <option v-for="b in baselines" :key="b.id" :value="b.id">
                  {{ b.body.name }} · v{{ b.version }}
                </option>
              </select></label
            ><label
              >当前快照<select v-model="currentRef" required :disabled="busy">
                <option value="">请选择</option>
                <option
                  v-for="a in assets"
                  :key="a.asset_id + ':' + a.version"
                  :value="a.asset_id + ':' + a.version"
                >
                  {{ a.name || a.asset_id }} · v{{ a.version }}
                </option>
              </select></label
            ><label
              >距离 / 缺失率差阈值<input
                v-model.number="threshold"
                type="number"
                min="0.001"
                max="1"
                step="0.001"
                required
                :disabled="busy"
            /></label>
          </div>
          <button class="primary" :disabled="busy">比较真实快照</button>
        </form>
        <form v-else class="wb-panel" @submit.prevent="createMonitor">
          <h2>创建定时监控</h2>
          <div class="wb-form">
            <label
              >监控名称<input
                v-model="monitorName"
                required
                maxlength="128"
                :disabled="busy" /></label
            ><label
              >已批准基线<select v-model="baselineID" required :disabled="busy">
                <option value="">请选择</option>
                <option v-for="b in baselines" :key="b.id" :value="b.id">
                  {{ b.body.name }}
                </option>
              </select></label
            ><label
              >当前快照<select v-model="currentRef" required :disabled="busy">
                <option value="">请选择</option>
                <option
                  v-for="a in assets"
                  :key="a.asset_id + ':' + a.version"
                  :value="a.asset_id + ':' + a.version"
                >
                  {{ a.name || a.asset_id }} · v{{ a.version }}
                </option>
              </select></label
            ><label
              >检查间隔（秒）<input
                v-model.number="interval"
                type="number"
                min="60"
                max="86400"
                required
                :disabled="busy" /></label
            ><label
              >漂移阈值<input
                v-model.number="threshold"
                type="number"
                min="0.001"
                max="1"
                step="0.001"
                required
                :disabled="busy" /></label
            ><label
              >跟随来源<select v-model="refreshMode" :disabled="busy">
                <option value="fixed">固定快照</option>
                <option value="latest_published">同一来源最新已发布快照</option>
              </select></label
            ><template v-if="refreshMode === 'latest_published'"
              ><label
                >来源类型<select v-model="refreshType">
                  <option
                    v-for="type in [
                      'datasource',
                      'web',
                      'knowledge',
                      'collection',
                      'result',
                      'evaluation',
                    ]"
                    :key="type"
                  >
                    {{ type }}
                  </option>
                </select></label
              ><label
                >来源标识<input
                  v-model="refreshID"
                  required
                  maxlength="2048" /></label
            ></template>
          </div>
          <p class="wb-footnote">
            每次执行重新检查权限。重复快照复用同一报告；只提示变化，不会自动修改基线或执行治理。
          </p>
          <button class="primary" :disabled="busy">创建监控</button>
        </form>
        <div
          v-if="options.state.hasMore || options.state.error"
          class="wb-actions"
        >
          <button :disabled="options.state.loading" @click="moreAssets">
            {{ options.state.error ? "重试资产列表" : "加载更多可选资产" }}
          </button>
        </div>
        <button
          v-if="baselineNext"
          :disabled="busy"
          @click="loadBaselines(true)"
        >
          加载更多已批准基线
        </button>
        <section v-if="selected" class="wb-panel" aria-label="漂移记录详情">
          <div class="wb-section-head">
            <h2>{{ recordName(selected) }}</h2>
            <span class="wb-badge">{{ status(selected.status) }}</span>
          </div>
          <template v-if="baseline"
            ><p>真实来源：{{ baseline.source_ref_id }}</p>
            <p>
              最多 {{ baseline.sampling.limit }} 行，最小
              {{ baseline.sampling.minimum }} 个有效值；固定种子
              {{ baseline.sampling.seed }}
            </p>
            <button
              v-if="selected.status === 'pending_approval'"
              :disabled="busy"
              @click="approve"
            >
              管理员批准此基线
            </button>
            <p v-else>批准时间：{{ baseline.approved_at }}</p></template
          >
          <template v-if="report"
            ><p class="wb-footnote">
              参考 {{ report.baseline_rows }} 行 / 抽样
              {{ report.baseline_sample }} 行；当前 {{ report.current_rows }} 行
              / 抽样 {{ report.current_sample }} 行。
            </p>
            <div class="drift-table">
              <table class="wb-table">
                <thead>
                  <tr>
                    <th>字段 / 方法</th>
                    <th>有效样本（参考 / 当前）</th>
                    <th>距离 / 阈值</th>
                    <th>结论</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(m, index) in report.metrics" :key="index">
                    <td>{{ m.field }}<br />{{ method(m.method) }}</td>
                    <td>{{ m.baseline_count }} / {{ m.current_count }}</td>
                    <td>
                      {{ m.effect == null ? "—" : m.effect.toFixed(4) }} /
                      {{ m.threshold }}
                    </td>
                    <td>{{ status(m.status) }}<br />{{ m.detail }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <p
              v-for="note in report.limitations"
              :key="note"
              class="wb-footnote"
            >
              {{ note }}
            </p>
            <div class="wb-actions">
              <RouterLink
                :to="{
                  path: '/analysis',
                  query: { asset: report.baseline_ref_id },
                }"
                >查看参考来源</RouterLink
              ><RouterLink
                :to="{
                  path: '/analysis',
                  query: { asset: report.current_ref_id },
                }"
                >查看当前来源</RouterLink
              ><RouterLink
                v-if="report.task_id"
                :to="{ path: '/tasks', query: { task_id: report.task_id } }"
                >查看治理草稿</RouterLink
              ><button
                v-else-if="report.changed"
                :disabled="busy"
                @click="remediate"
              >
                管理员创建治理草稿
              </button>
            </div></template
          >
          <template v-if="monitor"
            ><p>下次检查：{{ selected.next_run_at || "已暂停" }}</p>
            <p v-if="monitor.last_error" class="wb-error">
              {{ monitor.last_error }}
            </p>
            <div class="wb-actions">
              <button
                :disabled="busy"
                @click="
                  monitorAction(
                    selected.status === 'active' ? 'pause' : 'resume',
                  )
                "
              >
                {{
                  selected.status === "active" ? "暂停监控" : "恢复监控"
                }}</button
              ><button
                v-if="monitor.last_report_id"
                @click="open(monitor.last_report_id)"
              >
                查看最近比较报告
              </button>
            </div>
            <label class="wb-label"
              >申请替换基线<select v-model="replacement">
                <option value="">请选择已批准基线</option>
                <option
                  v-for="b in baselines.filter(
                    (b) => b.id !== monitor?.baseline_id,
                  )"
                  :key="b.id"
                  :value="b.id"
                >
                  {{ b.body.name }}
                </option>
              </select></label
            ><button
              :disabled="busy || !replacement"
              @click="monitorAction('propose_baseline', replacement)"
            >
              提交基线切换申请
            </button>
            <p v-if="monitor.pending_baseline_id">
              待批准切换：{{ monitor.pending_baseline_id }}
              <button
                :disabled="busy"
                @click="
                  monitorAction('approve_baseline', monitor.pending_baseline_id)
                "
              >
                管理员批准切换
              </button>
            </p></template
          >
          <details>
            <summary>审计与固定摘要</summary>
            <p>{{ selected.id }} · v{{ selected.version }}</p>
            <p v-if="baseline">{{ baseline.source_digest }}</p>
            <p v-if="report">
              {{ report.baseline_digest }}<br />{{ report.current_digest }}
            </p>
            <button :disabled="busy" @click="audit">读取审核记录</button>
            <ul>
              <li v-for="event in events" :key="event.id">
                {{ event.created_at }} · {{ event.action }} · v{{
                  event.version
                }}
              </li>
            </ul>
          </details>
        </section>
      </div>
    </div>
  </div>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import { useAuthStore } from "@/stores/auth";
import { governanceApi as api } from "@/api/governance";
import { lifecycleApi, type AssetRef } from "@/api/lifecycle";
import { useCursorOptions } from "@/composables/useCursorOptions";
import type {
  DriftRecord,
  DriftBaseline,
  DriftReport,
  DriftMonitor,
} from "@/api/governance/featureOperations";
import "@/components/workbench/workbench.css";
const route = useRoute(),
  auth = useAuthStore(),
  tabs = [
    { id: "baseline", label: "参考基线" },
    { id: "report", label: "比较报告" },
    { id: "monitor", label: "定时监控" },
  ];
const active = ref("baseline"),
  records = ref<DriftRecord[]>([]),
  next = ref(""),
  baselines = ref<DriftRecord<DriftBaseline>[]>([]),
  baselineNext = ref(""),
  selected = ref<DriftRecord>(),
  assets = ref<AssetRef[]>([]),
  busy = ref(false),
  error = ref(""),
  events = ref<
    { id: string; action: string; version: number; created_at: string }[]
  >([]);
const name = ref(""),
  source = ref(""),
  sampleLimit = ref(1000),
  minimum = ref(20),
  seed = ref("cognida-drift-v1"),
  baselineID = ref(""),
  currentRef = ref(""),
  threshold = ref(0.2),
  monitorName = ref(""),
  interval = ref(3600),
  refreshMode = ref("fixed"),
  refreshType = ref("datasource"),
  refreshID = ref(""),
  replacement = ref("");
const options = useCursorOptions(assets, (cursor) =>
    lifecycleApi.assets("", cursor),
  ),
  identity = computed(
    () => `${auth.authEpoch}:${auth.user?.id}:${auth.currentTenant?.id}`,
  );
const baseline = computed(() =>
    selected.value?.kind === "baseline"
      ? (selected.value.body as DriftBaseline)
      : undefined,
  ),
  report = computed(() =>
    selected.value?.kind === "report"
      ? (selected.value.body as DriftReport)
      : undefined,
  ),
  monitor = computed(() =>
    selected.value?.kind === "monitor"
      ? (selected.value.body as DriftMonitor)
      : undefined,
  );
let generation = 0,
  listRead = 0,
  detailRead = 0,
  timer: ReturnType<typeof setTimeout> | undefined;
const status = (s: string) =>
  ({
    pending_approval: "待管理员批准",
    approved: "已批准",
    changed: "发现变化",
    stable: "未达到变化阈值",
    insufficient: "样本不足",
    active: "监控中",
    paused: "已暂停",
    blocked: "来源权限失效",
  })[s] || s;
const method = (s: string) =>
  ({
    schema: "结构核对",
    empirical_ks: "经验分布距离（KS）",
    categorical_total_variation: "类别总变差距离",
    missing_rate_delta: "缺失率差",
  })[s] || s;
function recordName(r: DriftRecord) {
  return r.kind === "report"
    ? "真实快照比较"
    : (r.body as DriftBaseline | DriftMonitor).name;
}
function stop() {
  if (timer) clearTimeout(timer);
  timer = undefined;
}
async function run(fn: () => Promise<void>) {
  if (busy.value) return;
  const own = generation;
  busy.value = true;
  error.value = "";
  try {
    await fn();
  } catch (e) {
    if (own === generation)
      error.value = e instanceof Error ? e.message : "操作失败";
  } finally {
    if (own === generation) busy.value = false;
  }
}
async function load(append = false) {
  const own = generation,
    seq = ++listRead,
    kind = active.value;
  try {
    const r = await api.list<DriftRecord>(
      "/drift/records",
      append ? next.value : "",
      { kind },
    );
    if (own !== generation || seq !== listRead || kind !== active.value) return;
    records.value = append
      ? [...records.value, ...(r.data?.items || [])]
      : r.data?.items || [];
    next.value = r.data?.next_cursor || "";
  } catch (e) {
    if (own === generation && seq === listRead)
      error.value = e instanceof Error ? e.message : "记录加载失败";
  }
}
async function loadBaselines(append = false) {
  const own = generation;
  const r = await api.list<DriftRecord<DriftBaseline>>(
    "/drift/records",
    append ? baselineNext.value : "",
    { kind: "baseline" },
  );
  if (own !== generation) return;
  const rows = (r.data?.items || []).filter((b) => b.status === "approved");
  baselines.value = append ? [...baselines.value, ...rows] : rows;
  baselineNext.value = r.data?.next_cursor || "";
}
async function open(id: string) {
  const own = generation,
    seq = ++detailRead;
  stop();
  selected.value = undefined;
  events.value = [];
  try {
    const r = await api.get<DriftRecord>(
      `/drift/records/${encodeURIComponent(id)}`,
    );
    if (own !== generation || seq !== detailRead) return;
    if (!r.data) throw new Error("记录不存在或无权查看");
    selected.value = r.data;
    error.value = "";
    if (r.data.kind === "monitor" && r.data.status === "active")
      timer = setTimeout(() => void open(id), 4000);
  } catch (e) {
    if (own === generation && seq === detailRead)
      error.value = e instanceof Error ? e.message : "读取失败";
  }
}
async function mutate(path: string, payload: unknown) {
  const own = generation;
  await run(async () => {
    const r = await api.action<DriftRecord>(path, payload);
    if (own !== generation) return;
    if (!r.data) throw new Error("未返回已保存记录，请刷新核查");
    selected.value = r.data;
    events.value = [];
    await load();
    if (own !== generation) return;
    await loadBaselines();
  });
}
function createBaseline() {
  return mutate("/drift/baselines", {
    name: name.value,
    source_ref_id: source.value,
    sampling: {
      limit: sampleLimit.value,
      minimum: minimum.value,
      seed: seed.value,
    },
  });
}
function compare() {
  return mutate("/drift/compare", {
    baseline_id: baselineID.value,
    current_ref_id: currentRef.value,
    threshold: threshold.value,
  });
}
function createMonitor() {
  return mutate("/drift/monitors", {
    name: monitorName.value,
    baseline_id: baselineID.value,
    current_ref_id: currentRef.value,
    interval_seconds: interval.value,
    threshold: threshold.value,
    ...(refreshMode.value === "latest_published"
      ? {
          refresh: {
            mode: "latest_published",
            source_type: refreshType.value,
            source_id: refreshID.value,
          },
        }
      : {}),
  });
}
function approve() {
  if (selected.value)
    return mutate(`/drift/baselines/${selected.value.id}/approve`, {
      expected_version: selected.value.version,
    });
}
function monitorAction(action: string, id = "") {
  if (selected.value)
    return mutate(`/drift/monitors/${selected.value.id}/${action}`, {
      expected_version: selected.value.version,
      baseline_id: id,
    });
}
function remediate() {
  if (selected.value)
    return mutate(`/drift/reports/${selected.value.id}/remediation`, {
      expected_version: selected.value.version,
    });
}
async function audit() {
  const own = generation,
    id = selected.value?.id;
  if (!id) return;
  await run(async () => {
    const r = await api.get<typeof events.value>(`/drift/records/${id}/events`);
    if (own === generation && selected.value?.id === id)
      events.value = r.data || [];
  });
}
async function moreAssets() {
  await run(options.loadMore);
}
async function initialize() {
  const own = generation;
  await run(async () => {
    await Promise.all([load(), loadBaselines(), options.reset()]);
  });
  if (own !== generation) return;
  const id = route.query.record_id;
  if (typeof id === "string") await open(id);
}
watch(active, () => {
  records.value = [];
  next.value = "";
  void load();
});
watch(
  () => route.query.record_id,
  (id) => {
    detailRead++;
    stop();
    selected.value = undefined;
    if (typeof id === "string") void open(id);
  },
);
watch(identity, () => {
  generation++;
  listRead++;
  detailRead++;
  stop();
  busy.value = false;
  selected.value = undefined;
  records.value = [];
  baselines.value = [];
  assets.value = [];
  events.value = [];
  next.value = "";
  baselineNext.value = "";
  source.value = "";
  currentRef.value = "";
  baselineID.value = "";
  replacement.value = "";
  error.value = "";
  void initialize();
});
onMounted(initialize);
onBeforeUnmount(() => {
  generation++;
  stop();
});
</script>
<style scoped>
.drift-table {
  overflow-x: auto;
}
.wb-table {
  min-width: 520px;
}
.wb-panel p {
  overflow-wrap: anywhere;
}
.wb-tabs {
  display: flex;
  gap: 8px;
  margin-bottom: 20px;
  flex-wrap: wrap;
}
.wb-tabs [aria-selected="true"] {
  background: var(--primary-50);
  color: var(--primary-700);
}
</style>
