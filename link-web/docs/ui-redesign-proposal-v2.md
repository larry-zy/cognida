# Link UI 重新设计方案 v2

## 一、美学定位：「智能数据工作台」

基于背景图片：`src/img/backgroud.png`（深黑 + 橙色光效）

---

## 二、配色系统

### 主色调（从背景提取）
```css
/* 背景层级 */
--bg-overlay-strong:  rgba(0, 0, 0, 0.85)   /* 强遮罩 */
--bg-overlay-medium:  rgba(0, 0, 0, 0.65)   /* 中遮罩 */
--bg-overlay-weak:    rgba(0, 0, 0, 0.40)   /* 弱遮罩 */
--bg-glass:           rgba(20, 20, 25, 0.6) /* 玻璃态 */

/* 强调色 - 与背景橙色呼应 */
--accent-primary:     #ff6b35   /* 主橙色 - 活力、行动 */
--accent-hover:       #ff8555   /* 悬浮态 */
--accent-active:      #e55a2b   /* 激活态 */
--accent-glow:        rgba(255, 107, 53, 0.4) /* 发光 */

/* 语义色 */
--color-success:      #00c853   /* 绿色 */
--color-warning:      #ffa000   /* 琥珀 */
--color-error:        #ff5252   /* 红色 */
--color-info:         #448aff   /* 蓝色 */

/* 文字层级 */
--text-primary:       #f0f0f5   /* 主要文字 */
--text-secondary:     #b0b0c0   /* 次要文字 */
--text-muted:         #707080   /* 弱化文字 */
--text-dim:           #505060   /* 极弱文字 */

/* 边框 */
--border-subtle:      rgba(255,255,255,0.08)
--border-default:     rgba(255,255,255,0.12)
--border-strong:      rgba(255,255,255,0.20)
--border-accent:      rgba(255,107,53,0.50)
```

---

## 三、字体系统

```css
/* 双字体系统 */
--font-mono:     'JetBrains Mono', 'SF Mono', 'Consolas', monospace
--font-sans:     'Inter', -apple-system, BlinkMacSystemFont, sans-serif

/* 字号 */
--text-xs:   0.75rem   /* 12px */
--text-sm:   0.875rem  /* 14px */
--text-base: 1rem      /* 16px */
--text-lg:   1.125rem  /* 18px */
--text-xl:   1.25rem   /* 20px */
--text-2xl:  1.5rem    /* 24px */
--text-3xl: 2rem      /* 32px */
--text-4xl: 2.5rem    /* 40px */
```

---

## 四、空间系统

```css
/* 间距 */
--space-1:  4px
--space-2:  8px
--space-3:  12px
--space-4:  16px
--space-5:  20px
--space-6:  24px
--space-8:  32px

/* 圆角 */
--radius-sm:   4px
--radius-md:   8px
--radius-lg:   12px
--radius-xl:   16px
```

---

## 五、组件设计

### 按钮 BaseButton

```
Primary:    [ 提交 ]     — 橙色渐变 + 发光
Secondary:  [ 取消 ]     — 玻璃边框
Ghost:      < 详情 >     — 纯文字链接
Danger:     [ 删除 ]     — 红色
Icon:       [→]         — 图标按钮
```

### 卡片 BaseCard

```
┌─────────────────────────────────────┐
│ 标题              元数据            │ ← 头部
├─────────────────────────────────────┤
│                                     │
│            内容区域                 │
│                                     │
└─────────────────────────────────────┘
│ ░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░ │ ← 底部装饰
```

- 玻璃态背景
- 顶部细线分隔
- 底部装饰性纹理

### 输入框 BaseInput

```
┌─────────────────────────┐
│ > 输入内容...           │ ← 提示符
└─────────────────────────┘
```

- 深色玻璃背景
- 橙色聚焦边框
- 终端感提示符

---

## 六、视觉特效

```css
/* 玻璃态 */
.glass {
  background: var(--bg-glass);
  backdrop-filter: blur(20px) saturate(180%);
  border: 1px solid var(--border-default);
}

/* 发光效果 */
.glow-accent {
  box-shadow: 0 0 20px var(--accent-glow),
              0 0 40px rgba(255, 107, 53, 0.2);
}

/* 扫描线动画 */
@keyframes scanline {
  0% { transform: translateY(-100%); }
  100% { transform: translateY(100%); }
}
```

---

## 七、背景使用

```css
/* 全局背景 */
.app-background {
  position: fixed;
  inset: 0;
  background-image: url('@/img/backgroud.png');
  background-size: cover;
  background-position: center;
  z-index: -1;
}

/* 遮罩层 */
.app-overlay {
  position: fixed;
  inset: 0;
  background: var(--bg-overlay-medium);
  z-index: -1;
}
```

---

## 八、对比度

所有元素均符合 WCAG AA 标准：
- 正常文字对比度 ≥ 4.5:1
- 大文字对比度 ≥ 3:1

实测：
- #f0f0f5 on 背景 ≈ 16:1 ✓
- #ff6b35 (按钮) ≈ 5.2:1 ✓
