# 组件库使用文档

一套基于玻璃态设计风格的 Vue 3 组件库，适配蓝紫色科幻背景。

## 全局安装

```ts
// main.ts
import { createApp } from 'vue'
import App from './App.vue'
import BaseComponents from '@/component'

const app = createApp(App)
app.use(BaseComponents)
```

## 组件列表

### 基础组件

#### BaseButton - 按钮

```vue
<BaseButton variant="primary" size="md" @click="handleClick">
  点击我
</BaseButton>

<!-- 变体 -->
<BaseButton variant="primary">主按钮</BaseButton>
<BaseButton variant="secondary">次要按钮</BaseButton>
<BaseButton variant="ghost">幽灵按钮</BaseButton>
<BaseButton variant="danger">危险按钮</BaseButton>
<BaseButton variant="gradient">渐变按钮</BaseButton>
<BaseButton variant="glass">玻璃按钮</BaseButton>

<!-- 尺寸 -->
<BaseButton size="xs">超小</BaseButton>
<BaseButton size="sm">小</BaseButton>
<BaseButton size="md">中</BaseButton>
<BaseButton size="lg">大</BaseButton>
<BaseButton size="xl">超大</BaseButton>

<!-- 带图标 -->
<BaseButton :icon="StarIcon" variant="primary">
  星标
</BaseButton>

<!-- 加载状态 -->
<BaseButton loading>提交中...</BaseButton>

<!-- 发光效果 -->
<BaseButton glow>发光按钮</BaseButton>
```

#### BaseCard - 卡片

```vue
<BaseCard
  title="卡片标题"
  subtitle="副标题"
  :icon="Icon"
  variant="glass"
  hoverable
>
  卡片内容
  <template #footer>
    <BaseButton>操作</BaseButton>
  </template>
</BaseCard>

<!-- 变体 -->
<BaseCard variant="default">默认卡片</BaseCard>
<BaseCard variant="glass">玻璃卡片</BaseCard>
<BaseCard variant="elevated">浮起卡片</BaseCard>
<BaseCard variant="gradient">渐变卡片</BaseCard>

<!-- 尺寸 -->
<BaseCard size="sm">小卡片</BaseCard>
<BaseCard size="md">中卡片</BaseCard>
<BaseCard size="lg">大卡片</BaseCard>

<!-- 带光效 -->
<BaseCard glow>发光卡片</BaseCard>
```

#### BaseInput - 输入框

```vue
<BaseInput
  v-model="value"
  placeholder="请输入内容"
  size="md"
  variant="default"
  clearable
/>

<!-- 变体 -->
<BaseInput variant="default" />
<BaseInput variant="filled" />
<BaseInput variant="outlined" />
<BaseInput variant="ghost" />

<!-- 前后缀图标 -->
<BaseInput
  :icon-prefix="SearchIcon"
  :icon-suffix="CloseIcon"
/>

<!-- 密码输入 -->
<BaseInput
  type="password"
  placeholder="请输入密码"
/>

<!-- 文本域 -->
<BaseInput
  textarea
  :rows="4"
  placeholder="多行文本"
/>

<!-- 字符计数 -->
<BaseInput
  maxlength="100"
  show-count
/>

<!-- 状态 -->
<BaseInput error />
<BaseInput success />
<BaseInput loading />
```

#### BaseModal - 对话框

```vue
<BaseModal
  v-model="visible"
  title="对话框标题"
  subtitle="副标题"
  :icon="Icon"
  size="md"
  @confirm="handleConfirm"
>
  对话框内容
</BaseModal>

<!-- 尺寸 -->
<BaseModal size="sm">小对话框</BaseModal>
<BaseModal size="md">中对话框</BaseModal>
<BaseModal size="lg">大对话框</BaseModal>
<BaseModal size="xl">超大对话框</BaseModal>
<BaseModal size="full">全屏对话框</BaseModal>

<!-- 无默认底部 -->
<BaseModal hide-footer>
  自定义内容
</BaseModal>

<!-- 自定义底部 -->
<template #footer>
  <BaseButton @click="visible = false">关闭</BaseButton>
</template>
```

#### BaseTag - 标签

```vue
<BaseTag variant="primary">主色标签</BaseTag>
<BaseTag variant="success">成功</BaseTag>
<BaseTag variant="warning">警告</BaseTag>
<BaseTag variant="danger">危险</BaseTag>
<BaseTag variant="info">信息</BaseTag>
<BaseTag variant="gradient">渐变</BaseTag>

<!-- 形状 -->
<BaseTag shape="square">方形</BaseTag>
<BaseTag shape="rounded">圆角</BaseTag>
<BaseTag shape="pill">胶囊</BaseTag>

<!-- 可关闭 -->
<BaseTag closable @close="handleClose">
  可关闭标签
</BaseTag>

<!-- 带图标 -->
<BaseTag :icon="StarIcon">星标</BaseTag>

<!-- 发光 -->
<BaseTag glow>发光标签</BaseTag>
```

### 数据组件

#### BaseTable - 表格

```vue
<BaseTable
  :data="tableData"
  :columns="columns"
  selectable
  :pagination="true"
  :page-size="10"
  @row-click="handleRowClick"
>
  <template #cell-name="{ row }">
    <strong>{{ row.name }}</strong>
  </template>

  <template #action="{ row }">
    <BaseButton size="sm" variant="ghost">编辑</BaseButton>
  </template>
</BaseTable>

<script setup lang="ts">
const tableData = ref([
  { id: 1, name: '张三', email: 'zhang@example.com' },
  { id: 2, name: '李四', email: 'li@example.com' },
])

const columns = [
  { key: 'name', title: '姓名', sortable: true },
  { key: 'email', title: '邮箱', align: 'left' },
]
</script>
```

#### BaseLoader - 加载器

```vue
<!-- 圆形加载器 -->
<BaseLoader type="spinner" size="md" />
<BaseLoader type="circular" size="lg" />

<!-- 点状加载器 -->
<BaseLoader type="dots" />

<!-- 进度条 -->
<BaseLoader type="bar" :progress="75" />

<!-- 波浪 -->
<BaseLoader type="wave" />

<!-- 文字加载 -->
<BaseLoader type="text" />

<!-- 带文字 -->
<BaseLoader text="加载中..." />
```

#### EmptyState - 空状态

```vue
<EmptyState
  title="暂无数据"
  description="这里还没有任何内容"
  action-text="创建"
  @action="handleCreate"
/>

<!-- 自定义图标 -->
<EmptyState :icon="CustomIcon">
  自定义空状态
</EmptyState>

<!-- 使用图片 -->
<EmptyState image="/empty.png">
  带图片的空状态
</EmptyState>
```

### 布局组件

#### AppBackground - 背景层

```vue
<AppBackground
  variant="dark"
  :noise="true"
  :particles="false"
  :glow="true"
/>

<!-- 变体 -->
<AppBackground variant="dark" />    <!-- 默认暗色 -->
<AppBackground variant="darker" />  <!-- 更暗 -->
<AppBackground variant="light" />   <!-- 较亮 -->
<AppBackground variant="gradient" /> <!-- 渐变 -->
<AppBackground variant="glass" />   <!-- 玻璃感 -->

<!-- 自定义图片 -->
<AppBackground image="/custom-bg.png" />
```

## 工具类

```html
<!-- 玻璃态效果 -->
<div class="glass">玻璃容器</div>
<div class="glass-dark">深色玻璃</div>

<!-- 文字渐变 -->
<span class="text-gradient">渐变文字</span>

<!-- 动画 -->
<div class="animate-fade-in">淡入</div>
<div class="animate-slide-up">上滑进入</div>
<div class="animate-pulse">脉冲</div>
<div class="animate-glow">发光</div>
```

## CSS 变量

组件使用以下 CSS 变量，可以通过修改这些变量来自定义样式：

```css
:root {
  /* 主色调 */
  --color-primary: #6366f1;
  --color-primary-light: #818cf8;
  --color-primary-dark: #4f46e5;

  /* 背景色 */
  --color-bg-elevated: rgba(255, 255, 255, 0.08);
  --color-bg-secondary: rgba(30, 41, 59, 0.75);

  /* 文字色 */
  --color-text-primary: #f1f5f9;
  --color-text-secondary: #cbd5e1;

  /* 间距 */
  --spacing-sm: 8px;
  --spacing-md: 12px;
  --spacing-lg: 16px;
  --spacing-xl: 24px;

  /* 圆角 */
  --radius-md: 10px;
  --radius-lg: 16px;
  --radius-xl: 24px;
}
```

## 目录结构

```
component/
├── styles/
│   └── global.css       # 全局样式
├── BaseButton.vue        # 按钮
├── BaseCard.vue          # 卡片
├── BaseInput.vue         # 输入框
├── BaseModal.vue         # 对话框
├── BaseTag.vue           # 标签
├── BaseTable.vue         # 表格
├── BaseLoader.vue        # 加载器
├── AppBackground.vue     # 背景层
├── EmptyState.vue        # 空状态
├── types.ts              # 类型定义
├── index.ts              # 导出文件
└── README.md             # 使用文档
```
