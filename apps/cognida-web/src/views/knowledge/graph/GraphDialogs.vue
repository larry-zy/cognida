<!-- 图谱增删改弹窗集合 —— 拆分自 GraphView.vue〔M14〕
     职责：添加节点、添加关系、编辑节点、编辑关系四个 Modal。
     - 表单对象以 props 传入并就地双向绑定（同一引用，容器可读回填/提交值）；
     - 各弹窗可见性通过具名 v-model 与容器双向绑定；
     - 确定按钮以 emits 触发容器的提交处理（校验/请求/关闭仍由容器负责）。 -->
<template>
  <!-- ===== 添加节点 Modal ===== -->
  <UiModal v-model="addNodeVisible" title="添加节点" size="sm">
    <div class="gv-form-group">
      <UiInput v-model="addNodeForm.name" label="节点名称" placeholder="请输入节点名称" required />
    </div>
    <div class="gv-form-group">
      <UiSelect v-model="addNodeForm.entity_type" label="实体类型" :options="entityTypeOptions" placeholder="选择类型" />
    </div>
    <div class="gv-form-group">
      <UiInput v-model="addNodeForm.attributesStr" label="属性" type="text" placeholder="多个属性用逗号分隔" />
    </div>
    <template #footer>
      <UiButton variant="secondary" @click="addNodeVisible = false">取消</UiButton>
      <UiButton variant="primary" @click="$emit('confirm-add-node')" :loading="adding">确定</UiButton>
    </template>
  </UiModal>

  <!-- ===== 添加关系 Modal ===== -->
  <UiModal v-model="addRelationVisible" title="添加关系" size="sm">
    <div class="gv-form-group">
      <UiSelect v-model="addRelationForm.source" label="源节点" :options="nodeOptions" placeholder="选择源节点" filterable required />
    </div>
    <div class="gv-form-group">
      <UiSelect v-model="addRelationForm.target" label="目标节点" :options="nodeOptions" placeholder="选择目标节点" filterable required />
    </div>
    <div class="gv-form-group">
      <UiSelect v-model="addRelationForm.type" label="关系类型" :options="relationTypeOptions" placeholder="选择关系类型" required />
    </div>
    <div class="gv-form-group">
      <UiSlider v-model="addRelationForm.strength" label="强度" :min="1" :max="10" :marks="strengthMarks" :format-value="(v: number) => `当前值: ${v}`" />
    </div>
    <div class="gv-form-group">
      <UiSlider v-model="addRelationForm.weight" label="权重" :min="0" :max="10" :marks="strengthMarks" :format-value="(v: number) => `当前值: ${v}`" />
    </div>
    <template #footer>
      <UiButton variant="secondary" @click="addRelationVisible = false">取消</UiButton>
      <UiButton variant="primary" @click="$emit('confirm-add-relation')" :loading="adding">确定</UiButton>
    </template>
  </UiModal>

  <!-- ===== 编辑节点 Modal ===== -->
  <UiModal v-model="editNodeVisible" title="编辑节点" size="sm">
    <div class="gv-form-group">
      <UiInput v-model="editNodeForm.name" label="节点名称" placeholder="请输入节点名称" required />
    </div>
    <div class="gv-form-group">
      <UiSelect v-model="editNodeForm.entity_type" label="实体类型" :options="entityTypeOptions" placeholder="选择类型" />
    </div>
    <div class="gv-form-group">
      <UiInput v-model="editNodeForm.attributesStr" label="属性" type="text" placeholder="多个属性用逗号分隔" />
    </div>
    <template #footer>
      <UiButton variant="secondary" @click="editNodeVisible = false">取消</UiButton>
      <UiButton variant="primary" @click="$emit('confirm-edit-node')" :loading="editing">确定</UiButton>
    </template>
  </UiModal>

  <!-- ===== 编辑关系 Modal ===== -->
  <UiModal v-model="editRelationVisible" title="编辑关系" size="sm">
    <div class="gv-form-group">
      <UiSelect v-model="editRelationForm.type" label="关系类型" :options="relationTypeOptions" placeholder="选择关系类型" required />
    </div>
    <div class="gv-form-group">
      <UiSlider v-model="editRelationForm.strength" label="强度" :min="1" :max="10" :marks="strengthMarks" :format-value="(v: number) => `当前值: ${v}`" />
    </div>
    <template #footer>
      <UiButton variant="secondary" @click="editRelationVisible = false">取消</UiButton>
      <UiButton variant="primary" @click="$emit('confirm-edit-relation')" :loading="editing">确定</UiButton>
    </template>
  </UiModal>
</template>

<script setup lang="ts">
import UiButton from '@/components/ui/UiButton.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import UiSlider from '@/components/ui/UiSlider.vue'
import UiModal from '@/components/ui/UiModal.vue'
import type { RelationTypeOption } from '@/types'
import type {
  AddNodeForm,
  AddRelationForm,
  EditNodeForm,
  EditRelationForm,
  LabeledOption,
  SliderMark
} from './types'

// 只读选项与状态
defineProps<{
  entityTypeOptions: LabeledOption[]
  relationTypeOptions: RelationTypeOption[]
  nodeOptions: LabeledOption[]
  strengthMarks: SliderMark[]
  adding: boolean
  editing: boolean
}>()

defineEmits<{
  (e: 'confirm-add-node'): void
  (e: 'confirm-add-relation'): void
  (e: 'confirm-edit-node'): void
  (e: 'confirm-edit-relation'): void
}>()

// 表单对象（具名 v-model 双向绑定；表单内字段就地编辑，与容器共享同一引用）
const addNodeForm = defineModel<AddNodeForm>('addNodeForm', { required: true })
const addRelationForm = defineModel<AddRelationForm>('addRelationForm', { required: true })
const editNodeForm = defineModel<EditNodeForm>('editNodeForm', { required: true })
const editRelationForm = defineModel<EditRelationForm>('editRelationForm', { required: true })

// 四个弹窗可见性（具名 v-model）
const addNodeVisible = defineModel<boolean>('addNodeVisible', { default: false })
const addRelationVisible = defineModel<boolean>('addRelationVisible', { default: false })
const editNodeVisible = defineModel<boolean>('editNodeVisible', { default: false })
const editRelationVisible = defineModel<boolean>('editRelationVisible', { default: false })
</script>

<style scoped>
/* ===== 表单 ===== */
.gv-form-group {
  margin-bottom: var(--space-4);
}

.gv-form-group:last-child {
  margin-bottom: 0;
}
</style>
