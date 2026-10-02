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
export type Provenance = GovernanceProvenance;
export type AnnotationTask = GovernanceAnnotationTask;
export type AnnotationItem = GovernanceAnnotationItem;
export type TagTask = GovernanceTaggingTask;
export type TagResult = Omit<GovernanceTaggingResult, "entities"> & {
  entities: Record<string, unknown>[] | null;
};
export type Feature = Omit<
  GovernanceFeature,
  "definition" | "tenant_id" | "user_id"
> & { definition: { kind: string; field?: string; entity_field?: string } };
export type FeatureValue = GovernanceFeatureValue;
export type Suggestion = Omit<GovernanceSuggestion, "proposal"> & {
  proposal: Record<string, unknown>;
};
export const governanceApi = {
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
