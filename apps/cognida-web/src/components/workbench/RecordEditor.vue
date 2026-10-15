<template>
  <div class="wb-section-head">
    <h3>
      {{ title }} <span class="wb-muted">{{ modelValue.length }}</span>
    </h3>
    <button type="button" @click="add">＋ 添加{{ title }}</button>
  </div>
  <p v-if="!modelValue.length" class="wb-muted">
    {{ hint || "尚未添加，可按业务需要配置。" }}
  </p>
  <div v-else class="wb-table-wrap">
    <table class="wb-table">
      <thead>
        <tr>
          <th v-for="field in fields" :key="field.key">{{ field.label }}</th>
          <th>操作</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(row, index) in modelValue" :key="String(row.id || index)">
          <td v-for="field in fields" :key="field.key">
            <select
              v-if="field.options"
              :aria-label="`${title}${index + 1} ${field.label}`"
              :value="value(row, field)"
              @change="
                change(index, field, ($event.target as HTMLSelectElement).value)
              "
            >
              <option value="">请选择</option>
              <option
                v-for="option in field.options"
                :key="option.value"
                :value="option.value"
              >
                {{ option.label }}
              </option>
            </select>
            <input
              v-else
              :type="field.type === 'number' ? 'number' : 'text'"
              :step="field.type === 'number' ? 'any' : undefined"
              :aria-label="`${title}${index + 1} ${field.label}`"
              :value="value(row, field)"
              :placeholder="field.placeholder"
              @input="
                change(index, field, ($event.target as HTMLInputElement).value)
              "
            />
          </td>
          <td>
            <button
              type="button"
              class="danger"
              :aria-label="`删除${title}${index + 1}`"
              @click="
                $emit(
                  'update:modelValue',
                  modelValue.filter((_, i) => i !== index),
                )
              "
            >
              删除
            </button>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
<script setup lang="ts">
export interface EditorField {
  key: string;
  label: string;
  placeholder?: string;
  type?: 'text' | 'number';
  list?: boolean;
  options?: { value: string; label: string }[];
}
const props = defineProps<{
  title: string;
  hint?: string;
  modelValue: Record<string, unknown>[];
  fields: EditorField[];
}>();
const emit = defineEmits<{
  "update:modelValue": [Record<string, unknown>[]];
}>();
function add() {
  emit("update:modelValue", [...props.modelValue, { id: crypto.randomUUID() }]);
}
function value(row: Record<string, unknown>, field: EditorField) {
  const val = row[field.key];
  return Array.isArray(val) ? val.join(", ") : String(val ?? "");
}
function change(index: number, field: EditorField, value: string) {
  emit(
    "update:modelValue",
    props.modelValue.map((row, i) =>
      i === index
        ? {
            ...row,
            [field.key]: field.list
              ? value
                  .split(/[,，]/)
                  .map((v) => v.trim())
                  .filter(Boolean)
              : value,
          }
        : row,
    ),
  );
}
</script>
