ALTER TABLE task_control_tasks
  ADD COLUMN execution_plan_version BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN execution_key CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  ADD COLUMN execution_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  ADD COLUMN started_at DATETIME(6) NULL,
  ADD COLUMN deadline_at DATETIME(6) NULL,
  ADD COLUMN finished_at DATETIME(6) NULL;

ALTER TABLE task_control_children
  ADD COLUMN domain_status VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'unknown',
  ADD COLUMN output_refs JSON NULL,
  ADD COLUMN error VARCHAR(2000) NOT NULL DEFAULT '',
  ADD COLUMN observed_at DATETIME(6) NULL,
  ADD COLUMN calls_reserved INT NOT NULL DEFAULT 0,
  ADD COLUMN cost_micros_reserved BIGINT NOT NULL DEFAULT 0;

-- Leases use the database clock and epoch microseconds, independent of DSN timezone.
-- Their heartbeats deliberately do not change the public task's optimistic version.
CREATE TABLE task_control_leases (
  tenant_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  task_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  owner VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  epoch BIGINT NOT NULL DEFAULT 0,
  lease_until_us BIGINT NOT NULL DEFAULT 0,
  next_poll_us BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (tenant_id, user_id, task_id),
  KEY idx_tc_lease_due (next_poll_us, lease_until_us, task_id),
  CONSTRAINT fk_tc_lease_task FOREIGN KEY (tenant_id, user_id, task_id)
    REFERENCES task_control_tasks (tenant_id, user_id, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
