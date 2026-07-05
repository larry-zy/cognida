<template>
  <div class="datasource-container">
    <!-- 页头 -->
    <div class="header">
      <h2>数据源管理</h2>
      <UiButton variant="primary" icon @click="openCreateDialog">
        <template #icon>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 5v14M5 12h14" />
          </svg>
        </template>
        新建数据源
      </UiButton>
    </div>

    <!-- 列表 -->
    <div class="table-wrap">
      <UiTable
        :columns="columns"
        :data="datasources"
        hover
        border
        size="md"
      >
        <!-- 状态列 -->
        <template #cell-status="{ row }">
          <UiTag :variant="statusVariant(row.status)" size="sm">
            {{ statusLabel(row.status) }}
          </UiTag>
        </template>

        <!-- host:port 列 -->
        <template #cell-host="{ row }">
          <span class="mono">{{ row.host }}:{{ row.port }}</span>
        </template>

        <!-- 操作列 -->
        <template #cell-actions="{ row }">
          <div class="actions">
            <button class="action-btn" title="编辑" @click="openEditDialog(row)">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" />
                <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" />
              </svg>
              编辑
            </button>
            <button class="action-btn action-btn--test" title="测试连接" :disabled="testingId === row.id" @click="testSaved(row)">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                <path d="M5 12h14M12 5l7 7-7 7" />
              </svg>
              {{ testingId === row.id ? '测试中…' : '测试连接' }}
            </button>
            <button class="action-btn action-btn--danger" title="删除" @click="confirmDelete(row)">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                <polyline points="3 6 5 6 21 6" />
                <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
              </svg>
              删除
            </button>
          </div>
        </template>

        <!-- 空态 -->
        <template #empty>
          <div class="empty-state">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.2">
              <ellipse cx="12" cy="5" rx="7" ry="2.5" />
              <path d="M5 5v14c0 1.4 3.1 2.5 7 2.5s7-1.1 7-2.5V5" />
              <path d="M5 12c0 1.4 3.1 2.5 7 2.5s7-1.1 7-2.5" />
            </svg>
            <p>暂无数据源，<button class="link-btn" @click="openCreateDialog">新建一个</button></p>
          </div>
        </template>
      </UiTable>

      <!-- 加载态覆盖层 -->
      <div v-if="loading" class="loading-overlay">
        <div class="spinner"></div>
      </div>
    </div>

    <!-- 分页 -->
    <div v-if="total > pageSize" class="pagination-row">
      <UiPagination
        v-model:page="page"
        v-model:page-size="pageSize"
        :total="total"
        @change="loadDatasources"
      />
    </div>

    <!-- 创建/编辑对话框 -->
    <UiModal
      v-model="dialogVisible"
      :title="editingId ? '编辑数据源' : '新建数据源'"
      size="md"
      centered
    >
      <form class="ds-form" @submit.prevent="submitForm">
        <div class="form-grid">
          <!-- 名称 -->
          <div class="form-field form-field--full">
            <label class="form-label">
              名称 <span class="required">*</span>
            </label>
            <UiInput
              v-model="form.name"
              placeholder="请输入数据源名称"
              :error="errors.name"
            />
          </div>

          <!-- 类型 -->
          <div class="form-field">
            <label class="form-label">
              类型 <span class="required">*</span>
            </label>
            <UiSelect
              v-model="form.type"
              :options="typeOptions"
              placeholder="请选择类型"
            />
          </div>

          <!-- 数据库名称 -->
          <div class="form-field">
            <label class="form-label">
              数据库名 <span class="required">*</span>
            </label>
            <UiInput
              v-model="form.database_name"
              placeholder="请输入数据库名称"
              :error="errors.database_name"
            />
          </div>

          <!-- Host -->
          <div class="form-field">
            <label class="form-label">
              Host <span class="required">*</span>
            </label>
            <UiInput
              v-model="form.host"
              placeholder="例如 localhost 或 127.0.0.1"
              :error="errors.host"
            />
          </div>

          <!-- Port -->
          <div class="form-field">
            <label class="form-label">
              Port <span class="required">*</span>
            </label>
            <UiInputNumber
              v-model="form.port"
              :min="1"
              :max="65535"
              placeholder="3306"
              controls-position="right"
            />
          </div>

          <!-- 用户名 -->
          <div class="form-field">
            <label class="form-label">
              用户名 <span class="required">*</span>
            </label>
            <UiInput
              v-model="form.username"
              placeholder="请输入用户名"
              :error="errors.username"
            />
          </div>

          <!-- 密码 -->
          <div class="form-field">
            <label class="form-label">
              密码
              <span v-if="!editingId" class="required">*</span>
            </label>
            <UiInput
              v-model="form.password"
              type="password"
              show-password-toggle
              :placeholder="editingId ? '留空表示不修改密码' : '请输入密码'"
              :error="errors.password"
            />
            <span v-if="editingId" class="form-hint">留空将保留原密码</span>
          </div>
        </div>
      </form>

      <template #footer>
        <div class="dialog-footer">
          <!-- 表单内测试连接 -->
          <UiButton
            variant="ghost"
            size="sm"
            :loading="testingForm"
            @click="testFormConnection"
          >
            测试连接
          </UiButton>
          <div class="footer-right">
            <UiButton variant="secondary" @click="closeDialog">取消</UiButton>
            <UiButton variant="primary" :loading="submitting" @click="submitForm">
              {{ editingId ? '保存' : '创建' }}
            </UiButton>
          </div>
        </div>
      </template>
    </UiModal>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { UiButton, UiTable, UiTag, UiModal, UiInput, UiInputNumber, UiSelect, UiPagination } from '@/components'
import { datasourceApi } from '@/api/datasource'
import type { Datasource, DatasourceType } from '@/api/datasource'
import toast from '@/utils/toast'
import { ElMessageBox } from '@/utils/confirm'

// ==================== 列定义 ====================
const columns = [
  { key: 'name',          title: '名称',     width: '160' },
  { key: 'type',          title: '类型',     width: '80'  },
  { key: 'host',          title: 'Host:Port', width: '180' },
  { key: 'database_name', title: '数据库',   width: '130' },
  { key: 'username',      title: '用户名',   width: '110' },
  { key: 'status',        title: '状态',     width: '90', align: 'center' as const },
  { key: 'updated_at',    title: '更新时间', width: '170',
    formatter: (v: string) => v ? new Date(v).toLocaleString('zh-CN') : '-' },
  { key: 'actions',       title: '操作',     width: '220', align: 'right' as const }
]

const typeOptions = [
  { label: 'MySQL', value: 'mysql' }
]

// ==================== 列表状态 ====================
const datasources = ref<Datasource[]>([])
const loading = ref(false)
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)

async function loadDatasources() {
  loading.value = true
  try {
    const res = await datasourceApi.list({ page: page.value, page_size: pageSize.value })
    const data = res.data
    if (data) {
      datasources.value = data.items ?? []
      total.value = data.total ?? 0
    }
  } catch (err) {
    console.error('加载数据源列表失败:', err)
    toast.error('加载数据源列表失败')
  } finally {
    loading.value = false
  }
}

// ==================== 状态显示 ====================
function statusVariant(status: string): 'success' | 'danger' | 'warning' | 'default' {
  if (status === 'active') return 'success'
  if (status === 'error')  return 'danger'
  if (status === 'inactive' || status === 'need_credentials') return 'warning'
  return 'default'
}

function statusLabel(status: string): string {
  const map: Record<string, string> = {
    active:   '正常',
    inactive: '未激活',
    error:    '异常',
    need_credentials: '需重录凭证'
  }
  return map[status] ?? status
}

// ==================== 对话框 ====================
const dialogVisible = ref(false)
const editingId = ref<string | null>(null)
const submitting = ref(false)
const testingForm = ref(false)
const testingId = ref<string | null>(null)

interface FormState {
  name: string
  type: DatasourceType
  host: string
  port: number
  database_name: string
  username: string
  password: string
}

const form = reactive<FormState>({
  name: '',
  type: 'mysql',
  host: '',
  port: 3306,
  database_name: '',
  username: '',
  password: ''
})

const errors = reactive<Partial<Record<keyof FormState, string>>>({})

function resetForm() {
  form.name = ''
  form.type = 'mysql'
  form.host = ''
  form.port = 3306
  form.database_name = ''
  form.username = ''
  form.password = ''
  Object.keys(errors).forEach(k => delete (errors as any)[k])
}

function openCreateDialog() {
  editingId.value = null
  resetForm()
  dialogVisible.value = true
}

function openEditDialog(row: Datasource) {
  editingId.value = row.id
  form.name          = row.name
  form.type          = row.type
  form.host          = row.host
  form.port          = row.port
  form.database_name = row.database_name
  form.username      = row.username
  form.password      = ''        // 永不回填密码
  Object.keys(errors).forEach(k => delete (errors as any)[k])
  dialogVisible.value = true
}

function closeDialog() {
  dialogVisible.value = false
}

function validateForm(): boolean {
  Object.keys(errors).forEach(k => delete (errors as any)[k])
  let ok = true
  if (!form.name.trim())          { errors.name = '请输入名称'; ok = false }
  if (!form.host.trim())          { errors.host = '请输入 Host'; ok = false }
  if (!form.database_name.trim()) { errors.database_name = '请输入数据库名'; ok = false }
  if (!form.username.trim())      { errors.username = '请输入用户名'; ok = false }
  if (!editingId.value && !form.password) { errors.password = '创建时密码不能为空'; ok = false }
  return ok
}

async function submitForm() {
  if (!validateForm()) return
  submitting.value = true
  try {
    if (editingId.value) {
      await datasourceApi.update(editingId.value, {
        name:          form.name,
        type:          form.type,
        host:          form.host,
        port:          form.port,
        database_name: form.database_name,
        username:      form.username,
        password:      form.password   // 空字符串 = 保留旧密码
      })
      toast.success('数据源已更新')
    } else {
      await datasourceApi.create({
        name:          form.name,
        type:          form.type,
        host:          form.host,
        port:          form.port,
        database_name: form.database_name,
        username:      form.username,
        password:      form.password
      })
      toast.success('数据源已创建')
    }
    closeDialog()
    await loadDatasources()
  } catch (err: any) {
    const msg: string = err?.message || ''
    if (msg.includes('数据源名称已存在')) {
      errors.name = '数据源名称已存在'
    } else {
      toast.error(msg || (editingId.value ? '更新失败' : '创建失败'))
    }
  } finally {
    submitting.value = false
  }
}

// ==================== 测试连接 ====================

/** 从表单字段测试（对话框内按钮） */
async function testFormConnection() {
  if (!form.host || !form.database_name || !form.username) {
    toast.warning('请先填写 Host、数据库名和用户名')
    return
  }
  // 编辑模式下允许密码留空（后端用保存的密码），但这里走 full-form test 路径需要密码
  // 如果编辑时密码为空，换用 datasource_id 路径测试已保存连接
  if (editingId.value && !form.password) {
    await testSavedById(editingId.value)
    return
  }
  if (!form.password) {
    toast.warning('请先填写密码')
    return
  }
  testingForm.value = true
  try {
    const res = await datasourceApi.testConnection({
      name:          form.name || 'test',
      type:          form.type,
      host:          form.host,
      port:          form.port,
      database_name: form.database_name,
      username:      form.username,
      password:      form.password
    })
    const result = res.data
    if (result?.ok) {
      toast.success('连接成功')
    } else {
      toast.error(result?.message || '连接失败')
    }
  } catch (err: any) {
    toast.error(err?.message || '测试连接失败')
  } finally {
    testingForm.value = false
  }
}

/** 测试已保存的数据源（列表行按钮） */
async function testSaved(row: Datasource) {
  await testSavedById(row.id)
}

async function testSavedById(id: string) {
  testingId.value = id
  try {
    const res = await datasourceApi.testConnection({ datasource_id: id })
    const result = res.data
    if (result?.ok) {
      toast.success('连接成功')
    } else {
      toast.error(result?.message || '连接失败')
    }
  } catch (err: any) {
    toast.error(err?.message || '测试连接失败')
  } finally {
    testingId.value = null
  }
}

// ==================== 删除 ====================
async function confirmDelete(row: Datasource) {
  try {
    await ElMessageBox.confirm({
      message: `确定要删除数据源「${row.name}」吗？此操作不可撤销。`,
      title: '确认删除',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'danger'
    })
    await datasourceApi.delete(row.id)
    toast.success('删除成功')
    await loadDatasources()
  } catch (err) {
    if (err !== 'cancel') {
      console.error('删除数据源失败:', err)
      toast.error('删除失败')
    }
  }
}

// ==================== 生命周期 ====================
onMounted(() => {
  loadDatasources()
})
</script>

<style scoped>
.datasource-container {
  padding: 24px;
  background: rgba(0, 0, 0, 0.2);
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: var(--radius-lg);
}

.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.header h2 {
  font-size: 20px;
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

/* 表格外层：相对定位以便加载覆盖层 */
.table-wrap {
  position: relative;
}

.loading-overlay {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.3);
  border-radius: var(--radius-lg);
  z-index: 10;
}

.spinner {
  width: 24px;
  height: 24px;
  border: 2px solid var(--color-border-default, rgba(255,255,255,0.15));
  border-top-color: var(--primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

/* 操作列 */
.actions {
  display: flex;
  gap: 6px;
  justify-content: flex-end;
  flex-wrap: wrap;
}

.action-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 9px;
  font-size: 12px;
  font-weight: 500;
  border: 1px solid var(--color-border-default, rgba(255,255,255,0.12));
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-secondary, rgba(255,255,255,0.7));
  cursor: pointer;
  transition: all 0.15s ease;
}

.action-btn svg {
  width: 13px;
  height: 13px;
  flex-shrink: 0;
}

.action-btn:hover {
  background: var(--bg-elevated, rgba(255,255,255,0.08));
  color: var(--text-primary, #fff);
  border-color: var(--border-strong, rgba(255,255,255,0.2));
}

.action-btn--test:hover {
  color: var(--primary);
  border-color: rgba(34,211,238,0.4);
}

.action-btn--danger:hover {
  color: var(--danger, #ef4444);
  border-color: rgba(239,68,68,0.4);
}

.action-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
  pointer-events: none;
}

/* mono 字体 */
.mono {
  font-family: var(--font-mono);
  font-size: 13px;
}

/* 空态 */
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 40px;
  color: var(--text-muted);
}

.empty-state svg {
  width: 48px;
  height: 48px;
  opacity: 0.4;
}

.empty-state p {
  font-size: 14px;
  margin: 0;
}

.link-btn {
  background: transparent;
  border: none;
  color: var(--primary);
  cursor: pointer;
  font-size: 14px;
  padding: 0;
  text-decoration: underline;
}

/* 分页 */
.pagination-row {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}

/* 对话框表单 */
.ds-form {
  padding: 0;
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

.form-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.form-field--full {
  grid-column: 1 / -1;
}

.form-label {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-secondary, rgba(255,255,255,0.7));
}

.required {
  color: var(--danger, #ef4444);
}

.form-hint {
  font-size: 11.5px;
  color: var(--text-muted, rgba(255,255,255,0.4));
  margin-top: 2px;
}

/* 对话框底部 */
.dialog-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
}

.footer-right {
  display: flex;
  gap: 8px;
}
</style>
