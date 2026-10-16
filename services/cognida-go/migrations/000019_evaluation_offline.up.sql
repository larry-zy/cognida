-- Immutable offline answer snapshots and fenced, resumable scoring state.
-- evaluation_tasks/results remain the existing user-visible task/result tables.
CREATE TABLE evaluation_offline_runs (
  id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  tenant_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  operation_key CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  payload_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  status VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  version BIGINT NOT NULL,
  epoch BIGINT NOT NULL DEFAULT 0,
  lease_until_us BIGINT NOT NULL DEFAULT 0,
  deadline_us BIGINT NOT NULL,
  body JSON NOT NULL,
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_evaluation_offline_operation (tenant_id,user_id,operation_key),
  KEY idx_evaluation_offline_pending (status,lease_until_us,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
