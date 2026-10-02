<template>
  <div class="workbench">
    <header class="wb-head">
      <div>
        <span class="wb-kicker">DATA INTELLIGENCE</span>
        <h1>{{ wt("数据目录") }}</h1>
        <p>从物理表到业务术语，查看资产身份、数据画像与上下游影响。</p>
      </div>
      <button @click="run(() => load())">{{ wt("刷新目录") }}</button>
    </header>
    <div v-if="error" role="alert" class="wb-error">{{ error }}</div>
    <div v-if="notice" role="status" class="wb-success">{{ notice }}</div>
    <div class="wb-tabs">
      <button
        v-for="item in tabs"
        :key="item.key"
        :class="{ selected: tab === item.key }"
        @click="tab = item.key"
      >
        {{ item.title }}
      </button>
    </div>
    <div v-if="tab === 'catalog'" class="wb-stack">
      <form class="wb-panel wb-form" @submit.prevent="sync">
        <label
          >同步数据源<select v-model="source" required>
            <option value="">选择数据源</option>
            <option v-for="ds in sources" :key="ds.id" :value="ds.id">
              {{ ds.name }}
            </option>
          </select></label
        >
        <div class="wb-actions">
          <label class="wb-checkbox"
            ><input v-model="profile" type="checkbox" />{{
              wt("生成列画像")
            }}</label
          ><button class="primary" :disabled="busy">
            {{ wt("同步结构与漂移") }}
          </button>
        </div>
      </form>
      <div class="wb-grid">
        <aside class="wb-panel">
          <form class="wb-stack" @submit.prevent="run(() => load())">
            <label class="wb-label"
              >搜索资产<input
                v-model="query"
                placeholder="表名、字段、业务术语" /></label
            ><label class="wb-label"
              >{{ wt("标签")
              }}<input v-model="tag" placeholder="按治理标签筛选" /></label
            ><label class="wb-checkbox"
              ><input
                v-model="includeDeprecated"
                type="checkbox"
              />包含已下线资产</label
            ><button :disabled="busy">{{ wt("搜索") }}</button>
          </form>
          <div class="wb-list" style="margin-top: 16px">
            <button
              v-for="item in tables"
              :key="item.id"
              :class="{ selected: selected?.id === item.id }"
              @click="open(item.id)"
            >
              <strong>{{ item.table_name }}</strong
              ><small
                >{{ item.schema }} · {{ item.classification || "internal"
                }}{{ item.deprecated ? " · 已下线" : "" }}</small
              >
            </button>
          </div>
          <p v-if="!tables.length" class="wb-empty">
            没有匹配的资产，先同步数据源。
          </p>
          <button v-if="next" @click="run(() => load(true))">
            {{ wt("加载更多") }}
          </button>
        </aside>
        <section v-if="selected" class="wb-panel">
          <div class="wb-section-head" style="margin-top: 0">
            <h2>{{ selected.table_name }}</h2>
            <span class="wb-badge">{{
              selected.deprecated ? "已下线" : "有效资产"
            }}</span>
          </div>
          <p class="wb-muted">{{ selected.comment || "暂无表说明" }}</p>
          <div class="wb-metrics">
            <div>
              <strong>{{ selected.row_count }}</strong
              ><small>估计行数</small>
            </div>
            <div>
              <strong>{{ selected.columns?.length || 0 }}</strong
              ><small>字段</small>
            </div>
          </div>
          <p class="wb-footnote">
            最近同步：{{ selected.last_synced_at }} · ID {{ selected.id }}
          </p>
          <section v-if="profileCard" class="asset-profile-card">
            <h3>数据身份卡</h3>
            <p>{{ profileCard.summary }}</p>
            <p class="wb-footnote">
              {{ profileCard.method }} · 已采样
              {{ profileCard.profiled_columns }} /
              {{ profileCard.active_columns }} 列；完整性评分
              {{
                profileCard.quality_score ?? "未评估"
              }}（基于样本空值情况，不代表全量数据质量）。
            </p>
            <ul>
              <li
                v-for="risk in profileCard.risks"
                :key="risk.field + ':' + risk.message"
              >
                {{ risk.level }} · {{ risk.field }} {{ risk.message }}
              </li>
            </ul>
            <h4>使用建议</h4>
            <ul>
              <li
                v-for="recommendation in profileCard.recommendations"
                :key="recommendation"
              >
                {{ recommendation }}
              </li>
            </ul>
            <p>{{ profileCard.suitable_for?.join(" · ") }}</p>
            <h4 v-if="profileCard.related?.length">关联资产</h4>
            <div class="wb-actions">
              <button
                v-for="asset in profileCard.related"
                :key="asset.id"
                @click="open(asset.id)"
              >
                {{ asset.name }} · {{ asset.reason }}
              </button>
            </div>
            <hr class="wb-divider" />
          </section>
          <form class="wb-form" @submit.prevent="saveGovernance">
            <label>{{ wt("负责人") }}<input v-model="selected.owner" /></label
            ><label
              >{{ wt("敏感级别")
              }}<select v-model="selected.classification">
                <option v-for="level in levels" :key="level" :value="level">
                  {{ level }}
                </option>
              </select></label
            ><label class="wb-full"
              >{{ wt("标签")
              }}<input
                :value="(selected.tags || []).join(', ')"
                @input="
                  selected.tags = split(
                    ($event.target as HTMLInputElement).value,
                  )
                "
                placeholder="逗号分隔"
            /></label>
            <div class="wb-actions wb-full">
              <button class="primary" :disabled="busy">
                {{ wt("保存治理信息") }}</button
              ><button type="button" @click="showLineage(selected.id)">
                {{ wt("查看血缘与影响") }}
              </button>
            </div>
          </form>
          <hr class="wb-divider" />
          <h3>{{ wt("字段画像与治理") }}</h3>
          <div class="wb-table-wrap">
            <table class="wb-table">
              <thead>
                <tr>
                  <th>字段</th>
                  <th>画像</th>
                  <th>负责人 / 标签</th>
                  <th>{{ wt("敏感级别") }}</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="column in selected.columns" :key="column.id">
                  <td>
                    <strong>{{ column.name }}</strong>
                    <p class="wb-muted">
                      {{ column.data_type }} {{ column.is_pk ? "· 主键" : "" }}
                      {{ column.deprecated ? "· 下线" : "" }}
                    </p>
                    <small>{{ column.comment }}</small>
                  </td>
                  <td>
                    <template v-if="column.profiled"
                      >空值
                      {{
                        column.null_rate === null
                          ? "—"
                          : `${(column.null_rate * 100).toFixed(1)}%`
                      }}<br />基数 {{ column.distinct_count ?? "—" }}
                      <p class="wb-muted">
                        样例：{{ column.sample_values?.join(" / ") || "无" }}
                      </p></template
                    ><span v-else class="wb-muted">尚未生成画像</span>
                  </td>
                  <td>
                    <input
                      v-model="column.owner"
                      :aria-label="`${column.name}负责人`"
                    /><input
                      :value="(column.tags || []).join(', ')"
                      :aria-label="`${column.name}标签`"
                      @input="
                        column.tags = split(
                          ($event.target as HTMLInputElement).value,
                        )
                      "
                    />
                  </td>
                  <td>
                    <select
                      v-model="column.classification"
                      :aria-label="`${column.name}敏感级别`"
                    >
                      <option
                        v-for="level in levels"
                        :key="level"
                        :value="level"
                      >
                        {{ level }}
                      </option>
                    </select>
                  </td>
                  <td>
                    <button :disabled="busy" @click="saveColumn(column)">
                      {{ wt("保存") }}</button
                    ><button @click="showLineage(column.id)">
                      {{ wt("血缘") }}
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
        <div v-else class="wb-panel wb-empty">
          选择左侧资产，查看身份卡和数据画像。
        </div>
      </div>
    </div>
    <div v-else-if="tab === 'glossary'" class="wb-grid">
      <aside class="wb-panel">
        <button class="primary" @click="newTerm">＋ 新建术语</button>
        <div class="wb-list" style="margin-top: 16px">
          <button
            v-for="item in terms"
            :key="item.id"
            @click="Object.assign(term, JSON.parse(JSON.stringify(item)))"
          >
            <strong>{{ item.term }}</strong
            ><small>{{ (item.synonyms || []).join(" / ") }}</small>
          </button>
        </div>
        <button v-if="termNext" @click="run(() => loadTerms(true))">
          {{ wt("加载更多") }}
        </button>
      </aside>
      <form class="wb-panel wb-stack" @submit.prevent="saveTerm">
        <h2>{{ term.id ? "编辑术语" : "新建术语" }}</h2>
        <label class="wb-label"
          >{{ wt("业务术语") }}<input v-model="term.term" required /></label
        ><label class="wb-label"
          >定义<textarea v-model="term.description" required /></label
        ><label class="wb-label"
          >同义词<input
            :value="(term.synonyms || []).join(', ')"
            @input="
              term.synonyms = split(($event.target as HTMLInputElement).value)
            " /></label
        ><label class="wb-label"
          >加载表字段<select
            v-model="glossaryTableID"
            @change="loadGlossaryColumns"
          >
            <option value="">选择表以关联列</option>
            <option v-for="table in tables" :key="table.id" :value="table.id">
              {{ table.table_name }}
            </option>
          </select></label
        ><label class="wb-label"
          >关联资产<select multiple v-model="term.asset_ids" size="5">
            <option
              v-for="column in glossaryColumns"
              :key="column.id"
              :value="column.id"
            >
              字段 · {{ column.name }}
            </option>
            <option v-for="item in tables" :key="item.id" :value="item.id">
              {{ item.table_name }}
            </option>
          </select></label
        >
        <p class="wb-footnote">
          可关联当前已加载的目录资产；保存时保留已有的关联 ID。
        </p>
        <div class="wb-actions">
          <button class="primary" :disabled="busy">保存术语</button
          ><button
            v-if="term.id"
            type="button"
            class="danger"
            @click="deleteTerm"
          >
            删除术语
          </button>
        </div>
      </form>
    </div>
    <div v-else class="wb-stack">
      <form class="wb-panel wb-form" @submit.prevent="loadLineage">
        <label
          >资产 ID<input
            v-model="assetID"
            required
            placeholder="从目录身份卡进入可自动填写" /></label
        ><label
          >方向<select v-model="direction">
            <option value="both">上下游</option>
            <option value="upstream">上游来源</option>
            <option value="downstream">下游依赖</option>
          </select></label
        ><label
          >查询深度<input v-model.number="depth" type="number" min="1" max="10"
        /></label>
        <div class="wb-actions">
          <button class="primary" :disabled="busy">查询血缘</button
          ><button type="button" :disabled="busy || !assetID" @click="impact">
            影响分析
          </button>
        </div>
      </form>
      <section v-if="graph" class="wb-panel">
        <h2>
          {{ graph.nodes.length }} 个资产 · {{ graph.edges.length }} 条关系
        </h2>
        <div class="lineage-nodes">
          <button
            v-for="node in graph.nodes"
            :key="node.id"
            @click="showLineage(node.asset_id || node.id)"
          >
            <span class="wb-kicker">{{ node.kind }}</span
            ><strong>{{ node.name || node.id }}</strong
            ><small>{{ node.unresolved ? "待对齐" : "已解析" }}</small>
          </button>
        </div>
        <table class="wb-table">
          <thead>
            <tr>
              <th>来源</th>
              <th>{{ wt("关系") }}</th>
              <th>去向</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(edge, i) in graph.edges" :key="i">
              <td>{{ nodeName(edge.from) }}</td>
              <td>→ {{ edge.kind }}</td>
              <td>{{ nodeName(edge.to) }}</td>
            </tr>
          </tbody>
        </table>
        <p v-if="!graph.nodes.length" class="wb-empty">
          暂无血缘。可选择语义模型补建。
        </p>
      </section>
      <form class="wb-panel wb-actions" @submit.prevent="rebuild">
        <label class="wb-label"
          >从语义模型补建<select v-model="modelID" required>
            <option value="">选择模型</option>
            <option
              v-for="model in semanticModels"
              :key="model.id"
              :value="model.id"
            >
              {{ model.name }}
            </option>
          </select></label
        ><button :disabled="busy">补建血缘</button>
      </form>
    </div>
  </div>
</template>
<script setup lang="ts">
import { wt } from "@/i18n/workbench";

import { onMounted, reactive, ref } from "vue";
import "@/components/workbench/workbench.css";
import {
  catalogApi,
  type CatalogTable,
  type CatalogColumn,
  type Glossary,
  type ProfileCard,
  type LineageGraph,
} from "@/api/catalog";
import { semanticApi, type SemanticHead } from "@/api/semantic";
import { http } from "@/utils/request";
const tabs = [
    { key: "catalog", title: "资产目录" },
    { key: "glossary", title: "业务术语" },
    { key: "lineage", title: "血缘与影响" },
  ],
  levels = ["public", "internal", "confidential", "secret"];
const profileCard = ref<ProfileCard>(),
  glossaryTableID = ref(""),
  glossaryColumns = ref<CatalogColumn[]>([]);
const tab = ref("catalog"),
  busy = ref(false),
  error = ref(""),
  notice = ref(""),
  source = ref(""),
  profile = ref(false),
  query = ref(""),
  tag = ref(""),
  includeDeprecated = ref(false),
  next = ref(""),
  tables = ref<CatalogTable[]>([]),
  selected = ref<CatalogTable>(),
  sources = ref<{ id: string; name: string }[]>([]),
  terms = ref<Glossary[]>([]),
  termNext = ref(""),
  assetID = ref(""),
  direction = ref("both"),
  depth = ref(3),
  graph = ref<LineageGraph>(),
  modelID = ref(""),
  semanticModels = ref<SemanticHead[]>([]);
const blankTerm = (): Glossary => ({
    id: "",
    term: "",
    description: "",
    synonyms: [],
    asset_ids: [],
  }),
  term = reactive(blankTerm());
const split = (value: string) =>
  value
    .split(/[,，]/)
    .map((v) => v.trim())
    .filter(Boolean);
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
  const res = await catalogApi.list({
    q: query.value,
    tag: tag.value,
    include_deprecated: includeDeprecated.value,
    cursor: append ? next.value : "",
    page_size: 100,
  });
  tables.value = append
    ? [...tables.value, ...(res.data?.items || [])]
    : res.data?.items || [];
  next.value = res.data?.next_cursor || "";
}
async function loadGlossaryColumns() {
  await run(async () => {
    glossaryColumns.value = glossaryTableID.value
      ? (await catalogApi.get(glossaryTableID.value)).data?.columns || []
      : [];
  });
}
async function open(id: string) {
  await run(async () => {
    profileCard.value = undefined;
    selected.value = (await catalogApi.get(id)).data;
    profileCard.value = (await catalogApi.profileCard(id)).data;
  });
}
async function sync() {
  await run(async () => {
    const res = await catalogApi.sync(source.value, profile.value);
    await load();
    notice.value = `同步完成：${res.data?.tables} 张表，新增 ${res.data?.created}，更新 ${res.data?.updated}，下线 ${res.data?.deprecated}。${res.data?.warnings?.join("；") || ""}`;
  });
}
async function saveGovernance() {
  if (!selected.value) return;
  await run(async () => {
    await catalogApi.govern("tables", selected.value!.id, selected.value!);
    notice.value = "治理信息已保存";
    await load();
  });
}
async function saveColumn(column: CatalogColumn) {
  await run(async () => {
    await catalogApi.govern("columns", column.id, column);
    notice.value = `${column.name} 已保存`;
  });
}
async function loadTerms(append = false) {
  const res = await catalogApi.glossary(append ? termNext.value : "");
  terms.value = append
    ? [...terms.value, ...(res.data?.items || [])]
    : res.data?.items || [];
  termNext.value = res.data?.next_cursor || "";
}
function newTerm() {
  Object.assign(term, blankTerm());
}
async function saveTerm() {
  await run(async () => {
    const res = await catalogApi.saveTerm(term);
    if (res.data) Object.assign(term, res.data);
    await loadTerms();
    notice.value = "术语已保存";
  });
}
async function deleteTerm() {
  if (!window.confirm(`删除术语“${term.term}”？`)) return;
  await run(async () => {
    await catalogApi.deleteTerm(term.id);
    newTerm();
    await loadTerms();
  });
}
async function showLineage(id: string) {
  tab.value = "lineage";
  assetID.value = id;
  await loadLineage();
}
async function loadLineage() {
  await run(async () => {
    graph.value = (
      await catalogApi.lineage(assetID.value, direction.value, depth.value)
    ).data;
  });
}
async function impact() {
  await run(async () => {
    graph.value = (await catalogApi.impact(assetID.value)).data;
  });
}
async function rebuild() {
  await run(async () => {
    await catalogApi.rebuild(modelID.value);
    notice.value = "血缘已补建，请查询相关资产。";
  });
}
function nodeName(id: string) {
  return graph.value?.nodes.find((n) => n.id === id)?.name || id;
}
onMounted(() =>
  run(async () => {
    await Promise.all([load(), loadTerms()]);
    sources.value =
      (
        await http.get<{ items: { id: string; name: string }[] }>(
          "/datasources",
          { params: { page_size: 100 } },
        )
      ).data?.items || [];
    semanticModels.value = (await semanticApi.list()).data?.items || [];
  }),
);
</script>
<style scoped>
.lineage-nodes {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin: 18px 0;
}
.lineage-nodes button {
  display: flex;
  flex-direction: column;
  gap: 9px;
  text-align: left;
  min-width: 160px;
}
.lineage-nodes strong {
  font-size: 14px;
}
.lineage-nodes small {
  color: var(--text-muted);
}
</style>
