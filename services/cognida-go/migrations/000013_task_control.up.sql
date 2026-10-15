-- Durable coordination only. Domain jobs remain in their existing tables.
CREATE TABLE task_control_tasks (
  id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  tenant_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  goal TEXT NOT NULL,
  input_refs JSON NOT NULL,
  budget JSON NOT NULL,
  result_refs JSON NOT NULL,
  request_id VARCHAR(128) NOT NULL,
  idempotency_key CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  request_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  status VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  version BIGINT NOT NULL,
  current_plan_version BIGINT NOT NULL,
  reason VARCHAR(2000) NOT NULL DEFAULT '',
  cancel_requested_at DATETIME(6) NULL,
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_tc_task_scope (tenant_id, user_id, id),
  UNIQUE KEY uq_tc_task_idempotency (tenant_id, user_id, idempotency_key),
  KEY idx_tc_task_page (tenant_id, user_id, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE task_control_plans (
  tenant_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  task_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  version BIGINT NOT NULL,
  snapshot JSON NOT NULL,
  content_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  created_at DATETIME(6) NOT NULL,
  PRIMARY KEY (tenant_id, user_id, task_id, version),
  CONSTRAINT fk_tc_plan_task FOREIGN KEY (tenant_id, user_id, task_id)
    REFERENCES task_control_tasks (tenant_id, user_id, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE task_control_children (
  id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  tenant_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  task_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  plan_version BIGINT NOT NULL,
  node_id VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  attempt INT NOT NULL,
  domain VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  operation_key CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  request_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  domain_id VARCHAR(255) COLLATE utf8mb4_bin NOT NULL DEFAULT '',
  status VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  version BIGINT NOT NULL,
  receipt JSON NULL,
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_tc_child_scope (tenant_id, user_id, task_id, id),
  UNIQUE KEY uq_tc_child_attempt (tenant_id, user_id, task_id, plan_version, node_id, attempt),
  UNIQUE KEY uq_tc_child_operation (tenant_id, user_id, operation_key),
  CONSTRAINT fk_tc_child_plan FOREIGN KEY (tenant_id, user_id, task_id, plan_version)
    REFERENCES task_control_plans (tenant_id, user_id, task_id, version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE task_control_outbox (
  id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  tenant_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  task_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  child_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  operation_key CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  payload JSON NOT NULL,
  payload_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  status VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  version BIGINT NOT NULL,
  reason VARCHAR(2000) NOT NULL DEFAULT '',
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_tc_outbox_child (tenant_id, user_id, child_id),
  UNIQUE KEY uq_tc_outbox_operation (tenant_id, user_id, operation_key),
  KEY idx_tc_outbox_page (tenant_id, user_id, status, id),
  CONSTRAINT fk_tc_outbox_child FOREIGN KEY (tenant_id, user_id, task_id, child_id)
    REFERENCES task_control_children (tenant_id, user_id, task_id, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
