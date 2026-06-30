# Link UI 设计方案 - 参考即梦风格

## 即梦设计分析

### 核心特点
- **配色**：柔和的奶杏色 + 草绿色系，温柔治愈感
- **风格**：童趣绘本感 + 现代 AI 科技感
- **布局**：简洁卡片式，强视觉层级
- **氛围**：亲和、温暖、易用

---

## 适配 Link 的设计方案

### 一、配色系统

基于即梦的柔和色调，调整为适合数据平台的专业配色：

```css
:root {
  /* ========== 背景系统 ========== */
  /* 温暖的深色调（保持即梦的柔和感，但适配数据平台） */
  --bg-deep:      #1a1a1e;      /* 深灰紫 */
  --bg-surface:   #222228;      /* 表面 */
  --bg-elevated:  #2a2a32;      /* 浮起 */
  --bg-hover:     #32323c;      /* 悬浮 */

  /* 遮罩层级 */
  --bg-overlay-95:  rgba(26, 26, 30, 0.95);
  --bg-overlay-85:  rgba(26, 26, 30, 0.85);
  --bg-overlay-70:  rgba(26, 26, 30, 0.70);

  /* 玻璃态 */
  --bg-glass:     rgba(42, 42, 50, 0.70);

  /* ========== 主色调（参考即梦的柔和色系）========== */
  /* 主色 - 温暖的青绿色（类似即梦的草绿色，但更专业） */
  --primary-50:   #e6f7f5;
  --primary-100:  #b3e6df;
  --primary-200:  #80d5c9;
  --primary-300:  #4dc4b3;
  --primary-400:  #1ab39d;      /* 主青绿 */
  --primary-500:  #00a090;      /* 标准青绿 */
  --primary-600:  #008c7d;

  /* 辅助色 - 柔和的奶杏色系 */
  --accent-cream: #f5e6d3;      /* 奶杏色 */
  --accent-cream-dark: #e5d0b5;
  
  /* 紫色渐变（AI感） */
  --gradient-purple-start: #a78bfa;
  --gradient-purple-end: #818cf8;

  /* ========== 文字颜色 ========== */
  --text-primary:   #f0f0f5;     /* 主要文字 */
  --text-secondary: #b8b8c5;     /* 次要文字 */
  --text-muted:     #787888;     /* 弱化文字 */
  --text-accent:    #1ab39d;     /* 强调文字 */

  /* ========== 边框 ========== */
  --border-subtle:  rgba(255, 255, 255, 0.06);
  --border-default: rgba(255, 255, 255, 0.10);
  --border-medium:  rgba(255, 255, 255, 0.15);
  --border-strong:  rgba(255, 255, 255, 0.25);
  --border-accent:  rgba(26, 179, 157, 0.40);

  /* ========== 语义色 ========== */
  --color-success:    #1ab39d;     /* 青绿色 */
  --color-warning:    #fbbf24;     /* 温暖黄 */
  --color-error:      #f87171;     /* 柔和红 */
  --color-info:       #60a5fa;     /* 柔和蓝 */

  /* ========== 间距 ========== */
  --space-1:  4px;
  --space-2:  8px;
  --space-3:  12px;
  --space-4:  16px;
  --space-5:  20px;
  --space-6:  24px;
  --space-8:  32px;

  /* ========== 圆角（即梦风格：较圆润）========== */
  --radius-sm:  8px;
  --radius-md:  12px;
  --radius-lg:  16px;
  --radius-xl:  20px;
  --radius-2xl: 24px;

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
  --transition-fast:  150ms cubic-bezier(0.4, 0, 0.2, 1);
  --transition-base:  250ms cubic-bezier(0.4, 0, 0.2, 1);
  --transition-slow:  350ms cubic-bezier(0.4, 0, 0.2, 1);

  /* ========== 阴影（柔和）========== */
  --shadow-sm:  0 2px 8px rgba(0, 0, 0, 0.15);
  --shadow-md:  0 4px 16px rgba(0, 0, 0, 0.20);
  --shadow-lg:  0 8px 32px rgba(0, 0, 0, 0.25);
  --shadow-glow: 0 0 20px rgba(26, 179, 157, 0.25);
}
```

---

## 二、配色速查

```
背景层级（温暖深色）：
#1a1a1e  ━━━ 深灰紫主背景
#222228  ━━━━ 表面
#2a2a32  ━━━━━ 浮起

主色调（柔和青绿）：
#1ab39d  ━━━ 主青绿
#00a090  ━━━━ 标准青绿

辅助色：
#f5e6d3  ━━━ 奶杏色点缀
#a78bfa  ━━━ 紫色渐变

文字：
#f0f0f5  ━━━ 主要文字
#b8b8c5  ━━━━ 次要文字
```

---

## 三、组件设计

### 1. Button 按钮

```css
/* Primary - 青绿色渐变 */
.btn-primary {
  background: linear-gradient(135deg, #1ab39d 0%, #00a090 100%);
  color: #ffffff;
  border: none;
  border-radius: 12px;
  padding: 0 24px;
  height: 44px;
  font-weight: 500;
  box-shadow: 0 4px 12px rgba(26, 179, 157, 0.25);
}

.btn-primary:hover {
  background: linear-gradient(135deg, #26c9b3 0%, #00b0a0 100%);
  box-shadow: 0 6px 20px rgba(26, 179, 157, 0.35);
  transform: translateY(-1px);
}

/* Secondary - 奶杏色边框 */
.btn-secondary {
  background: transparent;
  color: #f5e6d3;
  border: 1.5px solid rgba(245, 230, 211, 0.30);
  border-radius: 12px;
  padding: 0 24px;
  height: 44px;
}

.btn-secondary:hover {
  background: rgba(245, 230, 211, 0.10);
  border-color: rgba(245, 230, 211, 0.50);
}

/* Ghost */
.btn-ghost {
  background: transparent;
  color: #b8b8c5;
  border: none;
}

.btn-ghost:hover {
  color: #1ab39d;
}
```

### 2. Card 卡片

```css
.card {
  background: rgba(42, 42, 50, 0.80);
  backdrop-filter: blur(20px);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 16px;
  overflow: hidden;
  transition: all 0.3s ease;
}

.card:hover {
  background: rgba(42, 42, 50, 0.95);
  border-color: rgba(26, 179, 157, 0.20);
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.30);
  transform: translateY(-2px);
}

/* 卡片头部渐变装饰 */
.card-header {
  background: linear-gradient(90deg, 
    rgba(26, 179, 157, 0.1) 0%, 
    transparent 100%
  );
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}
```

### 3. Input 输入框

```css
.input {
  background: rgba(30, 30, 36, 0.80);
  border: 1.5px solid rgba(255, 255, 255, 0.10);
  color: #f0f0f5;
  border-radius: 12px;
  transition: all 0.2s ease;
}

.input:focus {
  border-color: #1ab39d;
  box-shadow: 0 0 0 4px rgba(26, 179, 157, 0.10);
  background: rgba(30, 30, 36, 0.95);
}

.input::placeholder {
  color: #787888;
}
```

### 4. Tag 标签

```css
.tag {
  background: rgba(26, 179, 157, 0.15);
  color: #1ab39d;
  border: 1px solid rgba(26, 179, 157, 0.25);
  border-radius: 8px;
  padding: 4px 12px;
  font-size: 12px;
  font-weight: 500;
}

.tag.secondary {
  background: rgba(245, 230, 211, 0.15);
  color: #f5e6d3;
  border-color: rgba(245, 230, 211, 0.25);
}
```

---

## 四、页面布局

### 登录页

```
┌────────────────────────────────────────┐
│                                        │
│              ◉ Link                   │
│          智能数据工作台                 │
│                                        │
│   ┌──────────────────────────────┐    │
│   │ 邮箱                          │    │
│   └──────────────────────────────┘    │
│   ┌──────────────────────────────┐    │
│   │ 密码                          │    │
│   └──────────────────────────────┘    │
│                                        │
│            [ 登 录 ]                   │
│                                        │
│   还没有账号？立即注册                  │
│                                        │
└────────────────────────────────────────┘
```

### 首页（参考即梦的卡片式布局）

```
┌─────────────────────────────────────────────────────────┐
│  ◉ Link                      发现  短片  活动           │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  ┌─────────────────────────────────────────────────┐   │
│  │  🤖 Agent 模式                                   │   │
│  │  智能体协作，复杂任务自动化处理                   │   │
│  │                                    [ 开启 → ]  │   │
│  └─────────────────────────────────────────────────┘   │
│                                                         │
│  ┌───────────────┐  ┌───────────────┐  ┌─────────────┐│
│  │ 💬 智能对话   │  │ 📚 知识库     │  │ 🕸️ 知识图谱  ││
│  │               │  │               │  │             ││
│  │ 多模型对话    │  │ 文档向量化    │  │ 实体关系抽取 ││
│  │ Agent 模式   │  │ RAG 检索     │  │ 图谱可视化  ││
│  │               │  │               │  │             ││
│  │  [ 开始 ]    │  │  [ 管理 ]     │  │  [ 查看 ]   ││
│  └───────────────┘  └───────────────┘  └─────────────┘│
│                                                         │
│  ┌───────────────┐  ┌───────────────┐  ┌─────────────┐│
│  │ 📊 模型评测   │  │ ⚙️ 系统设置   │  │             ││
│  │               │  │               │  │             ││
│  │ 自动化评测    │  │ API 配置     │  │             ││
│  │ 指标分析      │  │ 模型管理     │  │             ││
│  │               │  │               │  │             ││
│  │  [ 评测 ]    │  │  [ 配置 ]     │  │             ││
│  └───────────────┘  └───────────────┘  └─────────────┘│
│                                                         │
└─────────────────────────────────────────────────────────┘
```

---

## 五、视觉效果

### 渐变

```css
/* 主渐变 - 青绿到深青 */
.gradient-primary {
  background: linear-gradient(135deg, #1ab39d 0%, #00a090 100%);
}

/* AI 渐变 - 紫色系 */
.gradient-ai {
  background: linear-gradient(135deg, #a78bfa 0%, #818cf8 100%);
}

/* 奶杏色渐变 */
.gradient-warm {
  background: linear-gradient(135deg, #f5e6d3 0%, #e5d0b5 100%);
}

/* 背景渐变 */
.gradient-bg {
  background: linear-gradient(180deg,
    #1a1a1e 0%,
    #222228 50%,
    #2a2a32 100%
  );
}
```

### 发光效果

```css
.glow-primary {
  box-shadow:
    0 0 20px rgba(26, 179, 157, 0.25),
    0 0 40px rgba(26, 179, 157, 0.15);
}

.glow-ai {
  box-shadow:
    0 0 20px rgba(167, 139, 250, 0.25),
    0 0 40px rgba(129, 140, 248, 0.15);
}
```

### 动画

```css
/* 柔和淡入 */
@keyframes softFadeIn {
  from {
    opacity: 0;
    transform: translateY(10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

/* 卡片上浮 */
@keyframes cardFloat {
  0%, 100% {
    transform: translateY(0);
  }
  50% {
    transform: translateY(-4px);
  }
}

/* 脉冲 */
@keyframes softPulse {
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.7;
  }
}
```

---

## 六、设计原则

1. **柔和配色**：青绿色 + 奶杏色，温暖专业
2. **圆润造型**：12-16px 圆角，亲和友好
3. **渐变点缀**：紫色渐变增加 AI 感
4. **玻璃态**：柔和的毛玻璃效果
5. **微妙动效**：轻盈的过渡和悬浮

---

## 七、颜色速查

| 用途 | 值 |
|------|-----|
| 主背景 | `#1a1a1e` |
| 主色 | `#1ab39d` |
| 奶杏色 | `#f5e6d3` |
| 紫色渐变起点 | `#a78bfa` |
| 文字主 | `#f0f0f5` |
| 文字次 | `#b8b8c5` |
| 边框 | `rgba(255,255,255,0.10)` |

---

**确认后开始实施代码？**
