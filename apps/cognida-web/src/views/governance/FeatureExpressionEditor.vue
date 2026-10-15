<template>
  <fieldset class="expression-editor">
    <legend>{{ label }}</legend>
    <label class="wb-label"
      >运算<select
        :value="numericConstant ? 'literal' : modelValue.op"
        @change="
          setOp(($event.target as HTMLSelectElement).value as ExpressionOp)
        "
      >
        <option v-for="(name, op) in operations" :key="op" :value="op">
          {{ name }}
        </option>
      </select></label
    >
    <label v-if="modelValue.op === 'column'" class="wb-label"
      >字段名<input
        :value="modelValue.field"
        required
        @input="patch({ field: ($event.target as HTMLInputElement).value })"
    /></label>
    <template v-if="modelValue.op === 'literal' || numericConstant">
      <label class="wb-label"
        >常量类型<select
          :value="numericConstant ? 'number' : 'string'"
          @change="setLiteralType(($event.target as HTMLSelectElement).value)"
        >
          <option value="number">精确数值</option>
          <option value="string">文本</option>
        </select></label
      >
      <label class="wb-label"
        >常量值<input
          type="text"
          :inputmode="numericConstant ? 'decimal' : 'text'"
          :value="literalValue"
          required
          @input="setLiteral(($event.target as HTMLInputElement).value)"
      /></label>
      <p v-if="numericConstant" class="wb-footnote">
        数值按原始十进制文本传递，支持高精度小数与大整数，不经过浏览器浮点转换。
      </p>
      <p v-if="literalError" class="wb-error" role="alert">
        {{ literalError }}
      </p>
    </template>
    <FeatureExpressionEditor
      v-for="(arg, i) in numericConstant ? [] : modelValue.args || []"
      :key="i"
      :model-value="arg"
      :label="`${label} · 参数 ${i + 1}`"
      @update:model-value="setArg(i, $event)"
    />
    <template v-if="modelValue.op === 'category_map'">
      <RecordEditor
        :model-value="mappingRows"
        title="类别映射"
        :fields="[
          { key: 'key', label: '原值' },
          { key: 'value', label: '目标类别' },
        ]"
        @update:model-value="setMapping"
      />
      <label class="wb-label"
        >未匹配时的类别<input
          :value="modelValue.default"
          @input="
            patch({ default: ($event.target as HTMLInputElement).value })
          "
      /></label>
    </template>
    <p v-if="modelValue.op === 'round'" class="wb-footnote">
      参数 2 为 0–18 的整数精度。
    </p>
    <p v-if="modelValue.op === 'clamp'" class="wb-footnote">
      依次输入值、下界、上界。
    </p>
  </fieldset>
</template>
<script setup lang="ts">
import { computed, ref } from "vue";
import RecordEditor from "@/components/workbench/RecordEditor.vue";
import type { FeatureExpression, ExpressionOp } from "@/api/governance";
const props = withDefaults(
  defineProps<{ modelValue: FeatureExpression; label?: string }>(),
  { label: "表达式" },
);
const emit = defineEmits<{ "update:modelValue": [FeatureExpression] }>();
const operations: Record<ExpressionOp, string> = {
  column: "读取字段",
  literal: "常量",
  add: "加法",
  sub: "减法",
  mul: "乘法",
  div: "除法",
  abs: "绝对值",
  round: "四舍五入",
  clamp: "限制区间",
  lower: "转小写",
  upper: "转大写",
  trim: "去除两端空格",
  concat: "拼接文本",
  category_map: "类别映射",
  to_number: "转换为数值",
};
const literalError = ref("");
const patch = (value: Partial<FeatureExpression>) =>
  emit("update:modelValue", { ...props.modelValue, ...value });
function setOp(op: ExpressionOp) {
  literalError.value = "";
  if (op === "column") emit("update:modelValue", { op, field: "" });
  else if (op === "literal") emit("update:modelValue", { op, value: 0 });
  else {
    const count =
      op === "clamp"
        ? 3
        : ["add", "sub", "mul", "div", "round", "concat"].includes(op)
          ? 2
          : 1;
    emit("update:modelValue", {
      op,
      args: Array.from({ length: count }, (_, i) =>
        i === 0
          ? { op: "column", field: "" }
          : { op: "literal", value: op === "round" ? 2 : 1 },
      ),
      ...(op === "category_map" ? { mapping: {}, default: "" } : {}),
    });
  }
}
const numericConstant = computed(
  () =>
    (props.modelValue.op === "literal" &&
      (typeof props.modelValue.value === "number" ||
        props.modelValue.value === null)) ||
    (props.modelValue.op === "to_number" &&
      props.modelValue.args?.length === 1 &&
      props.modelValue.args[0].op === "literal"),
);
const literalValue = computed(() =>
  props.modelValue.op === "to_number"
    ? props.modelValue.args?.[0].value
    : props.modelValue.value,
);
function setLiteralType(type: string) {
  literalError.value = "";
  emit(
    "update:modelValue",
    type === "number"
      ? { op: "to_number", args: [{ op: "literal", value: "0" }] }
      : { op: "literal", value: "" },
  );
}
function setLiteral(value: string) {
  literalError.value = "";
  if (!numericConstant.value) {
    patch({ value });
    return;
  }
  if (
    value.length > 256 ||
    !/^-?(0|[1-9]\d*)(\.\d+)?([eE][+-]?\d+)?$/.test(value)
  )
    literalError.value = "请输入有效的十进制数值。";
  // Always replace the AST, including invalid drafts, so an old constant cannot be submitted silently.
  emit("update:modelValue", {
    op: "to_number",
    args: [{ op: "literal", value }],
  });
}
function setArg(index: number, value: FeatureExpression) {
  const args = [...(props.modelValue.args || [])];
  args[index] = value;
  patch({ args });
}
const mappingRows = computed(() =>
  Object.entries(props.modelValue.mapping || {}).map(([key, value]) => ({
    key,
    value,
  })),
);
function setMapping(rows: Record<string, unknown>[]) {
  patch({
    mapping: Object.fromEntries(
      rows.map((row) => [String(row.key || ""), String(row.value || "")]),
    ),
  });
}
</script>
<style scoped>
.expression-editor {
  min-width: 0;
  margin: 12px 0;
  padding: 14px;
  border: 1px solid var(--border-default);
  border-radius: 8px;
}
.expression-editor legend {
  color: var(--text-muted);
  font-size: 12px;
}
.expression-editor .expression-editor {
  margin-left: 8px;
}
</style>
