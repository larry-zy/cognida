<template>
  <div ref="workbenchRoot" class="workbench lifecycle-workbench">
    <header class="wb-head">
      <div>
        <span class="wb-kicker">COMPOSABLE DATA WORKFLOWS</span>
        <h1>
          {{
            wt(
              isPipeline
                ? "数据流水线"
                : isAnalysis
                  ? "数据分析"
                  : "专家工作台",
            )
          }}
        </h1>
        <p>
          {{
            isPipeline
              ? "按受控模板编排任务，查看每一步的产出、质量门禁与审批。"
              : "独立调用数据接入、处理、分析与评估专家，所有输出保留来源。"
          }}
        </p>
      </div>
      <router-link to="/analysis/chat">历史数据源对话 →</router-link>
    </header>
    <div v-if="error" class="wb-error" role="alert">{{ error }}<button v-if="initialError" type="button" @click="initialize">重新加载工作台</button><button v-if="detailErrorID" type="button" @click="inspect(detailErrorID)">重试读取执行详情</button></div>
    <div v-if="notice" class="wb-success" role="status">{{ notice }}</div>
    <p v-if="initialLoading" class="wb-muted" role="status">正在加载可用模板、资产与执行历史…</p>
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
      <aside class="wb-panel wb-stack launch-panel">
        <div><h2>{{ wt("发起任务") }}</h2><p class="wb-muted">选择输入，确认处理方式，再开始执行。</p></div>
        <form class="wb-stack launch-form" novalidate @submit.prevent="start">
          <label class="wb-label"
            >{{ isPipeline ? "流水线模板" : "专家"
            }}<select v-model="executor" required>
              <option value="">请选择</option>
              <template v-if="isPipeline"
                ><option
                  v-for="item in templates"
                  :key="item.id"
                  :value="item.id"
                >
                  {{ item.name }} · v{{ item.version }}
                </option></template
              ><template v-else
                ><option v-for="item in agents" :key="item.id" :value="item.id">
                  {{ stageLabel(item.id) }}
                </option></template
              >
            </select></label
          >
          <p v-if="selectedAgent" class="wb-muted">
            工具权限：{{
              selectedAgent.tool_allowlist.join("、") || "无业务工具"
            }}<br />风险等级：{{ selectedAgent.risk_class }}
          </p>
          <label class="wb-label"
            >{{ wt("输入资产")
            }}<select ref="inputSelect" v-model="assetSelection" required>
              <option value="">选择输入资产</option>
              <option
                v-for="asset in assets"
                :key="asset.asset_id + asset.version"
                :value="`${asset.asset_id}:${asset.version}`"
              >
                {{ asset.name || asset.asset_id.slice(0, 16) }} ·
                {{ assetKindLabel(asset.kind) }} · v{{ asset.version }}
              </option>
            </select></label
          ><label class="wb-checkbox"
            ><input
              v-model="publishedOnly"
              type="checkbox"
              @change="run(loadAssets)"
            />{{ wt("仅显示已发布资产") }}</label
          >
          <button type="button" class="create-input-shortcut" @click="openInputDraft">＋ 创建新的输入资产</button>
          <p v-if="!assets.length && !assetOptions.state.loading" class="wb-muted">{{ publishedOnly ? '暂无可选的已发布资产，可取消筛选查看其他输入。' : '暂无可选输入。先创建资产，再配置处理任务。' }}</p>
          <p v-if="inputLoading" class="wb-muted" role="status">正在确认输入资产类型…</p>
          <p v-if="inputError" class="wb-error" role="alert">{{ inputError }}</p>
          <button v-if="inputError" type="button" @click="resolveSelectedInput">重试读取输入资产</button>
          <details
            v-for="stage in parameterStages"
            :key="stage.id"
            class="stage-options"
            :data-options-stage="stage.id"
            :open="!isPipeline || ['data-asset', 'data-analysis'].includes(stage.agent_id)"
          >
            <summary><span>{{ stageLabel(stage.agent_id) }}</span><small>{{ optionSummary(stage.agent_id, optionDrafts[stage.id]) }}</small></summary>
          <ExpertOptionsForm
            :stage-id="stage.id"
            :agent-id="stage.agent_id"
            :title="stageLabel(stage.agent_id)"
            :model-value="optionDrafts[stage.id] || defaultOptions(stage.agent_id)"
            :input-kind="selectedInputKind"
            :targets="publicationTargets"
            @update:model-value="optionDrafts[stage.id] = $event"
          />
          </details>
          <CleaningReview v-if="!isPipeline && executor === 'data-cleaning'" :source-ref="assetSelection" :options="cleaningPreviewOptions" @confirmed="cleaningPreviewDigest=$event" />
          <div v-if="needsPublication && selectedInputKind === 'text'" class="wb-stack">
            <p v-if="targetError" class="wb-error" role="alert">{{ targetError }}</p>
            <p v-else-if="!targetLoading && !publicationTargets.length" class="wb-muted">当前工作空间暂无知识库，请先在知识库页面创建。</p>
            <button v-if="targetHasMore || targetLoading || targetError" type="button" :disabled="targetLoading" @click="loadPublicationTargets">
              {{ targetLoading ? '正在加载知识库…' : targetError ? '重试加载知识库' : '加载更多知识库' }}
            </button>
          </div>
          <p v-if="parameterStages.length" class="wb-muted configuration-note">{{ isPipeline ? '质检、清洗与增强已设默认参数，可展开调整。发布前会暂停等待审批。' : '检查参数后开始执行，所有产出保留来源。' }}</p>
          <button class="primary start-action" :disabled="busy || inputLoading">
            {{ wt("开始执行") }}
          </button>
          <span v-if="writing" role="status" class="wb-muted">正在提交，请勿重复操作…</span>
        </form>
        <details ref="createPanel" class="wb-panel input-draft" :open="createExpanded || sourcePrefilled" @toggle="createExpanded = ($event.target as HTMLDetailsElement).open">
          <summary>{{ wt("创建输入资产") }}</summary>
          <form
            class="wb-stack"
            style="margin-top: 18px"
            @submit.prevent="createAsset"
          >
            <label class="wb-label"
              >{{ wt("资产名称") }}<input v-model="assetName" required /></label
            ><label class="wb-label"
              >{{ wt("数据类型")
              }}<select v-model="inputKind">
                <option value="text">文本</option>
                <option value="table">表格</option>
                <option value="web">网页 URL</option>
                <option value="knowledge">已有知识文档</option>
                <option value="datasource">数据源表</option>
                <option value="collection">采集运行产物</option>
                <option value="result">已有查询结果</option>
                <option value="evaluation">评测结果</option>
              </select></label
            ><label v-if="inputKind === 'text'" class="wb-label"
              >{{ wt("文本内容")
              }}<textarea v-model="inputText" required rows="6" /></label
            ><template v-else-if="inputKind === 'table'"
              ><label class="wb-label"
                >{{ wt("列名")
                }}<input
                  v-model="inputColumns"
                  required
                  placeholder="date,revenue,region" /></label
              ><p class="wb-muted">列默认按文本保留；需要均值等数值统计时，请将对应列设为数值。编号、账号等应保留文本类型。</p>
              <label v-for="field in rowFields" :key="field.key" class="wb-label">{{ field.label }} 类型
                <select :aria-label="`${field.label} 类型`" :value="columnTypes[field.key] || 'text'" @change="columnTypes[field.key] = ($event.target as HTMLSelectElement).value as 'text' | 'number'">
                  <option value="text">文本</option><option value="number">数值</option>
                </select>
              </label>
              <RecordEditor
                v-model="inputRows"
                title="数据行"
                :fields="rowFields" /></template
            ><label v-else-if="inputKind === 'web'" class="wb-label"
              >网页地址<input v-model="sourceURL" type="url" required /></label
            ><label v-else-if="inputKind === 'knowledge'" class="wb-label"
              >知识文档 ID<input v-model="knowledgeID" required /></label
            ><template
              v-else-if="
                ['collection', 'result', 'evaluation'].includes(inputKind)
              "
              ><label v-if="inputKind !== 'result'" class="wb-label"
                >运行 / 评测任务 ID<input
                  v-model="sourceRunID"
                  required /></label
              ><label v-if="inputKind === 'collection'" class="wb-label"
                >采集条目 ID<input v-model="sourceItemID" required /></label
              ><template v-if="inputKind === 'result'"
                ><label class="wb-label"
                  >会话 ID<input v-model="sourceSessionID" required /></label
                ><label class="wb-label"
                  >查询结果 ID<input
                    v-model="sourceResultID"
                    required /></label></template></template
            ><template v-else
              ><p class="wb-muted">导入当前表最多 1,000 行的快照。</p><label class="wb-label"
                >数据源<select v-model="datasourceID" required>
                  <option value="">请选择</option>
                  <option v-for="ds in datasources" :key="ds.id" :value="ds.id">
                    {{ ds.name }}
                  </option>
                </select></label
              ><label class="wb-label"
                >Schema<input
                  v-model="sourceSchema"
                  placeholder="数据库schema" /></label
              ><label class="wb-label"
                >表名<input v-model="sourceTable" required /></label></template
            ><button class="primary" :disabled="busy">
              {{ wt("创建资产") }}
            </button>
          </form>
        </details>
        <hr class="wb-divider" />
        <h3>{{ wt("执行历史") }}</h3>
        <button @click="run(() => loadHistory())">{{ wt("刷新历史") }}</button>
        <p v-if="historyLoading" class="wb-muted" role="status">正在更新执行历史…</p>
        <p v-else-if="!history.length" class="wb-muted">暂无执行记录。创建的任务会保留在这里，离开页面后也可回来查看。</p>
        <div class="wb-list" aria-label="执行历史" :aria-busy="historyLoading">
          <button
            v-for="item in history"
            :key="item.id"
            :class="{ selected: current?.id === item.id }"
            @click="inspect(item.id)"
          >
            <strong>{{
              executionLabel(item)
            }}</strong
            ><small
              >{{ item.id.slice(0, 12) }} · {{ statusLabel(item.status) }} ·
              {{ new Date(item.created_at).toLocaleString() }}</small
            >
          </button>
        </div>
        <button v-if="next" @click="run(() => loadHistory(true))">
          {{ wt("加载更多") }}
        </button>
      </aside>
      <div class="wb-stack">
        <section v-if="isPipeline && selectedTemplate" class="wb-panel journey-panel">
          <h2>{{ current ? '当前执行流程' : '模板预览' }} · {{ selectedTemplate.name }}</h2>
          <ol class="pipeline-dependencies">
            <li v-for="stage in selectedTemplate.stages" :key="stage.id">
              <span class="journey-number">{{ selectedTemplate.stages.indexOf(stage) + 1 }}</span><strong>{{ stageLabel(stage.agent_id) }}</strong><small>{{ current ? (stages.find(item => item.stage_key === stage.id) ? statusLabel(stages.find(item => item.stage_key === stage.id)!.status) : '待执行') : '模板步骤' }}</small>
            </li>
          </ol>
        </section>
        <p v-if="detailLoading && !current" class="wb-panel wb-muted" role="status">正在读取执行详情…</p>
        <section v-if="current" ref="executionPanel" class="wb-panel" tabindex="-1">
          <div class="wb-section-head" style="margin-top: 0">
            <h2>{{ wt("执行详情") }}</h2>
            <span class="wb-badge">{{ statusLabel(current.status) }}</span>
          </div>
          <p class="wb-footnote">{{ current.id }}</p>
          <LifecycleRunGuide :current="current" :stages="orderedStages" :approvals="approvals" :retryable="isPipeline && current.status === 'failed'" :failure-message="fixedMessage(current.error || orderedStages.find(stage => ['failed', 'quarantined'].includes(stage.status))?.error)" @open="openAsset" />
          <div class="wb-actions">
            <button @click="inspect(current.id)">{{ wt("刷新进度") }}</button
            ><button
              v-if="
                ['queued', 'running', 'awaiting_approval', 'waiting'].includes(
                  current.status,
                )
              "
              class="danger"
              :disabled="busy"
              @click="action('cancel')"
            >
              {{ wt("取消任务") }}</button
            ><button
              v-if="
                isPipeline &&
                current.status === 'failed'
              "
              :disabled="busy"
              @click="action('retry')"
            >
              {{ wt("重试失败任务") }}
            </button>
          </div>
          <div
            v-for="approval in approvals"
            :key="approval.id"
            class="wb-review"
            style="margin-top: 20px"
          >
            <h3>审批 · {{ statusLabel(approval.status) }}</h3>
            <p>{{ fixedMessage(approval.reason) }}</p>
            <p class="wb-muted">
              有效期：{{ approval.expires_at || "未提供" }}
            </p>
            <template
              v-if="['queued', 'running', 'waiting', 'awaiting_approval'].includes(current.status) && ['pending', 'awaiting_approval'].includes(approval.status)"
              ><label class="wb-label"
                >{{ wt("审核说明") }}<input v-model="approvalReason"
              /></label>
              <div class="wb-actions" style="margin-top: 10px">
                <button
                  :disabled="busy"
                  @click="approve(approval.id, 'approved')"
                >
                  {{ wt("批准并继续") }}</button
                ><button
                  :disabled="busy"
                  @click="approve(approval.id, 'rejected')"
                >
                  {{ wt("拒绝") }}
                </button>
              </div></template
            >
            <p v-else-if="['pending', 'awaiting_approval'].includes(approval.status)" class="wb-muted">执行已结束，此审批记录仅供查看，不能继续操作。</p>
          </div>
          <div v-if="stages.length" class="pipeline-stages">
            <article
              v-for="stage in orderedStages"
              :key="stage.id"
              class="pipeline-stage"
            >
              <span class="wb-badge">{{ statusLabel(stage.status) }}</span
              ><strong>{{ stageLabel(stage.agent_id) }} <small>{{ stage.stage_key }}</small></strong
              ><small
                >{{ stage.agent_id }} · 第 {{ stage.attempt }} 次执行</small
              >
              <p v-if="stage.updated_at" class="wb-footnote">
                {{ wt("最后更新") }} ·
                {{ new Date(stage.updated_at).toLocaleString() }}
              </p>
              <p v-if="stage.error" class="wb-error">{{ fixedMessage(stage.error) }}</p>
              <p>{{ fixedMessage(stage.output?.summary) }}</p>
              <p v-if="stage.output?.gate" class="wb-muted">
                门禁：{{ gateLabel(stage.output.gate.status) }} ·
                {{ fixedMessage(stage.output.gate.reason) }}
              </p>
              <div class="wb-actions">
                <button
                  v-for="ref in stage.output?.refs || []"
                  :key="ref.asset_id"
                  :title="ref.kind"
                  @click="openAsset(ref)"
                >
                  查看{{ assetKindLabel(ref.kind) }}产物
                </button>
              </div>
            </article>
          </div>

        </section>
        <section v-else-if="!detailLoading" class="wb-panel wb-empty workspace-empty">
          <h2>准备好一份输入，开始完整处理流程</h2><p>选择已有资产，或从文本、表格、网页及已有来源创建输入。<br />执行后可逐步查看质量、清洗与增强结果，在审批后发布并生成分析报告。</p><button type="button" @click="openInputDraft">创建输入并开始配置</button>
        </section>
        <DecisionCard
          v-if="output"
          :summary="fixedMessage(output.summary)"
          :decision="output.decision"
          :refs="
            output.decision?.evidence_refs ||
            output.gate?.evidence_refs ||
            output.refs ||
            []
          "
          @open="openAsset"
        />
        <section v-if="current?.output?.refs?.length" class="wb-panel">
          <h3>执行产出</h3>
          <div class="wb-actions">
            <button v-for="ref in current.output.refs" :key="`${ref.asset_id}:${ref.version}`" :title="ref.kind" @click="openAsset(ref)">查看产出 · {{ assetKindLabel(ref.kind) }} · v{{ ref.version }}</button>
          </div>
        </section>
        <p v-if="assetDetailLoading" class="wb-muted" role="status">正在读取产出内容…</p>
        <section v-if="assetDetail" ref="assetPanel" class="wb-panel asset-result" tabindex="-1">
          <div class="wb-section-head">
            <h2>{{ assetKindLabel(assetDetail.ref.kind) }} · v{{ assetDetail.ref.version }}</h2>
            <button @click="assetDetail = undefined">收起</button>
          </div>
          <p class="wb-muted">
            来源：{{ assetDetail.ref.producer_id }} ·
            {{ assetDetail.ref.kind }} · {{ statusLabel(assetDetail.ref.status) }}
          </p>
          <AnalysisReport :content="assetDetail.content" />
          <CleaningReview v-if="assetDetail.ref.kind === 'cleaned_asset'" :cleaned-ref="`${assetDetail.ref.asset_id}:${assetDetail.ref.version}`" />
          <details><summary>原始资产内容</summary><pre class="wb-code">{{
            typeof assetDetail.content === "string"
              ? assetDetail.content
              : JSON.stringify(assetDetail.content, null, 2)
          }}</pre></details>
        </section>

      </div>
    </div>
  </div>
</template>
<script setup lang="ts">
import { useCursorOptions } from "@/composables/useCursorOptions";
import { wt } from "@/i18n/workbench";

import { computed, nextTick, onMounted, onBeforeUnmount, reactive, ref, watch } from "vue";
import { onBeforeRouteLeave, useRoute } from "vue-router";
import { useAuthStore } from "@/stores/auth";
import { http } from "@/utils/request";
import {
  lifecycleApi,
  type Agent,
  type Template,
  type AssetRef,
  type RefID,
  type Invocation,
  type Stage,
  type Approval,
  type RunDetail,
  type StageOutput,
  type ExpertOptions,
  type PublicationTarget,
} from "@/api/lifecycle";
import DecisionCard from "@/components/workbench/DecisionCard.vue";
import AnalysisReport from "@/components/workbench/AnalysisReport.vue";
import RecordEditor from "@/components/workbench/RecordEditor.vue";
import CleaningReview from "./CleaningReview.vue";
import ExpertOptionsForm from "./ExpertOptionsForm.vue";
import LifecycleRunGuide from "./LifecycleRunGuide.vue";
import { defaultOptions, validatedOptions } from "./expertOptions";
import "@/components/workbench/workbench.css";
const route = useRoute(),
  isAnalysis = computed(() => route.path === "/analysis"),
  isPipeline = computed(() => route.path.startsWith("/pipelines")),
  resource = computed(() => "runs" as "runs" | "invocations");
const workbenchRoot = ref<HTMLElement>(), createPanel = ref<HTMLDetailsElement>(), inputSelect = ref<HTMLSelectElement>(), executionPanel = ref<HTMLElement>(), assetPanel = ref<HTMLElement>();
const cleaningPreviewDigest = ref("");
const createExpanded = ref(false), initialLoading = ref(true), initialError = ref(false), historyLoading = ref(false), detailLoading = ref(false), assetDetailLoading = ref(false), writing = ref(false);
const detailErrorID = ref('');
const agents = ref<Agent[]>([]),
  templates = ref<Template[]>([]),
  assets = ref<AssetRef[]>([]),
  history = ref<Invocation[]>([]),
  current = ref<Invocation>(),
  stages = ref<Stage[]>([]),
  approvals = ref<Approval[]>([]),
  executor = ref(""),
  assetSelection = ref(""),
  publishedOnly = ref(isAnalysis.value),
  busy = ref(false),
  error = ref(""),
  notice = ref(""),
  next = ref(""),
  approvalReason = ref(""),
  assetDetail = ref<{ ref: AssetRef; content: unknown }>(),
  assetName = ref(""),
  inputKind = ref("text"),
  inputText = ref(""),
  inputColumns = ref(""),
  inputRows = ref<Record<string, unknown>[]>([]);
const columnTypes = reactive<Record<string, 'text' | 'number'>>(Object.create(null));
const sourceRunID = ref(""),
  sourceItemID = ref(""),
  sourceSessionID = ref(""),
  sourceResultID = ref("");
const sourceURL = ref(""),
  knowledgeID = ref(""),
  datasourceID = ref(""),
  sourceSchema = ref(""),
  sourceTable = ref(""),
  datasources = ref<{ id: string; name: string }[]>([]);
const sourcePrefilled = ref(false);
function draftSnapshot() { return JSON.stringify([assetName.value, inputKind.value, inputText.value, inputColumns.value, inputRows.value, columnTypes, sourceURL.value, knowledgeID.value, datasourceID.value, sourceSchema.value, sourceTable.value, sourceRunID.value, sourceItemID.value, sourceSessionID.value, sourceResultID.value]); }
const draftBaseline = ref(draftSnapshot());
const hasUnsavedInput = computed(() => !!(assetName.value || inputText.value || inputColumns.value || sourceURL.value || knowledgeID.value || datasourceID.value || sourceRunID.value || sourceSessionID.value) && draftSnapshot() !== draftBaseline.value);
// Navigation only prepares an editable draft. It must never collect or publish on arrival.
watch(() => route.query, () => {
  const queryString = (key: string) => typeof route.query[key] === "string" ? String(route.query[key]).trim() : "";
  const source = queryString("source");
  const fields: Record<string, string[]> = { collection: ["run_id", "item_id"], knowledge: ["knowledge_id"], evaluation: ["run_id"], result: ["session_id", "result_id"] };
  const required = fields[source];
  if (!Array.isArray(required)) return;
  if (!required.every(key => queryString(key))) {
    if (sourcePrefilled.value) {
      inputKind.value = "text";
      assetName.value = sourceRunID.value = sourceItemID.value = knowledgeID.value = sourceSessionID.value = sourceResultID.value = "";
      sourcePrefilled.value = false;
      draftBaseline.value = draftSnapshot();
    }
    return;
  }
  inputKind.value = source;
  sourceRunID.value = ["collection", "evaluation"].includes(source) ? queryString("run_id") : "";
  sourceItemID.value = source === "collection" ? queryString("item_id") : "";
  knowledgeID.value = source === "knowledge" ? queryString("knowledge_id") : "";
  sourceSessionID.value = source === "result" ? queryString("session_id") : "";
  sourceResultID.value = source === "result" ? queryString("result_id") : "";
  assetName.value = queryString("name") || "导入来源";
  sourcePrefilled.value = true;
  draftBaseline.value = draftSnapshot();
}, { immediate: true });
async function openInputDraft() { createExpanded.value = true; await nextTick(); createPanel.value?.scrollIntoView?.({ block: 'nearest', behavior: 'smooth' }); createPanel.value?.querySelector<HTMLInputElement>('input')?.focus({ preventScroll: true }); }
function stageLabel(agentID: string) { return ({ 'data-access': '数据接入', 'data-quality': '质量检查', 'data-cleaning': '数据清洗', 'data-enrichment': '数据增强', 'data-asset': '审批与发布', 'data-analysis': '分析报告', 'data-evaluation': '结果评估' } as Record<string, string>)[agentID] || agents.value.find(agent => agent.id === agentID)?.name || agentID; }
function assetKindLabel(kind?: string) { return ({ raw_asset: '原始输入', cleaned_asset: '清洗版本', enriched_asset: '增强版本', published_asset: '已发布资产', quality_report: '质量报告', insight: '分析报告', evaluation: '评估报告', result: '计算结果' } as Record<string, string>)[kind || ''] || '资产'; }
function executionLabel(item: Invocation) { return item.agent_id ? stageLabel(item.agent_id) : item.template_id ? templates.value.find(template => template.id === item.template_id)?.name || '流水线任务' : item.id.slice(0, 14); }
const fixedMessages: Record<string, string> = {
  'administrator approval required for publication or model cost': '发布资产或调用模型前，需要管理员审核批准。',
  'Previously acquired input validated': '已校验采集到的输入资产。',
  'quality threshold met': '质量评分已达到阈值。',
  'quality score below threshold': '质量评分低于阈值，请检查输入数据或调整处理参数。',
  'Cleaning produced no usable data': '清洗后没有可用数据。',
  'cleaned data is empty or violates input bounds': '清洗后数据为空或超出允许范围。',
  'Created cleaned version and rechecked quality': '已创建清洗版本，并重新检查质量。',
  'Metadata recommendations generated; publication policy unchanged': '已生成元数据建议，发布策略保持不变。',
  'Approved asset published': '已发布获批的资产。',
  'Descriptive analysis completed with source evidence': '已完成描述统计，并保留来源证据。',
  'Artifact integrity and evidence evaluation completed': '已完成产物完整性与证据检查。',
  'missing source evidence': '缺少来源证据。',
  'stage retry budget exhausted': '此阶段已达到重试次数上限。',
  'upstream stage or quality gate did not permit this transition': '前序阶段或质量门禁未通过，无法继续此阶段。',
  'upstream produced no compatible reference': '前序阶段未生成可用于此阶段的资产。',
  'approval rejected': '审批已被拒绝。',
  'approval expired': '审批已过期。',
};
// Translate only exact, known system messages. Unknown errors and user reasons remain verbatim.
function fixedMessage(value?: string) { return value && typeof fixedMessages[value] === 'string' ? fixedMessages[value] : value || ''; }
function gateLabel(value: string) { return ({ passed: '已通过', failed: '未通过', quarantined: '已隔离' } as Record<string, string>)[value] || value; }
function optionSummary(agentID: string, options?: ExpertOptions) {
  if (agentID === 'data-quality') return `阈值 ${options?.min_score ?? '默认'}`;
  if (agentID === 'data-cleaning') return `${options?.operations?.length || 0} 项操作`;
  if (agentID === 'data-enrichment') return options?.method === 'llm' ? '模型 · 需审批' : '规则与关键词';
  if (agentID === 'data-asset') return selectedInputKind.value === 'table' ? '资产快照' : options?.knowledge_base_id ? '已选择知识库' : '待确认目标';
  if (agentID === 'data-analysis') return options?.prompt?.trim() ? '按目标分析' : '描述统计';
  return '自动执行';
}
const executionTemplate = computed(() => {
  const run = current.value;
  if (!run?.template_id) return undefined;
  // The registry exposes current definitions; a historical run may use an older
  // persisted version whose steps must not be replaced by the latest template.
  return templates.value.find(template => template.id === run.template_id && (run.template_version === undefined || template.version === run.template_version));
});
const selectedTemplate = computed(() => current.value ? executionTemplate.value : templates.value.find(template => template.id === executor.value));
const orderedStages = computed(() => {
  const declaredOrder = executionTemplate.value?.stages.map(stage => stage.id) || ['access', 'quality', 'cleaning', 'enrichment', 'publication', 'analysis', 'evaluation'];
  const ranks = new Map(declaredOrder.map((key, index) => [key, index]));
  return stages.value.map((stage, index) => ({ stage, index })).sort((a, b) => (ranks.get(a.stage.stage_key) ?? Number.MAX_SAFE_INTEGER) - (ranks.get(b.stage.stage_key) ?? Number.MAX_SAFE_INTEGER) || a.index - b.index).map(item => item.stage);
});
const parameterStages = computed(() => isPipeline.value
  ? templates.value.find((t) => t.id === executor.value)?.stages || []
  : executor.value ? [{ id: executor.value, agent_id: executor.value }] : []);
const optionDrafts = reactive<Record<string, ExpertOptions>>({});
// Preview and execution hash the same validated option shape.
const cleaningPreviewOptions = computed(() => {
  const draft = optionDrafts["data-cleaning"] || defaultOptions("data-cleaning");
  try { return validatedOptions("data-cleaning", draft, selectedInputKind.value, publicationTargets.value); }
  catch { return draft; }
});
const needsPublication = computed(() => parameterStages.value.some((stage) => stage.agent_id === "data-asset"));
const selectedInputKind = ref(""), inputLoading = ref(false), inputError = ref("");
const publicationTargets = ref<PublicationTarget[]>([]), targetLoading = ref(false), targetError = ref(""), targetHasMore = ref(false);
let targetPage = 1, targetLoaded = false, targetEpoch = 0, inputEpoch = 0;
let inputPending: Promise<void> | undefined;
watch(() => parameterStages.value.map((stage) => `${stage.id}:${stage.agent_id}`).join("|"), () => {
  for (const key of Object.keys(optionDrafts)) delete optionDrafts[key];
  for (const stage of parameterStages.value) optionDrafts[stage.id] = defaultOptions(stage.agent_id);
}, { immediate: true, flush: "sync" });
async function loadPublicationTargets() {
  if (targetLoading.value || (targetLoaded && !targetHasMore.value)) return;
  const epoch = targetEpoch, page = targetPage;
  targetLoading.value = true;
  targetError.value = "";
  try {
    const result = (await lifecycleApi.knowledgeBases(page)).data;
    if (disposed || epoch !== targetEpoch) return;
    if (!result || !Array.isArray(result.items)) throw new Error("知识库列表格式异常，请重试");
    const byID = new Map(publicationTargets.value.map((target) => [target.id, target]));
    for (const target of result.items) byID.set(target.id, target);
    publicationTargets.value = [...byID.values()];
    targetHasMore.value = page * result.size < result.total;
    if (targetHasMore.value && !result.items.length) throw new Error("知识库分页异常，请刷新页面重试");
    targetPage = page + 1;
    targetLoaded = true;
  } catch (e) {
    if (!disposed && epoch === targetEpoch) targetError.value = e instanceof Error ? e.message : "知识库加载失败";
  } finally {
    if (!disposed && epoch === targetEpoch) targetLoading.value = false;
  }
}
function resolveSelectedInput(): Promise<void> {
  const epoch = ++inputEpoch, selected = assetSelection.value;
  selectedInputKind.value = "";
  inputError.value = "";
  inputLoading.value = false;
  inputPending = undefined;
  if (!selected || !needsPublication.value) return Promise.resolve();
  const asset = assets.value.find((a) => `${a.asset_id}:${a.version}` === selected);
  if (!asset) return Promise.resolve();
  inputLoading.value = true;
  const promise = (async () => {
    try {
      const result = (await lifecycleApi.asset(asset.asset_id, asset.version)).data;
      if (disposed || epoch !== inputEpoch || selected !== assetSelection.value) return;
      const content = result?.content;
      const kind = content && typeof content === "object" && "kind" in content ? content.kind : undefined;
      if (kind !== "text" && kind !== "table") throw new Error("输入资产不是可发布的文本或表格");
      selectedInputKind.value = kind;
    } catch (e) {
      if (!disposed && epoch === inputEpoch) inputError.value = e instanceof Error ? e.message : "输入资产读取失败";
    } finally {
      if (!disposed && epoch === inputEpoch) { inputLoading.value = false; inputPending = undefined; }
    }
  })();
  inputPending = promise;
  return promise;
}
watch(needsPublication, (needed) => { if (needed) void loadPublicationTargets(); });
watch([assetSelection, needsPublication], () => { void resolveSelectedInput(); });
const selectedAgent = computed(() =>
    agents.value.find((a) => a.id === executor.value),
  ),
  output = computed<StageOutput | undefined>(
    () =>
      current.value?.output ||
      [...orderedStages.value].reverse().find((s) => s.output)?.output,
  ),
  rowFields = computed(() =>
    inputColumns.value
      .split(/[,，]/)
      .map((v) => v.trim())
      .filter(Boolean)
      .map((v) => ({ key: v, label: v, type: columnTypes[v] === 'number' ? 'number' as const : 'text' as const })),
  );
const kind = () => (isPipeline.value ? resource.value : "invocations");
const statusLabel = (status: string) =>
  ({
    queued: "已排队",
    running: "执行中",
    waiting: "等待依赖",
    awaiting_approval: "等待审批",
    succeeded: "已完成",
    failed: "失败",
    cancelled: "已取消",
    quarantined: "已隔离",
    pending: "待审批",
    approved: "已批准",
    rejected: "已拒绝",
    expired: "已过期",
    published: "已发布",
  })[status] || status;
const assetOptions = useCursorOptions(assets, (cursor) =>
  lifecycleApi.assets(publishedOnly.value ? "published_asset" : "", cursor),
);

let activeOperations = 0;
async function run(fn: () => Promise<void>, write = false) {
  activeOperations++;
  busy.value = true;
  if (write) writing.value = true;
  error.value = "";
  notice.value = "";
  try {
    await fn();
  } catch (e) {
    error.value = e instanceof Error ? e.message : "操作失败";
  } finally {
    if (write) writing.value = false;
    busy.value = --activeOperations > 0;
  }
}
async function loadAssets() {
  await assetOptions.reset();
}
let historyRequest = 0;
async function loadHistory(append = false) {
  if (disposed) return;
  const request = ++historyRequest;
  historyLoading.value = true;
  try {
  const res = await lifecycleApi.list(kind(), append ? next.value : "");
  if (disposed || request !== historyRequest) return;
  history.value = append
    ? [...history.value, ...(res.data?.items || [])]
    : res.data?.items || [];
  next.value = res.data?.next_cursor || "";
  } finally { if (!disposed && request === historyRequest) historyLoading.value = false; }
}
let detailTarget = "",
  detailRequest = 0,
  disposed = false;
let detailPending: { id: string; promise: Promise<void> } | undefined;
function loadDetail(id: string, select = false): Promise<void> {
  if (select) {
    detailTarget = id;
    detailErrorID.value = '';
    current.value = undefined;
    stages.value = [];
    approvals.value = [];
    approvalReason.value = '';
    assetDetail.value = undefined;
    assetDetailRequest++;
    assetDetailLoading.value = false;
    detailPending = undefined;
  }
  if (disposed || id !== detailTarget) return Promise.resolve();
  if (detailPending?.id === id) return detailPending.promise;
  const request = ++detailRequest;
  detailLoading.value = true;
  const promise = (async () => {
    const res = await lifecycleApi.get(kind(), id);
    if (
      disposed ||
      request !== detailRequest ||
      id !== detailTarget ||
      !res.data
    )
      return;
    detailErrorID.value = '';
    if ("run" in res.data) {
      const d = res.data as RunDetail;
      current.value = d.run;
      stages.value = d.stages || [];
      approvals.value = d.approvals || [];
    } else {
      current.value = res.data.invocation;
      stages.value = [];
      approvals.value = res.data.approvals || [];
    }
  })()
    .catch((error) => {
      if (!disposed && request === detailRequest) { detailErrorID.value = id; throw error; }
    })
    .finally(() => {
      if (request === detailRequest) { detailPending = undefined; detailLoading.value = false; }
    });
  detailPending = { id, promise };
  return promise;
}
async function inspect(id: string) {
  await run(() => loadDetail(id, true));
  if (current.value?.id === id) { await nextTick(); executionPanel.value?.scrollIntoView?.({ block: 'start', behavior: 'smooth' }); executionPanel.value?.focus({ preventScroll: true }); }
}
async function start() {
  if (busy.value) return;
  await run(async () => {
    const asset = assets.value.find(
      (a) => `${a.asset_id}:${a.version}` === assetSelection.value,
    );
    if (!asset) throw new Error("请选择输入资产");
    if (!parameterStages.value.length) throw new Error("请选择可用的专家或流水线模板");
    const selected = assetSelection.value, selectedExecutor = executor.value;
    if (needsPublication.value) {
      if (inputPending) await inputPending;
      if (disposed) return;
      if (selected !== assetSelection.value || selectedExecutor !== executor.value) throw new Error("任务选择已改变，请重新提交");
      if (inputError.value) throw new Error(inputError.value);
      if (!selectedInputKind.value) throw new Error("请先确认输入资产类型");
    }
    const options = Object.fromEntries(parameterStages.value.map((stage) => {
      try { return [stage.id, validatedOptions(stage.agent_id, optionDrafts[stage.id] || defaultOptions(stage.agent_id), selectedInputKind.value, publicationTargets.value)]; }
      catch (error) { const section = [...(workbenchRoot.value?.querySelectorAll<HTMLDetailsElement>('[data-options-stage]') || [])].find(item => item.dataset.optionsStage === stage.id); if (section) { section.open = true; section.scrollIntoView?.({ block: 'nearest' }); } throw error; }
    }));
    if(!isPipeline.value && executor.value==="data-cleaning" && cleaningPreviewDigest.value) Object.assign(options[executor.value],{preview_digest:cleaningPreviewDigest.value});
    const data = {
      ...(isPipeline.value
        ? { template_id: executor.value }
        : { agent_id: executor.value }),
      input_refs: [{ asset_id: asset.asset_id, version: asset.version }],
      options: isPipeline.value ? { stage_options: options } : options[executor.value],
    };
    const res = await lifecycleApi.start(kind(), data);
    if (disposed) return;
    await loadHistory();
    if (res.data) await loadDetail(res.data.id, true);
    await nextTick(); executionPanel.value?.scrollIntoView?.({ block: 'start', behavior: 'smooth' }); executionPanel.value?.focus({ preventScroll: true });
  }, true);
}
async function action(action: "retry" | "cancel") {
  if (busy.value) return;
  if (!current.value) return;
  await run(async () => {
    const id = current.value!.id;
    await lifecycleApi.action(kind(), id, action);
    await loadDetail(id);
    await loadHistory();
  }, true);
}
async function approve(id: string, decision: string) {
  if (busy.value) return;
  await run(async () => {
    await lifecycleApi.approve(id, decision, approvalReason.value);
    if (current.value) await loadDetail(current.value.id);
  }, true);
}
let assetDetailRequest = 0;
async function openAsset(ref: RefID) {
  const request = ++assetDetailRequest;
  assetDetail.value = undefined;
  assetDetailLoading.value = true;
  await run(async () => {
    try {
      const result = await lifecycleApi.asset(ref.asset_id, ref.version);
      if (!disposed && request === assetDetailRequest) {
        assetDetail.value = result.data;
        await nextTick(); assetPanel.value?.scrollIntoView?.({ block: 'start', behavior: 'smooth' }); assetPanel.value?.focus({ preventScroll: true });
      }
    } catch (error) {
      if (!disposed && request === assetDetailRequest) throw error;
    } finally { if (!disposed && request === assetDetailRequest) assetDetailLoading.value = false; }
  });
}
async function createAsset() {
  if (busy.value) return;
  await run(async () => {
    const submittedDraft = draftSnapshot();
    const columns = rowFields.value.map((c) => c.key);
    const input =
      inputKind.value === "text"
        ? { kind: "text", text: inputText.value }
        : {
            kind: "table",
            columns,
            rows: inputKind.value === "table" ? inputRows.value.map((row, index) =>
              columns.map((col) => tableCell(row[col], col, index)),
            ) : [],
          };
    const source =
      inputKind.value === "web"
        ? { kind: "web", url: sourceURL.value }
        : inputKind.value === "knowledge"
          ? { kind: "knowledge", knowledge_id: knowledgeID.value }
          : inputKind.value === "datasource"
            ? {
                kind: "datasource",
                datasource_id: datasourceID.value,
                schema: sourceSchema.value,
                table: sourceTable.value,
              }
            : inputKind.value === "collection"
              ? { kind: "collection", run_id: sourceRunID.value, item_id: sourceItemID.value }
              : inputKind.value === "evaluation"
                ? { kind: "evaluation", run_id: sourceRunID.value }
                : inputKind.value === "result"
                  ? { kind: "result", session_id: sourceSessionID.value, result_id: sourceResultID.value }
                  : undefined;
    const res = await lifecycleApi.createAsset(input, assetName.value, source);
    if (disposed) return;
    publishedOnly.value = false;
    await loadAssets();
    if (res.data) {
      const created = res.data;
      if (
        !assets.value.some(
          (a) =>
            a.asset_id === created.asset_id && a.version === created.version,
        )
      )
        assets.value.unshift(created);
      assetSelection.value = `${created.asset_id}:${created.version}`;
    }
    draftBaseline.value = submittedDraft;
    if (draftSnapshot() === submittedDraft) { createExpanded.value = false; sourcePrefilled.value = false; }
    notice.value = "输入资产已创建并选中。请确认模板与参数，再开始执行。";
    await nextTick(); inputSelect.value?.focus();
  }, true);
}
function tableCell(value: unknown, column: string, index: number): string | number | null {
  const text = String(value ?? "");
  if (columnTypes[column] !== "number") return text;
  if (!text.trim()) return null;
  const number = Number(text);
  if (!/^[+-]?(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?$/.test(text.trim()) || !Number.isFinite(number)) {
    throw new Error(`第 ${index + 1} 行「${column}」必须是有限数值`);
  }
  if (Number.isInteger(number) && !Number.isSafeInteger(number)) {
    throw new Error(`第 ${index + 1} 行「${column}」超出安全整数范围，请使用文本类型保留原值`);
  }
  return number;
}
let timer: ReturnType<typeof setInterval> | undefined;
let initializing = false;
async function initialize() {
  if (initializing) return;
  initializing = true; initialLoading.value = true; initialError.value = false;
  await run(async () => {
    try {
    const [a, t] = await Promise.all([
      lifecycleApi.agents(),
      lifecycleApi.templates(),
    ]);
    agents.value = a.data?.agents || [];
    if (isAnalysis.value) executor.value = "data-analysis";
    templates.value = t.data?.templates || [];
    await Promise.all([loadAssets(), loadHistory()]);
    datasources.value =
      (
        await http.get<{ items: { id: string; name: string }[] }>(
          "/datasources",
          { params: { page_size: 100 } },
        )
      ).data?.items || [];
    if (route.query.asset) {
      const [assetID, assetVersion] = String(route.query.asset).split(":");
      const requestedVersion = Number(assetVersion || route.query.version || 1);
      const ref = assets.value.find(
        (a) => a.asset_id === assetID && a.version === requestedVersion,
      );
      if (ref) await openAsset(ref);
      else await openAsset({ asset_id: assetID, version: requestedVersion });
    }
    } catch (cause) { initialError.value = true; throw cause; }
  });
  initializing = false; initialLoading.value = false;
}
function warnBeforeUnload(event: BeforeUnloadEvent) { if (hasUnsavedInput.value || writing.value) { event.preventDefault(); event.returnValue = ''; } }
onBeforeRouteLeave(() => !(hasUnsavedInput.value || writing.value) || window.confirm(writing.value ? '正在提交操作，离开后请从执行历史确认结果。仍要离开吗？' : '输入资产草稿尚未创建，离开页面将丢失草稿。仍要离开吗？'));
onMounted(() => {
  void initialize();
  window.addEventListener('beforeunload', warnBeforeUnload);
  timer = setInterval(async () => {
    if (
      !busy.value &&
      current.value &&
      ["queued", "running", "waiting", "awaiting_approval"].includes(
        current.value.status,
      )
    ) {
      try {
        await loadDetail(current.value.id);
      } catch (e) {
        error.value = e instanceof Error ? e.message : "进度更新失败";
      }
    }
  }, 4000);
});
let events: EventSource | undefined;
watch(
  () =>
    `${current.value?.id || ""}:${current.value?.status || ""}:${useAuthStore().token}`,
  () => {
    const id = current.value?.id;
    events?.close();
    events = undefined;
    if (
      id &&
      ["queued", "running", "waiting", "awaiting_approval"].includes(
        current.value?.status || "",
      ) &&
      isPipeline.value &&
      typeof EventSource !== "undefined"
    ) {
      events = new EventSource(
        `/api/v1/lifecycle/runs/${encodeURIComponent(id)}/events?token=${encodeURIComponent(useAuthStore().token)}`,
      );
      const refreshProgress = () => {
        if (!busy.value)
          loadDetail(id).catch((e) => {
            error.value = e instanceof Error ? e.message : "事件更新失败";
          });
      };
      events.onmessage = refreshProgress;
      for (const event of [
        "stage.started",
        "stage.succeeded",
        "stage.failed",
        "stage.cancelled",
        "stage.quarantined",
        "approval.requested",
        "approval.approved",
        "approval.rejected",
        "approval.expired",
      ])
        events.addEventListener(event, refreshProgress);
      // EventSource reconnects with Last-Event-ID after transient disconnects.
      // Polling continues as fallback; refresh notifications cannot change selection.
      events.onerror = () => {
        /* keep the browser-managed reconnect alive */
      };
    }
  },
);
onBeforeUnmount(() => {
  disposed = true;
  detailRequest++;
  targetEpoch++;
  inputEpoch++;
  assetDetailRequest++;
  if (timer) clearInterval(timer);
  events?.close();
  window.removeEventListener('beforeunload', warnBeforeUnload);
});
function clearExecution() {
  detailRequest++; detailTarget = ''; detailPending = undefined; detailLoading.value = false;
  assetDetailRequest++; assetDetail.value = undefined; assetDetailLoading.value = false;
  current.value = undefined;
  stages.value = [];
  approvals.value = []; approvalReason.value = '';
  error.value = ''; notice.value = '';
  detailErrorID.value = '';
}
watch(() => route.path, () => {
  clearExecution();
  executor.value = isAnalysis.value ? 'data-analysis' : '';
  publishedOnly.value = isAnalysis.value;
  run(async () => { await Promise.all([loadAssets(), loadHistory()]); });
});
let linkedDetail = '';
watch(() => [route.path, route.query.run_id, route.query.invocation_id, route.query.source], () => {
  const value = route.query.source ? undefined : isPipeline.value ? route.query.run_id : route.query.invocation_id;
  const id = typeof value === 'string' ? value.trim() : '';
  if (id) { linkedDetail = id; void inspect(id); }
  else if (linkedDetail) { linkedDetail = ''; clearExecution(); }
}, { immediate: true });
</script>
<style scoped>
.pipeline-dependencies {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(95px, 1fr));
  gap: 10px;
  list-style: none;
  padding: 0;
  margin: 0;
}
.pipeline-dependencies li {
  border-top: 2px solid var(--border-default);
  padding-top: 10px;
  display: grid;
  gap: 6px;
  font-size: 12px;
}
.pipeline-dependencies small { color: var(--text-muted); }.journey-number { color: var(--primary); font-variant-numeric: tabular-nums; }
.pipeline-stages {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(230px, 1fr));
  gap: 12px;
  margin-top: 20px;
}
.pipeline-stage {
  padding: 16px;
  border: 1px solid var(--border-default);
  border-radius: 6px;
}
.pipeline-stage strong {
  display: block;
  margin: 10px 0 6px;
}
.pipeline-stage small {
  color: var(--text-muted);
}
summary {
  cursor: pointer;
  font-weight: 600;
  font-size: 14px;
}
.lifecycle-workbench .wb-grid { grid-template-columns: minmax(300px, 350px) minmax(0, 1fr); align-items: start; }
.launch-panel { gap: 14px; }.launch-panel h2 { margin-bottom: 6px; }.launch-panel p { margin: 0; }.launch-form { gap: 13px; }
.stage-options { border: 1px solid var(--border-default); border-radius: 6px; min-width: 0; }.stage-options > summary { padding: 12px; font-size: 13px; }.stage-options > summary small { display: block; margin: 6px 0 0 14px; color: var(--text-muted); font-size: 11px; font-weight: 400; }
.stage-options :deep(.expert-options) { border: 0; padding: 4px 12px 14px; }.stage-options :deep(legend) { font-size: 10px; color: var(--text-muted); overflow-wrap: anywhere; margin-bottom: 10px; }
.input-draft { border: 1px dashed var(--primary); padding: 14px; border-radius: 6px; min-width: 0; }.input-draft > summary { color: var(--primary); }.create-input-shortcut { text-align: left; color: var(--primary); }
.start-action { min-height: 42px; }.configuration-note { font-size: 12px; }.workspace-empty { padding: 48px 24px; text-align: left; }.workspace-empty h2 { color: var(--text-primary); }.workspace-empty button { margin-top: 12px; }
.launch-panel .wb-list { max-height: 320px; overflow-y: auto; }
.pipeline-stage > strong > small { font-size: 11px; font-weight: 400; display: block; margin-top: 6px; }.wb-review { padding: 18px; background: var(--bg-elevated); border: 1px solid var(--primary); border-radius: 6px; }
.asset-result { scroll-margin-top: 24px; }.wb-error button { margin-left: 12px; }
button:focus-visible, summary:focus-visible, a:focus-visible { outline: 2px solid var(--primary); outline-offset: 3px; }
@media (max-width: 1050px) { .lifecycle-workbench .wb-grid { grid-template-columns: minmax(270px, 310px) minmax(0, 1fr); }.pipeline-stages { grid-template-columns: 1fr; } }
@media (max-width: 800px) { .lifecycle-workbench .wb-grid { grid-template-columns: 1fr; }.pipeline-dependencies { grid-template-columns: repeat(3, minmax(0, 1fr)); }.workspace-empty { padding: 24px 16px; } }
@media (prefers-reduced-motion: reduce) { * { scroll-behavior: auto; } }
</style>
