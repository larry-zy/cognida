<template>
  <div class="workbench">
    <header class="wb-head">
      <div>
        <span class="wb-kicker">MODELS · MANAGED TRANSFORMS</span>
        <h1>数仓建模</h1>
        <p>
          固定来源与业务粒度，核对真实转换结果；候选审核通过后再切换活动版本。
        </p>
      </div>
      <button :disabled="busy" @click="refresh">刷新列表与状态</button>
    </header>
    <p v-if="error" role="alert" class="wb-error">{{ error }}</p>
    <p v-if="notice" role="status" class="wb-success">{{ notice }}</p>
    <div class="wb-grid">
      <aside class="wb-panel">
        <h2>模型版本</h2>
        <div class="wb-list">
          <button
            v-for="m in models"
            :key="m.id"
            :class="{ selected: selected?.id === m.id }"
            @click="choose(m.id)"
          >
            <strong>{{ m.definition.name }}</strong
            ><small
              >v{{ m.version }} ·
              {{ m.active_ref ? "已有活动产物" : "尚未批准产物" }}</small
            >
          </button>
        </div>
        <p v-if="!models.length" class="wb-empty">
          暂无模型。先选已授权快照并声明粒度。
        </p>
        <button
          v-if="cursor"
          :disabled="busy"
          @click="action(() => loadModels(true))"
        >
          加载更多模型</button
        ><button :disabled="busy" @click="newDraft">新建模型</button>
      </aside>
      <div class="wb-stack">
        <form
          class="wb-panel"
          aria-label="建模方案"
          @submit.prevent="previewDraft"
        >
          <h2>{{ selected ? "创建下一模型版本" : "定义建模方案" }}</h2>
          <p class="wb-footnote">
            来源列表仅供选择，服务端会复查权限。外部数据源沿已有只读采集入口生成快照；这里仅生成受管派生资产。
          </p>
          <fieldset :disabled="busy">
            <div class="wb-form">
              <label
                >模型名称<input
                  v-model="draft.name"
                  required
                  maxlength="200" /></label
              ><label
                >主题域<input
                  v-model="draft.subject"
                  required
                  maxlength="200" /></label
              ><label
                >最终事实粒度<input
                  v-model="draft.grain"
                  required
                  placeholder="例如每区域一行" /></label
              ><label
                >输出唯一键<input
                  :value="draft.grain_keys.join(', ')"
                  required
                  @input="
                    draft.grain_keys = names(
                      ($event.target as HTMLInputElement).value,
                    )
                  " /></label
              ><label
                >事实来源步骤<input
                  v-model="draft.fact_node"
                  required
                  placeholder="orders" /></label
              ><label
                >稳定事实键<input
                  :value="draft.fact_keys.join(', ')"
                  required
                  @input="
                    draft.fact_keys = names(
                      ($event.target as HTMLInputElement).value,
                    )
                  " /></label
              ><label
                >事件时间列（增量必填）<input
                  v-model="draft.event_field" /></label
              ><label
                >最终输出步骤<input v-model="draft.output_node" required
              /></label>
            </div>
            <details>
              <summary>绑定现有语义口径（可选）</summary>
              <div class="wb-form">
                <label
                  >语义模型 ID<input v-model="draft.semantic_model_id" /></label
                ><label
                  >固定语义版本<input
                    v-model.number="draft.semantic_version"
                    type="number"
                    min="0"
                /></label>
              </div>
              <p class="wb-footnote">
                将核对当前租户、固定版本和实际引用；不会自动发布语义模型。
              </p>
            </details>
            <WarehouseNodeEditor
              v-model="draft.nodes"
              :assets="assets"
              :disabled="busy"
            />
            <button
              v-if="assetOptions.state.hasMore || assetOptions.state.error"
              type="button"
              :disabled="assetOptions.state.loading"
              @click="action(() => assetOptions.loadMore())"
            >
              {{
                assetOptions.state.error ? "重试来源列表" : "加载更多来源资产"
              }}
            </button>
            <p class="wb-footnote">
              每个快照最多 10,000 行、32 列、100,000 单元格。更新使用稳定键
              upsert；不检测删除；增量迟到记录拒绝并提示显式回填。表达式保留精确十进制，不开放
              SQL、DDL 或任意脚本。
            </p>
            <div class="wb-actions">
              <button class="primary" :disabled="busy">
                预览真实转换与影响</button
              ><button
                type="button"
                :disabled="
                  busy ||
                  !preview ||
                  preview.definition_digest !== previewDigest
                "
                @click="save"
              >
                保存固定模型版本
              </button>
            </div>
          </fieldset>
        </form>
        <section v-if="preview" class="wb-panel" aria-label="建模预览">
          <h2>候选预览 · {{ preview.total_rows }} 行</h2>
          <ol class="stages">
            <li v-for="s in preview.stages" :key="s.node">
              <strong>{{ layer(s.layer) }} · {{ s.node }}</strong
              ><span>{{ s.input_rows }} 输入 → {{ s.output_rows }} 输出</span>
            </li>
          </ol>
          <div class="wb-table-wrap">
            <table>
              <thead>
                <tr>
                  <th v-for="c in preview.output.columns" :key="c">{{ c }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(row, i) in preview.exact_rows" :key="i">
                  <td v-for="(cell, j) in row" :key="j">{{ exact(cell) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <p>显示前 {{ preview.exact_rows.length }} 行；已校验全部有界输入。</p>
          <details>
            <summary>列血缘与变更影响</summary>
            <p v-for="(line, i) in preview.lineage" :key="i">
              {{ line.node }}.{{ line.column }} ←
              {{ line.source_ref || line.input_node }}.{{
                line.input_column || "记录计数"
              }}
            </p>
            <p v-for="(impact, i) in preview.impact" :key="i">
              {{ impact.unknown ? "未知影响" : "已登记依赖" }} ·
              {{ impact.name || impact.kind }} <small>{{ impact.id }}</small>
            </p>
          </details>
          <p
            v-for="limitation in preview.limitations"
            :key="limitation"
            class="wb-footnote"
          >
            {{ limitation }}
          </p>
        </section>
        <section v-if="selected" class="wb-panel" aria-label="受管转换任务">
          <h2>
            {{ selected.definition.name }} · 固定版本 v{{ selected.version }}
          </h2>
          <p>
            活动产物：<RouterLink
              v-if="selected.active_ref"
              :to="assetLink(selected.active_ref)"
              >查看当前批准版本</RouterLink
            ><span v-else>尚未批准</span> · 水位
            {{
              selected.watermark
                ? new Date(selected.watermark).toLocaleString()
                : "未建立"
            }}
          </p>
          <form @submit.prevent="createRun">
            <fieldset :disabled="busy || dirty">
              <div class="wb-form">
                <label
                  >加工模式<select v-model="mode">
                    <option value="full">完整候选</option>
                    <option value="incremental">增量候选</option>
                    <option value="backfill">历史回填候选</option>
                  </select></label
                ><label
                  >事实新快照<select v-model="factSource">
                    <option value="">使用模型固定来源</option>
                    <option
                      v-for="a in assets"
                      :key="a.asset_id + ':' + a.version"
                      :value="a.asset_id + ':' + a.version"
                    >
                      {{ a.name || a.asset_id }} · v{{ a.version }}
                    </option>
                  </select></label
                ><template v-if="mode === 'backfill'"
                  ><label
                    >回填起始（含）<input
                      v-model="from"
                      type="datetime-local"
                      required /></label
                  ><label
                    >回填截止（不含）<input
                      v-model="to"
                      type="datetime-local"
                      required /></label
                ></template>
              </div>
              <button class="primary">生成受管候选</button>
            </fieldset>
          </form>
          <p v-if="dirty" class="wb-footnote">
            方案已修改。请先预览并保存新版本，再执行固定版本。
          </p>
          <div class="wb-list">
            <button v-for="r in runs" :key="r.id" @click="chooseRun(r.id)">
              <strong>{{ status(r.status) }}</strong
              ><small
                >{{ r.request.mode }} · v{{ r.request.model_version }} ·
                {{ r.partitions.length }} 分区</small
              >
            </button>
          </div>
          <button
            v-if="runCursor"
            :disabled="busy"
            @click="action(() => loadRuns(true))"
          >
            加载更多任务
          </button>
        </section>
        <section v-if="current" class="wb-panel" aria-label="数仓任务详情">
          <div class="wb-section-head">
            <h2>{{ status(current.status) }}</h2>
            <span class="wb-badge"
              >{{ current.partitions.length }} 个真实分区回执</span
            >
          </div>
          <p v-if="current.error" role="alert" class="wb-error">
            {{ current.error }}
          </p>
          <p v-if="current.status === 'awaiting_approval'">
            转换与实际血缘已登记。核对候选后，管理员可批准切换；原活动产物在批准前保持不变。
          </p>
          <p v-if="current.status === 'cancel_requested'">
            取消请求已记录，等待执行器确认终态。
          </p>
          <ol class="stages">
            <li v-for="s in current.stages" :key="s.node">
              <strong>{{ layer(s.layer) }} · {{ s.node }}</strong
              ><span>{{ s.input_rows }} → {{ s.output_rows }} 行</span>
            </li>
          </ol>
          <div class="wb-actions">
            <RouterLink
              v-if="current.output_ref"
              :to="assetLink(current.output_ref)"
              >查看候选数据</RouterLink
            ><RouterLink
              v-if="current.published_ref"
              :to="assetLink(current.published_ref)"
              >查看已批准产物</RouterLink
            ><button
              v-if="current.status === 'awaiting_approval'"
              class="primary"
              :disabled="busy"
              @click="control('approve')"
            >
              管理员批准并切换</button
            ><button
              v-if="
                ['queued', 'running', 'awaiting_approval'].includes(
                  current.status,
                )
              "
              :disabled="busy"
              @click="control('cancel')"
            >
              请求取消
            </button>
          </div>
          <details>
            <summary>分区与审计引用</summary>
            <p v-for="part in current.partitions" :key="part.index">
              分区 {{ part.index + 1 }} · {{ part.count }} 行 ·
              {{ part.committed_at }}
              <RouterLink :to="assetLink(part.output_ref)"
                >查看真实分区</RouterLink
              >
            </p>
            <small
              >任务 {{ current.id }} ·
              {{
                current.lineage_registered
                  ? "实际血缘已登记"
                  : "尚未完成血缘登记"
              }}</small
            >
          </details>
        </section>
      </div>
    </div>
  </div>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import {
  useRoute,
  useRouter,
  onBeforeRouteLeave,
  onBeforeRouteUpdate,
} from "vue-router";
import { useAuthStore } from "@/stores/auth";
import { lifecycleApi, type AssetRef } from "@/api/lifecycle";
import { useCursorOptions } from "@/composables/useCursorOptions";
import {
  warehouseApi as api,
  type WarehouseDefinition,
  type WarehouseModel,
  type WarehouseRun,
  type WarehousePreview,
} from "@/api/warehouse";
import WarehouseNodeEditor from "./WarehouseNodeEditor.vue";
import "@/components/workbench/workbench.css";
const auth = useAuthStore(),
  route = useRoute(),
  router = useRouter();
const blank = (): WarehouseDefinition => ({
  name: "",
  subject: "",
  grain: "",
  grain_keys: ["id"],
  fact_node: "source",
  fact_keys: ["id"],
  event_field: "",
  update_policy: "upsert",
  delete_policy: "unsupported",
  late_policy: "reject_incremental_use_backfill",
  nodes: [{ id: "source", layer: "raw", kind: "source", source_ref: "" }],
  output_node: "source",
});
const draft = ref(blank()),
  baseline = ref(JSON.stringify(draft.value)),
  models = ref<WarehouseModel[]>([]),
  selected = ref<WarehouseModel>(),
  preview = ref<WarehousePreview>(),
  previewDigest = ref(""),
  runs = ref<WarehouseRun[]>([]),
  current = ref<WarehouseRun>(),
  assets = ref<AssetRef[]>([]),
  cursor = ref(""),
  runCursor = ref(""),
  busy = ref(false),
  error = ref(""),
  notice = ref(""),
  mode = ref("full"),
  factSource = ref(""),
  from = ref(""),
  to = ref("");
const assetOptions = useCursorOptions(assets, (cursor) =>
  lifecycleApi.assets("", cursor),
);
const dirty = computed(() => JSON.stringify(draft.value) !== baseline.value);
let generation = 0,
  detailEpoch = 0,
  poll: ReturnType<typeof setTimeout> | undefined;
const names = (s: string) =>
  s
    .split(",")
    .map((v) => v.trim())
    .filter(Boolean);
const layer = (s: string) =>
  ({
    raw: "原始层",
    detail: "明细层",
    summary: "汇总层",
    application: "应用层",
  })[s] || s;
const status = (s: string) =>
  ({
    queued: "等待加工",
    running: "正在生成候选",
    awaiting_approval: "候选待审批",
    approved: "已批准并切换",
    cancel_requested: "取消待确认",
    cancelled: "已取消",
    failed: "生成失败",
  })[s] || s;
const assetLink = (id: string) => ({ path: "/analysis", query: { asset: id } });
const exact = (text: string) =>
  text.startsWith('"') ? JSON.parse(text) : text;
function clear() {
  generation++;
  detailEpoch++;
  clearTimeout(poll);
  busy.value = false;
  error.value = "";
  notice.value = "";
  models.value = [];
  selected.value = undefined;
  preview.value = undefined;
  current.value = undefined;
  runs.value = [];
  cursor.value = "";
  runCursor.value = "";
  draft.value = blank();
  baseline.value = JSON.stringify(draft.value);
  mode.value = "full";
  factSource.value = "";
  from.value = "";
  to.value = "";
  assets.value = [];
}
async function action(fn: (epoch: number) => Promise<void>) {
  if (busy.value) return;
  const epoch = generation;
  busy.value = true;
  error.value = "";
  try {
    await fn(epoch);
  } catch (e) {
    if (epoch === generation) {
      const status = (e as { response?: { status?: number } })?.response
        ?.status;
      if (status && [401, 403, 404].includes(status)) {
        selected.value = undefined;
        current.value = undefined;
        preview.value = undefined;
        runs.value = [];
        draft.value = blank();
        baseline.value = JSON.stringify(draft.value);
        clearTimeout(poll);
      }
      error.value =
        e instanceof Error ? e.message : "操作失败，请核对权限和版本";
    }
  } finally {
    if (epoch === generation) busy.value = false;
  }
}
async function loadModels(append = false) {
  const epoch = generation;
  const r = await api.models(append ? cursor.value : "");
  if (epoch !== generation) return;
  models.value = append
    ? [...models.value, ...(r.data?.items || [])]
    : r.data?.items || [];
  cursor.value = r.data?.next_cursor || "";
}
async function loadRuns(append = false) {
  const epoch = generation,
    id = selected.value?.id;
  if (!id) return;
  const r = await api.runs(id, append ? runCursor.value : "");
  if (epoch !== generation || selected.value?.id !== id) return;
  runs.value = append
    ? [...runs.value, ...(r.data?.items || [])]
    : r.data?.items || [];
  runCursor.value = r.data?.next_cursor || "";
}
async function openModel(id: string) {
  const epoch = generation,
    seq = ++detailEpoch;
  selected.value = undefined;
  current.value = undefined;
  runs.value = [];
  preview.value = undefined;
  const r = await api.model(id);
  if (epoch !== generation || seq !== detailEpoch) return;
  if (!r.data) throw new Error("模型不存在或无权访问");
  selected.value = r.data;
  draft.value = structuredClone(r.data.definition);
  baseline.value = JSON.stringify(draft.value);
  await loadRuns();
}
async function choose(id: string) {
  await router.push({ path: "/warehouse", query: { model_id: id } });
}
async function chooseRun(id: string) {
  await router.push({ path: "/warehouse", query: { run_id: id } });
}
function newDraft() {
  if (dirty.value && !window.confirm("放弃当前未保存的建模修改？")) return;
  detailEpoch++;
  selected.value = undefined;
  current.value = undefined;
  runs.value = [];
  preview.value = undefined;
  draft.value = blank();
  baseline.value = JSON.stringify(draft.value);
  void router.replace({ path: "/warehouse", query: {} });
}
async function previewDraft() {
  const key = JSON.stringify(draft.value);
  await action(async (epoch) => {
    const r = await api.preview(JSON.parse(key));
    if (epoch !== generation || key !== JSON.stringify(draft.value)) return;
    preview.value = r.data;
    previewDigest.value = r.data?.definition_digest || "";
  });
}
async function save() {
  if (!preview.value) return;
  const key = JSON.stringify(draft.value);
  await action(async (epoch) => {
    const r = await api.save(JSON.parse(key), selected.value);
    if (epoch !== generation || key !== JSON.stringify(draft.value) || !r.data)
      return;
    selected.value = r.data;
    draft.value = structuredClone(r.data.definition);
    baseline.value = JSON.stringify(draft.value);
    notice.value = `固定版本 v${r.data.version} 已保存，尚未生成或批准产物`;
    await loadModels();
  });
  if (selected.value && !dirty.value && !error.value)
    await router.replace({
      path: "/warehouse",
      query: { model_id: selected.value.id },
    });
}
function schedule() {
  clearTimeout(poll);
  if (
    current.value &&
    ["queued", "running", "cancel_requested"].includes(current.value.status)
  )
    poll = setTimeout(() => {
      if (busy.value) {
        schedule();
        return;
      }
      void openRun(current.value!.id).catch((e) => {
        error.value = e instanceof Error ? e.message : "状态读取失败";
      });
    }, 4000);
}
async function openRun(id: string) {
  if (current.value?.id !== id) current.value = undefined;
  const epoch = generation,
    seq = ++detailEpoch;
  try {
    const r = await api.run(id);
    if (epoch !== generation || seq !== detailEpoch) return;
    if (!r.data) throw new Error("任务不存在或无权访问");
    current.value = r.data;
    schedule();
  } catch (e) {
    if (epoch !== generation || seq !== detailEpoch) return;
    current.value = undefined;
    clearTimeout(poll);
    throw e;
  }
}
async function createRun() {
  if (!selected.value || dirty.value) return;
  await action(async (epoch) => {
    const r = await api.createRun({
      model_id: selected.value!.id,
      model_version: selected.value!.version,
      mode: mode.value,
      fact_source_ref: factSource.value || undefined,
      ...(mode.value === "backfill"
        ? {
            event_from: new Date(from.value).toISOString(),
            event_to: new Date(to.value).toISOString(),
          }
        : {}),
    });
    if (epoch !== generation || !r.data) return;
    current.value = r.data;
    schedule();
    await loadRuns();
  });
  if (current.value && !error.value)
    await router.replace({
      path: "/warehouse",
      query: { run_id: current.value.id },
    });
}
async function control(kind: "approve" | "cancel") {
  if (!current.value) return;
  const snapshot = current.value;
  await action(async (epoch) => {
    const r = await api.control(snapshot, kind);
    if (epoch !== generation || current.value?.id !== snapshot.id) return;
    current.value = r.data;
    schedule();
    if (kind === "approve" && r.data) {
      const model = await api.model(r.data.request.model_id);
      if (epoch !== generation) return;
      selected.value = model.data;
      await loadModels();
    }
    await loadRuns();
  });
}
async function refresh() {
  if (dirty.value && !window.confirm("刷新将放弃未保存的建模修改。继续？"))
    return;
  await initialize();
}
async function initialize() {
  await action(async (epoch) => {
    await Promise.all([loadModels(), assetOptions.reset()]);
    if (epoch !== generation) return;
    const run =
      typeof route.query.run_id === "string" ? route.query.run_id : "";
    const model =
      typeof route.query.model_id === "string" ? route.query.model_id : "";
    if (run) {
      await openRun(run);
      if (epoch !== generation || !current.value) return;
      const result = await api.model(
        current.value.request.model_id,
        current.value.request.model_version,
      );
      if (epoch !== generation) return;
      selected.value = result.data;
      if (result.data) {
        draft.value = structuredClone(result.data.definition);
        baseline.value = JSON.stringify(draft.value);
        await loadRuns();
      }
    } else if (model) await openModel(model);
  });
}
watch(
  () => JSON.stringify(draft.value),
  () => {
    preview.value = undefined;
    previewDigest.value = "";
  },
);
watch(
  () => `${auth.authEpoch}:${auth.user?.id}:${auth.currentTenant?.id}`,
  () => {
    clear();
    void initialize();
  },
);
watch(
  () => [route.query.run_id, route.query.model_id],
  () => {
    clear();
    void initialize();
  },
);
const guard = () =>
  !busy.value &&
  (!dirty.value || window.confirm("离开将放弃未保存的建模修改。继续？"));
onBeforeRouteLeave(guard);
onBeforeRouteUpdate(guard);
onMounted(initialize);
onBeforeUnmount(() => {
  generation++;
  detailEpoch++;
  clearTimeout(poll);
  assets.value = [];
});
</script>
<style scoped>
.stages {
  display: flex;
  gap: 0.75rem;
  flex-wrap: wrap;
  padding: 0;
  list-style: none;
}
.stages li {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  border-left: 3px solid var(--primary-color);
  padding: 0.5rem 1rem;
}
fieldset {
  border: 0;
  padding: 0;
  min-width: 0;
}
.wb-table-wrap {
  overflow: auto;
}
.wb-grid {
  align-items: start;
}
details {
  margin: 1rem 0;
}
summary {
  cursor: pointer;
}
small {
  overflow-wrap: anywhere;
}
</style>
