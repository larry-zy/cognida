CREATE TABLE warehouse_runs (
 id VARCHAR(36) NOT NULL PRIMARY KEY, tenant_id BIGINT NOT NULL, user_id BIGINT NOT NULL,
 model_id VARCHAR(36) NOT NULL, model_version INT NOT NULL, version INT NOT NULL, status VARCHAR(32) NOT NULL,
 idempotency_key VARCHAR(100) NOT NULL, digest CHAR(64) NOT NULL, body LONGTEXT NOT NULL,
 lease VARCHAR(36) NOT NULL DEFAULT '', lease_until DATETIME(6) NULL, created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL,
 UNIQUE KEY uk_warehouse_run_key(tenant_id,user_id,idempotency_key), KEY idx_warehouse_run_scope(tenant_id,user_id,model_id,id), KEY idx_warehouse_pending(status,updated_at),
 CONSTRAINT ck_warehouse_run_body CHECK(JSON_VALID(body))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE warehouse_events (
 id VARCHAR(36) NOT NULL PRIMARY KEY, run_id VARCHAR(36) NOT NULL, tenant_id BIGINT NOT NULL, user_id BIGINT NOT NULL,
 action VARCHAR(32) NOT NULL, actor_id BIGINT NOT NULL, created_at DATETIME(6) NOT NULL,
 KEY idx_warehouse_event_run(tenant_id,user_id,run_id,created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
