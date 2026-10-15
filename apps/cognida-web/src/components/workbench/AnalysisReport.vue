<template>
  <article v-if="report" class="analysis-report wb-stack" aria-label="分析报告">
    <div class="wb-section-head">
      <h3>分析报告</h3>
      <span class="wb-badge">{{ report.decision?.verification_status || '未提供验证结论' }}</span>
    </div>
    <p v-if="report.prompt"><strong>分析目标：</strong>{{ report.prompt }}</p>
    <p v-if="report.narrative" class="report-narrative">{{ report.narrative }}</p>
    <section v-if="report.findings.length">
      <h4>发现与依据</h4>
      <ol>
        <li v-for="(finding, index) in report.findings" :key="index">
          <p>{{ finding.text }}</p>
          <span class="wb-muted">证据：</span>
          <a v-for="id in finding.evidence_ids" :key="id" :href="`#${anchor(id)}`" class="evidence-link">{{ id }}</a>
          <span v-if="!finding.evidence_ids.length" class="wb-muted">未附带证据</span>
        </li>
      </ol>
    </section>
    <section v-if="report.evidence.length">
      <h4>证据摘录</h4>
      <dl>
        <template v-for="evidence in report.evidence" :key="evidence.id">
          <dt :id="anchor(evidence.id)">{{ evidence.id }}</dt>
          <dd>{{ evidence.content }}</dd>
        </template>
      </dl>
    </section>
    <section v-if="report.limitations.length">
      <h4>限制与待核实事项</h4>
      <ul><li v-for="(limitation, index) in report.limitations" :key="index">{{ limitation }}</li></ul>
    </section>
    <section v-if="report.columns.length || report.record_count !== undefined || report.character_count !== undefined">
      <h4>描述统计</h4>
      <p v-if="report.record_count !== undefined">数据行数：{{ report.record_count }}</p>
      <p v-if="report.character_count !== undefined">字符数：{{ report.character_count }}<span v-if="report.word_count !== undefined"> · 词数：{{ report.word_count }}</span></p>
      <div v-if="report.columns.length" class="wb-table-wrap">
        <table class="wb-table">
          <thead><tr><th>字段</th><th>缺失数</th><th>数值数量</th><th>均值</th><th>最小值</th><th>最大值</th></tr></thead>
          <tbody><tr v-for="(column, index) in report.columns" :key="index"><td>{{ column.name }}</td><td>{{ column.missing_count ?? '—' }}</td><td>{{ column.numeric_count ?? '—' }}</td><td>{{ column.mean ?? '—' }}</td><td>{{ column.min ?? '—' }}</td><td>{{ column.max ?? '—' }}</td></tr></tbody>
        </table>
      </div>
    </section>
    <p v-if="report.source_ref" class="wb-footnote">来源资产：{{ report.source_ref }}</p>
    <p v-if="report.narrative" class="wb-muted">模型解释须由业务人员核实；证据引用不代表因果关系已验证。</p>
  </article>
</template>
<script setup lang="ts">
import { computed } from "vue";
const props = defineProps<{ content: unknown }>();
const prefix = `report-${crypto.randomUUID()}`;
const anchor = (id: string) => `${prefix}-${encodeURIComponent(id)}`;
const object = (value: unknown): Record<string, unknown> | undefined => value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : undefined;
const strings = (value: unknown) => Array.isArray(value) ? value.filter((v): v is string => typeof v === "string") : [];
const scalar = (value: unknown) => typeof value === "number" || typeof value === "string" ? value : undefined;
const text = (value: unknown) => typeof value === "string" ? value : "";
const report = computed(() => {
  const source = object(props.content);
  if (!source || !("narrative" in source || "record_count" in source || "character_count" in source)) return undefined;
  const rows = (value: unknown) => Array.isArray(value) ? value.map(object).filter((v): v is Record<string, unknown> => !!v) : [];
  const decision = object(source.decision);
  return {
    prompt: text(source.prompt), narrative: text(source.narrative),
    source_ref: text(source.source_ref) || text(source.source_ref_id),
    findings: rows(source.findings).map((f) => ({ text: text(f.text), evidence_ids: strings(f.evidence_ids) })),
    evidence: rows(source.evidence).map((e) => ({ id: text(e.id), content: text(e.content) })),
    limitations: strings(source.limitations),
    columns: rows(source.columns).map((c) => ({ name: text(c.name), missing_count: scalar(c.missing_count), numeric_count: scalar(c.numeric_count), mean: scalar(c.mean), min: scalar(c.min), max: scalar(c.max) })),
    record_count: scalar(source.record_count), character_count: scalar(source.character_count), word_count: scalar(source.word_count),
    decision: decision ? { verification_status: text(decision.verification_status) } : undefined,
  };
});
</script>
<style scoped>
.analysis-report { overflow-wrap: anywhere; font-size: 14px; line-height: 1.7; }
.report-narrative, dd { white-space: pre-wrap; }
.evidence-link { margin-right: 10px; }
dt { font-weight: 600; margin-top: 12px; scroll-margin-top: 24px; }
dd { margin: 4px 0 0; color: var(--text-secondary); }
ol, ul { padding-left: 22px; }
h4 { margin: 0 0 8px; }
</style>
