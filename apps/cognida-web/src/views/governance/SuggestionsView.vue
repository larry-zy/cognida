<template>
  <div class="workbench">
    <header class="wb-head">
      <div>
        <span class="wb-kicker">REVIEWED LEARNING</span>
        <h1>{{ wt("反馈与改进建议") }}</h1>
        <p>
          将未覆盖术语、对话洞察和评测反馈转为待审建议；采纳后实际写入目标资产。
        </p>
      </div>
      <div class="wb-actions">
        <button :disabled="busy" @click="discover">
          {{ wt("发现改进机会") }}</button
        ><button class="primary" @click="newSuggestion">
          {{ wt("＋ 记录建议") }}
        </button>
      </div>
    </header>
    <div v-if="error" role="alert" class="wb-error">{{ error }}</div>
    <div v-if="notice" role="status" class="wb-success">{{ notice }}</div>
    <div class="wb-grid">
      <aside class="wb-panel">
        <label class="wb-label"
          >审核状态<select v-model="status" @change="run(() => load())">
            <option value="">{{ wt("全部") }}</option>
            <option value="pending">{{ wt("待审核") }}</option>
            <option value="applied">{{ wt("已采纳") }}</option>
            <option value="rejected">{{ wt("已拒绝") }}</option>
            <option value="failed">{{ wt("采纳失败") }}</option>
          </select></label
        >
        <div class="wb-list" style="margin-top: 16px">
          <button
            v-for="item in suggestions"
            :key="item.id"
            :class="{ selected: current?.id === item.id }"
            @click="current = item"
          >
            <strong>{{ item.title }}</strong
            ><small>{{ item.kind }} · {{ item.status }}</small>
          </button>
        </div>
        <p v-if="!suggestions.length" class="wb-empty">暂无改进建议</p>
        <button v-if="next" @click="run(() => load(true))">
          {{ wt("加载更多") }}
        </button>
      </aside>
      <form v-if="!current" class="wb-panel" @submit.prevent="create">
        <h2>{{ wt("记录可验证的改进建议") }}</h2>
        <div class="wb-form">
          <label
            >来源类型<select v-model="form.source.type">
              <option value="uncovered_term">未覆盖术语</option>
              <option value="conversation_insight">对话洞察</option>
              <option value="evaluation_feedback">评测反馈</option>
            </select></label
          ><label
            >来源记录 ID<input
              v-model="form.source.id"
              required
              placeholder="覆盖事件、消息/会话或评测记录ID" /></label
          ><label>建议标题<input v-model="form.title" required /></label
          ><label
            >采纳目标<select v-model="form.kind">
              <option value="knowledge">知识条目</option>
              <option value="term">指标同义词</option>
              <option value="experience">经验</option>
            </select></label
          ><label class="wb-full"
            >建议内容<textarea v-model="form.content" required /></label
          ><label class="wb-full"
            >证据资产<select multiple v-model="form.evidence_ref_ids" size="4">
              <option
                v-for="asset in assets"
                :key="asset.asset_id"
                :value="`${asset.asset_id}:${asset.version}`"
              >
                {{ asset.name || asset.asset_id }} · {{ asset.kind }}
              </option>
            </select></label
          ><template v-if="form.kind === 'knowledge'"
            ><label
              >目标知识库<select v-model="knowledgeBaseID" required>
                <option value="">选择知识库</option>
                <option
                  v-for="kb in knowledgeBases"
                  :key="kb.id"
                  :value="kb.id"
                >
                  {{ kb.name }}
                </option>
              </select></label
            ><label>知识标题<input v-model="knowledgeTitle" required /></label
            ><label class="wb-full"
              >知识正文<textarea
                v-model="knowledgeContent"
                required
                rows="5"
              /></label></template
          ><template v-else-if="form.kind === 'term'"
            ><label
              >语义模型<select v-model="modelID" required @change="loadMetrics">
                <option value="">选择模型</option>
                <option
                  v-for="model in models"
                  :key="model.id"
                  :value="model.id"
                >
                  {{ model.name }}
                </option>
              </select></label
            ><label
              >目标指标<select v-model="metricID" required>
                <option value="">选择指标</option>
                <option
                  v-for="metric in metrics"
                  :key="String(metric.id)"
                  :value="String(metric.id)"
                >
                  {{ metric.name }}
                </option>
              </select></label
            ><label class="wb-full"
              >待补充同义词<input
                v-model="synonyms"
                required
                placeholder="逗号分隔" /></label></template
          ><template v-else
            ><label
              >Agent 类型<input
                v-model="agentType"
                required
                placeholder="data_analysis" /></label
            ><label>适用问题<input v-model="question" required /></label
            ><label class="wb-full"
              >经验答案<textarea v-model="answer" required /></label
          ></template>
        </div>
        <p class="wb-footnote">
          服务端会验证来源与证据的真实性和租户权限。提交后进入待审核状态。
        </p>
        <button class="primary" :disabled="busy">
          {{ wt("提交待审建议") }}
        </button>
      </form>
      <section v-else class="wb-panel">
        <div class="wb-section-head" style="margin-top: 0">
          <h2>{{ current.title }}</h2>
          <span class="wb-badge">{{ current.status }}</span>
        </div>
        <p class="wb-muted">
          来源 {{ current.source.type }} · {{ current.source.id }}
        </p>
        <p class="suggestion-content">{{ current.content }}</p>
        <h3>建议采纳内容</h3>
        <dl>
          <template v-for="(value, key) in current.proposal" :key="key"
            ><dt>{{ proposalLabel(String(key)) }}</dt>
            <dd>
              {{ Array.isArray(value) ? value.join("、") : value }}
            </dd></template
          >
        </dl>
        <h3>证据</h3>
        <div class="wb-actions">
          <router-link
            v-for="id in current.evidence_ref_ids"
            :key="id"
            :to="{ path: '/experts', query: { asset: id } }"
            >{{ id }}</router-link
          >
        </div>
        <p v-if="!current.evidence_ref_ids?.length" class="wb-muted">
          没有附加资产引用，请结合来源记录审查。
        </p>
        <p v-if="current.error" class="wb-error">{{ current.error }}</p>
        <div v-if="current.target_ref_id" class="wb-success">
          已写入目标：{{ current.target_ref_id }}
        </div>
        <template
          v-if="current.status === 'pending' || current.status === 'failed'"
          ><hr class="wb-divider" />
          <label class="wb-label"
            >审核意见<textarea v-model="reviewComment" />
          </label>
          <div class="wb-actions" style="margin-top: 14px">
            <button class="primary" :disabled="busy" @click="review('approve')">
              {{ wt("批准并采纳") }}</button
            ><button :disabled="busy" @click="review('reject')">
              {{ wt("拒绝建议") }}
            </button>
          </div>
          <p class="wb-footnote">
            批准将真实更新目标知识、术语或经验。不会自动修改生产分析策略。
          </p></template
        >
        <p v-else class="wb-muted">
          审核意见：{{ current.review_comment || "未填写" }}
        </p>
      </section>
    </div>
  </div>
</template>
<script setup lang="ts">
import { collectPages } from "@/utils/cursorPages";
import { wt } from "@/i18n/workbench";

import { onMounted, reactive, ref } from "vue";
import { governanceApi as api, type Suggestion } from "@/api/governance";
import { lifecycleApi, type AssetRef } from "@/api/lifecycle";
import {
  semanticApi,
  type SemanticHead,
  type SemanticRow,
} from "@/api/semantic";
import { http } from "@/utils/request";
import "@/components/workbench/workbench.css";
const suggestions = ref<Suggestion[]>([]),
  current = ref<Suggestion>(),
  assets = ref<AssetRef[]>([]),
  models = ref<SemanticHead[]>([]),
  metrics = ref<SemanticRow[]>([]),
  knowledgeBases = ref<{ id: string; name: string }[]>([]),
  busy = ref(false),
  error = ref(""),
  notice = ref(""),
  status = ref(""),
  next = ref(""),
  reviewComment = ref(""),
  knowledgeBaseID = ref(""),
  knowledgeTitle = ref(""),
  knowledgeContent = ref(""),
  modelID = ref(""),
  metricID = ref(""),
  synonyms = ref(""),
  agentType = ref("data_analysis"),
  question = ref(""),
  answer = ref("");
const form = reactive({
  source: { type: "uncovered_term", id: "" },
  kind: "knowledge",
  title: "",
  content: "",
  evidence_ref_ids: [] as string[],
});
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
  const res = await api.list<Suggestion>(
    "/improvement-suggestions",
    append ? next.value : "",
    { status: status.value },
  );
  suggestions.value = append
    ? [...suggestions.value, ...(res.data?.items || [])]
    : res.data?.items || [];
  next.value = res.data?.next_cursor || "";
}
async function discover() {
  await run(async () => {
    const response = await api.action<{ items: Suggestion[] }>(
      "/improvement-suggestions/discover",
      { source_types: ["coverage", "conversation", "evaluation"], limit: 20 },
    );
    await load();
    notice.value = response.data?.items.length
      ? `发现 ${response.data.items.length} 条建议，请逐条审核`
      : "暂无新的改进建议";
    if (response.data?.items[0]) current.value = response.data.items[0];
  });
}
function newSuggestion() {
  current.value = undefined;
  reviewComment.value = "";
}
async function loadMetrics() {
  await run(async () => {
    metrics.value = (await semanticApi.get(modelID.value)).data?.metrics || [];
    metricID.value = "";
  });
}
async function create() {
  await run(async () => {
    const proposal =
      form.kind === "knowledge"
        ? {
            knowledge_base_id: knowledgeBaseID.value,
            title: knowledgeTitle.value,
            content: knowledgeContent.value,
          }
        : form.kind === "term"
          ? {
              target_type: "semantic_metric",
              target_id: metricID.value,
              synonyms: synonyms.value
                .split(/[,，]/)
                .map((v) => v.trim())
                .filter(Boolean),
            }
          : {
              agent_type: agentType.value,
              question: question.value,
              answer: answer.value,
            };
    const res = await api.create<Suggestion>("/improvement-suggestions", {
      ...form,
      proposal,
    });
    current.value = res.data;
    await load();
    notice.value = "建议已进入待审核列表";
  });
}
async function review(decision: string) {
  if (!current.value) return;
  await run(async () => {
    const res = await api.action<Suggestion>(
      `/improvement-suggestions/${current.value!.id}/review`,
      {
        decision,
        comment: reviewComment.value,
        expected_version: current.value!.version,
      },
    );
    if (res.data) current.value = res.data;
    await load();
    notice.value =
      current.value?.status === "applied"
        ? "建议已实际采纳"
        : `审核状态：${current.value?.status}`;
  });
}
function proposalLabel(key: string) {
  return (
    (
      {
        knowledge_base_id: "目标知识库",
        title: "标题",
        content: "正文",
        target_type: "目标类型",
        target_id: "指标ID",
        synonyms: "同义词",
        agent_type: "Agent类型",
        question: "适用问题",
        answer: "经验答案",
      } as Record<string, string>
    )[key] || key
  );
}
onMounted(() =>
  run(async () => {
    await load();
    const [a, m, k] = await Promise.all([
      collectPages((cursor) => lifecycleApi.assets("", cursor)),
      semanticApi.list(),
      http.get<{ id: string; name: string }[]>("/knowledge-bases"),
    ]);
    assets.value = a;
    models.value = m.data?.items || [];
    knowledgeBases.value = k.data || [];
  }),
);
</script>
<style scoped>
.suggestion-content {
  font-size: 14px;
  line-height: 1.8;
  white-space: pre-wrap;
  margin: 20px 0;
}
dl {
  display: grid;
  grid-template-columns: 110px 1fr;
  gap: 12px;
  font-size: 13px;
  line-height: 1.7;
}
dt {
  color: var(--text-muted);
}
dd {
  margin: 0;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
