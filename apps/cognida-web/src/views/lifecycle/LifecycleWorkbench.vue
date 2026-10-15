<template>
  <div class="workbench">
    <header class="wb-head">
      <div>
        <span class="wb-kicker">COMPOSABLE DATA WORKFLOWS</span>
        <h1>
          {{
            wt(
              isPipeline
                ? "数据流水线"
                : isAnalysis
                  ? "数据分析"
                  : "专家工作台",
            )
          }}
        </h1>
        <p>
          {{
            isPipeline
              ? "按受控模板编排任务，查看每一步的产出、质量门禁与审批。"
              : "独立调用数据接入、处理、分析与评估专家，所有输出保留来源。"
          }}
        </p>
      </div>
      <router-link to="/analysis/chat">历史数据源对话 →</router-link>
    </header>
    <div v-if="error" class="wb-error" role="alert">{{ error }}</div>
    <div v-if="notice" class="wb-success" role="status">{{ notice }}</div>
    <div
      v-if="
        assetOptions.state.hasMore ||
        assetOptions.state.loading ||
        assetOptions.state.error
      "
      class="wb-actions"
      aria-live="polite"
    >
      <span class="wb-muted">已加载 {{ assets.length }} 个可选资产</span>
      <button
        type="button"
        :disabled="assetOptions.state.loading"
        @click="run(assetOptions.loadMore)"
      >
        {{
          assetOptions.state.loading
            ? "正在加载资产…"
            : assetOptions.state.error
              ? "重试加载资产"
              : "加载更多资产"
        }}
      </button>
    </div>
    <div class="wb-grid">
      <aside class="wb-panel wb-stack">
        <h2>{{ wt("发起任务") }}</h2>
        <form class="wb-stack" @submit.prevent="start">
          <label class="wb-label"
            >{{ isPipeline ? "流水线模板" : "专家"
            }}<select v-model="executor" required>
              <option value="">请选择</option>
              <template v-if="isPipeline"
                ><option
                  v-for="item in templates"
                  :key="item.id"
                  :value="item.id"
                >
                  {{ item.name }} · v{{ item.version }}
                </option></template
              ><template v-else
                ><option v-for="item in agents" :key="item.id" :value="item.id">
                  {{ item.name }}
                </option></template
              >
            </select></label
          >
          <p v-if="selectedAgent" class="wb-muted">
            工具权限：{{
              selectedAgent.tool_allowlist.join("、") || "无业务工具"
            }}<br />风险等级：{{ selectedAgent.risk_class }}
          </p>
          <label class="wb-label"
            >{{ wt("输入资产")
            }}<select v-model="assetSelection" required>
              <option value="">选择输入资产</option>
              <option
                v-for="asset in assets"
                :key="asset.asset_id + asset.version"
                :value="`${asset.asset_id}:${asset.version}`"
              >
                {{ asset.name || asset.asset_id.slice(0, 16) }} ·
                {{ asset.kind }} · v{{ asset.version }}
              </option>
            </select></label
          ><label class="wb-checkbox"
            ><input
              v-model="publishedOnly"
              type="checkbox"
              @change="run(loadAssets)"
            />{{ wt("仅显示已发布资产") }}</label
          >
          <p v-if="inputLoading" class="wb-muted" role="status">正在确认输入资产类型…</p>
          <p v-if="inputError" class="wb-error" role="alert">{{ inputError }}</p>
          <button v-if="inputError" type="button" @click="resolveSelectedInput">重试读取输入资产</button>
          <ExpertOptionsForm
            v-for="stage in parameterStages"
            :key="stage.id"
            :stage-id="stage.id"
            :agent-id="stage.agent_id"
            :title="`${isPipeline ? stage.id + ' · ' : ''}${agents.find((a) => a.id === stage.agent_id)?.name || stage.agent_id}`"
            :model-value="optionDrafts[stage.id] || defaultOptions(stage.agent_id)"
            :input-kind="selectedInputKind"
            :targets="publicationTargets"
            @update:model-value="optionDrafts[stage.id] = $event"
          />
          <div v-if="needsPublication && selectedInputKind === 'text'" class="wb-stack">
            <p v-if="targetError" class="wb-error" role="alert">{{ targetError }}</p>
            <p v-else-if="!targetLoading && !publicationTargets.length" class="wb-muted">当前工作空间暂无知识库，请先在知识库页面创建。</p>
            <button v-if="targetHasMore || targetLoading || targetError" type="button" :disabled="targetLoading" @click="loadPublicationTargets">
              {{ targetLoading ? '正在加载知识库…' : targetError ? '重试加载知识库' : '加载更多知识库' }}
            </button>
          </div>
          <button class="primary" :disabled="busy || inputLoading">
            {{ wt("开始执行") }}
          </button>
        </form>
        <hr class="wb-divider" />
        <h3>{{ wt("执行历史") }}</h3>
        <button @click="run(() => loadHistory())">{{ wt("刷新历史") }}</button>
        <div class="wb-list">
          <button
            v-for="item in history"
            :key="item.id"
            :class="{ selected: current?.id === item.id }"
            @click="inspect(item.id)"
          >
            <strong>{{
              item.agent_id || item.template_id || item.id.slice(0, 14)
            }}</strong
            ><small
              >{{ statusLabel(item.status) }} ·
              {{ new Date(item.created_at).toLocaleString() }}</small
            >
          </button>
        </div>
        <button v-if="next" @click="run(() => loadHistory(true))">
          {{ wt("加载更多") }}
        </button>
      </aside>
      <div class="wb-stack">
        <section v-if="isPipeline && selectedTemplate" class="wb-panel">
          <h2>{{ wt("阶段依赖") }} · {{ selectedTemplate.name }}</h2>
          <ol class="pipeline-dependencies">
            <li v-for="stage in selectedTemplate.stages" :key="stage.id">
              <strong>{{ stage.id }}</strong> · {{ stage.agent_id }}
              <p class="wb-muted">
                {{
                  (stage.depends_on || []).length
                    ? stage.depends_on.join(" + ") + " → " + stage.id
                    : wt("输入资产") + " → " + stage.id
                }}
              </p>
            </li>
          </ol>
        </section>
        <section v-if="current" class="wb-panel">
          <div class="wb-section-head" style="margin-top: 0">
            <h2>{{ wt("执行详情") }}</h2>
            <span class="wb-badge">{{ statusLabel(current.status) }}</span>
          </div>
          <p class="wb-footnote">{{ current.id }}</p>
          <div class="wb-actions">
            <button @click="inspect(current.id)">{{ wt("刷新进度") }}</button
            ><button
              v-if="
                ['queued', 'running', 'awaiting_approval', 'waiting'].includes(
                  current.status,
                )
              "
              class="danger"
              :disabled="busy"
              @click="action('cancel')"
            >
              {{ wt("取消任务") }}</button
            ><button
              v-if="
                isPipeline &&
                ['failed', 'cancelled', 'quarantined'].includes(current.status)
              "
              :disabled="busy"
              @click="action('retry')"
            >
              {{ wt("重试失败任务") }}
            </button>
          </div>
          <div v-if="current.error" class="wb-error">{{ current.error }}</div>
          <div v-if="stages.length" class="pipeline-stages">
            <article
              v-for="stage in stages"
              :key="stage.id"
              class="pipeline-stage"
            >
              <span class="wb-badge">{{ statusLabel(stage.status) }}</span
              ><strong>{{ stage.stage_key }}</strong
              ><small
                >{{ stage.agent_id }} · 第 {{ stage.attempt }} 次执行</small
              >
              <p v-if="stage.updated_at" class="wb-footnote">
                {{ wt("最后更新") }} ·
                {{ new Date(stage.updated_at).toLocaleString() }}
              </p>
              <p v-if="stage.error" class="wb-error">{{ stage.error }}</p>
              <p>{{ stage.output?.summary }}</p>
              <p v-if="stage.output?.gate" class="wb-muted">
                门禁：{{ stage.output.gate.status }} ·
                {{ stage.output.gate.reason }}
              </p>
              <div class="wb-actions">
                <button
                  v-for="ref in stage.output?.refs || []"
                  :key="ref.asset_id"
                  @click="openAsset(ref)"
                >
                  查看 {{ ref.kind || "资产" }}
                </button>
              </div>
            </article>
          </div>
          <div
            v-for="approval in approvals"
            :key="approval.id"
            class="wb-review"
            style="margin-top: 20px"
          >
            <h3>审批 · {{ approval.status }}</h3>
            <p>{{ approval.reason }}</p>
            <p class="wb-muted">
              有效期：{{ approval.expires_at || "未提供" }}
            </p>
            <template
              v-if="['pending', 'awaiting_approval'].includes(approval.status)"
              ><label class="wb-label"
                >{{ wt("审核说明") }}<input v-model="approvalReason"
              /></label>
              <div class="wb-actions" style="margin-top: 10px">
                <button
                  :disabled="busy"
                  @click="approve(approval.id, 'approved')"
                >
                  {{ wt("批准并继续") }}</button
                ><button
                  :disabled="busy"
                  @click="approve(approval.id, 'rejected')"
                >
                  {{ wt("拒绝") }}
                </button>
              </div></template
            >
          </div>
        </section>
        <section v-else class="wb-panel wb-empty">
          选择专家与资产启动任务，或从历史查看执行结果。
        </section>
        <DecisionCard
          v-if="output"
          :summary="output.summary"
          :decision="output.decision"
          :refs="
            output.decision?.evidence_refs ||
            output.gate?.evidence_refs ||
            output.refs ||
            []
          "
          @open="openAsset"
        />
        <section v-if="current?.output?.refs?.length" class="wb-panel">
          <h3>执行产出</h3>
          <div class="wb-actions">
            <button v-for="ref in current.output.refs" :key="`${ref.asset_id}:${ref.version}`" @click="openAsset(ref)">查看产出 · {{ ref.kind || '资产' }} · v{{ ref.version }}</button>
          </div>
        </section>
        <section v-if="assetDetail" class="wb-panel">
          <div class="wb-section-head">
            <h2>资产内容 · v{{ assetDetail.ref.version }}</h2>
            <button @click="assetDetail = undefined">收起</button>
          </div>
          <p class="wb-muted">
            来源：{{ assetDetail.ref.producer_id }} ·
            {{ assetDetail.ref.kind }} · {{ assetDetail.ref.status }}
          </p>
          <AnalysisReport :content="assetDetail.content" />
          <details><summary>原始资产内容</summary><pre class="wb-code">{{
            typeof assetDetail.content === "string"
              ? assetDetail.content
              : JSON.stringify(assetDetail.content, null, 2)
          }}</pre></details>
        </section>
        <details class="wb-panel" :open="sourcePrefilled">
          <summary>{{ wt("创建输入资产") }}</summary>
          <form
            class="wb-stack"
            style="margin-top: 18px"
            @submit.prevent="createAsset"
          >
            <label class="wb-label"
              >{{ wt("资产名称") }}<input v-model="assetName" required /></label
            ><label class="wb-label"
              >{{ wt("数据类型")
              }}<select v-model="inputKind">
                <option value="text">文本</option>
                <option value="table">表格</option>
                <option value="web">网页 URL</option>
                <option value="knowledge">已有知识文档</option>
                <option value="datasource">数据源表</option>
                <option value="collection">采集运行产物</option>
                <option value="result">已有查询结果</option>
                <option value="evaluation">评测结果</option>
              </select></label
            ><label v-if="inputKind === 'text'" class="wb-label"
              >{{ wt("文本内容")
              }}<textarea v-model="inputText" required rows="6" /></label
            ><template v-else-if="inputKind === 'table'"
              ><label class="wb-label"
                >{{ wt("列名")
                }}<input
                  v-model="inputColumns"
                  required
                  placeholder="date,revenue,region" /></label
              ><p class="wb-muted">列默认按文本保留；需要均值等数值统计时，请将对应列设为数值。编号、账号等应保留文本类型。</p>
              <label v-for="field in rowFields" :key="field.key" class="wb-label">{{ field.label }} 类型
                <select :aria-label="`${field.label} 类型`" :value="columnTypes[field.key] || 'text'" @change="columnTypes[field.key] = ($event.target as HTMLSelectElement).value as 'text' | 'number'">
                  <option value="text">文本</option><option value="number">数值</option>
                </select>
              </label>
              <RecordEditor
                v-model="inputRows"
                title="数据行"
                :fields="rowFields" /></template
            ><label v-else-if="inputKind === 'web'" class="wb-label"
              >网页地址<input v-model="sourceURL" type="url" required /></label
            ><label v-else-if="inputKind === 'knowledge'" class="wb-label"
              >知识文档 ID<input v-model="knowledgeID" required /></label
            ><template
              v-else-if="
                ['collection', 'result', 'evaluation'].includes(inputKind)
              "
              ><label v-if="inputKind !== 'result'" class="wb-label"
                >运行 / 评测任务 ID<input
                  v-model="sourceRunID"
                  required /></label
              ><label v-if="inputKind === 'collection'" class="wb-label"
                >采集条目 ID<input v-model="sourceItemID" required /></label
              ><template v-if="inputKind === 'result'"
                ><label class="wb-label"
                  >会话 ID<input v-model="sourceSessionID" required /></label
                ><label class="wb-label"
                  >查询结果 ID<input
                    v-model="sourceResultID"
                    required /></label></template></template
            ><template v-else
              ><p class="wb-muted">导入当前表最多 1,000 行的快照。</p><label class="wb-label"
                >数据源<select v-model="datasourceID" required>
                  <option value="">请选择</option>
                  <option v-for="ds in datasources" :key="ds.id" :value="ds.id">
                    {{ ds.name }}
                  </option>
                </select></label
              ><label class="wb-label"
                >Schema<input
                  v-model="sourceSchema"
                  placeholder="数据库schema" /></label
              ><label class="wb-label"
                >表名<input v-model="sourceTable" required /></label></template
            ><button class="primary" :disabled="busy">
              {{ wt("创建资产") }}
            </button>
          </form>
        </details>
      </div>
    </div>
  </div>
</template>
<script setup lang="ts">
import { useCursorOptions } from "@/composables/useCursorOptions";
import { wt } from "@/i18n/workbench";

import { computed, onMounted, onBeforeUnmount, reactive, ref, watch } from "vue";
import { useRoute } from "vue-router";
import { useAuthStore } from "@/stores/auth";
import { http } from "@/utils/request";
import {
  lifecycleApi,
  type Agent,
  type Template,
  type AssetRef,
  type RefID,
  type Invocation,
  type Stage,
  type Approval,
  type RunDetail,
  type StageOutput,
  type ExpertOptions,
  type PublicationTarget,
} from "@/api/lifecycle";
import DecisionCard from "@/components/workbench/DecisionCard.vue";
import AnalysisReport from "@/components/workbench/AnalysisReport.vue";
import RecordEditor from "@/components/workbench/RecordEditor.vue";
import ExpertOptionsForm from "./ExpertOptionsForm.vue";
import { defaultOptions, validatedOptions } from "./expertOptions";
import "@/components/workbench/workbench.css";
const route = useRoute(),
  isAnalysis = computed(() => route.path === "/analysis"),
  isPipeline = computed(() => route.path.startsWith("/pipelines")),
  resource = computed(() => "runs" as "runs" | "invocations");
const agents = ref<Agent[]>([]),
  templates = ref<Template[]>([]),
  assets = ref<AssetRef[]>([]),
  history = ref<Invocation[]>([]),
  current = ref<Invocation>(),
  stages = ref<Stage[]>([]),
  approvals = ref<Approval[]>([]),
  executor = ref(""),
  assetSelection = ref(""),
  publishedOnly = ref(isAnalysis.value),
  busy = ref(false),
  error = ref(""),
  notice = ref(""),
  next = ref(""),
  approvalReason = ref(""),
  assetDetail = ref<{ ref: AssetRef; content: unknown }>(),
  assetName = ref(""),
  inputKind = ref("text"),
  inputText = ref(""),
  inputColumns = ref(""),
  inputRows = ref<Record<string, unknown>[]>([]);
const columnTypes = reactive<Record<string, 'text' | 'number'>>(Object.create(null));
const sourceRunID = ref(""),
  sourceItemID = ref(""),
  sourceSessionID = ref(""),
  sourceResultID = ref("");
const sourceURL = ref(""),
  knowledgeID = ref(""),
  datasourceID = ref(""),
  sourceSchema = ref(""),
  sourceTable = ref(""),
  datasources = ref<{ id: string; name: string }[]>([]);
const sourcePrefilled = ref(false);
// Navigation only prepares an editable draft. It must never collect or publish on arrival.
watch(() => route.query, () => {
  const queryString = (key: string) => typeof route.query[key] === "string" ? String(route.query[key]).trim() : "";
  const source = queryString("source");
  const fields: Record<string, string[]> = { collection: ["run_id", "item_id"], knowledge: ["knowledge_id"], evaluation: ["run_id"], result: ["session_id", "result_id"] };
  const required = fields[source];
  if (!Array.isArray(required)) return;
  if (!required.every(key => queryString(key))) {
    if (sourcePrefilled.value) {
      inputKind.value = "text";
      assetName.value = sourceRunID.value = sourceItemID.value = knowledgeID.value = sourceSessionID.value = sourceResultID.value = "";
      sourcePrefilled.value = false;
    }
    return;
  }
  inputKind.value = source;
  sourceRunID.value = ["collection", "evaluation"].includes(source) ? queryString("run_id") : "";
  sourceItemID.value = source === "collection" ? queryString("item_id") : "";
  knowledgeID.value = source === "knowledge" ? queryString("knowledge_id") : "";
  sourceSessionID.value = source === "result" ? queryString("session_id") : "";
  sourceResultID.value = source === "result" ? queryString("result_id") : "";
  assetName.value = queryString("name") || "导入来源";
  sourcePrefilled.value = true;
}, { immediate: true });
const selectedTemplate = computed(() =>
  templates.value.find(
    (t) => t.id === (current.value?.template_id || executor.value),
  ),
);
const parameterStages = computed(() => isPipeline.value
  ? templates.value.find((t) => t.id === executor.value)?.stages || []
  : executor.value ? [{ id: executor.value, agent_id: executor.value }] : []);
const optionDrafts = reactive<Record<string, ExpertOptions>>({});
const needsPublication = computed(() => parameterStages.value.some((stage) => stage.agent_id === "data-asset"));
const selectedInputKind = ref(""), inputLoading = ref(false), inputError = ref("");
const publicationTargets = ref<PublicationTarget[]>([]), targetLoading = ref(false), targetError = ref(""), targetHasMore = ref(false);
let targetPage = 1, targetLoaded = false, targetEpoch = 0, inputEpoch = 0;
let inputPending: Promise<void> | undefined;
watch(() => parameterStages.value.map((stage) => `${stage.id}:${stage.agent_id}`).join("|"), () => {
  for (const key of Object.keys(optionDrafts)) delete optionDrafts[key];
  for (const stage of parameterStages.value) optionDrafts[stage.id] = defaultOptions(stage.agent_id);
}, { immediate: true, flush: "sync" });
async function loadPublicationTargets() {
  if (targetLoading.value || (targetLoaded && !targetHasMore.value)) return;
  const epoch = targetEpoch, page = targetPage;
  targetLoading.value = true;
  targetError.value = "";
  try {
    const result = (await lifecycleApi.knowledgeBases(page)).data;
    if (disposed || epoch !== targetEpoch) return;
    if (!result || !Array.isArray(result.items)) throw new Error("知识库列表格式异常，请重试");
    const byID = new Map(publicationTargets.value.map((target) => [target.id, target]));
    for (const target of result.items) byID.set(target.id, target);
    publicationTargets.value = [...byID.values()];
    targetHasMore.value = page * result.size < result.total;
    if (targetHasMore.value && !result.items.length) throw new Error("知识库分页异常，请刷新页面重试");
    targetPage = page + 1;
    targetLoaded = true;
  } catch (e) {
    if (!disposed && epoch === targetEpoch) targetError.value = e instanceof Error ? e.message : "知识库加载失败";
  } finally {
    if (!disposed && epoch === targetEpoch) targetLoading.value = false;
  }
}
function resolveSelectedInput(): Promise<void> {
  const epoch = ++inputEpoch, selected = assetSelection.value;
  selectedInputKind.value = "";
  inputError.value = "";
  inputLoading.value = false;
  inputPending = undefined;
  if (!selected || !needsPublication.value) return Promise.resolve();
  const asset = assets.value.find((a) => `${a.asset_id}:${a.version}` === selected);
  if (!asset) return Promise.resolve();
  inputLoading.value = true;
  const promise = (async () => {
    try {
      const result = (await lifecycleApi.asset(asset.asset_id, asset.version)).data;
      if (disposed || epoch !== inputEpoch || selected !== assetSelection.value) return;
      const content = result?.content;
      const kind = content && typeof content === "object" && "kind" in content ? content.kind : undefined;
      if (kind !== "text" && kind !== "table") throw new Error("输入资产不是可发布的文本或表格");
      selectedInputKind.value = kind;
    } catch (e) {
      if (!disposed && epoch === inputEpoch) inputError.value = e instanceof Error ? e.message : "输入资产读取失败";
    } finally {
      if (!disposed && epoch === inputEpoch) { inputLoading.value = false; inputPending = undefined; }
    }
  })();
  inputPending = promise;
  return promise;
}
watch(needsPublication, (needed) => { if (needed) void loadPublicationTargets(); });
watch([assetSelection, needsPublication], () => { void resolveSelectedInput(); });
const selectedAgent = computed(() =>
    agents.value.find((a) => a.id === executor.value),
  ),
  output = computed<StageOutput | undefined>(
    () =>
      current.value?.output ||
      [...stages.value].reverse().find((s) => s.output)?.output,
  ),
  rowFields = computed(() =>
    inputColumns.value
      .split(/[,，]/)
      .map((v) => v.trim())
      .filter(Boolean)
      .map((v) => ({ key: v, label: v, type: columnTypes[v] === 'number' ? 'number' as const : 'text' as const })),
  );
const kind = () => (isPipeline.value ? resource.value : "invocations");
const statusLabel = (status: string) =>
  ({
    queued: "已排队",
    running: "执行中",
    waiting: "等待依赖",
    awaiting_approval: "等待审批",
    succeeded: "已完成",
    failed: "失败",
    cancelled: "已取消",
    quarantined: "已隔离",
  })[status] || status;
const assetOptions = useCursorOptions(assets, (cursor) =>
  lifecycleApi.assets(publishedOnly.value ? "published_asset" : "", cursor),
);

let activeOperations = 0;
async function run(fn: () => Promise<void>) {
  activeOperations++;
  busy.value = true;
  error.value = "";
  notice.value = "";
  try {
    await fn();
  } catch (e) {
    error.value = e instanceof Error ? e.message : "操作失败";
  } finally {
    busy.value = --activeOperations > 0;
  }
}
async function loadAssets() {
  await assetOptions.reset();
}
let historyRequest = 0;
async function loadHistory(append = false) {
  if (disposed) return;
  const request = ++historyRequest;
  const res = await lifecycleApi.list(kind(), append ? next.value : "");
  if (disposed || request !== historyRequest) return;
  history.value = append
    ? [...history.value, ...(res.data?.items || [])]
    : res.data?.items || [];
  next.value = res.data?.next_cursor || "";
}
let detailTarget = "",
  detailRequest = 0,
  disposed = false;
let detailPending: { id: string; promise: Promise<void> } | undefined;
function loadDetail(id: string, select = false): Promise<void> {
  if (select) {
    detailTarget = id;
    current.value = undefined;
    stages.value = [];
    approvals.value = [];
    detailPending = undefined;
  }
  if (disposed || id !== detailTarget) return Promise.resolve();
  if (detailPending?.id === id) return detailPending.promise;
  const request = ++detailRequest;
  const promise = (async () => {
    const res = await lifecycleApi.get(kind(), id);
    if (
      disposed ||
      request !== detailRequest ||
      id !== detailTarget ||
      !res.data
    )
      return;
    if ("run" in res.data) {
      const d = res.data as RunDetail;
      current.value = d.run;
      stages.value = d.stages || [];
      approvals.value = d.approvals || [];
    } else {
      current.value = res.data.invocation;
      stages.value = [];
      approvals.value = res.data.approvals || [];
    }
  })()
    .catch((error) => {
      if (!disposed && request === detailRequest) throw error;
    })
    .finally(() => {
      if (request === detailRequest) detailPending = undefined;
    });
  detailPending = { id, promise };
  return promise;
}
async function inspect(id: string) {
  await run(() => loadDetail(id, true));
}
async function start() {
  if (busy.value) return;
  await run(async () => {
    const asset = assets.value.find(
      (a) => `${a.asset_id}:${a.version}` === assetSelection.value,
    );
    if (!asset) throw new Error("请选择输入资产");
    if (!parameterStages.value.length) throw new Error("请选择可用的专家或流水线模板");
    const selected = assetSelection.value, selectedExecutor = executor.value;
    if (needsPublication.value) {
      if (inputPending) await inputPending;
      if (disposed) return;
      if (selected !== assetSelection.value || selectedExecutor !== executor.value) throw new Error("任务选择已改变，请重新提交");
      if (inputError.value) throw new Error(inputError.value);
      if (!selectedInputKind.value) throw new Error("请先确认输入资产类型");
    }
    const options = Object.fromEntries(parameterStages.value.map((stage) => [stage.id, validatedOptions(stage.agent_id, optionDrafts[stage.id] || defaultOptions(stage.agent_id), selectedInputKind.value, publicationTargets.value)]));
    const data = {
      ...(isPipeline.value
        ? { template_id: executor.value }
        : { agent_id: executor.value }),
      input_refs: [{ asset_id: asset.asset_id, version: asset.version }],
      options: isPipeline.value ? { stage_options: options } : options[executor.value],
    };
    const res = await lifecycleApi.start(kind(), data);
    if (disposed) return;
    await loadHistory();
    if (res.data) await loadDetail(res.data.id, true);
  });
}
async function action(action: "retry" | "cancel") {
  if (busy.value) return;
  if (!current.value) return;
  await run(async () => {
    const id = current.value!.id;
    await lifecycleApi.action(kind(), id, action);
    await loadDetail(id);
    await loadHistory();
  });
}
async function approve(id: string, decision: string) {
  if (busy.value) return;
  await run(async () => {
    await lifecycleApi.approve(id, decision, approvalReason.value);
    if (current.value) await loadDetail(current.value.id);
  });
}
let assetDetailRequest = 0;
async function openAsset(ref: RefID) {
  const request = ++assetDetailRequest;
  assetDetail.value = undefined;
  await run(async () => {
    try {
      const result = await lifecycleApi.asset(ref.asset_id, ref.version);
      if (!disposed && request === assetDetailRequest) assetDetail.value = result.data;
    } catch (error) {
      if (!disposed && request === assetDetailRequest) throw error;
    }
  });
}
async function createAsset() {
  if (busy.value) return;
  await run(async () => {
    const columns = rowFields.value.map((c) => c.key);
    const input =
      inputKind.value === "text"
        ? { kind: "text", text: inputText.value }
        : {
            kind: "table",
            columns,
            rows: inputKind.value === "table" ? inputRows.value.map((row, index) =>
              columns.map((col) => tableCell(row[col], col, index)),
            ) : [],
          };
    const source =
      inputKind.value === "web"
        ? { kind: "web", url: sourceURL.value }
        : inputKind.value === "knowledge"
          ? { kind: "knowledge", knowledge_id: knowledgeID.value }
          : inputKind.value === "datasource"
            ? {
                kind: "datasource",
                datasource_id: datasourceID.value,
                schema: sourceSchema.value,
                table: sourceTable.value,
              }
            : inputKind.value === "collection"
              ? { kind: "collection", run_id: sourceRunID.value, item_id: sourceItemID.value }
              : inputKind.value === "evaluation"
                ? { kind: "evaluation", run_id: sourceRunID.value }
                : inputKind.value === "result"
                  ? { kind: "result", session_id: sourceSessionID.value, result_id: sourceResultID.value }
                  : undefined;
    const res = await lifecycleApi.createAsset(input, assetName.value, source);
    if (disposed) return;
    publishedOnly.value = false;
    await loadAssets();
    if (res.data) {
      const created = res.data;
      if (
        !assets.value.some(
          (a) =>
            a.asset_id === created.asset_id && a.version === created.version,
        )
      )
        assets.value.unshift(created);
      assetSelection.value = `${created.asset_id}:${created.version}`;
    }
    notice.value = "输入资产已创建，可选择专家进行处理与发布。";
  });
}
function tableCell(value: unknown, column: string, index: number): string | number | null {
  const text = String(value ?? "");
  if (columnTypes[column] !== "number") return text;
  if (!text.trim()) return null;
  const number = Number(text);
  if (!/^[+-]?(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?$/.test(text.trim()) || !Number.isFinite(number)) {
    throw new Error(`第 ${index + 1} 行「${column}」必须是有限数值`);
  }
  if (Number.isInteger(number) && !Number.isSafeInteger(number)) {
    throw new Error(`第 ${index + 1} 行「${column}」超出安全整数范围，请使用文本类型保留原值`);
  }
  return number;
}
let timer: ReturnType<typeof setInterval> | undefined;
onMounted(() => {
  run(async () => {
    const [a, t] = await Promise.all([
      lifecycleApi.agents(),
      lifecycleApi.templates(),
    ]);
    agents.value = a.data?.agents || [];
    if (isAnalysis.value) executor.value = "data-analysis";
    templates.value = t.data?.templates || [];
    await Promise.all([loadAssets(), loadHistory()]);
    datasources.value =
      (
        await http.get<{ items: { id: string; name: string }[] }>(
          "/datasources",
          { params: { page_size: 100 } },
        )
      ).data?.items || [];
    if (route.query.asset) {
      const [assetID, assetVersion] = String(route.query.asset).split(":");
      const requestedVersion = Number(assetVersion || route.query.version || 1);
      const ref = assets.value.find(
        (a) => a.asset_id === assetID && a.version === requestedVersion,
      );
      if (ref) await openAsset(ref);
      else await openAsset({ asset_id: assetID, version: requestedVersion });
    }
  });
  timer = setInterval(async () => {
    if (
      !busy.value &&
      current.value &&
      ["queued", "running", "waiting", "awaiting_approval"].includes(
        current.value.status,
      )
    ) {
      try {
        await loadDetail(current.value.id);
      } catch (e) {
        error.value = e instanceof Error ? e.message : "进度更新失败";
      }
    }
  }, 4000);
});
let events: EventSource | undefined;
watch(
  () =>
    `${current.value?.id || ""}:${current.value?.status || ""}:${useAuthStore().token}`,
  () => {
    const id = current.value?.id;
    events?.close();
    events = undefined;
    if (
      id &&
      ["queued", "running", "waiting", "awaiting_approval"].includes(
        current.value?.status || "",
      ) &&
      isPipeline.value &&
      typeof EventSource !== "undefined"
    ) {
      events = new EventSource(
        `/api/v1/lifecycle/runs/${encodeURIComponent(id)}/events?token=${encodeURIComponent(useAuthStore().token)}`,
      );
      const refreshProgress = () => {
        if (!busy.value)
          loadDetail(id).catch((e) => {
            error.value = e instanceof Error ? e.message : "事件更新失败";
          });
      };
      events.onmessage = refreshProgress;
      for (const event of [
        "stage.started",
        "stage.succeeded",
        "stage.failed",
        "stage.cancelled",
        "stage.quarantined",
        "approval.requested",
        "approval.approved",
        "approval.rejected",
        "approval.expired",
      ])
        events.addEventListener(event, refreshProgress);
      // EventSource reconnects with Last-Event-ID after transient disconnects.
      // Polling continues as fallback; refresh notifications cannot change selection.
      events.onerror = () => {
        /* keep the browser-managed reconnect alive */
      };
    }
  },
);
onBeforeUnmount(() => {
  disposed = true;
  detailRequest++;
  targetEpoch++;
  inputEpoch++;
  assetDetailRequest++;
  if (timer) clearInterval(timer);
  events?.close();
});
watch(isPipeline, () => {
  current.value = undefined;
  stages.value = [];
  executor.value = "";
  run(() => loadHistory());
});
</script>
<style scoped>
.pipeline-dependencies {
  display: grid;
  gap: 12px;
  padding-left: 24px;
}
.pipeline-dependencies li {
  border-left: 2px solid var(--border-default);
  padding-left: 12px;
}
.pipeline-stages {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(230px, 1fr));
  gap: 12px;
  margin-top: 20px;
}
.pipeline-stage {
  padding: 16px;
  border: 1px solid var(--border-default);
  border-radius: 6px;
}
.pipeline-stage strong {
  display: block;
  margin: 10px 0 6px;
}
.pipeline-stage small {
  color: var(--text-muted);
}
summary {
  cursor: pointer;
  font-weight: 600;
  font-size: 14px;
}
</style>
