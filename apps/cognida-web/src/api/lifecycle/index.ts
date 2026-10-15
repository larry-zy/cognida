import type { LifecycleInput, LifecycleRefID } from "../contracts.generated";
import { http } from "@/utils/request";
import { withIdempotency } from "@/utils/idempotent";
export type RefID = LifecycleRefID & { kind?: string };
export interface AssetRef {
  asset_id: string;
  version: number;
  kind: string;
  status: string;
  name?: string;
  producer_id: string;
  parents: RefID[];
  policy?: { sensitivity: string; scopes: string[]; approved: boolean };
  created_at: string;
}
export interface Gate {
  status: string;
  reason: string;
  evidence_refs: RefID[];
}
export interface Decision {
  evidence_refs?: RefID[];
  method?: string;
  assumptions?: string[];
  risks?: string[];
  verification_status?: string;
  confidence?: string | number | null;
  recommendation?: string;
}
export interface StageOutput {
  refs: RefID[];
  summary: string;
  gate: Gate;
  decision?: Decision;
}
export interface Invocation {
  id: string;
  agent_id?: string;
  template_id?: string;
  status: string;
  created_at: string;
  updated_at: string;
  error?: string;
  output?: StageOutput;
}
export interface Stage {
  id: string;
  stage_key: string;
  agent_id: string;
  status: string;
  attempt: number;
  created_at?: string;
  updated_at?: string;
  error?: string;
  output?: StageOutput;
}
export interface Approval {
  id: string;
  status: string;
  reason?: string;
  expires_at?: string;
}
export interface RunDetail {
  run: Invocation;
  stages: Stage[];
  approvals: Approval[];
}
export interface Agent {
  id: string;
  name: string;
  input_kinds: string[];
  output_kinds: string[];
  tool_allowlist: string[];
  risk_class: string;
}
export interface Template {
  id: string;
  name: string;
  version: number;
  stages: { id: string; agent_id: string; depends_on: string[] }[];
}
export type CleanOperation = "trim" | "normalize_ws" | "drop_empty" | "dedup" | "text_cleaner" | "pii_masker";
export type EnrichmentFeature = "tags" | "entities" | "keywords" | "summary" | "category" | "sentiment" | "sensitivity";
export interface ExpertOptions {
  min_score?: number;
  operations?: CleanOperation[];
  columns?: string[];
  keep?: "first" | "last";
  method?: "rules" | "llm";
  labels?: { name: string; keywords: string[] }[];
  max_keywords?: number;
  max_summary_chars?: number;
  features?: EnrichmentFeature[];
  knowledge_base_id?: string;
  catalog_asset_id?: string;
  prompt?: string;
}
export interface PublicationTarget { id: string; name: string; status?: number }
export interface PublicationTargetPage { items: PublicationTarget[]; total: number; page: number; size: number }
export interface Page<T> {
  items: T[];
  next_cursor: string;
  has_more: boolean;
}
export const lifecycleApi = {
  // The existing knowledge-base endpoint uses numbered pages; do not truncate
  // publication targets to the shared store's first cached page.
  knowledgeBases: (page = 1) =>
    http.get<PublicationTargetPage>("/knowledge-bases", { params: { page, page_size: 100 } }),
  agents: () => http.get<{ agents: Agent[] }>("/lifecycle/agents"),
  templates: () => http.get<{ templates: Template[] }>("/lifecycle/templates"),
  assets: (kind = "", cursor = "") =>
    http.get<Page<AssetRef>>("/lifecycle/assets", {
      params: { kind, cursor, page_size: 100 },
    }),
  asset: (id: string, version: number) =>
    http.get<{ ref: AssetRef; content: unknown }>(
      `/lifecycle/assets/${encodeURIComponent(id)}`,
      { params: { version } },
    ),
  createAsset: (input: LifecycleInput, name: string, source?: unknown) => {
    const data = source ? { source, name } : { input, name };
    return withIdempotency("/lifecycle/assets", data, (key) =>
      http.post<AssetRef>("/lifecycle/assets", data, {
        headers: { "Idempotency-Key": key },
      }),
    );
  },
  list: (kind: "runs" | "invocations", cursor = "") =>
    http.get<Page<Invocation>>(`/lifecycle/${kind}`, {
      params: { cursor, page_size: 50 },
    }),
  get: (kind: "runs" | "invocations", id: string) =>
    http.get<RunDetail | { invocation: Invocation; approvals: Approval[] }>(
      `/lifecycle/${kind}/${id}`,
    ),
  start: (kind: "runs" | "invocations", data: unknown) =>
    withIdempotency(`/lifecycle/${kind}`, data, (key) =>
      http.post<Invocation>(`/lifecycle/${kind}`, data, {
        headers: { "Idempotency-Key": key },
      }),
    ),
  action: (
    kind: "runs" | "invocations",
    id: string,
    action: "retry" | "cancel",
  ) => http.post(`/lifecycle/${kind}/${id}/${action}`),
  approve: (id: string, decision: string, reason: string) =>
    http.post(`/lifecycle/approvals/${id}/resolve`, { decision, reason }),
};
