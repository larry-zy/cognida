# Cognida UI 黑青色设计方案

## 设计理念

**黑青色调** - 深邃黑 + 科技青

> 纯黑基底 + 青蓝色强调，无灰色，科技感十足

---

## 一、配色系统

### CSS 变量

```css
:root {
  /* ========== 背景系统（纯黑）========== */
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
  --bg-glass:     rgba(0, 0, 0, 0.75);

  /* ========== 主色调：青蓝色系 ========== */
  --cyan-400:     #22d3ee;      /* 主青色 */
  --cyan-500:     #06b6d4;      /* 标准青 */
  --cyan-600:     #0891b2;      /* 深青 */

  /* 辅助蓝色 */
  --blue-400:     #60a5fa;      /* 亮蓝 */
  --blue-500:     #3b82f6;      /* 标准蓝 */

  /* 背景发光版本 */
  --cyan-glow:    rgba(34, 211, 238, 0.15);
  --cyan-glow-strong: rgba(34, 211, 238, 0.30);

  /* ========== 白色层级 ========== */
  --white-100:    #ffffff;
  --white-80:     rgba(255, 255, 255, 0.80);
  --white-60:     rgba(255, 255, 255, 0.60);
  --white-40:     rgba(255, 255, 255, 0.40);

  /* ========== 文字颜色（白/青双色）========== */
  --text-primary:   #f0f0f5;     /* 主要文字 - 白 */
  --text-secondary: #a0e0f0;     /* 次要文字 - 淡青 */
  --text-muted:     #60a0b0;     /* 弱化文字 - 深青 */
  --text-accent:    #22d3ee;     /* 强调文字 - 亮青 */

  /* ========== 边框（青色系）========== */
  --border-subtle:  rgba(34, 211, 238, 0.08);
  --border-default: rgba(34, 211, 238, 0.15);
  --border-medium:  rgba(34, 211, 238, 0.25);
  --border-strong:  rgba(34, 211, 238, 0.40);

  /* ========== 语义色（青色变体）========== */
  --color-success:    #10b981;     /* 绿色 */
  --color-warning:    #f59e0b;     /* 橙色 */
  --color-error:      #ef4444;     /* 红色 */
  --color-info:       #22d3ee;     /* 青色 */

  /* ========== 间距 ========== */
  --space-1:  4px;
  --space-2:  8px;
  --space-3:  12px;
  --space-4:  16px;
  --space-5:  20px;
  --space-6:  24px;
  --space-8:  32px;

  /* ========== 圆角 ========== */
  --radius-sm:  4px;
  --radius-md:  8px;
  --radius-lg:  12px;
  --radius-xl:  16px;

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

  /* ========== 阴影（青色发光）========== */
  --shadow-sm:    0 2px 8px rgba(0, 0, 0, 0.8);
  --shadow-md:    0 4px 16px rgba(0, 0, 0, 0.9);
  --shadow-lg:    0 8px 32px rgba(0, 0, 0, 1);
  --shadow-glow:  0 0 20px rgba(34, 211, 238, 0.30);
  --shadow-glow-strong: 0 0 40px rgba(34, 211, 238, 0.50);
}
```

---

## 二、配色层次

```
┌─────────────────────────────────────────────────────────┐
│  背景                    │  文字              │  边框   │
├─────────────────────────────────────────────────────────┤
│  #000000          ████  │  #f0f0f5  ██████   │  青色40%│
│  #030303          ███▊  │  #a0e0f0  ████▒   │  青色25%│
│  #080808          ███▎  │  #60a0b0  ███▒▒   │  青色15%│
│  #0d0d0d          ██▎   │  #22d3ee  ██▒▒▒   │  青色8% │
└─────────────────────────────────────────────────────────┘

▒ = 青色
█ = 黑色
```

---

## 三、组件设计

### 1. Button 按钮

```
┌─────────────────────────────────────────────────────────┐
│                                                         │
│  Primary:    [ 提交 ]     青色渐变                     │
│                                                         │
│  Secondary:  [ 取消 ]     透明 + 青色边框               │
│                                                         │
│  Ghost:      < 详情 >     青色文字                     │
│                                                         │
│  Text:       · 链接 ·     青色 + 中点                  │
│                                                         │
│  Danger:     [ 删除 ]     红色                         │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

#### 按钮代码

```css
/* Primary - 青色渐变 */
.btn-primary {
  background: linear-gradient(135deg, #22d3ee 0%, #0891b2 100%);
  color: #000000;
  border: none;
  border-radius: 8px;
  padding: 0 20px;
  height: 40px;
  font-weight: 500;
}

.btn-primary:hover {
  background: linear-gradient(135deg, #67e8f9 0%, #06b6d4 100%);
  box-shadow: 0 0 20px rgba(34, 211, 238, 0.40);
}

/* Secondary - 青色边框 */
.btn-secondary {
  background: transparent;
  color: #22d3ee;
  border: 1px solid rgba(34, 211, 238, 0.30);
  border-radius: 8px;
  padding: 0 20px;
  height: 40px;
}

.btn-secondary:hover {
  background: rgba(34, 211, 238, 0.10);
  border-color: rgba(34, 211, 238, 0.50);
}

/* Ghost - 青色文字 */
.btn-ghost {
  background: transparent;
  color: #60a0b0;
  border: none;
}

.btn-ghost:hover {
  color: #22d3ee;
}

/* Danger - 红色 */
.btn-danger {
  background: linear-gradient(135deg, #ef4444 0%, #dc2626 100%);
  color: #ffffff;
  border: none;
}
```

---

### 2. Card 卡片

```
┌─────────────────────────────────────────────────────────┐
│ ▓ 标题                                      [元数据]   │
├─────────────────────────────────────────────────────────┤
│                                                         │
│                       内容                              │
│                                                         │
└─────────────────────────────────────────────────────────┘
▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
```

#### 卡片代码

```css
.card {
  background: rgba(0, 0, 0, 0.85);
  backdrop-filter: blur(20px);
  border: 1px solid rgba(34, 211, 238, 0.15);
  border-radius: 12px;
  position: relative;
}

.card:hover {
  border-color: rgba(34, 211, 238, 0.30);
  box-shadow: 0 0 30px rgba(34, 211, 238, 0.15);
}

/* 左侧青色装饰条 */
.card::before {
  content: '';
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 4px;
  background: linear-gradient(180deg,
    #22d3ee 0%,
    #0891b2 100%
  );
}

/* 底部青色纹理 */
.card::after {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 4px;
  background: repeating-linear-gradient(
    90deg,
    rgba(34, 211, 238, 0.3) 0px,
    rgba(34, 211, 238, 0.3) 4px,
    transparent 4px,
    transparent 8px
  );
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
  border: 1px solid rgba(34, 211, 238, 0.15);
  color: #f0f0f5;
  border-radius: 8px;
}

.input:focus {
  border-color: #22d3ee;
  box-shadow: 0 0 0 3px rgba(34, 211, 238, 0.15);
  background: rgba(0, 0, 0, 0.70);
}

.input::placeholder {
  color: #60a0b0;
}

/* 提示符 */
.input-prompt::before {
  content: '> ';
  color: #22d3ee;
}
```

---

### 4. Tag 标签

```
[ SUCCESS ]  青色边框
[ WARNING ]  橙色边框
[ ERROR ]    红色边框
[ INFO ]     青色边框
```

```css
.tag {
  background: rgba(34, 211, 238, 0.10);
  border: 1px solid rgba(34, 211, 238, 0.30);
  color: #22d3ee;
  border-radius: 6px;
  padding: 4px 12px;
  font-size: 12px;
}

.tag.success {
  background: rgba(16, 185, 129, 0.10);
  border-color: rgba(16, 185, 129, 0.30);
  color: #10b981;
}

.tag.error {
  background: rgba(239, 68, 68, 0.10);
  border-color: rgba(239, 68, 68, 0.30);
  color: #ef4444;
}
```

---

### 5. Sidebar 侧边栏

```
┌─────────────┐
│  ◉ Cognida    │
├─────────────┤
│             │
│  ● 对话     │ ← 激活：青色高亮
│  ○ 知识库   │
│  ○ 图谱     │
│             │
├─────────────┤
│  ○ 设置     │
└─────────────┘
```

```css
.sidebar {
  background: rgba(0, 0, 0, 0.75);
  border-right: 1px solid rgba(34, 211, 238, 0.15);
}

.menu-item {
  color: #60a0b0;
}

.menu-item.active {
  color: #22d3ee;
  background: rgba(34, 211, 238, 0.10);
  border-left: 3px solid #22d3ee;
}

.menu-item:hover {
  color: #a0e0f0;
  background: rgba(34, 211, 238, 0.05);
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

### 首页

```
┌─────────────────────────────────────────────────────────┐
│                                                         │
│                    ◉ Cognida                              │
│              智能数据工作台                              │
│                                                         │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐    │
│  │ ▓ 💬 对话   │  │ ▓ 📚 知识库  │  │ ▓ 🕸️ 图谱   │    │
│  │             │  │             │  │             │    │
│  │ 智能对话    │  │ 文档管理    │  │ 知识图谱    │    │
│  │ 多模型支持  │  │ 向量检索    │  │ 实体关系    │    │
│  │             │  │             │  │             │    │
│  │ [ 开始 ]    │  │ [ 管理 ]    │  │ [ 查看 ]    │    │
│  └─────────────┘  └─────────────┘  └─────────────┘    │
│                                                         │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐    │
│  │ ▓ 🤖 Agent  │  │ ▓ 📊 评测   │  │ ▓ ⚙️ 设置   │    │
│  │             │  │             │  │             │    │
│  │ 智能体管理  │  │ 模型评测    │  │ 系统配置    │    │
│  │ 工具编排    │  │ 指标分析    │  │ API 管理    │    │
│  │             │  │             │  │             │    │
│  │ [ 管理 ]    │  │ [ 评测 ]    │  │ [ 配置 ]    │    │
│  └─────────────┘  └─────────────┘  └─────────────┘    │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

---

### 聊天页

```
┌─────────────────────────────────────────────────────────┐
│  ◉ Cognida                   [+ 新对话]              ●     │
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
  background: rgba(0, 0, 0, 0.75);
  backdrop-filter: blur(20px);
  border: 1px solid rgba(34, 211, 238, 0.15);
}
```

### 青色发光

```css
.glow-cyan {
  box-shadow:
    0 0 20px rgba(34, 211, 238, 0.30),
    0 0 40px rgba(34, 211, 238, 0.15);
}

.glow-cyan-strong {
  box-shadow:
    0 0 30px rgba(34, 211, 238, 0.50),
    0 0 60px rgba(34, 211, 238, 0.25);
}
```

### 渐变

```css
/* 主按钮渐变 */
.gradient-cyan {
  background: linear-gradient(135deg, #22d3ee 0%, #0891b2 100%);
}

/* 文字渐变 */
.text-gradient-cyan {
  background: linear-gradient(135deg, #22d3ee, #60a5fa);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
}
```

### 滚动条

```css
::-webkit-scrollbar-thumb {
  background: rgba(34, 211, 238, 0.20);
}

::-webkit-scrollbar-thumb:hover {
  background: rgba(34, 211, 238, 0.40);
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

/* 青色脉冲 */
@keyframes pulseCyan {
  0%, 100% {
    box-shadow: 0 0 10px rgba(34, 211, 238, 0.20);
  }
  50% {
    box-shadow: 0 0 25px rgba(34, 211, 238, 0.40);
  }
}

/* 扫描线 */
@keyframes scanlineCyan {
  0% {
    transform: translateY(-100%);
    opacity: 0;
  }
  50% {
    opacity: 1;
  }
  100% {
    transform: translateY(100%);
    opacity: 0;
  }
}
```

---

## 七、颜色速查

| 用途 | 值 |
|------|-----|
| 背景主 | `#000000` |
| 青色主 | `#22d3ee` |
| 青色深 | `#0891b2` |
| 蓝色辅 | `#60a5fa` |
| 文字主 | `#f0f0f5` |
| 文字次 | `#a0e0f0` |
| 边框默认 | `rgba(34,211,238,0.15)` |
| 边框强 | `rgba(34,211,238,0.40)` |

---

## 八、设计原则

1. **纯黑基底**：#000000 起步
2. **青色强调**：#22d3ee 主色调
3. **无灰色**：用青色透明度代替
4. **发光效果**：青色光晕
5. **科技感**：边框、发光、渐变

---

**确认后开始实施？**
