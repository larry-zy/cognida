<template>
  <section class="wb-panel" aria-label="受治理策略版本">
    <h2>策略版本与回归</h2>
    <p class="wb-footnote">
      候选只属于当前用户。固定评测通过并明确审批后才切换；旧运行保留原版本，撤权或停用仍会阻止继续消费。耗时成本指墙钟时间，不是账单金额。
    </p>
    <label
      >策略消费者<select v-model="kind">
        <option v-for="(name, key) in consumers" :key="key" :value="key">
          {{ labels[key] || key }} · {{ name }}
        </option>
      </select></label
    ><button type="button" :disabled="busy" @click="load()">刷新策略</button>
    <p v-if="error" role="alert" class="wb-error">{{ error }}</p>
    <p v-if="notice" role="status">{{ notice }}</p>
    <template v-if="!head"
      ><button
        type="button"
        :disabled="busy || !consumers[kind]"
        @click="baseline"
      >
        登记服务端现有默认基线
      </button>
      <p>首次登记从实际消费者读取默认配置，不接受手填基线。</p></template
    ><template v-else
      ><p>
        当前活动版本：<code>{{
          head.active_id || "已停用，消费将被拒绝"
        }}</code>
        · 切换修订 {{ head.revision }}
      </p>
      <div class="strategy-grid">
        <aside>
          <button
            v-for="version in versions"
            :key="version.id"
            type="button"
            class="version"
            :disabled="busy"
            @click="select(version)"
          >
            <strong
              >v{{ version.version }} ·
              {{
                version.id === head.active_id
                  ? "活动"
                  : version.disabled_at
                    ? "已停用"
                    : "候选／历史"
              }}</strong
            ><small>{{ version.id }}</small></button
          ><button
            v-if="versionCursor"
            type="button"
            :disabled="busy"
            @click="load(true)"
          >
            更多版本
          </button>
        </aside>
        <div>
          <form @submit.prevent="create">
            <h3>记录候选差异</h3>
            <label
              >来源证据资产引用（每行一个）<textarea
                v-model="evidence"
                placeholder="asset-id:version"
                required
              /></label
            ><label
              >受限策略内容 JSON<textarea v-model="body" rows="6" required />
            </label>
            <p>
              基于活动版本
              {{ head.active_id }}。内容不能改变工具授权、知识库范围、审批或
              Sandbox。
            </p>
            <button :disabled="busy || !head.active_id">
              保存候选，不激活
            </button>
          </form>
          <template v-if="selected"
            ><h3>v{{ selected.version }} 版本核对</h3>
            <code>{{ selected.digest }}</code>
            <pre>{{ JSON.stringify(selected.body, null, 2) }}</pre>
            <p>固定门槛：{{ JSON.stringify(selected.gates) }}</p>
            <p>
              在“评测任务”分别选择基线版本和该候选；使用相同的固定数据集、模型及评分器。清洗规则会真实预处理问题后再生成答案；检索策略只用于
              RAG。
            </p>
            <form v-if="kind === 'orchestration'" @submit.prevent="trial">
              <label
                >待启动的固定 DAG 任务 ID<input
                  v-model="trialTask"
                  required /></label
              ><button :disabled="busy">固定到隔离编排试验，不启动</button>
              <p>
                先创建两个计划、根输入和预算相同的任务，各固定基线或候选后从任务页显式启动。需要审批或外部写入的计划不能作为试验。
              </p>
            </form>
            <form @submit.prevent="regress">
              <label
                >已完成基线评测 ID<input
                  v-model="baselineTask"
                  required /></label
              ><label
                >已完成候选评测 ID<input
                  v-model="candidateTask"
                  required /></label
              ><button :disabled="busy || !selected.baseline_id">
                核对真实回归结果
              </button>
            </form>
            <p v-if="receiptLoading" role="status">
              正在读取当前版本的回归与批准回执…
            </p>
            <p v-if="receiptError" role="alert" class="wb-error">
              {{ receiptError }}
              <button
                type="button"
                :disabled="receiptLoading"
                @click="select(selected)"
              >
                重新读取当前版本回执
              </button>
            </p>
            <label
              >回归记录<select v-model="regressionID">
                <option value="">选择已核对的回归</option>
                <option
                  v-for="event in regressions"
                  :key="event.id"
                  :value="event.id"
                >
                  {{ event.payload.passed ? "通过" : "未通过" }} ·
                  {{ event.id }}
                </option>
              </select></label
            ><label>审核／切换原因<textarea v-model="reason" required /></label
            ><button
              type="button"
              :disabled="busy || receiptLoading || !regressionID || !reason"
              @click="approve(true)"
            >
              批准当前确切摘要</button
            ><button
              type="button"
              :disabled="busy || receiptLoading || !regressionID || !reason"
              @click="approve(false)"
            >
              拒绝当前确切摘要
            </button>
            <p>
              每个不可变版本只接受一次最终审核；修订或重审请创建新候选版本。
            </p>
            <label
              >批准回执<select v-model="approvalID">
                <option value="">选择批准回执</option>
                <option
                  v-for="event in approvals"
                  :key="event.id"
                  :value="event.id"
                >
                  {{ event.id }}
                </option>
              </select></label
            ><button
              type="button"
              :disabled="
                busy ||
                receiptLoading ||
                !approvalID ||
                !reason ||
                !!selected.disabled_at
              "
              @click="activate(false)"
            >
              确认激活此版本</button
            ><button
              type="button"
              :disabled="busy || !reason || !!selected.disabled_at"
              @click="activate(true)"
            >
              确认回退到此历史版本</button
            ><button
              type="button"
              :disabled="busy || !reason || !!selected.disabled_at"
              @click="disable"
            >
              停用此版本
            </button>
            <form @submit.prevent="observe">
              <label
                >监测用真实评测 ID<input
                  v-model="observationTask"
                  required /></label
              ><button :disabled="busy">登记按版本质量／错误／耗时</button>
            </form></template
          >
        </div>
      </div>
      <h3>真实回执与运行快照</h3>
      <article v-for="event in events" :key="event.id">
        <strong>{{ event.kind }} · {{ event.id }}</strong>
        <pre>{{ JSON.stringify(event.payload, null, 2) }}</pre>
      </article>
      <button
        v-if="eventCursor"
        type="button"
        :disabled="busy"
        @click="moreEvents"
      >
        更多回执
      </button></template
    >
  </section>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useAuthStore } from "@/stores/auth";
import {
  strategyApi,
  type StrategyVersion,
  type StrategyHead,
  type StrategyEvent,
} from "@/api/governance/strategies";
const auth = useAuthStore(),
  kind = ref("cleaning"),
  consumers = ref<Record<string, string>>({}),
  head = ref<StrategyHead>(),
  versions = ref<StrategyVersion[]>([]),
  events = ref<StrategyEvent[]>([]),
  selected = ref<StrategyVersion>(),
  versionCursor = ref(""),
  eventCursor = ref(""),
  busy = ref(false),
  error = ref(""),
  notice = ref(""),
  evidence = ref(""),
  body = ref(""),
  reason = ref(""),
  baselineTask = ref(""),
  candidateTask = ref(""),
  observationTask = ref(""),
  trialTask = ref(""),
  regressionID = ref(""),
  approvalID = ref("");
const labels: Record<string, string> = {
  cleaning: "清洗规则",
  retrieval: "检索参数",
  skill: "Skill 指导",
  playbook: "Playbook 指导",
  experience: "经验指导",
  orchestration: "编排并发",
};
const versionEvents = ref<StrategyEvent[]>([]),
  receiptLoading = ref(false),
  receiptError = ref("");
const slot = computed(() => kind.value + ".default"),
  regressions = computed(() =>
    versionEvents.value.filter(
      (v) => v.kind === "regression" && v.version_id === selected.value?.id,
    ),
  ),
  approvals = computed(() =>
    versionEvents.value.filter(
      (v) =>
        v.kind === "approval" &&
        v.version_id === selected.value?.id &&
        v.payload.approved,
    ),
  );
let epoch = 0,
  selectionEpoch = 0;
function reset() {
  epoch++;
  selectionEpoch++;
  versionEvents.value = [];
  receiptLoading.value = false;
  receiptError.value = "";
  head.value = undefined;
  selected.value = undefined;
  versions.value = [];
  events.value = [];
  versionCursor.value = "";
  eventCursor.value = "";
  busy.value = false;
  error.value = "";
  notice.value = "";
  body.value = "";
  evidence.value = "";
  reason.value = "";
  baselineTask.value = "";
  candidateTask.value = "";
  observationTask.value = "";
  trialTask.value = "";
  regressionID.value = "";
  approvalID.value = "";
}
watch(
  () =>
    `${auth.authEpoch}:${auth.user?.id}:${auth.currentTenant?.id}:${kind.value}`,
  () => {
    reset();
    void load();
  },
);
onBeforeUnmount(() => {
  epoch++;
});
onMounted(async () => {
  const own = epoch;
  try {
    const r = await strategyApi.consumers();
    if (own !== epoch) return;
    consumers.value = r.data || {};
    await load();
  } catch (e) {
    if (own === epoch)
      error.value = e instanceof Error ? e.message : "消费者加载失败";
  }
});
async function load(append = false, chosen = selected.value?.id) {
  if (busy.value) return;
  const own = epoch;
  busy.value = true;
  error.value = "";
  try {
    const [hr, vr, er] = await Promise.all([
      strategyApi.head(slot.value),
      strategyApi.versions(slot.value, append ? versionCursor.value : ""),
      strategyApi.events(slot.value),
    ]);
    if (own !== epoch) return;
    head.value = hr.data;
    versions.value = append
      ? [
          ...new Map(
            [...versions.value, ...(vr.data?.items || [])].map((version) => [
              version.id,
              version,
            ]),
          ).values(),
        ]
      : vr.data?.items || [];
    events.value = er.data?.items || [];
    versionCursor.value = vr.data?.next_cursor || "";
    eventCursor.value = er.data?.next_cursor || "";
    const target = chosen || head.value?.active_id;
    let version = versions.value.find((v) => v.id === target);
    if (!version && target) {
      const result = await strategyApi.version(slot.value, target);
      if (own !== epoch) return;
      version = result.data;
      if (version) versions.value.push(version);
    }
    if (version) await select(version);
  } catch (e) {
    if (own === epoch) {
      error.value =
        e instanceof Error ? e.message : "策略读取失败；首次使用可登记基线";
    }
  } finally {
    if (own === epoch) busy.value = false;
  }
}
async function select(v: StrategyVersion) {
  selected.value = v;
  body.value = JSON.stringify(v.body, null, 2);
  regressionID.value = "";
  approvalID.value = "";
  versionEvents.value = [];
  receiptError.value = "";
  receiptLoading.value = true;
  const own = epoch,
    selection = ++selectionEpoch,
    currentSlot = slot.value;
  const current = () =>
    own === epoch &&
    selection === selectionEpoch &&
    selected.value?.id === v.id;
  async function readKind(eventKind: string) {
    const found: StrategyEvent[] = [],
      seen = new Set<string>();
    let cursor = "";
    for (let page = 0; page < 100; page++) {
      const result = await strategyApi.events(currentSlot, cursor, eventKind);
      if (!current()) return [];
      found.push(
        ...(result.data?.items || []).filter(
          (event) => event.version_id === v.id,
        ),
      );
      const next = result.data?.next_cursor || "";
      if (!next) return found;
      if (seen.has(next) || next === cursor)
        throw new Error("回执分页游标重复，请重新读取当前版本回执");
      seen.add(next);
      cursor = next;
    }
    throw new Error("当前版本回执超过单次读取范围，请缩小历史范围后重试");
  }
  try {
    const pages = await Promise.all([
      readKind("regression"),
      readKind("approval"),
    ]);
    if (current())
      versionEvents.value = [
        ...new Map(pages.flat().map((event) => [event.id, event])).values(),
      ];
  } catch (e) {
    if (current()) {
      versionEvents.value = [];
      receiptError.value =
        e instanceof Error ? e.message : "当前版本回执读取失败";
    }
  } finally {
    if (current()) receiptLoading.value = false;
  }
}
async function moreEvents() {
  if (busy.value) return;
  const own = epoch;
  busy.value = true;
  try {
    const r = await strategyApi.events(slot.value, eventCursor.value);
    if (own !== epoch) return;
    events.value.push(...(r.data?.items || []));
    eventCursor.value = r.data?.next_cursor || "";
  } catch (e) {
    if (own === epoch)
      error.value = e instanceof Error ? e.message : "回执读取失败";
  } finally {
    if (own === epoch) busy.value = false;
  }
}
async function mutate(path: string, input: unknown) {
  if (busy.value) return;
  const own = epoch;
  busy.value = true;
  error.value = "";
  try {
    const r = await strategyApi.write(slot.value, path, input);
    if (own !== epoch) return;
    notice.value = `已保存真实回执 ${r.data?.id || ""}`;
    const chosen =
      (path === "/versions" ? r.data?.id : r.data?.version_id) ||
      selected.value?.id;
    busy.value = false;
    await load(false, chosen);
  } catch (e) {
    if (own === epoch)
      error.value = e instanceof Error ? e.message : "策略操作失败";
  } finally {
    if (own === epoch) busy.value = false;
  }
}
function baseline() {
  void mutate("/baseline", { kind: kind.value });
}
function create() {
  try {
    const parsed = JSON.parse(body.value);
    void mutate("/versions", {
      kind: kind.value,
      baseline_id: head.value?.active_id,
      body: parsed,
      evidence_refs: evidence.value
        .split(/\n/)
        .map((v) => v.trim())
        .filter(Boolean),
    });
  } catch {
    error.value = "策略内容不是有效 JSON";
  }
}
function trial() {
  if (selected.value)
    void mutate(`/versions/${selected.value.id}/trials`, {
      task_id: trialTask.value,
    });
}
function regress() {
  if (selected.value)
    void mutate(`/versions/${selected.value.id}/regressions`, {
      baseline_task_id: baselineTask.value,
      candidate_task_id: candidateTask.value,
    });
}
function approve(approved: boolean) {
  if (selected.value)
    void mutate(`/versions/${selected.value.id}/approvals`, {
      digest: selected.value.digest,
      regression_id: regressionID.value,
      approved,
      comment: reason.value,
    });
}
function activate(rollback: boolean) {
  if (selected.value)
    void mutate(`/versions/${selected.value.id}/activate`, {
      digest: selected.value.digest,
      approval_id: approvalID.value,
      expected_revision: head.value?.revision,
      reason: reason.value,
      rollback,
    });
}
function disable() {
  if (selected.value)
    void mutate(`/versions/${selected.value.id}/disable`, {
      reason: reason.value,
    });
}
function observe() {
  if (selected.value)
    void mutate(`/versions/${selected.value.id}/observations`, {
      task_id: observationTask.value,
    });
}
</script>
<style scoped>
.strategy-grid {
  display: grid;
  grid-template-columns: minmax(180px, 1fr) 3fr;
  gap: 20px;
}
.version {
  display: block;
  width: 100%;
  text-align: left;
  padding: 12px;
}
label {
  display: block;
  margin: 12px 0;
}
textarea,
input,
select {
  display: block;
  width: 100%;
  margin-top: 5px;
}
pre {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  max-height: 280px;
  overflow: auto;
}
code,
small {
  overflow-wrap: anywhere;
}
small {
  display: block;
}
button {
  margin: 6px 8px 6px 0;
}
@media (max-width: 800px) {
  .strategy-grid {
    grid-template-columns: 1fr;
  }
}
</style>
