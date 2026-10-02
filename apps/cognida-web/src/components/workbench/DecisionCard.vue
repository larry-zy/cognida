<template>
  <section class="wb-panel decision-card">
    <div class="wb-section-head" style="margin-top: 0">
      <h3>{{ wt("可信决策") }}</h3>
      <span class="wb-badge">{{
        decision?.verification_status || "未提供验证结论"
      }}</span>
    </div>
    <p>{{ summary || "暂无结论" }}</p>
    <p v-if="decision?.recommendation" class="decision-recommendation">
      <strong>{{ wt("建议行动") }}</strong
      ><br />{{ decision.recommendation }}
    </p>
    <dl>
      <dt>{{ wt("计算方法") }}</dt>
      <dd>{{ decision?.method || "生产者未提供方法说明" }}</dd>
      <dt>{{ wt("置信度") }}</dt>
      <dd>{{ decision?.confidence ?? "未评估" }}</dd>
      <dt>{{ wt("假设") }}</dt>
      <dd>
        <ul v-if="decision?.assumptions?.length">
          <li v-for="item in decision.assumptions" :key="item">{{ item }}</li>
        </ul>
        <span v-else>未提供</span>
      </dd>
      <dt>{{ wt("风险与限制") }}</dt>
      <dd>
        <ul v-if="decision?.risks?.length">
          <li v-for="item in decision.risks" :key="item">{{ item }}</li>
        </ul>
        <span v-else>未提供风险评估</span>
      </dd>
    </dl>
    <h3>{{ wt("依据与来源") }}</h3>
    <div v-if="refs.length" class="wb-actions">
      <button
        v-for="ref in refs"
        :key="`${ref.asset_id}:${ref.version}`"
        @click="$emit('open', ref)"
      >
        {{ ref.kind || "资产" }} · {{ ref.asset_id.slice(0, 12) }} · v{{
          ref.version
        }}
      </button>
    </div>
    <p v-else class="wb-muted">未附带证据引用，不能认定为已验证结论。</p>
  </section>
</template>
<script setup lang="ts">
import { wt } from "@/i18n/workbench";

import type { RefID, Decision } from "@/api/lifecycle";
defineProps<{ decision?: Decision; summary?: string; refs: RefID[] }>();
defineEmits<{ open: [RefID] }>();
</script>
<style scoped>
.decision-card {
  border-left: 3px solid var(--primary);
}
dl {
  display: grid;
  grid-template-columns: 90px 1fr;
  gap: 12px;
  font-size: 12px;
  margin: 20px 0;
}
dt {
  color: var(--text-muted);
}
dd {
  margin: 0;
  line-height: 1.6;
}
ul {
  margin: 0;
  padding-left: 16px;
}
</style>
