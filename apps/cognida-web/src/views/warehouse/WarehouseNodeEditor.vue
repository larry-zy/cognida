<template>
  <div class="wb-stack">
    <article v-for="(node, index) in modelValue" :key="index" class="wb-panel">
      <div class="wb-section-head">
        <h3>步骤 {{ index + 1 }} · {{ kinds[node.kind] }}</h3>
        <button
          type="button"
          :disabled="disabled || modelValue.length <= 1"
          @click="remove(index)"
        >
          移除此步骤
        </button>
      </div>
      <div class="wb-form">
        <label
          >步骤标识<input
            v-model="node.id"
            required
            pattern="[a-z][a-z0-9_]{0,63}"
            :disabled="disabled"
            @input="changed"
        /></label>
        <label
          >操作<select
            :value="node.kind"
            :disabled="disabled"
            @change="kind(node, ($event.target as HTMLSelectElement).value)"
          >
            <option v-for="(text, value) in kinds" :key="value" :value="value">
              {{ text }}
            </option>
          </select></label
        >
        <label
          >数据层<select
            v-model="node.layer"
            :disabled="disabled"
            @change="changed"
          >
            <option value="raw">原始层</option>
            <option value="detail">明细层</option>
            <option value="summary">汇总层</option>
            <option value="application">应用层</option>
          </select></label
        >
        <label v-if="node.kind === 'source'"
          >来源快照<select
            v-model="node.source_ref"
            required
            :disabled="disabled"
            @change="changed"
          >
            <option value="">请选择可选资产</option>
            <option
              v-for="a in assets"
              :key="a.asset_id + ':' + a.version"
              :value="a.asset_id + ':' + a.version"
            >
              {{ a.name || a.asset_id }} · v{{ a.version }}
            </option>
          </select></label
        >
        <template v-else
          ><label
            >输入步骤<input
              :value="node.inputs?.join(', ')"
              required
              :disabled="disabled"
              placeholder="orders 或 orders, customers"
              @input="
                node.inputs = names(($event.target as HTMLInputElement).value);
                changed();
              " /></label
        ></template>
      </div>
      <template v-if="node.kind === 'left_join_many_to_one' && node.join"
        ><p class="wb-footnote">
          仅允许多对一左关联；右侧键必须唯一，重复维度会阻止生成候选。
        </p>
        <div class="wb-form">
          <label
            >事实关联键<input
              :value="node.join.left_keys.join(', ')"
              :disabled="disabled"
              @input="
                node.join.left_keys = names(
                  ($event.target as HTMLInputElement).value,
                );
                changed();
              " /></label
          ><label
            >维度唯一键<input
              :value="node.join.right_keys.join(', ')"
              :disabled="disabled"
              @input="
                node.join.right_keys = names(
                  ($event.target as HTMLInputElement).value,
                );
                changed();
              " /></label
          ><label
            >带入的维度列<input
              :value="node.join.right_columns.join(', ')"
              :disabled="disabled"
              @input="
                node.join.right_columns = names(
                  ($event.target as HTMLInputElement).value,
                );
                changed();
              " /></label
          ><label
            >维度列前缀<input
              v-model="node.join.prefix"
              :disabled="disabled"
              @input="changed"
          /></label></div
      ></template>
      <template v-if="node.kind === 'aggregate'"
        ><label
          >分组键<input
            :value="node.group_by?.join(', ')"
            :disabled="disabled"
            @input="
              node.group_by = names(($event.target as HTMLInputElement).value);
              changed();
            "
        /></label>
        <div v-for="(metric, i) in node.metrics" :key="i" class="wb-form">
          <label
            >度量名称<input
              v-model="metric.name"
              :disabled="disabled"
              @input="changed" /></label
          ><label
            >聚合方式<select
              v-model="metric.aggregate"
              :disabled="disabled"
              @change="
                metric.aggregate === 'count' && (metric.field = '');
                changed();
              "
            >
              <option value="sum">求和</option>
              <option value="mean">均值</option>
              <option value="min">最小值</option>
              <option value="max">最大值</option>
              <option value="count">记录数</option>
            </select></label
          ><label v-if="metric.aggregate !== 'count'"
            >度量来源列<input
              v-model="metric.field"
              :disabled="disabled"
              @input="changed" /></label
          ><label
            >业务口径<input
              v-model="metric.caliber"
              :disabled="disabled"
              @input="changed" /></label
          ><button
            type="button"
            :disabled="disabled"
            @click="
              node.metrics?.splice(i, 1);
              changed();
            "
          >
            移除度量
          </button>
        </div>
        <button
          type="button"
          :disabled="disabled || (node.metrics?.length || 0) >= 16"
          @click="
            node.metrics?.push({
              name: 'metric_' + ((node.metrics?.length || 0) + 1),
              field: '',
              aggregate: 'sum',
              caliber: '',
            });
            changed();
          "
        >
          添加度量
        </button></template
      >
      <template v-if="node.kind === 'projection'"
        ><div v-for="(column, i) in node.columns" :key="i" class="projection">
          <div class="wb-form">
            <label
              >输出列名<input
                v-model="column.name"
                :disabled="disabled"
                @input="changed" /></label
            ><label
              >输出类型<select
                v-model="column.type"
                :disabled="disabled"
                @change="changed"
              >
                <option value="text">文本</option>
                <option value="number">数值</option>
                <option value="boolean">布尔</option>
              </select></label
            ><button
              type="button"
              :disabled="disabled"
              @click="
                node.columns?.splice(i, 1);
                changed();
              "
            >
              移除列
            </button>
          </div>
          <FeatureExpressionEditor
            v-model="column.expression"
            :disabled="disabled"
            @update:model-value="changed"
          />
        </div>
        <button
          type="button"
          :disabled="disabled || (node.columns?.length || 0) >= 32"
          @click="
            node.columns?.push({
              name: 'column_' + ((node.columns?.length || 0) + 1),
              type: 'text',
              expression: { op: 'column', field: '' },
            });
            changed();
          "
        >
          添加输出列
        </button></template
      >
    </article>
    <button
      type="button"
      :disabled="disabled || modelValue.length >= 8"
      @click="add"
    >
      添加转换步骤（最多 8 步）
    </button>
  </div>
</template>
<script setup lang="ts">
import type { AssetRef } from "@/api/lifecycle";
import type { WarehouseNode } from "@/api/warehouse";
import FeatureExpressionEditor from "@/views/governance/FeatureExpressionEditor.vue";
const props = defineProps<{
  modelValue: WarehouseNode[];
  assets: AssetRef[];
  disabled?: boolean;
}>();
const emit = defineEmits<{
  (e: "update:modelValue", value: WarehouseNode[]): void;
}>();
const kinds = {
  source: "固定来源快照",
  projection: "字段与表达式投影",
  left_join_many_to_one: "多对一维度关联",
  aggregate: "分组聚合",
};
const names = (text: string) =>
  text
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
function changed() {
  emit("update:modelValue", [...props.modelValue]);
}
function remove(i: number) {
  emit(
    "update:modelValue",
    props.modelValue.filter((_, n) => n !== i),
  );
}
function add() {
  emit("update:modelValue", [
    ...props.modelValue,
    {
      id: "step_" + (props.modelValue.length + 1),
      kind: "projection",
      layer: "detail",
      inputs: [props.modelValue.at(-1)?.id || "source"],
      columns: [
        { name: "id", type: "text", expression: { op: "column", field: "id" } },
      ],
    },
  ]);
}
function kind(node: WarehouseNode, value: string) {
  const id = node.id,
    layer = node.layer;
  Object.keys(node).forEach(
    (k) => delete (node as unknown as Record<string, unknown>)[k],
  );
  Object.assign(node, { id, layer, kind: value });
  if (value === "source") {
    node.layer = "raw";
    node.source_ref = "";
  } else {
    node.inputs = [];
    if (value === "projection") node.columns = [];
    if (value === "aggregate") {
      node.group_by = [];
      node.metrics = [];
    }
    if (value === "left_join_many_to_one")
      node.join = {
        left_keys: [],
        right_keys: [],
        right_columns: [],
        prefix: "dim_",
      };
  }
  changed();
}
</script>
<style scoped>
.projection {
  border-top: 1px solid var(--border-color);
  padding: 1rem 0;
}
.wb-form {
  margin: 0.8rem 0;
}
</style>
