CREATE TABLE warehouse_models (
 id VARCHAR(36) NOT NULL PRIMARY KEY, tenant_id BIGINT NOT NULL, user_id BIGINT NOT NULL,
 version INT NOT NULL, active_run_id VARCHAR(36) NOT NULL DEFAULT '', active_ref VARCHAR(100) NOT NULL DEFAULT '',
 watermark DATETIME(6) NULL, idempotency_key VARCHAR(100) NOT NULL, created_at DATETIME(6) NOT NULL,
 UNIQUE KEY uk_warehouse_model_key(tenant_id,user_id,idempotency_key), KEY idx_warehouse_model_scope(tenant_id,user_id,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE warehouse_model_versions (
 model_id VARCHAR(36) NOT NULL, version INT NOT NULL, definition LONGTEXT NOT NULL, digest CHAR(64) NOT NULL,
 idempotency_key VARCHAR(100) NOT NULL, created_at DATETIME(6) NOT NULL,
 PRIMARY KEY(model_id,version), UNIQUE KEY uk_warehouse_version_key(model_id,idempotency_key),
 CONSTRAINT ck_warehouse_definition CHECK(JSON_VALID(definition))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
