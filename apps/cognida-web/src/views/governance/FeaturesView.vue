<template>
  <div class="workbench">
    <header class="wb-head">
      <div>
        <span class="wb-kicker">VERSIONED FEATURES</span>
        <h1>{{ wt("特征存储") }}</h1>
        <p>注册可追溯的特征定义，按实体写入与读取指定版本的特征值。</p>
      </div>
      <button class="primary" @click="newFeature">
        {{ wt("＋ 注册特征") }}
      </button>
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
            ><label
              >{{ wt("来源字段")
              }}<input
                v-model="form.definition.field"
                required
                placeholder="如：total_spend"
            /></label>
          </div>
          <label class="wb-label" style="margin-top: 14px"
            >实体标识字段（可选）<input
              v-model="form.definition.entity_field"
              placeholder="如：customer_id；留空按行生成实体ID"
          /></label>
          <p class="wb-footnote">
            定义采用字段映射。已有版本不可覆盖，修改定义将创建新版本。
          </p>
          <button class="primary" :disabled="busy">
            {{ form.id ? "创建新版本" : "注册特征" }}
          </button>
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
                    {{ version.definition.kind }} ·
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
          <button :disabled="busy" @click="compute">
            {{ wt("从来源资产计算所选版本") }}
          </button>
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
                <td>{{ JSON.stringify(item.value) }}</td>
                <td>{{ item.timestamp }}<br />{{ item.source_ref_id }}</td>
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
import { useCursorOptions } from "@/composables/useCursorOptions";
import { wt } from "@/i18n/workbench";

import { computed, onMounted, reactive, ref } from "vue";
import {
  governanceApi as api,
  type Feature,
  type FeatureValue,
} from "@/api/governance";
import { lifecycleApi, type AssetRef } from "@/api/lifecycle";
import "@/components/workbench/workbench.css";
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
  queried = ref(false);
const versionOptions = useCursorOptions(versions, (cursor) =>
  api.list<Feature>(`/features/${form.id}/versions`, cursor),
);
const assetOptions = useCursorOptions(assets, (cursor) =>
  lifecycleApi.assets("", cursor),
);

function localTime() {
  const date = new Date();
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 16);
}
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
  const res = await api.list<Feature>("/features", append ? next.value : "");
  features.value = append
    ? [...features.value, ...(res.data?.items || [])]
    : res.data?.items || [];
  next.value = res.data?.next_cursor || "";
}
function newFeature() {
  Object.assign(form, empty());
  versions.value = [];
  values.value = [];
  queried.value = false;
}
async function loadVersions() {
  await versionOptions.reset();
}
async function select(item: Feature) {
  Object.assign(form, JSON.parse(JSON.stringify(item)));
  valueVersion.value = item.version;
  values.value = [];
  queried.value = false;
  await run(loadVersions);
}
async function save() {
  if (busy.value) return;
  await run(async () => {
    const res = await api.create<Feature>(
      form.id ? `/features/${form.id}/versions` : "/features",
      {
        name: form.name,
        type: form.type,
        description: form.description,
        definition: form.definition,
        source_ref_id: form.source_ref_id,
        expected_version: form.version,
      },
    );
    if (res.data) {
      Object.assign(form, res.data);
      valueVersion.value = form.version;
    }
    await load();
    await loadVersions();
    notice.value = `版本 v${form.version} 已保存`;
  });
}
const selectedVersion = computed(
  () =>
    versions.value.find((v) => v.version === valueVersion.value) ||
    (form.version === valueVersion.value ? form : undefined),
);
function parsedValue() {
  if (!selectedVersion.value) throw new Error("请选择已有的特征版本");
  if (selectedVersion.value.type === "numerical") {
    const n = Number(value.value);
    if (!Number.isFinite(n)) throw new Error("请输入有效数值");
    return n;
  }
  if (selectedVersion.value.type === "vector") {
    const numbers = value.value.split(/[,，]/).map((v) => Number(v.trim()));
    if (!numbers.length || numbers.some((n) => !Number.isFinite(n)))
      throw new Error("向量必须为逗号分隔的有限数值");
    return numbers;
  }
  return value.value;
}
async function writeValue() {
  if (busy.value) return;
  await run(async () => {
    await api.create("/feature-values", {
      values: [
        {
          feature_id: form.id,
          version: valueVersion.value,
          entity_id: entityID.value,
          value: parsedValue(),
          timestamp: new Date(timestamp.value).toISOString(),
        },
      ],
    });
    queryEntities.value = entityID.value;
    notice.value = "特征值已写入";
  });
}
async function compute() {
  if (busy.value) return;
  await run(async () => {
    const response = await api.action<{ count: number }>(
      `/features/${form.id}/compute`,
      { version: valueVersion.value },
    );
    notice.value = `已计算并写入 ${response.data?.count || 0} 个实体特征值`;
  });
}
async function query() {
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
    values.value = Array.isArray(res.data) ? res.data : res.data?.items || [];
    queried.value = true;
  });
}
onMounted(() =>
  run(async () => {
    await load();
    await assetOptions.reset();
  }),
);
</script>
