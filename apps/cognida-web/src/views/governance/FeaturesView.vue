<template>
  <div class="workbench">
    <header class="wb-head">
      <div>
        <span class="wb-kicker">VERSIONED FEATURES</span>
        <h1>{{ wt("特征工程") }}</h1>
        <p>
          定义字段、表达式或时间窗口，预览计算结果，再物化为可追溯的特征资产。
        </p>
      </div>
      <button class="primary" :disabled="busy" @click="newFeature">
        {{ wt("＋ 注册特征") }}
      </button>
    </header>
    <div v-if="error" role="alert" class="wb-error">{{ error }}</div>
    <div v-if="notice" role="status" class="wb-success">{{ notice }}</div>
    <section v-if="execution" class="wb-panel" aria-label="领域执行结果">
      <div class="wb-section-head">
        <h2>领域执行结果</h2>
        <span class="wb-badge">{{ executionStatus(execution.status) }}</span>
      </div>
      <p class="wb-footnote">恢复已保存的执行状态；打开本页不会再次运行。</p>
      <div class="wb-actions">
        <RouterLink
          v-for="(output, index) in execution.output_refs"
          :key="`${output.asset_id}:${output.version}`"
          :to="{
            path: '/analysis',
            query: { asset: `${output.asset_id}:${output.version}` },
          }"
          >查看执行产物 {{ index + 1 }}</RouterLink
        >
      </div>
    </section>
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
      <aside class="wb-panel">
        <div class="wb-list">
          <button
            v-for="feature in features"
            :key="feature.id"
            :class="{ selected: form.id === feature.id }"
            @click="select(feature)"
          >
            <strong>{{ feature.name }}</strong
            ><small>{{ feature.type }} · v{{ feature.version }}</small>
          </button>
        </div>
        <p v-if="!features.length" class="wb-empty">暂无特征定义</p>
        <button v-if="next" @click="run(() => load(true))">
          {{ wt("加载更多") }}
        </button>
      </aside>
      <div class="wb-stack">
        <form class="wb-panel" @submit.prevent="save">
          <h2>{{ form.id ? "定义与版本" : "注册特征" }}</h2>
          <div class="wb-form">
            <label>{{ wt("名称") }}<input v-model="form.name" required /></label
            ><label
              >{{ wt("特征类型")
              }}<select v-model="form.type">
                <option value="numerical">数值</option>
                <option value="categorical">类别</option>
                <option value="vector">向量</option>
              </select></label
            ><label class="wb-full"
              >{{ wt("业务说明")
              }}<textarea v-model="form.description" /></label
            ><label
              >计算方式<select
                v-model="form.definition.kind"
                @change="changeDefinition"
              >
                <option value="field">字段映射</option>
                <option value="expression">表达式转换</option>
                <option value="window_aggregate">时间窗口聚合</option>
                <option
                  value="embedding"
                  :disabled="!embeddingModel?.available"
                >
                  真实模型向量（embedding）
                </option>
              </select></label
            ><label
              >{{ wt("来源资产")
              }}<select v-model="form.source_ref_id" required>
                <option
                  v-if="
                    form.source_ref_id &&
                    !assets.some(
                      (a) =>
                        `${a.asset_id}:${a.version}` === form.source_ref_id,
                    )
                  "
                  :value="form.source_ref_id"
                >
                  当前引用 · {{ form.source_ref_id }}
                </option>
                <option value="">请选择</option>
                <option
                  v-for="asset in assets"
                  :key="`${asset.asset_id}:${asset.version}`"
                  :value="`${asset.asset_id}:${asset.version}`"
                >
                  {{ asset.name || asset.asset_id }}
                </option>
              </select></label
            ><label v-if="form.definition.kind !== 'expression'"
              >{{ wt("来源字段")
              }}<input
                v-model="form.definition.field"
                :required="form.definition.aggregate !== 'count'"
                placeholder="如：total_spend"
            /></label>
          </div>
          <FeatureExpressionEditor
            v-if="
              form.definition.kind === 'expression' &&
              form.definition.expression
            "
            v-model="form.definition.expression"
          />
          <label class="wb-label" style="margin-top: 14px"
            >实体标识字段{{
              form.definition.kind === "window_aggregate"
                ? "（必填）"
                : "（可选）"
            }}<input
              v-model="form.definition.entity_field"
              :required="form.definition.kind === 'window_aggregate'"
              placeholder="如：customer_id；留空按行生成实体ID"
          /></label>
          <label class="wb-label" style="margin-top: 14px"
            >事件时间字段{{
              form.definition.kind === "window_aggregate"
                ? "（必填）"
                : "（可选）"
            }}<input
              v-model="form.definition.event_time_field"
              :required="form.definition.kind === 'window_aggregate'"
              placeholder="如：occurred_at"
          /></label>
          <p
            v-if="form.definition.kind !== 'window_aggregate'"
            class="wb-footnote"
          >
            同一实体有多条记录时，请指定事件时间字段以区分观测。
          </p>
          <div
            v-if="form.definition.kind === 'window_aggregate'"
            class="wb-form"
            style="margin-top: 14px"
          >
            <label
              >窗口秒数<input
                v-model.number="form.definition.window_seconds"
                required
                type="number"
                min="1"
                step="1"
            /></label>
            <label
              >聚合方法<select v-model="form.definition.aggregate">
                <option value="sum">求和</option>
                <option value="mean">均值</option>
                <option value="min">最小值</option>
                <option value="max">最大值</option>
                <option value="count">计数</option>
              </select></label
            >
          </div>
          <div
            v-if="
              form.definition.kind === 'embedding' && form.definition.embedding
            "
            class="wb-form"
          >
            <p class="wb-footnote wb-full">
              固定模型 {{ form.definition.embedding.model_id }} · 修订
              {{
                form.definition.embedding.revision
              }}。预览会实际调用模型；不生成替代向量，也不保证金额上限。持久化请使用分区物化任务。
            </p>
            <label
              >向量维度<input
                v-model.number="form.definition.embedding.dimension"
                type="number"
                min="1"
                max="4096"
                required
            /></label>
          </div>
          <p v-if="!embeddingModel?.available" class="wb-footnote">
            模型向量当前不可用：{{
              embeddingModel?.reason || "尚未加载可信模型配置"
            }}
          </p>
          <p class="wb-footnote">
            同步预览最多计算 1,000 行；分区任务支持完整快照最多 10,000
            行。窗口按实体与事件时间计算 (当前时间 − 窗口,
            当前时间]。已有版本不可覆盖，修改定义将创建新版本。
          </p>
          <div class="wb-actions">
            <button
              type="button"
              :disabled="
                busy ||
                (form.definition.kind === 'embedding' &&
                  !embeddingModel?.available)
              "
              @click="previewDefinition"
            >
              预览计算结果</button
            ><button class="primary" :disabled="busy">
              {{ form.id ? "创建新版本" : "注册特征" }}
            </button>
          </div>
          <section
            v-if="preview"
            aria-label="特征预览"
            style="margin-top: 18px"
          >
            <h3>预览结果 · 未写入</h3>
            <p class="wb-footnote">
              共 {{ preview.total_count }} 个值，展示前
              {{ preview.values.length }} 个。编辑定义后需重新预览。
            </p>
            <p
              v-for="warning in preview.warnings"
              :key="warning"
              class="wb-error"
            >
              {{ warning }}
            </p>
            <table class="wb-table">
              <thead>
                <tr>
                  <th>实体</th>
                  <th>计算值</th>
                  <th>事件时间</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(row, index) in preview.values" :key="index">
                  <td>{{ row.entity_id }}</td>
                  <td>{{ row.value_exact ?? JSON.stringify(row.value) }}</td>
                  <td>{{ row.timestamp }}</td>
                </tr>
              </tbody>
            </table>
            <details>
              <summary>来源与定义摘要</summary>
              <p class="wb-footnote">
                {{ preview.source_digest }}<br />{{ preview.definition_digest }}
              </p>
            </details>
          </section>
          <div v-if="versions.length" style="margin-top: 20px">
            <h3>{{ wt("历史版本") }}</h3>
            <table class="wb-table">
              <thead>
                <tr>
                  <th>版本</th>
                  <th>定义</th>
                  <th>来源</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="version in versions" :key="version.version">
                  <td>v{{ version.version }}</td>
                  <td>
                    {{ kindLabel(version.definition.kind) }} ·
                    {{ version.definition.field }}
                  </td>
                  <td>{{ version.source_ref_id }}</td>
                  <td>
                    <button
                      type="button"
                      @click="valueVersion = version.version"
                    >
                      {{ wt("读取此版本") }}
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </form>
        <FeatureJobsPanel
          v-if="selectedVersion"
          :feature="selectedVersion"
          :assets="assets"
          :job-id="
            typeof route?.query.job_id === 'string'
              ? route.query.job_id
              : undefined
          "
        />
        <section v-if="form.id" class="wb-panel">
          <button
            v-if="versionOptions.state.hasMore || versionOptions.state.error"
            type="button"
            :disabled="versionOptions.state.loading"
            @click="run(versionOptions.loadMore)"
          >
            {{
              versionOptions.state.error
                ? "重试加载历史版本"
                : "加载更多历史版本"
            }}
          </button>
          <h2>{{ wt("实体特征值") }}</h2>
          <button
            v-if="selectedVersion?.definition.kind !== 'embedding'"
            :disabled="busy"
            @click="compute"
          >
            计算并物化所选版本
          </button>
          <div v-if="receipt" class="wb-success" role="status">
            <strong
              >已物化 {{ receipt.count }} 个特征值 · v{{
                receipt.version
              }}</strong
            >
            <p>可用时间：{{ receipt.available_at }}</p>
            <RouterLink
              :to="{
                path: '/analysis',
                query: { asset: receipt.output_ref_id },
              }"
              >查看物化产物</RouterLink
            >
            <details>
              <summary>执行回执</summary>
              <p>
                回执：{{ receipt.receipt_id }}<br />来源：{{
                  receipt.source_ref_id
                }}<br />定义摘要：{{ receipt.definition_digest }}
              </p>
            </details>
          </div>
          <form class="wb-form" @submit.prevent="writeValue">
            <label
              >{{ wt("实体 ID")
              }}<input
                v-model="entityID"
                required
                placeholder="customer-1001" /></label
            ><label
              >特征版本<input
                v-model.number="valueVersion"
                type="number"
                min="1"
                required /></label
            ><label class="wb-full"
              >值<input
                v-model="value"
                required
                :placeholder="
                  selectedVersion?.type === 'vector'
                    ? '0.2, 0.8, 0.5'
                    : '输入特征值'
                " /></label
            ><label
              >观测时间<input
                v-model="timestamp"
                type="datetime-local"
                required
            /></label>
            <div class="wb-actions">
              <button class="primary" :disabled="busy">
                {{ wt("写入特征值") }}
              </button>
            </div>
          </form>
          <hr class="wb-divider" />
          <form class="wb-form" @submit.prevent="query">
            <label
              >读取实体（逗号分隔）<input
                v-model="queryEntities"
                required /></label
            ><label
              >时间上界<input v-model="asOf" type="datetime-local" /></label
            ><button :disabled="busy">{{ wt("批量读取指定版本") }}</button>
          </form>
          <table v-if="values.length" class="wb-table" style="margin-top: 16px">
            <thead>
              <tr>
                <th>实体</th>
                <th>版本</th>
                <th>值</th>
                <th>观测时间 / 来源</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="item in values"
                :key="`${item.entity_id}:${item.feature_id}:${item.version}`"
              >
                <td>{{ item.entity_id }}</td>
                <td>v{{ item.version }}</td>
                <td>{{ item.value_exact ?? JSON.stringify(item.value) }}</td>
                <td>
                  {{ item.timestamp }}<br />可用：{{
                    item.available_at || "未提供"
                  }}<br />{{ item.source_ref_id }}
                </td>
              </tr>
            </tbody>
          </table>
          <p v-else-if="queried" class="wb-empty">
            未找到该实体、版本与时间范围内的特征值。
          </p>
        </section>
      </div>
    </div>
  </div>
</template>
<script setup lang="ts">
import {
  computed,
  onMounted,
  onBeforeUnmount,
  reactive,
  ref,
  watch,
} from "vue";
import { useRoute } from "vue-router";
import { useAuthStore } from "@/stores/auth";
import { useCursorOptions } from "@/composables/useCursorOptions";
import { wt } from "@/i18n/workbench";
import {
  governanceApi as api,
  type Feature,
  type FeatureValue,
  type FeaturePreview,
  type FeatureMaterialization,
  type GovernanceExecution,
  type FeatureExpression,
} from "@/api/governance";
import { lifecycleApi, type AssetRef } from "@/api/lifecycle";
import FeatureExpressionEditor from "./FeatureExpressionEditor.vue";
import FeatureJobsPanel from "./FeatureJobsPanel.vue";
import type {
  EmbeddingModel,
  FeatureJob,
} from "@/api/governance/featureOperations";
const embeddingModel = ref<EmbeddingModel>();
import "@/components/workbench/workbench.css";
const execution = ref<GovernanceExecution>();
const executionStatus = (status: string) =>
  ({
    queued: "排队中",
    running: "执行中",
    completed: "已完成",
    succeeded: "已完成",
    cancelled: "已取消",
    failed: "执行失败",
    cancel_requested: "取消处理中",
  })[status] || status;
const route = useRoute(),
  auth = useAuthStore();
const identity = computed(
  () => `${auth.authEpoch}:${auth.user?.id}:${auth.currentTenant?.id}`,
);
const empty = (): Feature => ({
  id: "",
  version: 0,
  name: "",
  type: "numerical",
  description: "",
  definition: { kind: "field", field: "" },
  source_ref_id: "",
  created_at: "",
  updated_at: "",
});
const form = reactive(empty()),
  features = ref<Feature[]>([]),
  versions = ref<Feature[]>([]),
  assets = ref<AssetRef[]>([]),
  next = ref(""),
  busy = ref(false),
  error = ref(""),
  notice = ref(""),
  entityID = ref(""),
  valueVersion = ref(1),
  value = ref(""),
  timestamp = ref(localTime()),
  queryEntities = ref(""),
  asOf = ref(""),
  values = ref<FeatureValue[]>([]),
  queried = ref(false),
  preview = ref<FeaturePreview>(),
  receipt = ref<FeatureMaterialization>();
let generation = 0,
  scope = 0,
  listRequest = 0,
  activeOperations = 0;
const versionOptions = useCursorOptions(versions, async (cursor) => {
  const epoch = generation;
  const res = await api.list<Feature>(`/features/${form.id}/versions`, cursor);
  current(epoch);
  return res;
});
const assetOptions = useCursorOptions(assets, (cursor) =>
  lifecycleApi.assets("", cursor),
);
function localTime() {
  const date = new Date();
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 16);
}
function current(epoch: number) {
  if (epoch !== generation) throw new Error("请求已失效");
}
async function run(fn: () => Promise<void>) {
  const epoch = generation;
  activeOperations++;
  busy.value = true;
  error.value = "";
  notice.value = "";
  try {
    await fn();
  } catch (e) {
    if (epoch === generation)
      error.value = e instanceof Error ? e.message : "操作失败";
  } finally {
    busy.value = --activeOperations > 0;
  }
}
async function load(append = false) {
  const request = ++listRequest,
    ownScope = scope;
  const res = await api.list<Feature>("/features", append ? next.value : "");
  if (ownScope !== scope || request !== listRequest) return;
  features.value = append
    ? [...features.value, ...(res.data?.items || [])]
    : res.data?.items || [];
  next.value = res.data?.next_cursor || "";
}
function newFeature() {
  execution.value = undefined;
  generation++;
  Object.assign(form, empty());
  versions.value = [];
  values.value = [];
  entityID.value = "";
  value.value = "";
  queryEntities.value = "";
  asOf.value = "";
  queried.value = false;
  preview.value = undefined;
  receipt.value = undefined;
  error.value = "";
  notice.value = "";
}
async function select(item: Feature) {
  newFeature();
  Object.assign(form, JSON.parse(JSON.stringify(item)));
  valueVersion.value = item.version;
  await run(() => versionOptions.reset());
}
const kindLabel = (kind: string) =>
  ({
    field: "字段映射",
    expression: "表达式",
    window_aggregate: "时间窗口聚合",
    embedding: "固定模型向量",
  })[kind] || kind;
function changeDefinition() {
  const kind = form.definition.kind,
    entity = form.definition.entity_field,
    event = form.definition.event_time_field;
  if (kind === "embedding") {
    form.type = "vector";
    form.definition = {
      kind,
      field: "",
      entity_field: entity,
      event_time_field: event,
      embedding: {
        model_id: embeddingModel.value?.model_id || "",
        revision: embeddingModel.value?.revision || "",
        dimension: 1024,
      },
    };
    return;
  }
  if (form.type === "vector") form.type = "numerical";
  form.definition =
    kind === "expression"
      ? {
          kind,
          entity_field: entity,
          event_time_field: event,
          expression: { op: "column", field: "" },
        }
      : kind === "window_aggregate"
        ? {
            kind,
            entity_field: entity,
            field: "",
            event_time_field: event || "",
            window_seconds: 3600,
            aggregate: "sum",
          }
        : { kind, field: "", entity_field: entity, event_time_field: event };
}
const draftKey = computed(() =>
  JSON.stringify([form.name, form.type, form.definition, form.source_ref_id]),
);
watch(draftKey, () => {
  preview.value = undefined;
});
watch(valueVersion, () => {
  receipt.value = undefined;
  values.value = [];
  queried.value = false;
});
function validateExpression(expr?: FeatureExpression): void {
  if (!expr) throw new Error("请配置表达式");
  if (expr.op === "column" && !expr.field?.trim())
    throw new Error("请填写表达式字段名");
  if (
    expr.op === "literal" &&
    (expr.value == null ||
      (typeof expr.value === "number" &&
        (!Number.isFinite(expr.value) ||
          (Number.isInteger(expr.value) && !Number.isSafeInteger(expr.value)))))
  )
    throw new Error("表达式常量必须是有效文本或有限安全数值");
  if (
    expr.op === "to_number" &&
    expr.args?.[0]?.op === "literal" &&
    typeof expr.args[0].value === "string" &&
    (expr.args[0].value.length > 256 ||
      !/^-?(0|[1-9]\d*)(\.\d+)?([eE][+-]?\d+)?$/.test(expr.args[0].value))
  )
    throw new Error("请输入有效的十进制数值");
  for (const arg of expr.args || []) validateExpression(arg);
}
function draft() {
  if (!form.name.trim() || !form.source_ref_id)
    throw new Error("请填写名称并选择来源资产");
  if (
    form.definition.kind === "embedding" &&
    (!embeddingModel.value?.available ||
      form.definition.embedding?.model_id !== embeddingModel.value.model_id ||
      form.definition.embedding?.revision !== embeddingModel.value.revision)
  )
    throw new Error("可信模型配置不可用或已变更，请重新选择模型向量");
  const def = form.definition;
  if (def.kind === "expression") validateExpression(def.expression);
  else if (def.aggregate !== "count" && !def.field?.trim())
    throw new Error("请填写来源字段");
  if (
    def.kind === "window_aggregate" &&
    (!def.entity_field?.trim() ||
      !def.event_time_field?.trim() ||
      !Number.isSafeInteger(def.window_seconds) ||
      Number(def.window_seconds) <= 0)
  )
    throw new Error("窗口聚合需实体字段、事件时间字段与正整数窗口秒数");
  return JSON.parse(
    JSON.stringify({
      name: form.name,
      type: form.type,
      description: form.description,
      definition: form.definition,
      source_ref_id: form.source_ref_id,
    }),
  );
}
async function previewDefinition() {
  if (busy.value) return;
  const epoch = generation,
    key = draftKey.value;
  await run(async () => {
    const res = await api.previewFeature(draft(), 20);
    current(epoch);
    if (key === draftKey.value) preview.value = res.data;
  });
}
async function save() {
  if (busy.value) return;
  const epoch = generation;
  await run(async () => {
    const res = await api.create<Feature>(
      form.id ? `/features/${form.id}/versions` : "/features",
      { ...draft(), expected_version: form.version },
    );
    current(epoch);
    if (!res.data) throw new Error("保存未返回特征版本");
    Object.assign(form, res.data);
    valueVersion.value = form.version;
    await load();
    current(epoch);
    await versionOptions.reset();
    current(epoch);
    notice.value = `版本 v${form.version} 已保存`;
  });
}
const selectedVersion = computed(() => {
  const version =
    versions.value.find((v) => v.version === valueVersion.value) ||
    (form.version === valueVersion.value ? form : undefined);
  // Historical version records have their own storage IDs. Operations target
  // the feature head together with an explicit immutable version number.
  return version ? { ...version, id: form.id } : undefined;
});
function parsedValue() {
  if (!selectedVersion.value) throw new Error("请选择已有的特征版本");
  if (selectedVersion.value.type === "numerical") {
    const exact = value.value.trim();
    if (
      exact.length > 256 ||
      !/^-?(0|[1-9]\d*)(\.\d+)?([eE][+-]?\d+)?$/.test(exact)
    )
      throw new Error("请输入有效的十进制数值");
    return { value: null, value_exact: exact };
  }
  if (selectedVersion.value.type === "vector") {
    const numbers = value.value.split(/[,，]/).map((v) => Number(v.trim()));
    if (!numbers.length || numbers.some((n) => !Number.isFinite(n)))
      throw new Error("向量必须为逗号分隔的有限数值");
    return { value: numbers };
  }
  return { value: value.value };
}
async function writeValue() {
  if (busy.value) return;
  const epoch = generation;
  await run(async () => {
    await api.create("/feature-values", {
      values: [
        {
          feature_id: form.id,
          version: valueVersion.value,
          entity_id: entityID.value,
          ...parsedValue(),
          timestamp: new Date(timestamp.value).toISOString(),
        },
      ],
    });
    current(epoch);
    queryEntities.value = entityID.value;
    notice.value = "特征值已写入";
  });
}
async function compute() {
  if (busy.value) return;
  const epoch = generation,
    id = form.id,
    version = valueVersion.value;
  await run(async () => {
    if (!selectedVersion.value) throw new Error("请选择已有的特征版本");
    const response = await api.action<FeatureMaterialization>(
      `/features/${form.id}/materialize`,
      { version: valueVersion.value },
    );
    current(epoch);
    if (id !== form.id || version !== valueVersion.value) return;
    if (!response.data?.receipt_id || !response.data.output_ref_id)
      throw new Error("物化未返回完整回执，请刷新核查状态");
    receipt.value = response.data;
  });
}
const queryKey = computed(() =>
  JSON.stringify([
    form.id,
    valueVersion.value,
    queryEntities.value,
    asOf.value,
  ]),
);
watch(queryKey, () => {
  values.value = [];
  queried.value = false;
});
async function query() {
  if (busy.value) return;
  const epoch = generation,
    requested = queryKey.value;
  await run(async () => {
    const res = await api.action<FeatureValue[] | { items: FeatureValue[] }>(
      "/feature-values/query",
      {
        entity_ids: queryEntities.value
          .split(/[,，]/)
          .map((v) => v.trim())
          .filter(Boolean),
        features: [{ feature_id: form.id, version: valueVersion.value }],
        ...(asOf.value ? { as_of: new Date(asOf.value).toISOString() } : {}),
      },
    );
    current(epoch);
    if (requested !== queryKey.value) return;
    values.value = Array.isArray(res.data) ? res.data : res.data?.items || [];
    queried.value = true;
  });
}
async function openLink() {
  newFeature();
  const epoch = generation;
  await run(async () => {
    let id =
        typeof route?.query.feature_id === "string"
          ? route.query.feature_id
          : "",
      version = 0;
    const executionID =
      typeof route?.query.execution_id === "string"
        ? route.query.execution_id
        : "";
    const jobID =
      typeof route?.query.job_id === "string" ? route.query.job_id : "";
    if (jobID) {
      const res = await api.get<FeatureJob>(
        `/feature-jobs/${encodeURIComponent(jobID)}`,
      );
      current(epoch);
      if (!res.data) throw new Error("任务不存在或无权查看");
      id = res.data.request.feature_id;
      version = res.data.request.feature_version;
    }
    if (executionID && !jobID) {
      const res = await api.get<GovernanceExecution>(
        `/governance-executions/${encodeURIComponent(executionID)}`,
      );
      current(epoch);
      execution.value = res.data;
      if (!res.data?.feature_id) throw new Error("该执行尚无可查看的特征定义");
      id = res.data.feature_id;
      version = res.data.feature_version || 0;
    }
    if (!id) return;
    const res = await api.get<Feature>(
      `/features/${encodeURIComponent(id)}${version ? `/versions/${version}` : ""}`,
    );
    current(epoch);
    if (!res.data) throw new Error("特征不存在或无权查看");
    Object.assign(form, res.data);
    valueVersion.value = version || res.data.version;
    try {
      await versionOptions.reset();
      current(epoch);
    } catch (e) {
      if (epoch === generation) {
        Object.assign(form, empty());
        versions.value = [];
      }
      throw e;
    }
  });
}
watch(
  () => [
    route?.query.execution_id,
    route?.query.feature_id,
    route?.query.job_id,
  ],
  () => void openLink(),
);
watch(identity, () => {
  scope++;
  newFeature();
  features.value = [];
  assets.value = [];
  embeddingModel.value = undefined;
  next.value = "";
  void initialize();
});
async function initialize() {
  const epoch = generation;
  await run(async () => {
    await Promise.all([load(), assetOptions.reset()]);
    current(epoch);
    try {
      const res = await api.get<EmbeddingModel>("/feature-models");
      current(epoch);
      embeddingModel.value = res?.data;
    } catch {
      if (epoch === generation)
        embeddingModel.value = { available: false, reason: "无法读取模型配置" };
    }
  });
  if (
    epoch === generation &&
    (route?.query.execution_id ||
      route?.query.feature_id ||
      route?.query.job_id)
  )
    await openLink();
}
onMounted(initialize);
onBeforeUnmount(() => {
  scope++;
  generation++;
});
</script>

<style scoped>
.wb-table {
  table-layout: fixed;
}
.wb-table td,
.wb-panel p {
  overflow-wrap: anywhere;
}
</style>
