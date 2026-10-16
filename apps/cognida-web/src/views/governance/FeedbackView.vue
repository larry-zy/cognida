<template>
  <div class="workbench">
    <header class="wb-head">
      <div>
        <span class="wb-kicker">EVIDENCE · REPAIR · VALIDATE</span>
        <h1>反馈与返工</h1>
        <p>
          保留真实问题与证据，在固定样本上比较修复效果。验证和审核通过后再切换正式版本。
        </p>
      </div>
      <div class="wb-actions">
        <button :disabled="busy" @click="refresh">刷新真实状态</button
        ><button :disabled="busy" @click="newCase">记录反馈</button>
      </div>
    </header>
    <p v-if="error" class="wb-error" role="alert">{{ error }}</p>
    <p v-if="notice" class="wb-success" role="status">{{ notice }}</p>
    <div v-if="versionError" class="wb-error" role="alert">
      {{ versionError }}
      <button :disabled="busy" @click="action(() => loadVersions(false))">
        重新读取评测版本
      </button>
    </div>
    <div class="wb-grid">
      <aside class="wb-panel">
        <h2>反馈记录</h2>
        <div class="wb-list">
          <button
            v-for="row in cases"
            :key="row.id"
            :class="{ selected: current?.id === row.id }"
            :disabled="busy"
            @click="choose(row.id)"
          >
            <strong>{{ row.title || "来源已失效的反馈" }}</strong
            ><small
              >{{ label(row.status) }} · 已预留
              {{ row.reserved_calls }} 次调用</small
            >
          </button>
        </div>
        <p v-if="!cases.length" class="wb-empty">
          暂无反馈。先选择真实知识来源和已审核的评测版本。
        </p>
        <button
          v-if="cursor"
          :disabled="busy"
          @click="action(() => loadList(true))"
        >
          加载更多反馈
        </button>
      </aside>
      <div class="wb-stack">
        <form
          v-if="!current"
          class="wb-panel"
          aria-label="记录质量反馈"
          @submit.prevent="create"
        >
          <h2>1. 固定问题、来源与验证门槛</h2>
          <p class="wb-footnote">
            基线需为从知识库导入的文本资产。可选列表不代表永久权限；每一步都会复查来源和样本。
          </p>
          <fieldset :disabled="busy">
            <div class="wb-form">
              <label
                >反馈标题<input v-model="draft.title" required maxlength="256"
              /></label>
              <label
                >问题环节<select v-model="draft.stage">
                  <option value="collection">采集</option>
                  <option value="cleaning">清洗</option>
                  <option value="labeling">标签</option>
                  <option value="retrieval">检索</option>
                  <option value="research">研究</option>
                </select></label
              >
              <label
                >基线知识资产<select v-model="draft.baseline" required>
                  <option value="">选择已导入的知识资产</option>
                  <option
                    v-for="a in knowledgeAssets"
                    :key="refKey(a)"
                    :value="refKey(a)"
                  >
                    {{ a.name || a.asset_id }} · v{{ a.version }}
                  </option>
                </select></label
              >
              <label
                >问题输出资产<select v-model="draft.output" required>
                  <option value="">选择真实输出</option>
                  <option
                    v-for="a in assets"
                    :key="refKey(a)"
                    :value="refKey(a)"
                  >
                    {{ a.name || a.asset_id }} · {{ a.kind }}
                  </option>
                </select></label
              >
              <label
                >目标知识库<select v-model="draft.kb" required>
                  <option value="">选择原知识库</option>
                  <option v-for="kb in kbs" :key="kb.id" :value="kb.id">
                    {{ kb.name }}
                  </option>
                </select></label
              >
              <label
                >固定评测版本<select v-model="draft.dataset" required>
                  <option value="">选择已审核的 RAG 版本</option>
                  <option
                    v-for="v in versions.filter((v) => v.profile === 'rag')"
                    :key="v.id"
                    :value="v.id"
                  >
                    {{ v.name }} · v{{ v.version }}
                  </option>
                </select></label
              >
              <label class="wb-full"
                >原始问题<textarea
                  v-model="draft.question"
                  required
                  maxlength="16000"
                /></label
              ><label class="wb-full"
                >发现的错误与修复目标<textarea
                  v-model="draft.problem"
                  required
                  maxlength="16000"
                />
              </label>
              <label>评测任务 ID（可选）<input v-model="draft.task" /></label
              ><label
                >对应结果 ID（可选）<input v-model="draft.result"
              /></label>
            </div>
            <details>
              <summary>固定验证条件与返工预算</summary>
              <label
                >检索必需标签（可选，逗号分隔；基线与候选使用同一条件）<input
                  v-model="draft.appliedLabels"
              /></label>
              <div class="wb-form">
                <label
                  >最少配对样本<input
                    v-model.number="draft.minPairs"
                    type="number"
                    min="2"
                    max="100"
                    required /></label
                ><label
                  >最低平均提升<input
                    v-model.number="draft.improvement"
                    type="number"
                    min="0.000001"
                    max="1"
                    step="any"
                    required /></label
                ><label
                  >允许退化边界<input
                    v-model.number="draft.regression"
                    type="number"
                    min="0"
                    max="1"
                    step="any"
                    required /></label
                ><label
                  >最多返工轮次<input
                    v-model.number="draft.rounds"
                    type="number"
                    min="1"
                    max="3"
                    required /></label
                ><label
                  >总调用预留上限<input
                    v-model.number="draft.calls"
                    type="number"
                    min="1"
                    max="2000"
                    required
                /></label>
              </div>
              <p class="wb-footnote">
                固定 ROUGE-L、top-k=5、ROUGE/BLEU
                评分与已登记模型。配对区间只能说明这组样本的变化，不代表通用语义正确。预算限制调用次数，不保证第三方模型账单金额。
              </p>
            </details>
            <div class="wb-actions">
              <button type="submit" class="primary">登记反馈与固定门槛</button
              ><button
                v-if="assetOptions.state.hasMore"
                type="button"
                @click="action(assetOptions.loadMore)"
              >
                加载更多资产</button
              ><button
                v-if="versionCursor"
                type="button"
                @click="action(() => loadVersions(true))"
              >
                加载更多评测版本</button
              ><button
                v-if="kbMore"
                type="button"
                @click="action(() => loadKB(true))"
              >
                加载更多知识库
              </button>
            </div>
          </fieldset>
        </form>
        <template v-else>
          <section class="wb-panel" aria-label="反馈详情">
            <div class="wb-section-head">
              <h2>{{ current.title }}</h2>
              <span class="wb-badge">{{ label(current.status) }}</span>
            </div>
            <p>{{ current.evidence.problem }}</p>
            <dl class="facts">
              <div>
                <dt>固定基线</dt>
                <dd>
                  <RouterLink :to="assetLink(current.baseline_ref)"
                    >查看原始知识资产</RouterLink
                  >
                </dd>
              </div>
              <div>
                <dt>验证门槛</dt>
                <dd>
                  {{ current.gate.metric }} · 至少
                  {{ current.gate.min_pairs }} 对 · 提升 ≥
                  {{ current.gate.min_improvement }}
                </dd>
              </div>
              <div>
                <dt>剩余调用预留</dt>
                <dd>
                  {{ current.budget.max_calls - current.reserved_calls }} /
                  {{ current.budget.max_calls }}
                </dd>
              </div>
              <div>
                <dt>活动版本</dt>
                <dd>
                  {{
                    current.active_version_id
                      ? "已批准修复版本"
                      : current.status === "withdrawn"
                        ? "已撤回，停止消费"
                        : "原始基线"
                  }}
                </dd>
              </div>
            </dl>
            <p class="wb-footnote">
              计划、完整性检查和未审核候选均不代表发布成功。调用预留不是实际模型费用。
            </p>
            <div class="wb-actions">
              <RouterLink
                :to="{ path: '/feedback', query: { case_id: current.id } }"
                >此反馈链接</RouterLink
              ><button :disabled="busy" @click="withdraw">撤回正式消费</button
              ><button :disabled="busy" @click="rollback('baseline')">
                回退到仍有效的基线
              </button>
            </div>
          </section>
          <section class="wb-panel" aria-label="人工修正与返工">
            <form @submit.prevent="saveCorrection">
              <h2>2. 修正草稿与真实返工</h2>
              <p>
                人工修正保留基线血缘，随后由现有清洗专家执行。保存草稿不会发布知识。
              </p>
              <fieldset :disabled="busy || closed">
                <label
                  >建议的完整修正文本<textarea
                    v-model="correction"
                    maxlength="16000"
                    required
                  /></label
                ><button type="submit">保存人工修正草稿</button>
              </fieldset>
            </form>
            <div class="wb-form">
              <label
                >返工专家<select
                  v-model="repairMode"
                  :disabled="busy || closed"
                >
                  <option value="lifecycle.data-cleaning">清洗文本</option>
                  <option value="governance.tagging">标签规则修复</option>
                </select></label
              >
              <label v-if="repairMode === 'governance.tagging'"
                >标签方案（每行：标签 | 关键词,关键词）<textarea
                  v-model="labelRules"
                  :disabled="busy || closed"
                />
              </label>
              <label v-else
                >返工输入<select v-model="repairRef" :disabled="busy || closed">
                  <option :value="refKey(current.baseline_ref)">
                    原始基线
                  </option>
                  <option
                    v-for="(c, index) in current.corrections || []"
                    :key="refKey(c.ref)"
                    :value="refKey(c.ref)"
                  >
                    人工修正 {{ index + 1 }}
                  </option>
                </select></label
              >
            </div>
            <button
              class="primary"
              :disabled="busy || closed || !cleanCapability"
              @click="rework"
            >
              创建并启动返工
            </button>
            <p class="wb-footnote">
              清洗使用固定 trim/dedup
              及真实质量门槛；标签规则仅生成预标注，需到标注页人工审核。最多
              {{ current.budget.max_rounds }} 轮，已使用
              {{ current.rounds?.length || 0 }} 轮。任务状态来自控制器。
            </p>
            <div v-for="r in current.rounds || []" :key="r.id" class="repair">
              <RouterLink
                v-if="r.task_id"
                :to="{ path: '/tasks', query: { task_id: r.task_id } }"
                >查看返工任务</RouterLink
              ><button
                :disabled="busy || !r.task_id"
                @click="loadOutputs(r.id, r.task_id!)"
              >
                读取真实返工产物
              </button>
            </div>
            <form v-if="outputs.length" @submit.prevent="createCandidate">
              <label
                >用于隔离验证的产物<select v-model="outputRef" required>
                  <option
                    v-for="o in outputs"
                    :key="refKey(o)"
                    :value="refKey(o)"
                  >
                    {{ o.name || o.asset_id }} · {{ o.kind }}
                  </option>
                </select></label
              >
              <template v-if="labelTask"
                ><RouterLink
                  :to="{
                    path: '/labeling',
                    query: { task_id: labelTask, kind: 'tagging' },
                  }"
                  >打开标签任务并审核</RouterLink
                ><label
                  >已审核标签结果<select v-model="labelChoice" required>
                    <option value="">请先审核后重新读取产物</option>
                    <option v-for="r in labelChoices" :key="r.id" :value="r.id">
                      {{ r.category || r.tags?.join("、") || "已审核标签" }}
                    </option>
                  </select></label
                ></template
              >
              <button
                :disabled="busy || closed || (!!labelTask && !labelChoice)"
                type="submit"
              >
                创建隔离候选
              </button>
            </form>
          </section>
          <section class="wb-panel" aria-label="候选验证与发布">
            <h2>3. 固定复测与审批发布</h2>
            <p v-if="!publications.length" class="wb-empty">
              完成真实返工后选择产物，创建隔离候选。
            </p>
            <article v-for="v in publications" :key="v.id" class="candidate">
              <div class="wb-section-head">
                <h3>修复候选</h3>
                <span class="wb-badge">{{ label(v.status) }}</span>
              </div>
              <p v-if="v.error" class="wb-error">{{ v.error }}</p>
              <RouterLink :to="assetLink(v.source_ref)"
                >查看候选来源</RouterLink
              >
              <p v-if="v.projection">
                索引：{{ v.projection.index }} · 已核对
                {{ v.projection.chunk_ids.length }} 个真实片段
              </p>
              <div class="wb-actions">
                <RouterLink
                  v-if="v.baseline_evaluation_id"
                  :to="{
                    path: '/evaluation',
                    query: { task_id: v.baseline_evaluation_id },
                  }"
                  >查看基线评测</RouterLink
                ><RouterLink
                  v-if="v.candidate_evaluation_id"
                  :to="{
                    path: '/evaluation',
                    query: { task_id: v.candidate_evaluation_id },
                  }"
                  >查看候选评测</RouterLink
                >
              </div>
              <div v-if="v.comparison" class="comparison">
                <p>
                  有效配对 {{ v.comparison.pairs }} · 缺失
                  {{ v.comparison.missing_pairs }} · 均值
                  {{ v.comparison.baseline_mean }} →
                  {{ v.comparison.candidate_mean }}
                </p>
                <p>
                  提升 {{ v.comparison.improvement }} · 区间
                  {{ v.comparison.lower_95 ?? "—" }} 至
                  {{ v.comparison.upper_95 ?? "—" }}
                </p>
                <p>{{ v.comparison.recommendation }}</p>
                <p
                  v-for="text in v.comparison.limitations"
                  :key="text"
                  class="wb-footnote"
                >
                  {{ text }}
                </p>
              </div>
              <label v-if="v.status === 'validated'"
                >审核理由<input v-model="reviewComment" maxlength="2000"
              /></label>
              <div class="wb-actions">
                <button
                  v-if="
                    ['candidate', 'projecting', 'reconcile_required'].includes(
                      v.status,
                    )
                  "
                  :disabled="busy || closed"
                  @click="control(v, 'project')"
                >
                  {{
                    v.status === "candidate" ? "建立隔离索引" : "核对投影回执"
                  }}</button
                ><button
                  v-if="['ready', 'validating'].includes(v.status)"
                  :disabled="busy || closed"
                  @click="control(v, 'validate')"
                >
                  {{
                    v.status === "ready"
                      ? "启动固定基线复测"
                      : "恢复评测任务绑定"
                  }}</button
                ><button
                  v-if="['validating', 'rejected'].includes(v.status)"
                  :disabled="busy"
                  @click="control(v, 'compare')"
                >
                  读取真实比较结果</button
                ><button
                  v-if="v.status === 'validated'"
                  :disabled="busy || !reviewComment.trim()"
                  @click="control(v, 'approve')"
                >
                  审核通过</button
                ><button
                  v-if="
                    v.status === 'approved' &&
                    current.active_version_id !== v.id
                  "
                  :disabled="busy"
                  @click="control(v, 'activate')"
                >
                  激活已批准版本</button
                ><span v-if="current.active_version_id === v.id" role="status"
                  >当前正式活动版本</span
                >
              </div>
            </article>
          </section>
        </template>
      </div>
    </div>
  </div>
</template>
<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch } from "vue";
import {
  useRoute,
  useRouter,
  onBeforeRouteLeave,
  onBeforeRouteUpdate,
} from "vue-router";
import { useAuthStore } from "@/stores/auth";
import {
  feedbackApi as api,
  type FeedbackCase,
  type FeedbackPublication,
} from "@/api/feedback";
import { lifecycleApi, type AssetRef, type RefID } from "@/api/lifecycle";
import {
  taskControlApi,
  type Capability,
  type PlanSpec,
} from "@/api/taskcontrol";
import {
  datasetVersionsApi,
  type DatasetVersion,
} from "@/api/evaluation/versions";
import { knowledgeApi } from "@/api/knowledge";
import { governanceApi, type TagResult } from "@/api/governance";
import type { KnowledgeBase } from "@/types";
import { useCursorOptions } from "@/composables/useCursorOptions";
import "@/components/workbench/workbench.css";
const auth = useAuthStore(),
  route = useRoute(),
  router = useRouter();
const blank = () => ({
  title: "",
  stage: "retrieval",
  baseline: "",
  output: "",
  kb: "",
  dataset: "",
  question: "",
  problem: "",
  task: "",
  result: "",
  minPairs: 2,
  improvement: 0.01,
  regression: 0,
  rounds: 2,
  calls: 300,
  appliedLabels: "",
});
const draft = ref(blank()),
  savedDraft = ref(JSON.stringify(draft.value)),
  cases = ref<FeedbackCase[]>([]),
  current = ref<FeedbackCase>(),
  publications = ref<FeedbackPublication[]>([]),
  assets = ref<AssetRef[]>([]),
  versions = ref<DatasetVersion[]>([]),
  kbs = ref<KnowledgeBase[]>([]),
  capabilities = ref<Capability[]>([]),
  cursor = ref(""),
  versionCursor = ref(""),
  versionError = ref(""),
  kbMore = ref(false),
  busy = ref(false),
  error = ref(""),
  notice = ref(""),
  correction = ref(""),
  repairRef = ref(""),
  outputs = ref<AssetRef[]>([]),
  outputRef = ref(""),
  roundID = ref(""),
  reviewComment = ref("");
let epoch = 0,
  detailEpoch = 0,
  kbPage = 0,
  poll: ReturnType<typeof setTimeout> | undefined;
const assetOptions = useCursorOptions(assets, async (cursor) => {
  const g = epoch;
  const result = await lifecycleApi.assets("", cursor);
  if (g !== epoch) throw new Error("身份已变化");
  return result;
});
const knowledgeAssets = computed(() =>
  assets.value.filter((a) => a.provenance?.source_type === "knowledge"),
);
const repairMode = ref("lifecycle.data-cleaning"),
  labelRules = ref(""),
  labelTask = ref(""),
  labelChoices = ref<TagResult[]>([]),
  labelChoice = ref("");
const cleanCapability = computed(() =>
  capabilities.value.find((c) => c.id === repairMode.value && c.execution),
);
const closed = computed(
  () =>
    !!current.value &&
    ["withdrawn", "invalidated"].includes(current.value.status),
);
const dirty = computed(
  () =>
    correction.value.trim() !== "" ||
    (!current.value && JSON.stringify(draft.value) !== savedDraft.value),
);
const refKey = (v: RefID) => `${v.asset_id}:${v.version}`;
const parse = (s: string): RefID => {
  const i = s.lastIndexOf(":");
  const version = Number(s.slice(i + 1));
  if (i < 1 || !Number.isSafeInteger(version) || version < 1)
    throw new Error("请选择有效资产版本");
  return { asset_id: s.slice(0, i), version };
};
const assetLink = (r: RefID) => ({
  path: "/analysis",
  query: { asset: refKey(r) },
});
const label = (s: string) =>
  ({
    open: "待返工",
    active: "已激活",
    withdrawn: "已撤回",
    invalidated: "来源失效",
    candidate: "待建立隔离索引",
    projecting: "投影处理中",
    reconcile_required: "待核对回执",
    ready: "待固定复测",
    validating: "评测中",
    validated: "验证通过，待审核",
    approved: "已批准",
    rejected: "门槛未通过",
    cleanup_failed: "读取已阻断，清理待重试",
    cleaned: "派生索引已清理",
  })[s] || s;
async function action(fn: (generation: number) => Promise<unknown>) {
  if (busy.value) return;
  const g = epoch;
  busy.value = true;
  error.value = "";
  try {
    await fn(g);
  } catch (e) {
    if (g === epoch)
      error.value =
        e instanceof Error ? e.message : "操作失败，请刷新真实状态核对";
  } finally {
    if (g === epoch) busy.value = false;
  }
}
async function loadList(more = false) {
  const g = epoch;
  const r = await api.list(more ? cursor.value : "");
  if (g !== epoch) return;
  cases.value = more
    ? [...cases.value, ...(r.data?.items || [])]
    : r.data?.items || [];
  cursor.value = r.data?.next_cursor || "";
}
async function loadVersions(more = false) {
  const g = epoch;
  try {
    const r = await datasetVersionsApi.list(more ? versionCursor.value : "");
    if (g !== epoch) return;
    versions.value = more
      ? [...versions.value, ...(r.data?.items || [])]
      : r.data?.items || [];
    versionCursor.value = r.data?.next_cursor || "";
    versionError.value = "";
  } catch (cause) {
    if (g !== epoch) return;
    versions.value = [];
    versionCursor.value = "";
    versionError.value = "评测版本读取失败，请核对当前权限后重新读取。";
    throw cause;
  }
}
async function loadKB(more = false) {
  const g = epoch,
    page = more ? kbPage + 1 : 1;
  const r = await knowledgeApi.getPage(page, 50);
  if (g !== epoch) return;
  kbs.value = more
    ? [...kbs.value, ...(r.data?.items || [])]
    : r.data?.items || [];
  kbPage = page;
  kbMore.value = kbs.value.length < (r.data?.total || 0);
}
async function open(id: string) {
  const g = epoch,
    d = ++detailEpoch;
  current.value = undefined;
  publications.value = [];
  outputs.value = [];
  const r = await api.get(id);
  if (g !== epoch || d !== detailEpoch) return;
  const p = await api.publications(id);
  if (g !== epoch || d !== detailEpoch) return;
  current.value = r.data;
  publications.value = p.data?.items || [];
  repairRef.value = current.value
    ? refKey(
        current.value.corrections?.at(-1)?.ref || current.value.baseline_ref,
      )
    : "";
  schedule();
}
async function choose(id: string) {
  if (
    dirty.value &&
    !window.confirm("切换将放弃未保存的反馈或修正文字。继续？")
  )
    return;
  correction.value = "";
  await action(async (g) => {
    await open(id);
    if (g !== epoch) return;
    await router.replace({ path: "/feedback", query: { case_id: id } });
  });
}
async function refresh() {
  await action(async (g) => {
    await loadList();
    if (g === epoch && current.value) await refreshCurrent();
  });
}
async function refreshCurrent() {
  const id = current.value?.id;
  if (!id) return;
  const g = epoch,
    d = detailEpoch;
  try {
    const [r, p] = await Promise.all([api.get(id), api.publications(id)]);
    if (g !== epoch || d !== detailEpoch || current.value?.id !== id) return;
    current.value = r.data;
    publications.value = p.data?.items || [];
    schedule();
  } catch (e) {
    if (g === epoch && d === detailEpoch) {
      current.value = undefined;
      publications.value = [];
      outputs.value = [];
    }
    throw e;
  }
}
function schedule() {
  const g = epoch;
  clearTimeout(poll);
  if (
    publications.value.some((v) =>
      ["projecting", "validating"].includes(v.status),
    )
  ) {
    poll = setTimeout(async () => {
      if (!busy.value) {
        try {
          await refreshCurrent();
        } catch (e) {
          if (g === epoch)
            error.value = e instanceof Error ? e.message : "状态读取失败";
        }
      } else schedule();
    }, 4000);
  }
}
async function create() {
  await action(async (g) => {
    const version = versions.value.find((v) => v.id === draft.value.dataset);
    if (!version) throw new Error("请选择固定评测版本");
    const d = { ...draft.value };
    const r = await api.create({
      title: d.title,
      knowledge_base_id: d.kb,
      baseline_ref: parse(d.baseline),
      evidence: {
        output_ref: parse(d.output),
        source_refs: [parse(d.baseline)],
        question: d.question,
        problem: d.problem,
        stage: d.stage,
        ...(d.task
          ? { evaluation_task_id: d.task, evaluation_result_id: d.result }
          : {}),
      },
      dataset_version_id: version.id,
      evaluation_config: {
        dataset_id: version.dataset_id,
        type: "rag",
        kb_id: d.kb,
        config: {
          top_k: 5,
          graders: ["rouge", "bleu"],
          ...(d.appliedLabels.trim()
            ? {
                applied_labels: d.appliedLabels
                  .split(/[,，]/)
                  .map((v) => v.trim())
                  .filter(Boolean),
              }
            : {}),
        },
      },
      gate: {
        metric: "rouge_l",
        higher_is_better: true,
        min_pairs: d.minPairs,
        max_regression: d.regression,
        min_improvement: d.improvement,
      },
      budget: {
        max_rounds: d.rounds,
        max_calls: d.calls,
        max_cost_micros: 0,
        timeout_seconds: 86400,
      },
    });
    if (g !== epoch || !r.data) return;
    savedDraft.value = JSON.stringify(draft.value);
    await open(r.data.id);
    if (g !== epoch) return;
    await router.replace({ path: "/feedback", query: { case_id: r.data.id } });
    if (g === epoch) await loadList();
  });
}
async function saveCorrection() {
  const c = current.value;
  if (!c) return;
  await action(async (g) => {
    const r = await api.correct(c, correction.value);
    if (g !== epoch) return;
    current.value = r.data;
    correction.value = "";
    repairRef.value = r.data?.corrections?.length
      ? refKey(r.data.corrections.at(-1)!.ref)
      : repairRef.value;
    notice.value = "人工修正草稿已保存，尚未运行或发布。";
  });
}
async function rework() {
  const c = current.value,
    cap = cleanCapability.value;
  if (!c || !cap?.execution) return;
  await action(async (g) => {
    const input =
      repairMode.value === "governance.tagging"
        ? c.baseline_ref
        : parse(repairRef.value);
    const schema = labelRules.value
      .split("\n")
      .map((v) => v.trim())
      .filter(Boolean)
      .map((row, i) => {
        const [name, words] = row.split("|");
        return {
          id: `repair-label-${i + 1}`,
          name: name!.trim(),
          keywords: (words || "")
            .split(/[,，]/)
            .map((v) => v.trim())
            .filter(Boolean),
        };
      });
    if (
      repairMode.value === "governance.tagging" &&
      (schema.length < 1 ||
        schema.length > 30 ||
        schema.some((v) => !v.name || !v.keywords.length) ||
        new Set(schema.map((v) => v.name)).size !== schema.length)
    )
      throw new Error("请输入 1–30 个不重复的标签与关键词");
    const read = await lifecycleApi.asset(input.asset_id, input.version);
    if (g !== epoch || current.value?.id !== c.id) return;
    const asset = read.data?.ref;
    if (!asset) throw new Error("返工输入不可用");
    const calls = cap.execution!.base_calls + cap.execution!.calls_per_input;
    const plan: PlanSpec = {
      schema_version: 1,
      budget: {
        max_nodes: 1,
        max_concurrency: 1,
        max_attempts: 1,
        max_calls: calls,
        max_cost_micros: 0,
        timeout_seconds: 60,
      },
      nodes: [
        {
          id: "repair",
          ...(repairMode.value === "governance.tagging"
            ? { parameters: { governance: { label_schema: schema } } }
            : {}),
          capability: cap.id,
          capability_version: cap.version,
          depends_on: [],
          inputs: [{ asset: input, kind: asset.kind }],
          output_kinds: cap.output_kinds,
          tools: cap.tools,
          permissions: cap.permissions,
          max_attempts: 1,
          max_calls_per_attempt: calls,
          max_cost_micros_per_attempt: 0,
          timeout_seconds: 60,
        },
      ],
    };
    await api.rework(c, plan);
    if (g !== epoch) return;
    await refreshCurrent();
    notice.value = "已创建真实返工任务，可打开任务中心查看执行。";
  });
}
async function loadOutputs(round: string, task: string) {
  await action(async (g) => {
    const result = await taskControlApi.summary(task);
    if (g !== epoch) return;
    if (!result.data?.terminal) throw new Error("返工尚未终结，请稍后刷新");
    const loaded: AssetRef[] = [];
    labelTask.value = "";
    labelChoices.value = [];
    labelChoice.value = "";
    for (const ref of result.data.result_refs || []) {
      const r = await lifecycleApi.asset(ref.asset_id, ref.version);
      if (g !== epoch) return;
      const manifest = r.data?.content as
        { task_kind?: string; task_id?: string } | undefined;
      if (
        r.data?.ref &&
        manifest?.task_kind === "tagging_task" &&
        manifest.task_id
      ) {
        labelTask.value = manifest.task_id;
        const tagged = await governanceApi.list<TagResult>(
          `/tagging-tasks/${manifest.task_id}/results`,
        );
        if (g !== epoch) return;
        labelChoices.value = (tagged.data?.items || []).filter(
          (v) => v.review_status === "approved" && v.reviewed_by,
        );
        labelChoice.value = labelChoices.value[0]?.id || "";
        loaded.push(r.data.ref);
        continue;
      }
      if (
        r.data?.ref &&
        ["raw_asset", "cleaned_asset", "enriched_asset"].includes(
          r.data.ref.kind,
        )
      )
        loaded.push(r.data.ref);
    }
    outputs.value = loaded;
    roundID.value = round;
    outputRef.value = loaded[0] ? refKey(loaded[0]) : "";
    if (!loaded.length)
      throw new Error("返工没有可发布的文本产物，请查看真实任务错误。");
  });
}
async function createCandidate() {
  const c = current.value;
  if (!c) return;
  await action(async (g) => {
    if (
      labelTask.value &&
      !labelChoices.value.some((v) => v.id === labelChoice.value)
    )
      throw new Error("请先审核该真实标签任务，再重新读取返工产物");
    await api.candidate(c, {
      source_ref: parse(outputRef.value),
      round_id: roundID.value,
      ...(labelTask.value
        ? {
            label_result_id: labelChoice.value,
            label_review_version: labelChoices.value.find(
              (v) => v.id === labelChoice.value,
            )!.version,
          }
        : {}),
    });
    if (g !== epoch) return;
    await refreshCurrent();
  });
}
async function control(v: FeedbackPublication, actionName: string) {
  const c = current.value;
  if (!c) return;
  await action(async (g) => {
    await api.control(v, actionName, c, reviewComment.value);
    if (g !== epoch) return;
    await refreshCurrent();
    notice.value = "操作已完成，以下为服务端最新状态。";
  });
}
async function withdraw() {
  const c = current.value;
  if (!c || !window.confirm("撤回后立即停止该反馈版本的正式消费。继续？"))
    return;
  await action(async (g) => {
    await api.withdraw(c);
    if (g !== epoch) return;
    await refreshCurrent();
  });
}
async function rollback(id: string) {
  const c = current.value;
  if (!c || !window.confirm("仅在目标历史版本及来源仍有效时切换。继续？"))
    return;
  await action(async (g) => {
    await api.rollback(c, id);
    if (g !== epoch) return;
    await refreshCurrent();
  });
}
function clear() {
  epoch++;
  detailEpoch++;
  clearTimeout(poll);
  current.value = undefined;
  publications.value = [];
  cases.value = [];
  assets.value = [];
  versions.value = [];
  kbs.value = [];
  capabilities.value = [];
  outputs.value = [];
  labelTask.value = "";
  labelChoices.value = [];
  labelChoice.value = "";
  labelRules.value = "";
  repairMode.value = "lifecycle.data-cleaning";
  correction.value = "";
  reviewComment.value = "";
  error.value = "";
  notice.value = "";
  busy.value = false;
  draft.value = blank();
  savedDraft.value = JSON.stringify(draft.value);
  cursor.value = "";
  versionCursor.value = "";
  versionError.value = "";
}
async function loadDraftReferences(g: number) {
  const task =
    typeof route.query.task_id === "string" ? route.query.task_id : "";
  const result =
    typeof route.query.result_id === "string" ? route.query.result_id : "";
  if (task && result) {
    let cursor = "";
    const seen = new Set<string>();
    let found = false;
    for (let page = 0; page < 100; page++) {
      const response = await api.evidenceResults(task, cursor);
      if (g !== epoch) return;
      const item = response.data?.results.find(
        (row) => String(row.id) === result,
      );
      if (item) {
        if (typeof item.question !== "string" || item.question.length > 16000)
          throw new Error("该问题超出当前反馈长度范围");
        draft.value.task = task;
        draft.value.result = result;
        draft.value.question = item.question;
        found = true;
        break;
      }
      const next = response.data?.next_cursor || "";
      if (!next || seen.has(next)) break;
      seen.add(next);
      cursor = next;
    }
    if (!found)
      throw new Error("评测样本不存在或当前不可读取，请返回评测结果重新选择。");
  }
  const output =
    typeof route.query.output_ref === "string" ? route.query.output_ref : "";
  if (output) {
    const id = parse(output);
    const response = await lifecycleApi.asset(id.asset_id, id.version);
    if (g !== epoch) return;
    const asset = response.data?.ref;
    if (!asset || refKey(asset) !== output)
      throw new Error("输出引用当前不可读取");
    if (!assets.value.some((item) => refKey(item) === output))
      assets.value.push(asset);
    draft.value.output = output;
    if (asset.provenance?.source_type === "knowledge")
      draft.value.baseline = output;
  }
}
async function initialize() {
  await action(async (g) => {
    const results = await Promise.allSettled([
      loadList(),
      loadVersions(),
      loadKB(),
      assetOptions.reset(),
      taskControlApi.capabilities().then((r) => {
        if (g === epoch) capabilities.value = r.data?.capabilities || [];
      }),
    ]);
    if (g !== epoch) return;
    if (
      results.some(
        (result, index) => index !== 1 && result.status === "rejected",
      )
    )
      error.value = "部分可选列表读取失败，请刷新真实状态后重试。";
    const id =
      typeof route.query.case_id === "string" ? route.query.case_id : "";
    if (id) await open(id);
    else {
      await loadDraftReferences(g);
      if (g !== epoch) return;
      savedDraft.value = JSON.stringify(draft.value);
    }
  });
}
async function newCase() {
  if (dirty.value && !window.confirm("放弃当前未保存文字？")) return;
  correction.value = "";
  savedDraft.value = JSON.stringify(draft.value);
  if (route.query.case_id) {
    await router.replace({ path: "/feedback", query: {} });
  } else {
    clear();
    void initialize();
  }
}
const guard = () =>
  !busy.value &&
  (!dirty.value || window.confirm("离开将放弃未保存的反馈或修正文字。继续？"));
onBeforeRouteLeave(guard);
onBeforeRouteUpdate((to, from) =>
  to.query.case_id === current.value?.id &&
  to.query.case_id !== from.query.case_id
    ? true
    : guard(),
);
watch(
  () => route.query.case_id,
  (id) => {
    if (id === current.value?.id) return;
    clear();
    void initialize();
  },
);
watch(
  () => `${auth.authEpoch}:${auth.user?.id}:${auth.currentTenant?.id}`,
  () => {
    clear();
    void initialize();
  },
);
onMounted(initialize);
onBeforeUnmount(() => {
  epoch++;
  detailEpoch++;
  clearTimeout(poll);
});
</script>
<style scoped>
.facts {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 1rem;
}
.facts dt {
  color: var(--text-muted);
}
.facts dd {
  margin: 0.3rem 0;
}
.candidate {
  padding: 1rem 0;
  border-top: 1px solid var(--border-color);
}
.repair {
  display: flex;
  flex-wrap: wrap;
  gap: 1rem;
  align-items: center;
  padding: 0.65rem 0;
}
.comparison {
  margin: 1rem 0;
  padding: 1rem;
  background: var(--bg-secondary);
  border-radius: 8px;
}
fieldset {
  border: 0;
  padding: 0;
  margin: 0;
}
label {
  display: grid;
  gap: 0.5rem;
}
textarea {
  min-height: 6rem;
}
details {
  margin: 1rem 0;
}
summary {
  cursor: pointer;
}
.wb-full {
  grid-column: 1/-1;
}
@media (max-width: 640px) {
  .facts {
    grid-template-columns: 1fr;
  }
}
</style>
