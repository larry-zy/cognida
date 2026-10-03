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
              <option value="">选择已发布资产</option>
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
          ><label class="wb-label"
            >{{ wt("分析目标")
            }}<textarea
              v-model="prompt"
              placeholder="例如：分析近一个月收入的变化与主要原因"
            /></label
          ><button class="primary" :disabled="busy">
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
        <section v-if="assetDetail" class="wb-panel">
          <div class="wb-section-head">
            <h2>资产内容 · v{{ assetDetail.ref.version }}</h2>
            <button @click="assetDetail = undefined">收起</button>
          </div>
          <p class="wb-muted">
            来源：{{ assetDetail.ref.producer_id }} ·
            {{ assetDetail.ref.kind }} · {{ assetDetail.ref.status }}
          </p>
          <pre class="wb-code">{{
            typeof assetDetail.content === "string"
              ? assetDetail.content
              : JSON.stringify(assetDetail.content, null, 2)
          }}</pre>
        </section>
        <details class="wb-panel">
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
              ><RecordEditor
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
              ><label class="wb-label"
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

import { computed, onMounted, onBeforeUnmount, ref, watch } from "vue";
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
} from "@/api/lifecycle";
import DecisionCard from "@/components/workbench/DecisionCard.vue";
import RecordEditor from "@/components/workbench/RecordEditor.vue";
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
  prompt = ref(""),
  publishedOnly = ref(true),
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
const selectedTemplate = computed(() =>
  templates.value.find(
    (t) => t.id === (current.value?.template_id || executor.value),
  ),
);
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
      .map((v) => ({ key: v, label: v })),
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
    const data = {
      ...(isPipeline.value
        ? { template_id: executor.value }
        : { agent_id: executor.value }),
      input_refs: [{ asset_id: asset.asset_id, version: asset.version }],
      options: { prompt: prompt.value },
    };
    const res = await lifecycleApi.start(kind(), data);
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
async function openAsset(ref: RefID) {
  await run(async () => {
    assetDetail.value = (
      await lifecycleApi.asset(ref.asset_id, ref.version)
    ).data;
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
            rows: inputRows.value.map((row) =>
              columns.map((col) => row[col] ?? ""),
            ),
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
            : ["collection", "result", "evaluation"].includes(inputKind.value)
              ? {
                  kind: inputKind.value,
                  run_id: sourceRunID.value,
                  item_id: sourceItemID.value,
                  session_id: sourceSessionID.value,
                  result_id: sourceResultID.value,
                }
              : undefined;
    const res = await lifecycleApi.createAsset(input, assetName.value, source);
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
      else
        assetDetail.value = (
          await lifecycleApi.asset(assetID, requestedVersion)
        ).data;
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
