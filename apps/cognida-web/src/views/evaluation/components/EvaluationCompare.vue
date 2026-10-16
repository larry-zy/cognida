<template>
  <details class="workbench compare-panel">
    <summary>版本评测对比 / Paired evaluation comparison</summary>
    <p>
      选择同一数据集和任务类型的两个已完成任务，按逐题指标比较。结果仅供灰度与回滚审查，不自动更改生产开关。
    </p>
    <form class="wb-form" @submit.prevent="compare">
      <label>基线任务 ID<input v-model="form.baseline_id" required /></label>
      <label>候选任务 ID<input v-model="form.candidate_id" required /></label>
      <label
        >指标<input
          v-model="form.metric"
          required
          list="comparison-metrics" /><datalist id="comparison-metrics">
          <option value="answer_accuracy" />
          <option value="tool_selection" />
          <option value="step_efficiency" />
          <option value="faithfulness" /></datalist
      ></label>
      <label
        >最少配对数量<input
          v-model.number="form.min_pairs"
          type="number"
          min="2"
          required
      /></label>
      <label
        >允许回退幅度<input
          v-model.number="form.max_regression"
          type="number"
          min="0"
          step="0.01"
          required
      /></label>
      <label class="wb-checkbox"
        ><input
          v-model="form.higher_is_better"
          type="checkbox"
        />指标越高越好</label
      >
      <button class="primary" :disabled="busy">
        {{ busy ? "计算中…" : "比较真实结果" }}
      </button>
    </form>
    <p v-if="error" class="wb-error" role="alert">{{ error }}</p>
    <section v-if="result" class="wb-panel" style="margin-top: 16px">
      <h3>{{ gateLabel[result.gate] || result.gate }}</h3>
      <p>{{ result.recommendation }}</p>
      <router-link
        v-if="comparedCandidate"
        :to="{ path: `/evaluation/${encodeURIComponent(comparedCandidate)}` }"
        >查看候选逐题结果并记录返工反馈</router-link
      >
      <div class="wb-metrics">
        <div>
          <strong>{{ result.pairs }}</strong
          ><small>配对题目</small>
        </div>
        <div>
          <strong>{{ result.missing_pairs }}</strong
          ><small>未配对题目</small>
        </div>
        <div>
          <strong>{{ result.improvement.toFixed(4) }}</strong
          ><small>按指标方向校正的改善</small>
        </div>
      </div>
      <p>
        基线均值 {{ result.baseline_mean }} · 候选均值
        {{ result.candidate_mean }}
      </p>
      <p>
        95% 区间：{{ result.lower_95 ?? "证据不足" }} —
        {{ result.upper_95 ?? "证据不足" }}
      </p>
      <p class="wb-footnote">方法：{{ result.method }}</p>
      <ul>
        <li v-for="item in result.limitations" :key="item">{{ item }}</li>
      </ul>
    </section>
  </details>
</template>
<script setup lang="ts">
import { reactive, ref } from "vue";
import { http } from "@/utils/request";
import "@/components/workbench/workbench.css";
interface Comparison {
  pairs: number;
  missing_pairs: number;
  baseline_mean: number;
  candidate_mean: number;
  improvement: number;
  lower_95: number | null;
  upper_95: number | null;
  gate: string;
  recommendation: string;
  method: string;
  limitations: string[];
}
const form = reactive({
    baseline_id: "",
    candidate_id: "",
    metric: "answer_accuracy",
    higher_is_better: true,
    min_pairs: 20,
    max_regression: 0,
  }),
  busy = ref(false),
  error = ref(""),
  result = ref<Comparison>(),
  comparedCandidate = ref("");
const gateLabel: Record<string, string> = {
  insufficient_evidence: "证据不足",
  regression: "发现回退",
  eligible_for_review: "可进入人工发布审查",
  inconclusive: "结论不确定",
};
async function compare() {
  busy.value = true;
  error.value = "";
  result.value = undefined;
  comparedCandidate.value = "";
  try {
    if (form.baseline_id === form.candidate_id)
      throw new Error("请选择不同的两个任务");
    const input = { ...form };
    result.value = (
      await http.post<Comparison>("/evaluation/compare", input)
    ).data;
    comparedCandidate.value = input.candidate_id;
  } catch (e) {
    error.value = e instanceof Error ? e.message : "对比失败";
  } finally {
    busy.value = false;
  }
}
</script>
<style scoped>
.compare-panel {
  padding: 18px;
  border: 1px solid var(--border-color);
  border-radius: 10px;
  margin-bottom: 20px;
}
.compare-panel summary {
  cursor: pointer;
  font-weight: 600;
}
.compare-panel p {
  font-size: 13px;
  line-height: 1.7;
}
</style>
