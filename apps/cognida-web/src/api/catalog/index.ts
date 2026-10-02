import { http } from "@/utils/request";
import type {CatalogAssetTable,CatalogAssetColumn,CatalogGlossary,CatalogGovernance} from '../contracts.generated'
export type Governance=CatalogGovernance
export type CatalogColumn=CatalogAssetColumn
export type CatalogTable=CatalogAssetTable
export type Glossary=Omit<CatalogGlossary,'tenant_id'>
export interface ProfileCard {
  summary: string;
  method: string;
  quality_score: number | null;
  profiled_columns: number;
  active_columns: number;
  suitable_for: string[];
  recommendations: string[];
  risks: { level: string; field?: string; message: string }[];
  related: { id: string; name: string; reason: string }[];
}
export interface CatalogPage<T> {
  items: T[];
  next_cursor: string;
  has_more: boolean;
}
export interface LineageGraph {
  nodes: {
    id: string;
    kind: string;
    name: string;
    asset_id: string;
    unresolved?: boolean;
  }[];
  edges: { from: string; to: string; kind: string }[];
}
export const catalogApi = {
  list: (params: Record<string, unknown>) =>
    http.get<CatalogPage<CatalogTable>>("/catalog/tables", { params }),
  profileCard: (id: string) =>
    http.get<ProfileCard>(`/catalog/tables/${id}/profile-card`),
  get: (id: string) => http.get<CatalogTable>(`/catalog/tables/${id}`),
  govern: (kind: "tables" | "columns", id: string, value: Governance) =>
    http.put(`/catalog/${kind}/${id}`, value),
  sync: (datasource_id: string, profile: boolean) =>
    http.post<{
      tables: number;
      columns: number;
      created: number;
      updated: number;
      deprecated: number;
      warnings: string[];
    }>(
      "/catalog/sync",
      { datasource_id, profile },
      { headers: { "Idempotency-Key": crypto.randomUUID() } },
    ),
  glossary: (cursor = "") =>
    http.get<CatalogPage<Glossary>>("/catalog/glossary", {
      params: { cursor, page_size: 100 },
    }),
  saveTerm: (term: Glossary) =>
    term.id
      ? http.put<Glossary>(`/catalog/glossary/${term.id}`, term)
      : http.post<Glossary>("/catalog/glossary", term),
  deleteTerm: (id: string) => http.delete(`/catalog/glossary/${id}`),
  lineage: (id: string, direction = "both", depth = 3) =>
    http.get<LineageGraph>(`/lineage/${encodeURIComponent(id)}`, {
      params: { direction, depth },
    }),
  impact: (id: string) =>
    http.get<LineageGraph>(`/lineage/${encodeURIComponent(id)}/impact`),
  rebuild: (model_id: string) => http.post("/lineage/rebuild", { model_id }),
};
