import { http } from "@/utils/request";
import { withIdempotency } from "@/utils/idempotent";
export interface StrategyVersion {
  id: string;
  slot: string;
  kind: string;
  version: number;
  digest: string;
  baseline_id?: string;
  body: Record<string, unknown>;
  evidence_refs: string[];
  disabled_at?: string;
  created_at: string;
  gates: Record<string, number>;
}
export interface StrategyHead {
  active_id: string;
  baseline_id: string;
  revision: number;
  kind: string;
  slot: string;
}
export interface StrategyEvent {
  id: string;
  kind: string;
  version_id: string;
  created_at: string;
  payload: Record<string, any>;
}
export interface StrategyPage<T> {
  items: T[];
  next_cursor: string;
}
const root = (slot: string) => `/strategies/${encodeURIComponent(slot)}`;
export const strategyApi = {
  consumers: () => http.get<Record<string, string>>("/strategies/consumers"),
  head: (slot: string) => http.get<StrategyHead>(root(slot)),
  versions: (slot: string, cursor = "") =>
    http.get<StrategyPage<StrategyVersion>>(root(slot) + "/versions", {
      params: { cursor, page_size: 20 },
    }),
  version: (slot: string, id: string) =>
    http.get<StrategyVersion>(
      root(slot) + "/versions/" + encodeURIComponent(id),
    ),
  events: (slot: string, cursor = "", kind = "") =>
    http.get<StrategyPage<StrategyEvent>>(root(slot) + "/events", {
      params: { cursor, page_size: 50, ...(kind ? { kind } : {}) },
    }),
  write: <T = StrategyEvent>(slot: string, path: string, body: unknown) =>
    withIdempotency("strategy-" + slot + "-" + path, body, (key) =>
      http.post<T>(root(slot) + path, body, {
        headers: { "Idempotency-Key": key },
      }),
    ),
};
