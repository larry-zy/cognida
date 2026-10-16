<template>
  <section class="wb-panel" aria-label="分区物化任务">
    <div class="wb-section-head">
      <h2>分区物化与回填</h2>
      <button type="button" :disabled="busy" @click="reload()">刷新任务</button>
    </div>
    <p class="wb-footnote">
      固定 v{{ feature.version }} 定义及完整输入快照，最多 10,000
      行，超限拒绝。窗口计算先读取完整快照中的历史，再分区提交。增量只写入已提交水位之后的事件；迟到数据请回填。回填不会推进水位。
    </p>
    <p v-if="feature.definition.kind === 'embedding'" class="wb-error">
      将调用固定模型与修订号生成真实向量。每批最多 32
      条，不能保证金额上限；中断后无法核实的调用需人工核对，不会自动重放。
    </p>
    <form class="wb-form" @submit.prevent="create">
      <label
        >物化模式<select v-model="mode" :disabled="busy">
          <option value="full">完整快照</option>
          <option value="incremental">增量</option>
          <option value="backfill">回填</option>
        </select></label
      >
      <label
        >物化来源快照<select v-model="source" :disabled="busy" required>
          <option :value="feature.source_ref_id">
            定义来源 · {{ feature.source_ref_id }}
          </option>
          <option
            v-for="a in assets.filter(
              (a) => `${a.asset_id}:${a.version}` !== feature.source_ref_id,
            )"
            :key="`${a.asset_id}:${a.version}`"
            :value="`${a.asset_id}:${a.version}`"
          >
            {{ a.name || a.asset_id }} · v{{ a.version }}
          </option>
        </select></label
      >
      <label
        >每分区行数<input
          v-model.number="partition"
          :disabled="busy"
          type="number"
          min="1"
          :max="feature.definition.kind === 'embedding' ? 32 : 1000"
          required
      /></label>
      <template v-if="mode === 'backfill'"
        ><label
          >回填起始（含）<input
            v-model="from"
            type="datetime-local"
            :disabled="busy"
            required /></label
        ><label
          >回填截止（不含）<input
            v-model="to"
            type="datetime-local"
            :disabled="busy"
            required /></label
      ></template>
      <p
        v-if="mode !== 'full' && !feature.definition.event_time_field"
        class="wb-error"
      >
        此版本没有事件时间字段，请创建有事件时间的新版本后使用增量或回填。
      </p>
      <button
        class="primary"
        :disabled="
          busy || (mode !== 'full' && !feature.definition.event_time_field)
        "
      >
        创建物化任务
      </button>
    </form>
    <p v-if="error" class="wb-error" role="alert">{{ error }}</p>
    <div class="wb-list">
      <button
        v-for="j in jobs"
        :key="j.id"
        :class="{ selected: current?.id === j.id }"
        @click="open(j.id)"
      >
        <strong>{{ status(j.status) }} · {{ modeLabel(j.request.mode) }}</strong
        ><small
          >{{ j.committed_rows }} 个已提交值 / {{ j.total_rows }} 条输入 ·
          {{ j.created_at }}</small
        >
      </button>
    </div>
    <button v-if="next" :disabled="busy" @click="reload(true)">
      加载更多物化任务
    </button>
    <p v-if="!jobs.length && !current" class="wb-empty">
      尚无此版本的持久物化任务。
    </p>
    <section v-if="current" aria-label="物化任务详情">
      <div class="wb-section-head">
        <h3>{{ status(current.status) }}</h3>
        <span class="wb-badge"
          >{{
            current.partitions.filter((p) => p.status === "committed").length
          }}
          / {{ current.partitions.length }} 分区已提交</span
        >
      </div>
      <p v-if="current.error" class="wb-error" role="alert">
        {{ current.error }}
      </p>
      <p class="wb-footnote">
        水位：{{ current.watermark_before || "尚无" }} →
        {{
          current.watermark_after || "尚未推进"
        }}。每个分区的可用时间由数据库提交时钟确定；历史读取不会看到之后回填的值。
      </p>
      <div class="wb-actions">
        <button v-if="current.status === 'failed' && !current.embedding && !current.operation_id" :disabled="busy" @click="control('retry')">重试未提交分区</button>
        <button
          v-if="['queued', 'running'].includes(current.status)"
          :disabled="busy"
          @click="control('cancel')"
        >
          请求取消</button
        ><button
          v-if="
            !['queued', 'running', 'cancel_requested', 'withdrawn'].includes(
              current.status,
            )
          "
          :disabled="busy"
          @click="control('withdraw')"
        >
          管理员撤回特征值
        </button>
      </div>
      <p v-if="current.status === 'cancel_requested'" role="status">
        取消已请求，等待后端确认；已提交分区仍保留。
      </p>
      <table class="wb-table">
        <thead>
          <tr>
            <th>分区</th>
            <th>状态 / 数量</th>
            <th>可用时间 / 产物</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in current.partitions" :key="p.index">
            <td>{{ p.index + 1 }}</td>
            <td>
              {{ p.status === "committed" ? "已提交" : "待处理" }} ·
              {{ p.count }}
            </td>
            <td>
              {{ p.available_at || "未提交" }}<br /><RouterLink
                v-if="p.output_ref_id"
                :to="{ path: '/analysis', query: { asset: p.output_ref_id } }"
                >查看分区产物</RouterLink
              >
            </td>
          </tr>
        </tbody>
      </table>
      <details>
        <summary>固定来源与摘要</summary>
        <p>
          {{ current.id }}<br />{{ current.request.source_ref_id }}<br />{{
            current.source_digest
          }}<br />{{ current.definition_digest }}
        </p>
      </details>
    </section>
  </section>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { useAuthStore } from "@/stores/auth";
import { governanceApi as api, type Feature } from "@/api/governance";
import type { FeatureJob } from "@/api/governance/featureOperations";
import type { AssetRef } from "@/api/lifecycle";
const props = defineProps<{
  feature: Feature;
  assets: AssetRef[];
  jobId?: string;
}>();
const auth = useAuthStore(),
  mode = ref("full"),
  source = ref(""),
  partition = ref(100),
  from = ref(""),
  to = ref(""),
  busy = ref(false),
  error = ref(""),
  jobs = ref<FeatureJob[]>([]),
  current = ref<FeatureJob>(),
  next = ref("");
const scope = computed(
  () =>
    `${auth.authEpoch}:${auth.user?.id}:${auth.currentTenant?.id}:${props.feature.id}:${props.feature.version}`,
);
let epoch = 0,
  timer: ReturnType<typeof setTimeout> | undefined,
  read = 0;
const status = (s: string) =>
  ({
    queued: "排队中",
    running: "执行中",
    completed: "已完成",
    failed: "失败",
    cancel_requested: "取消处理中",
    cancelled: "已取消",
    reconcile_required: "需核对模型调用",
    withdrawn: "已撤回",
  })[s] || s;
const modeLabel = (s: string) =>
  ({ full: "完整快照", incremental: "增量", backfill: "回填" })[s] || s;
function stop() {
  if (timer) clearTimeout(timer);
  timer = undefined;
}
function schedule() {
  stop();
  if (
    current.value &&
    ["queued", "running", "cancel_requested"].includes(current.value.status)
  )
    timer = setTimeout(() => void open(current.value!.id), 4000);
}
async function reload(append = false) {
  const own = epoch;
  try {
    const r = await api.list<FeatureJob>(
      "/feature-jobs",
      append ? next.value : "",
    );
    if (own !== epoch) return;
    const rows = (r.data?.items || []).filter(
      (j) =>
        j.request.feature_id === props.feature.id &&
        j.request.feature_version === props.feature.version,
    );
    jobs.value = append ? [...jobs.value, ...rows] : rows;
    next.value = r.data?.next_cursor || "";
  } catch (e) {
    if (own === epoch)
      error.value = e instanceof Error ? e.message : "任务加载失败";
  }
}
async function open(id: string) {
  const own = epoch,
    seq = ++read;
  stop();
  try {
    const r = await api.get<FeatureJob>(
      `/feature-jobs/${encodeURIComponent(id)}`,
    );
    if (own !== epoch || seq !== read) return;
    if (!r.data) throw new Error("任务不存在或无权查看");
    if (
      r.data.request.feature_id !== props.feature.id ||
      r.data.request.feature_version !== props.feature.version
    )
      throw new Error("任务不属于当前特征版本");
    current.value = r.data;
    error.value = "";
    schedule();
  } catch (e) {
    if (own === epoch && seq === read) {
      current.value = undefined;
      error.value = e instanceof Error ? e.message : "任务读取失败";
    }
  }
}
async function mutate(fn: () => Promise<void>) {
  if (busy.value) return;
  const own = epoch;
  busy.value = true;
  error.value = "";
  try {
    await fn();
  } catch (e) {
    if (own === epoch)
      error.value = e instanceof Error ? e.message : "操作失败";
  } finally {
    if (own === epoch) busy.value = false;
  }
}
async function create() {
  const own = epoch;
  await mutate(async () => {
    const data = {
      feature_id: props.feature.id,
      feature_version: props.feature.version,
      source_ref_id: source.value,
      mode: mode.value,
      partition_size: partition.value,
      ...(mode.value === "backfill"
        ? {
            event_from: new Date(from.value).toISOString(),
            event_to: new Date(to.value).toISOString(),
          }
        : {}),
    };
    const r = await api.create<FeatureJob>("/feature-jobs", data);
    if (own !== epoch) return;
    if (!r.data) throw new Error("未返回任务，请刷新核查");
    current.value = r.data;
    schedule();
    await reload();
  });
}
async function control(action: string) {
  const j = current.value;
  if (!j) return;
  const own = epoch;
  await mutate(async () => {
    const r = await api.action<FeatureJob>(`/feature-jobs/${j.id}/${action}`, {
      expected_version: j.version,
    });
    if (own !== epoch) return;
    current.value = r.data;
    schedule();
    await reload();
  });
}
watch(
  [scope, () => props.jobId],
  () => {
    epoch++;
    read++;
    stop();
    busy.value = false;
    error.value = "";
    jobs.value = [];
    current.value = undefined;
    next.value = "";
    source.value = props.feature.source_ref_id;
    partition.value = props.feature.definition.kind === "embedding" ? 32 : 100;
    void reload();
    if (props.jobId) void open(props.jobId);
  },
  { immediate: true },
);
onBeforeUnmount(() => {
  epoch++;
  stop();
});
</script>
<style scoped>
.wb-table {
  table-layout: fixed;
}
.wb-panel p,
.wb-table td {
  overflow-wrap: anywhere;
}
</style>
