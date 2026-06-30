<template>
  <div class="home-view">
    <!-- Hero -->
    <section class="hero-section">
      <span class="hero-badge">AI-Native 智能 Data 平台</span>
      <h1 class="hero-title">
        <GradientText variant="primary">Link</GradientText>
      </h1>
      <p class="hero-tagline">让每个组织都能构建自主进化的数据大脑</p>
      <p class="hero-desc">从"数据助手"到"业务伙伴"的跃迁 —— AI 具备主动思考、深度分析、输出行动建议的能力</p>
      <div class="tech-tags">
        <span class="tech-tag">Go 1.21+</span>
        <span class="tech-tag">Python 3.10+</span>
        <span class="tech-tag">MIT License</span>
      </div>
    </section>

    <!-- 对比 -->
    <section class="section">
      <h2 class="section-title">核心定位</h2>
      <div class="comparison-box">
        <div class="comp-side traditional">
          <h3>传统数据工具</h3>
          <ul>
            <li>被动响应指令</li>
            <li>生成图表报表</li>
            <li>描述"发生了什么"</li>
            <li>工具属性</li>
          </ul>
        </div>
        <div class="comp-arrow">→</div>
        <div class="comp-side link">
          <h3>Link AI 数据专家</h3>
          <ul>
            <li>主动思考拆解任务</li>
            <li>输出决策建议</li>
            <li>回答"如何行动"</li>
            <li>业务伙伴属性</li>
          </ul>
        </div>
      </div>
    </section>

    <!-- 核心能力 -->
    <section class="section">
      <h2 class="section-title">核心能力</h2>
      <div class="capabilities-grid">
        <div class="capability-card" v-for="(item, i) in capabilities" :key="i">
          <span class="capability-icon">{{ item.icon }}</span>
          <h3>{{ item.title }}</h3>
          <p>{{ item.desc }}</p>
        </div>
      </div>
    </section>

    <!-- 架构 -->
    <section class="section arch-section">
      <h2 class="section-title">技术架构</h2>
      <div class="arch-diagram">
        <div class="arch-tier gateway">API Gateway</div>
        <div class="arch-arrow">↓</div>
        <div class="arch-services">
          <div class="arch-service go">
            <span class="service-badge">GO</span>
            <span class="service-name">编排与实时</span>
            <div class="service-items">
              <span>Agent</span>
              <span>RAG</span>
              <span>Memory</span>
              <span>Tools</span>
            </div>
          </div>
          <div class="arch-service python">
            <span class="service-badge">PY</span>
            <span class="service-name">AI/ML 重计算</span>
            <div class="service-items">
              <span>Document</span>
              <span>Evaluation</span>
            </div>
          </div>
        </div>
        <div class="arch-arrow">↓</div>
        <div class="arch-storage">
          <span>MySQL</span>
          <span>Milvus</span>
          <span>Neo4j</span>
          <span>Redis</span>
        </div>
      </div>
    </section>

    <!-- 场景 -->
    <section class="section">
      <h2 class="section-title">应用场景</h2>
      <div class="scenarios-grid">
        <div class="scenario-card" v-for="(sc, i) in scenarios" :key="i">
          <span class="scenario-icon">{{ sc.icon }}</span>
          <h3>{{ sc.industry }}</h3>
          <div class="scenario-items">
            <div v-for="item in sc.items" :key="item.scene">
              <span>{{ item.scene }}</span>
              <b>{{ item.value }}</b>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- 快速开始 -->
    <section class="section quickstart-section">
      <h2 class="section-title">快速开始</h2>
      <div class="quickstart">
        <div class="qs-steps">
          <div class="qs-step" v-for="(step, i) in steps" :key="i">
            <span class="qs-num">{{ i + 1 }}</span>
            <div>
              <h4>{{ step.title }}</h4>
              <p>{{ step.desc }}</p>
            </div>
          </div>
        </div>
        <div class="qs-code">
          <pre><code>git clone https://github.com/your-org/link.git
cd link
docker-compose up -d</code></pre>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import GradientText from '@/components/visual/GradientText.vue'

const capabilities = [
  { icon: '🤖', title: 'Agent 系统', desc: 'ReAct 编排、深度研究、Hooks 机制' },
  { icon: '🔍', title: 'RAG 检索', desc: '向量检索、全文检索、混合重排序' },
  { icon: '🕸️', title: '知识图谱', desc: '实体关系抽取、多跳查询、可视化' },
  { icon: '📊', title: '评测系统', desc: '检索评测、生成评测、LLM 智能评分' },
  { icon: '🔧', title: '工具系统', desc: 'SQL 生成、数据查询、网页抓取' },
  { icon: '✅', title: '质量服务', desc: '五维质量检查、数据清洗、漂移检测' }
]

const scenarios = [
  {
    icon: '🛒',
    industry: '零售',
    items: [
      { scene: '销售分析', value: '+80% 效率' },
      { scene: '库存优化', value: '-30% 积压' }
    ]
  },
  {
    icon: '💰',
    industry: '金融',
    items: [
      { scene: '智能投研', value: '10x 效率' },
      { scene: '风险预警', value: '提前2-3天' }
    ]
  },
  {
    icon: '🏭',
    industry: '制造',
    items: [
      { scene: '设备健康', value: '-50% 停机' },
      { scene: '生产优化', value: '+20% 效率' }
    ]
  },
  {
    icon: '🏥',
    industry: '医疗',
    items: [
      { scene: '知识助手', value: '5x 效率' },
      { scene: '科研分析', value: '-40% 周期' }
    ]
  }
]

const steps = [
  { title: '环境准备', desc: '安装 Docker & Docker Compose' },
  { title: '克隆项目', desc: 'git clone 仓库到本地' },
  { title: '启动服务', desc: 'docker-compose up -d' }
]
</script>

<style scoped>
.home-view {
  width: 100%;
  max-width: 1000px;
  margin: 0 auto;
}

.section {
  margin-bottom: var(--space-12);
}

.section-title {
  font: var(--font-display);
  font-size: var(--text-2xl);
  font-weight: 700;
  color: var(--text-primary);
  text-align: center;
  margin: 0 0 var(--space-8);
}

/* Hero */
.hero-section {
  text-align: center;
  padding: var(--space-12) var(--space-4);
  margin-bottom: var(--space-12);
}

.hero-badge {
  display: inline-block;
  padding: var(--space-2) var(--space-4);
  background: rgba(34, 211, 238, 0.1);
  border: 1px solid rgba(34, 211, 238, 0.3);
  border-radius: var(--radius-full);
  font-size: var(--text-xs);
  color: var(--primary);
  margin-bottom: var(--space-4);
}

.hero-title {
  font-size: var(--text-4xl);
  font-weight: 800;
  margin: 0 0 var(--space-4);
}

.hero-tagline {
  font-size: var(--text-xl);
  color: var(--text-primary);
  margin: 0 0 var(--space-2);
}

.hero-desc {
  font-size: var(--text-base);
  color: var(--text-secondary);
  line-height: 1.6;
  margin: 0 0 var(--space-5);
}

.tech-tags {
  display: flex;
  gap: var(--space-2);
  justify-content: center;
  flex-wrap: wrap;
}

.tech-tag {
  padding: var(--space-1) var(--space-3);
  background: var(--bg-tertiary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  font-size: var(--text-xs);
  font-family: var(--font-mono);
  color: var(--text-secondary);
}

/* Comparison */
.comparison-box {
  display: flex;
  align-items: stretch;
  gap: var(--space-4);
  max-width: 700px;
  margin: 0 auto;
}

.comp-side {
  flex: 1;
  padding: var(--space-5);
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
}

.comp-side.link {
  background: rgba(34, 211, 238, 0.05);
  border-color: rgba(34, 211, 238, 0.2);
}

.comp-side h3 {
  font: var(--font-display);
  font-size: var(--text-base);
  font-weight: 600;
  color: var(--text-primary);
  margin: 0 0 var(--space-3);
}

.comp-side ul {
  list-style: none;
  padding: 0;
  margin: 0;
}

.comp-side li {
  padding: var(--space-2) 0;
  font-size: var(--text-sm);
  color: var(--text-secondary);
  border-bottom: 1px solid var(--border-subtle);
}

.comp-side li:last-child {
  border-bottom: none;
}

.comp-side.link li {
  color: var(--text-primary);
}

.comp-arrow {
  display: flex;
  align-items: center;
  font-size: 24px;
  color: var(--primary);
}

/* Capabilities */
.capabilities-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--space-4);
}

.capability-card {
  padding: var(--space-5);
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  transition: all var(--duration-base);
}

.capability-card:hover {
  border-color: var(--primary);
  box-shadow: 0 12px 24px -12px rgba(34, 211, 238, 0.2);
}

.capability-icon {
  font-size: 24px;
  margin-bottom: var(--space-3);
}

.capability-card h3 {
  font: var(--font-display);
  font-size: var(--text-lg);
  font-weight: 600;
  color: var(--text-primary);
  margin: 0 0 var(--space-2);
}

.capability-card p {
  font-size: var(--text-sm);
  color: var(--text-secondary);
  margin: 0;
}

/* Architecture */
.arch-diagram {
  max-width: 500px;
  margin: 0 auto;
}

.arch-tier {
  text-align: center;
  padding: var(--space-3);
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  font-size: var(--text-sm);
  font-weight: 600;
  color: var(--text-primary);
}

.arch-tier.gateway {
  background: rgba(34, 211, 238, 0.08);
  border-color: rgba(34, 211, 238, 0.2);
}

.arch-arrow {
  text-align: center;
  color: var(--text-muted);
  margin: var(--space-2) 0;
}

.arch-services {
  display: grid;
  grid-template-columns: 2fr 1fr;
  gap: var(--space-3);
}

.arch-service {
  padding: var(--space-4);
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
}

.arch-service.go {
  background: rgba(34, 211, 238, 0.03);
  border-color: rgba(34, 211, 238, 0.15);
}

.arch-service.python {
  background: rgba(244, 114, 182, 0.03);
  border-color: rgba(244, 114, 182, 0.15);
}

.service-badge {
  display: inline-block;
  padding: var(--space-1) var(--space-2);
  background: var(--bg-tertiary);
  border-radius: var(--radius-sm);
  font-size: var(--text-xs);
  font-weight: 700;
}

.arch-service.go .service-badge { color: var(--primary); }
.arch-service.python .service-badge { color: var(--accent); }

.service-name {
  display: block;
  margin-top: var(--space-2);
  font-size: var(--text-sm);
  color: var(--text-primary);
}

.service-items {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  margin-top: var(--space-4);
}

.service-items span {
  padding: var(--space-1) var(--space-3);
  background: var(--bg-tertiary);
  border-radius: var(--radius-sm);
  font-size: var(--text-xs);
  color: var(--text-secondary);
}

.arch-storage {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--space-3);
}

.arch-storage span {
  padding: var(--space-3);
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  text-align: center;
  font-size: var(--text-sm);
  color: var(--text-secondary);
}

/* Scenarios */
.scenarios-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--space-4);
}

.scenario-card {
  padding: var(--space-4);
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  text-align: center;
}

.scenario-icon {
  font-size: 24px;
  margin-bottom: var(--space-2);
}

.scenario-card h3 {
  font: var(--font-display);
  font-size: var(--text-base);
  font-weight: 600;
  color: var(--text-primary);
  margin: 0 0 var(--space-3);
}

.scenario-items > div {
  display: flex;
  justify-content: space-between;
  padding: var(--space-2) 0;
  font-size: var(--text-sm);
  border-top: 1px solid var(--border-subtle);
}

.scenario-items span:first-child {
  color: var(--text-secondary);
}

.scenario-items b {
  color: var(--primary);
}

/* Quickstart */
.quickstart {
  max-width: 450px;
  margin: 0 auto;
  text-align: center;
}

.qs-steps {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  margin-bottom: var(--space-6);
  text-align: left;
}

.qs-step {
  display: flex;
  gap: var(--space-3);
}

.qs-num {
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--primary);
  color: #000;
  border-radius: 50%;
  font-weight: 700;
  flex-shrink: 0;
  font-size: var(--text-xs);
}

.qs-step h4 {
  font-size: var(--text-sm);
  font-weight: 600;
  color: var(--text-primary);
  margin: 0 0 var(--space-1);
}

.qs-step p {
  font-size: var(--text-xs);
  color: var(--text-secondary);
  margin: 0;
}

.qs-code {
  background: var(--bg-tertiary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  padding: var(--space-4);
  text-align: left;
}

.qs-code pre {
  margin: 0;
  font-family: var(--font-mono);
  font-size: var(--text-xs);
  color: var(--text-secondary);
  line-height: 1.6;
}

/* Responsive */
@media (max-width: 768px) {
  .capabilities-grid {
    grid-template-columns: repeat(2, 1fr);
  }

  .scenarios-grid {
    grid-template-columns: repeat(2, 1fr);
  }

  .comparison-box {
    flex-direction: column;
  }

  .comp-arrow {
    transform: rotate(90deg);
  }
}

@media (max-width: 480px) {
  .capabilities-grid,
  .scenarios-grid {
    grid-template-columns: 1fr;
  }

  .arch-services {
    grid-template-columns: 1fr;
  }

  .arch-storage {
    grid-template-columns: repeat(2, 1fr);
  }

  .hero-title {
    font-size: var(--text-3xl);
  }
}
</style>
