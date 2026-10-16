export interface EmbeddingModel {
  available: boolean;
  model_id?: string;
  revision?: string;
  reason?: string;
  max_batch?: number;
  cost_limit_enforced?: boolean;
}
export interface FeatureJob {
  operation_id?: string;
  id: string;
  version: number;
  status: string;
  request: {
    feature_id: string;
    feature_version: number;
    source_ref_id: string;
    mode: string;
    event_from?: string;
    event_to?: string;
    partition_size: number;
  };
  definition_digest: string;
  source_digest: string;
  watermark_before?: string;
  watermark_after?: string;
  embedding: boolean;
  partitions: {
    index: number;
    status: string;
    count: number;
    digest: string;
    output_ref_id: string;
    available_at?: string;
  }[];
  total_rows: number;
  committed_rows: number;
  error?: string;
  created_at: string;
  updated_at: string;
}
export interface DriftSampling {
  limit: number;
  minimum: number;
  seed: string;
}
export interface DriftBaseline {
  name: string;
  source_ref_id: string;
  source_digest: string;
  sampling: DriftSampling;
  approved_by?: number;
  approved_at?: string;
}
export interface DriftMetric {
  field: string;
  method: string;
  baseline_count: number;
  current_count: number;
  effect?: number;
  threshold: number;
  status: string;
  detail?: string;
}
export interface DriftReport {
  baseline_id: string;
  baseline_version: number;
  baseline_ref_id: string;
  current_ref_id: string;
  baseline_digest: string;
  current_digest: string;
  sampling: DriftSampling;
  baseline_rows: number;
  current_rows: number;
  baseline_sample: number;
  current_sample: number;
  metrics: DriftMetric[];
  changed: boolean;
  limitations: string[];
  task_id?: string;
}
export interface DriftMonitor {
  name: string;
  baseline_id: string;
  current_ref_id: string;
  refresh?: {
    mode: "latest_published";
    source_type: string;
    source_id: string;
  };
  interval_seconds: number;
  threshold: number;
  pending_baseline_id?: string;
  last_report_id?: string;
  last_error?: string;
}
export interface DriftRecord<T = DriftBaseline | DriftReport | DriftMonitor> {
  id: string;
  kind: "baseline" | "report" | "monitor";
  version: number;
  status: string;
  body: T;
  next_run_at?: string;
  created_at: string;
  updated_at: string;
}
