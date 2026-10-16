import { http } from "@/utils/request";
import { withIdempotency } from "@/utils/idempotent";
import type { RefID } from "@/api/lifecycle";
import type { PlanSpec } from "@/api/taskcontrol";
export interface FeedbackInput {
  title: string;
  knowledge_base_id: string;
  baseline_ref: RefID;
  evidence: {
    output_ref: RefID;
    source_refs: RefID[];
    evaluation_task_id?: string;
    evaluation_result_id?: string;
    question: string;
    problem: string;
    stage: string;
  };
  dataset_version_id: string;
  evaluation_config: {
    dataset_id: string;
    type: "rag";
    kb_id: string;
    config: { top_k: number; graders: string[] };
  };
  gate: {
    metric: string;
    higher_is_better: boolean;
    min_pairs: number;
    max_regression: number;
    min_improvement: number;
  };
  budget: {
    max_rounds: number;
    max_calls: number;
    max_cost_micros: number;
    timeout_seconds: number;
  };
}
export interface FeedbackCase extends FeedbackInput {
  id: string;
  version: number;
  status: string;
  baseline_knowledge_id: string;
  active_version_id: string;
  reserved_calls: number;
  reserved_cost_micros: number;
  deadline: string;
  error?: string;
  corrections: { ref: RefID; digest: string }[];
  rounds: {
    id: string;
    task_id?: string;
    status: string;
    reserved_calls: number;
  }[];
}
export interface FeedbackPublication {
  id: string;
  case_id: string;
  version: number;
  status: string;
  source_ref: RefID;
  round_id: string;
  knowledge_id: string;
  labels: string[];
  projection?: { index: string; state: string; chunk_ids: string[] };
  baseline_evaluation_id?: string;
  candidate_evaluation_id?: string;
  comparison?: {
    pairs: number;
    missing_pairs: number;
    baseline_mean: number;
    candidate_mean: number;
    improvement: number;
    lower_95?: number;
    upper_95?: number;
    gate: string;
    recommendation: string;
    limitations: string[];
  };
  approved_by?: number;
  error?: string;
}
const path = "/feedback-cases";
const post = <T>(url: string, data: unknown) =>
  withIdempotency(url, data, (key) =>
    http.post<T>(url, data, {
      headers: { "Idempotency-Key": key },
      timeout: 120000,
    }),
  );
export const feedbackApi = {
  evidenceResults: (task: string, cursor = "") =>
    http.get<{
      results: { id: string | number; question: string }[];
      next_cursor?: string;
    }>(`/evaluation/results/${encodeURIComponent(task)}/qa`, {
      params: { cursor, page_size: 100 },
    }),
  list: (cursor = "") =>
    http.get<{ items: FeedbackCase[]; next_cursor: string }>(path, {
      params: { cursor },
    }),
  get: (id: string) =>
    http.get<FeedbackCase>(`${path}/${encodeURIComponent(id)}`),
  create: (data: FeedbackInput) => post<FeedbackCase>(path, data),
  publications: (id: string) =>
    http.get<{ items: FeedbackPublication[] }>(
      `${path}/${encodeURIComponent(id)}/publications`,
    ),
  events: (id: string) =>
    http.get<{
      items: {
        id: string;
        action: string;
        detail: string;
        created_at: string;
      }[];
    }>(`${path}/${encodeURIComponent(id)}/events`),
  correct: (c: FeedbackCase, text: string) =>
    post<FeedbackCase>(`${path}/${c.id}/corrections`, {
      expected_version: c.version,
      text,
    }),
  rework: (c: FeedbackCase, plan: PlanSpec) =>
    post<FeedbackCase>(`${path}/${c.id}/rework`, {
      expected_version: c.version,
      plan,
    }),
  candidate: (
    c: FeedbackCase,
    candidate: {
      source_ref: RefID;
      round_id: string;
      label_result_id?: string;
      label_review_version?: number;
    },
  ) =>
    post<FeedbackPublication>(`${path}/${c.id}/candidates`, {
      expected_version: c.version,
      candidate,
    }),
  control: (
    v: FeedbackPublication,
    action: string,
    c: FeedbackCase,
    comment = "",
  ) =>
    post<FeedbackPublication | FeedbackCase>(
      `/feedback-publications/${v.id}/${action}`,
      {
        expected_version: v.version,
        expected_case_version: c.version,
        comment,
      },
    ),
  withdraw: (c: FeedbackCase) =>
    post<FeedbackCase>(`${path}/${c.id}/withdraw`, {
      expected_version: c.version,
    }),
  rollback: (c: FeedbackCase, publication_id: string) =>
    post<FeedbackCase>(`${path}/${c.id}/rollback`, {
      expected_version: c.version,
      publication_id,
    }),
};
