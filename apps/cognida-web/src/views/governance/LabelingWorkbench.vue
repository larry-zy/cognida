<template>
  <div class="workbench">
    <header class="wb-head">
      <div>
        <span class="wb-kicker">HUMAN REVIEW</span>
        <h1>{{ wt("标注与智能打标") }}</h1>
        <p>
          配置关键词或实体字典，生成待审核候选；也可从真实表格映射问答数据，人工审核后导出。
        </p>
      </div>
      <button class="primary" :disabled="busy" @click="newTask">
        {{ wt("＋ 新建任务") }}
      </button>
    </header>
    <div v-if="error" class="wb-error" role="alert">{{ error }}</div>
    <div v-if="notice" class="wb-success" role="status">{{ notice }}</div>
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
    <div class="wb-tabs">
      <button
        :class="{ selected: mode === 'annotation' }"
        @click="changeMode('annotation')"
      >
        {{ wt("人工标注 / 规则预标注") }}</button
      ><button
        :class="{ selected: mode === 'tagging' }"
        @click="changeMode('tagging')"
      >
        {{ wt("智能打标") }}
      </button>
    </div>
    <div class="wb-grid">
      <aside class="wb-panel">
        <div class="wb-list">
          <button
            v-for="task in tasks"
            :key="task.id"
            :class="{ selected: currentID === task.id }"
            :disabled="busy"
            @click="selectTask(task)"
          >
            <strong>{{ task.name }}</strong
            ><small
              >{{ task.status }} ·
              {{
                task.created_at
                  ? new Date(task.created_at).toLocaleDateString()
                  : ""
              }}</small
            >
          </button>
        </div>
        <p v-if="!tasks.length" class="wb-empty">暂无任务</p>
        <button v-if="next" @click="run(() => load(true))">
          {{ wt("加载更多") }}
        </button>
      </aside>
      <div class="wb-stack">
        <form v-if="!currentID" class="wb-panel" @submit.prevent="create">
          <h2>{{ wt("任务设置") }}</h2>
          <div class="wb-form">
            <label>{{ wt("任务名称") }}<input v-model="name" required /></label
            ><label
              >{{ wt("来源资产")
              }}<select v-model="sourceRef" required>
                <option value="">选择资产</option>
                <option
                  v-for="asset in assets"
                  :key="`${asset.asset_id}:${asset.version}`"
                  :value="`${asset.asset_id}:${asset.version}`"
                >
                  {{ asset.name || asset.asset_id }} · {{ asset.kind }}
                </option>
              </select></label
            ><label v-if="mode === 'annotation'"
              >{{ wt("标注类型")
              }}<select v-model="annotationType">
                <option value="classification">分类</option>
                <option value="named_entity">命名实体</option>
                <option value="relation">{{ wt("关系") }}</option>
                <option value="qa">问答标注</option>
              </select></label
            >
          </div>
          <RecordEditor
            v-if="mode === 'tagging' || annotationType !== 'qa'"
            v-model="labels"
            title="标签"
            :fields="labelFields"
          />
          <div
            v-if="mode === 'annotation' && annotationType === 'qa'"
            class="wb-form"
            style="margin-top: 18px"
          >
            <label
              >问题列<input
                v-model="qaMapping.question_column"
                required
                placeholder="如：question"
            /></label>
            <label
              >参考答案列<input
                v-model="qaMapping.reference_answer_column"
                required
                placeholder="如：answer"
            /></label>
            <p class="wb-footnote wb-full">
              选择表格资产中两个不同的文本列。预标注只复制原始问答，不生成或补造答案。
            </p>
          </div>
          <p v-else class="wb-footnote">
            {{
              annotationType === "named_entity" && mode === "annotation"
                ? "实体词条按 Unicode 原文精确匹配；重叠歧义保留为待确认。"
                : annotationType === "relation" && mode === "annotation"
                  ? "关系采用人工标注，请创建标签后逐项填写实体与关系。"
                  : "关键词以逗号分隔。未命中时保留为未确定，不自动选择标签。"
            }}
          </p>
          <div
            v-if="mode === 'tagging'"
            class="wb-form"
            style="margin-top: 18px"
          >
            <label
              v-for="field in tagOptions"
              :key="field.key"
              class="wb-checkbox"
              ><input v-model="tagConfig[field.key]" type="checkbox" />{{
                field.label
              }}</label
            >
          </div>
          <button class="primary" :disabled="busy" style="margin-top: 18px">
            {{ mode === "annotation" ? "创建标注任务" : "创建并执行打标" }}
          </button>
        </form>
        <section v-else class="wb-panel">
          <div class="wb-section-head" style="margin-top: 0">
            <h2>{{ activeTask?.name }}</h2>
            <span class="wb-badge">{{ activeTask?.status }}</span>
          </div>
          <div class="wb-actions">
            <button @click="run(() => loadItems())">{{ wt("刷新结果") }}</button
            ><template v-if="mode === 'annotation'"
              ><button
                :disabled="
                  busy || (activeTask as AnnotationTask)?.type === 'relation'
                "
                @click="preannotate"
              >
                {{ wt("生成预标注") }}</button
              ><button :disabled="busy" @click="exportData">
                {{ wt("导出 JSONL") }}
              </button></template
            >
          </div>
          <p v-if="'error' in (activeTask || {})" class="wb-error">
            {{ (activeTask as TagTask)?.error }}
          </p>
          <p class="wb-footnote">
            规则结果仍需人工审核；“未确定”表示未匹配或存在歧义，不是低概率预测。修改后需重新审核。导出仅包含审核通过的数据。
          </p>
          <div class="wb-list" style="margin-top: 16px">
            <button
              v-for="item in items"
              :key="item.id"
              :class="{ selected: currentItem?.id === item.id }"
              :disabled="busy"
              @click="selectItem(item)"
            >
              <strong
                >{{ item.content.slice(0, 90)
                }}{{ item.content.length > 90 ? "…" : "" }}</strong
              ><small
                >{{ item.review_status }} · 来源 {{ item.provenance.source }} ·
                {{ decisionLabel(item.provenance.decision_status) }} · 置信
                {{ confidence(item.provenance.confidence) }}</small
              >
            </button>
          </div>
          <p v-if="!items.length" class="wb-empty">
            暂无结果。任务正在执行时可稍后刷新。
          </p>
          <button v-if="itemNext" @click="run(() => loadItems(true))">
            {{ wt("加载更多") }}
          </button>
        </section>
        <section v-if="currentItem" class="wb-panel">
          <h2>{{ wt("审核工作区") }}</h2>
          <blockquote class="labeling-content">
            {{ currentItem.content }}
          </blockquote>
          <p class="wb-muted">
            来源：{{ currentItem.provenance.source }} · 方法：{{
              currentItem.provenance.method
            }}
            · 模型：{{ currentItem.provenance.model || "未提供" }} · 置信度：{{
              confidence(currentItem.provenance.confidence)
            }}
          </p>
          <p
            v-if="currentItem.provenance.decision_status === 'uncertain'"
            class="wb-error"
            role="status"
          >
            未确定：规则未命中或存在歧义，请核对原文并人工修正。
          </p>
          <p
            v-else-if="currentItem.provenance.decision_status"
            class="wb-footnote"
          >
            {{ decisionLabel(currentItem.provenance.decision_status) }}
          </p>
          <ul
            v-if="currentItem.provenance.evidence_spans?.length"
            class="wb-footnote"
            aria-label="规则命中依据"
          >
            <li
              v-for="(span, index) in currentItem.provenance.evidence_spans"
              :key="index"
            >
              {{ span.match }} · 标签 {{ labelName(span.label_id) }} · 字符 [{{
                span.start
              }}, {{ span.end }}) · {{ span.rule }}
            </li>
          </ul>
          <p v-if="currentItem.reviewed_by" class="wb-muted">
            审核人：{{ currentItem.reviewed_by }} ·
            {{ currentItem.review_status }}<br />{{
              currentItem.review_comment
            }}
          </p>
          <form v-if="mode === 'annotation'" @submit.prevent="saveItem">
            <div
              v-if="(activeTask as AnnotationTask)?.type === 'qa'"
              class="wb-form"
            >
              <label class="wb-full"
                >问题<textarea v-model="annotation.qa.question" required />
              </label>
              <label class="wb-full"
                >参考答案<textarea
                  v-model="annotation.qa.reference_answer"
                  required
                />
              </label>
            </div>
            <label
              v-if="(activeTask as AnnotationTask)?.type === 'classification'"
              class="wb-label"
              >{{ wt("分类标签")
              }}<select v-model="annotation.labels" multiple size="4">
                <option
                  v-for="label in (activeTask as AnnotationTask)?.labels || []"
                  :key="label.id"
                  :value="label.id"
                >
                  {{ label.name }}
                </option>
              </select></label
            ><RecordEditor
              v-if="
                ['named_entity', 'relation'].includes(
                  (activeTask as AnnotationTask)?.type,
                )
              "
              v-model="annotation.entities"
              title="实体"
              :fields="entityFields"
            /><RecordEditor
              v-if="(activeTask as AnnotationTask)?.type === 'relation'"
              v-model="annotation.relations"
              title="关系"
              :fields="relationFields"
            />
            <p
              v-if="
                ['named_entity', 'relation'].includes(
                  (activeTask as AnnotationTask)?.type,
                )
              "
              class="wb-footnote"
            >
              实体位置使用 Unicode 字符索引，起点包含、终点不包含。当前文本
              {{ Array.from(currentItem.content).length }} 个字符。
            </p>
            <div class="wb-actions">
              <button class="primary" :disabled="busy">
                {{ wt("保存标注") }}
              </button>
            </div>
          </form>
          <form v-else class="wb-form" @submit.prevent="saveItem">
            <label>{{ wt("标签") }}<input v-model="tagEdit.tags" /></label
            ><label
              >情感<select v-model="tagEdit.sentiment">
                <option value="">未判断</option>
                <option value="positive">正面</option>
                <option value="negative">负面</option>
                <option value="neutral">中性</option>
              </select></label
            ><label>关键词<input v-model="tagEdit.keywords" /></label
            ><label>分类<input v-model="tagEdit.category" /></label
            ><label class="wb-full"
              >摘要<textarea v-model="tagEdit.summary" />
            </label>
            <div class="wb-full">
              <RecordEditor
                v-model="tagEntities"
                title="抽取实体"
                :fields="[
                  { key: 'text', label: '文本' },
                  { key: 'type', label: '类别' },
                  {
                    key: 'label',
                    label: '字典标签',
                    options: (
                      (activeTask as TagTask)?.config?.label_schema || []
                    ).map((label) => ({ value: label.id, label: label.name })),
                  },
                  { key: 'start', label: '起点' },
                  { key: 'end', label: '终点' },
                ]"
              />
            </div>
            <p class="wb-muted wb-full">
              标签置信：{{
                Object.entries((currentItem as TagResult).tag_confidence || {})
                  .map(([tag, score]) => `${tag} ${confidence(score)}`)
                  .join(" · ") || "未提供"
              }}<br />质量评分：{{
                (currentItem as TagResult).quality_score ?? "未评估"
              }}
            </p>
            <button class="primary" :disabled="busy">保存人工修正</button>
          </form>
          <div class="review-controls">
            <hr class="wb-divider" />
            <label class="wb-label"
              >{{ wt("审核说明")
              }}<input v-model="reviewComment" placeholder="说明采纳或退回原因"
            /></label>
            <div class="wb-actions" style="margin-top: 12px">
              <button :disabled="busy" @click="review('approved')">
                {{ wt("审核通过") }}</button
              ><button :disabled="busy" @click="review('rejected')">
                {{ wt("退回修正") }}
              </button>
            </div>
          </div>
        </section>
      </div>
    </div>
  </div>
</template>
<script setup lang="ts">
import { useCursorOptions } from "@/composables/useCursorOptions";
import { wt } from "@/i18n/workbench";

import {
  computed,
  onMounted,
  onBeforeUnmount,
  reactive,
  ref,
  watch,
} from "vue";
import {
  governanceApi as api,
  type AnnotationTask,
  type AnnotationItem,
  type TagTask,
  type TagResult,
  type GovernanceExecution,
} from "@/api/governance";
import { lifecycleApi, type AssetRef } from "@/api/lifecycle";
import RecordEditor, {
  type EditorField,
} from "@/components/workbench/RecordEditor.vue";
import "@/components/workbench/workbench.css";
import { useAuthStore } from "@/stores/auth";
import { useRoute } from "vue-router";
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
let generation = 0,
  scope = 0;
const detailTask = ref<Task>();
type Task = AnnotationTask | TagTask;
type Item = AnnotationItem | TagResult;
const mode = ref("annotation"),
  tasks = ref<Task[]>([]),
  items = ref<Item[]>([]),
  assets = ref<AssetRef[]>([]),
  currentID = ref(""),
  currentItem = ref<Item>(),
  name = ref(""),
  sourceRef = ref(""),
  annotationType = ref("classification"),
  labels = ref<Record<string, unknown>[]>([]),
  qaMapping = reactive({ question_column: "", reference_answer_column: "" }),
  busy = ref(false),
  error = ref(""),
  notice = ref(""),
  next = ref(""),
  itemNext = ref(""),
  reviewComment = ref("");
const tagConfig = reactive({
    enable_tags: true,
    enable_sentiment: true,
    enable_extraction: true,
    enable_classification: true,
  }),
  tagOptions = [
    { key: "enable_tags" as const, label: "自动标签" },
    { key: "enable_sentiment" as const, label: "情感分析" },
    { key: "enable_extraction" as const, label: "实体与关键词" },
    { key: "enable_classification" as const, label: "内容分类" },
  ];
const annotation = reactive({
    labels: [] as string[],
    entities: [] as Record<string, unknown>[],
    relations: [] as Record<string, unknown>[],
    qa: { question: "", reference_answer: "" },
  }),
  tagEdit = reactive({
    tags: "",
    sentiment: "",
    keywords: "",
    summary: "",
    category: "",
  }),
  tagEntities = ref<Record<string, unknown>[]>([]);
const activeTask = computed(() =>
    detailTask.value?.id === currentID.value
      ? detailTask.value
      : tasks.value.find((t) => t.id === currentID.value),
  ),
  base = computed(() =>
    mode.value === "annotation" ? "/annotation-tasks" : "/tagging-tasks",
  );
const labelFields = computed<EditorField[]>(() => [
  { key: "name", label: "标签名称" },
  ...(mode.value === "annotation" && annotationType.value === "relation"
    ? []
    : [
        {
          key:
            mode.value === "annotation" &&
            annotationType.value === "named_entity"
              ? "terms"
              : "keywords",
          label:
            mode.value === "annotation" &&
            annotationType.value === "named_entity"
              ? "实体词条"
              : "匹配关键词",
          list: true,
        },
      ]),
]);
const decisionLabel = (value?: string) =>
  ({ matched: "规则已命中", uncertain: "未确定", manual: "人工修正" })[
    value || ""
  ] ||
  value ||
  "未提供判断依据";
const labelName = (id: string) =>
  (
    (activeTask.value as AnnotationTask)?.labels ||
    (activeTask.value as TagTask)?.config?.label_schema ||
    []
  ).find((l) => l.id === id)?.name || id;
const entityFields = computed<EditorField[]>(() => [
  {
    key: "label",
    label: "标签",
    options: ((activeTask.value as AnnotationTask)?.labels || []).map((l) => ({
      value: l.id,
      label: l.name,
    })),
  },
  { key: "text", label: "实体文本" },
  { key: "start", label: "起点" },
  { key: "end", label: "终点" },
]);
const relationFields = computed<EditorField[]>(() => [
  {
    key: "from",
    label: "来源实体",
    options: annotation.entities.map((e) => ({
      value: String(e.id),
      label: String(e.text),
    })),
  },
  {
    key: "to",
    label: "目标实体",
    options: annotation.entities.map((e) => ({
      value: String(e.id),
      label: String(e.text),
    })),
  },
  {
    key: "label",
    label: "关系标签",
    options: ((activeTask.value as AnnotationTask)?.labels || []).map((l) => ({
      value: l.id,
      label: l.name,
    })),
  },
]);
const split = (v: string) =>
    v
      .split(/[,，]/)
      .map((x) => x.trim())
      .filter(Boolean),
  confidence = (value: number | null | undefined) =>
    value == null ? "未评估" : `${(value * 100).toFixed(1)}%`;
let activeOperations = 0;
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
let taskListEpoch = 0;
async function load(append = false) {
  const source = base.value,
    ownScope = scope,
    request = ++taskListEpoch;
  const res = await api.list<Task>(source, append ? next.value : "");
  if (ownScope !== scope || source !== base.value || request !== taskListEpoch)
    return;
  tasks.value = append
    ? [...tasks.value, ...(res.data?.items || [])]
    : res.data?.items || [];
  next.value = res.data?.next_cursor || "";
}
const assetOptions = useCursorOptions(assets, (cursor) =>
  lifecycleApi.assets("", cursor),
);

function newTask() {
  execution.value = undefined;
  generation++;
  itemsEpoch++;
  detailTask.value = undefined;
  itemNext.value = "";
  error.value = "";
  notice.value = "";
  labels.value = [];
  annotationType.value = "classification";
  qaMapping.question_column = "";
  qaMapping.reference_answer_column = "";
  currentID.value = "";
  currentItem.value = undefined;
  items.value = [];
  name.value = "";
  sourceRef.value = "";
}
async function changeMode(value: string) {
  mode.value = value;
  newTask();
  await run(() => load());
}
async function create() {
  if (busy.value) return;
  await run(async () => {
    const epoch = generation;
    if (!name.value.trim() || !sourceRef.value)
      throw new Error("请填写任务名称并选择来源资产");
    if (
      mode.value === "annotation" &&
      annotationType.value === "qa" &&
      (!qaMapping.question_column.trim() ||
        !qaMapping.reference_answer_column.trim() ||
        qaMapping.question_column === qaMapping.reference_answer_column)
    )
      throw new Error("问答映射需要两个不同的非空文本列名");
    const input =
      mode.value === "annotation"
        ? {
            name: name.value,
            type: annotationType.value,
            source_ref_id: sourceRef.value,
            labels: annotationType.value === "qa" ? [] : labels.value,
            ...(annotationType.value === "qa"
              ? { qa_mapping: { ...qaMapping } }
              : {}),
          }
        : {
            name: name.value,
            source_ref_id: sourceRef.value,
            config: { ...tagConfig, label_schema: labels.value },
          };
    const res = await api.create<Task>(base.value, input);
    if (epoch !== generation) return;
    await load();
    if (res.data && epoch === generation) {
      detailTask.value = res.data;
      currentID.value = res.data.id;
      await loadItems();
    }
  });
}
async function selectTask(task: Task) {
  generation++;
  itemsEpoch++;
  items.value = [];
  itemNext.value = "";
  detailTask.value = task;
  currentID.value = task.id;
  currentItem.value = undefined;
  await run(() => loadItems());
}
let itemsEpoch = 0;
async function loadItems(append = false) {
  const id = currentID.value,
    epoch = generation,
    source = base.value,
    request = ++itemsEpoch;
  const suffix = mode.value === "annotation" ? "items" : "results";
  const res = await api.list<Item>(
    `${base.value}/${currentID.value}/${suffix}`,
    append ? itemNext.value : "",
  );
  if (
    epoch !== generation ||
    id !== currentID.value ||
    source !== base.value ||
    request !== itemsEpoch
  )
    return;
  items.value = append
    ? [...items.value, ...(res.data?.items || [])]
    : res.data?.items || [];
  itemNext.value = res.data?.next_cursor || "";
  await load();
}
let annotationSnapshot = "",
  tagSnapshot = "";
function selectItem(item: Item) {
  currentItem.value = JSON.parse(JSON.stringify(item));
  reviewComment.value = "";
  if ("annotation" in item) {
    Object.assign(
      annotation,
      JSON.parse(
        JSON.stringify({
          labels: item.annotation?.labels || [],
          entities: item.annotation?.entities || [],
          relations: item.annotation?.relations || [],
          qa: item.annotation?.qa || { question: "", reference_answer: "" },
        }),
      ),
    );
    annotationSnapshot = JSON.stringify(annotationValue());
  } else {
    Object.assign(tagEdit, {
      tags: (item.tags || []).join(", "),
      sentiment: item.sentiment,
      keywords: (item.keywords || []).join(", "),
      summary: item.summary,
      category: item.category,
    });
    tagEntities.value = JSON.parse(JSON.stringify(item.entities || []));
    tagSnapshot = JSON.stringify(tagValue());
  }
}
const entities = (rows: Record<string, unknown>[]) =>
  rows.map((row) => ({
    ...row,
    start: Number(row.start),
    end: Number(row.end),
  }));
function annotationValue() {
  return {
    labels: annotation.labels,
    entities: entities(annotation.entities),
    relations: annotation.relations,
    ...((activeTask.value as AnnotationTask)?.type === "qa"
      ? { qa: { ...annotation.qa } }
      : {}),
  };
}
function tagValue() {
  return {
    ...tagEdit,
    tags: split(tagEdit.tags),
    keywords: split(tagEdit.keywords),
    entities: entities(tagEntities.value),
  };
}
async function persist() {
  if (!currentItem.value) return;
  const epoch = generation,
    itemID = currentItem.value.id;
  const suffix = mode.value === "annotation" ? "items" : "results";
  const data =
    mode.value === "annotation"
      ? {
          annotation: annotationValue(),
          expected_version: currentItem.value.version,
        }
      : {
          ...tagValue(),
          expected_version: currentItem.value.version,
        };
  const res = await api.update<Item>(
    `${base.value}/${currentID.value}/${suffix}/${currentItem.value.id}`,
    data,
  );
  if (epoch !== generation || itemID !== currentItem.value?.id)
    throw new Error("当前审核项已切换，请重新操作");
  if (res.data) selectItem(res.data);
  await loadItems();
}
async function saveItem() {
  if (busy.value) return;
  await run(async () => {
    await persist();
    notice.value = "人工修正已保存";
  });
}
async function review(decision: string) {
  if (busy.value) return;
  await run(async () => {
    if (!currentItem.value) return;
    const epoch = generation,
      originalItem = currentItem.value.id;
    const ensureCurrent = () => {
      if (epoch !== generation || originalItem !== currentItem.value?.id)
        throw new Error("当前审核项已切换，请重新操作");
    };
    if (mode.value === "annotation") {
      const itemID = currentItem.value.id;
      const comment = reviewComment.value;
      if (JSON.stringify(annotationValue()) !== annotationSnapshot)
        await persist();
      ensureCurrent();
      await api.action(
        `${base.value}/${currentID.value}/items/${itemID}/review`,
        { decision, comment, expected_version: currentItem.value?.version },
      );
      ensureCurrent();
      await loadItems();
      ensureCurrent();
      const updated = items.value.find((i) => i.id === itemID);
      if (updated) selectItem(updated);
    } else {
      const itemID = currentItem.value.id,
        comment = reviewComment.value;
      if (JSON.stringify(tagValue()) !== tagSnapshot) await persist();
      ensureCurrent();
      const response = await api.action<TagResult>(
        `/tagging-results/${itemID}/review`,
        { decision, comment, expected_version: currentItem.value?.version },
      );
      ensureCurrent();
      if (response.data) selectItem(response.data);
      await loadItems();
    }
    notice.value = decision === "approved" ? "审核通过" : "已退回修正";
  });
}
async function preannotate() {
  if (busy.value || (activeTask.value as AnnotationTask)?.type === "relation")
    return;
  const epoch = generation;
  await run(async () => {
    await api.action(`${base.value}/${currentID.value}/preannotate`);
    if (epoch !== generation) return;
    await loadItems();
    notice.value = "预标注已提交，结果仍需人工审核。";
  });
}
async function exportData() {
  if (busy.value) return;
  const epoch = generation,
    taskID = currentID.value;
  await run(async () => {
    const response = await fetch(
      `/api/v1/annotation-tasks/${encodeURIComponent(taskID)}/export?format=jsonl`,
      {
        headers: {
          Authorization: `Bearer ${auth.token}`,
          ...(auth.currentTenant
            ? { "X-Tenant-ID": String(auth.currentTenant.id) }
            : {}),
        },
      },
    );
    if (epoch !== generation) return;
    if (!response.ok) throw new Error(`导出失败 (${response.status})`);
    const blob = await response.blob();
    if (epoch !== generation) return;
    const url = URL.createObjectURL(blob),
      anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `annotations-${taskID}.jsonl`;
    anchor.click();
    URL.revokeObjectURL(url);
    notice.value = "已导出审核通过的标注。";
  });
}
async function openLink() {
  newTask();
  const epoch = generation;
  await run(async () => {
    let id =
        typeof route?.query.task_id === "string" ? route.query.task_id : "",
      kind =
        route?.query.kind === "tagging" ? "tagging_task" : "annotation_task";
    const executionID =
      typeof route?.query.execution_id === "string"
        ? route.query.execution_id
        : "";
    if (executionID) {
      const res = await api.get<GovernanceExecution>(
        `/governance-executions/${encodeURIComponent(executionID)}`,
      );
      if (epoch !== generation) return;
      execution.value = res.data;
      if (
        !res.data?.domain_id ||
        !["annotation_task", "tagging_task"].includes(res.data.domain_kind)
      )
        throw new Error("该执行尚无可查看的标注任务");
      id = res.data.domain_id;
      kind = res.data.domain_kind;
    }
    if (!id) return;
    mode.value = kind === "tagging_task" ? "tagging" : "annotation";
    const res = await api.get<Task>(`${base.value}/${encodeURIComponent(id)}`);
    if (epoch !== generation) return;
    if (!res.data) throw new Error("任务不存在或无权查看");
    detailTask.value = res.data;
    currentID.value = res.data.id;
    try {
      await loadItems();
    } catch (e) {
      if (epoch === generation) {
        detailTask.value = undefined;
        currentID.value = "";
        items.value = [];
        currentItem.value = undefined;
      }
      throw e;
    }
  });
}
watch(
  () => [route?.query.execution_id, route?.query.task_id, route?.query.kind],
  () => void openLink(),
);
watch(identity, () => {
  scope++;
  newTask();
  tasks.value = [];
  assets.value = [];
  next.value = "";
  void initialize();
});
async function initialize() {
  const epoch = generation;
  await run(async () => {
    await Promise.all([load(), assetOptions.reset()]);
  });
  if (
    epoch === generation &&
    (route?.query.execution_id || route?.query.task_id)
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
.wb-panel p,
.wb-panel li {
  overflow-wrap: anywhere;
}
.labeling-content {
  margin: 16px 0;
  padding: 18px;
  border-left: 3px solid var(--primary);
  background: var(--bg-elevated);
  font-size: 14px;
  white-space: pre-wrap;
  line-height: 1.9;
  max-height: 300px;
  overflow: auto;
}
</style>
