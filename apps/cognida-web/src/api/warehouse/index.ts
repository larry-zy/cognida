import { governanceApi as api, type FeatureExpression } from "@/api/governance";
export interface WarehouseProjection {
  name: string;
  type: "text" | "number" | "boolean";
  expression: FeatureExpression;
}
export interface WarehouseNode {
  id: string;
  layer: "raw" | "detail" | "summary" | "application";
  kind: "source" | "projection" | "left_join_many_to_one" | "aggregate";
  inputs?: string[];
  source_ref?: string;
  columns?: WarehouseProjection[];
  join?: {
    left_keys: string[];
    right_keys: string[];
    right_columns: string[];
    prefix: string;
  };
  group_by?: string[];
  metrics?: {
    name: string;
    field?: string;
    aggregate: string;
    caliber: string;
  }[];
}
export interface WarehouseDefinition {
  name: string;
  subject: string;
  grain: string;
  grain_keys: string[];
  semantic_model_id?: string;
  semantic_version?: number;
  fact_node: string;
  fact_keys: string[];
  event_field?: string;
  update_policy: "upsert";
  delete_policy: "unsupported";
  late_policy: "reject_incremental_use_backfill";
  nodes: WarehouseNode[];
  output_node: string;
}
export interface WarehouseModel {
  id: string;
  version: number;
  definition: WarehouseDefinition;
  digest: string;
  active_run_id?: string;
  active_ref?: string;
  watermark?: string;
}
export interface WarehousePreview {
  definition_digest: string;
  total_rows: number;
  output: { columns: string[]; rows: unknown[][] };
  exact_rows: string[][];
  stages: WarehouseStage[];
  lineage: {
    node: string;
    column: string;
    input_node: string;
    input_column: string;
    source_ref?: string;
  }[];
  impact: { kind: string; id?: string; name?: string; unknown?: boolean }[];
  limitations: string[];
}
export interface WarehouseStage {
  node: string;
  layer: string;
  kind: string;
  input_rows: number;
  output_rows: number;
  digest: string;
}
export interface WarehouseRun {
  id: string;
  version: number;
  status: string;
  request: {
    model_id: string;
    model_version: number;
    mode: string;
    fact_source_ref?: string;
    event_from?: string;
    event_to?: string;
  };
  base_run_id?: string;
  output_ref?: string;
  published_ref?: string;
  watermark?: string;
  error?: string;
  lineage_registered: boolean;
  stages: WarehouseStage[];
  partitions: {
    index: number;
    count: number;
    digest: string;
    output_ref: string;
    committed_at: string;
  }[];
}
export const warehouseApi = {
  models: (cursor = "") =>
    api.list<WarehouseModel>("/warehouse/models", cursor),
  model: (id: string, version = 0) =>
    api.get<WarehouseModel>(
      `/warehouse/models/${encodeURIComponent(id)}?version=${version}`,
    ),
  preview: (definition: WarehouseDefinition) =>
    api.create<WarehousePreview>("/warehouse/preview", {
      definition,
      limit: 20,
    }),
  save: (definition: WarehouseDefinition, model?: WarehouseModel) =>
    api.create<WarehouseModel>(
      model ? `/warehouse/models/${model.id}/versions` : "/warehouse/models",
      { definition, expected_version: model?.version || 0 },
    ),
  runs: (id: string, cursor = "") =>
    api.list<WarehouseRun>("/warehouse/runs", cursor, { model_id: id }),
  run: (id: string) =>
    api.get<WarehouseRun>(`/warehouse/runs/${encodeURIComponent(id)}`),
  createRun: (request: WarehouseRun["request"]) =>
    api.create<WarehouseRun>("/warehouse/runs", request),
  control: (run: WarehouseRun, action: "approve" | "cancel") =>
    api.action<WarehouseRun>(`/warehouse/runs/${run.id}/${action}`, {
      expected_version: run.version,
    }),
};
