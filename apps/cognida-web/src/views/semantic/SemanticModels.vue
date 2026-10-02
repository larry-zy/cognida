<template>
  <div class="workbench">
    <header class="wb-head">
      <div>
        <span class="wb-kicker">GOVERNED METRICS</span>
        <h1>{{ wt("语义建模") }}</h1>
        <p>把指标口径、业务术语与真实数据源连接起来。</p>
      </div>
      <button class="primary" @click="newModel">{{ wt("＋ 新建模型") }}</button>
    </header>
    <div v-if="error" role="alert" class="wb-error">{{ error }}</div>
    <div v-if="notice" role="status" class="wb-success">{{ notice }}</div>
    <div class="wb-grid">
      <aside class="wb-panel">
        <label class="wb-label"
          >{{ wt("模型状态")
          }}<select v-model="status" @change="run(() => load())">
            <option value="all">{{ wt("全部状态") }}</option>
            <option value="draft">草稿</option>
            <option value="active">已发布</option>
            <option value="deprecated">已停用</option>
          </select></label
        >
        <div class="wb-list" style="margin-top: 16px">
          <button
            v-for="model in models"
            :key="model.id"
            :class="{ selected: currentId === model.id }"
            @click="open(model.id)"
          >
            <strong>{{ model.name }}</strong
            ><small>{{ labels[model.status] }} · v{{ model.version }}</small>
          </button>
        </div>
        <p v-if="!models.length" class="wb-empty">
          {{ busy ? "正在加载…" : "暂无模型。新建一个业务域开始。" }}
        </p>
        <button v-if="nextCursor" @click="load(true)">
          {{ wt("加载更多") }}
        </button>
      </aside>
      <form class="wb-panel" @submit.prevent="save">
        <div class="wb-section-head" style="margin-top: 0">
          <h2>{{ currentId ? "编辑模型" : "新建草稿" }}</h2>
          <span v-if="version" class="wb-badge"
            >v{{ version }} · {{ labels[form.status] }}</span
          >
        </div>
        <div class="wb-form">
          <label
            >{{ wt("模型名称")
            }}<input
              v-model="form.name"
              required
              maxlength="100"
              placeholder="例如：订单销售" /></label
          ><label
            >{{ wt("业务说明")
            }}<input
              v-model="form.description"
              placeholder="说明模型覆盖的业务范围"
          /></label>
        </div>
        <RecordEditor
          v-model="form.logical_tables"
          title="逻辑表"
          :fields="tableFields"
          hint="先添加逻辑表并绑定数据源，再定义维度和指标。"
        />
        <RecordEditor
          v-model="form.dimensions"
          title="维度"
          :fields="dimensionFields"
        />
        <div
          v-for="dimension in form.dimensions"
          :key="String(dimension.id)"
          class="wb-review"
          style="margin-top: 12px"
        >
          <div class="wb-section-head">
            <h3>{{ dimension.name || "未命名维度" }} · 值映射</h3>
            <button type="button" @click="addMapping(dimension)">
              ＋ 映射
            </button>
          </div>
          <div
            v-for="(pair, i) in mappings[String(dimension.id)] || []"
            :key="i"
            class="wb-form"
            style="margin-bottom: 8px"
          >
            <input
              v-model="pair.label"
              aria-label="业务标签"
              placeholder="业务标签，例如：已完成"
            />
            <div class="wb-actions">
              <input
                v-model="pair.value"
                aria-label="存储值"
                placeholder="存储值，例如：completed"
                style="flex: 1"
              /><button
                type="button"
                @click="mappings[String(dimension.id)].splice(i, 1)"
              >
                删除
              </button>
            </div>
          </div>
        </div>
        <RecordEditor
          v-model="form.measures"
          title="度量"
          :fields="measureFields"
        />
        <RecordEditor
          v-model="form.metrics"
          title="指标"
          :fields="metricFields"
        />
        <RecordEditor
          v-model="form.relations"
          title="关系"
          :fields="relationFields"
        />
        <hr class="wb-divider" />
        <div class="wb-actions">
          <button class="primary" :disabled="busy">
            {{ busy ? "处理中…" : "保存模型" }}</button
          ><button
            v-if="currentId && form.status !== 'active'"
            type="button"
            :disabled="busy"
            @click="transition('publish')"
          >
            {{ wt("保存并发布") }}</button
          ><button
            v-if="currentId && form.status === 'active'"
            type="button"
            :disabled="busy"
            @click="transition('deprecate')"
          >
            {{ wt("停用模型") }}</button
          ><button
            v-if="currentId"
            type="button"
            :disabled="busy"
            @click="diagnose"
          >
            {{ wt("诊断值映射") }}
          </button>
        </div>
        <p class="wb-footnote">
          发布后参与数据分析的口径匹配。停用保留模型内容与历史版本信息。
        </p>
        <div v-if="coverage" class="wb-metrics">
          <div>
            <strong>{{ (coverage.hit_ratio * 100).toFixed(1) }}%</strong
            ><small>治理命中率</small>
          </div>
          <div>
            <strong>{{ coverage.total }}</strong
            ><small>查询总数</small>
          </div>
          <div>
            <strong>{{ coverage.fallback }}</strong
            ><small>未覆盖回退</small>
          </div>
        </div>
        <div v-if="diagnosis !== null">
          <h3>值映射诊断</h3>
          <pre class="wb-code">{{ diagnosis }}</pre>
        </div>
      </form>
    </div>
  </div>
</template>
<script setup lang="ts">
import { wt } from "@/i18n/workbench";

import { computed, onMounted, reactive, ref } from "vue";
import RecordEditor, {
  type EditorField,
} from "@/components/workbench/RecordEditor.vue";
import "@/components/workbench/workbench.css";
import {
  semanticApi,
  type SemanticInput,
  type SemanticHead,
  type SemanticBundle,
  type Coverage,
  type SemanticRow,
} from "@/api/semantic";
import { http } from "@/utils/request";
const fresh = (): SemanticInput => ({
  name: "",
  description: "",
  status: "draft",
  logical_tables: [],
  dimensions: [],
  measures: [],
  metrics: [],
  relations: [],
});
const form = reactive(fresh()),
  models = ref<SemanticHead[]>([]),
  status = ref("all"),
  nextCursor = ref(""),
  currentId = ref(""),
  version = ref(0),
  busy = ref(false),
  error = ref(""),
  notice = ref(""),
  diagnosis = ref<unknown>(null),
  stats = ref<Coverage[]>([]),
  datasources = ref<{ id: string; name: string }[]>([]);
const mappings = reactive<Record<string, { label: string; value: string }[]>>(
  {},
);
const labels: Record<string, string> = {
  draft: "草稿",
  active: "已发布",
  deprecated: "已停用",
};
const tables = computed(() =>
  form.logical_tables.map((row) => ({
    value: String(row.id),
    label: String(row.name || "未命名表"),
  })),
);
const tableFields = computed<EditorField[]>(() => [
  { key: "name", label: "逻辑名" },
  { key: "physical_table", label: "物理表" },
  {
    key: "database_id",
    label: "数据源",
    options: datasources.value.map((d) => ({ value: d.id, label: d.name })),
  },
  { key: "primary_key", label: "主键" },
]);
const dimensionFields = computed<EditorField[]>(() => [
  { key: "logical_table_id", label: "逻辑表", options: tables.value },
  { key: "name", label: "业务名称" },
  { key: "expr", label: "列或表达式" },
  {
    key: "data_type",
    label: "类型",
    options: ["string", "number", "date", "datetime", "boolean"].map((v) => ({
      value: v,
      label: v,
    })),
  },
  { key: "synonyms", label: "同义词", list: true, placeholder: "逗号分隔" },
]);
const measureFields = computed<EditorField[]>(() => [
  { key: "logical_table_id", label: "逻辑表", options: tables.value },
  { key: "name", label: "度量名称" },
  { key: "expr", label: "表达式" },
  {
    key: "aggregation",
    label: "聚合",
    options: ["sum", "count", "avg", "min", "max", "none"].map((v) => ({
      value: v,
      label: v,
    })),
  },
]);
const metricFields: EditorField[] = [
  { key: "name", label: "指标名" },
  { key: "expr", label: "计算表达式", placeholder: "SUM(orders.amount)" },
  { key: "caliber", label: "口径说明" },
  {
    key: "format",
    label: "格式",
    options: ["number", "currency", "percent"].map((v) => ({
      value: v,
      label: v,
    })),
  },
  { key: "synonyms", label: "同义词", list: true },
];
const relationFields = computed<EditorField[]>(() => [
  { key: "left_table_id", label: "左表", options: tables.value },
  { key: "right_table_id", label: "右表", options: tables.value },
  {
    key: "join_type",
    label: "连接",
    options: ["left", "inner", "right"].map((v) => ({ value: v, label: v })),
  },
  {
    key: "join_condition",
    label: "连接条件",
    placeholder: "orders.customer_id = customers.id",
  },
]);
const coverage = computed(() => stats.value.find((s) => s.model === form.name));
async function run(fn: () => Promise<void>) {
  busy.value = true;
  error.value = "";
  notice.value = "";
  try {
    await fn();
  } catch (e) {
    error.value = e instanceof Error ? e.message : "操作失败，请重试";
  } finally {
    busy.value = false;
  }
}
async function load(append = false) {
  const res = await semanticApi.list(
    status.value,
    append ? nextCursor.value : "",
  );
  models.value = append
    ? [...models.value, ...(res.data?.items || [])]
    : res.data?.items || [];
  nextCursor.value = res.data?.next_cursor || "";
}
function apply(bundle: SemanticBundle) {
  currentId.value = bundle.model.id;
  version.value = bundle.model.version;
  Object.assign(form, {
    ...bundle,
    name: bundle.model.name,
    description: bundle.model.description,
    status: bundle.model.status,
  });
  for (const key of Object.keys(mappings)) delete mappings[key];
  for (const d of form.dimensions)
    mappings[String(d.id)] = Object.entries(
      (d.value_map || {}) as Record<string, string>,
    ).map(([label, value]) => ({ label, value }));
  diagnosis.value = null;
}
function newModel() {
  currentId.value = "";
  version.value = 0;
  Object.assign(form, fresh());
  error.value = "";
  notice.value = "";
  diagnosis.value = null;
}
async function open(id: string) {
  await run(async () => {
    const res = await semanticApi.get(id);
    if (res.data) apply(res.data);
  });
}
function addMapping(d: SemanticRow) {
  (mappings[String(d.id)] ||= []).push({ label: "", value: "" });
}
function input() {
  return {
    ...form,
    dimensions: form.dimensions.map((d) => ({
      ...d,
      value_map: Object.fromEntries(
        (mappings[String(d.id)] || [])
          .filter((p) => p.label.trim())
          .map((p) => [p.label.trim(), p.value]),
      ),
    })),
  };
}
async function save() {
  await run(async () => {
    const res = await semanticApi.save(currentId.value || undefined, input());
    if (res.data) apply(res.data);
    await load();
    notice.value = "模型已保存";
  });
}
async function transition(action: "publish" | "deprecate") {
  await run(async () => {
    if (action === "publish") {
      const saved = await semanticApi.save(currentId.value, input());
      if (saved.data) apply(saved.data);
    }
    const res = await semanticApi.transition(currentId.value, action);
    if (res.data) apply(res.data);
    await load();
    notice.value = action === "publish" ? "模型已发布" : "模型已停用";
  });
}
async function diagnose() {
  await run(async () => {
    const res = await semanticApi.diagnose(currentId.value);
    diagnosis.value = res.data?.enabled
      ? res.data.report
      : "画像诊断未启用，请先为数据源生成列画像。";
  });
}
onMounted(() =>
  run(async () => {
    await load();
    const [ds, cv] = await Promise.all([
      http.get<{ items: { id: string; name: string }[] }>("/datasources", {
        params: { page_size: 100 },
      }),
      semanticApi.coverage(),
    ]);
    datasources.value = ds.data?.items || [];
    stats.value = cv.data?.items || [];
  }),
);
</script>
