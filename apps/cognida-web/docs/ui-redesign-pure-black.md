# Cognida UI 纯黑色调设计方案

## 设计理念

**纯黑暗色** - 深邃、克制、聚焦内容

> 主色调纯黑，极简白色点缀，无任何彩色干扰

---

## 一、配色系统

### CSS 变量

```css
:root {
  /* ========== 背景系统 ========== */
  /* 纯黑层级 */
  --bg-deep:      #000000;      /* 纯黑 */
  --bg-surface:   #030303;      /* 近黑 */
  --bg-elevated:  #080808;      /* 微亮黑 */
  --bg-hover:     #0d0d0d;      /* 悬浮黑 */

  /* 遮罩层级 */
  --bg-overlay-95:  rgba(0, 0, 0, 0.95);
  --bg-overlay-85:  rgba(0, 0, 0, 0.85);
  --bg-overlay-70:  rgba(0, 0, 0, 0.70);
  --bg-overlay-50:  rgba(0, 0, 0, 0.50);

  /* 玻璃态 */
  --bg-glass:     rgba(0, 0, 0, 0.70);

  /* ========== 白色层级（唯一强调色）========== */
  --white-100:    #ffffff;
  --white-80:     rgba(255, 255, 255, 0.80);
  --white-60:     rgba(255, 255, 255, 0.60);
  --white-40:     rgba(255, 255, 255, 0.40);
  --white-25:     rgba(255, 255, 255, 0.25);
  --white-15:     rgba(255, 255, 255, 0.15);
  --white-10:     rgba(255, 255, 255, 0.10);
  --white-5:      rgba(255, 255, 255, 0.05);

  /* ========== 文字颜色 ========== */
  --text-primary:   #f5f5f5;     /* 主要文字 - 接近白 */
  --text-secondary: #a0a0a0;     /* 次要文字 - 灰 */
  --text-muted:     #606060;     /* 弱化文字 - 深灰 */
  --text-dim:       #404040;     /* 极弱文字 - 更深灰 */

  /* ========== 边框 ========== */
  --border-subtle:  rgba(255, 255, 255, 0.05);
  --border-default: rgba(255, 255, 255, 0.08);
  --border-medium:  rgba(255, 255, 255, 0.12);
  --border-strong:  rgba(255, 255, 255, 0.20);

  /* ========== 语义色（极低饱和度）========== */
  --color-success:    #7a7a7a;     /* 成功 - 灰 */
  --color-warning:    #888888;     /* 警告 - 灰 */
  --color-error:      #707070;     /* 错误 - 灰 */
  --color-info:       #808080;     /* 信息 - 灰 */

  /* ========== 间距 ========== */
  --space-1:  4px;
  --space-2:  8px;
  --space-3:  12px;
  --space-4:  16px;
  --space-5:  20px;
  --space-6:  24px;
  --space-8:  32px;

  /* ========== 圆角 ========== */
  --radius-sm:  2px;   /* 更小的圆角 */
  --radius-md:  4px;
  --radius-lg:  6px;
  --radius-xl:  8px;

  /* ========== 字体 ========== */
  --font-sans: 'Inter', -apple-system, sans-serif;
  --font-mono: 'JetBrains Mono', 'SF Mono', monospace;

  /* ========== 字号 ========== */
  --text-xs:   0.75rem;
  --text-sm:   0.875rem;
  --text-base: 1rem;
  --text-lg:   1.125rem;
  --text-xl:   1.25rem;
  --text-2xl:  1.5rem;
  --text-3xl:  2rem;

  /* ========== 过渡 ========== */
  --transition-fast:  150ms;
  --transition-base:  250ms;
  --transition-slow:  350ms;

  /* ========== 阴影 ========== */
  --shadow-sm:  0 2px 8px rgba(0, 0, 0, 0.8);
  --shadow-md:  0 4px 16px rgba(0, 0, 0, 0.9);
  --shadow-lg:  0 8px 32px rgba(0, 0, 0, 1);
}
```

---

## 二、配色层次

```
┌─────────────────────────────────────────────────────────┐
│  背景                        │  文字            │  边框  │
├─────────────────────────────────────────────────────────┤
│  #000000              ████  │  #f5f5f5  ██████  │  20%   │
│  #030303              ███▊  │  #a0a0a0  █████  │  12%   │
│  #080808              ███▎  │  #606060  ████   │  8%    │
│  #0d0d0d              ██▎   │  #404040  ███    │  5%    │
└─────────────────────────────────────────────────────────┘
```

**核心原则**：
- 背景：纯黑 #000000 起步
- 文字：纯白 #f5f5f5 到深灰 #404040
- 边框：5% - 20% 白色透明度
- **无任何彩色**

---

## 三、组件设计

### 1. Button 按钮

```
┌─────────────────────────────────────────────────────────┐
│                                                         │
│  Primary:    [ 提交 ]     白底黑字                     │
│                                                         │
│  Secondary:  [ 取消 ]     透明 + 白色细边框             │
│                                                         │
│  Ghost:      < 详情 >     深灰文字                     │
│                                                         │
│  Text:       · 链接 ·     深灰 + 中点                  │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

#### 按钮代码

```css
/* Primary - 纯白底黑字 */
.btn-primary {
  background: #ffffff;
  color: #000000;
  border: none;
  border-radius: 4px;
  padding: 0 20px;
  height: 38px;
}

.btn-primary:hover {
  background: #e8e8e8;
}

/* Secondary - 透明 + 白边 */
.btn-secondary {
  background: transparent;
  color: #f5f5f5;
  border: 1px solid rgba(255, 255, 255, 0.15);
  border-radius: 4px;
  padding: 0 20px;
  height: 38px;
}

.btn-secondary:hover {
  background: rgba(255, 255, 255, 0.05);
  border-color: rgba(255, 255, 255, 0.25);
}

/* Ghost - 深灰文字 */
.btn-ghost {
  background: transparent;
  color: #606060;
  border: none;
}

.btn-ghost:hover {
  color: #a0a0a0;
}
```

---

### 2. Card 卡片

```
┌─────────────────────────────────────────────────────────┐
│ ┃ 标题                                          ○ ○ ○  │
├─────────────────────────────────────────────────────────┤
│                                                         │
│                       内容                              │
│                                                         │
└─────────────────────────────────────────────────────────┘
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
```

#### 卡片代码

```css
.card {
  background: rgba(0, 0, 0, 0.85);
  backdrop-filter: blur(20px);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 6px;
}

.card:hover {
  border-color: rgba(255, 255, 255, 0.12);
}

/* 左侧装饰条 - 极细白线 */
.card::before {
  content: '';
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 2px;
  background: rgba(255, 255, 255, 0.15);
}
```

---

### 3. Input 输入框

```
┌─────────────────────────────────┐
│ > 输入内容...                    │
└─────────────────────────────────┘
```

```css
.input {
  background: rgba(0, 0, 0, 0.50);
  border: 1px solid rgba(255, 255, 255, 0.08);
  color: #f5f5f5;
  border-radius: 4px;
}

.input:focus {
  border-color: rgba(255, 255, 255, 0.20);
  background: rgba(0, 0, 0, 0.70);
}

.input::placeholder {
  color: #404040;
}
```

---

### 4. Tag 标签

```
[ 标签 ]  — 白色细边框，半透明黑底
```

```css
.tag {
  background: rgba(0, 0, 0, 0.50);
  border: 1px solid rgba(255, 255, 255, 0.12);
  color: #a0a0a0;
  border-radius: 4px;
  padding: 4px 10px;
  font-size: 12px;
}
```

---

### 5. Sidebar 侧边栏

```
┌─────────────┐
│  ◉ Cognida    │
├─────────────┤
│             │
│  ● 对话     │ ← 激活：左侧白线
│  ○ 知识库   │
│  ○ 图谱     │
│             │
├─────────────┤
│  ○ 设置     │
└─────────────┘
```

```css
.sidebar {
  background: rgba(0, 0, 0, 0.70);
  border-right: 1px solid rgba(255, 255, 255, 0.08);
}

.menu-item {
  color: #606060;
}

.menu-item.active {
  color: #f5f5f5;
  border-left: 2px solid rgba(255, 255, 255, 0.30);
}

.menu-item:hover {
  color: #a0a0a0;
}
```

---

## 四、页面布局

### 登录页

```
┌────────────────────────────────────────┐
│                                        │
│              ◉ Cognida                   │
│          智能数据工作台                 │
│                                        │
│   ┌──────────────────────────────┐    │
│   │ > email                      │    │
│   └──────────────────────────────┘    │
│   ┌──────────────────────────────┐    │
│   │ > ••••••••                   │    │
│   └──────────────────────────────┘    │
│                                        │
│            [ 登 录 ]                   │
│                                        │
│   还没有账号？立即注册                  │
│                                        │
└────────────────────────────────────────┘
```

---

### 首页卡片

```
┌─────────────────────────────────────────────────────────┐
│ ┃ 对话                                         [ AI ]  │
├─────────────────────────────────────────────────────────┤
│                                                         │
│   ┌─────────────────────────────────────────────┐      │
│   │                                             │      │
│   │   💬                                        │      │
│   │   智能对话系统                               │      │
│   │   多模型支持 · Agent 模式 · 深度研究        │      │
│   │                                             │      │
│   └─────────────────────────────────────────────┘      │
│                                                         │
│                              [ 开始对话 ]               │
└─────────────────────────────────────────────────────────┘
```

---

### 聊天页

```
┌─────────────────────────────────────────────────────────┐
│  ◉ Cognida                    [+ 新对话]              ●   │
├───────────────────┬─────────────────────────────────────┤
│                   │                                     │
│  ● 当前对话       │  ┌─────────────────────────────┐    │
│  ○ 对话 A         │  │ 用户：分析销售数据           │    │
│  ○ 对话 B         │  └─────────────────────────────┘    │
│                   │                                     │
│  ○ 知识库        │  ┌─────────────────────────────┐    │
│  ○ 图谱          │  │                             │    │
│                   │  │ AI: 本周销售额 125.3万      │    │
│  ═══════════     │  │ 同比↑12%                     │    │
│                   │  │                             │    │
│  [+ 新对话]       │  │ 建议：增加 A 渠道投放        │    │
│                   │  │                             │    │
│                   │  └─────────────────────────────┘    │
│                   │                                     │
│                   │  ┌─────────────────────────────┐    │
│                   │  │ > 输入消息...            [→] │    │
│                   │  └─────────────────────────────┘    │
└───────────────────┴─────────────────────────────────────┘
```

---

## 五、视觉效果

### 玻璃态

```css
.glass {
  background: rgba(0, 0, 0, 0.70);
  backdrop-filter: blur(20px);
  border: 1px solid rgba(255, 255, 255, 0.08);
}

.glass-strong {
  background: rgba(0, 0, 0, 0.90);
  backdrop-filter: blur(30px);
  border: 1px solid rgba(255, 255, 255, 0.12);
}
```

### 分割线

```css
.divider {
  height: 1px;
  background: rgba(255, 255, 255, 0.08);
}

.divider-vertical {
  width: 1px;
  background: rgba(255, 255, 255, 0.08);
}
```

### 滚动条

```css
::-webkit-scrollbar {
  width: 6px;
  height: 6px;
}

::-webkit-scrollbar-track {
  background: transparent;
}

::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.10);
  border-radius: 3px;
}

::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.15);
}
```

---

## 六、动画

```css
/* 淡入 */
@keyframes fadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
}

/* 上滑 */
@keyframes slideUp {
  from {
    opacity: 0;
    transform: translateY(12px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

/* 闪烁（光标效果） */
@keyframes blink {
  0%, 50% { opacity: 1; }
  51%, 100% { opacity: 0; }
}
```

---

## 七、颜色速查

| 用途 | 值 |
|------|-----|
| 背景主 | `#000000` |
| 背景次 | `#030303` |
| 按钮白 | `#ffffff` |
| 文字主 | `#f5f5f5` |
| 文字次 | `#a0a0a0` |
| 文字弱 | `#606060` |
| 边框默认 | `rgba(255,255,255,0.08)` |
| 边框强 | `rgba(255,255,255,0.20)` |

---

## 八、设计原则

1. **纯黑基底**：#000000 起步，不用深灰
2. **白色点缀**：仅用白色透明度区分层次
3. **无彩色**：完全去掉彩色强调
4. **极简边框**：5%-20% 透明度
5. **小圆角**：2-6px，不圆润
6. **克制动效**：淡入、上滑，无花哨

---

**确认后开始实施？**
