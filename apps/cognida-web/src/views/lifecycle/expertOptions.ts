import type { ExpertOptions, PublicationTarget } from "@/api/lifecycle";

export const cleanOperations = [
  { value: "trim", label: "去除首尾空白" },
  { value: "normalize_ws", label: "合并连续空白" },
  { value: "drop_empty", label: "删除空行" },
  { value: "dedup", label: "去重" },
  { value: "text_cleaner", label: "文本规范化" },
  { value: "pii_masker", label: "个人信息脱敏" },
] as const;
export const enrichmentFeatures = [
  { value: "tags", label: "标签" },
  { value: "entities", label: "实体" },
  { value: "keywords", label: "关键词" },
  { value: "summary", label: "摘要" },
  { value: "category", label: "分类" },
  { value: "sentiment", label: "情感" },
  { value: "sensitivity", label: "敏感信息建议" },
] as const;
export const splitValues = (value: string) => [...new Set(value.split(/[,，]/).map((v) => v.trim()).filter(Boolean))];

export function defaultOptions(agent: string): ExpertOptions {
  switch (agent) {
    case "data-quality": return { min_score: 70 };
    case "data-cleaning": return { min_score: 70, operations: ["trim", "dedup"], keep: "first" };
    case "data-enrichment": return {
      method: "rules", labels: [], max_keywords: 10, max_summary_chars: 300,
      features: enrichmentFeatures.map((f) => f.value),
    };
    case "data-analysis": return { prompt: "" };
    default: return {};
  }
}

/** Build only the selected expert's supported fields, validating before any write. */
export function validatedOptions(agent: string, draft: ExpertOptions, inputKind: string, targets: PublicationTarget[]): ExpertOptions {
  const bounded = (value: number | undefined, min: number, max: number, name: string, integer = false) => {
    if (typeof value !== "number" || !Number.isFinite(value) || value < min || value > max || (integer && !Number.isInteger(value))) {
      throw new Error(`${name}须为 ${min}–${max} 之间的${integer ? "整数" : "数值"}`);
    }
    return value;
  };
  switch (agent) {
    case "data-quality": return { min_score: bounded(draft.min_score, 0, 100, "质量阈值") };
    case "data-cleaning": {
      const operations = draft.operations || [];
      if (!operations.length || operations.some((op) => !cleanOperations.some((item) => item.value === op))) throw new Error("请选择有效的清洗操作");
      if (draft.keep !== "first" && draft.keep !== "last") throw new Error("请选择去重保留策略");
      const columns = (draft.columns || []).map((v) => v.trim()).filter(Boolean);
      if (columns.length > 200 || columns.some((v) => v.length > 200)) throw new Error("清洗列最多 200 个，列名最长 200 字符");
      return { min_score: bounded(draft.min_score, 0, 100, "清洗后质量阈值"), operations: [...operations], keep: draft.keep, ...(columns.length ? { columns } : {}) };
    }
    case "data-enrichment": {
      if (draft.method !== "rules" && draft.method !== "llm") throw new Error("请选择富集方法");
      const labels = (draft.labels || []).map((label) => ({ name: label.name.trim(), keywords: label.keywords.map((v) => v.trim()).filter(Boolean) }));
      if (labels.length > 30 || labels.some((label) => !label.name || label.name.length > 80 || !label.keywords.length || label.keywords.length > 30 || label.keywords.some((v) => v.length > 100))) throw new Error("每个标签需名称（1–80 字符）和 1–30 个关键词（每个最多 100 字符），最多 30 个标签");
      const features = draft.features || [];
      if (!features.length || features.some((f) => !enrichmentFeatures.some((item) => item.value === f))) throw new Error("请选择至少一项富集内容");
      return { method: draft.method, labels, features: [...features], max_keywords: bounded(draft.max_keywords, 1, 30, "关键词数量", true), max_summary_chars: bounded(draft.max_summary_chars, 20, 2000, "摘要长度", true) };
    }
    case "data-asset": {
      if (inputKind === "text") {
        if (!draft.knowledge_base_id) throw new Error("文本发布前请选择目标知识库");
        if (!targets.some((target) => target.id === draft.knowledge_base_id && target.status !== 0)) throw new Error("目标知识库不在当前可用列表中，请刷新后选择");
        return { knowledge_base_id: draft.knowledge_base_id };
      }
      if (inputKind !== "table") throw new Error("无法确认输入类型，请重新选择输入资产");
      const id = draft.catalog_asset_id?.trim();
      return id ? { catalog_asset_id: id } : {};
    }
    case "data-analysis": {
      const prompt = (draft.prompt || "").trim();
      if (prompt.length > 4000) throw new Error("分析目标最多 4000 字符");
      return { prompt };
    }
    default: return {};
  }
}
