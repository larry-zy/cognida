<template>
  <div class="home-view">

    <!-- ============ 1. Hero ============ -->
    <section class="hero">
      <h1 class="hero__title">让每个组织都能构建<br>自主进化的数据大脑</h1>
      <p class="hero__desc">
        从「数据助手」到「业务伙伴」的跃迁——<br>
        AI 主动思考、深度分析、输出行动建议
      </p>
      <div class="hero__tags">
        <span class="tag">Go 1.21+</span>
        <span class="tag">Python 3.10+</span>
        <span class="tag">MIT License</span>
        <span class="tag">Self-hosted</span>
      </div>
    </section>

    <!-- ============ 2. KPI 横带 ============ -->
    <section>
      <div class="section-label">PLATFORM OVERVIEW</div>
      <div class="kpi-band">

        <div class="kpi-col">
          <div class="kpi__label">知识库文档</div>
          <div class="kpi__value">128<span class="kpi__unit">篇</span></div>
          <div class="kpi__sub">已建索引</div>
          <svg class="kpi__spark" viewBox="0 0 60 24" aria-hidden="true">
            <polyline
              points="2,20 10,16 18,18 26,14 34,12 42,9 54,5"
              fill="none" stroke="var(--text-secondary)" stroke-width="1.3"
              stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
        </div>

        <div class="kpi-col">
          <div class="kpi__label">向量条目</div>
          <div class="kpi__value">246,318</div>
          <div class="kpi__sub">Milvus</div>
        </div>

        <div class="kpi-col">
          <div class="kpi__label">图谱实体</div>
          <div class="kpi__value">3,204</div>
          <div class="kpi__sub">Neo4j</div>
        </div>

        <div class="kpi-col">
          <div class="kpi__label">评测任务</div>
          <div class="kpi__value">47</div>
          <div class="kpi__sub">已完成 / 总计</div>
        </div>

      </div>
    </section>

    <!-- ============ 3. 传统方案 vs Link 对比 ============ -->
    <section>
      <div class="section-label">POSITIONING</div>
      <div class="compare">

        <div class="compare__col">
          <div class="compare__meta">TRADITIONAL TOOLS</div>
          <div class="compare__title compare__title--dim">传统数据工具</div>
          <ul class="compare__list compare__list--dim">
            <li>被动响应指令，等待人工提问</li>
            <li>生成图表与报表，呈现历史数据</li>
            <li>只描述「发生了什么」</li>
            <li>依赖 BI 工程师人工建模</li>
            <li>无法跨数据源联合推理</li>
          </ul>
        </div>

        <div class="compare__col compare__col--right">
          <div class="compare__meta">LINK AI EXPERT</div>
          <div class="compare__title compare__title--bright">Link AI 数据专家</div>
          <ul class="compare__list compare__list--bright">
            <li><span class="hi">主动思考</span>，拆解任务规划步骤</li>
            <li><span class="hi">输出决策建议</span>，而非仅呈现数字</li>
            <li>回答「<span class="hi">如何行动</span>」，驱动下一步</li>
            <li>自动获取 Schema，自主构造 SQL</li>
            <li>知识库 + 数据库 + 图谱联合推理</li>
          </ul>
        </div>

      </div>
    </section>

    <!-- ============ 4. 核心能力 2×2 ============ -->
    <section>
      <div class="section-label">CORE CAPABILITIES</div>
      <div class="cap-grid">

        <!-- RAG 知识问答 -->
        <div class="cap-cell">
          <div class="cap-cell__icon">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"
              stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20V2H6.5A2.5 2.5 0 0 0 4 4.5z"/>
              <path d="M4 19.5A2.5 2.5 0 0 0 6.5 22H20v-5"/>
              <path d="M9 10h6M9 14h4"/>
            </svg>
          </div>
          <div class="cap-cell__title">RAG 知识问答</div>
          <div class="cap-cell__desc">
            向量检索 + 图谱扩展联合召回，答案逐句可溯源至原始文档片段，重排序保证相关度。
          </div>
          <div class="cap-cell__note">REFS · RERANK · GRAPH</div>
        </div>

        <!-- SQL 数据分析 -->
        <div class="cap-cell">
          <div class="cap-cell__icon">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"
              stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <ellipse cx="12" cy="5" rx="8" ry="3"/>
              <path d="M4 5v14c0 1.7 3.6 3 8 3s8-1.3 8-3V5"/>
              <path d="M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3"/>
            </svg>
          </div>
          <div class="cap-cell__title">SQL 数据分析</div>
          <div class="cap-cell__desc">
            自动获取表结构、构造 SQL、执行并返回真实行集，结合 Python 做趋势拟合与预测，数字全部来源于真实查询。
          </div>
          <div class="cap-cell__note">SCHEMA · EXECUTE · PYTHON</div>
        </div>

        <!-- 知识图谱 -->
        <div class="cap-cell">
          <div class="cap-cell__icon">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"
              stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <circle cx="12" cy="5" r="2.5"/>
              <circle cx="4" cy="19" r="2.5"/>
              <circle cx="20" cy="19" r="2.5"/>
              <line x1="12" y1="7.5" x2="5.5" y2="16.8"/>
              <line x1="12" y1="7.5" x2="18.5" y2="16.8"/>
              <line x1="6.5" y1="19" x2="17.5" y2="19"/>
            </svg>
          </div>
          <div class="cap-cell__title">知识图谱</div>
          <div class="cap-cell__desc">
            实体关系与数据血缘可视化，支持从任意实体出发扩展关联，辅助根因分析与影响面评估。
          </div>
          <div class="cap-cell__note">NEO4J · LINEAGE · ENTITY</div>
        </div>

        <!-- RAG 评测 -->
        <div class="cap-cell">
          <div class="cap-cell__icon">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"
              stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="M3 3v18h18"/>
              <path d="M7 15l4-6 3 4 5-8"/>
              <circle cx="11" cy="9" r="1.5" fill="currentColor" stroke="none"/>
              <circle cx="14" cy="13" r="1.5" fill="currentColor" stroke="none"/>
            </svg>
          </div>
          <div class="cap-cell__title">RAG 评测</div>
          <div class="cap-cell__desc">
            量化检索质量，计算 Faithfulness、Relevance、Recall 等指标，任务级聚合持久化，支持多版本对比。
          </div>
          <div class="cap-cell__note">METRICS · BATCH · PERSIST</div>
        </div>

      </div>
    </section>

    <!-- ============ 5. 架构台账 ============ -->
    <section>
      <div class="section-label">ARCHITECTURE</div>
      <div class="arch-block">

        <div class="arch-layer arch-layer--first">
          <div class="arch-layer__label">GATEWAY</div>
          <div class="arch-layer__row">
            <span class="arch-chip arch-chip--hi">API Gateway</span>
            <span class="arch-sep">·</span>
            <span class="arch-note">HTTP / REST · request_id 全链路传递 · 鉴权 · 限流</span>
          </div>
        </div>

        <div class="arch-connector">
          <div class="arch-connector__line"></div>
        </div>

        <div class="arch-layer">
          <div class="arch-layer__label">GO · 编排与实时响应</div>
          <div class="arch-layer__row">
            <span class="arch-chip arch-chip--hi">Agent</span>
            <span class="arch-sep">·</span>
            <span class="arch-chip">RAG</span>
            <span class="arch-sep">·</span>
            <span class="arch-chip">Memory</span>
            <span class="arch-sep">·</span>
            <span class="arch-chip">Tools</span>
            <span class="arch-sep">·</span>
            <span class="arch-chip">Graph</span>
          </div>
          <div class="arch-sub">
            handler → service → model ← repository &nbsp;·&nbsp; Eino Agent 框架 &nbsp;·&nbsp; 流式输出
          </div>
        </div>

        <div class="arch-connector">
          <div class="arch-connector__line"></div>
        </div>

        <div class="arch-layer">
          <div class="arch-layer__label">PYTHON · 重计算与分析</div>
          <div class="arch-layer__row">
            <span class="arch-chip arch-chip--hi">Document</span>
            <span class="arch-sep">·</span>
            <span class="arch-chip">Evaluation</span>
            <span class="arch-sep">·</span>
            <span class="arch-chip">DataAnalysis</span>
          </div>
          <div class="arch-sub">
            gRPC（高性能大数据）&nbsp;|&nbsp; MCP（AI 工具调用）&nbsp;·&nbsp; 只做计算，不暴露 UI 接口
          </div>
        </div>

        <div class="arch-connector">
          <div class="arch-connector__line"></div>
        </div>

        <div class="arch-layer arch-layer--last">
          <div class="arch-layer__label">STORAGE</div>
          <div class="storage-row">
            <span class="storage-tag">MySQL · 元数据 / 配置 / 任务状态</span>
            <span class="storage-tag">Milvus · 向量 / 特征</span>
            <span class="storage-tag">Neo4j · 知识图谱 / 血缘</span>
            <span class="storage-tag">Redis · 缓存 / 队列</span>
          </div>
        </div>

      </div>
    </section>

    <!-- ============ 6. 页脚 caption ============ -->
    <footer class="page-footer">
      <span>v0.9-beta</span>
      <span class="footer-dot">·</span>
      <span>单机部署</span>
      <span class="footer-dot">·</span>
      <span>数据不出域</span>
      <span class="footer-dot">·</span>
      <span>MIT License</span>
      <span class="footer-dot">·</span>
      <span>2026-07-04</span>
    </footer>

  </div>
</template>

<script setup lang="ts">
// No external decoration components needed
</script>

<style scoped>
/* ── 容器 ── */
.home-view {
  max-width: 960px;
  margin: 0 auto;
  padding: 48px 0 64px;
  display: flex;
  flex-direction: column;
  gap: 60px;
  font-family: var(--font-body);
  color: var(--text-primary);
}

/* ── 区块标签 ── */
.section-label {
  font-family: var(--font-mono);
  font-size: 10px;
  letter-spacing: 2px;
  color: var(--text-muted);
  margin-bottom: 14px;
}

/* ══════════════ 1. Hero ══════════════ */
.hero {
  padding: 20px 0 4px;
}

.hero__title {
  font-family: var(--font-display);
  font-size: 32px;
  font-weight: 700;
  line-height: 1.4;
  color: var(--text-primary);
  letter-spacing: 0.5px;
  margin-bottom: 16px;
}

.hero__desc {
  font-size: 14.5px;
  line-height: 1.85;
  color: var(--text-secondary);
  max-width: 680px;
  margin-bottom: 28px;
}

.hero__tags {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.tag {
  font-family: var(--font-mono);
  font-size: 10px;
  letter-spacing: 1.2px;
  color: var(--text-muted);
  padding: 3px 10px;
  border-radius: 3px;
  border: 1px solid var(--border-default);
}

/* ══════════════ 2. KPI 横带 ══════════════ */
.kpi-band {
  border-top: 1px solid var(--border-default);
  border-bottom: 1px solid var(--border-default);
  display: flex;
}

.kpi-col {
  flex: 1;
  padding: 18px 20px;
  position: relative;
}

.kpi-col + .kpi-col {
  border-left: 1px solid var(--border-subtle);
}

.kpi__label {
  font-size: 11px;
  color: var(--text-muted);
  margin-bottom: 8px;
  letter-spacing: 0.3px;
}

.kpi__value {
  font-family: var(--font-mono);
  font-size: 24px;
  font-weight: 600;
  letter-spacing: -0.5px;
  color: var(--text-primary);
  line-height: 1;
}

.kpi__unit {
  font-size: 11px;
  color: var(--text-muted);
  margin-left: 2px;
  font-weight: 400;
}

.kpi__sub {
  margin-top: 6px;
  font-size: 10.5px;
  color: var(--text-muted);
  font-family: var(--font-mono);
}

.kpi__spark {
  position: absolute;
  right: 14px;
  bottom: 16px;
  width: 60px;
  height: 24px;
  opacity: 0.5;
}

/* ══════════════ 3. 对比 ══════════════ */
.compare {
  display: flex;
  gap: 0;
}

.compare__col {
  flex: 1;
  padding: 0 32px 0 0;
}

.compare__col--right {
  padding: 0 0 0 32px;
  border-left: 1px solid var(--border-default);
}

.compare__meta {
  font-family: var(--font-mono);
  font-size: 10px;
  letter-spacing: 1.8px;
  color: var(--text-muted);
  margin-bottom: 16px;
}

.compare__title {
  font-family: var(--font-display);
  font-size: 15px;
  font-weight: 700;
  margin-bottom: 16px;
  line-height: 1.4;
}

.compare__title--dim {
  color: var(--text-muted);
}

.compare__title--bright {
  color: var(--text-primary);
}

.compare__list {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.compare__list li {
  font-size: 13.5px;
  line-height: 1.7;
  padding-left: 16px;
  position: relative;
}

.compare__list--dim li {
  color: var(--text-muted);
}

.compare__list--dim li::before {
  content: "–";
  position: absolute;
  left: 0;
  color: var(--text-muted);
}

.compare__list--bright li {
  color: var(--text-primary);
}

.compare__list--bright li::before {
  content: "→";
  position: absolute;
  left: 0;
  color: var(--primary);
}

.hi {
  color: var(--primary);
  font-weight: 600;
}

/* ══════════════ 4. 核心能力 2×2 ══════════════ */
.cap-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 1px;
  background: var(--border-default);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  overflow: hidden;
}

.cap-cell {
  background: var(--bg-tertiary);
  padding: 24px 26px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.cap-cell__icon {
  width: 32px;
  height: 32px;
  color: var(--primary-dark);
  flex: 0 0 32px;
}

.cap-cell__icon svg {
  width: 24px;
  height: 24px;
}

.cap-cell__title {
  font-family: var(--font-display);
  font-size: 14px;
  font-weight: 700;
  color: var(--text-primary);
  line-height: 1.4;
}

.cap-cell__desc {
  font-size: 12.5px;
  line-height: 1.8;
  color: var(--text-muted);
}

.cap-cell__note {
  font-family: var(--font-mono);
  font-size: 10px;
  letter-spacing: 1px;
  color: var(--primary);
  margin-top: 2px;
}

/* ══════════════ 5. 架构台账 ══════════════ */
.arch-block {
  font-family: var(--font-mono);
  font-size: 12.5px;
  line-height: 1.9;
  color: var(--text-secondary);
}

.arch-layer {
  background: var(--bg-tertiary);
  padding: 16px 20px;
  border-top: 1px solid var(--border-subtle);
}

.arch-layer--first {
  border-top: 1px solid var(--border-default);
  border-radius: var(--radius-md) var(--radius-md) 0 0;
}

.arch-layer--last {
  border-bottom: 1px solid var(--border-default);
  border-radius: 0 0 var(--radius-md) var(--radius-md);
}

.arch-layer__label {
  font-size: 10px;
  letter-spacing: 2px;
  color: var(--text-muted);
  margin-bottom: 10px;
}

.arch-layer__row {
  display: flex;
  align-items: baseline;
  gap: 8px;
  flex-wrap: wrap;
}

.arch-sep {
  color: var(--text-muted);
  font-size: 11px;
}

.arch-note {
  font-size: 11px;
  color: var(--text-muted);
}

.arch-chip {
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--text-secondary);
  padding: 3px 10px;
  border-radius: 3px;
  border: 1px solid var(--border-default);
  background: var(--bg-elevated);
}

.arch-chip--hi {
  color: var(--primary);
  border-color: rgba(156, 180, 205, 0.3);
}

.arch-sub {
  margin-top: 8px;
  font-size: 10.5px;
  color: var(--text-muted);
}

.arch-connector {
  display: flex;
  align-items: center;
  height: 20px;
  padding-left: 24px;
}

.arch-connector__line {
  width: 1px;
  height: 100%;
  background: var(--border-default);
}

.storage-row {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 6px;
}

.storage-tag {
  font-family: var(--font-mono);
  font-size: 10.5px;
  color: var(--text-muted);
  padding: 2px 9px;
  border-radius: 3px;
  border: 1px solid var(--border-subtle);
}

/* ══════════════ 6. 页脚 ══════════════ */
.page-footer {
  border-top: 1px solid var(--border-subtle);
  padding-top: 14px;
  font-family: var(--font-mono);
  font-size: 10.5px;
  color: var(--text-muted);
  display: flex;
  gap: 20px;
  align-items: center;
  flex-wrap: wrap;
}

.footer-dot {
  color: var(--border-default);
}

/* ══════════════ 滚动条 ══════════════ */
::-webkit-scrollbar {
  width: 4px;
  height: 4px;
}
::-webkit-scrollbar-thumb {
  background: var(--border-default);
  border-radius: 2px;
}
::-webkit-scrollbar-track {
  background: transparent;
}
</style>
