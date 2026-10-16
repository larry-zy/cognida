import { http } from "@/utils/request";
import { withIdempotency } from "@/utils/idempotent";
import type {
  GovernanceMeta,
  GovernanceSubscription,
  GovernanceSubscriptionRun,
  GovernanceNotification,
  GovernanceProvenance,
  GovernanceAnnotationTask,
  GovernanceAnnotationItem,
  GovernanceTaggingTask,
  GovernanceTaggingResult,
  GovernanceFeature,
  GovernanceFeatureValue,
  GovernanceSuggestion,
} from "../contracts.generated";
export type Meta = GovernanceMeta;
export interface Page<T> {
  items: T[];
  next_cursor: string;
  has_more: boolean;
}
export type Subscription = GovernanceSubscription;
export type SubscriptionRun = GovernanceSubscriptionRun;
export type Notification = GovernanceNotification;
export interface Label {
  id: string;
  name: string;
  keywords?: string[];
  terms?: string[];
}
export interface QAMapping {
  question_column: string;
  reference_answer_column: string;
}
export type Provenance = GovernanceProvenance & {
  decision_status?: "matched" | "uncertain" | "manual";
  evidence_spans?: {
    label_id: string;
    rule: string;
    match: string;
    start: number;
    end: number;
  }[];
};
export type AnnotationTask = Omit<GovernanceAnnotationTask, "labels"> & {
  labels: Label[] | null;
  standard_version_id?: string;
  qa_mapping?: QAMapping;
};
export type AnnotationItem = Omit<
  GovernanceAnnotationItem,
  "annotation" | "provenance"
> & {
  annotation: GovernanceAnnotationItem["annotation"] & {
    qa?: { question: string; reference_answer: string };
    evaluation_profile?: import("@/api/evaluation/versions").EvaluationProfile;
  };
  provenance: Provenance;
};
export type TagTask = Omit<GovernanceTaggingTask, "config"> & {
  standard_version_id?: string;
  config: GovernanceTaggingTask["config"] & { label_schema?: Label[] };
};
export type TagResult = Omit<
  GovernanceTaggingResult,
  "entities" | "provenance"
> & {
  entities: Record<string, unknown>[] | null;
  provenance: Provenance;
};
export type ExpressionOp =
  | "column"
  | "literal"
  | "add"
  | "sub"
  | "mul"
  | "div"
  | "abs"
  | "round"
  | "clamp"
  | "lower"
  | "upper"
  | "trim"
  | "concat"
  | "category_map"
  | "to_number";
export interface FeatureExpression {
  op: ExpressionOp;
  field?: string;
  value?: unknown;
  args?: FeatureExpression[];
  mapping?: Record<string, string>;
  default?: string;
}
export interface FeatureDefinition {
  kind: "field" | "expression" | "window_aggregate" | "embedding";
  embedding?: { model_id: string; revision: string; dimension: number };
  field?: string;
  entity_field?: string;
  event_time_field?: string;
  expression?: FeatureExpression;
  window_seconds?: number;
  aggregate?: "sum" | "mean" | "min" | "max" | "count";
}
export type Feature = Omit<
  GovernanceFeature,
  "definition" | "tenant_id" | "user_id"
> & { definition: FeatureDefinition };
export type FeatureValue = GovernanceFeatureValue & {
  available_at?: string;
  value_exact?: string;
};
export interface FeaturePreview {
  values: FeatureValue[];
  total_count: number;
  warnings: string[];
  definition_digest: string;
  source_digest: string;
}
export interface FeatureMaterialization {
  receipt_id: string;
  feature_id: string;
  version: number;
  source_ref_id: string;
  count: number;
  output_ref_id: string;
  available_at: string;
  definition_digest: string;
}
export interface GovernanceExecution {
  id: string;
  status: string;
  domain_id: string;
  domain_kind: string;
  feature_id?: string;
  feature_version?: number;
  output_refs: { asset_id: string; version: number }[];
}
export type Suggestion = Omit<GovernanceSuggestion, "proposal"> & {
  proposal: Record<string, unknown>;
};
export const governanceApi = {
  previewFeature: (
    feature: Pick<Feature, "name" | "type" | "definition" | "source_ref_id">,
    limit = 20,
  ) => http.post<FeaturePreview>("/features/preview", { feature, limit }),
  list: <T>(path: string, cursor = "", extra: Record<string, unknown> = {}) =>
    http.get<Page<T>>(path, { params: { cursor, page_size: 50, ...extra } }),
  get: <T>(path: string) => http.get<T>(path),
  create: <T>(path: string, data: unknown) =>
    withIdempotency(path, data, (key) =>
      http.post<T>(path, data, { headers: { "Idempotency-Key": key } }),
    ),
  update: <T>(path: string, data: unknown) => http.put<T>(path, data),
  action: <T>(path: string, data?: unknown) =>
    withIdempotency(path, data, (key) =>
      http.post<T>(path, data, { headers: { "Idempotency-Key": key } }),
    ),
};
