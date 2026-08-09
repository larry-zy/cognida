# Cognida UI 设计方案

## 背景色：#0f0f12

---

## 一、配色系统

```css
:root {
  /* ========== 背景系统 ========== */
  --bg-deep:      #0f0f12;      /* 主背景 */
  --bg-surface:   #15151a;      /* 表面 */
  --bg-elevated:  #1a1a20;      /* 浮起 */
  --bg-hover:     #1f1f26;      /* 悬浮 */

  /* 遮罩层级 */
  --bg-overlay-95:  rgba(15, 15, 18, 0.95);
  --bg-overlay-85:  rgba(15, 15, 18, 0.85);
  --bg-overlay-70:  rgba(15, 15, 18, 0.70);

  /* 玻璃态 */
  --bg-glass:     rgba(26, 26, 32, 0.70);

  /* ========== 主色调 ========== */
  /* 青绿色系 */
  --primary-400:  #22d3ee;      /* 亮青 */
  --primary-500:  #14b8a6;      /* 标准青绿 */
  --primary-600:  #0d9488;      /* 深青绿 */

  /* 发光版本 */
  --primary-glow:    rgba(34, 211, 238, 0.20);
  --primary-glow-strong: rgba(34, 211, 238, 0.35);

  /* ========== 辅助色 ========== */
  /* 蓝紫色（AI感） */
  --accent-400:      #818cf8;     /* 亮蓝紫 */
  --accent-500:      #6366f1;     /* 标准蓝紫 */

  /* ========== 文字颜色 ========== */
  --text-primary:   #f0f0f5;     /* 主要文字 */
  --text-secondary: #a0a0b0;     /* 次要文字 */
  --text-muted:     #606070;     /* 弱化文字 */
  --text-dim:       #404050;     /* 极弱文字 */

  /* ========== 边框 ========== */
  --border-subtle:  rgba(255, 255, 255, 0.06);
  --border-default: rgba(255, 255, 255, 0.10);
  --border-medium:  rgba(255, 255, 255, 0.15);
  --border-strong:  rgba(255, 255, 255, 0.25);
  --border-primary: rgba(34, 211, 238, 0.30);

  /* ========== 语义色 ========== */
  --color-success:    #22c55e;     /* 绿色 */
  --color-warning:    #f59e0b;     /* 橙色 */
  --color-error:      #ef4444;     /* 红色 */
  --color-info:       #3b82f6;     /* 蓝色 */

  /* ========== 间距 ========== */
  --space-1:  4px;
  --space-2:  8px;
  --space-3:  12px;
  --space-4:  16px;
  --space-5:  20px;
  --space-6:  24px;
  --space-8:  32px;

  /* ========== 圆角 ========== */
  --radius-sm:  6px;
  --radius-md:  10px;
  --radius-lg:  14px;
  --radius-xl:  18px;

  /* ========== 字体 ========== */
  --font-sans: 'Inter', -apple-system, 'PingFang SC', sans-serif;
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
  --shadow-sm:  0 2px 8px rgba(0, 0, 0, 0.4);
  --shadow-md:  0 4px 16px rgba(0, 0, 0, 0.5);
  --shadow-lg:  0 8px 32px rgba(0, 0, 0, 0.6);
  --shadow-glow: 0 0 20px rgba(34, 211, 238, 0.30);
}
```

---

## 二、配色速查

```
背景：#0f0f12
┌─────────────────────────────────────────┐
│  #0f0f12  ████████  主背景              │
│  #15151a  ██████▒  表面                 │
│  #1a1a20  █████▒▒  浮起                 │
│  #1f1f26  ███▒▒▒▒  悬浮                 │
└─────────────────────────────────────────┘

主色：青绿色
┌─────────────────────────────────────────┐
│  #22d3ee  ▒▒▒▒▒▒▒▒  亮青（主强调）     │
│  #14b8a6  ▒▒▒▒▒▒▒   标准青绿           │
│  #0d9488  ▒▒▒▒▒▒    深青绿             │
└─────────────────────────────────────────┘

辅助：蓝紫色
┌─────────────────────────────────────────┐
│  #818cf8  ▒▒▒▒▒▒▒▒  蓝紫（AI感）       │
│  #6366f1  ▒▒▒▒▒▒▒   标准蓝紫           │
└─────────────────────────────────────────┘

文字：
┌─────────────────────────────────────────┐
│  #f0f0f5  ████████  主要文字            │
│  #a0a0b0  ██████▒▒  次要文字            │
│  #606070  █████▒▒▒▒  弱化文字            │
└─────────────────────────────────────────┘
```

---

## 三、组件设计

### Button 按钮

```css
/* Primary - 青色渐变 */
.btn-primary {
  background: linear-gradient(135deg, #22d3ee 0%, #14b8a6 100%);
  color: #0f0f12;
  border: none;
  border-radius: 10px;
  padding: 0 24px;
  height: 42px;
  font-weight: 600;
  box-shadow: 0 4px 12px rgba(34, 211, 238, 0.25);
}

/* Secondary - 青色边框 */
.btn-secondary {
  background: transparent;
  color: #22d3ee;
  border: 1.5px solid rgba(34, 211, 238, 0.30);
  border-radius: 10px;
  padding: 0 24px;
  height: 42px;
}

/* Ghost */
.btn-ghost {
  background: transparent;
  color: #a0a0b0;
  border: none;
}

.btn-ghost:hover {
  color: #22d3ee;
}
```

### Card 卡片

```css
.card {
  background: rgba(26, 26, 32, 0.80);
  backdrop-filter: blur(20px);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 14px;
}

.card:hover {
  border-color: rgba(34, 211, 238, 0.20);
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4);
}
```

### Input 输入框

```css
.input {
  background: rgba(21, 21, 26, 0.80);
  border: 1.5px solid rgba(255, 255, 255, 0.10);
  color: #f0f0f5;
  border-radius: 10px;
}

.input:focus {
  border-color: #22d3ee;
  box-shadow: 0 0 0 3px rgba(34, 211, 238, 0.15);
}
```

---

## 四、视觉效果

### 渐变

```css
/* 主渐变 */
.gradient-primary {
  background: linear-gradient(135deg, #22d3ee 0%, #14b8a6 100%);
}

/* AI 渐变 */
.gradient-ai {
  background: linear-gradient(135deg, #818cf8 0%, #6366f1 100%);
}
```

### 发光

```css
.glow-cyan {
  box-shadow:
    0 0 20px rgba(34, 211, 238, 0.30),
    0 0 40px rgba(34, 211, 238, 0.15);
}
```

---

## 五、颜色速查

| 用途 | 值 |
|------|-----|
| 背景主 | `#0f0f12` |
| 青色主 | `#22d3ee` |
| 青色标准 | `#14b8a6` |
| 蓝紫 | `#818cf8` |
| 文字主 | `#f0f0f5` |
| 文字次 | `#a0a0b0` |
| 边框 | `rgba(255,255,255,0.10)` |

---

**确认后开始实施代码？**
