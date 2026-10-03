<template>
  <div class="workbench">
    <header class="wb-head">
      <div>
        <span class="wb-kicker">PROACTIVE INSIGHTS</span>
        <h1>报告与指标看护</h1>
        <p>为已发布资产安排定时分析，查看每次执行的结果与失败原因。</p>
      </div>
      <button class="primary" @click="newSubscription">＋ 创建订阅</button>
    </header>
    <div v-if="error" role="alert" class="wb-error">{{ error }}</div>
    <div v-if="notice" role="status" class="wb-success">{{ notice }}</div>
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
        :class="{ selected: tab === 'subscriptions' }"
        @click="tab = 'subscriptions'"
      >
        订阅管理</button
      ><button
        :class="{ selected: tab === 'notifications' }"
        @click="
          tab = 'notifications';
          run(() => loadNotifications());
        "
      >
        站内通知
      </button>
    </div>
    <div v-if="tab === 'subscriptions'" class="wb-grid">
      <aside class="wb-panel">
        <div class="wb-list">
          <button
            v-for="item in subscriptions"
            :key="item.id"
            :class="{ selected: form.id === item.id }"
            @click="select(item)"
          >
            <strong>{{ item.name }}</strong
            ><small
              >{{ item.kind === "report" ? "定时报告" : "指标看护" }} ·
              {{ item.enabled ? "已启用" : "已停用" }}</small
            >
          </button>
        </div>
        <p v-if="!subscriptions.length" class="wb-empty">
          暂无订阅。选择资产，安排第一次分析。
        </p>
        <button v-if="next" @click="run(() => load(true))">
          {{ wt("加载更多") }}
        </button>
      </aside>
      <div class="wb-stack">
        <form class="wb-panel" @submit.prevent="save">
          <p v-if="form.schedule_error" class="wb-error" role="status">
            订阅已自动停用：{{
              form.schedule_error === "invalid_schedule_configuration"
                ? "执行计划或时区无效，请修正后重新启用。"
                : form.schedule_error === "owner_access_unavailable"
                  ? "订阅所有者已无访问权限，请恢复权限后重新启用。"
                  : form.schedule_error
            }}
          </p>
          <h2>{{ form.id ? "编辑订阅" : "新建订阅" }}</h2>
          <div class="wb-form">
            <label>订阅名称<input v-model="form.name" required /></label
            ><label
              >类型<select v-model="form.kind">
                <option value="report">定时报告</option>
                <option value="metric_watch">指标看护</option>
              </select></label
            ><label class="wb-full"
              >已发布资产<select v-model="form.input.asset_ref_id" required>
                <option
                  v-if="
                    form.input.asset_ref_id &&
                    !assets.some(
                      (a) =>
                        `${a.asset_id}:${a.version}` ===
                        form.input.asset_ref_id,
                    )
                  "
                  :value="form.input.asset_ref_id"
                >
                  当前引用 · {{ form.input.asset_ref_id }}
                </option>
                <option value="">请选择</option>
                <option
                  v-for="asset in assets"
                  :key="`${asset.asset_id}:${asset.version}`"
                  :value="`${asset.asset_id}:${asset.version}`"
                >
                  {{ asset.name || asset.asset_id }} · v{{ asset.version }}
                </option>
              </select></label
            ><label
              >{{ wt("执行计划（Cron）")
              }}<input
                v-model="form.schedule.cron"
                required
                placeholder="0 9 * * *" /></label
            ><label
              >{{ wt("时区")
              }}<input
                v-model="form.schedule.timezone"
                required
                list="timezones" /><datalist id="timezones">
                <option value="Asia/Shanghai" />
                <option value="UTC" />
                <option value="America/New_York" /></datalist
            ></label>
            <label
              >{{ wt("输入更新策略")
              }}<select v-model="refreshMode">
                <option value="snapshot">{{ wt("固定已发布快照") }}</option>
                <option value="latest_published">
                  {{ wt("同来源最新已发布资产") }}
                </option>
              </select></label
            >
            <template v-if="refreshMode === 'latest_published'"
              ><label
                >明确来源类型<select v-model="refreshSourceType" required>
                  <option value="">选择来源类型</option>
                  <option
                    v-for="kind in [
                      'datasource',
                      'web',
                      'knowledge',
                      'collection',
                      'result',
                      'evaluation',
                    ]"
                    :key="kind"
                    :value="kind"
                  >
                    {{ kind }}
                  </option>
                </select></label
              ><label
                >明确来源 ID<input
                  v-model="refreshSourceID"
                  required
                  placeholder="原始资产 provenance.source_id"
              /></label>
              <p class="wb-footnote wb-full">
                以所选已发布资产为起点，每次只读取该来源最新且已获批准的发布资产。新原始数据仍需经过处理、审核和发布。
              </p></template
            >
            <p class="wb-muted wb-full">
              每天 09:00：0 9 * * *　每小时：0 * * * *　每周一：0 9 * * 1
            </p>
            <label v-if="form.kind === 'report'" class="wb-full"
              >{{ wt("报告目标")
              }}<textarea
                v-model="form.input.prompt"
                required
                placeholder="总结业务表现、趋势及异常"
              /></label
            ><template v-else
              ><label
                >{{ wt("指标名")
                }}<input v-model="form.input.metric" required /></label
              ><label
                >时间字段<input
                  v-model="form.input.time_column"
                  required /></label
              ><label
                >数值字段<input
                  v-model="form.input.value_column"
                  required /></label
              ><label
                >异常阈值<input
                  v-model.number="watch.threshold"
                  type="number"
                  step="0.1"
                  min="0.1"
                  required /></label
              ><label class="wb-full"
                >归因维度<input
                  :value="watch.attribution_dimensions.join(', ')"
                  @input="
                    watch.attribution_dimensions = split(
                      ($event.target as HTMLInputElement).value,
                    )
                  "
                  placeholder="region,product" /></label></template
            ><label class="wb-checkbox"
              ><input
                v-model="form.enabled"
                type="checkbox"
              />启用定时执行</label
            ><span class="wb-muted">通知渠道：站内</span>
          </div>
          <div class="wb-actions" style="margin-top: 20px">
            <button class="primary" :disabled="busy">保存订阅</button
            ><button
              v-if="form.id"
              type="button"
              :disabled="busy"
              @click="execute"
            >
              立即执行一次
            </button>
          </div>
          <p v-if="form.next_run_at" class="wb-footnote">
            下一次执行：{{ new Date(form.next_run_at).toLocaleString() }}
          </p>
        </form>
        <section v-if="form.id" class="wb-panel">
          <div class="wb-section-head" style="margin-top: 0">
            <h2>执行历史</h2>
            <button @click="run(() => loadRuns())">刷新</button>
          </div>
          <p v-if="!runs.length" class="wb-empty">尚无执行记录</p>
          <table v-else class="wb-table">
            <thead>
              <tr>
                <th>计划时间</th>
                <th>状态</th>
                <th>结果</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in runs" :key="item.id">
                <td>{{ new Date(item.scheduled_at).toLocaleString() }}</td>
                <td>
                  <span class="wb-badge">{{ item.status }}</span
                  ><span v-if="item.alert_triggered">异常命中</span>
                  <p v-if="item.error" class="wb-error">{{ item.error }}</p>
                </td>
                <td>
                  <router-link
                    v-if="item.result_ref_id"
                    :to="{
                      path: '/experts',
                      query: { asset: item.result_ref_id },
                    }"
                    >查看分析结果 →</router-link
                  ><span v-else class="wb-muted">暂无结果</span>
                </td>
              </tr>
            </tbody>
          </table>
          <button v-if="runNext" @click="run(() => loadRuns(true))">
            {{ wt("加载更多") }}
          </button>
        </section>
      </div>
    </div>
    <section v-else class="wb-panel">
      <div class="wb-section-head" style="margin-top: 0">
        <h2>站内通知</h2>
        <label class="wb-checkbox"
          ><input
            v-model="unreadOnly"
            type="checkbox"
            @change="run(() => loadNotifications())"
          />仅未读</label
        >
      </div>
      <p v-if="!notifications.length" class="wb-empty">
        暂无{{
          unreadOnly ? "未读" : ""
        }}通知。报告完成或看护命中后会出现在这里。
      </p>
      <article
        v-for="item in notifications"
        :key="item.id"
        class="notification-item"
      >
        <div>
          <h3>
            {{ item.title }}
            <span v-if="!item.read_at" class="wb-badge">未读</span>
          </h3>
          <p>{{ item.body }}</p>
          <small class="wb-muted">{{
            new Date(item.created_at).toLocaleString()
          }}</small>
        </div>
        <div class="wb-actions">
          <router-link
            v-if="item.result_ref_id"
            :to="{ path: '/experts', query: { asset: item.result_ref_id } }"
            >查看结果</router-link
          ><button v-if="!item.read_at" @click="markRead(item.id)">
            标记已读
          </button>
        </div>
      </article>
      <button
        v-if="notificationNext"
        @click="run(() => loadNotifications(true))"
      >
        {{ wt("加载更多") }}
      </button>
    </section>
  </div>
</template>
<script setup lang="ts">
import { useCursorOptions } from "@/composables/useCursorOptions";
import { wt } from "@/i18n/workbench";

import { onMounted, reactive, ref } from "vue";
import {
  governanceApi as api,
  type Subscription,
  type SubscriptionRun,
  type Notification,
} from "@/api/governance";
import { lifecycleApi, type AssetRef } from "@/api/lifecycle";
import "@/components/workbench/workbench.css";
const empty = () =>
  ({
    id: "",
    version: 0,
    name: "",
    kind: "report",
    enabled: true,
    schedule: { cron: "0 9 * * *", timezone: "Asia/Shanghai" },
    input: {
      asset_ref_id: "",
      prompt: "",
      metric: "",
      time_column: "",
      value_column: "",
    },
    watch: null,
    channel: "in_app",
    created_at: "",
    updated_at: "",
    next_run_at: "",
  }) as Subscription;
const form = reactive(empty()),
  watch = reactive({
    method: "zscore",
    threshold: 3,
    attribution_dimensions: [] as string[],
  }),
  subscriptions = ref<Subscription[]>([]),
  runs = ref<SubscriptionRun[]>([]),
  notifications = ref<Notification[]>([]),
  assets = ref<AssetRef[]>([]),
  busy = ref(false),
  error = ref(""),
  notice = ref(""),
  tab = ref("subscriptions"),
  next = ref(""),
  runNext = ref(""),
  notificationNext = ref(""),
  unreadOnly = ref(false);
const refreshMode = ref("snapshot"),
  refreshSourceType = ref(""),
  refreshSourceID = ref("");
const split = (v: string) =>
  v
    .split(/[,，]/)
    .map((x) => x.trim())
    .filter(Boolean);
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
async function load(append = false) {
  const res = await api.list<Subscription>(
    "/subscriptions",
    append ? next.value : "",
  );
  subscriptions.value = append
    ? [...subscriptions.value, ...(res.data?.items || [])]
    : res.data?.items || [];
  next.value = res.data?.next_cursor || "";
}
const assetOptions = useCursorOptions(assets, (cursor) =>
  lifecycleApi.assets("published_asset", cursor),
);

function newSubscription() {
  refreshMode.value = "snapshot";
  refreshSourceType.value = "";
  refreshSourceID.value = "";
  Object.assign(form, empty());
  Object.assign(watch, {
    method: "zscore",
    threshold: 3,
    attribution_dimensions: [],
  });
  runs.value = [];
  tab.value = "subscriptions";
}
async function select(item: Subscription) {
  refreshMode.value = item.input.refresh?.mode || "snapshot";
  refreshSourceType.value = item.input.refresh?.source_type || "";
  refreshSourceID.value = item.input.refresh?.source_id || "";
  Object.assign(form, JSON.parse(JSON.stringify(item)));
  Object.assign(
    watch,
    item.watch
      ? {
          ...item.watch,
          attribution_dimensions: item.watch.attribution_dimensions || [],
        }
      : {
          method: "zscore",
          threshold: 3,
          attribution_dimensions: [],
        },
  );
  await run(() => loadRuns());
}
async function save() {
  if (busy.value) return;
  await run(async () => {
    const input = {
      name: form.name,
      kind: form.kind,
      enabled: form.enabled,
      schedule: form.schedule,
      input: {
        ...form.input,
        refresh:
          refreshMode.value === "latest_published"
            ? {
                mode: "latest_published",
                source_type: refreshSourceType.value,
                source_id: refreshSourceID.value,
              }
            : undefined,
      },
      channel: "in_app",
      watch: form.kind === "metric_watch" ? watch : null,
      expected_version: form.version,
    };
    const res = form.id
      ? await api.update<Subscription>(`/subscriptions/${form.id}`, input)
      : await api.create<Subscription>("/subscriptions", input);
    if (res.data) Object.assign(form, res.data);
    await load();
    notice.value = "订阅已保存";
  });
}
let runsEpoch = 0;
async function loadRuns(append = false) {
  const id = form.id,
    request = ++runsEpoch;
  const res = await api.list<SubscriptionRun>(
    `/subscriptions/${id}/runs`,
    append ? runNext.value : "",
  );
  if (id !== form.id || request !== runsEpoch) return;
  runs.value = append
    ? [...runs.value, ...(res.data?.items || [])]
    : res.data?.items || [];
  runNext.value = res.data?.next_cursor || "";
}
async function execute() {
  if (busy.value) return;
  await run(async () => {
    await api.action(`/subscriptions/${form.id}/run`);
    await loadRuns();
    notice.value = "已提交执行，可刷新历史查看进度。";
  });
}
async function loadNotifications(append = false) {
  const res = await api.list<Notification>(
    "/notifications",
    append ? notificationNext.value : "",
    { unread: unreadOnly.value },
  );
  notifications.value = append
    ? [...notifications.value, ...(res.data?.items || [])]
    : res.data?.items || [];
  notificationNext.value = res.data?.next_cursor || "";
}
async function markRead(id: string) {
  if (busy.value) return;
  await run(async () => {
    await api.action(`/notifications/${id}/read`);
    await loadNotifications();
  });
}
onMounted(() =>
  run(async () => {
    await load();
    await assetOptions.reset();
  }),
);
</script>
<style scoped>
.notification-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  padding: 18px 0;
  border-bottom: 1px solid var(--border-default);
}
.notification-item p {
  font-size: 13px;
  line-height: 1.6;
}
</style>
