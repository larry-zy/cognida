<template>
  <div class="workbench">
    <header class="wb-head">
      <div>
        <span class="wb-kicker">HUMAN REVIEW</span>
        <h1>{{ wt("标注与智能打标") }}</h1>
        <p>保留 AI 建议的来源与置信度，用人工审核建立可追溯的数据集。</p>
      </div>
      <button class="primary" @click="newTask">{{ wt("＋ 新建任务") }}</button>
    </header>
    <div v-if="error" class="wb-error" role="alert">{{ error }}</div>
    <div v-if="notice" class="wb-success" role="status">{{ notice }}</div>
    <div class="wb-tabs">
      <button
        :class="{ selected: mode === 'annotation' }"
        @click="changeMode('annotation')"
      >
        {{ wt("人工标注 / AI 预标注") }}</button
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
                  :key="asset.asset_id"
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
              </select></label
            >
          </div>
          <RecordEditor
            v-if="mode === 'annotation'"
            v-model="labels"
            title="标签"
            :fields="[{ key: 'name', label: '标签名称' }]"
          />
          <div v-else class="wb-form" style="margin-top: 18px">
            <label
              v-for="field in tagOptions"
              :key="field.key"
              class="wb-checkbox"
              ><input v-model="tagConfig[field.key]" type="checkbox" />{{
                field.label
              }}</label
            ><label class="wb-full"
              >标签体系<input
                v-model="tagSchema"
                placeholder="例如：价格,售后,交付"
            /></label>
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
              ><button :disabled="busy" @click="preannotate">
                {{ wt("AI 预标注") }}</button
              ><button :disabled="busy" @click="exportData">
                {{ wt("导出 JSONL") }}
              </button></template
            >
          </div>
          <p v-if="'error' in (activeTask || {})" class="wb-error">
            {{ (activeTask as TagTask)?.error }}
          </p>
          <p class="wb-footnote">
            AI
            生成结果待人工审核。修改后保存会保留来源记录；分类与实体使用任务定义的标签。
          </p>
          <div class="wb-list" style="margin-top: 16px">
            <button
              v-for="item in items"
              :key="item.id"
              :class="{ selected: currentItem?.id === item.id }"
              @click="selectItem(item)"
            >
              <strong
                >{{ item.content.slice(0, 90)
                }}{{ item.content.length > 90 ? "…" : "" }}</strong
              ><small
                >{{ item.review_status }} · 来源 {{ item.provenance.source }} ·
                置信 {{ confidence(item.provenance.confidence) }}</small
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
          <p v-if="currentItem.reviewed_by" class="wb-muted">
            审核人：{{ currentItem.reviewed_by }} ·
            {{ currentItem.review_status }}<br />{{
              currentItem.review_comment
            }}
          </p>
          <form v-if="mode === 'annotation'" @submit.prevent="saveItem">
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
              v-if="(activeTask as AnnotationTask)?.type !== 'classification'"
              v-model="annotation.entities"
              title="实体"
              :fields="entityFields"
            /><RecordEditor
              v-if="(activeTask as AnnotationTask)?.type === 'relation'"
              v-model="annotation.relations"
              title="关系"
              :fields="relationFields"
            />
            <p class="wb-footnote">
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
import { collectPages } from "@/utils/cursorPages";
import { wt } from "@/i18n/workbench";

import { computed, onMounted, reactive, ref } from "vue";
import {
  governanceApi as api,
  type AnnotationTask,
  type AnnotationItem,
  type TagTask,
  type TagResult,
} from "@/api/governance";
import { lifecycleApi, type AssetRef } from "@/api/lifecycle";
import RecordEditor, {
  type EditorField,
} from "@/components/workbench/RecordEditor.vue";
import "@/components/workbench/workbench.css";
import { useAuthStore } from "@/stores/auth";
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
  tagSchema = ref(""),
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
    tasks.value.find((t) => t.id === currentID.value),
  ),
  base = computed(() =>
    mode.value === "annotation" ? "/annotation-tasks" : "/tagging-tasks",
  );
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
async function run(fn: () => Promise<void>) {
  busy.value = true;
  error.value = "";
  notice.value = "";
  try {
    await fn();
  } catch (e) {
    error.value = e instanceof Error ? e.message : "操作失败";
  } finally {
    busy.value = false;
  }
}
async function load(append = false) {
  const res = await api.list<Task>(base.value, append ? next.value : "");
  tasks.value = append
    ? [...tasks.value, ...(res.data?.items || [])]
    : res.data?.items || [];
  next.value = res.data?.next_cursor || "";
}
function newTask() {
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
  await run(async () => {
    const input =
      mode.value === "annotation"
        ? {
            name: name.value,
            type: annotationType.value,
            source_ref_id: sourceRef.value,
            labels: labels.value,
          }
        : {
            name: name.value,
            source_ref_id: sourceRef.value,
            config: { ...tagConfig, tag_schema: split(tagSchema.value) },
          };
    const res = await api.create<Task>(base.value, input);
    await load();
    if (res.data) {
      currentID.value = res.data.id;
      await loadItems();
    }
  });
}
async function selectTask(task: Task) {
  currentID.value = task.id;
  currentItem.value = undefined;
  await run(() => loadItems());
}
async function loadItems(append = false) {
  const suffix = mode.value === "annotation" ? "items" : "results";
  const res = await api.list<Item>(
    `${base.value}/${currentID.value}/${suffix}`,
    append ? itemNext.value : "",
  );
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
  return { ...annotation, entities: entities(annotation.entities) };
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
  if (res.data) selectItem(res.data);
  await loadItems();
}
async function saveItem() {
  await run(async () => {
    await persist();
    notice.value = "人工修正已保存";
  });
}
async function review(decision: string) {
  await run(async () => {
    if (!currentItem.value) return;
    if (mode.value === "annotation") {
      const itemID = currentItem.value.id;
      const comment = reviewComment.value;
      if (JSON.stringify(annotationValue()) !== annotationSnapshot)
        await persist();
      await api.action(
        `${base.value}/${currentID.value}/items/${itemID}/review`,
        { decision, comment, expected_version: currentItem.value?.version },
      );
      await loadItems();
      const updated = items.value.find((i) => i.id === itemID);
      if (updated) selectItem(updated);
    } else {
      const itemID = currentItem.value.id,
        comment = reviewComment.value;
      if (JSON.stringify(tagValue()) !== tagSnapshot) await persist();
      const response = await api.action<TagResult>(
        `/tagging-results/${itemID}/review`,
        { decision, comment, expected_version: currentItem.value?.version },
      );
      if (response.data) selectItem(response.data);
      await loadItems();
    }
    notice.value = decision === "approved" ? "审核通过" : "已退回修正";
  });
}
async function preannotate() {
  await run(async () => {
    await api.action(`${base.value}/${currentID.value}/preannotate`);
    await loadItems();
    notice.value = "预标注已提交，结果仍需人工审核。";
  });
}
async function exportData() {
  await run(async () => {
    const response = await fetch(
      `/api/v1/annotation-tasks/${currentID.value}/export?format=jsonl`,
      { headers: { Authorization: `Bearer ${useAuthStore().token}` } },
    );
    if (!response.ok) throw new Error(`导出失败 (${response.status})`);
    const url = URL.createObjectURL(await response.blob());
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `annotations-${currentID.value}.jsonl`;
    anchor.click();
    URL.revokeObjectURL(url);
  });
}
onMounted(() =>
  run(async () => {
    await load();
    assets.value = await collectPages((cursor) =>
      lifecycleApi.assets("", cursor),
    );
  }),
);
</script>
<style scoped>
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
