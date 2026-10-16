<template>
  <section class="wb-panel execution-panel" aria-labelledby="summary-title">
    <div class="execution-heading">
      <div>
        <span class="wb-kicker">04 / EXECUTION</span>
        <h2 id="summary-title">当前状态汇总</h2>
      </div>
      <span class="wb-badge">{{ statusLabel(task.status) }}</span>
    </div>
    <p
      v-if="task.status === 'cancel_requested'"
      class="execution-notice"
      role="status"
    >
      已申请取消，正在等待领域任务确认。已有结果和实际终态会继续更新。
    </p>
    <p v-if="task.reason" class="execution-reason">{{ task.reason }}</p>
    <p v-if="error" class="wb-error" role="alert">{{ error }}</p>
    <p v-if="actionError" class="wb-error" role="alert">{{ actionError }}</p>
    <p v-if="uncertain" class="execution-notice" role="status">
      操作结果尚未确认，正在核对任务状态。不会自动重发启动或取消请求。
    </p>
    <div class="execution-controls">
      <button
        v-if="canStart"
        class="primary"
        :disabled="busy || loading"
        @click="$emit('start')"
      >
        启动任务
      </button>
      <button
        v-if="canCancel"
        :disabled="busy || loading"
        @click="$emit('cancel')"
      >
        申请取消
      </button>
      <button :disabled="busy || loading" @click="$emit('refresh')">
        {{ loading ? "正在核对状态…" : "重新核对执行状态" }}
      </button>
      <span v-if="action" class="wb-muted" role="status">{{
        action === "start" ? "正在提交启动请求…" : "正在提交取消请求…"
      }}</span>
      <span v-else-if="updatedAt" class="wb-muted small"
        >最近核对 {{ updatedAt }}</span
      >
    </div>
    <div
      v-if="!task.execution_plan_version && !summary?.terminal"
      class="execution-admission"
    >
      <h3>执行准入</h3>
      <p v-if="!task.current_plan_version" class="wb-muted">
        先保存计划，再核对当前版本能否执行。
      </p>
      <p v-if="candidatePending" class="wb-muted">
        本页还有未保存候选。保存或明确放弃候选后，再核对已保存计划的执行准入。
      </p>
      <p v-if="readinessError" class="wb-error" role="alert">
        {{ readinessError }}
      </p>
      <template v-if="readiness">
        <p
          v-if="readiness.allowed && readiness.execution_enabled"
          class="wb-success"
        >
          已保存计划 v{{
            readiness.plan_version
          }}
          已通过当前执行准入。请单独点击启动。
        </p>
        <p v-else class="wb-muted">当前计划尚不能启动。</p>
        <ul v-if="readiness.blockers?.length" class="execution-blockers">
          <li v-for="(blocker, index) in readiness.blockers" :key="index">
            <strong v-if="blocker.node_id"
              >{{ nodeLabel(blocker.node_id) }}：</strong
            >{{ blocker.message }}
            <details>
              <summary>阻塞标识</summary>
              <span>{{ blocker.code }}</span>
            </details>
          </li>
        </ul>
      </template>
      <p class="wb-muted small">
        按保存的领域参数执行。特征工程使用固定定义版本与来源；知识标签和训练／评测样本产出候选，等待人工审核。发布与任意脚本仍需独立接入。
      </p>
    </div>
    <dl
      v-if="task.started_at || task.deadline_at || task.finished_at"
      class="execution-times"
    >
      <div v-if="task.started_at">
        <dt>开始时间</dt>
        <dd>{{ timeLabel(task.started_at) }}</dd>
      </div>
      <div v-if="task.deadline_at">
        <dt>执行期限</dt>
        <dd>{{ timeLabel(task.deadline_at) }}</dd>
      </div>
      <div v-if="task.finished_at">
        <dt>结束时间</dt>
        <dd>{{ timeLabel(task.finished_at) }}</dd>
      </div>
    </dl>
    <template v-if="summary">
      <p>
        已保存计划
        {{
          summary.current_plan_version
            ? `v${summary.current_plan_version}`
            : "暂无"
        }}
        · {{ summary.planned_nodes }} 个计划节点 ·
        {{ summary.children?.length || 0 }} 条子任务记录
      </p>
      <p v-if="task.execution_plan_version" class="wb-muted">
        当前执行计划 v{{
          task.execution_plan_version
        }}；已预留节点保留各自原计划版本与回执。
      </p>
      <p v-if="summary.nodes?.length" class="node-progress">
        已成功
        {{
          summary.nodes.filter(
            (node) =>
              node.status === "succeeded" || node.status === "completed",
          ).length
        }}
        / {{ summary.nodes.length }} 个节点<span v-if="!summary.terminal">
          · 状态自动核对中</span
        >
      </p>
      <ol
        v-if="summary.nodes?.length"
        class="runtime-nodes"
        aria-label="节点执行状态"
      >
        <li v-for="node in orderedNodes" :key="node.node_id">
          <div class="execution-heading">
            <h3>{{ capabilityLabel(node.capability) }}</h3>
            <span class="wb-badge">{{ statusLabel(node.status) }}</span>
          </div>
          <p class="wb-muted small">
            {{ node.node_id }} ·
            {{ node.attempt ? `第 ${node.attempt} 次尝试` : "尚未尝试" }}
          </p>
          <p v-if="node.error" class="wb-error">{{ node.error }}</p>
          <router-link v-if="domainLink(node)" :to="domainLink(node)!"
            >查看领域任务 ↗</router-link
          >
          <ul v-if="node.output_refs?.length" aria-label="节点产物">
            <li
              v-for="output in node.output_refs"
              :key="`${output.asset_id}:${output.version}`"
            >
              <router-link
                :to="{
                  path: '/analysis',
                  query: { asset: `${output.asset_id}:${output.version}` },
                }"
                >查看{{ assetLabel(output.kind) }} · v{{
                  output.version
                }}
                ↗</router-link
              >
            </li>
          </ul>
        </li>
      </ol>
      <p v-if="!summary.children?.length" class="wb-muted">
        尚无子任务执行记录，计划节点不代表已执行进度。
      </p>
      <details v-else class="dispatch-details">
        <summary>投递与领域回执（{{ summary.children.length }}）</summary>
        <ul class="child-records">
          <li v-for="child in summary.children" :key="child.id">
            <strong>{{ nodeLabel(child.node_id) }}</strong>
            <p>
              投递状态：{{ statusLabel(child.dispatch_status) }} · 领域状态：{{
                statusLabel(child.domain_status)
              }}
            </p>
            <p class="wb-muted small">
              计划 v{{ child.plan_version }} · 第 {{ child.attempt }} 次尝试 ·
              {{
                child.receipt_present
                  ? "已有提交回执，仍需核实领域结果"
                  : "尚无提交回执"
              }}
            </p>
            <router-link v-if="domainLink(child)" :to="domainLink(child)!"
              >查看领域任务 ↗</router-link
            >
            <p class="wb-muted small">
              {{ child.domain }} · {{ child.domain_id || "尚未绑定" }}
            </p>
          </li>
        </ul>
      </details>
      <dl
        v-if="summary.usage"
        class="execution-usage"
        aria-label="执行预留额度"
      >
        <div>
          <dt>已预留尝试</dt>
          <dd>{{ summary.usage.reserved_attempts }}</dd>
        </div>
        <div>
          <dt>已预留调用</dt>
          <dd>{{ summary.usage.reserved_calls }}</dd>
        </div>
        <div>
          <dt>已预留成本额度</dt>
          <dd>{{ summary.usage.reserved_cost_micros.toLocaleString() }}</dd>
        </div>
      </dl>
      <p v-if="summary.usage" class="wb-muted small">
        以上为包含重试的预留额度，不代表实际消耗或已完成进度。
      </p>
      <p v-if="!summary.result_refs?.length" class="wb-muted">
        暂无已确认的结果引用。
      </p>
      <ul v-else>
        <li
          v-for="result in summary.result_refs"
          :key="`${result.asset_id}:${result.version}`"
        >
          <router-link
            :to="{
              path: '/analysis',
              query: { asset: `${result.asset_id}:${result.version}` },
            }"
            >查看结果 · {{ result.asset_id }} · v{{
              result.version
            }}</router-link
          >
          ·
          <router-link
            :to="{
              path: '/feedback',
              query: { output_ref: `${result.asset_id}:${result.version}` },
            }"
            >记录此产物反馈与返工</router-link
          >
        </li>
      </ul>
      <ul v-if="summary.warnings?.length" class="summary-warnings">
        <li v-for="(warning, index) in summary.warnings" :key="index">
          {{ warning }}
        </li>
      </ul>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type {
  ExecutionReadiness,
  GovernedTask,
  SavedPlan,
  TaskSummary,
} from "@/api/taskcontrol";
import { assetLabel, capabilityLabel, statusLabel } from "./labels";
const props = defineProps<{
  task: GovernedTask;
  summary?: TaskSummary;
  savedPlan?: SavedPlan;
  readiness?: ExecutionReadiness;
  error: string;
  readinessError: string;
  actionError: string;
  loading: boolean;
  busy: boolean;
  action: string;
  uncertain: boolean;
  candidatePending: boolean;
  canStart: boolean;
  canCancel: boolean;
  updatedAt: string;
}>();
defineEmits<{ start: []; cancel: []; refresh: [] }>();
function domainLink(item: { domain_kind?: string; domain_id?: string }) {
  if (!item.domain_id) return undefined;
  if (item.domain_kind === "sandbox_run")
    return { path: "/sandbox", query: { run_id: item.domain_id } };
  if (item.domain_kind === "warehouse_run")
    return { path: "/warehouse", query: { run_id: item.domain_id } };
  if (item.domain_kind === "research")
    return { path: "/research", query: { run_id: item.domain_id } };
  if (item.domain_kind === "evaluation")
    return { path: `/evaluation/${encodeURIComponent(item.domain_id)}` };
  if (item.domain_kind === "collection")
    return { path: "/collection-tasks", query: { run_id: item.domain_id } };
  if (item.domain_kind === "governance_execution")
    return { path: "/labeling", query: { execution_id: item.domain_id } };
  if (item.domain_kind === "feature_execution")
    return { path: "/features", query: { execution_id: item.domain_id } };
  if (item.domain_kind === "invocation")
    return { path: "/experts", query: { invocation_id: item.domain_id } };
  if (item.domain_kind === "run")
    return { path: "/pipelines", query: { run_id: item.domain_id } };
  return undefined;
}
const nodeLabel = (id: string) => {
  const node = props.savedPlan?.snapshot.nodes.find((node) => node.id === id);
  return node ? `${capabilityLabel(node.capability)}（${id}）` : id;
};
const timeLabel = (value: string) => {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString("zh-CN");
};
const orderedNodes = computed(() => {
  const remaining = [...(props.savedPlan?.snapshot.nodes || [])],
    order: string[] = [];
  while (remaining.length) {
    const index = remaining.findIndex((node) =>
      (node.depends_on || []).every((id) => order.includes(id)),
    );
    if (index < 0) break;
    order.push(remaining.splice(index, 1)[0]!.id);
  }
  const indices = new Map(order.map((id, index) => [id, index]));
  return [...(props.summary?.nodes || [])].sort(
    (a, b) =>
      (indices.get(a.node_id) ?? order.length) -
      (indices.get(b.node_id) ?? order.length),
  );
});
</script>

<style scoped>
.execution-heading,
.execution-controls {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}
.execution-heading h2,
.execution-heading h3 {
  margin: 0;
}
.execution-heading .wb-kicker {
  display: block;
  margin-bottom: 8px;
}
.execution-controls {
  justify-content: flex-start;
  margin: 18px 0;
}
.execution-panel p,
.execution-panel li,
.execution-panel a {
  font-size: 12px;
  line-height: 1.8;
  overflow-wrap: anywhere;
}
.execution-admission,
.execution-notice {
  padding: 16px;
  border-left: 3px solid var(--primary);
  background: var(--bg-tertiary);
}
.execution-admission h3 {
  margin: 0 0 12px;
}
.execution-blockers {
  padding-left: 18px;
}
.execution-blockers li + li {
  margin-top: 12px;
}
.runtime-nodes,
.child-records {
  list-style: none;
  padding: 0;
}
.runtime-nodes > li {
  padding: 18px 0;
  border-top: 1px solid var(--border-default);
}
.runtime-nodes h3 {
  font-size: 14px;
}
.execution-times,
.execution-usage {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 14px;
  margin: 20px 0;
  font-size: 12px;
}
.execution-times dt,
.execution-usage dt {
  color: var(--text-muted);
}
.execution-times dd,
.execution-usage dd {
  margin: 6px 0 0;
  line-height: 1.7;
  overflow-wrap: anywhere;
}
.execution-reason {
  white-space: pre-wrap;
}
.dispatch-details {
  margin: 20px 0;
}
summary {
  cursor: pointer;
  font-size: 12px;
  color: var(--text-secondary);
}
.node-progress {
  color: var(--text-primary);
}
.small,
.summary-warnings {
  font-size: 12px;
}
.summary-warnings {
  padding-left: 20px;
}
@media (max-width: 680px) {
  .execution-times,
  .execution-usage {
    grid-template-columns: 1fr;
  }
  .execution-controls button {
    flex: 1 1 auto;
  }
}
</style>
