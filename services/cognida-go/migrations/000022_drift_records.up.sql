CREATE TABLE drift_records (
 id VARCHAR(36) NOT NULL PRIMARY KEY,
 tenant_id BIGINT NOT NULL, user_id BIGINT NOT NULL,
 kind VARCHAR(32) NOT NULL, status VARCHAR(32) NOT NULL,
 version INT NOT NULL, idempotency_key VARCHAR(100) NOT NULL,
 request_digest CHAR(64) NOT NULL,
 body LONGTEXT NOT NULL,
 next_run_at DATETIME(6) NULL,
 created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL,
 CONSTRAINT ck_drift_body CHECK (JSON_VALID(body)),
 UNIQUE KEY uk_drift_key (tenant_id,user_id,kind,idempotency_key),
 KEY idx_drift_scope (tenant_id,user_id,kind,id),
 KEY idx_drift_due (kind,status,next_run_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
