import { http } from "@/utils/request";
import { withIdempotency } from "@/utils/idempotent";
export interface SemanticHead {
  id: string;
  name: string;
  description: string;
  status: "draft" | "active" | "deprecated";
  version: number;
}
export type SemanticRow = Record<string, unknown>;
export interface SemanticInput {
  name: string;
  description: string;
  status: string;
  logical_tables: SemanticRow[];
  dimensions: SemanticRow[];
  measures: SemanticRow[];
  metrics: SemanticRow[];
  relations: SemanticRow[];
}
export interface SemanticBundle
  extends Omit<SemanticInput, "name" | "description" | "status"> {
  model: SemanticHead;
}
export interface Coverage {
  model: string;
  covered: number;
  cache_hit: number;
  fallback: number;
  total: number;
  hit_ratio: number;
}
export const semanticApi = {
  list: (status = "all", cursor = "") =>
    http.get<{ items: SemanticHead[]; next_cursor: string }>(
      "/semantic-models",
      { params: { status, cursor, page_size: 100 } },
    ),
  get: (id: string) => http.get<SemanticBundle>(`/semantic-models/${id}`),
  save: (id: string | undefined, data: SemanticInput) =>
    id
      ? http.put<SemanticBundle>(`/semantic-models/${id}`, data)
      : withIdempotency("/semantic-models", data, (key) =>
          http.post<SemanticBundle>("/semantic-models", data, {
            headers: { "Idempotency-Key": key },
          }),
        ),
  transition: (id: string, action: "publish" | "deprecate") =>
    http.post<SemanticBundle>(`/semantic-models/${id}/${action}`),
  coverage: () =>
    http.get<{ items: Coverage[]; enabled: boolean }>("/semantic-coverage"),
  diagnose: (id: string) =>
    http.get<{ enabled: boolean; report?: unknown }>(
      `/semantic-models/${id}/valuemap-diagnosis`,
    ),
};
