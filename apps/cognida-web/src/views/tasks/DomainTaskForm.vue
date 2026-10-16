<template>
  <form
    class="wb-stack domain-form"
    aria-label="领域专家任务"
    @submit.prevent="preview"
  >
    <p class="wb-muted">
      固定方案后预览专家任务，也可使用下方 Supervisor
      规划多步协作。预览和保存均不会启动执行。
    </p>
    <fieldset :disabled="busy || loading">
      <label class="wb-label"
        >领域能力<select v-model="selected" aria-label="领域能力">
          <option value="">选择能力</option>
          <option v-for="cap in options" :key="cap.id" :value="cap.id">
            {{ capabilityLabel(cap.id) }}
          </option>
        </select></label
      >
      <SandboxTaskFields
        v-if="capability?.parameter_kind === 'sandbox'"
        ref="sandboxFields"
        :assets="eligibleAssets"
      />
      <template v-else-if="capability?.parameter_kind === 'collection'">
        <label class="wb-label"
          >公开网页 URL<input
            v-model="url"
            type="url"
            required
            aria-label="采集网页"
            placeholder="https://example.com/page"
        /></label>
        <label class="wb-label"
          >响应体上限（字节）<input
            v-model.number="maxBytes"
            type="number"
            min="1024"
            max="2097152"
            required
            aria-label="采集响应上限"
        /></label>
        <p class="wb-muted">
          仅抓取一个公开网页并保存候选快照。内容须转入流水线，经审批发布后才进入正式知识检索。
        </p>
      </template>
      <template v-else-if="capability?.parameter_kind === 'research'">
        <label class="wb-label"
          >研究问题<textarea
            v-model="researchQuery"
            rows="3"
            maxlength="4000"
            required
            aria-label="研究问题"
          />
        </label>
        <label class="wb-label"
          >知识库 ID<input
            v-model="kbText"
            required
            aria-label="研究知识库"
            placeholder="多个 ID 用逗号分隔"
        /></label>
        <label
          ><input v-model="graphEnabled" type="checkbox" />
          加入授权图谱证据</label
        ><label
          ><input v-model="webEnabled" type="checkbox" />
          使用已配置网络搜索</label
        >
        <p class="wb-muted">
          最多 3 个子问题、1 轮补充检索和 4
          次模型调用。报告区分支持、冲突和证据不足；逻辑调用预算不代表供应商费用。也可在<router-link
            to="/research"
            >研究工作台</router-link
          >选择知识库。
        </p>
      </template>
      <template v-else-if="capability?.parameter_kind === 'warehouse'">
        <label class="wb-label"
          >数仓模型<select v-model="warehouseID" aria-label="专家数仓模型">
            <option value="">选择已保存模型</option>
            <option
              v-for="item in warehouseModels"
              :key="item.id"
              :value="item.id"
            >
              {{ item.definition.name }} · v{{ item.version }}
            </option>
          </select></label
        >
        <label v-if="warehouseModel" class="wb-label"
          >固定模型版本<input
            v-model.number="warehouseVersion"
            type="number"
            min="1"
            :max="warehouseModel.version"
            aria-label="专家数仓版本"
        /></label>
        <label class="wb-label"
          >加工模式<select v-model="warehouseMode" aria-label="专家数仓模式">
            <option value="full">完整候选</option>
            <option value="incremental">增量候选</option>
            <option value="backfill">历史回填</option>
          </select></label
        >
        <label class="wb-label"
          >事实新快照<select
            v-model="warehouseSource"
            aria-label="专家数仓事实快照"
          >
            <option value="">模型固定来源</option>
            <option
              v-for="item in eligibleAssets"
              :key="key(item)"
              :value="key(item)"
            >
              {{ item.name || item.asset_id }} · v{{ item.version }}
            </option>
          </select></label
        >
        <div v-if="warehouseMode === 'backfill'" class="wb-form">
          <label
            >回填起始（含）<input
              v-model="warehouseFrom"
              type="datetime-local"
              required /></label
          ><label
            >回填截止（不含）<input
              v-model="warehouseTo"
              type="datetime-local"
              required
          /></label>
        </div>
        <p class="wb-muted">
          从<router-link to="/warehouse">数仓建模</router-link
          >保存模型。自动核对并显式绑定全部事实/维度快照，生成候选成功后仍须管理员审核切换活动产物。
        </p>
        <button
          v-if="warehouseCursor"
          type="button"
          @click="loadWarehouse(true)"
        >
          加载更多数仓模型
        </button>
      </template>
      <template v-else-if="capability?.parameter_kind === 'feature'">
        <label class="wb-label"
          >特征定义<select v-model="featureID" aria-label="专家特征定义">
            <option value="">选择已有定义</option>
            <option v-for="item in features" :key="item.id" :value="item.id">
              {{ item.name }} · v{{ item.version }}
            </option>
          </select></label
        >
        <label v-if="feature" class="wb-label"
          >固定版本<input
            v-model.number="version"
            type="number"
            min="1"
            :max="feature.version"
            aria-label="专家特征版本"
        /></label>
        <p class="wb-muted">
          先在<router-link to="/features">特征工程</router-link
          >配置表达式或窗口并预览。这里执行指定版本的物化，保存实际值与回执。
        </p>
        <p v-if="featureEmbedding" class="wb-muted">
          此固定特征版本会真实调用已绑定的嵌入模型；调用数按来源批次预留，不保证供应商费用上限。
        </p>
        <button v-if="featureCursor" type="button" @click="loadFeatures(true)">
          加载更多特征
        </button>
      </template>
      <template v-else-if="capability">
        <label class="wb-label"
          >处理资产<select v-model="assetID" aria-label="领域处理资产">
            <option value="">选择固定版本</option>
            <option
              v-for="asset in eligibleAssets"
              :key="key(asset)"
              :value="key(asset)"
            >
              {{ asset.name || asset.asset_id }} ·
              {{ assetLabel(asset.kind) }} · v{{ asset.version }}
            </option>
          </select></label
        >
        <template v-if="capability.parameter_kind === 'evaluation'"
          ><div class="wb-form">
            <label class="wb-label"
              >问题列<input
                v-model="questionColumn"
                required
                aria-label="离线问题列" /></label
            ><label class="wb-label"
              >参考答案列<input
                v-model="referenceColumn"
                required
                aria-label="离线参考答案列" /></label
            ><label class="wb-label"
              >已有生成答案列<input
                v-model="answerColumn"
                required
                aria-label="已有答案列"
            /></label>
          </div>
          <p class="wb-muted">
            冻结最多 100
            行已有答案，用精确匹配和包含匹配评分。不会调用模型生成答案，不等同在线评测。
          </p></template
        >
        <template v-else-if="selected === 'governance.qa'"
          ><div class="wb-form">
            <label class="wb-label"
              >问题列<input
                v-model="questionColumn"
                required
                aria-label="问题列" /></label
            ><label class="wb-label"
              >参考答案列<input
                v-model="referenceColumn"
                required
                aria-label="参考答案列"
            /></label>
          </div>
          <p class="wb-muted">
            从所选表格提取已有问题和答案，生成待审核样本，不虚构参考答案。
          </p></template
        >
        <template v-else
          ><label class="wb-label"
            >标签与规则<textarea
              v-model="labels"
              rows="4"
              maxlength="12000"
              placeholder="每行：标签名称 | 触发词1,触发词2"
              aria-label="标签与规则"
            />
          </label>
          <p class="wb-muted">
            每行一个标签，竖线后为逗号分隔的{{
              selected === "governance.named_entity" ? "实体字典词" : "关键词"
            }}。规则候选保留证据，等待人工审核。
          </p></template
        >
      </template>
    </fieldset>
    <p v-if="loading" class="wb-muted" role="status">正在读取固定定义与来源…</p>
    <p v-if="error" class="wb-error" role="alert">{{ error }}</p>
    <button class="primary" :disabled="busy || !capability || loading">
      预览领域计划
    </button>
  </form>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { governanceApi, type Feature, type Label } from "@/api/governance";
import { lifecycleApi, type AssetRef } from "@/api/lifecycle";
import type {
  Capability,
  DraftInput,
  NodeParameters,
  PlanSpec,
} from "@/api/taskcontrol";
import { assetLabel, capabilityLabel } from "./labels";
import type { WarehouseModel } from "@/api/warehouse";
import SandboxTaskFields from "./SandboxTaskFields.vue";
const sandboxFields = ref<InstanceType<typeof SandboxTaskFields>>();
const props = defineProps<{
  capabilities: Capability[];
  assets: AssetRef[];
  busy: boolean;
}>();
const emit = defineEmits<{ preview: [draft: DraftInput, plan: PlanSpec] }>();
const selected = ref(""),
  assetID = ref(""),
  labels = ref(""),
  featureID = ref(""),
  version = ref(1);
const questionColumn = ref("question"),
  referenceColumn = ref("reference_answer"),
  answerColumn = ref("generated_answer");
const url = ref(""),
  maxBytes = ref(1048576),
  researchQuery = ref(""),
  kbText = ref(""),
  graphEnabled = ref(false),
  webEnabled = ref(false);
const featureEmbedding = ref(false);
const warehouseID = ref(""),
  warehouseVersion = ref(1),
  warehouseMode = ref("full"),
  warehouseSource = ref(""),
  warehouseFrom = ref(""),
  warehouseTo = ref(""),
  warehouseModels = ref<WarehouseModel[]>([]),
  warehouseCursor = ref("");
const warehouseModel = computed(() =>
  warehouseModels.value.find((m) => m.id === warehouseID.value),
);
const error = ref(""),
  loading = ref(false),
  features = ref<Feature[]>([]),
  featureCursor = ref("");
let generation = 0;
const options = computed(() =>
  props.capabilities.filter((cap) => cap.parameter_kind && cap.execution),
);
const capability = computed(() =>
  options.value.find((cap) => cap.id === selected.value),
);
const feature = computed(() =>
  features.value.find((item) => item.id === featureID.value),
);
const key = (asset: AssetRef) => `${asset.asset_id}:${asset.version}`;
const eligibleAssets = computed(() =>
  props.assets.filter(
    (asset) =>
      asset.status === "available" &&
      (capability.value?.input_kinds || []).includes(asset.kind),
  ),
);
async function loadFeatures(more = false) {
  const current = ++generation;
  loading.value = true;
  error.value = "";
  try {
    const result = (
      await governanceApi.list<Feature>(
        "/features",
        more ? featureCursor.value : "",
      )
    ).data;
    if (current === generation && result) {
      features.value = more
        ? [...features.value, ...result.items]
        : result.items;
      featureCursor.value = result.next_cursor || "";
    }
  } catch {
    if (current === generation)
      error.value = "读取特征失败，请重新选择能力重试。";
  } finally {
    if (current === generation) loading.value = false;
  }
}
async function loadWarehouse(more = false) {
  const epoch = ++generation;
  loading.value = true;
  error.value = "";
  try {
    const result = (
      await governanceApi.list<WarehouseModel>(
        "/warehouse/models",
        more ? warehouseCursor.value : "",
      )
    ).data;
    if (epoch !== generation) return;
    warehouseModels.value = more
      ? [...warehouseModels.value, ...(result?.items || [])]
      : result?.items || [];
    warehouseCursor.value = result?.next_cursor || "";
  } catch {
    if (epoch === generation)
      error.value = "读取数仓模型失败，请重新选择能力重试。";
  } finally {
    if (epoch === generation) loading.value = false;
  }
}
watch(warehouseID, () => {
  warehouseVersion.value = warehouseModel.value?.version || 1;
});
watch(selected, () => {
  featureEmbedding.value = false;
  error.value = "";
  assetID.value = "";
  featureID.value = "";
  generation++;
  loading.value = false;
  warehouseID.value = "";
  warehouseModels.value = [];
  warehouseCursor.value = "";
  warehouseMode.value = "full";
  warehouseSource.value = "";
  if (capability.value?.parameter_kind === "feature") void loadFeatures();
  if (capability.value?.parameter_kind === "warehouse") void loadWarehouse();
});
watch(featureID, () => {
  version.value = feature.value?.version || 1;
});
onBeforeUnmount(() => {
  generation++;
});
async function preview() {
  error.value = "";
  const cap = capability.value;
  if (!cap?.execution || props.busy || loading.value) return;
  let asset = eligibleAssets.value.find((item) => key(item) === assetID.value),
    parameters: NodeParameters;
  const current = ++generation;
  let requiredCalls: number | undefined;
  let warehouseAssets: AssetRef[] | undefined;
  if (cap.parameter_kind === "sandbox") {
    loading.value = true;
    try {
      const prepared = await sandboxFields.value?.prepare();
      if (current !== generation || !prepared) return;
      parameters = prepared.parameters;
      warehouseAssets = prepared.assets;
    } catch (e) {
      if (current === generation)
        error.value = e instanceof Error ? e.message : "Sandbox来源验证失败。";
      return;
    } finally {
      if (current === generation) loading.value = false;
    }
  } else if (cap.parameter_kind === "collection") {
    try {
      const parsed = new URL(url.value.trim());
      if (parsed.protocol !== "https:" || parsed.username || parsed.password)
        throw new Error();
    } catch {
      error.value = "请输入不含凭据的 HTTPS 网页地址。";
      return;
    }
    if (
      !Number.isSafeInteger(maxBytes.value) ||
      maxBytes.value < 1024 ||
      maxBytes.value > 2097152
    ) {
      error.value = "响应上限须在 1024–2097152 字节之间。";
      return;
    }
    parameters = {
      collection: { url: url.value.trim(), max_response_bytes: maxBytes.value },
    };
  } else if (cap.parameter_kind === "research") {
    const ids = kbText.value.split(/[,，\s]+/).filter(Boolean);
    if (
      !researchQuery.value.trim() ||
      ids.length < 1 ||
      ids.length > 50 ||
      new Set(ids).size !== ids.length
    ) {
      error.value = "请输入研究问题与 1–50 个不重复知识库 ID。";
      return;
    }
    parameters = {
      research: {
        query: researchQuery.value.trim(),
        kb_ids: ids,
        graph_enabled: graphEnabled.value,
        enable_web_search: webEnabled.value,
      },
    };
  } else if (cap.parameter_kind === "warehouse") {
    if (
      !warehouseModel.value ||
      !Number.isSafeInteger(warehouseVersion.value) ||
      warehouseVersion.value < 1 ||
      warehouseVersion.value > warehouseModel.value.version
    ) {
      error.value = "请选择模型及有效固定版本。";
      return;
    }
    if (warehouseMode.value !== "full" && !warehouseSource.value) {
      error.value = "增量和回填必须选择事实新快照。";
      return;
    }
    const params: NonNullable<NodeParameters["warehouse"]> = {
      model_id: warehouseID.value,
      version: warehouseVersion.value,
      mode: warehouseMode.value,
      ...(warehouseSource.value
        ? { fact_source_ref: warehouseSource.value }
        : {}),
    };
    if (warehouseMode.value === "backfill") {
      const start = new Date(warehouseFrom.value),
        end = new Date(warehouseTo.value);
      if (
        !Number.isFinite(start.getTime()) ||
        !Number.isFinite(end.getTime()) ||
        end <= start
      ) {
        error.value = "请填写有效半开回填范围。";
        return;
      }
      params.event_from = start.toISOString();
      params.event_to = end.toISOString();
    }
    parameters = { warehouse: params };
    loading.value = true;
    try {
      const result = (
        await governanceApi.get<{
          profile: string;
          model_id: string;
          version: number;
          input_refs: { asset_id: string; version: number }[];
          max_calls_per_attempt: number;
          max_attempts: number;
        }>(
          `/warehouse/models/${encodeURIComponent(params.model_id)}/requirements?version=${params.version}`,
        )
      ).data;
      if (current !== generation) return;
      if (
        !result ||
        result.profile !== cap.execution.profile ||
        result.model_id !== params.model_id ||
        result.version !== params.version ||
        result.max_attempts !== 1 ||
        !result.input_refs?.length
      )
        throw new Error();
      const refs = [...result.input_refs];
      if (params.fact_source_ref) {
        const split = params.fact_source_ref.lastIndexOf(":");
        const extra = {
          asset_id: params.fact_source_ref.slice(0, split),
          version: Number(params.fact_source_ref.slice(split + 1)),
        };
        if (
          !refs.some(
            (r) => r.asset_id === extra.asset_id && r.version === extra.version,
          )
        )
          refs.push(extra);
      }
      warehouseAssets = [];
      for (const ref of refs) {
        const read = (await lifecycleApi.asset(ref.asset_id, ref.version)).data
          ?.ref;
        if (current !== generation) return;
        if (
          !read ||
          read.asset_id !== ref.asset_id ||
          read.version !== ref.version ||
          read.status !== "available" ||
          !(cap.input_kinds || []).includes(read.kind)
        )
          throw new Error();
        warehouseAssets.push(read);
      }
      requiredCalls =
        cap.execution.base_calls +
        cap.execution.calls_per_input * warehouseAssets.length;
      if (
        !Number.isSafeInteger(result.max_calls_per_attempt) ||
        result.max_calls_per_attempt < 1 ||
        requiredCalls < result.max_calls_per_attempt
      )
        throw new Error();
    } catch {
      if (current === generation)
        error.value = "固定模型或全部来源读取失败；不会生成不完整输入计划。";
      return;
    } finally {
      if (current === generation) loading.value = false;
    }
  } else if (cap.parameter_kind === "feature") {
    if (
      !feature.value ||
      !Number.isSafeInteger(version.value) ||
      version.value < 1 ||
      version.value > feature.value.version
    ) {
      error.value = "请选择特征及有效版本。";
      return;
    }
    parameters = {
      feature: { feature_id: feature.value.id, version: version.value },
    };
    loading.value = true;
    try {
      const fixed = (
        await governanceApi.get<Feature>(
          `/features/${encodeURIComponent(feature.value.id)}/versions/${version.value}`,
        )
      ).data;
      if (current !== generation) return;
      if (!fixed) throw new Error();
      const requirements = (
        await governanceApi.get<{
          feature_id: string;
          version: number;
          max_calls_per_attempt: number;
          max_attempts: number;
          embedding: boolean;
          requires_zero_cost_reservation: boolean;
        }>(
          `/features/${encodeURIComponent(feature.value.id)}/versions/${version.value}/requirements`,
        )
      ).data;
      if (current !== generation) return;
      if (
        !requirements ||
        requirements.feature_id !== fixed.id ||
        requirements.version !== version.value ||
        requirements.max_attempts !== 1 ||
        !Number.isSafeInteger(requirements.max_calls_per_attempt) ||
        requirements.max_calls_per_attempt < 1 ||
        (requirements.embedding && !requirements.requires_zero_cost_reservation)
      )
        throw new Error();
      requiredCalls = requirements.max_calls_per_attempt;
      featureEmbedding.value = requirements.embedding;
      const split = fixed.source_ref_id.lastIndexOf(":");
      const id = fixed.source_ref_id.slice(0, split),
        sourceVersion = Number(fixed.source_ref_id.slice(split + 1));
      asset = (await lifecycleApi.asset(id, sourceVersion)).data?.ref;
      if (current !== generation) return;
      if (!asset || asset.status !== "available") throw new Error();
    } catch {
      if (current === generation)
        error.value = "固定特征版本或来源读取失败，请刷新后重试。";
      return;
    } finally {
      if (current === generation) loading.value = false;
    }
  } else {
    if (!asset) {
      error.value = "请选择可访问的资产版本。";
      return;
    }
    if (cap.parameter_kind === "evaluation") {
      const columns = [
        questionColumn.value.trim(),
        referenceColumn.value.trim(),
        answerColumn.value.trim(),
      ];
      if (
        columns.some((value) => !value || value.length > 128) ||
        new Set(columns).size !== 3
      ) {
        error.value = "三列必须非空、不同，且每列名称不超过 128 字符。";
        return;
      }
      parameters = {
        evaluation: {
          question_column: columns[0]!,
          reference_answer_column: columns[1]!,
          generated_answer_column: columns[2]!,
        },
      };
    } else if (selected.value === "governance.qa") {
      const question = questionColumn.value.trim(),
        reference = referenceColumn.value.trim();
      if (!question || !reference || question === reference) {
        error.value = "问题列和参考答案列须非空且不同。";
        return;
      }
      parameters = {
        governance: {
          qa_mapping: {
            question_column: question,
            reference_answer_column: reference,
          },
        },
      };
    } else {
      const rows = labels.value
        .split("\n")
        .map((row) => row.trim())
        .filter(Boolean);
      const schema: Label[] = rows.map((row, i) => {
        const [name, terms] = row.split("|");
        const words =
          terms
            ?.split(/[,，]/)
            .map((item) => item.trim())
            .filter(Boolean) || [];
        return {
          id: `label-${i + 1}`,
          name: name!.trim(),
          ...(selected.value === "governance.named_entity"
            ? { terms: words }
            : { keywords: words }),
        };
      });
      if (
        schema.length < 1 ||
        schema.length > 30 ||
        schema.some(
          (item) => !item.name || Array.from(item.name).length > 80,
        ) ||
        new Set(schema.map((item) => item.name)).size !== schema.length
      ) {
        error.value = "请输入 1–30 个不重复标签，每个名称最多 80 字符。";
        return;
      }
      if (
        selected.value === "governance.named_entity" &&
        schema.some((item) => !item.terms?.length)
      ) {
        error.value = "实体预标注每个标签都需要明确字典词。";
        return;
      }
      parameters = { governance: { label_schema: schema } };
    }
  }
  if (
    current !== generation ||
    (!asset &&
      !["collection", "research", "warehouse", "sandbox"].includes(
        cap.parameter_kind || "",
      ))
  )
    return;
  const boundAssets = warehouseAssets || (asset ? [asset] : []);
  const refs = boundAssets.map((item) => ({
    asset_id: item.asset_id,
    version: item.version,
  }));
  const calls =
    requiredCalls ??
    cap.execution.base_calls + cap.execution.calls_per_input * refs.length;
  const timeout =
    parameters.sandbox?.resources.timeout_seconds ??
    Math.min(300, cap.execution.max_timeout_seconds);
  const budget = {
    max_nodes: 1,
    max_concurrency: 1,
    max_attempts: 1,
    max_calls: calls,
    max_cost_micros: 0,
    timeout_seconds: timeout,
  };
  const plan: PlanSpec = {
    schema_version: 1,
    budget,
    nodes: [
      {
        id: "work",
        capability: cap.id,
        capability_version: cap.version,
        depends_on: [],
        inputs: boundAssets.map((item, i) => ({
          asset: refs[i]!,
          kind: item.kind,
        })),
        output_kinds: [...cap.output_kinds],
        tools: [...cap.tools],
        permissions: [...cap.permissions],
        max_attempts: 1,
        max_calls_per_attempt: calls,
        max_cost_micros_per_attempt: 0,
        timeout_seconds: timeout,
        parameters,
      },
    ],
  };
  emit(
    "preview",
    {
      goal: `${capabilityLabel(cap.id)}：${(cap.parameter_kind === "feature" ? feature.value?.name : cap.parameter_kind === "warehouse" ? warehouseModel.value?.definition.name : asset?.name || researchQuery.value.trim() || url.value.trim()) || "固定资产版本"}`,
      input_refs: refs,
      budget,
    },
    plan,
  );
}
</script>
<style scoped>
.domain-form fieldset {
  border: 0;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 16px;
}
.domain-form p {
  font-size: 12px;
  line-height: 1.8;
}
.domain-form label {
  font-size: 13px;
}
</style>
